package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// TokenCounts holds the token usage extracted from the upstream SSE stream or JSON response.
type TokenCounts struct {
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	MessageID           string
	Model               string
	// OutputTextBytes is the sum of bytes from output_text.delta events during SSE
	// streaming. Used to estimate OutputTokens when the upstream doesn't report usage.
	OutputTextBytes int64
}

// ParseSSE reads SSE bytes from Anthropic, writes them through to the client (w),
// and returns extracted token counts after the stream ends.
//
// It parses:
//   - message_start: input_tokens, cache_creation_tokens, cache_read_tokens, message.id, model
//   - message_delta: output_tokens from usage field
func ParseSSE(ctx context.Context, body io.Reader, w http.ResponseWriter) (TokenCounts, error) {
	flusher, canFlush := w.(http.Flusher)

	var counts TokenCounts
	scanner := bufio.NewScanner(body)

	// SSE lines can be long (especially for large tool results); set a 1MB buffer.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var eventType string
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		// Write raw line to client
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			// Client disconnected; drain but stop writing
			break
		}
		if canFlush {
			flusher.Flush()
		}

		// Parse SSE fields
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			dataLines = nil
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			dataLines = append(dataLines, data)
		} else if line == "" {
			// End of an SSE event block
			if len(dataLines) > 0 {
				joined := strings.Join(dataLines, "\n")
				parseSSEEvent(eventType, joined, &counts)
			}
			eventType = ""
			dataLines = nil
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return counts, err
	}

	return counts, nil
}

// parseSSEEvent extracts token counts from a single SSE event.
func parseSSEEvent(eventType, data string, counts *TokenCounts) {
	switch eventType {
	case "message_start":
		var evt struct {
			Type    string `json:"type"`
			Message struct {
				ID    string `json:"id"`
				Model string `json:"model"`
				Usage struct {
					InputTokens         int64 `json:"input_tokens"`
					CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
					CacheReadTokens     int64 `json:"cache_read_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err == nil {
			counts.MessageID = evt.Message.ID
			counts.Model = evt.Message.Model
			counts.InputTokens = evt.Message.Usage.InputTokens
			counts.CacheCreationTokens = evt.Message.Usage.CacheCreationTokens
			counts.CacheReadTokens = evt.Message.Usage.CacheReadTokens
		}

	case "message_delta":
		var evt struct {
			Type  string `json:"type"`
			Usage struct {
				OutputTokens int64 `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err == nil {
			counts.OutputTokens = evt.Usage.OutputTokens
		}
	}
}

// extractTokensFromJSON parses token usage from a non-streaming Anthropic response body.
func extractTokensFromJSON(body []byte) TokenCounts {
	var resp struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens         int64 `json:"input_tokens"`
			OutputTokens        int64 `json:"output_tokens"`
			CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return TokenCounts{}
	}
	return TokenCounts{
		MessageID:           resp.ID,
		Model:               resp.Model,
		InputTokens:         resp.Usage.InputTokens,
		OutputTokens:        resp.Usage.OutputTokens,
		CacheCreationTokens: resp.Usage.CacheCreationTokens,
		CacheReadTokens:     resp.Usage.CacheReadTokens,
	}
}

// extractTokensFromOpenAIResponsesJSON parses token usage from an OpenAI Responses API
// non-streaming JSON response body. It handles both Responses API field names
// (input_tokens/output_tokens) and Chat Completions field names
// (prompt_tokens/completion_tokens) for compatibility with the ChatGPT backend.
func extractTokensFromOpenAIResponsesJSON(body []byte) TokenCounts {
	var resp struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			// Responses API format
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
			// Chat Completions format (ChatGPT backend)
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return TokenCounts{}
	}
	inputTokens := resp.Usage.InputTokens
	outputTokens := resp.Usage.OutputTokens
	// Fall back to Chat Completions field names if Responses API fields are zero.
	if inputTokens == 0 && resp.Usage.PromptTokens != 0 {
		inputTokens = resp.Usage.PromptTokens
	}
	if outputTokens == 0 && resp.Usage.CompletionTokens != 0 {
		outputTokens = resp.Usage.CompletionTokens
	}
	return TokenCounts{
		MessageID:    resp.ID,
		Model:        resp.Model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}
}

// isStreamingRequest checks whether the request body asks for stream: true.
func isStreamingRequest(body []byte) bool {
	var req struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	return req.Stream
}

// newBodyReader returns a new io.Reader from a byte slice (used after peeking body).
func newBodyReader(b []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(b))
}
