package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// ResponsesRequest is the minimal struct used to extract routing-relevant fields
// from an OpenAI Responses API request body.
type ResponsesRequest struct {
	Model           string          `json:"model"`
	Input           json.RawMessage `json:"input"`
	Instructions    string          `json:"instructions,omitempty"`
	MaxOutputTokens *int            `json:"max_output_tokens,omitempty"`
	Stream          bool            `json:"stream"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
}

// HandleResponses handles POST /v1/responses — the OpenAI Responses API.
// It resolves the provider from the model name, then either forwards to OpenAI
// as-is or translates to the Anthropic Messages API.
func (h *ProxyHandler) HandleResponses(c *gin.Context) {
	start := time.Now()
	tenantID := c.GetUint("tenant_id")
	projectID := c.GetUint("project_id")
	keyID, _ := c.Get("key_id")
	keyIDStr, _ := keyID.(string)

	authMethod := c.GetString("auth_method")
	billingMode := c.GetString("billing_mode")

	// Read the request body.
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "tg_bad_request",
			"message": "Failed to read request body.",
		}})
		return
	}

	// Parse the Responses request to extract model and routing info.
	var req ResponsesRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "tg_bad_request",
			"message": "Invalid JSON in request body.",
		}})
		return
	}
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "tg_bad_request",
			"message": "Missing required field: model.",
		}})
		return
	}

	// Model allowlist enforcement.
	if akI, exists := c.Get("api_key"); exists {
		if ak, ok := akI.(*models.APIKey); ok && ak.ModelAllowlist != "" {
			if !isModelAllowed(ak.ModelAllowlist, req.Model) {
				c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
					"type":    "model_not_allowed",
					"message": fmt.Sprintf("Model %q is not in this API key's allowlist.", req.Model),
				}})
				return
			}
		}
	}

	// Resolve provider from model name, falling back to the API key's provider field.
	keyProvider := Provider(c.GetString("provider"))
	provider, resolved := ResolveProviderFromModel(req.Model)
	if !resolved {
		if keyProvider != "" {
			provider = keyProvider
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
				"type":    "tg_bad_request",
				"message": fmt.Sprintf("Cannot determine provider for model %q.", req.Model),
			}})
			return
		}
	}

	// Enforce provider match: the model's provider must match the API key's provider.
	// e.g. an Anthropic key cannot be used to proxy requests to OpenAI models and vice versa.
	if keyProvider != "" && provider != keyProvider {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
			"type":    "tg_provider_mismatch",
			"message": fmt.Sprintf("API key provider %q does not match model provider %q for model %q.", string(keyProvider), string(provider), req.Model),
		}})
		return
	}
	// Compute max_tokens for rate limiting / budget.
	maxTokens := 0
	if req.MaxOutputTokens != nil {
		maxTokens = *req.MaxOutputTokens
	}

	apiUsageBilled := determineBillable(billingMode)

	if h.checkRateLimit(c, tenantID, keyIDStr, provider, req.Model, len(bodyBytes), maxTokens) {
		if h.gatewayEventSvc != nil {
			h.gatewayEventSvc.Record(c.Request.Context(), &models.GatewayEvent{
				TenantID:   tenantID,
				KeyID:      keyIDStr,
				Provider:   string(provider),
				Model:      req.Model,
				EventType:  "rate_limit_429",
				StatusCode: http.StatusTooManyRequests,
				LatencyMs:  time.Since(start).Milliseconds(),
				CreatedAt:  time.Now(),
			})
		}
		return
	}

	reservedAmount, ok := h.preCheckBudget(c, tenantID, keyIDStr, provider, req.Model, maxTokens)
	if !ok {
		if h.gatewayEventSvc != nil {
			h.gatewayEventSvc.Record(c.Request.Context(), &models.GatewayEvent{
				TenantID:   tenantID,
				KeyID:      keyIDStr,
				Provider:   string(provider),
				Model:      req.Model,
				EventType:  "budget_exceeded_402",
				StatusCode: http.StatusPaymentRequired,
				LatencyMs:  time.Since(start).Milliseconds(),
				CreatedAt:  time.Now(),
			})
		}
		return
	}

	byokKey, ok := h.resolveAuth(c, tenantID, provider, authMethod)
	if !ok {
		return
	}

	// Extract provider key hint (masked) for audit display.
	var providerKeyHint string
	if apiUsageBilled {
		var rawKey string
		if byokKey != nil {
			rawKey = string(byokKey)
		} else if v := c.Request.Header.Get("x-api-key"); v != "" {
			rawKey = v
		} else if v := c.Request.Header.Get("Authorization"); strings.HasPrefix(v, "Bearer ") {
			rawKey = strings.TrimPrefix(v, "Bearer ")
		}
		providerKeyHint = models.MaskKey(rawKey)
	}

	// Measure only TokenGate's own overhead, excluding upstream provider time.
	preUpstreamMs := time.Since(start).Milliseconds()

	now := time.Now()
	var counts TokenCounts

	switch provider {
	case ProviderOpenAI:
		counts, err = h.handleResponsesOpenAI(c, bodyBytes, req, provider, authMethod, billingMode, byokKey)
	case ProviderAnthropic:
		counts, err = h.handleResponsesAnthropic(c, bodyBytes, req, provider, byokKey)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "tg_bad_request",
			"message": fmt.Sprintf("Provider %q does not support /v1/responses.", string(provider)),
		}})
		return
	}

	postUpstreamStart := time.Now()

	if err != nil {
		// Error response already written by the sub-handler.
		return
	}

	// If model wasn't extracted from response, use the request model.
	if counts.Model == "" {
		counts.Model = req.Model
	}

	// Gateway latency = pre-upstream processing + post-upstream processing (excludes upstream call time).
	gatewayMs := preUpstreamMs + time.Since(postUpstreamStart).Milliseconds()
	h.reconcilePostResponse(c.Request.Context(), tenantID, keyIDStr, provider, req.Model, maxTokens, counts.OutputTokens, reservedAmount)
	h.publishUsageEvent(c.Request.Context(), tenantID, projectID, keyIDStr, provider, counts, apiUsageBilled, providerKeyHint, gatewayMs, now)

	slog.Info("proxy_request_completed",
		"tenant_id", tenantID, "key_id", keyIDStr,
		"provider", string(provider), "model", req.Model,
		"gateway_latency_ms", gatewayMs,
		"input_tokens", counts.InputTokens, "output_tokens", counts.OutputTokens,
		"api_usage_billed", apiUsageBilled, "endpoint", "responses",
	)
}

// handleResponsesOpenAI forwards the request to OpenAI's /v1/responses as-is.
// For Codex passthrough (ChatGPT OAuth), the upstream is the ChatGPT backend
// because the OAuth token is not accepted by api.openai.com.
func (h *ProxyHandler) handleResponsesOpenAI(c *gin.Context, body []byte, req ResponsesRequest, provider Provider, authMethod, billingMode string, byokKey []byte) (TokenCounts, error) {
	upstreamURL := upstreamBase(ProviderOpenAI) + "/v1/responses"
	// Route to ChatGPT backend for subscription users with browser OAuth (Codex passthrough).
	if authMethod == models.AuthMethodBrowserOAuth && billingMode == models.BillingModeMonthlySubscription && byokKey == nil {
		upstreamURL = chatGPTCodexResponsesURL
	}

	upstreamReq, err := h.buildUpstreamRequest(c.Request.Context(), http.MethodPost, upstreamURL, body, provider, byokKey, c.Request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"type":    "tg_internal_error",
			"message": "Failed to build upstream request.",
		}})
		return TokenCounts{}, err
	}

	resp, err := h.httpClient.Do(upstreamReq)
	if err != nil {
		if c.Request.Context().Err() != nil {
			return TokenCounts{}, err
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"type":    "tg_upstream_error",
			"message": "Upstream request failed.",
		}})
		return TokenCounts{}, err
	}
	defer resp.Body.Close()

	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	copyAndWriteResponseHeaders(resp, c.Writer, isSSE)
	c.Writer.WriteHeader(resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		io.Copy(c.Writer, resp.Body)
		return TokenCounts{}, fmt.Errorf("upstream returned %d", resp.StatusCode)
	}

	var counts TokenCounts
	var respBodyLen int64
	if isSSE {
		counts, err = ParseOpenAIResponsesSSE(c.Request.Context(), resp.Body, c.Writer)
		if err != nil {
			slog.Error("proxy_openai_responses_sse_error", "tenant_id", c.GetUint("tenant_id"), "error", err)
		}
	} else {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			slog.Error("proxy_read_openai_responses_body_error", "tenant_id", c.GetUint("tenant_id"), "error", readErr)
			return TokenCounts{}, readErr
		}
		respBodyLen = int64(len(respBody))
		c.Writer.Write(respBody)
		counts = extractTokensFromOpenAIResponsesJSON(respBody)
	}

	// Codex passthrough via the ChatGPT backend may not return a response ID
	// or usage in the standard OpenAI format. Generate a synthetic ID so the
	// usage event is always published (the guard in publishUsageEvent skips
	// events where MessageID, InputTokens, and OutputTokens are all zero).
	// When usage data is missing, estimate tokens from the content that flowed
	// through the proxy (~4 bytes per token).
	if authMethod == models.AuthMethodBrowserOAuth && billingMode == models.BillingModeMonthlySubscription {
		if counts.MessageID == "" {
			counts.MessageID = "codex_" + uuid.New().String()
		}
		if counts.InputTokens == 0 {
			counts.InputTokens = int64(len(body)) / 4
			if counts.InputTokens < 1 {
				counts.InputTokens = 1
			}
		}
		if counts.OutputTokens == 0 {
			if counts.OutputTextBytes > 0 {
				counts.OutputTokens = counts.OutputTextBytes / 4
			} else if respBodyLen > 0 {
				counts.OutputTokens = respBodyLen / 4
			}
			if counts.OutputTokens < 1 {
				counts.OutputTokens = 1
			}
		}
		slog.Info("proxy_codex_passthrough_usage",
			"tenant_id", c.GetUint("tenant_id"),
			"message_id", counts.MessageID, "model", counts.Model,
			"input_tokens", counts.InputTokens, "output_tokens", counts.OutputTokens,
		)
	}

	return counts, nil
}

// handleResponsesAnthropic translates the Responses request to the Anthropic
// Messages API, sends it upstream, and translates the response back.
func (h *ProxyHandler) handleResponsesAnthropic(c *gin.Context, body []byte, req ResponsesRequest, provider Provider, byokKey []byte) (TokenCounts, error) {
	// Translate Responses request → Anthropic Messages request.
	anthropicBody, err := translateResponsesToAnthropic(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "tg_bad_request",
			"message": err.Error(),
		}})
		return TokenCounts{}, err
	}

	upstreamURL := upstreamBase(ProviderAnthropic) + "/v1/messages"

	upstreamReq, err := h.buildUpstreamRequest(c.Request.Context(), http.MethodPost, upstreamURL, anthropicBody, provider, byokKey, c.Request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"type":    "tg_internal_error",
			"message": "Failed to build upstream request.",
		}})
		return TokenCounts{}, err
	}

	resp, err := h.httpClient.Do(upstreamReq)
	if err != nil {
		if c.Request.Context().Err() != nil {
			return TokenCounts{}, err
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"type":    "tg_upstream_error",
			"message": "Upstream request failed.",
		}})
		return TokenCounts{}, err
	}
	defer resp.Body.Close()

	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")

	if resp.StatusCode != http.StatusOK {
		// Pass through the upstream error, translating to Responses shape.
		errBody, _ := io.ReadAll(resp.Body)
		translatedErr := translateAnthropicErrorToResponses(resp.StatusCode, errBody)
		c.Writer.Header().Set("Content-Type", "application/json")
		c.Writer.WriteHeader(resp.StatusCode)
		c.Writer.Write(translatedErr)
		return TokenCounts{}, fmt.Errorf("upstream returned %d", resp.StatusCode)
	}

	var counts TokenCounts
	if isSSE {
		// Set SSE headers for the client.
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.Header().Del("Content-Length")
		c.Writer.WriteHeader(http.StatusOK)

		counts, err = TranslateAnthropicSSEToResponses(c.Request.Context(), resp.Body, c.Writer)
		if err != nil {
			slog.Error("proxy_anthropic_responses_sse_error", "tenant_id", c.GetUint("tenant_id"), "error", err)
		}
	} else {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			slog.Error("proxy_read_anthropic_response_body_error", "tenant_id", c.GetUint("tenant_id"), "error", readErr)
			return TokenCounts{}, readErr
		}
		translated, translatedCounts, transErr := translateAnthropicToResponsesJSON(respBody)
		if transErr != nil {
			// Fall back to raw body on translation error.
			c.Writer.Header().Set("Content-Type", "application/json")
			c.Writer.WriteHeader(http.StatusOK)
			c.Writer.Write(respBody)
			return extractTokensFromJSON(respBody), nil
		}
		c.Writer.Header().Set("Content-Type", "application/json")
		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.Write(translated)
		counts = translatedCounts
	}
	return counts, nil
}
