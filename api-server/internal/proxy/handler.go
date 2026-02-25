package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/xiaoboyu/tokengate/api-server/internal/events"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/pricing"
	"github.com/xiaoboyu/tokengate/api-server/internal/ratelimit"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

const defaultAnthropicVersion = "2023-06-01"

// ProxyHandler handles reverse proxy requests to upstream AI providers.
type ProxyHandler struct {
	providerKeySvc *services.ProviderKeyService
	eventQueue     *events.EventQueue
	pricingEngine  *pricing.PricingEngine
	rateLimiter    *ratelimit.Limiter
	httpClient     *http.Client
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(providerKeySvc *services.ProviderKeyService, eventQueue *events.EventQueue, pricingEngine *pricing.PricingEngine, rateLimiter *ratelimit.Limiter) *ProxyHandler {
	return &ProxyHandler{
		providerKeySvc: providerKeySvc,
		eventQueue:     eventQueue,
		pricingEngine:  pricingEngine,
		rateLimiter:    rateLimiter,
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

// ─── Shared helpers ──────────────────────────────────────────────────────────

// determineBillable returns true when the request should be recorded as billable API usage.
func determineBillable(billingMode string) bool {
	return models.IsBillableMode(billingMode)
}

// checkRateLimit performs a rate limit check. If the limit is exceeded it writes
// a 429 response and returns true. Returns false when the request may proceed.
func (h *ProxyHandler) checkRateLimit(c *gin.Context, tenantID uint, keyID string, provider Provider, model string, bodyLen int, maxTokens int) (exceeded bool) {
	if h.rateLimiter == nil {
		return false
	}
	estimatedInputTokens := int64(bodyLen / 4)
	if estimatedInputTokens < 1 {
		estimatedInputTokens = 1
	}
	result, _ := h.rateLimiter.Check(c.Request.Context(), tenantID, keyID, string(provider), model, estimatedInputTokens, maxTokens)
	if result != nil && result.Exceeded {
		retryAfterSec := result.RetryAfterMs / 1000
		if retryAfterSec < 1 {
			retryAfterSec = 1
		}
		c.Header("Retry-After", fmt.Sprintf("%d", retryAfterSec))
		c.Header("X-Tokengate-RateLimit-Metric", result.Metric)
		c.JSON(http.StatusTooManyRequests, gin.H{"type": "error", "error": gin.H{
			"type":    "rate_limit_error",
			"message": fmt.Sprintf("Rate limit exceeded: %d %s for %s", result.Limit, result.Metric, model),
		}})
		return true
	}
	return false
}

// preCheckBudget performs a budget pre-check and reserves worst-case output spend.
// If the budget is exceeded it writes a 402 response and returns ok=false.
func (h *ProxyHandler) preCheckBudget(c *gin.Context, tenantID uint, keyID string, provider Provider, model string, maxTokens int) (reservedAmount decimal.Decimal, ok bool) {
	reservedAmount = decimal.Zero
	if h.pricingEngine == nil {
		return reservedAmount, true
	}
	status, err := h.pricingEngine.PreCheckBudget(c.Request.Context(), tenantID, keyID, string(provider), time.Now())
	if err != nil {
		var budgetErr *pricing.ErrBudgetExceeded
		if errors.As(err, &budgetErr) {
			c.JSON(http.StatusPaymentRequired, gin.H{"error": gin.H{
				"type":    "tg_budget_exceeded",
				"message": fmt.Sprintf("Budget limit exceeded for period=%s. Limit: %s, Current: %s", budgetErr.Period, budgetErr.LimitAmount.StringFixed(4), budgetErr.CurrentSpend.StringFixed(4)),
			}})
			return reservedAmount, false
		}
	}
	if status != nil && status.AtWarning {
		c.Header("X-Tokengate-Budget-Warning", "true")
		c.Header("X-Tokengate-Budget-Limit", status.LimitAmount.StringFixed(4))
		c.Header("X-Tokengate-Budget-Used", status.CurrentSpend.StringFixed(4))
		c.Header("X-Tokengate-Budget-Period", status.Period)
		c.Header("X-Tokengate-Budget-Scope", status.Scope)
	}
	if maxTokens > 0 {
		reserved, _ := h.pricingEngine.ReserveSpend(c.Request.Context(), tenantID, keyID, string(provider), model, maxTokens)
		reservedAmount = reserved
	}
	return reservedAmount, true
}

// resolveAuth fetches the BYOK key when authMethod is BYOK. For other auth
// methods it returns nil. Returns ok=false (and writes a 500 response) on internal errors.
func (h *ProxyHandler) resolveAuth(c *gin.Context, tenantID uint, provider Provider, authMethod string) (byokKey []byte, ok bool) {
	if authMethod != models.AuthMethodBYOK {
		return nil, true
	}
	plaintextKey, err := h.providerKeySvc.GetActiveKey(c.Request.Context(), tenantID, string(provider))
	if err != nil {
		if !errors.Is(err, services.ErrNoActiveKey) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"type":    "tg_internal_error",
				"message": "Failed to retrieve provider key.",
			}})
			return nil, false
		}
		return nil, true
	}
	if len(plaintextKey) > 0 {
		return plaintextKey, true
	}
	return nil, true
}

// buildUpstreamRequest creates an HTTP request for the upstream provider with
// correct headers and authentication applied.
func (h *ProxyHandler) buildUpstreamRequest(ctx context.Context, method, url string, body []byte, provider Provider, byokKey []byte, clientReq *http.Request) (*http.Request, error) {
	upstreamReq, err := http.NewRequestWithContext(ctx, method, url, newBodyReader(body))
	if err != nil {
		return nil, err
	}

	// Copy safe provider-specific headers from the client request.
	copyClientHeadersForProvider(provider, clientReq, upstreamReq)

	// Security: strip all TokenGate headers — never forward them upstream.
	stripTokengateHeaders(upstreamReq)

	if byokKey != nil {
		applyByokAuth(provider, byokKey, upstreamReq)
	} else {
		// Pass-through mode: forward provider auth headers from the client as-is.
		for _, hdr := range []string{"Authorization", "x-api-key", "x-goog-api-key"} {
			if v := clientReq.Header.Get(hdr); v != "" {
				upstreamReq.Header.Set(hdr, v)
			}
		}
	}

	// Forward Content-Type from the original request.
	if ct := clientReq.Header.Get("Content-Type"); ct != "" {
		upstreamReq.Header.Set("Content-Type", ct)
	}

	// For Anthropic, set a default API version if the client didn't supply one.
	if provider == ProviderAnthropic && upstreamReq.Header.Get("anthropic-version") == "" {
		upstreamReq.Header.Set("anthropic-version", defaultAnthropicVersion)
	}

	return upstreamReq, nil
}

// copyAndWriteResponseHeaders copies upstream response headers to the client writer
// and sets SSE-specific overrides when isSSE is true.
func copyAndWriteResponseHeaders(resp *http.Response, w gin.ResponseWriter, isSSE bool) {
	for key, vals := range resp.Header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	if isSSE {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.Header().Del("Content-Length")
	}
}

// reconcilePostResponse adjusts rate limit OTPM counters and releases the spend reservation.
func (h *ProxyHandler) reconcilePostResponse(ctx context.Context, tenantID uint, keyID string, provider Provider, model string, maxTokens int, outputTokens int64, reserved decimal.Decimal) {
	if h.rateLimiter != nil && maxTokens > 0 {
		h.rateLimiter.Reconcile(ctx, tenantID, keyID, string(provider), model, outputTokens, maxTokens)
	}
	if h.pricingEngine != nil && !reserved.IsZero() {
		h.pricingEngine.ReleaseReservation(ctx, tenantID, keyID, reserved)
	}
}

// publishUsageEvent publishes a usage event to Redis Streams (fire-and-forget).
func (h *ProxyHandler) publishUsageEvent(ctx context.Context, tenantID uint, keyID string, provider Provider, counts TokenCounts, billed bool, ts time.Time) {
	if tenantID == 0 {
		return
	}
	if counts.MessageID == "" && counts.InputTokens == 0 && counts.OutputTokens == 0 {
		return
	}
	msg := events.UsageEventMsg{
		TenantID:            tenantID,
		KeyID:               keyID,
		Provider:            string(provider),
		Model:               counts.Model,
		InputTokens:         counts.InputTokens,
		OutputTokens:        counts.OutputTokens,
		CacheCreationTokens: counts.CacheCreationTokens,
		CacheReadTokens:     counts.CacheReadTokens,
		MessageID:           counts.MessageID,
		Timestamp:           ts,
		APIUsageBilled:      billed,
	}
	if pubErr := h.eventQueue.Publish(ctx, msg); pubErr != nil {
		log.Printf("proxy: publish usage event (tenant=%d): %v", tenantID, pubErr)
	}
}

// ─── HandleProxy ─────────────────────────────────────────────────────────────

// HandleProxy handles proxy requests to any supported upstream provider.
// It resolves the provider from the X-TokenGate-Provider header or request path,
// attempts BYOK auth, and falls back to pass-through when no key is configured.
func (h *ProxyHandler) HandleProxy(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")
	keyID, _ := c.Get("key_id")
	keyIDStr, _ := keyID.(string)

	fmt.Println("------- HandleProxy called ------- tenantID:", tenantID, "keyID:", keyIDStr)

	// Read provider, auth_method, and billing_mode from context (set by auth middleware from the API key record).
	provider := Provider(c.GetString("provider"))
	authMethod := c.GetString("auth_method")
	billingMode := c.GetString("billing_mode")
	if provider == "" {
		provider = ProviderAnthropic
	}
	fmt.Println("------- provider:", provider, "auth_method:", authMethod, "billing_mode:", billingMode)

	apiUsageBilled := determineBillable(billingMode)

	// Read the request body early so we can parse model/max_tokens for rate limiting and spend reservation.
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "tg_bad_request",
			"message": "Failed to read request body.",
		}})
		return
	}
	reqMeta := parseRequestMeta(bodyBytes)

	if h.checkRateLimit(c, tenantID, keyIDStr, provider, reqMeta.Model, len(bodyBytes), reqMeta.MaxTokens) {
		return
	}

	reservedAmount, ok := h.preCheckBudget(c, tenantID, keyIDStr, provider, reqMeta.Model, reqMeta.MaxTokens)
	if !ok {
		return
	}
	fmt.Println("------- Budget pre-check passed -------")

	byokKey, ok := h.resolveAuth(c, tenantID, provider, authMethod)
	if !ok {
		return
	}
	fmt.Println("------ auth_method:", authMethod, "byokKey set:", byokKey != nil)

	// Build the upstream URL: base + provider-stripped path + query string.
	upstreamURL := upstreamBase(provider) + upstreamPath(provider, c.Request.URL.Path)
	if c.Request.URL.RawQuery != "" {
		upstreamURL += "?" + c.Request.URL.RawQuery
	}

	upstreamReq, err := h.buildUpstreamRequest(c.Request.Context(), c.Request.Method, upstreamURL, bodyBytes, provider, byokKey, c.Request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"type":    "tg_internal_error",
			"message": "Failed to build upstream request.",
		}})
		return
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

	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	copyAndWriteResponseHeaders(resp, c.Writer, isSSE)
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

	h.reconcilePostResponse(c.Request.Context(), tenantID, keyIDStr, provider, reqMeta.Model, reqMeta.MaxTokens, counts.OutputTokens, reservedAmount)
	h.publishUsageEvent(c.Request.Context(), tenantID, keyIDStr, provider, counts, apiUsageBilled, now)
}

// HandleMessages is a backward-compatible alias for HandleProxy (Anthropic /v1/messages).
func (h *ProxyHandler) HandleMessages(c *gin.Context) {
	h.HandleProxy(c)
}

// HandleModels handles GET /v1/models — Anthropic model list passthrough.
func (h *ProxyHandler) HandleModels(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")
	authMethod := c.GetString("auth_method")
	fmt.Println("------- HandleModels called ------- tenantID:", tenantID, "auth_method:", authMethod)

	// Resolve API key based on auth method.
	var resolvedKey string
	if authMethod == models.AuthMethodBYOK {
		plaintextKey, err := h.providerKeySvc.GetActiveKey(c.Request.Context(), tenantID, "anthropic")
		if err != nil {
			if !errors.Is(err, services.ErrNoActiveKey) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve provider key"})
				return
			}
			// ErrNoActiveKey → fall through to pass-through mode below.
		} else {
			resolvedKey = string(plaintextKey)
		}
	}

	// Pass-through: use the client's own key when no BYOK key was resolved.
	if resolvedKey == "" {
		if v := c.Request.Header.Get("x-api-key"); v != "" {
			resolvedKey = v
		} else if v := c.Request.Header.Get("Authorization"); v != "" {
			resolvedKey = strings.TrimPrefix(v, "Bearer ")
		}
		if resolvedKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no_active_provider_key"})
			return
		}
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
// to the upstream request. Kept for backward compatibility.
func copyClientHeaders(src *http.Request, dst *http.Request) {
	copyClientHeadersForProvider(ProviderAnthropic, src, dst)
}
