package proxy

import (
	"encoding/json"
	"testing"
)

func TestTranslateResponsesToAnthropic_StringInput(t *testing.T) {
	maxTokens := 1024
	req := &ResponsesRequest{
		Model:           "claude-sonnet-4-6",
		Input:           json.RawMessage(`"Hello, world!"`),
		Instructions:    "Be helpful",
		MaxOutputTokens: &maxTokens,
		Stream:          false,
	}

	body, err := translateResponsesToAnthropic(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if result["model"] != "claude-sonnet-4-6" {
		t.Errorf("model = %v, want claude-sonnet-4-6", result["model"])
	}
	if result["system"] != "Be helpful" {
		t.Errorf("system = %v, want 'Be helpful'", result["system"])
	}
	if result["max_tokens"] != float64(1024) {
		t.Errorf("max_tokens = %v, want 1024", result["max_tokens"])
	}

	messages, ok := result["messages"].([]interface{})
	if !ok || len(messages) != 1 {
		t.Fatalf("expected 1 message, got %v", result["messages"])
	}
	msg := messages[0].(map[string]interface{})
	if msg["role"] != "user" {
		t.Errorf("message role = %v, want user", msg["role"])
	}
	if msg["content"] != "Hello, world!" {
		t.Errorf("message content = %v, want 'Hello, world!'", msg["content"])
	}
}

func TestTranslateResponsesToAnthropic_MessageArray(t *testing.T) {
	maxTokens := 2048
	req := &ResponsesRequest{
		Model: "claude-sonnet-4-6",
		Input: json.RawMessage(`[
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there!"},
			{"role": "user", "content": "How are you?"}
		]`),
		MaxOutputTokens: &maxTokens,
	}

	body, err := translateResponsesToAnthropic(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	messages, ok := result["messages"].([]interface{})
	if !ok || len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
}

func TestTranslateResponsesToAnthropic_ItemsWithType(t *testing.T) {
	maxTokens := 1024
	req := &ResponsesRequest{
		Model: "claude-sonnet-4-6",
		Input: json.RawMessage(`[
			{"type": "message", "role": "user", "content": "Hello from items"}
		]`),
		MaxOutputTokens: &maxTokens,
	}

	body, err := translateResponsesToAnthropic(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(body, &result)
	messages := result["messages"].([]interface{})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	msg := messages[0].(map[string]interface{})
	if msg["role"] != "user" {
		t.Errorf("role = %v, want user", msg["role"])
	}
}

func TestTranslateResponsesToAnthropic_DefaultMaxTokens(t *testing.T) {
	req := &ResponsesRequest{
		Model: "claude-sonnet-4-6",
		Input: json.RawMessage(`"test"`),
	}

	body, err := translateResponsesToAnthropic(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(body, &result)
	if result["max_tokens"] != float64(4096) {
		t.Errorf("max_tokens = %v, want 4096", result["max_tokens"])
	}
}

func TestTranslateResponsesToAnthropic_StreamAndTemp(t *testing.T) {
	temp := 0.7
	topP := 0.9
	req := &ResponsesRequest{
		Model:       "claude-sonnet-4-6",
		Input:       json.RawMessage(`"test"`),
		Stream:      true,
		Temperature: &temp,
		TopP:        &topP,
	}

	body, err := translateResponsesToAnthropic(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(body, &result)
	if result["stream"] != true {
		t.Errorf("stream = %v, want true", result["stream"])
	}
	if result["temperature"] != 0.7 {
		t.Errorf("temperature = %v, want 0.7", result["temperature"])
	}
	if result["top_p"] != 0.9 {
		t.Errorf("top_p = %v, want 0.9", result["top_p"])
	}
}

func TestTranslateAnthropicToResponsesJSON(t *testing.T) {
	anthropicBody := `{
		"id": "msg_123",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-6",
		"content": [
			{"type": "text", "text": "Hello!"}
		],
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 10,
			"output_tokens": 5
		}
	}`

	translated, counts, err := translateAnthropicToResponsesJSON([]byte(anthropicBody))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if counts.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", counts.InputTokens)
	}
	if counts.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", counts.OutputTokens)
	}
	if counts.MessageID != "resp_msg_123" {
		t.Errorf("MessageID = %q, want resp_msg_123", counts.MessageID)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(translated, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result["id"] != "resp_msg_123" {
		t.Errorf("id = %v, want resp_msg_123", result["id"])
	}
	if result["object"] != "response" {
		t.Errorf("object = %v, want response", result["object"])
	}
	if result["status"] != "completed" {
		t.Errorf("status = %v, want completed", result["status"])
	}

	output := result["output"].([]interface{})
	if len(output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(output))
	}

	usage := result["usage"].(map[string]interface{})
	if usage["total_tokens"] != float64(15) {
		t.Errorf("total_tokens = %v, want 15", usage["total_tokens"])
	}
}

func TestTranslateAnthropicToResponsesJSON_MaxTokensStop(t *testing.T) {
	anthropicBody := `{
		"id": "msg_456",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-6",
		"content": [{"type": "text", "text": "partial"}],
		"stop_reason": "max_tokens",
		"usage": {"input_tokens": 10, "output_tokens": 100}
	}`

	translated, _, err := translateAnthropicToResponsesJSON([]byte(anthropicBody))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(translated, &result)
	if result["status"] != "incomplete" {
		t.Errorf("status = %v, want incomplete", result["status"])
	}
}

func TestExtractTokensFromOpenAIResponsesJSON(t *testing.T) {
	body := `{
		"id": "resp_abc",
		"model": "gpt-4o",
		"usage": {
			"input_tokens": 100,
			"output_tokens": 50
		}
	}`

	counts := extractTokensFromOpenAIResponsesJSON([]byte(body))
	if counts.MessageID != "resp_abc" {
		t.Errorf("MessageID = %q, want resp_abc", counts.MessageID)
	}
	if counts.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", counts.InputTokens)
	}
	if counts.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", counts.OutputTokens)
	}
}
