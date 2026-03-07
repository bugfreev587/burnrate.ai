package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestOpenAIAdapter_Name(t *testing.T) {
	a := NewOpenAIAdapter()
	if a.Name() != "openai" {
		t.Errorf("expected 'openai', got %q", a.Name())
	}
}

func TestOpenAIAdapter_TransformRequest(t *testing.T) {
	a := NewOpenAIAdapter()
	req := &ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	httpReq, err := a.TransformRequest(context.Background(), req, "sk-test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if httpReq.URL.String() != openAIBaseURL {
		t.Errorf("expected URL %s, got %s", openAIBaseURL, httpReq.URL.String())
	}
	if httpReq.Header.Get("Authorization") != "Bearer sk-test-key" {
		t.Errorf("expected bearer auth header")
	}
	if httpReq.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected json content type")
	}

	// Verify body
	body, _ := io.ReadAll(httpReq.Body)
	var parsed ChatCompletionRequest
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if parsed.Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %s", parsed.Model)
	}
}

func TestOpenAIAdapter_TransformRequest_WithOrg(t *testing.T) {
	a := &OpenAIAdapter{Organization: "org-test"}
	req := &ChatCompletionRequest{Model: "gpt-4o", Messages: []Message{{Role: "user", Content: "Hi"}}}

	httpReq, err := a.TransformRequest(context.Background(), req, "sk-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if httpReq.Header.Get("OpenAI-Organization") != "org-test" {
		t.Errorf("expected organization header")
	}
}

func TestOpenAIAdapter_TransformResponse(t *testing.T) {
	a := NewOpenAIAdapter()

	respBody := `{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"created": 1700000000,
		"model": "gpt-4o",
		"choices": [{
			"index": 0,
			"message": {"role": "assistant", "content": "Hello!"},
			"finish_reason": "stop"
		}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
	}`

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(respBody))),
	}

	result, err := a.TransformResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != "chatcmpl-123" {
		t.Errorf("expected id chatcmpl-123, got %s", result.ID)
	}
	if len(result.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(result.Choices))
	}
	if result.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", result.Usage.TotalTokens)
	}
}

func TestOpenAIAdapter_TransformResponse_Error(t *testing.T) {
	a := NewOpenAIAdapter()
	resp := &http.Response{
		StatusCode: 429,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"rate limited"}`))),
	}

	_, err := a.TransformResponse(resp)
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestOpenAIAdapter_TransformStreamChunk(t *testing.T) {
	a := NewOpenAIAdapter()

	tests := []struct {
		name    string
		input   string
		wantN   int
		wantEOF bool
		wantErr bool
	}{
		{
			name:  "normal chunk",
			input: `data: {"id":"cmpl-1","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hi"}}]}`,
			wantN: 1,
		},
		{
			name:    "done signal",
			input:   "data: [DONE]",
			wantEOF: true,
		},
		{
			name:  "empty line",
			input: "",
			wantN: 0,
		},
		{
			name:  "comment",
			input: ": keep-alive",
			wantN: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks, err := a.TransformStreamChunk([]byte(tt.input))
			if tt.wantEOF {
				if err != io.EOF {
					t.Errorf("expected EOF, got %v", err)
				}
				return
			}
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if len(chunks) != tt.wantN {
				t.Errorf("expected %d chunks, got %d", tt.wantN, len(chunks))
			}
		})
	}
}

func TestOpenAIAdapter_ExtractRateLimitInfo(t *testing.T) {
	a := NewOpenAIAdapter()

	header := http.Header{}
	header.Set("x-ratelimit-remaining-requests", "99")
	header.Set("x-ratelimit-remaining-tokens", "9000")
	header.Set("x-ratelimit-limit-requests", "100")
	header.Set("x-ratelimit-limit-tokens", "10000")

	info := a.ExtractRateLimitInfo(header)
	if info == nil {
		t.Fatal("expected rate limit info")
	}
	if info.RemainingRequests != 99 {
		t.Errorf("expected 99 remaining requests, got %d", info.RemainingRequests)
	}
	if info.LimitTokens != 10000 {
		t.Errorf("expected 10000 limit tokens, got %d", info.LimitTokens)
	}
}

func TestOpenAIAdapter_ExtractRateLimitInfo_Empty(t *testing.T) {
	a := NewOpenAIAdapter()
	info := a.ExtractRateLimitInfo(http.Header{})
	if info != nil {
		t.Error("expected nil for empty headers")
	}
}

func TestOpenAIAdapter_ExtractUsage(t *testing.T) {
	a := NewOpenAIAdapter()

	body := []byte(`{"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`)
	usage := a.ExtractUsage(body)
	if usage == nil {
		t.Fatal("expected usage")
	}
	if usage.TotalTokens != 30 {
		t.Errorf("expected 30 total tokens, got %d", usage.TotalTokens)
	}
}

func TestOpenAIAdapter_ExtractUsage_NoUsage(t *testing.T) {
	a := NewOpenAIAdapter()
	usage := a.ExtractUsage([]byte(`{"id":"test"}`))
	if usage != nil {
		t.Error("expected nil for no usage field")
	}
}
