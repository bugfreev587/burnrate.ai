package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const deepseekBaseURL = "https://api.deepseek.com/v1/chat/completions"

// DeepSeekAdapter implements ProviderAdapter for the DeepSeek API.
// DeepSeek is highly OpenAI-compatible, so this is mostly pass-through.
// The key difference is handling the `reasoning_content` field returned
// by deepseek-reasoner models.
type DeepSeekAdapter struct{}

func NewDeepSeekAdapter() *DeepSeekAdapter {
	return &DeepSeekAdapter{}
}

func (a *DeepSeekAdapter) Name() string {
	return "deepseek"
}

func (a *DeepSeekAdapter) TransformRequest(ctx context.Context, req *ChatCompletionRequest, apiKey string) (*http.Request, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, deepseekBaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	return httpReq, nil
}

// deepseekResponse extends OpenAI format with reasoning_content.
type deepseekResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []deepseekChoice `json:"choices"`
	Usage   *Usage           `json:"usage"`
}

type deepseekChoice struct {
	Index        int             `json:"index"`
	Message      deepseekMessage `json:"message"`
	FinishReason *string         `json:"finish_reason"`
}

type deepseekMessage struct {
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}

func (a *DeepSeekAdapter) TransformResponse(resp *http.Response) (*ChatCompletionResponse, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepseek error (status %d): %s", resp.StatusCode, string(body))
	}

	var dsResp deepseekResponse
	if err := json.Unmarshal(body, &dsResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return a.convertResponse(&dsResp), nil
}

func (a *DeepSeekAdapter) convertResponse(dsResp *deepseekResponse) *ChatCompletionResponse {
	choices := make([]Choice, len(dsResp.Choices))
	for i, c := range dsResp.Choices {
		content := c.Message.Content
		// If there's reasoning_content, prepend it as a <think> block
		// so downstream consumers can see the chain-of-thought.
		if c.Message.ReasoningContent != "" {
			content = "<think>\n" + c.Message.ReasoningContent + "\n</think>\n" + content
		}

		msg := &Message{
			Role:      c.Message.Role,
			Content:   content,
			ToolCalls: c.Message.ToolCalls,
		}
		choices[i] = Choice{
			Index:        c.Index,
			Message:      msg,
			FinishReason: c.FinishReason,
		}
	}

	return &ChatCompletionResponse{
		ID:      dsResp.ID,
		Object:  dsResp.Object,
		Created: dsResp.Created,
		Model:   dsResp.Model,
		Choices: choices,
		Usage:   dsResp.Usage,
	}
}

// deepseekStreamChunk extends OpenAI chunk with reasoning_content in delta.
type deepseekStreamChunk struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []deepseekChunkChoice `json:"choices"`
	Usage   *Usage               `json:"usage,omitempty"`
}

type deepseekChunkChoice struct {
	Index        int                `json:"index"`
	Delta        deepseekChunkDelta `json:"delta"`
	FinishReason *string            `json:"finish_reason"`
}

type deepseekChunkDelta struct {
	Role             string     `json:"role,omitempty"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}

func (a *DeepSeekAdapter) TransformStreamChunk(chunk []byte) ([]*StreamChunk, error) {
	line := strings.TrimSpace(string(chunk))

	if line == "" || strings.HasPrefix(line, ":") {
		return nil, nil
	}

	if !strings.HasPrefix(line, "data: ") {
		return nil, nil
	}

	data := strings.TrimPrefix(line, "data: ")
	if data == "[DONE]" {
		return nil, io.EOF
	}

	var dsChunk deepseekStreamChunk
	if err := json.Unmarshal([]byte(data), &dsChunk); err != nil {
		return nil, fmt.Errorf("unmarshal chunk: %w", err)
	}

	// Convert to standard OpenAI StreamChunk, merging reasoning_content into content
	choices := make([]Choice, len(dsChunk.Choices))
	for i, c := range dsChunk.Choices {
		content := c.Delta.Content
		if c.Delta.ReasoningContent != "" {
			// Send reasoning content as regular content for transparent forwarding
			content = c.Delta.ReasoningContent
		}

		delta := &Message{
			Role:      c.Delta.Role,
			ToolCalls: c.Delta.ToolCalls,
		}
		if content != "" {
			delta.Content = content
		}

		choices[i] = Choice{
			Index:        c.Index,
			Delta:        delta,
			FinishReason: c.FinishReason,
		}
	}

	return []*StreamChunk{
		{
			ID:      dsChunk.ID,
			Object:  dsChunk.Object,
			Created: dsChunk.Created,
			Model:   dsChunk.Model,
			Choices: choices,
			Usage:   dsChunk.Usage,
		},
	}, nil
}

func (a *DeepSeekAdapter) ExtractRateLimitInfo(header http.Header) *RateLimitInfo {
	// DeepSeek uses the same x-ratelimit-* headers as OpenAI
	oai := &OpenAIAdapter{}
	return oai.ExtractRateLimitInfo(header)
}

func (a *DeepSeekAdapter) ExtractUsage(body []byte) *Usage {
	var resp struct {
		Usage *Usage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Usage == nil {
		return nil
	}
	return resp.Usage
}
