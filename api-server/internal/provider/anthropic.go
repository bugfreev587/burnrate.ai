package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	anthropicBaseURL = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
)

// AnthropicAdapter implements ProviderAdapter for Anthropic's Messages API.
type AnthropicAdapter struct {
	streamState *AnthropicStreamTranslator
	baseURL     string // overridable for testing; defaults to anthropicBaseURL
}

func NewAnthropicAdapter() *AnthropicAdapter {
	return &AnthropicAdapter{
		streamState: NewAnthropicStreamTranslator(),
		baseURL:     anthropicBaseURL,
	}
}

func (a *AnthropicAdapter) Name() string {
	return "anthropic"
}

// --- Anthropic-specific request types ---

type anthropicRequest struct {
	Model         string                `json:"model"`
	Messages      []anthropicMessage    `json:"messages"`
	System        any                   `json:"system,omitempty"` // string or []anthropicContent
	MaxTokens     int                   `json:"max_tokens"`
	Stream        bool                  `json:"stream,omitempty"`
	Temperature   *float64              `json:"temperature,omitempty"`
	TopP          *float64              `json:"top_p,omitempty"`
	StopSequences []string              `json:"stop_sequences,omitempty"`
	Tools         []anthropicTool       `json:"tools,omitempty"`
	ToolChoice    any                   `json:"tool_choice,omitempty"`
	Metadata      map[string]any        `json:"metadata,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []anthropicContent
}

type anthropicContent struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Input       any    `json:"input,omitempty"`
	ToolUseID   string `json:"tool_use_id,omitempty"`
	Content     any    `json:"content,omitempty"` // for tool_result
	Source      *anthropicImageSource `json:"source,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

// --- Anthropic response types ---

type anthropicResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Role         string             `json:"role"`
	Content      []anthropicContent `json:"content"`
	Model        string             `json:"model"`
	StopReason   string             `json:"stop_reason"`
	StopSequence *string            `json:"stop_sequence"`
	Usage        anthropicUsage     `json:"usage"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// --- Request Transformation ---

func (a *AnthropicAdapter) TransformRequest(ctx context.Context, req *ChatCompletionRequest, apiKey string) (*http.Request, error) {
	antReq, err := a.buildAnthropicRequest(req)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(antReq)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	url := a.baseURL
	if url == "" {
		url = anthropicBaseURL
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	return httpReq, nil
}

func (a *AnthropicAdapter) buildAnthropicRequest(req *ChatCompletionRequest) (*anthropicRequest, error) {
	antReq := &anthropicRequest{
		Model:       req.Model,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}

	// Default max_tokens
	if req.MaxTokens != nil {
		antReq.MaxTokens = *req.MaxTokens
	} else {
		antReq.MaxTokens = 4096
	}

	// Convert stop sequences
	if req.Stop != nil {
		switch v := req.Stop.(type) {
		case string:
			antReq.StopSequences = []string{v}
		case []any:
			for _, s := range v {
				if str, ok := s.(string); ok {
					antReq.StopSequences = append(antReq.StopSequences, str)
				}
			}
		}
	}

	// Extract system messages and convert other messages
	var systemParts []string
	var messages []anthropicMessage

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			systemParts = append(systemParts, msg.GetContentText())
			continue
		}

		antMsg, err := a.convertMessage(msg)
		if err != nil {
			return nil, err
		}
		messages = append(messages, antMsg)
	}

	if len(systemParts) > 0 {
		antReq.System = strings.Join(systemParts, "\n\n")
	}
	antReq.Messages = messages

	// Convert tools
	if len(req.Tools) > 0 {
		for _, tool := range req.Tools {
			antReq.Tools = append(antReq.Tools, anthropicTool{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				InputSchema: tool.Function.Parameters,
			})
		}
	}

	// Convert tool_choice
	if req.ToolChoice != nil {
		antReq.ToolChoice = a.convertToolChoice(req.ToolChoice)
	}

	return antReq, nil
}

func (a *AnthropicAdapter) convertMessage(msg Message) (anthropicMessage, error) {
	// Handle tool result messages
	if msg.Role == "tool" {
		return anthropicMessage{
			Role: "user",
			Content: []anthropicContent{
				{
					Type:      "tool_result",
					ToolUseID: msg.ToolCallID,
					Content:   msg.GetContentText(),
				},
			},
		}, nil
	}

	antMsg := anthropicMessage{Role: msg.Role}

	// Convert content
	switch v := msg.Content.(type) {
	case string:
		if len(msg.ToolCalls) > 0 {
			// Assistant message with tool calls needs content block array
			content := a.buildAssistantContent(v, msg.ToolCalls)
			antMsg.Content = content
		} else {
			antMsg.Content = v
		}
	case []any:
		parts, err := a.convertContentParts(v)
		if err != nil {
			return anthropicMessage{}, err
		}
		antMsg.Content = parts
	default:
		if msg.Content == nil && len(msg.ToolCalls) > 0 {
			content := a.buildAssistantContent("", msg.ToolCalls)
			antMsg.Content = content
		} else if msg.Content != nil {
			// Try JSON roundtrip for unknown types
			raw, _ := json.Marshal(msg.Content)
			var parts []any
			if json.Unmarshal(raw, &parts) == nil {
				converted, err := a.convertContentParts(parts)
				if err != nil {
					return anthropicMessage{}, err
				}
				antMsg.Content = converted
			} else {
				antMsg.Content = string(raw)
			}
		}
	}

	return antMsg, nil
}

func (a *AnthropicAdapter) buildAssistantContent(text string, toolCalls []ToolCall) []anthropicContent {
	var content []anthropicContent
	if text != "" {
		content = append(content, anthropicContent{Type: "text", Text: text})
	}
	for _, tc := range toolCalls {
		var input any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
		content = append(content, anthropicContent{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}
	return content
}

func (a *AnthropicAdapter) convertContentParts(parts []any) ([]anthropicContent, error) {
	var result []anthropicContent
	for _, part := range parts {
		raw, _ := json.Marshal(part)
		var cp ContentPart
		if err := json.Unmarshal(raw, &cp); err != nil {
			continue
		}

		switch cp.Type {
		case "text":
			result = append(result, anthropicContent{Type: "text", Text: cp.Text})
		case "image_url":
			if cp.ImageURL != nil {
				ac, err := a.convertImageURL(cp.ImageURL.URL)
				if err != nil {
					return nil, err
				}
				result = append(result, ac)
			}
		}
	}
	return result, nil
}

func (a *AnthropicAdapter) convertImageURL(url string) (anthropicContent, error) {
	// Handle base64 data URLs: data:image/png;base64,xxxxx
	if strings.HasPrefix(url, "data:") {
		parts := strings.SplitN(url, ",", 2)
		if len(parts) != 2 {
			return anthropicContent{}, fmt.Errorf("invalid data URL format")
		}
		meta := strings.TrimPrefix(parts[0], "data:")
		meta = strings.TrimSuffix(meta, ";base64")

		return anthropicContent{
			Type: "image",
			Source: &anthropicImageSource{
				Type:      "base64",
				MediaType: meta,
				Data:      parts[1],
			},
		}, nil
	}

	// For regular URLs, Anthropic supports url source type
	return anthropicContent{
		Type: "image",
		Source: &anthropicImageSource{
			Type: "url",
			Data: url,
		},
	}, nil
}

func (a *AnthropicAdapter) convertToolChoice(tc any) any {
	switch v := tc.(type) {
	case string:
		switch v {
		case "auto":
			return map[string]string{"type": "auto"}
		case "none":
			return map[string]string{"type": "none"}
		case "required":
			return map[string]string{"type": "any"}
		default:
			return map[string]string{"type": "auto"}
		}
	case map[string]any:
		if fn, ok := v["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok {
				return map[string]string{"type": "tool", "name": name}
			}
		}
	}
	return nil
}

// --- Response Transformation ---

func (a *AnthropicAdapter) TransformResponse(resp *http.Response) (*ChatCompletionResponse, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic error (status %d): %s", resp.StatusCode, string(body))
	}

	var antResp anthropicResponse
	if err := json.Unmarshal(body, &antResp); err != nil {
		return nil, fmt.Errorf("unmarshal anthropic response: %w", err)
	}

	return a.convertResponse(&antResp), nil
}

func (a *AnthropicAdapter) convertResponse(antResp *anthropicResponse) *ChatCompletionResponse {
	msg := &Message{
		Role: "assistant",
	}

	// Extract text content and tool calls
	var textParts []string
	var toolCalls []ToolCall

	for _, block := range antResp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: ToolCallFunction{
					Name:      block.Name,
					Arguments: string(args),
				},
			})
		}
	}

	msg.Content = strings.Join(textParts, "")
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}

	finishReason := mapStopReason(antResp.StopReason)

	return &ChatCompletionResponse{
		ID:      antResp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   antResp.Model,
		Choices: []Choice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: &finishReason,
			},
		},
		Usage: &Usage{
			PromptTokens:     antResp.Usage.InputTokens,
			CompletionTokens: antResp.Usage.OutputTokens,
			TotalTokens:      antResp.Usage.InputTokens + antResp.Usage.OutputTokens,
		},
	}
}

func mapStopReason(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "stop_sequence":
		return "stop"
	default:
		return "stop"
	}
}

// --- Streaming ---

// AnthropicStreamTranslator maintains state for translating Anthropic SSE events
// to OpenAI-compatible SSE chunks.
type AnthropicStreamTranslator struct {
	messageID         string
	model             string
	contentBlockIndex int
	toolCallIndex     int
	created           int64
}

func NewAnthropicStreamTranslator() *AnthropicStreamTranslator {
	return &AnthropicStreamTranslator{
		created: time.Now().Unix(),
	}
}

func (a *AnthropicAdapter) TransformStreamChunk(chunk []byte) ([]*StreamChunk, error) {
	return a.streamState.TranslateEvent(chunk)
}

func (t *AnthropicStreamTranslator) TranslateEvent(chunk []byte) ([]*StreamChunk, error) {
	lines := strings.Split(string(chunk), "\n")
	var eventType, eventData string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			eventData = strings.TrimPrefix(line, "data: ")
		}
	}

	if eventType == "" || eventData == "" {
		return nil, nil
	}

	switch eventType {
	case "message_start":
		return t.handleMessageStart(eventData)
	case "content_block_start":
		return t.handleContentBlockStart(eventData)
	case "content_block_delta":
		return t.handleContentBlockDelta(eventData)
	case "content_block_stop":
		t.contentBlockIndex++
		return nil, nil
	case "message_delta":
		return t.handleMessageDelta(eventData)
	case "message_stop":
		return nil, io.EOF
	case "ping":
		return nil, nil
	default:
		return nil, nil
	}
}

func (t *AnthropicStreamTranslator) handleMessageStart(data string) ([]*StreamChunk, error) {
	var evt struct {
		Message struct {
			ID    string `json:"id"`
			Model string `json:"model"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return nil, fmt.Errorf("parse message_start: %w", err)
	}

	t.messageID = evt.Message.ID
	t.model = evt.Message.Model

	role := "assistant"
	return []*StreamChunk{
		{
			ID:      t.messageID,
			Object:  "chat.completion.chunk",
			Created: t.created,
			Model:   t.model,
			Choices: []Choice{
				{
					Index: 0,
					Delta: &Message{Role: role},
				},
			},
		},
	}, nil
}

func (t *AnthropicStreamTranslator) handleContentBlockStart(data string) ([]*StreamChunk, error) {
	var evt struct {
		Index        int `json:"index"`
		ContentBlock struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"content_block"`
	}
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return nil, fmt.Errorf("parse content_block_start: %w", err)
	}

	if evt.ContentBlock.Type == "tool_use" {
		sc := &StreamChunk{
			ID:      t.messageID,
			Object:  "chat.completion.chunk",
			Created: t.created,
			Model:   t.model,
			Choices: []Choice{
				{
					Index: 0,
					Delta: &Message{
						ToolCalls: []ToolCall{
							{
								ID:   evt.ContentBlock.ID,
								Type: "function",
								Function: ToolCallFunction{
									Name:      evt.ContentBlock.Name,
									Arguments: "",
								},
							},
						},
					},
				},
			},
		}
		t.toolCallIndex = evt.Index
		return []*StreamChunk{sc}, nil
	}

	return nil, nil
}

func (t *AnthropicStreamTranslator) handleContentBlockDelta(data string) ([]*StreamChunk, error) {
	var evt struct {
		Index int `json:"index"`
		Delta struct {
			Type           string `json:"type"`
			Text           string `json:"text"`
			PartialJSON    string `json:"partial_json"`
		} `json:"delta"`
	}
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return nil, fmt.Errorf("parse content_block_delta: %w", err)
	}

	switch evt.Delta.Type {
	case "text_delta":
		return []*StreamChunk{
			{
				ID:      t.messageID,
				Object:  "chat.completion.chunk",
				Created: t.created,
				Model:   t.model,
				Choices: []Choice{
					{
						Index: 0,
						Delta: &Message{Content: evt.Delta.Text},
					},
				},
			},
		}, nil

	case "input_json_delta":
		return []*StreamChunk{
			{
				ID:      t.messageID,
				Object:  "chat.completion.chunk",
				Created: t.created,
				Model:   t.model,
				Choices: []Choice{
					{
						Index: 0,
						Delta: &Message{
							ToolCalls: []ToolCall{
								{
									Function: ToolCallFunction{
										Arguments: evt.Delta.PartialJSON,
									},
								},
							},
						},
					},
				},
			},
		}, nil
	}

	return nil, nil
}

func (t *AnthropicStreamTranslator) handleMessageDelta(data string) ([]*StreamChunk, error) {
	var evt struct {
		Delta struct {
			StopReason string `json:"stop_reason"`
		} `json:"delta"`
		Usage struct {
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return nil, fmt.Errorf("parse message_delta: %w", err)
	}

	finishReason := mapStopReason(evt.Delta.StopReason)
	return []*StreamChunk{
		{
			ID:      t.messageID,
			Object:  "chat.completion.chunk",
			Created: t.created,
			Model:   t.model,
			Choices: []Choice{
				{
					Index:        0,
					Delta:        &Message{},
					FinishReason: &finishReason,
				},
			},
		},
	}, nil
}

// --- Rate Limits & Usage ---

func (a *AnthropicAdapter) ExtractRateLimitInfo(header http.Header) *RateLimitInfo {
	info := &RateLimitInfo{}
	hasData := false

	if v := header.Get("anthropic-ratelimit-requests-remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			info.RemainingRequests = n
			hasData = true
		}
	}
	if v := header.Get("anthropic-ratelimit-tokens-remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			info.RemainingTokens = n
			hasData = true
		}
	}
	if v := header.Get("anthropic-ratelimit-requests-limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			info.LimitRequests = n
			hasData = true
		}
	}
	if v := header.Get("anthropic-ratelimit-tokens-limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			info.LimitTokens = n
			hasData = true
		}
	}
	if v := header.Get("anthropic-ratelimit-requests-reset"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			info.ResetRequests = t
			hasData = true
		}
	}
	if v := header.Get("anthropic-ratelimit-tokens-reset"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			info.ResetTokens = t
			hasData = true
		}
	}

	if !hasData {
		return nil
	}
	return info
}

func (a *AnthropicAdapter) ExtractUsage(body []byte) *Usage {
	var resp struct {
		Usage anthropicUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	if resp.Usage.InputTokens == 0 && resp.Usage.OutputTokens == 0 {
		return nil
	}
	return &Usage{
		PromptTokens:     resp.Usage.InputTokens,
		CompletionTokens: resp.Usage.OutputTokens,
		TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
	}
}
