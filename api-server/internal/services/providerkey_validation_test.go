package services

import (
	"errors"
	"testing"
)

func TestValidateProviderKeyFormat(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		apiKey   string
		wantErr  bool
	}{
		// Anthropic
		{"anthropic key on anthropic", "anthropic", "sk-ant-api03-xxxxxxxxxxxx", false},
		{"openai key on anthropic", "anthropic", "sk-proj-xxxxxxxxxxxx", true},
		{"random key on anthropic", "anthropic", "AIzaSyxxxxxxxxxx", true},

		// OpenAI
		{"openai key on openai", "openai", "sk-proj-xxxxxxxxxxxx", false},
		{"bare sk- key on openai", "openai", "sk-xxxxxxxxxxxx", false},
		{"anthropic key on openai", "openai", "sk-ant-api03-xxxxxxxxxxxx", true},
		{"random key on openai", "openai", "AIzaSyxxxxxxxxxx", true},

		// Gemini
		{"gemini-looking key on gemini", "gemini", "AIzaSyxxxxxxxxxx", false},
		{"anthropic key on gemini", "gemini", "sk-ant-api03-xxxxxxxxxxxx", true},
		{"openai key on gemini", "gemini", "sk-proj-xxxxxxxxxxxx", true},

		// Bedrock
		{"bedrock access key on bedrock", "bedrock", "AKIAIOSFODNN7EXAMPLE", false},
		{"anthropic key on bedrock", "bedrock", "sk-ant-api03-xxxxxxxxxxxx", true},
		{"openai key on bedrock", "bedrock", "sk-proj-xxxxxxxxxxxx", true},

		// Vertex
		{"service account JSON on vertex", "vertex", `{"type":"service_account"}`, false},
		{"anthropic key on vertex", "vertex", "sk-ant-api03-xxxxxxxxxxxx", true},
		{"openai key on vertex", "vertex", "sk-proj-xxxxxxxxxxxx", true},

		// Unknown provider — always OK (forward-compatible)
		{"any key on unknown provider", "some-future-provider", "sk-ant-api03-xxxxxxxxxxxx", false},
		{"any key on empty provider", "", "sk-ant-api03-xxxxxxxxxxxx", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProviderKeyFormat(tt.provider, tt.apiKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProviderKeyFormat(%q, %q) error = %v, wantErr %v", tt.provider, tt.apiKey, err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidProviderKeyFormat) {
				t.Errorf("expected error to wrap ErrInvalidProviderKeyFormat, got: %v", err)
			}
		})
	}
}
