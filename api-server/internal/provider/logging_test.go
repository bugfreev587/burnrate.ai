package provider

import (
	"testing"
	"time"
)

func TestBuildRouteLog_Success(t *testing.T) {
	finalAttempt := &Attempt{
		Deployment: &Deployment{
			ID:       "deploy-1",
			Provider: "openai",
			Model:    "gpt-4o",
		},
		Response: &ChatCompletionResponse{
			Usage: &Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		},
		StatusCode: 200,
		Duration:   500 * time.Millisecond,
		TTFB:       200 * time.Millisecond,
	}

	cost := &CostResult{TotalCost: 0.001}

	log := BuildRouteLog("req-123", "gpt-4o-group", "gpt-4o", false, finalAttempt, []Attempt{*finalAttempt}, 500*time.Millisecond, cost)

	if log.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", log.RequestID)
	}
	if log.ModelGroup != "gpt-4o-group" {
		t.Errorf("expected model group gpt-4o-group, got %s", log.ModelGroup)
	}
	if !log.Success {
		t.Error("expected success")
	}
	if log.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", log.InputTokens)
	}
	if log.EstimatedCost != 0.001 {
		t.Errorf("expected cost 0.001, got %v", log.EstimatedCost)
	}
	if log.TriggeredFallback {
		t.Error("should not have triggered fallback with 1 attempt")
	}
	if log.Stream {
		t.Error("should not be marked as stream")
	}
}

func TestBuildRouteLog_Fallback(t *testing.T) {
	attempt1 := Attempt{
		Deployment: &Deployment{ID: "deploy-1", Provider: "openai", Model: "gpt-4o"},
		Error:      &ProviderError{Type: ErrorTypeRateLimit, Message: "rate limited"},
		StatusCode: 429,
		TTFB:       100 * time.Millisecond,
	}
	attempt2 := Attempt{
		Deployment: &Deployment{ID: "deploy-2", Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
		Response: &ChatCompletionResponse{
			Usage: &Usage{PromptTokens: 100, CompletionTokens: 50},
		},
		StatusCode: 200,
		TTFB:       300 * time.Millisecond,
	}

	allAttempts := []Attempt{attempt1, attempt2}

	log := BuildRouteLog("req-456", "smart-group", "gpt-4o", true, &attempt2, allAttempts, time.Second, nil)

	if !log.TriggeredFallback {
		t.Error("expected triggered fallback")
	}
	if log.AttemptCount != 2 {
		t.Errorf("expected 2 attempts, got %d", log.AttemptCount)
	}
	if len(log.FallbackChain) != 2 {
		t.Fatalf("expected 2 items in fallback chain, got %d", len(log.FallbackChain))
	}
	if log.FallbackChain[0] != "deploy-1" || log.FallbackChain[1] != "deploy-2" {
		t.Errorf("unexpected fallback chain: %v", log.FallbackChain)
	}
	if log.Provider != "anthropic" {
		t.Errorf("expected final provider anthropic, got %s", log.Provider)
	}
	if !log.Success {
		t.Error("expected success (final attempt succeeded)")
	}
	if !log.Stream {
		t.Error("expected stream=true")
	}
}

func TestBuildRouteLog_Failure(t *testing.T) {
	finalAttempt := &Attempt{
		Deployment: &Deployment{ID: "deploy-1", Provider: "openai", Model: "gpt-4o"},
		Error:      &ProviderError{Type: ErrorTypeServer, Message: "internal error"},
		StatusCode: 500,
		TTFB:       100 * time.Millisecond,
	}

	log := BuildRouteLog("req-789", "group", "gpt-4o", false, finalAttempt, []Attempt{*finalAttempt}, time.Second, nil)

	if log.Success {
		t.Error("expected failure")
	}
	if log.ErrorType != ErrorTypeServer {
		t.Errorf("expected error type server_error, got %s", log.ErrorType)
	}
	if log.ErrorMsg != "internal error" {
		t.Errorf("expected error msg 'internal error', got %s", log.ErrorMsg)
	}
	if log.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", log.StatusCode)
	}
}

func TestSlogRouteLogSink_DoesNotPanic(t *testing.T) {
	sink := &SlogRouteLogSink{}

	log := &RouteLog{
		RequestID:  "test",
		ModelGroup: "test-group",
		Success:    true,
		StatusCode: 200,
	}
	// Should not panic
	sink.WriteRouteLog(log)

	log.Success = false
	log.ErrorType = ErrorTypeServer
	log.ErrorMsg = "test error"
	log.FallbackChain = []string{"d1", "d2"}
	sink.WriteRouteLog(log)
}
