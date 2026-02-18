package proxy

import (
	"errors"
	"io"
	"log"
	"net/http"
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
		httpClient: &http.Client{
			Timeout: 300 * time.Second,
		},
	}
}

// HandleMessages handles POST /v1/messages — the main Anthropic proxy endpoint.
func (h *ProxyHandler) HandleMessages(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")

	// Fetch the active Anthropic key for this tenant
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

	// Read the request body (we need it to detect streaming and forward it)
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	streaming := isStreamingRequest(bodyBytes)

	// Build upstream request
	upstreamURL := anthropicBaseURL + "/v1/messages"
	upstreamReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstreamURL, newBodyReader(bodyBytes))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build upstream request"})
		return
	}

	// Copy relevant headers from the client request
	copyHeaders(c.Request, upstreamReq)

	// Override Authorization with the tenant's Anthropic key
	upstreamReq.Header.Set("Authorization", "Bearer "+string(plaintextKey))
	upstreamReq.Header.Set("x-api-key", string(plaintextKey))
	upstreamReq.Header.Set("Content-Type", "application/json")

	// Forward anthropic-version; default if not set
	if upstreamReq.Header.Get("anthropic-version") == "" {
		upstreamReq.Header.Set("anthropic-version", defaultAnthropicVersion)
	}

	// Execute upstream request
	resp, err := h.httpClient.Do(upstreamReq)
	if err != nil {
		if c.Request.Context().Err() != nil {
			return // client disconnected
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream request failed"})
		return
	}
	defer resp.Body.Close()

	// Copy response headers to client
	for key, vals := range resp.Header {
		for _, v := range vals {
			c.Header(key, v)
		}
	}
	c.Status(resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		// Forward error response verbatim
		io.Copy(c.Writer, resp.Body)
		return
	}

	now := time.Now()
	var counts TokenCounts

	if streaming {
		// Pipe SSE stream to client while parsing token counts
		counts, err = ParseSSE(c.Request.Context(), resp.Body, c.Writer)
		if err != nil {
			log.Printf("proxy: SSE parse error (tenant=%d): %v", tenantID, err)
		}
	} else {
		// Read full JSON response, forward it, then extract usage
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			log.Printf("proxy: read response body (tenant=%d): %v", tenantID, readErr)
			return
		}
		c.Writer.Write(respBody)
		counts = extractTokensFromJSON(respBody)
	}

	// Publish usage event to Redis Streams (fire-and-forget)
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
		for _, v := range vals {
			c.Header(key, v)
		}
	}
	c.Status(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

// copyHeaders copies safe request headers from the client to the upstream request.
func copyHeaders(src *http.Request, dst *http.Request) {
	safe := []string{
		"anthropic-version",
		"anthropic-beta",
		"content-type",
		"accept",
	}
	for _, h := range safe {
		if v := src.Header.Get(h); v != "" {
			dst.Header.Set(h, v)
		}
	}
}
