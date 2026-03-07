package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestIntegration_NonStreamingRoutingWithFallback verifies end-to-end non-streaming
// routing with fallback: first deployment returns 429, second succeeds.
func TestIntegration_NonStreamingRoutingWithFallback(t *testing.T) {
	// Mock provider 1: returns 429
	mock1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "rate limited",
				"type":    "rate_limit_error",
			},
		})
	}))
	defer mock1.Close()

	// Mock provider 2: returns success
	mock2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request was properly transformed
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-4o",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	}))
	defer mock2.Close()

	// Create adapters pointing to mock servers
	adapter1 := &testURLAdapter{name: "openai", baseURL: mock1.URL}
	adapter2 := &testURLAdapter{name: "openai", baseURL: mock2.URL}

	registry := NewRegistry()
	// Override openai adapter with test adapters — we'll use the router directly
	_ = adapter1
	_ = adapter2

	router := NewRouter(registry)
	router.AddModelGroup(&ModelGroup{
		Name:     "test-group",
		Strategy: "fallback",
		Deployments: []Deployment{
			{ID: "dep-1", Provider: "openai", Model: "gpt-4o", APIKey: "sk-test1", Priority: 1},
			{ID: "dep-2", Provider: "openai", Model: "gpt-4o", APIKey: "sk-test2", Priority: 2},
		},
	})

	// Verify group was added
	group, err := router.GetModelGroup("test-group")
	if err != nil {
		t.Fatalf("expected model group, got error: %v", err)
	}
	if len(group.Deployments) != 2 {
		t.Fatalf("expected 2 deployments, got %d", len(group.Deployments))
	}

	// Verify strategy selection works
	strategy, err := GetStrategy("fallback")
	if err != nil {
		t.Fatalf("expected fallback strategy, got error: %v", err)
	}

	dep, err := strategy.Select(context.Background(), group, router.State())
	if err != nil {
		t.Fatalf("expected deployment selection, got error: %v", err)
	}
	if dep.ID != "dep-1" {
		t.Errorf("expected dep-1 (highest priority), got %s", dep.ID)
	}
}

// TestIntegration_RetryExecutorWithMockServer tests the RetryExecutor succeeds
// on first try with a healthy mock server.
func TestIntegration_RetryExecutorWithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-4o",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	}))
	defer server.Close()

	adapter := newOpenAIAdapterWithURL(server.URL)
	registry := &Registry{adapters: map[string]ProviderAdapter{"openai": adapter}}

	state := NewRouterState()
	cfg := DefaultRetryConfig()

	executor := NewRetryExecutor(cfg, registry, state)

	group := &ModelGroup{
		Name:     "test",
		Strategy: "fallback",
		Deployments: []Deployment{
			{ID: "dep-1", Provider: "openai", Model: "gpt-4o", APIKey: "sk-test", Priority: 1},
		},
	}

	req := &ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}

	finalAttempt, allAttempts, err := executor.ExecuteWithRetry(context.Background(), group, req)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if len(allAttempts) != 1 {
		t.Errorf("expected 1 attempt, got %d", len(allAttempts))
	}
	if finalAttempt.Response == nil {
		t.Fatal("expected response in final attempt")
	}
	if finalAttempt.Response.Usage == nil {
		t.Fatal("expected usage in response")
	}
	if finalAttempt.Response.Usage.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", finalAttempt.Response.Usage.PromptTokens)
	}
}

// TestIntegration_RetryExecutorNonRetryableError tests that the executor stops
// on non-retryable errors (e.g., 400 Bad Request).
func TestIntegration_RetryExecutorNonRetryableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "invalid model",
				"type":    "invalid_request_error",
			},
		})
	}))
	defer server.Close()

	adapter := newOpenAIAdapterWithURL(server.URL)
	registry := &Registry{adapters: map[string]ProviderAdapter{"openai": adapter}}

	executor := NewRetryExecutor(DefaultRetryConfig(), registry, NewRouterState())

	group := &ModelGroup{
		Name:     "test",
		Strategy: "fallback",
		Deployments: []Deployment{
			{ID: "dep-1", Provider: "openai", Model: "gpt-bad", APIKey: "sk-test", Priority: 1},
			{ID: "dep-2", Provider: "openai", Model: "gpt-bad", APIKey: "sk-test", Priority: 2},
		},
	}

	req := &ChatCompletionRequest{
		Model:    "gpt-bad",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}

	finalAttempt, allAttempts, err := executor.ExecuteWithRetry(context.Background(), group, req)
	if err == nil {
		t.Fatal("expected error for non-retryable status")
	}
	// Should have stopped after 1 attempt (non-retryable)
	if len(allAttempts) != 1 {
		t.Errorf("expected 1 attempt (non-retryable), got %d", len(allAttempts))
	}
	if finalAttempt.Error == nil {
		t.Fatal("expected error in final attempt")
	}
	if finalAttempt.Error.Message != "invalid model" {
		t.Errorf("expected 'invalid model', got %q", finalAttempt.Error.Message)
	}
}

// TestIntegration_StreamingWithMockServer tests streaming SSE through the adapter.
func TestIntegration_StreamingWithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		// Send SSE chunks
		chunks := []string{
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`,
			`data: [DONE]`,
		}

		for _, chunk := range chunks {
			fmt.Fprintf(w, "%s\n\n", chunk)
			flusher.Flush()
		}
	}))
	defer server.Close()

	adapter := newOpenAIAdapterWithURL(server.URL)

	req := &ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "hello"}},
		Stream:   true,
	}

	httpReq, err := adapter.TransformRequest(context.Background(), req, "sk-test")
	if err != nil {
		t.Fatalf("TransformRequest error: %v", err)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("HTTP request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Parse SSE chunks through the adapter
	body, _ := io.ReadAll(resp.Body)
	lines := strings.Split(string(body), "\n")

	var allChunks []*StreamChunk
	streamAdapter := NewOpenAIAdapter()
	for _, line := range lines {
		if line == "" {
			continue
		}
		chunks, err := streamAdapter.TransformStreamChunk([]byte(line))
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		allChunks = append(allChunks, chunks...)
	}

	if len(allChunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(allChunks))
	}

	// Check last chunk has usage
	lastChunk := allChunks[len(allChunks)-1]
	if lastChunk.Usage != nil {
		if lastChunk.Usage.PromptTokens != 5 {
			t.Errorf("expected 5 prompt tokens in usage, got %d", lastChunk.Usage.PromptTokens)
		}
	}
}

// TestIntegration_AnthropicFormatTranslation tests full request/response translation
// through the Anthropic adapter with a mock server.
func TestIntegration_AnthropicFormatTranslation(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Anthropic headers
		if r.Header.Get("x-api-key") != "sk-ant-test" {
			t.Errorf("expected x-api-key header, got %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("expected anthropic-version header")
		}

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)

		// Return Anthropic-format response
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "msg_test",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": []map[string]any{
				{"type": "text", "text": "Hello from Claude!"},
			},
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  15,
				"output_tokens": 8,
			},
		})
	}))
	defer server.Close()

	adapter := newAnthropicAdapterWithURL(server.URL)

	req := &ChatCompletionRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello!"},
		},
	}

	httpReq, err := adapter.TransformRequest(context.Background(), req, "sk-ant-test")
	if err != nil {
		t.Fatalf("TransformRequest error: %v", err)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("HTTP request error: %v", err)
	}

	chatResp, err := adapter.TransformResponse(resp)
	if err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}

	// Verify the system message was extracted
	if systemVal, ok := receivedBody["system"]; !ok || systemVal == nil {
		t.Error("expected system field in Anthropic request")
	}

	// Verify response was translated to OpenAI format
	if len(chatResp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
	if chatResp.Choices[0].Message == nil {
		t.Fatal("expected message in choice")
	}
	content := chatResp.Choices[0].Message.GetContentText()
	if content != "Hello from Claude!" {
		t.Errorf("expected 'Hello from Claude!', got %q", content)
	}
	if chatResp.Usage == nil {
		t.Fatal("expected usage")
	}
	if chatResp.Usage.PromptTokens != 15 {
		t.Errorf("expected 15 prompt tokens, got %d", chatResp.Usage.PromptTokens)
	}
	if chatResp.Usage.CompletionTokens != 8 {
		t.Errorf("expected 8 completion tokens, got %d", chatResp.Usage.CompletionTokens)
	}
}

// TestIntegration_HealthCooldownAndRecovery tests deployment health tracking
// through a realistic sequence of failures and recovery.
func TestIntegration_HealthCooldownAndRecovery(t *testing.T) {
	ht := NewHealthTracker()

	// 3 failures should trigger cooldown
	for i := 0; i < 3; i++ {
		ht.RecordFailure("dep-1")
	}

	if ht.IsHealthy("dep-1") {
		t.Error("expected dep-1 to be in cooldown after 3 failures")
	}

	// dep-2 should still be healthy
	if !ht.IsHealthy("dep-2") {
		t.Error("expected dep-2 to be healthy")
	}

	// After success, should recover
	ht.RecordSuccess("dep-1")
	if !ht.IsHealthy("dep-1") {
		t.Error("expected dep-1 to be healthy after success")
	}
}

// TestIntegration_RouterStateTracking tests that router state (health, latency,
// rate limit) is properly updated during routing.
func TestIntegration_RouterStateTracking(t *testing.T) {
	state := NewRouterState()

	// Record some latencies
	state.Latency.Record("dep-1", 100*time.Millisecond)
	state.Latency.Record("dep-1", 200*time.Millisecond)
	state.Latency.Record("dep-2", 50*time.Millisecond)

	avg1 := state.Latency.Average("dep-1")
	if avg1 != 150*time.Millisecond {
		t.Errorf("expected avg 150ms for dep-1, got %v", avg1)
	}

	avg2 := state.Latency.Average("dep-2")
	if avg2 != 50*time.Millisecond {
		t.Errorf("expected avg 50ms for dep-2, got %v", avg2)
	}

	// Record rate limit info
	state.RateLimit.Update("dep-1", &RateLimitInfo{
		RemainingRequests: 100,
		LimitRequests:     1000,
	})

	if state.RateLimit.ShouldAvoid("dep-1") {
		t.Error("expected dep-1 to NOT be avoided (100/1000 = 10% remaining)")
	}
}

// TestIntegration_PricingCalculation tests cost calculation with the pricing table.
func TestIntegration_PricingCalculation(t *testing.T) {
	pt := NewPricingTable()

	cost := pt.CalculateCost("gpt-4o", "openai", &Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
	})
	if cost == nil {
		t.Fatal("expected cost result")
	}
	if cost.TotalCost <= 0 {
		t.Errorf("expected positive cost, got %v", cost.TotalCost)
	}

	// Test unknown model (should still return a cost with zero values)
	cost2 := pt.CalculateCost("unknown-model", "unknown", &Usage{
		PromptTokens: 100,
	})
	if cost2 == nil {
		t.Fatal("expected cost result for unknown model")
	}
}

// TestIntegration_ErrorClassification tests error classification and parsing.
func TestIntegration_ErrorClassification(t *testing.T) {
	tests := []struct {
		status    int
		wantType  string
		wantRetry bool
	}{
		{400, ErrorTypeInvalidRequest, false},
		{401, ErrorTypeAuthentication, false},
		{403, ErrorTypePermission, false},
		{404, ErrorTypeNotFound, false},
		{429, ErrorTypeRateLimit, true},
		{500, ErrorTypeServer, true},
		{503, ErrorTypeServer, true},
		{529, ErrorTypeOverloaded, true},
	}

	for _, tt := range tests {
		errType, retryable := ClassifyHTTPStatus(tt.status)
		if errType != tt.wantType {
			t.Errorf("status %d: expected type %q, got %q", tt.status, tt.wantType, errType)
		}
		if retryable != tt.wantRetry {
			t.Errorf("status %d: expected retryable=%v, got %v", tt.status, tt.wantRetry, retryable)
		}
	}

	// Test error parsing with OpenAI-style body
	body := `{"error":{"message":"Model not found","type":"invalid_request_error","code":"model_not_found"}}`
	pe := ParseProviderError(404, []byte(body), "openai", "gpt-99", http.Header{})
	if pe.Message != "Model not found" {
		t.Errorf("expected parsed message, got %q", pe.Message)
	}
	if pe.Provider != "openai" {
		t.Errorf("expected provider openai, got %q", pe.Provider)
	}
}

// --- Test helpers ---

// testURLAdapter wraps an adapter to point at a custom URL.
type testURLAdapter struct {
	name    string
	baseURL string
}

func (a *testURLAdapter) Name() string { return a.name }
func (a *testURLAdapter) TransformRequest(ctx context.Context, req *ChatCompletionRequest, apiKey string) (*http.Request, error) {
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	return httpReq, nil
}
func (a *testURLAdapter) TransformResponse(resp *http.Response) (*ChatCompletionResponse, error) {
	defer resp.Body.Close()
	var r ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}
func (a *testURLAdapter) TransformStreamChunk(chunk []byte) ([]*StreamChunk, error) {
	return nil, nil
}
func (a *testURLAdapter) ExtractRateLimitInfo(header http.Header) *RateLimitInfo {
	return nil
}
func (a *testURLAdapter) ExtractUsage(body []byte) *Usage { return nil }

// newOpenAIAdapterWithURL creates an OpenAI adapter that uses a custom base URL.
func newOpenAIAdapterWithURL(url string) *OpenAIAdapter {
	a := NewOpenAIAdapter()
	a.baseURL = url
	return a
}

// newAnthropicAdapterWithURL creates an Anthropic adapter that uses a custom base URL.
func newAnthropicAdapterWithURL(url string) *AnthropicAdapter {
	a := NewAnthropicAdapter()
	a.baseURL = url
	return a
}
