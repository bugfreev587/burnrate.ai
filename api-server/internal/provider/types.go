package provider

import (
	"context"
	"net/http"
	"time"
)

// --- OpenAI-compatible request/response types ---

// ChatCompletionRequest represents an OpenAI-compatible chat completion request.
type ChatCompletionRequest struct {
	Model            string         `json:"model"`
	Messages         []Message      `json:"messages"`
	Tools            []Tool         `json:"tools,omitempty"`
	ToolChoice       any            `json:"tool_choice,omitempty"` // string or object
	Stream           bool           `json:"stream,omitempty"`
	Temperature      *float64       `json:"temperature,omitempty"`
	TopP             *float64       `json:"top_p,omitempty"`
	MaxTokens        *int           `json:"max_tokens,omitempty"`
	Stop             any            `json:"stop,omitempty"` // string or []string
	FrequencyPenalty *float64       `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64       `json:"presence_penalty,omitempty"`
	Seed             *int           `json:"seed,omitempty"`
	N                *int           `json:"n,omitempty"`
	User             string         `json:"user,omitempty"`
	ResponseFormat   map[string]any `json:"response_format,omitempty"`
}

// Message represents a chat message.
type Message struct {
	Role       string        `json:"role"`
	Content    any           `json:"content"` // string or []ContentPart
	Name       string        `json:"name,omitempty"`
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

// ContentPart represents a multimodal content part.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL represents an image URL reference.
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// Tool represents a tool definition.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a function tool.
type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ToolCall represents a tool call in a response.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction describes the function being called.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatCompletionResponse represents an OpenAI-compatible chat completion response.
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

// Choice represents a single completion choice.
type Choice struct {
	Index        int      `json:"index"`
	Message      *Message `json:"message,omitempty"`
	Delta        *Message `json:"delta,omitempty"`
	FinishReason *string  `json:"finish_reason,omitempty"`
}

// StreamChunk represents an OpenAI SSE streaming chunk.
type StreamChunk struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

// Usage represents token usage statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// RateLimitInfo represents rate limit information from provider response headers.
type RateLimitInfo struct {
	RemainingRequests int       `json:"remaining_requests,omitempty"`
	RemainingTokens   int       `json:"remaining_tokens,omitempty"`
	LimitRequests     int       `json:"limit_requests,omitempty"`
	LimitTokens       int       `json:"limit_tokens,omitempty"`
	ResetRequests     time.Time `json:"reset_requests,omitempty"`
	ResetTokens       time.Time `json:"reset_tokens,omitempty"`
}

// --- Provider Adapter Interface ---

// ProviderAdapter defines the contract for translating between OpenAI-compatible
// format and a specific provider's API format.
type ProviderAdapter interface {
	// Name returns the provider identifier (e.g., "openai", "anthropic").
	Name() string

	// TransformRequest converts an OpenAI-compatible request into a provider-specific HTTP request.
	TransformRequest(ctx context.Context, req *ChatCompletionRequest, apiKey string) (*http.Request, error)

	// TransformResponse converts a provider-specific HTTP response into an OpenAI-compatible response.
	TransformResponse(resp *http.Response) (*ChatCompletionResponse, error)

	// TransformStreamChunk converts a provider-specific SSE chunk into OpenAI-compatible chunks.
	// Returns a slice because one provider event may map to multiple OpenAI chunks.
	TransformStreamChunk(chunk []byte) ([]*StreamChunk, error)

	// ExtractRateLimitInfo extracts rate limit info from response headers.
	ExtractRateLimitInfo(header http.Header) *RateLimitInfo

	// ExtractUsage extracts token usage from a response body.
	ExtractUsage(body []byte) *Usage
}

// --- Routing Types ---

// ModelGroup defines a group of deployments that serve the same logical model.
type ModelGroup struct {
	Name        string       `json:"name"`
	Strategy    string       `json:"strategy"` // "fallback", "round-robin", "lowest-latency", "cost-optimized"
	Deployments []Deployment `json:"deployments"`
}

// Deployment represents a single provider deployment within a model group.
type Deployment struct {
	ID              string  `json:"id"`
	Provider        string  `json:"provider"` // "openai", "anthropic"
	Model           string  `json:"model"`    // actual model name at the provider
	APIKey          string  `json:"api_key"`
	Priority        int     `json:"priority"`          // for fallback ordering (lower = higher priority)
	Weight          int     `json:"weight"`            // for weighted round-robin
	CostPer1KInput  float64 `json:"cost_per_1k_input"`
	CostPer1KOutput float64 `json:"cost_per_1k_output"`
}

// GetContentText extracts text content from a Message, handling both string and []ContentPart formats.
func (m *Message) GetContentText() string {
	if m.Content == nil {
		return ""
	}
	switch v := m.Content.(type) {
	case string:
		return v
	default:
		return ""
	}
}
