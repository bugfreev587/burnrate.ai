package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	tenantID := c.GetUint("tenant_id")
	keyID, _ := c.Get("key_id")
	keyIDStr, _ := keyID.(string)

	fmt.Println("------- HandleResponses -------")

	mode := c.GetString("mode")

	fmt.Println("tenantID:", tenantID, "keyID:", keyIDStr, "mode:", mode)

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
	fmt.Println("------- request parsed ------- model:", req.Model, "stream:", req.Stream, "max_output_tokens:", req.MaxOutputTokens)

	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "tg_bad_request",
			"message": "Missing required field: model.",
		}})
		return
	}

	// Resolve provider from model name, falling back to the API key's provider field.
	provider, resolved := ResolveProviderFromModel(req.Model)
	if !resolved {
		keyProvider := Provider(c.GetString("provider"))
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
	fmt.Println("------- resolved provider:", provider)

	// Compute max_tokens for rate limiting / budget.
	maxTokens := 0
	if req.MaxOutputTokens != nil {
		maxTokens = *req.MaxOutputTokens
	}

	apiUsageBilled := determineBillable(mode, c.Request.Header)

	if h.checkRateLimit(c, tenantID, keyIDStr, provider, req.Model, len(bodyBytes), maxTokens) {
		return
	}

	reservedAmount, ok := h.preCheckBudget(c, tenantID, keyIDStr, provider, req.Model, maxTokens)
	if !ok {
		return
	}

	byokKey, ok := h.resolveAuth(c, tenantID, provider, mode)
	if !ok {
		return
	}
	fmt.Println("----- handle response byokKey: ", byokKey)

	now := time.Now()
	var counts TokenCounts

	switch provider {
	case ProviderOpenAI:
		counts, err = h.handleResponsesOpenAI(c, bodyBytes, req, provider, mode, byokKey)
	case ProviderAnthropic:
		counts, err = h.handleResponsesAnthropic(c, bodyBytes, req, provider, mode, byokKey)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type":    "tg_bad_request",
			"message": fmt.Sprintf("Provider %q does not support /v1/responses.", string(provider)),
		}})
		return
	}

	if err != nil {
		// Error response already written by the sub-handler.
		return
	}

	// If model wasn't extracted from response, use the request model.
	if counts.Model == "" {
		counts.Model = req.Model
	}

	h.reconcilePostResponse(c.Request.Context(), tenantID, keyIDStr, provider, req.Model, maxTokens, counts.OutputTokens, reservedAmount)
	h.publishUsageEvent(c.Request.Context(), tenantID, keyIDStr, provider, counts, apiUsageBilled, now)
}

// handleResponsesOpenAI forwards the request to OpenAI's /v1/responses as-is.
// For Codex passthrough (ChatGPT OAuth), the upstream is the ChatGPT backend
// because the OAuth token is not accepted by api.openai.com.
func (h *ProxyHandler) handleResponsesOpenAI(c *gin.Context, body []byte, req ResponsesRequest, provider Provider, mode string, byokKey []byte) (TokenCounts, error) {
	upstreamURL := upstreamBase(ProviderOpenAI) + "/v1/responses"
	if mode == models.OpenAIModeCodexPassthrough && byokKey == nil {
		upstreamURL = chatGPTCodexResponsesURL
	}

	upstreamReq, err := h.buildUpstreamRequest(c.Request.Context(), http.MethodPost, upstreamURL, body, provider, mode, byokKey, c.Request)
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
	if isSSE {
		counts, err = ParseOpenAIResponsesSSE(c.Request.Context(), resp.Body, c.Writer)
		if err != nil {
			log.Printf("proxy: OpenAI Responses SSE parse error (tenant=%d): %v", c.GetUint("tenant_id"), err)
		}
	} else {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			log.Printf("proxy: read OpenAI Responses body (tenant=%d): %v", c.GetUint("tenant_id"), readErr)
			return TokenCounts{}, readErr
		}
		c.Writer.Write(respBody)
		counts = extractTokensFromOpenAIResponsesJSON(respBody)
	}

	// Codex passthrough via the ChatGPT backend may not return a response ID
	// or usage in the standard OpenAI format. Generate a synthetic ID so the
	// usage event is always published (the guard in publishUsageEvent skips
	// events where MessageID, InputTokens, and OutputTokens are all zero).
	if mode == models.OpenAIModeCodexPassthrough {
		if counts.MessageID == "" {
			counts.MessageID = "codex_" + uuid.New().String()
		}
		log.Printf("proxy: codex passthrough usage — MessageID=%s Model=%s InputTokens=%d OutputTokens=%d (tenant=%d)",
			counts.MessageID, counts.Model, counts.InputTokens, counts.OutputTokens, c.GetUint("tenant_id"))
	}

	return counts, nil
}

// handleResponsesAnthropic translates the Responses request to the Anthropic
// Messages API, sends it upstream, and translates the response back.
func (h *ProxyHandler) handleResponsesAnthropic(c *gin.Context, body []byte, req ResponsesRequest, provider Provider, mode string, byokKey []byte) (TokenCounts, error) {
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

	upstreamReq, err := h.buildUpstreamRequest(c.Request.Context(), http.MethodPost, upstreamURL, anthropicBody, provider, mode, byokKey, c.Request)
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
			log.Printf("proxy: Anthropic→Responses SSE translation error (tenant=%d): %v", c.GetUint("tenant_id"), err)
		}
	} else {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			log.Printf("proxy: read Anthropic response body (tenant=%d): %v", c.GetUint("tenant_id"), readErr)
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
