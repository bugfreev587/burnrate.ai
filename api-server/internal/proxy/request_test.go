package proxy

import "testing"

func TestParseRequestMeta(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantModel     string
		wantMaxTokens int
	}{
		{
			name:          "standard request",
			body:          `{"model":"claude-sonnet-4-6","max_tokens":1024}`,
			wantModel:     "claude-sonnet-4-6",
			wantMaxTokens: 1024,
		},
		{
			name:          "max_output_tokens fallback",
			body:          `{"model":"gpt-4o","max_output_tokens":2048}`,
			wantModel:     "gpt-4o",
			wantMaxTokens: 2048,
		},
		{
			name:          "max_tokens takes precedence",
			body:          `{"model":"gpt-4o","max_tokens":512,"max_output_tokens":2048}`,
			wantModel:     "gpt-4o",
			wantMaxTokens: 512,
		},
		{
			name:          "no tokens",
			body:          `{"model":"gpt-4o"}`,
			wantModel:     "gpt-4o",
			wantMaxTokens: 0,
		},
		{
			name:          "invalid JSON",
			body:          `not json`,
			wantModel:     "",
			wantMaxTokens: 0,
		},
		{
			name:          "empty body",
			body:          ``,
			wantModel:     "",
			wantMaxTokens: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := parseRequestMeta([]byte(tt.body))
			if meta.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", meta.Model, tt.wantModel)
			}
			if meta.MaxTokens != tt.wantMaxTokens {
				t.Errorf("MaxTokens = %d, want %d", meta.MaxTokens, tt.wantMaxTokens)
			}
		})
	}
}
