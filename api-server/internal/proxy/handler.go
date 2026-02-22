package proxy

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/events"
	"github.com/xiaoboyu/tokengate/api-server/internal/pricing"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

const defaultAnthropicVersion = "2023-06-01"

// ProxyHandler handles reverse proxy requests to upstream AI providers.
type ProxyHandler struct {
	providerKeySvc *services.ProviderKeyService
	eventQueue     *events.EventQueue
	pricingEngine  *pricing.PricingEngine
	httpClient     *http.Client
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(providerKeySvc *services.ProviderKeyService, eventQueue *events.EventQueue, pricingEngine *pricing.PricingEngine) *ProxyHandler {
	return &ProxyHandler{
		providerKeySvc: providerKeySvc,
		eventQueue:     eventQueue,
		pricingEngine:  pricingEngine,
		// No overall Timeout: streaming responses can run arbitrarily long.
		// Client disconnect is handled via context cancellation on the upstream request.
		// ResponseHeaderTimeout ensures we fail fast if the upstream is unresponsive.
		httpClient: &http.Client{
			Transport: &http.Transport{
				ResponseHeaderTimeout: 30 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
			},
		},
	}
}

// HandleProxy handles proxy requests to any supported upstream provider.
// It resolves the provider from the X-TokenGate-Provider header or request path,
// attempts BYOK auth, and falls back to pass-through when no key is configured.
func (h *ProxyHandler) HandleProxy(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")
	keyID, _ := c.Get("key_id")
	keyIDStr, _ := keyID.(string)

	fmt.Println("------- HandleProxy called ------- tenantID:", tenantID, "keyID:", keyIDStr)

	// Resolve provider from X-TokenGate-Provider header or path prefix.
	provider := resolveProvider(c.GetHeader("X-TokenGate-Provider"), c.Request.URL.Path)
	fmt.Println("------- Resolved provider:", provider)

	// Pre-check budget before forwarding.
	if h.pricingEngine != nil {
		status, err := h.pricingEngine.PreCheckBudget(c.Request.Context(), tenantID, keyIDStr, time.Now())
		if err != nil {
			var budgetErr *pricing.ErrBudgetExceeded
			if errors.As(err, &budgetErr) {
				c.JSON(http.StatusPaymentRequired, gin.H{"error": gin.H{
					"type":    "tg_budget_exceeded",
					"message": fmt.Sprintf("Budget limit exceeded for period=%s. Limit: %s, Current: %s", budgetErr.Period, budgetErr.LimitAmount.StringFixed(4), budgetErr.CurrentSpend.StringFixed(4)),
				}})
				return
			}
		}
		if status != nil && status.AtWarning {
			c.Header("X-Tokengate-Budget-Warning", "true")
			c.Header("X-Tokengate-Budget-Limit", status.LimitAmount.StringFixed(4))
			c.Header("X-Tokengate-Budget-Used", status.CurrentSpend.StringFixed(4))
			c.Header("X-Tokengate-Budget-Period", status.Period)
			c.Header("X-Tokengate-Budget-Scope", status.Scope)
		}
	}
	fmt.Println("------- Budget pre-check passed -------")

	// BYOK attempt: get the active provider key for this tenant+provider.
	// If ENABLED_BYOK_LOOKUP is not "true", skip lookup and use pass-through mode.
	var byokKey []byte
	if strings.EqualFold(os.Getenv("ENABLED_BYOK_LOOKUP"), "true") {
		plaintextKey, err := h.providerKeySvc.GetActiveKey(c.Request.Context(), tenantID, string(provider))
		if err != nil {
			if !errors.Is(err, services.ErrNoActiveKey) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
					"type":    "tg_internal_error",
					"message": "Failed to retrieve provider key.",
				}})
				return
			}
			// ErrNoActiveKey → pass-through mode; byokKey stays nil.
		} else {
			fmt.Println("------ len of retrieved BYOK key:", len(plaintextKey), "plaintext: ", string(plaintextKey))
			// Only use the retrieved key when it's non-empty; otherwise fall back to pass-through mode.
			if len(plaintextKey) > 0 {
				fmt.Println("------ Set byokKey -----")
				byokKey = plaintextKey
			}
		}
	}
	fmt.Println("------ BYOK byokKey len: ", len(byokKey))
	fmt.Println("------- BYOK lookup enabled:", strings.EqualFold(os.Getenv("ENABLED_BYOK_LOOKUP"), "true"), "byokKey is set:", byokKey != nil)

	// Read the request body.
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "tg_bad_request",
			"message": "Failed to read request body.",
		}})
		return
	}

	// Build the upstream URL: base + provider-stripped path + query string.
	upstreamURL := upstreamBase(provider) + upstreamPath(provider, c.Request.URL.Path)
	if c.Request.URL.RawQuery != "" {
		upstreamURL += "?" + c.Request.URL.RawQuery
	}

	upstreamReq, err := http.NewRequestWithContext(
		c.Request.Context(), c.Request.Method, upstreamURL, newBodyReader(bodyBytes),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"type":    "tg_internal_error",
			"message": "Failed to build upstream request.",
		}})
		return
	}

	// Copy safe Anthropic-specific headers from the client request.
	copyClientHeaders(c.Request, upstreamReq)

	// Security: strip all TokenGate headers — never forward them upstream.
	stripTokengateHeaders(upstreamReq)

	if byokKey != nil {
		// BYOK mode: apply provider-specific auth using the stored key.
		applyByokAuth(provider, byokKey, upstreamReq)
	} else {
		// Pass-through mode: forward provider auth headers from the client as-is.
		// This supports Claude subscription users and similar direct-auth scenarios.
		for _, hdr := range []string{"Authorization", "x-api-key", "x-goog-api-key"} {
			if v := c.Request.Header.Get(hdr); v != "" {
				upstreamReq.Header.Set(hdr, v)
			}
		}
	}

	// Forward Content-Type from the original request.
	if ct := c.Request.Header.Get("Content-Type"); ct != "" {
		upstreamReq.Header.Set("Content-Type", ct)
	}

	// For Anthropic, set a default API version if the client didn't supply one.
	if provider == ProviderAnthropic && upstreamReq.Header.Get("anthropic-version") == "" {
		upstreamReq.Header.Set("anthropic-version", defaultAnthropicVersion)
	}

	resp, err := h.httpClient.Do(upstreamReq)
	if err != nil {
		if c.Request.Context().Err() != nil {
			return // client disconnected; nothing to write
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"type":    "tg_upstream_error",
			"message": "Upstream request failed.",
		}})
		return
	}
	defer resp.Body.Close()

	// Determine from the upstream response whether this is an SSE stream.
	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")

	// Copy upstream response headers.
	// Use Add (not Set) to preserve multi-value headers (e.g. Set-Cookie).
	// Skip Content-Length: for SSE it is absent; for errors forwarding a wrong
	// Content-Length alongside a modified body confuses clients.
	for key, vals := range resp.Header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, v := range vals {
			c.Writer.Header().Add(key, v)
		}
	}

	// For SSE responses, override headers that matter for streaming to work
	// correctly through proxies (Railway/Nginx) and browsers.
	if isSSE {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.Header().Del("Content-Length")
	}

	c.Writer.WriteHeader(resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		io.Copy(c.Writer, resp.Body)
		return
	}

	now := time.Now()
	var counts TokenCounts

	if provider == ProviderAnthropic {
		// Anthropic: parse SSE or JSON body to extract token counts for billing.
		if isSSE {
			counts, err = ParseSSE(c.Request.Context(), resp.Body, c.Writer)
			if err != nil {
				log.Printf("proxy: SSE parse error (tenant=%d): %v", tenantID, err)
			}
		} else {
			respBody, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				log.Printf("proxy: read response body (tenant=%d): %v", tenantID, readErr)
				return
			}
			c.Writer.Write(respBody)
			counts = extractTokensFromJSON(respBody)
		}
	} else {
		// Non-Anthropic providers: pass through without token extraction for now.
		if isSSE {
			flusher, canFlush := c.Writer.(http.Flusher)
			scanner := bufio.NewScanner(resp.Body)
			buf := make([]byte, 0, 64*1024)
			scanner.Buffer(buf, 1024*1024)
			for scanner.Scan() {
				line := scanner.Text()
				if _, writeErr := io.WriteString(c.Writer, line+"\n"); writeErr != nil {
					break // client disconnected
				}
				if canFlush {
					flusher.Flush()
				}
			}
		} else {
			io.Copy(c.Writer, resp.Body)
		}
	}

	// Publish usage event to Redis Streams (fire-and-forget).
	if counts.MessageID != "" || counts.InputTokens > 0 || counts.OutputTokens > 0 {
		msg := events.UsageEventMsg{
			TenantID:            tenantID,
			KeyID:               keyIDStr,
			Provider:            string(provider),
			Model:               counts.Model,
			InputTokens:         counts.InputTokens,
			OutputTokens:        counts.OutputTokens,
			CacheCreationTokens: counts.CacheCreationTokens,
			CacheReadTokens:     counts.CacheReadTokens,
			MessageID:           counts.MessageID,
			Timestamp:           now,
		}
		if pubErr := h.eventQueue.Publish(c.Request.Context(), msg); pubErr != nil {
			log.Printf("proxy: publish usage event (tenant=%d): %v", tenantID, pubErr)
		}
	}
}

// HandleMessages is a backward-compatible alias for HandleProxy (Anthropic /v1/messages).
func (h *ProxyHandler) HandleMessages(c *gin.Context) {
	h.HandleProxy(c)
}

// HandleModels handles GET /v1/models — Anthropic model list passthrough.
func (h *ProxyHandler) HandleModels(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")
	fmt.Println("------- HandleModels called ------- tenantID:", tenantID)

	// Resolve API key: prefer BYOK, fall back to client's x-api-key (pass-through).
	var resolvedKey string
	plaintextKey, err := h.providerKeySvc.GetActiveKey(c.Request.Context(), tenantID, "anthropic")
	if err != nil {
		if !errors.Is(err, services.ErrNoActiveKey) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve provider key"})
			return
		}
		// Pass-through mode: forward the client's key.
		resolvedKey = c.Request.Header.Get("x-api-key")
		if resolvedKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no_active_provider_key"})
			return
		}
	} else {
		resolvedKey = string(plaintextKey)
	}

	upstreamURL := upstreamBase(ProviderAnthropic) + "/v1/models"
	upstreamReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, upstreamURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build upstream request"})
		return
	}

	upstreamReq.Header.Set("x-api-key", resolvedKey)
	upstreamReq.Header.Set("anthropic-version", defaultAnthropicVersion)
	if v := c.GetHeader("anthropic-version"); v != "" {
		upstreamReq.Header.Set("anthropic-version", v)
	}

	resp, err := h.httpClient.Do(upstreamReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream request failed"})
		return
	}
	defer resp.Body.Close()

	for key, vals := range resp.Header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, v := range vals {
			c.Writer.Header().Add(key, v)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

// copyClientHeaders copies safe Anthropic-specific headers from the client request
// to the upstream request.
func copyClientHeaders(src *http.Request, dst *http.Request) {
	for _, h := range []string{"anthropic-version", "anthropic-beta", "accept", "anthropic-dangerous-direct-browser-access"} {
		if v := src.Header.Get(h); v != "" {
			dst.Header.Set(h, v)
		}
	}
}
