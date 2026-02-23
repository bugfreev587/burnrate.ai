package proxy

import (
	"encoding/json"
	"fmt"
)

// ─── Request translation: Responses → Anthropic Messages ─────────────────────

// translateResponsesToAnthropic converts an OpenAI Responses API request
// into an Anthropic Messages API request body.
func translateResponsesToAnthropic(req *ResponsesRequest) ([]byte, error) {
	messages, err := translateInputToMessages(req.Input)
	if err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"model":    req.Model,
		"messages": messages,
	}

	if req.Instructions != "" {
		body["system"] = req.Instructions
	}

	if req.MaxOutputTokens != nil {
		body["max_tokens"] = *req.MaxOutputTokens
	} else {
		body["max_tokens"] = 4096
	}

	if req.Stream {
		body["stream"] = true
	}

	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}

	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}

	return json.Marshal(body)
}

// translateInputToMessages converts the Responses API "input" field into
// Anthropic Messages format. Supports:
//   - string input → single user message
//   - array of {role, content} messages
//   - array of items with type "message"
func translateInputToMessages(input json.RawMessage) ([]map[string]interface{}, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("input field is required")
	}

	// Try string input first.
	var strInput string
	if err := json.Unmarshal(input, &strInput); err == nil {
		return []map[string]interface{}{
			{"role": "user", "content": strInput},
		}, nil
	}

	// Try array input.
	var items []json.RawMessage
	if err := json.Unmarshal(input, &items); err != nil {
		return nil, fmt.Errorf("input format not supported for Anthropic translation: expected string or array")
	}

	var messages []map[string]interface{}
	for _, item := range items {
		msg, err := translateInputItem(item)
		if err != nil {
			return nil, err
		}
		if msg != nil {
			messages = append(messages, msg)
		}
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("input array produced no messages")
	}

	return messages, nil
}

// translateInputItem converts a single input array item into an Anthropic message.
func translateInputItem(raw json.RawMessage) (map[string]interface{}, error) {
	// Peek at the item to determine its shape.
	var peek struct {
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &peek); err != nil {
		return nil, fmt.Errorf("input format not supported for Anthropic translation: invalid item")
	}

	// Items with type "message" — extract role and content.
	if peek.Type == "message" {
		content, err := translateContent(peek.Content)
		if err != nil {
			return nil, err
		}
		role := peek.Role
		if role == "" {
			role = "user"
		}
		return map[string]interface{}{
			"role":    role,
			"content": content,
		}, nil
	}

	// Simple {role, content} messages (no type field or type is empty).
	if peek.Role != "" {
		content, err := translateContent(peek.Content)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"role":    peek.Role,
			"content": content,
		}, nil
	}

	return nil, fmt.Errorf("input format not supported for Anthropic translation: item has no role or recognized type")
}

// translateContent converts the content field from Responses format to Anthropic format.
// Supports string content and array of content parts with text type.
func translateContent(raw json.RawMessage) (interface{}, error) {
	if len(raw) == 0 {
		return "", nil
	}

	// Try string content.
	var strContent string
	if err := json.Unmarshal(raw, &strContent); err == nil {
		return strContent, nil
	}

	// Try array content — pass through as-is (Anthropic supports content arrays).
	var arrContent []map[string]interface{}
	if err := json.Unmarshal(raw, &arrContent); err == nil {
		// Validate: only text type content blocks are supported.
		for _, block := range arrContent {
			blockType, _ := block["type"].(string)
			if blockType != "text" && blockType != "" {
				return nil, fmt.Errorf("input format not supported for Anthropic translation: content type %q not supported", blockType)
			}
		}
		return arrContent, nil
	}

	return nil, fmt.Errorf("input format not supported for Anthropic translation: unsupported content format")
}

// ─── Response translation: Anthropic Messages → Responses ────────────────────

// translateAnthropicToResponsesJSON converts an Anthropic Messages API response
// into an OpenAI Responses API response.
func translateAnthropicToResponsesJSON(body []byte) ([]byte, TokenCounts, error) {
	var anthropicResp struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Role    string `json:"role"`
		Model   string `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens         int64 `json:"input_tokens"`
			OutputTokens        int64 `json:"output_tokens"`
			CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return nil, TokenCounts{}, fmt.Errorf("failed to parse Anthropic response: %w", err)
	}

	counts := TokenCounts{
		MessageID:           "resp_" + anthropicResp.ID,
		Model:               anthropicResp.Model,
		InputTokens:         anthropicResp.Usage.InputTokens,
		OutputTokens:        anthropicResp.Usage.OutputTokens,
		CacheCreationTokens: anthropicResp.Usage.CacheCreationTokens,
		CacheReadTokens:     anthropicResp.Usage.CacheReadTokens,
	}

	// Build output items from content blocks.
	var output []map[string]interface{}
	for i, block := range anthropicResp.Content {
		if block.Type == "text" {
			output = append(output, map[string]interface{}{
				"type": "message",
				"id":   fmt.Sprintf("msg_%d", i),
				"role": "assistant",
				"content": []map[string]interface{}{
					{
						"type": "output_text",
						"text": block.Text,
					},
				},
				"status": "completed",
			})
		}
	}

	totalTokens := anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens

	status := "completed"
	if anthropicResp.StopReason == "max_tokens" {
		status = "incomplete"
	}

	responsesResp := map[string]interface{}{
		"id":     "resp_" + anthropicResp.ID,
		"object": "response",
		"status": status,
		"model":  anthropicResp.Model,
		"output": output,
		"usage": map[string]interface{}{
			"input_tokens":  anthropicResp.Usage.InputTokens,
			"output_tokens": anthropicResp.Usage.OutputTokens,
			"total_tokens":  totalTokens,
		},
	}

	translated, err := json.Marshal(responsesResp)
	if err != nil {
		return nil, counts, fmt.Errorf("failed to marshal Responses response: %w", err)
	}

	return translated, counts, nil
}

// translateAnthropicErrorToResponses wraps an Anthropic error body in the
// Responses API error shape.
func translateAnthropicErrorToResponses(statusCode int, body []byte) []byte {
	var anthropicErr struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &anthropicErr); err != nil {
		// Can't parse — return as-is.
		return body
	}

	responsesErr := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    anthropicErr.Error.Type,
			"message": anthropicErr.Error.Message,
			"code":    statusCode,
		},
	}
	result, err := json.Marshal(responsesErr)
	if err != nil {
		return body
	}
	return result
}
