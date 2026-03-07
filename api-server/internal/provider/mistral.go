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

const mistralBaseURL = "https://api.mistral.ai/v1/chat/completions"

// MistralAdapter implements ProviderAdapter for the Mistral API.
// Mistral is highly OpenAI-compatible, but does not support
// frequency_penalty, presence_penalty, or certain tool_choice formats.
type MistralAdapter struct{}

func NewMistralAdapter() *MistralAdapter {
	return &MistralAdapter{}
}

func (a *MistralAdapter) Name() string {
	return "mistral"
}

// mistralRequest is the Mistral-specific request body.
// It omits fields that Mistral doesn't support.
type mistralRequest struct {
	Model          string         `json:"model"`
	Messages       []Message      `json:"messages"`
	Tools          []Tool         `json:"tools,omitempty"`
	ToolChoice     any            `json:"tool_choice,omitempty"`
	Stream         bool           `json:"stream,omitempty"`
	Temperature    *float64       `json:"temperature,omitempty"`
	TopP           *float64       `json:"top_p,omitempty"`
	MaxTokens      *int           `json:"max_tokens,omitempty"`
	Stop           any            `json:"stop,omitempty"`
	N              *int           `json:"n,omitempty"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
	// Mistral does NOT support: frequency_penalty, presence_penalty, seed, user
}

func (a *MistralAdapter) TransformRequest(ctx context.Context, req *ChatCompletionRequest, apiKey string) (*http.Request, error) {
	mReq := &mistralRequest{
		Model:          req.Model,
		Messages:       req.Messages,
		Tools:          req.Tools,
		ToolChoice:     a.convertToolChoice(req.ToolChoice),
		Stream:         req.Stream,
		Temperature:    req.Temperature,
		TopP:           req.TopP,
		MaxTokens:      req.MaxTokens,
		Stop:           req.Stop,
		N:              req.N,
		ResponseFormat: req.ResponseFormat,
	}

	body, err := json.Marshal(mReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, mistralBaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	return httpReq, nil
}

// convertToolChoice maps OpenAI tool_choice to Mistral format.
// Mistral supports "auto", "none", "any", and {"type":"function","function":{"name":"xxx"}}.
func (a *MistralAdapter) convertToolChoice(tc any) any {
	if tc == nil {
		return nil
	}

	switch v := tc.(type) {
	case string:
		switch v {
		case "auto", "none":
			return v
		case "required":
			return "any" // Mistral uses "any" instead of "required"
		default:
			return "auto"
		}
	case map[string]any:
		// Pass through object form (Mistral supports same object format)
		return v
	}
	return nil
}

func (a *MistralAdapter) TransformResponse(resp *http.Response) (*ChatCompletionResponse, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mistral error (status %d): %s", resp.StatusCode, string(body))
	}

	var result ChatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &result, nil
}

func (a *MistralAdapter) TransformStreamChunk(chunk []byte) ([]*StreamChunk, error) {
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

	var sc StreamChunk
	if err := json.Unmarshal([]byte(data), &sc); err != nil {
		return nil, fmt.Errorf("unmarshal chunk: %w", err)
	}
	return []*StreamChunk{&sc}, nil
}

func (a *MistralAdapter) ExtractRateLimitInfo(header http.Header) *RateLimitInfo {
	// Mistral uses the same x-ratelimit-* headers as OpenAI
	oai := &OpenAIAdapter{}
	return oai.ExtractRateLimitInfo(header)
}

func (a *MistralAdapter) ExtractUsage(body []byte) *Usage {
	var resp struct {
		Usage *Usage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Usage == nil {
		return nil
	}
	return resp.Usage
}
