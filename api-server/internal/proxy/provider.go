package proxy

import (
	"net/http"
	"strings"
)

// Provider represents an upstream AI provider.
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderGemini    Provider = "gemini"
	ProviderBedrock   Provider = "bedrock"
	ProviderVertex    Provider = "vertex"
)

var defaultUpstreamURLs = map[Provider]string{
	ProviderAnthropic: "https://api.anthropic.com",
	ProviderOpenAI:    "https://api.openai.com",
	ProviderGemini:    "https://generativelanguage.googleapis.com",
	ProviderBedrock:   "https://bedrock-runtime.us-east-1.amazonaws.com",
	ProviderVertex:    "https://us-central1-aiplatform.googleapis.com",
}

// tokengateHeaders lists headers that must never be forwarded upstream.
var tokengateHeaders = []string{
	"X-Tokengate-Key",
	"X-Tokengate-Provider",
	"X-Tokengate-User",
	"X-Tokengate-Project",
	"X-Tokengate-Session",
}

// resolveProvider determines the upstream provider from the header value or request path.
func resolveProvider(headerVal, path string) Provider {
	if headerVal != "" {
		return Provider(strings.ToLower(headerVal))
	}
	switch {
	case strings.HasPrefix(path, "/v1/openai/"):
		return ProviderOpenAI
	case strings.HasPrefix(path, "/v1/gemini/"):
		return ProviderGemini
	case strings.HasPrefix(path, "/v1/bedrock/"):
		return ProviderBedrock
	case strings.HasPrefix(path, "/v1/vertex/"):
		return ProviderVertex
	default:
		return ProviderAnthropic
	}
}

// upstreamBase returns the base URL for the given provider.
func upstreamBase(p Provider) string {
	if url, ok := defaultUpstreamURLs[p]; ok {
		return url
	}
	return defaultUpstreamURLs[ProviderAnthropic]
}

// upstreamPath strips the /v1/{provider} prefix for non-Anthropic providers.
// e.g. /v1/openai/chat/completions → /v1/chat/completions
func upstreamPath(p Provider, originalPath string) string {
	if p == ProviderAnthropic {
		return originalPath
	}
	prefix := "/v1/" + string(p)
	if strings.HasPrefix(originalPath, prefix) {
		return originalPath[len(prefix):]
	}
	return originalPath
}

// applyByokAuth sets the appropriate auth header for the provider using the BYOK key.
// For Anthropic, Authorization is removed and x-api-key is set.
func applyByokAuth(p Provider, key []byte, req *http.Request) {
	switch p {
	case ProviderAnthropic:
		req.Header.Del("Authorization")
		req.Header.Set("x-api-key", string(key))
	case ProviderOpenAI:
		req.Header.Set("Authorization", "Bearer "+string(key))
	case ProviderGemini:
		req.Header.Set("x-goog-api-key", string(key))
	case ProviderBedrock, ProviderVertex:
		// Full SigV4 / Google signing is future work; set the key as a stub.
		req.Header.Set("x-api-key", string(key))
	}
}

// stripTokengateHeaders removes all TokenGate-specific headers from the request
// so they are never forwarded upstream.
func stripTokengateHeaders(req *http.Request) {
	for _, h := range tokengateHeaders {
		req.Header.Del(h)
	}
}
