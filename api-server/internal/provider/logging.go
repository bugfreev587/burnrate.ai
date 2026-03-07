package provider

import (
	"log/slog"
	"time"
)

// RouteLog captures the full lifecycle of a routed request for analytics.
type RouteLog struct {
	// Routing info
	RequestID    string `json:"request_id"`
	ModelGroup   string `json:"model_group"`
	RequestModel string `json:"request_model"` // model the client requested

	// Final deployment info
	Provider       string `json:"provider"`
	DeploymentID   string `json:"deployment_id"`
	DeploymentModel string `json:"deployment_model"` // actual model at provider

	// Timing
	TTFB        time.Duration `json:"ttfb_ms"`        // time to first byte
	TotalTime   time.Duration `json:"total_time_ms"`  // total request duration
	StartedAt   time.Time     `json:"started_at"`

	// Token usage
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`

	// Cost
	EstimatedCost float64 `json:"estimated_cost"`

	// Fallback chain
	AttemptCount   int      `json:"attempt_count"`
	FallbackChain  []string `json:"fallback_chain,omitempty"` // deployment IDs tried
	TriggeredFallback bool  `json:"triggered_fallback"`

	// Final status
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code"`
	ErrorType  string `json:"error_type,omitempty"`
	ErrorMsg   string `json:"error_message,omitempty"`

	// Request metadata
	Stream   bool   `json:"stream"`
	TenantID string `json:"tenant_id,omitempty"`
	UserID   string `json:"user_id,omitempty"`
}

// RouteLogSink is the interface for writing route logs.
// Implementations can write to PostgreSQL, stdout, or any other destination.
type RouteLogSink interface {
	WriteRouteLog(log *RouteLog)
}

// SlogRouteLogSink writes route logs via slog (structured logging).
type SlogRouteLogSink struct{}

func (s *SlogRouteLogSink) WriteRouteLog(log *RouteLog) {
	attrs := []any{
		"request_id", log.RequestID,
		"model_group", log.ModelGroup,
		"provider", log.Provider,
		"deployment_id", log.DeploymentID,
		"deployment_model", log.DeploymentModel,
		"ttfb_ms", log.TTFB.Milliseconds(),
		"total_ms", log.TotalTime.Milliseconds(),
		"input_tokens", log.InputTokens,
		"output_tokens", log.OutputTokens,
		"estimated_cost", log.EstimatedCost,
		"attempts", log.AttemptCount,
		"fallback", log.TriggeredFallback,
		"success", log.Success,
		"status", log.StatusCode,
		"stream", log.Stream,
	}

	if log.ErrorType != "" {
		attrs = append(attrs, "error_type", log.ErrorType, "error_msg", log.ErrorMsg)
	}
	if len(log.FallbackChain) > 0 {
		attrs = append(attrs, "fallback_chain", log.FallbackChain)
	}

	if log.Success {
		slog.Info("route_request", attrs...)
	} else {
		slog.Warn("route_request", attrs...)
	}
}

// BuildRouteLog constructs a RouteLog from retry execution results.
func BuildRouteLog(
	requestID string,
	groupName string,
	requestModel string,
	stream bool,
	finalAttempt *Attempt,
	allAttempts []Attempt,
	totalTime time.Duration,
	cost *CostResult,
) *RouteLog {
	log := &RouteLog{
		RequestID:    requestID,
		ModelGroup:   groupName,
		RequestModel: requestModel,
		Stream:       stream,
		TotalTime:    totalTime,
		AttemptCount: len(allAttempts),
		StartedAt:    time.Now().Add(-totalTime),
	}

	// Build fallback chain
	for _, a := range allAttempts {
		if a.Deployment != nil {
			log.FallbackChain = append(log.FallbackChain, a.Deployment.ID)
		}
	}
	log.TriggeredFallback = len(allAttempts) > 1

	// Fill from final attempt
	if finalAttempt != nil {
		if finalAttempt.Deployment != nil {
			log.Provider = finalAttempt.Deployment.Provider
			log.DeploymentID = finalAttempt.Deployment.ID
			log.DeploymentModel = finalAttempt.Deployment.Model
		}
		log.TTFB = finalAttempt.TTFB
		log.StatusCode = finalAttempt.StatusCode
		log.Success = finalAttempt.Error == nil

		if finalAttempt.Error != nil {
			log.ErrorType = finalAttempt.Error.Type
			log.ErrorMsg = finalAttempt.Error.Message
		}

		if finalAttempt.Response != nil && finalAttempt.Response.Usage != nil {
			log.InputTokens = finalAttempt.Response.Usage.PromptTokens
			log.OutputTokens = finalAttempt.Response.Usage.CompletionTokens
			log.TotalTokens = finalAttempt.Response.Usage.TotalTokens
		}
	}

	if cost != nil {
		log.EstimatedCost = cost.TotalCost
	}

	return log
}
