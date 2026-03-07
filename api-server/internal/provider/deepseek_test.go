package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDeepSeekAdapter_Name(t *testing.T) {
	a := NewDeepSeekAdapter()
	if a.Name() != "deepseek" {
		t.Errorf("expected 'deepseek', got %q", a.Name())
	}
}

func TestDeepSeekAdapter_TransformRequest(t *testing.T) {
	a := NewDeepSeekAdapter()
	req := &ChatCompletionRequest{
		Model:    "deepseek-chat",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	}

	httpReq, err := a.TransformRequest(context.Background(), req, "sk-ds-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if httpReq.URL.String() != deepseekBaseURL {
		t.Errorf("expected URL %s, got %s", deepseekBaseURL, httpReq.URL.String())
	}
	if httpReq.Header.Get("Authorization") != "Bearer sk-ds-test" {
		t.Error("expected bearer auth header")
	}
	if httpReq.Header.Get("Content-Type") != "application/json" {
		t.Error("expected json content type")
	}

	body, _ := io.ReadAll(httpReq.Body)
	var parsed ChatCompletionRequest
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if parsed.Model != "deepseek-chat" {
		t.Errorf("expected model deepseek-chat, got %s", parsed.Model)
	}
}

func TestDeepSeekAdapter_TransformResponse_Basic(t *testing.T) {
	a := NewDeepSeekAdapter()

	respBody := `{
		"id": "ds-123",
		"object": "chat.completion",
		"created": 1700000000,
		"model": "deepseek-chat",
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

	if result.ID != "ds-123" {
		t.Errorf("expected id ds-123, got %s", result.ID)
	}
	if result.Choices[0].Message.GetContentText() != "Hello!" {
		t.Errorf("expected 'Hello!', got %q", result.Choices[0].Message.GetContentText())
	}
	if result.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", result.Usage.TotalTokens)
	}
}

func TestDeepSeekAdapter_TransformResponse_WithReasoningContent(t *testing.T) {
	a := NewDeepSeekAdapter()

	respBody := `{
		"id": "ds-456",
		"object": "chat.completion",
		"created": 1700000000,
		"model": "deepseek-reasoner",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "The answer is 42.",
				"reasoning_content": "Let me think step by step..."
			},
			"finish_reason": "stop"
		}],
		"usage": {"prompt_tokens": 20, "completion_tokens": 30, "total_tokens": 50}
	}`

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(respBody))),
	}

	result, err := a.TransformResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := result.Choices[0].Message.GetContentText()
	if !strings.Contains(content, "<think>") {
		t.Error("expected <think> tag in content when reasoning_content present")
	}
	if !strings.Contains(content, "Let me think step by step...") {
		t.Error("expected reasoning content in output")
	}
	if !strings.Contains(content, "The answer is 42.") {
		t.Error("expected original content in output")
	}
}

func TestDeepSeekAdapter_TransformResponse_Error(t *testing.T) {
	a := NewDeepSeekAdapter()
	resp := &http.Response{
		StatusCode: 429,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"rate limited"}`))),
	}

	_, err := a.TransformResponse(resp)
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestDeepSeekAdapter_TransformStreamChunk(t *testing.T) {
	a := NewDeepSeekAdapter()

	tests := []struct {
		name    string
		input   string
		wantN   int
		wantEOF bool
	}{
		{
			name:  "normal chunk",
			input: `data: {"id":"ds-1","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hi"}}]}`,
			wantN: 1,
		},
		{
			name:  "reasoning content chunk",
			input: `data: {"id":"ds-1","object":"chat.completion.chunk","choices":[{"delta":{"reasoning_content":"thinking..."}}]}`,
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
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if len(chunks) != tt.wantN {
				t.Errorf("expected %d chunks, got %d", tt.wantN, len(chunks))
			}
		})
	}
}

func TestDeepSeekAdapter_TransformStreamChunk_ReasoningContent(t *testing.T) {
	a := NewDeepSeekAdapter()

	input := `data: {"id":"ds-1","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-reasoner","choices":[{"index":0,"delta":{"reasoning_content":"step 1..."}}]}`
	chunks, err := a.TransformStreamChunk([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	// reasoning_content should be forwarded as regular content
	delta := chunks[0].Choices[0].Delta
	if delta.GetContentText() != "step 1..." {
		t.Errorf("expected reasoning content 'step 1...', got %q", delta.GetContentText())
	}
}

func TestDeepSeekAdapter_ExtractUsage(t *testing.T) {
	a := NewDeepSeekAdapter()

	body := []byte(`{"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`)
	usage := a.ExtractUsage(body)
	if usage == nil {
		t.Fatal("expected usage")
	}
	if usage.TotalTokens != 30 {
		t.Errorf("expected 30 total tokens, got %d", usage.TotalTokens)
	}
}

func TestDeepSeekAdapter_ExtractRateLimitInfo(t *testing.T) {
	a := NewDeepSeekAdapter()

	header := http.Header{}
	header.Set("x-ratelimit-remaining-requests", "50")
	header.Set("x-ratelimit-limit-requests", "60")

	info := a.ExtractRateLimitInfo(header)
	if info == nil {
		t.Fatal("expected rate limit info")
	}
	if info.RemainingRequests != 50 {
		t.Errorf("expected 50 remaining requests, got %d", info.RemainingRequests)
	}
}
