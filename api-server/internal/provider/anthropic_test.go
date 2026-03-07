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

func TestAnthropicAdapter_Name(t *testing.T) {
	a := NewAnthropicAdapter()
	if a.Name() != "anthropic" {
		t.Errorf("expected 'anthropic', got %q", a.Name())
	}
}

func TestAnthropicAdapter_TransformRequest_Basic(t *testing.T) {
	a := NewAnthropicAdapter()
	req := &ChatCompletionRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	httpReq, err := a.TransformRequest(context.Background(), req, "sk-ant-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if httpReq.URL.String() != anthropicBaseURL {
		t.Errorf("expected URL %s, got %s", anthropicBaseURL, httpReq.URL.String())
	}
	if httpReq.Header.Get("x-api-key") != "sk-ant-test" {
		t.Error("expected x-api-key header")
	}
	if httpReq.Header.Get("anthropic-version") != "2023-06-01" {
		t.Error("expected anthropic-version header")
	}

	body, _ := io.ReadAll(httpReq.Body)
	var parsed anthropicRequest
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if parsed.MaxTokens != 4096 {
		t.Errorf("expected default max_tokens 4096, got %d", parsed.MaxTokens)
	}
}

func TestAnthropicAdapter_TransformRequest_SystemExtraction(t *testing.T) {
	a := NewAnthropicAdapter()
	req := &ChatCompletionRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hello"},
		},
	}

	httpReq, err := a.TransformRequest(context.Background(), req, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var parsed anthropicRequest
	json.Unmarshal(body, &parsed)

	if parsed.System != "You are helpful" {
		t.Errorf("expected system text, got %v", parsed.System)
	}
	if len(parsed.Messages) != 1 {
		t.Errorf("expected 1 message (system extracted), got %d", len(parsed.Messages))
	}
}

func TestAnthropicAdapter_TransformRequest_ToolDefinitions(t *testing.T) {
	a := NewAnthropicAdapter()
	req := &ChatCompletionRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []Message{
			{Role: "user", Content: "What's the weather?"},
		},
		Tools: []Tool{
			{
				Type: "function",
				Function: ToolFunction{
					Name:        "get_weather",
					Description: "Get weather info",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"city": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}

	httpReq, err := a.TransformRequest(context.Background(), req, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var parsed anthropicRequest
	json.Unmarshal(body, &parsed)

	if len(parsed.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(parsed.Tools))
	}
	if parsed.Tools[0].Name != "get_weather" {
		t.Errorf("expected tool name get_weather, got %s", parsed.Tools[0].Name)
	}
	if parsed.Tools[0].InputSchema == nil {
		t.Error("expected input_schema to be set")
	}
}

func TestAnthropicAdapter_TransformRequest_ToolChoice(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		wantType string
	}{
		{"auto string", "auto", "auto"},
		{"none string", "none", "none"},
		{"required string", "required", "any"},
		{
			"specific function",
			map[string]any{"function": map[string]any{"name": "my_tool"}},
			"tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAnthropicAdapter()
			req := &ChatCompletionRequest{
				Model:      "claude-sonnet-4-20250514",
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

			tc, ok := parsed["tool_choice"].(map[string]any)
			if !ok {
				t.Fatal("expected tool_choice to be an object")
			}
			if tc["type"] != tt.wantType {
				t.Errorf("expected type %s, got %v", tt.wantType, tc["type"])
			}
		})
	}
}

func TestAnthropicAdapter_TransformRequest_ToolResultMessage(t *testing.T) {
	a := NewAnthropicAdapter()
	req := &ChatCompletionRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []Message{
			{Role: "user", Content: "Call the tool"},
			{Role: "assistant", Content: "", ToolCalls: []ToolCall{
				{ID: "tc_1", Type: "function", Function: ToolCallFunction{Name: "get_weather", Arguments: `{"city":"NYC"}`}},
			}},
			{Role: "tool", Content: "72°F sunny", ToolCallID: "tc_1"},
		},
	}

	httpReq, err := a.TransformRequest(context.Background(), req, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var parsed anthropicRequest
	json.Unmarshal(body, &parsed)

	// Tool result should be converted to user role
	if len(parsed.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(parsed.Messages))
	}

	toolResult := parsed.Messages[2]
	if toolResult.Role != "user" {
		t.Errorf("expected role 'user' for tool result, got %s", toolResult.Role)
	}
}

func TestAnthropicAdapter_TransformRequest_StopSequences(t *testing.T) {
	a := NewAnthropicAdapter()
	req := &ChatCompletionRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Stop:     []any{"</s>", "<|end|>"},
	}

	httpReq, err := a.TransformRequest(context.Background(), req, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var parsed anthropicRequest
	json.Unmarshal(body, &parsed)

	if len(parsed.StopSequences) != 2 {
		t.Errorf("expected 2 stop sequences, got %d", len(parsed.StopSequences))
	}
}

func TestAnthropicAdapter_TransformRequest_MaxTokens(t *testing.T) {
	a := NewAnthropicAdapter()
	maxTokens := 1000
	req := &ChatCompletionRequest{
		Model:     "claude-sonnet-4-20250514",
		Messages:  []Message{{Role: "user", Content: "Hi"}},
		MaxTokens: &maxTokens,
	}

	httpReq, err := a.TransformRequest(context.Background(), req, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var parsed anthropicRequest
	json.Unmarshal(body, &parsed)

	if parsed.MaxTokens != 1000 {
		t.Errorf("expected max_tokens 1000, got %d", parsed.MaxTokens)
	}
}

func TestAnthropicAdapter_TransformRequest_ImageContent(t *testing.T) {
	a := NewAnthropicAdapter()
	req := &ChatCompletionRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []Message{
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "What's in this image?"},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": "data:image/png;base64,iVBORw0KGgo=",
						},
					},
				},
			},
		},
	}

	httpReq, err := a.TransformRequest(context.Background(), req, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var parsed map[string]json.RawMessage
	json.Unmarshal(body, &parsed)

	var messages []map[string]json.RawMessage
	json.Unmarshal(parsed["messages"], &messages)

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	var content []map[string]any
	json.Unmarshal(messages[0]["content"], &content)

	if len(content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(content))
	}
	if content[1]["type"] != "image" {
		t.Errorf("expected image type, got %v", content[1]["type"])
	}
}

func TestAnthropicAdapter_TransformResponse(t *testing.T) {
	a := NewAnthropicAdapter()

	respBody := `{
		"id": "msg_123",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "Hello!"}],
		"model": "claude-sonnet-4-20250514",
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(respBody))),
	}

	result, err := a.TransformResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != "msg_123" {
		t.Errorf("expected id msg_123, got %s", result.ID)
	}
	if result.Object != "chat.completion" {
		t.Errorf("expected object chat.completion, got %s", result.Object)
	}
	if len(result.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(result.Choices))
	}

	choice := result.Choices[0]
	if choice.Message.GetContentText() != "Hello!" {
		t.Errorf("expected content 'Hello!', got %q", choice.Message.GetContentText())
	}
	if *choice.FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %s", *choice.FinishReason)
	}

	if result.Usage.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", result.Usage.PromptTokens)
	}
	if result.Usage.CompletionTokens != 5 {
		t.Errorf("expected 5 completion tokens, got %d", result.Usage.CompletionTokens)
	}
}

func TestAnthropicAdapter_TransformResponse_ToolUse(t *testing.T) {
	a := NewAnthropicAdapter()

	respBody := `{
		"id": "msg_456",
		"type": "message",
		"role": "assistant",
		"content": [
			{"type": "text", "text": "Let me check the weather."},
			{"type": "tool_use", "id": "tu_1", "name": "get_weather", "input": {"city": "NYC"}}
		],
		"model": "claude-sonnet-4-20250514",
		"stop_reason": "tool_use",
		"usage": {"input_tokens": 20, "output_tokens": 15}
	}`

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(respBody))),
	}

	result, err := a.TransformResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	choice := result.Choices[0]
	if *choice.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason 'tool_calls', got %s", *choice.FinishReason)
	}
	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(choice.Message.ToolCalls))
	}
	tc := choice.Message.ToolCalls[0]
	if tc.ID != "tu_1" {
		t.Errorf("expected tool call id tu_1, got %s", tc.ID)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected function name get_weather, got %s", tc.Function.Name)
	}
}

func TestAnthropicAdapter_TransformResponse_Error(t *testing.T) {
	a := NewAnthropicAdapter()
	resp := &http.Response{
		StatusCode: 400,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"message":"bad request"}}`))),
	}

	_, err := a.TransformResponse(resp)
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestAnthropicAdapter_StopReasonMapping(t *testing.T) {
	tests := []struct {
		anthropic string
		openai    string
	}{
		{"end_turn", "stop"},
		{"max_tokens", "length"},
		{"tool_use", "tool_calls"},
		{"stop_sequence", "stop"},
		{"unknown", "stop"},
	}

	for _, tt := range tests {
		t.Run(tt.anthropic, func(t *testing.T) {
			got := mapStopReason(tt.anthropic)
			if got != tt.openai {
				t.Errorf("mapStopReason(%q) = %q, want %q", tt.anthropic, got, tt.openai)
			}
		})
	}
}

func TestAnthropicStreamTranslator_MessageStart(t *testing.T) {
	translator := NewAnthropicStreamTranslator()

	chunk := []byte("event: message_start\ndata: {\"message\":{\"id\":\"msg_1\",\"model\":\"claude-sonnet-4-20250514\"}}\n")
	chunks, err := translator.TranslateEvent(chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].ID != "msg_1" {
		t.Errorf("expected id msg_1, got %s", chunks[0].ID)
	}
	if chunks[0].Choices[0].Delta.Role != "assistant" {
		t.Errorf("expected role assistant")
	}
}

func TestAnthropicStreamTranslator_TextDelta(t *testing.T) {
	translator := NewAnthropicStreamTranslator()
	translator.messageID = "msg_1"
	translator.model = "claude-sonnet-4-20250514"

	chunk := []byte("event: content_block_delta\ndata: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n")
	chunks, err := translator.TranslateEvent(chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Choices[0].Delta.GetContentText() != "Hello" {
		t.Errorf("expected content 'Hello'")
	}
}

func TestAnthropicStreamTranslator_ToolUseStart(t *testing.T) {
	translator := NewAnthropicStreamTranslator()
	translator.messageID = "msg_1"
	translator.model = "claude-sonnet-4-20250514"

	chunk := []byte("event: content_block_start\ndata: {\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"tu_1\",\"name\":\"get_weather\"}}\n")
	chunks, err := translator.TranslateEvent(chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	tc := chunks[0].Choices[0].Delta.ToolCalls
	if len(tc) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(tc))
	}
	if tc[0].ID != "tu_1" {
		t.Errorf("expected tool call id tu_1")
	}
	if tc[0].Function.Name != "get_weather" {
		t.Errorf("expected function name get_weather")
	}
}

func TestAnthropicStreamTranslator_InputJsonDelta(t *testing.T) {
	translator := NewAnthropicStreamTranslator()
	translator.messageID = "msg_1"
	translator.model = "claude-sonnet-4-20250514"

	chunk := []byte("event: content_block_delta\ndata: {\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"city\\\"\"}}\n")
	chunks, err := translator.TranslateEvent(chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	tc := chunks[0].Choices[0].Delta.ToolCalls
	if len(tc) != 1 {
		t.Fatalf("expected 1 tool call delta")
	}
}

func TestAnthropicStreamTranslator_MessageDelta(t *testing.T) {
	translator := NewAnthropicStreamTranslator()
	translator.messageID = "msg_1"
	translator.model = "claude-sonnet-4-20250514"

	chunk := []byte("event: message_delta\ndata: {\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":10}}\n")
	chunks, err := translator.TranslateEvent(chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if *chunks[0].Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop'")
	}
}

func TestAnthropicStreamTranslator_MessageStop(t *testing.T) {
	translator := NewAnthropicStreamTranslator()

	chunk := []byte("event: message_stop\ndata: {}\n")
	_, err := translator.TranslateEvent(chunk)
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

func TestAnthropicStreamTranslator_Ping(t *testing.T) {
	translator := NewAnthropicStreamTranslator()

	chunk := []byte("event: ping\ndata: {}\n")
	chunks, err := translator.TranslateEvent(chunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chunks != nil {
		t.Errorf("expected nil chunks for ping")
	}
}

func TestAnthropicStreamTranslator_FullConversation(t *testing.T) {
	translator := NewAnthropicStreamTranslator()

	events := []string{
		"event: message_start\ndata: {\"message\":{\"id\":\"msg_1\",\"model\":\"claude-sonnet-4-20250514\"}}",
		"event: content_block_start\ndata: {\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}",
		"event: content_block_delta\ndata: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}",
		"event: content_block_delta\ndata: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" there!\"}}",
		"event: content_block_stop\ndata: {\"index\":0}",
		"event: message_delta\ndata: {\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}",
		"event: message_stop\ndata: {}",
	}

	var allChunks []*StreamChunk
	for _, evt := range events {
		chunks, err := translator.TranslateEvent([]byte(evt))
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error on event %q: %v", evt[:30], err)
		}
		allChunks = append(allChunks, chunks...)
	}

	if len(allChunks) < 3 {
		t.Errorf("expected at least 3 chunks, got %d", len(allChunks))
	}

	// First chunk should have role
	if allChunks[0].Choices[0].Delta.Role != "assistant" {
		t.Error("first chunk should have assistant role")
	}
}

func TestAnthropicAdapter_ExtractRateLimitInfo(t *testing.T) {
	a := NewAnthropicAdapter()

	header := http.Header{}
	header.Set("anthropic-ratelimit-requests-remaining", "45")
	header.Set("anthropic-ratelimit-tokens-remaining", "80000")
	header.Set("anthropic-ratelimit-requests-limit", "50")
	header.Set("anthropic-ratelimit-tokens-limit", "100000")

	info := a.ExtractRateLimitInfo(header)
	if info == nil {
		t.Fatal("expected rate limit info")
	}
	if info.RemainingRequests != 45 {
		t.Errorf("expected 45 remaining requests, got %d", info.RemainingRequests)
	}
	if info.LimitTokens != 100000 {
		t.Errorf("expected 100000 limit tokens, got %d", info.LimitTokens)
	}
}

func TestAnthropicAdapter_ExtractUsage(t *testing.T) {
	a := NewAnthropicAdapter()

	body := []byte(`{"usage":{"input_tokens":10,"output_tokens":20}}`)
	usage := a.ExtractUsage(body)
	if usage == nil {
		t.Fatal("expected usage")
	}
	if usage.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 20 {
		t.Errorf("expected 20 completion tokens, got %d", usage.CompletionTokens)
	}
	if usage.TotalTokens != 30 {
		t.Errorf("expected 30 total tokens, got %d", usage.TotalTokens)
	}
}

func TestAnthropicAdapter_ExtractUsage_Empty(t *testing.T) {
	a := NewAnthropicAdapter()
	usage := a.ExtractUsage([]byte(`{"id":"test"}`))
	if usage != nil {
		t.Error("expected nil for missing usage")
	}
}

func TestAnthropicAdapter_TransformRequest_MultipleSystemMessages(t *testing.T) {
	a := NewAnthropicAdapter()
	req := &ChatCompletionRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []Message{
			{Role: "system", Content: "Be helpful"},
			{Role: "system", Content: "Be concise"},
			{Role: "user", Content: "Hello"},
		},
	}

	httpReq, err := a.TransformRequest(context.Background(), req, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var parsed anthropicRequest
	json.Unmarshal(body, &parsed)

	systemStr, ok := parsed.System.(string)
	if !ok {
		t.Fatal("expected system to be a string")
	}
	if !strings.Contains(systemStr, "Be helpful") || !strings.Contains(systemStr, "Be concise") {
		t.Errorf("expected both system messages, got %q", systemStr)
	}
	if len(parsed.Messages) != 1 {
		t.Errorf("expected 1 non-system message, got %d", len(parsed.Messages))
	}
}
