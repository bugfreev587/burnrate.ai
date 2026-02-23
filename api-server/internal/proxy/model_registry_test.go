package proxy

import "testing"

func TestResolveProviderFromModel(t *testing.T) {
	tests := []struct {
		model    string
		want     Provider
		wantOK   bool
	}{
		// OpenAI models
		{"gpt-4o", ProviderOpenAI, true},
		{"gpt-4o-mini", ProviderOpenAI, true},
		{"gpt-3.5-turbo", ProviderOpenAI, true},
		{"o1-preview", ProviderOpenAI, true},
		{"o1-mini", ProviderOpenAI, true},
		{"o3-mini", ProviderOpenAI, true},
		{"o4-mini", ProviderOpenAI, true},
		{"chatgpt-4o-latest", ProviderOpenAI, true},

		// Case insensitive
		{"GPT-4o", ProviderOpenAI, true},
		{"Claude-sonnet-4-6", ProviderAnthropic, true},

		// Anthropic models
		{"claude-sonnet-4-6", ProviderAnthropic, true},
		{"claude-opus-4-6", ProviderAnthropic, true},
		{"claude-haiku-4-5-20251001", ProviderAnthropic, true},
		{"claude-3-5-sonnet-20241022", ProviderAnthropic, true},

		// Gemini models
		{"gemini-1.5-pro", ProviderGemini, true},
		{"gemini-2.0-flash", ProviderGemini, true},

		// Unknown models
		{"unknown-model", "", false},
		{"mistral-large", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got, ok := ResolveProviderFromModel(tt.model)
			if ok != tt.wantOK {
				t.Errorf("ResolveProviderFromModel(%q) ok = %v, want %v", tt.model, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("ResolveProviderFromModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}
