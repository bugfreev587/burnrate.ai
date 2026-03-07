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

const openAIBaseURL = "https://api.openai.com/v1/chat/completions"

// OpenAIAdapter implements ProviderAdapter for OpenAI.
// Since OpenAI is the canonical format, most operations are pass-through.
type OpenAIAdapter struct {
	Organization string // optional Organization header
	baseURL      string // overridable for testing; defaults to openAIBaseURL
}

func NewOpenAIAdapter() *OpenAIAdapter {
	return &OpenAIAdapter{baseURL: openAIBaseURL}
}

func (a *OpenAIAdapter) Name() string {
	return "openai"
}

func (a *OpenAIAdapter) TransformRequest(ctx context.Context, req *ChatCompletionRequest, apiKey string) (*http.Request, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := a.baseURL
	if url == "" {
		url = openAIBaseURL
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	if a.Organization != "" {
		httpReq.Header.Set("OpenAI-Organization", a.Organization)
	}

	return httpReq, nil
}

func (a *OpenAIAdapter) TransformResponse(resp *http.Response) (*ChatCompletionResponse, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai error (status %d): %s", resp.StatusCode, string(body))
	}

	var result ChatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &result, nil
}

func (a *OpenAIAdapter) TransformStreamChunk(chunk []byte) ([]*StreamChunk, error) {
	line := strings.TrimSpace(string(chunk))

	if line == "" || strings.HasPrefix(line, ":") {
		return nil, nil // comment or empty line
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

func (a *OpenAIAdapter) ExtractRateLimitInfo(header http.Header) *RateLimitInfo {
	info := &RateLimitInfo{}
	hasData := false

	if v := header.Get("x-ratelimit-remaining-requests"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			info.RemainingRequests = n
			hasData = true
		}
	}
	if v := header.Get("x-ratelimit-remaining-tokens"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			info.RemainingTokens = n
			hasData = true
		}
	}
	if v := header.Get("x-ratelimit-limit-requests"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			info.LimitRequests = n
			hasData = true
		}
	}
	if v := header.Get("x-ratelimit-limit-tokens"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			info.LimitTokens = n
			hasData = true
		}
	}
	if v := header.Get("x-ratelimit-reset-requests"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			info.ResetRequests = t
			hasData = true
		}
	}
	if v := header.Get("x-ratelimit-reset-tokens"); v != "" {
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

func (a *OpenAIAdapter) ExtractUsage(body []byte) *Usage {
	var resp struct {
		Usage *Usage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Usage == nil {
		return nil
	}
	return resp.Usage
}
