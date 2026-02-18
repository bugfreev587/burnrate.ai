package proxy

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/events"
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/services"
)

const anthropicBaseURL = "https://api.anthropic.com"
const defaultAnthropicVersion = "2023-06-01"

// ProxyHandler handles reverse proxy requests to Anthropic.
type ProxyHandler struct {
	providerKeySvc *services.ProviderKeyService
	eventQueue     *events.EventQueue
	httpClient     *http.Client
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(providerKeySvc *services.ProviderKeyService, eventQueue *events.EventQueue) *ProxyHandler {
	return &ProxyHandler{
		providerKeySvc: providerKeySvc,
		eventQueue:     eventQueue,
		// No overall Timeout: streaming responses can run arbitrarily long.
		// Client disconnect is handled via context cancellation on the upstream request.
		// ResponseHeaderTimeout ensures we fail fast if Anthropic is unresponsive.
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

// HandleMessages handles POST /v1/messages — the main Anthropic proxy endpoint.
func (h *ProxyHandler) HandleMessages(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")

	// Fetch the active Anthropic key for this tenant.
	plaintextKey, err := h.providerKeySvc.GetActiveKey(c.Request.Context(), tenantID, "anthropic")
	if err != nil {
		if errors.Is(err, services.ErrNoActiveKey) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "no_active_provider_key",
				"message": "No active Anthropic provider key configured for your tenant. " +
					"Add and activate one via the Management dashboard.",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve provider key"})
		return
	}

	// Read the request body to detect streaming intent and forward it.
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	// Build upstream request, bound to the client's context so that if the
	// client disconnects, the upstream request is cancelled automatically.
	upstreamURL := anthropicBaseURL + "/v1/messages"
	upstreamReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstreamURL, newBodyReader(bodyBytes))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build upstream request"})
		return
	}

	copyClientHeaders(c.Request, upstreamReq)
	upstreamReq.Header.Set("Authorization", "Bearer "+string(plaintextKey))
	upstreamReq.Header.Set("x-api-key", string(plaintextKey))
	upstreamReq.Header.Set("Content-Type", "application/json")
	if upstreamReq.Header.Get("anthropic-version") == "" {
		upstreamReq.Header.Set("anthropic-version", defaultAnthropicVersion)
	}

	resp, err := h.httpClient.Do(upstreamReq)
	if err != nil {
		if c.Request.Context().Err() != nil {
			return // client disconnected; nothing to write
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream request failed"})
		return
	}
	defer resp.Body.Close()

	// Determine from the upstream response whether this is an SSE stream.
	// This is the authoritative signal — more reliable than reading the request body.
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
		c.Writer.Header().Set("X-Accel-Buffering", "no") // prevents Nginx/Railway buffering
		c.Writer.Header().Del("Content-Length")
	}

	c.Writer.WriteHeader(resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		io.Copy(c.Writer, resp.Body)
		return
	}

	now := time.Now()
	var counts TokenCounts

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

	// Publish usage event to Redis Streams (fire-and-forget).
	if counts.MessageID != "" || counts.InputTokens > 0 || counts.OutputTokens > 0 {
		msg := events.UsageEventMsg{
			TenantID:            tenantID,
			Provider:            "anthropic",
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

// HandleModels handles GET /v1/models — simple passthrough.
func (h *ProxyHandler) HandleModels(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")

	plaintextKey, err := h.providerKeySvc.GetActiveKey(c.Request.Context(), tenantID, "anthropic")
	if err != nil {
		if errors.Is(err, services.ErrNoActiveKey) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no_active_provider_key"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve provider key"})
		return
	}

	upstreamURL := anthropicBaseURL + "/v1/models"
	upstreamReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, upstreamURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build upstream request"})
		return
	}

	upstreamReq.Header.Set("x-api-key", string(plaintextKey))
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
	for _, h := range []string{"anthropic-version", "anthropic-beta", "accept"} {
		if v := src.Header.Get(h); v != "" {
			dst.Header.Set(h, v)
		}
	}
}
