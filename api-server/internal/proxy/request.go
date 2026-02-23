package proxy

import "encoding/json"

// RequestMeta holds model and max_tokens extracted from the request body.
type RequestMeta struct {
	Model           string `json:"model"`
	MaxTokens       int    `json:"max_tokens"`
	MaxOutputTokens int    `json:"max_output_tokens"`
}

// parseRequestMeta extracts model and max_tokens from a JSON request body.
// Returns zero values on parse failure (never errors — we don't want to block).
// When max_tokens is 0 but max_output_tokens is set, copies max_output_tokens into MaxTokens.
func parseRequestMeta(body []byte) RequestMeta {
	var meta RequestMeta
	_ = json.Unmarshal(body, &meta)
	if meta.MaxTokens == 0 && meta.MaxOutputTokens > 0 {
		meta.MaxTokens = meta.MaxOutputTokens
	}
	return meta
}
