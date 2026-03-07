package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestMistralAdapter_Name(t *testing.T) {
	a := NewMistralAdapter()
	if a.Name() != "mistral" {
		t.Errorf("expected 'mistral', got %q", a.Name())
	}
}

func TestMistralAdapter_TransformRequest_Basic(t *testing.T) {
	a := NewMistralAdapter()
	req := &ChatCompletionRequest{
		Model:    "mistral-large-latest",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	}

	httpReq, err := a.TransformRequest(context.Background(), req, "sk-mistral-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if httpReq.URL.String() != mistralBaseURL {
		t.Errorf("expected URL %s, got %s", mistralBaseURL, httpReq.URL.String())
	}
	if httpReq.Header.Get("Authorization") != "Bearer sk-mistral-test" {
		t.Error("expected bearer auth header")
	}
}

func TestMistralAdapter_TransformRequest_StripsUnsupportedParams(t *testing.T) {
	a := NewMistralAdapter()
	fp := 0.5
	pp := 0.5
	seed := 42
	req := &ChatCompletionRequest{
		Model:            "mistral-large-latest",
		Messages:         []Message{{Role: "user", Content: "Hi"}},
		FrequencyPenalty: &fp,
		PresencePenalty:  &pp,
		Seed:             &seed,
		User:             "test-user",
	}

	httpReq, err := a.TransformRequest(context.Background(), req, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var parsed map[string]any
	json.Unmarshal(body, &parsed)

	if _, ok := parsed["frequency_penalty"]; ok {
		t.Error("frequency_penalty should not be sent to Mistral")
	}
	if _, ok := parsed["presence_penalty"]; ok {
		t.Error("presence_penalty should not be sent to Mistral")
	}
	if _, ok := parsed["seed"]; ok {
		t.Error("seed should not be sent to Mistral")
	}
	if _, ok := parsed["user"]; ok {
		t.Error("user should not be sent to Mistral")
	}
}

func TestMistralAdapter_TransformRequest_ToolChoice(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{"auto", "auto", "auto"},
		{"none", "none", "none"},
		{"required maps to any", "required", "any"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewMistralAdapter()
			req := &ChatCompletionRequest{
				Model:      "mistral-large-latest",
				Messages:   []Message{{Role: "user", Content: "Hi"}},
				ToolChoice: tt.input,
				Tools:      []Tool{{Type: "function", Function: ToolFunction{Name: "test", Parameters: map[string]any{}}}},
			}

			httpReq, err := a.TransformRequest(context.Background(), req, "key")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			body, _ := io.ReadAll(httpReq.Body)
			var parsed map[string]any
			json.Unmarshal(body, &parsed)

			if parsed["tool_choice"] != tt.expected {
				t.Errorf("expected tool_choice %v, got %v", tt.expected, parsed["tool_choice"])
			}
		})
	}
}

func TestMistralAdapter_TransformResponse(t *testing.T) {
	a := NewMistralAdapter()

	respBody := `{
		"id": "cmpl-mistral-123",
		"object": "chat.completion",
		"created": 1700000000,
		"model": "mistral-large-latest",
		"choices": [{
			"index": 0,
			"message": {"role": "assistant", "content": "Bonjour!"},
			"finish_reason": "stop"
		}],
		"usage": {"prompt_tokens": 8, "completion_tokens": 3, "total_tokens": 11}
	}`

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(respBody))),
	}

	result, err := a.TransformResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != "cmpl-mistral-123" {
		t.Errorf("expected id cmpl-mistral-123, got %s", result.ID)
	}
	if result.Usage.TotalTokens != 11 {
		t.Errorf("expected 11 total tokens, got %d", result.Usage.TotalTokens)
	}
}

func TestMistralAdapter_TransformResponse_Error(t *testing.T) {
	a := NewMistralAdapter()
	resp := &http.Response{
		StatusCode: 400,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"message":"bad request"}}`))),
	}

	_, err := a.TransformResponse(resp)
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestMistralAdapter_TransformStreamChunk(t *testing.T) {
	a := NewMistralAdapter()

	tests := []struct {
		name    string
		input   string
		wantN   int
		wantEOF bool
	}{
		{
			name:  "normal chunk",
			input: `data: {"id":"m-1","object":"chat.completion.chunk","choices":[{"delta":{"content":"Salut"}}]}`,
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

func TestMistralAdapter_ExtractUsage(t *testing.T) {
	a := NewMistralAdapter()

	body := []byte(`{"usage":{"prompt_tokens":5,"completion_tokens":10,"total_tokens":15}}`)
	usage := a.ExtractUsage(body)
	if usage == nil {
		t.Fatal("expected usage")
	}
	if usage.TotalTokens != 15 {
		t.Errorf("expected 15, got %d", usage.TotalTokens)
	}
}

func TestMistralAdapter_ExtractRateLimitInfo(t *testing.T) {
	a := NewMistralAdapter()

	header := http.Header{}
	header.Set("x-ratelimit-remaining-requests", "25")

	info := a.ExtractRateLimitInfo(header)
	if info == nil {
		t.Fatal("expected rate limit info")
	}
	if info.RemainingRequests != 25 {
		t.Errorf("expected 25 remaining, got %d", info.RemainingRequests)
	}
}
