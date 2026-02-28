package proxy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseOpenAIResponsesSSE(t *testing.T) {
	sseData := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","delta":"Hello"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4o","status":"completed","usage":{"input_tokens":10,"output_tokens":5}}}`,
		"",
	}, "\n")

	recorder := httptest.NewRecorder()
	counts, err := ParseOpenAIResponsesSSE(context.Background(), strings.NewReader(sseData), recorder)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if counts.MessageID != "resp_1" {
		t.Errorf("MessageID = %q, want resp_1", counts.MessageID)
	}
	if counts.Model != "gpt-4o" {
		t.Errorf("Model = %q, want gpt-4o", counts.Model)
	}
	if counts.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", counts.InputTokens)
	}
	if counts.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", counts.OutputTokens)
	}

	// All events should be passed through
	body := recorder.Body.String()
	if !strings.Contains(body, "response.created") {
		t.Error("expected response.created event in passthrough output")
	}
	if !strings.Contains(body, "response.completed") {
		t.Error("expected response.completed event in passthrough output")
	}
}

func TestTranslateAnthropicSSEToResponses(t *testing.T) {
	sseData := strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_abc","model":"claude-sonnet-4-6","usage":{"input_tokens":20,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
		"",
		"event: content_block_start",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" there"}}`,
		"",
		"event: content_block_stop",
		`data: {"type":"content_block_stop","index":0}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":8}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")

	recorder := httptest.NewRecorder()
	counts, err := TranslateAnthropicSSEToResponses(context.Background(), strings.NewReader(sseData), recorder)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if counts.MessageID != "resp_msg_abc" {
		t.Errorf("MessageID = %q, want resp_msg_abc", counts.MessageID)
	}
	if counts.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want claude-sonnet-4-6", counts.Model)
	}
	if counts.InputTokens != 20 {
		t.Errorf("InputTokens = %d, want 20", counts.InputTokens)
	}
	if counts.OutputTokens != 8 {
		t.Errorf("OutputTokens = %d, want 8", counts.OutputTokens)
	}

	body := recorder.Body.String()

	// Check that translated events are present.
	expectedEvents := []string{
		"response.created",
		"response.output_item.added",
		"response.content_part.added",
		"response.output_text.delta",
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done",
		"response.completed",
	}
	for _, evt := range expectedEvents {
		if !strings.Contains(body, "event: "+evt) {
			t.Errorf("expected event %q in translated output", evt)
		}
	}

	// Check that the text deltas contain the expected text.
	if !strings.Contains(body, `"Hi"`) {
		t.Error("expected 'Hi' text delta in output")
	}
	if !strings.Contains(body, `" there"`) {
		t.Error("expected ' there' text delta in output")
	}
}

func TestParseOpenAIChatCompletionsSSE(t *testing.T) {
	// Simulate OpenAI Chat Completions SSE format with usage in the final chunk.
	sseData := strings.Join([]string{
		`data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","model":"gpt-4.1","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","model":"gpt-4.1","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","model":"gpt-4.1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":7,"total_tokens":22}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	recorder := httptest.NewRecorder()
	counts, err := ParseOpenAIChatCompletionsSSE(context.Background(), strings.NewReader(sseData), recorder)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if counts.MessageID != "chatcmpl-abc123" {
		t.Errorf("MessageID = %q, want chatcmpl-abc123", counts.MessageID)
	}
	if counts.Model != "gpt-4.1" {
		t.Errorf("Model = %q, want gpt-4.1", counts.Model)
	}
	if counts.InputTokens != 15 {
		t.Errorf("InputTokens = %d, want 15", counts.InputTokens)
	}
	if counts.OutputTokens != 7 {
		t.Errorf("OutputTokens = %d, want 7", counts.OutputTokens)
	}

	// All events should be passed through
	body := recorder.Body.String()
	if !strings.Contains(body, "chatcmpl-abc123") {
		t.Error("expected chatcmpl-abc123 in passthrough output")
	}
	if !strings.Contains(body, "[DONE]") {
		t.Error("expected [DONE] in passthrough output")
	}
}

func TestParseOpenAIChatCompletionsSSE_NoUsage(t *testing.T) {
	// Simulate SSE without usage data (stream_options.include_usage not set).
	sseData := strings.Join([]string{
		`data: {"id":"chatcmpl-xyz","object":"chat.completion.chunk","model":"gpt-4.1","choices":[{"index":0,"delta":{"role":"assistant","content":"Hi"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-xyz","object":"chat.completion.chunk","model":"gpt-4.1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	recorder := httptest.NewRecorder()
	counts, err := ParseOpenAIChatCompletionsSSE(context.Background(), strings.NewReader(sseData), recorder)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if counts.MessageID != "chatcmpl-xyz" {
		t.Errorf("MessageID = %q, want chatcmpl-xyz", counts.MessageID)
	}
	if counts.Model != "gpt-4.1" {
		t.Errorf("Model = %q, want gpt-4.1", counts.Model)
	}
	// No usage data — tokens should be zero
	if counts.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0", counts.InputTokens)
	}
	if counts.OutputTokens != 0 {
		t.Errorf("OutputTokens = %d, want 0", counts.OutputTokens)
	}
}

// mockResponseWriter is a minimal http.ResponseWriter + http.Flusher for tests.
type mockResponseWriter struct {
	buf     bytes.Buffer
	headers http.Header
	code    int
}

func (m *mockResponseWriter) Header() http.Header         { return m.headers }
func (m *mockResponseWriter) Write(b []byte) (int, error)  { return m.buf.Write(b) }
func (m *mockResponseWriter) WriteHeader(code int)          { m.code = code }
func (m *mockResponseWriter) Flush()                        {}
