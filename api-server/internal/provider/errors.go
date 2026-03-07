package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ProviderError represents a unified error from any provider, compatible
// with the OpenAI error format.
type ProviderError struct {
	StatusCode int    `json:"-"`
	Message    string `json:"message"`
	Type       string `json:"type"`
	Code       any    `json:"code,omitempty"`      // int or string
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Retryable  bool   `json:"-"`
	RetryAfter int    `json:"-"` // seconds, from Retry-After header
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("%s: %s (status %d)", e.Provider, e.Message, e.StatusCode)
}

// ToJSON serializes the error into OpenAI-compatible JSON.
func (e *ProviderError) ToJSON() []byte {
	out := struct {
		Error *ProviderError `json:"error"`
	}{Error: e}
	b, _ := json.Marshal(out)
	return b
}

// ErrorType constants for unified error classification.
const (
	ErrorTypeInvalidRequest = "invalid_request_error"
	ErrorTypeAuthentication = "authentication_error"
	ErrorTypePermission     = "permission_error"
	ErrorTypeNotFound       = "not_found_error"
	ErrorTypeRateLimit      = "rate_limit_error"
	ErrorTypeServer         = "server_error"
	ErrorTypeOverloaded     = "overloaded_error"
	ErrorTypeTimeout        = "timeout_error"
)

// ClassifyHTTPStatus maps an HTTP status code to our unified error type
// and determines if the error is retryable.
func ClassifyHTTPStatus(status int) (errorType string, retryable bool) {
	switch {
	case status == http.StatusBadRequest:
		return ErrorTypeInvalidRequest, false
	case status == http.StatusUnauthorized:
		return ErrorTypeAuthentication, false
	case status == http.StatusForbidden:
		return ErrorTypePermission, false
	case status == http.StatusNotFound:
		return ErrorTypeNotFound, false
	case status == http.StatusTooManyRequests:
		return ErrorTypeRateLimit, true
	case status == http.StatusRequestEntityTooLarge:
		return ErrorTypeInvalidRequest, false
	case status == 529: // Anthropic overloaded
		return ErrorTypeOverloaded, true
	case status >= 500 && status < 600:
		return ErrorTypeServer, true
	default:
		if status >= 400 && status < 500 {
			return ErrorTypeInvalidRequest, false
		}
		return ErrorTypeServer, true
	}
}

// ParseProviderError parses a raw error body from a provider response
// into a unified ProviderError.
func ParseProviderError(status int, body []byte, providerName, model string, header http.Header) *ProviderError {
	errorType, retryable := ClassifyHTTPStatus(status)

	pe := &ProviderError{
		StatusCode: status,
		Type:       errorType,
		Code:       status,
		Provider:   providerName,
		Model:      model,
		Retryable:  retryable,
	}

	// Parse Retry-After header
	if ra := header.Get("Retry-After"); ra != "" {
		var seconds int
		if _, err := fmt.Sscanf(ra, "%d", &seconds); err == nil {
			pe.RetryAfter = seconds
		}
	}

	// Try to extract message from OpenAI-style error body
	var oaiErr struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &oaiErr) == nil && oaiErr.Error.Message != "" {
		pe.Message = oaiErr.Error.Message
		if oaiErr.Error.Type != "" {
			pe.Type = oaiErr.Error.Type
		}
		return pe
	}

	// Try Anthropic-style error body
	var antErr struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &antErr) == nil && antErr.Error.Message != "" {
		pe.Message = antErr.Error.Message
		return pe
	}

	// Fallback
	if len(body) > 0 && len(body) < 500 {
		pe.Message = string(body)
	} else {
		pe.Message = fmt.Sprintf("provider %s returned status %d", providerName, status)
	}
	return pe
}
