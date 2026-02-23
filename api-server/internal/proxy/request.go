package proxy

import "encoding/json"

// RequestMeta holds model and max_tokens extracted from the request body.
type RequestMeta struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
}

// parseRequestMeta extracts model and max_tokens from a JSON request body.
// Returns zero values on parse failure (never errors — we don't want to block).
func parseRequestMeta(body []byte) RequestMeta {
	var meta RequestMeta
	_ = json.Unmarshal(body, &meta)
	return meta
}
