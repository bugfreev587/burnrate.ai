package provider

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestClassifyHTTPStatus(t *testing.T) {
	tests := []struct {
		status    int
		wantType  string
		wantRetry bool
	}{
		{400, ErrorTypeInvalidRequest, false},
		{401, ErrorTypeAuthentication, false},
		{403, ErrorTypePermission, false},
		{404, ErrorTypeNotFound, false},
		{413, ErrorTypeInvalidRequest, false},
		{429, ErrorTypeRateLimit, true},
		{500, ErrorTypeServer, true},
		{502, ErrorTypeServer, true},
		{503, ErrorTypeServer, true},
		{529, ErrorTypeOverloaded, true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			gotType, gotRetry := ClassifyHTTPStatus(tt.status)
			if gotType != tt.wantType {
				t.Errorf("status %d: expected type %q, got %q", tt.status, tt.wantType, gotType)
			}
			if gotRetry != tt.wantRetry {
				t.Errorf("status %d: expected retryable=%v, got %v", tt.status, tt.wantRetry, gotRetry)
			}
		})
	}
}

func TestParseProviderError_OpenAIFormat(t *testing.T) {
	body := []byte(`{
		"error": {
			"message": "Rate limit reached",
			"type": "rate_limit_error",
			"code": 429
		}
	}`)

	header := http.Header{}
	header.Set("Retry-After", "5")

	pe := ParseProviderError(429, body, "openai", "gpt-4o", header)

	if pe.Message != "Rate limit reached" {
		t.Errorf("expected 'Rate limit reached', got %q", pe.Message)
	}
	if pe.Type != "rate_limit_error" {
		t.Errorf("expected type rate_limit_error, got %s", pe.Type)
	}
	if pe.Provider != "openai" {
		t.Errorf("expected provider openai, got %s", pe.Provider)
	}
	if pe.RetryAfter != 5 {
		t.Errorf("expected retry-after 5, got %d", pe.RetryAfter)
	}
	if !pe.Retryable {
		t.Error("429 should be retryable")
	}
}

func TestParseProviderError_AnthropicFormat(t *testing.T) {
	body := []byte(`{
		"error": {
			"type": "overloaded_error",
			"message": "Overloaded"
		}
	}`)

	pe := ParseProviderError(529, body, "anthropic", "claude-sonnet-4-20250514", http.Header{})

	if pe.Message != "Overloaded" {
		t.Errorf("expected 'Overloaded', got %q", pe.Message)
	}
	if !pe.Retryable {
		t.Error("529 should be retryable")
	}
}

func TestParseProviderError_PlainText(t *testing.T) {
	body := []byte("Internal Server Error")

	pe := ParseProviderError(500, body, "deepseek", "deepseek-chat", http.Header{})

	if pe.Message != "Internal Server Error" {
		t.Errorf("expected fallback to body, got %q", pe.Message)
	}
}

func TestParseProviderError_NonRetryable(t *testing.T) {
	body := []byte(`{"error":{"message":"Invalid model","type":"invalid_request_error"}}`)

	pe := ParseProviderError(400, body, "openai", "gpt-nonexist", http.Header{})

	if pe.Retryable {
		t.Error("400 should not be retryable")
	}
	if pe.Type != "invalid_request_error" {
		t.Errorf("expected invalid_request_error, got %s", pe.Type)
	}
}

func TestProviderError_ToJSON(t *testing.T) {
	pe := &ProviderError{
		StatusCode: 429,
		Message:    "Too many requests",
		Type:       ErrorTypeRateLimit,
		Code:       429,
		Provider:   "openai",
		Model:      "gpt-4o",
	}

	data := pe.ToJSON()

	var parsed struct {
		Error struct {
			Message  string `json:"message"`
			Type     string `json:"type"`
			Provider string `json:"provider"`
			Model    string `json:"model"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if parsed.Error.Message != "Too many requests" {
		t.Errorf("expected message in JSON output")
	}
	if parsed.Error.Provider != "openai" {
		t.Errorf("expected provider in JSON output")
	}
}

func TestProviderError_ErrorInterface(t *testing.T) {
	pe := &ProviderError{
		StatusCode: 503,
		Message:    "Service unavailable",
		Provider:   "anthropic",
	}

	errStr := pe.Error()
	if errStr != "anthropic: Service unavailable (status 503)" {
		t.Errorf("unexpected error string: %s", errStr)
	}
}
