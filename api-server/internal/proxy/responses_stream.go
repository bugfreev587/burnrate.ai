package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ParseOpenAIResponsesSSE reads SSE from OpenAI's /v1/responses endpoint,
// passes all events through to the client, and extracts token counts from
// the response.completed event's usage field.
func ParseOpenAIResponsesSSE(ctx context.Context, body io.Reader, w http.ResponseWriter) (TokenCounts, error) {
	flusher, canFlush := w.(http.Flusher)

	var counts TokenCounts
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var eventType string
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if _, err := io.WriteString(w, line+"\n"); err != nil {
			break
		}
		if canFlush {
			flusher.Flush()
		}

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			dataLines = nil
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			dataLines = append(dataLines, data)
		} else if line == "" {
			if len(dataLines) > 0 {
				joined := strings.Join(dataLines, "\n")
				parseOpenAIResponsesSSEEvent(eventType, joined, &counts)
			}
			eventType = ""
			dataLines = nil
		}
	}

	// Process any remaining event data (e.g. stream ended without a trailing blank line).
	if len(dataLines) > 0 {
		joined := strings.Join(dataLines, "\n")
		parseOpenAIResponsesSSEEvent(eventType, joined, &counts)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return counts, err
	}
	return counts, nil
}

// parseOpenAIResponsesSSEEvent extracts token counts from OpenAI Responses SSE events.
func parseOpenAIResponsesSSEEvent(eventType, data string, counts *TokenCounts) {
	if eventType != "response.completed" {
		return
	}
	var evt struct {
		Response struct {
			ID    string `json:"id"`
			Model string `json:"model"`
			Usage struct {
				InputTokens  int64 `json:"input_tokens"`
				OutputTokens int64 `json:"output_tokens"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(data), &evt); err == nil {
		counts.MessageID = evt.Response.ID
		counts.Model = evt.Response.Model
		counts.InputTokens = evt.Response.Usage.InputTokens
		counts.OutputTokens = evt.Response.Usage.OutputTokens
	}
}

// TranslateAnthropicSSEToResponses reads Anthropic SSE events and translates them
// into OpenAI Responses API SSE events, writing them to the client.
func TranslateAnthropicSSEToResponses(ctx context.Context, body io.Reader, w http.ResponseWriter) (TokenCounts, error) {
	flusher, canFlush := w.(http.Flusher)

	var counts TokenCounts
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var eventType string
	var dataLines []string

	// Track state for building Responses events.
	var responseID string
	var model string
	outputItemIndex := 0
	contentPartIndex := 0

	writeSSE := func(event, data string) {
		line := fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
		if _, err := io.WriteString(w, line); err != nil {
			return
		}
		if canFlush {
			flusher.Flush()
		}
	}

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			dataLines = nil
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			dataLines = append(dataLines, data)
		} else if line == "" {
			if len(dataLines) > 0 {
				joined := strings.Join(dataLines, "\n")
				translateAnthropicSSEEvent(eventType, joined, &counts, &responseID, &model, &outputItemIndex, &contentPartIndex, writeSSE)
			}
			eventType = ""
			dataLines = nil
		}
	}

	// Process any remaining event data (e.g. stream ended without a trailing blank line).
	if len(dataLines) > 0 {
		joined := strings.Join(dataLines, "\n")
		translateAnthropicSSEEvent(eventType, joined, &counts, &responseID, &model, &outputItemIndex, &contentPartIndex, writeSSE)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return counts, err
	}
	return counts, nil
}

// translateAnthropicSSEEvent translates a single Anthropic SSE event into
// corresponding OpenAI Responses SSE events.
func translateAnthropicSSEEvent(
	eventType, data string,
	counts *TokenCounts,
	responseID, model *string,
	outputItemIndex, contentPartIndex *int,
	writeSSE func(string, string),
) {
	switch eventType {
	case "message_start":
		var evt struct {
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
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return
		}
		*responseID = "resp_" + evt.Message.ID
		*model = evt.Message.Model
		counts.MessageID = *responseID
		counts.Model = evt.Message.Model
		counts.InputTokens = evt.Message.Usage.InputTokens
		counts.CacheCreationTokens = evt.Message.Usage.CacheCreationTokens
		counts.CacheReadTokens = evt.Message.Usage.CacheReadTokens

		// Emit response.created
		createdJSON, _ := json.Marshal(map[string]interface{}{
			"type": "response.created",
			"response": map[string]interface{}{
				"id":     *responseID,
				"object": "response",
				"status": "in_progress",
				"model":  *model,
				"output": []interface{}{},
			},
		})
		writeSSE("response.created", string(createdJSON))

		// Emit response.in_progress
		inProgressJSON, _ := json.Marshal(map[string]interface{}{
			"type": "response.in_progress",
			"response": map[string]interface{}{
				"id":     *responseID,
				"object": "response",
				"status": "in_progress",
				"model":  *model,
				"output": []interface{}{},
			},
		})
		writeSSE("response.in_progress", string(inProgressJSON))

	case "content_block_start":
		// Emit output_item.added and content_part.added
		outputItem := map[string]interface{}{
			"type":    "message",
			"id":      fmt.Sprintf("msg_%d", *outputItemIndex),
			"status":  "in_progress",
			"role":    "assistant",
			"content": []interface{}{},
		}
		addedJSON, _ := json.Marshal(map[string]interface{}{
			"type":         "response.output_item.added",
			"output_index": *outputItemIndex,
			"item":         outputItem,
		})
		writeSSE("response.output_item.added", string(addedJSON))

		contentPart := map[string]interface{}{
			"type": "output_text",
			"text": "",
		}
		partAddedJSON, _ := json.Marshal(map[string]interface{}{
			"type":          "response.content_part.added",
			"output_index":  *outputItemIndex,
			"content_index": *contentPartIndex,
			"part":          contentPart,
		})
		writeSSE("response.content_part.added", string(partAddedJSON))

	case "content_block_delta":
		var evt struct {
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return
		}
		if evt.Delta.Type == "text_delta" && evt.Delta.Text != "" {
			deltaJSON, _ := json.Marshal(map[string]interface{}{
				"type":          "response.output_text.delta",
				"output_index":  *outputItemIndex,
				"content_index": *contentPartIndex,
				"delta":         evt.Delta.Text,
			})
			writeSSE("response.output_text.delta", string(deltaJSON))
		}

	case "content_block_stop":
		// Emit output_text.done and content_part.done
		doneJSON, _ := json.Marshal(map[string]interface{}{
			"type":          "response.output_text.done",
			"output_index":  *outputItemIndex,
			"content_index": *contentPartIndex,
			"text":          "",
		})
		writeSSE("response.output_text.done", string(doneJSON))

		partDoneJSON, _ := json.Marshal(map[string]interface{}{
			"type":          "response.content_part.done",
			"output_index":  *outputItemIndex,
			"content_index": *contentPartIndex,
			"part": map[string]interface{}{
				"type": "output_text",
				"text": "",
			},
		})
		writeSSE("response.content_part.done", string(partDoneJSON))

		*contentPartIndex++

	case "message_delta":
		var evt struct {
			Usage struct {
				OutputTokens int64 `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err == nil {
			counts.OutputTokens = evt.Usage.OutputTokens
		}

	case "message_stop":
		// Emit output_item.done
		itemDoneJSON, _ := json.Marshal(map[string]interface{}{
			"type":         "response.output_item.done",
			"output_index": *outputItemIndex,
			"item": map[string]interface{}{
				"type":    "message",
				"id":      fmt.Sprintf("msg_%d", *outputItemIndex),
				"status":  "completed",
				"role":    "assistant",
				"content": []interface{}{},
			},
		})
		writeSSE("response.output_item.done", string(itemDoneJSON))

		*outputItemIndex++
		*contentPartIndex = 0

		// Emit response.completed with usage
		totalTokens := counts.InputTokens + counts.OutputTokens
		completedJSON, _ := json.Marshal(map[string]interface{}{
			"type": "response.completed",
			"response": map[string]interface{}{
				"id":     *responseID,
				"object": "response",
				"status": "completed",
				"model":  *model,
				"usage": map[string]interface{}{
					"input_tokens":  counts.InputTokens,
					"output_tokens": counts.OutputTokens,
					"total_tokens":  totalTokens,
				},
			},
		})
		writeSSE("response.completed", string(completedJSON))
	}
}
