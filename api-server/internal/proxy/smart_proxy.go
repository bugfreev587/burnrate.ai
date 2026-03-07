package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/xiaoboyu/tokengate/api-server/internal/events"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/provider"
)

const (
	// streamChunkTimeout is the max time to wait for the next SSE chunk
	// before considering the upstream connection stale.
	streamChunkTimeout = 30 * time.Second
)

// SmartProxyHandler handles multi-provider routing with format translation,
// streaming support, fallback, and request logging.
// It integrates with the existing TokenGate auth, usage, and rate-limit pipeline.
type SmartProxyHandler struct {
	router     *provider.Router
	registry   *provider.Registry
	pricing    *provider.PricingTable
	logSink    provider.RouteLogSink
	retryCfg   provider.RetryConfig
	eventQueue *events.EventQueue
}

// NewSmartProxyHandler creates a SmartProxyHandler with the given registry and router.
func NewSmartProxyHandler(registry *provider.Registry, router *provider.Router, eventQueue *events.EventQueue) *SmartProxyHandler {
	return &SmartProxyHandler{
		registry:   registry,
		router:     router,
		pricing:    provider.NewPricingTable(),
		logSink:    &provider.SlogRouteLogSink{},
		retryCfg:   provider.DefaultRetryConfig(),
		eventQueue: eventQueue,
	}
}

// HandleSmartProxy handles an OpenAI-compatible chat completion request,
// routing it through the configured model groups with automatic format translation,
// retry with backoff, and fallback on retryable errors.
//
// If the requested model doesn't match any model group, returns 400.
// Tenant context (tenant_id, key_id, project_id, billing_mode) is read from
// the Gin context, set by upstream TenantAuthMiddleware.
func (h *SmartProxyHandler) HandleSmartProxy(c *gin.Context) {
	requestID := uuid.New().String()
	start := time.Now()

	// Read tenant context from middleware
	tenantID := c.GetUint("tenant_id")
	projectID := c.GetUint("project_id")
	keyID, _ := c.Get("key_id")
	keyIDStr, _ := keyID.(string)
	billingMode := c.GetString("billing_mode")
	apiUsageBilled := models.IsBillableMode(billingMode)

	// Parse the OpenAI-compatible request
	var req provider.ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "invalid request body: " + err.Error(),
			"type":    "invalid_request_error",
		}})
		return
	}

	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "model is required",
			"type":    "invalid_request_error",
		}})
		return
	}

	groupName := req.Model

	group, err := h.router.GetModelGroup(groupName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": fmt.Sprintf("unknown model group: %s", groupName),
			"type":    "invalid_request_error",
		}})
		return
	}

	if req.Stream {
		h.handleStream(c, requestID, start, group, &req, tenantID, projectID, keyIDStr, apiUsageBilled)
		return
	}

	// Non-streaming: use RetryExecutor for full fallback support
	executor := provider.NewRetryExecutor(h.retryCfg, h.registry, h.router.State())
	finalAttempt, allAttempts, err := executor.ExecuteWithRetry(c.Request.Context(), group, &req)

	totalTime := time.Since(start)

	// Build and write route log
	var cost *provider.CostResult
	if finalAttempt != nil && finalAttempt.Response != nil {
		cost = h.pricing.CalculateCost(
			finalAttempt.Deployment.Model,
			finalAttempt.Deployment.Provider,
			finalAttempt.Response.Usage,
		)
	}
	routeLog := provider.BuildRouteLog(requestID, groupName, req.Model, false, finalAttempt, allAttempts, totalTime, cost)
	go h.logSink.WriteRouteLog(routeLog)

	// Publish usage event for successful requests
	if finalAttempt != nil && finalAttempt.Response != nil && finalAttempt.Response.Usage != nil {
		go h.publishUsageEvent(tenantID, projectID, keyIDStr, finalAttempt, totalTime, apiUsageBilled)
	}

	if err != nil {
		if finalAttempt != nil && finalAttempt.Error != nil {
			c.Data(finalAttempt.StatusCode, "application/json", finalAttempt.Error.ToJSON())
			return
		}
		slog.Error("smart proxy route failed", "error", err, "model", req.Model)
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
			"message": fmt.Sprintf("routing failed: %v", err),
			"type":    "server_error",
		}})
		return
	}

	c.JSON(http.StatusOK, finalAttempt.Response)
}

// handleStream handles streaming requests with connection-phase fallback (Plan A):
// fallback only happens before the first byte is sent to the client.
// Once streaming starts, mid-stream provider failures are not retried.
func (h *SmartProxyHandler) handleStream(c *gin.Context, requestID string, start time.Time, group *provider.ModelGroup, req *provider.ChatCompletionRequest, tenantID uint, projectID uint, keyID string, apiUsageBilled bool) {
	strategy, err := provider.GetStrategy(group.Strategy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "invalid routing strategy",
			"type":    "server_error",
		}})
		return
	}

	// Connection-phase fallback: try multiple deployments until one succeeds
	maxAttempts := h.retryCfg.MaxRetries + 1
	if maxAttempts > len(group.Deployments) {
		maxAttempts = len(group.Deployments)
	}

	var allAttempts []provider.Attempt
	tried := make(map[string]bool)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if c.Request.Context().Err() != nil {
			break // client disconnected
		}

		deployment, err := strategy.Select(c.Request.Context(), group, h.router.State())
		if err != nil {
			break
		}
		if tried[deployment.ID] {
			continue
		}
		tried[deployment.ID] = true

		adapter, err := h.registry.Get(deployment.Provider)
		if err != nil {
			continue
		}

		reqCopy := *req
		reqCopy.Model = deployment.Model

		httpReq, err := adapter.TransformRequest(c.Request.Context(), &reqCopy, deployment.APIKey)
		if err != nil {
			allAttempts = append(allAttempts, provider.Attempt{
				Deployment: deployment,
				Error: &provider.ProviderError{
					Message:  fmt.Sprintf("transform request: %v", err),
					Type:     provider.ErrorTypeServer,
					Provider: deployment.Provider,
					Model:    deployment.Model,
				},
			})
			continue
		}

		// Use streaming-appropriate client: no overall timeout, but connect timeout
		transport := &http.Transport{
			ResponseHeaderTimeout: h.retryCfg.StreamFirstChunkTO,
		}
		client := &http.Client{Transport: transport}

		connStart := time.Now()
		resp, err := client.Do(httpReq)
		ttfb := time.Since(connStart)

		if err != nil {
			h.router.State().Health.RecordFailure(deployment.ID)
			allAttempts = append(allAttempts, provider.Attempt{
				Deployment: deployment,
				Error: &provider.ProviderError{
					Message:   fmt.Sprintf("connect failed: %v", err),
					Type:      provider.ErrorTypeTimeout,
					Provider:  deployment.Provider,
					Model:     deployment.Model,
					Retryable: true,
				},
				TTFB: ttfb,
			})
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			h.router.State().Health.RecordFailure(deployment.ID)

			pe := provider.ParseProviderError(resp.StatusCode, body, deployment.Provider, deployment.Model, resp.Header)
			allAttempts = append(allAttempts, provider.Attempt{
				Deployment: deployment,
				Error:      pe,
				RawBody:    body,
				StatusCode: resp.StatusCode,
				TTFB:       ttfb,
			})

			if !pe.Retryable {
				// Non-retryable → return error immediately
				totalTime := time.Since(start)
				lastAttempt := allAttempts[len(allAttempts)-1]
				routeLog := provider.BuildRouteLog(requestID, group.Name, req.Model, true, &lastAttempt, allAttempts, totalTime, nil)
				go h.logSink.WriteRouteLog(routeLog)

				c.Data(resp.StatusCode, "application/json", pe.ToJSON())
				return
			}
			continue
		}

		// Connection established — begin streaming to client
		h.router.State().Health.RecordSuccess(deployment.ID)
		h.router.State().Latency.Record(deployment.ID, ttfb)

		// Extract rate limit info
		rlInfo := adapter.ExtractRateLimitInfo(resp.Header)
		h.router.State().RateLimit.Update(deployment.ID, rlInfo)

		streamUsage := h.streamResponse(c, resp, deployment, adapter)

		totalTime := time.Since(start)

		// Build final attempt for logging
		finalAttempt := provider.Attempt{
			Deployment: deployment,
			StatusCode: http.StatusOK,
			TTFB:       ttfb,
			Duration:   totalTime,
		}
		if streamUsage != nil {
			finalAttempt.Response = &provider.ChatCompletionResponse{Usage: streamUsage}
		}
		allAttempts = append(allAttempts, finalAttempt)

		var cost *provider.CostResult
		if streamUsage != nil {
			cost = h.pricing.CalculateCost(deployment.Model, deployment.Provider, streamUsage)
		}

		routeLog := provider.BuildRouteLog(requestID, group.Name, req.Model, true, &finalAttempt, allAttempts, totalTime, cost)
		go h.logSink.WriteRouteLog(routeLog)

		// Publish usage event
		if streamUsage != nil {
			go h.publishUsageEvent(tenantID, projectID, keyID, &finalAttempt, totalTime, apiUsageBilled)
		}
		return
	}

	// All attempts failed
	totalTime := time.Since(start)
	var lastAttempt *provider.Attempt
	if len(allAttempts) > 0 {
		lastAttempt = &allAttempts[len(allAttempts)-1]
	}
	routeLog := provider.BuildRouteLog(requestID, group.Name, req.Model, true, lastAttempt, allAttempts, totalTime, nil)
	go h.logSink.WriteRouteLog(routeLog)

	if lastAttempt != nil && lastAttempt.Error != nil {
		c.Data(lastAttempt.StatusCode, "application/json", lastAttempt.Error.ToJSON())
		return
	}

	c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
		"message": "all deployments exhausted",
		"type":    "server_error",
	}})
}

// publishUsageEvent publishes a usage event to the existing Redis Streams pipeline.
func (h *SmartProxyHandler) publishUsageEvent(tenantID, projectID uint, keyID string, attempt *provider.Attempt, totalTime time.Duration, billed bool) {
	if h.eventQueue == nil || tenantID == 0 || attempt == nil || attempt.Response == nil || attempt.Response.Usage == nil {
		return
	}
	usage := attempt.Response.Usage
	msg := events.UsageEventMsg{
		TenantID:       tenantID,
		ProjectID:      projectID,
		KeyID:          keyID,
		Provider:       attempt.Deployment.Provider,
		Model:          attempt.Deployment.Model,
		InputTokens:    int64(usage.PromptTokens),
		OutputTokens:   int64(usage.CompletionTokens),
		MessageID:      "route_" + uuid.New().String(),
		LatencyMs:      totalTime.Milliseconds(),
		Timestamp:      time.Now(),
		APIUsageBilled: billed,
	}
	if err := h.eventQueue.Publish(context.Background(), msg); err != nil {
		slog.Error("smart_proxy_publish_usage_failed", "tenant_id", tenantID, "error", err)
	}
}

// streamResponse pipes the SSE stream from upstream to the client,
// translating chunks through the adapter. Returns accumulated usage if available.
func (h *SmartProxyHandler) streamResponse(
	c *gin.Context,
	resp *http.Response,
	deployment *provider.Deployment,
	adapter provider.ProviderAdapter,
) *provider.Usage {
	defer resp.Body.Close()

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	// Create a fresh stream translator for this request
	streamAdapter := h.getStreamAdapter(deployment.Provider)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	// Track accumulated usage from stream chunks
	var accumulatedUsage *provider.Usage

	// Use context for client disconnect detection
	ctx := c.Request.Context()

	for {
		// Check client disconnect
		select {
		case <-ctx.Done():
			slog.Info("client disconnected during stream",
				"deployment", deployment.ID,
				"provider", deployment.Provider,
			)
			return accumulatedUsage
		default:
		}

		// Read next line with timeout
		// bufio.Scanner doesn't support deadline directly,
		// so we rely on the context cancellation and upstream read deadlines.
		if !scanner.Scan() {
			break
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		chunks, err := streamAdapter.TransformStreamChunk(line)
		if err == io.EOF {
			fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
			c.Writer.Flush()
			return accumulatedUsage
		}
		if err != nil {
			slog.Error("stream chunk translation error",
				"error", err,
				"provider", deployment.Provider,
			)
			continue
		}

		for _, chunk := range chunks {
			// Accumulate usage from chunks (OpenAI sends usage in last chunk
			// when stream_options.include_usage is true)
			if chunk.Usage != nil {
				accumulatedUsage = chunk.Usage
			}

			data, err := json.Marshal(chunk)
			if err != nil {
				continue
			}
			if _, writeErr := fmt.Fprintf(c.Writer, "data: %s\n\n", data); writeErr != nil {
				slog.Info("write to client failed (client likely disconnected)",
					"error", writeErr,
				)
				return accumulatedUsage
			}
			c.Writer.Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Error("stream scanner error",
			"error", err,
			"provider", deployment.Provider,
		)
	}

	return accumulatedUsage
}

// getStreamAdapter returns a fresh adapter with clean stream state for the given provider.
func (h *SmartProxyHandler) getStreamAdapter(providerName string) provider.ProviderAdapter {
	switch providerName {
	case "anthropic":
		return provider.NewAnthropicAdapter()
	case "openai":
		return provider.NewOpenAIAdapter()
	case "deepseek":
		return provider.NewDeepSeekAdapter()
	case "mistral":
		return provider.NewMistralAdapter()
	default:
		adapter, _ := h.registry.Get(providerName)
		return adapter
	}
}
