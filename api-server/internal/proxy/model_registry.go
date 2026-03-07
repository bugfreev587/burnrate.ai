package proxy

import "strings"

// modelPrefixEntry maps a model name prefix to a provider.
type modelPrefixEntry struct {
	Prefix   string
	Provider Provider
}

// modelPrefixToProvider is an ordered list of prefix→provider mappings.
// First match wins; entries are checked in order.
var modelPrefixToProvider = []modelPrefixEntry{
	{"gpt-", ProviderOpenAI},
	{"o1", ProviderOpenAI},
	{"o3", ProviderOpenAI},
	{"o4", ProviderOpenAI},
	{"chatgpt-", ProviderOpenAI},
	{"claude-", ProviderAnthropic},
	{"gemini-", ProviderGemini},
	{"deepseek-", ProviderOpenAI},  // DeepSeek is OpenAI-compatible
	{"mistral-", ProviderOpenAI},   // Mistral is OpenAI-compatible
	{"codestral-", ProviderOpenAI}, // Mistral's Codestral
}

// ResolveProviderFromModel returns the provider for a given model name
// using case-insensitive prefix matching. Returns false if no match is found.
func ResolveProviderFromModel(model string) (Provider, bool) {
	lower := strings.ToLower(model)
	for _, entry := range modelPrefixToProvider {
		if strings.HasPrefix(lower, entry.Prefix) {
			return entry.Provider, true
		}
	}
	return "", false
}
