package provider

import (
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxRetries != 3 {
		t.Errorf("expected 3 max retries, got %d", cfg.MaxRetries)
	}
	if cfg.ConnectTimeout != 5*time.Second {
		t.Errorf("expected 5s connect timeout, got %v", cfg.ConnectTimeout)
	}
	if cfg.ReadTimeout != 60*time.Second {
		t.Errorf("expected 60s read timeout, got %v", cfg.ReadTimeout)
	}
	if cfg.StreamFirstChunkTO != 30*time.Second {
		t.Errorf("expected 30s stream first chunk timeout, got %v", cfg.StreamFirstChunkTO)
	}
	if cfg.BaseBackoff != 1*time.Second {
		t.Errorf("expected 1s base backoff, got %v", cfg.BaseBackoff)
	}
	if cfg.MaxBackoff != 8*time.Second {
		t.Errorf("expected 8s max backoff, got %v", cfg.MaxBackoff)
	}
}

func TestRetryExecutor_ComputeBackoff(t *testing.T) {
	re := NewRetryExecutor(DefaultRetryConfig(), NewRegistry(), NewRouterState())

	tests := []struct {
		name       string
		attemptNum int
		attempt    Attempt
		wantMin    time.Duration
		wantMax    time.Duration
	}{
		{
			name:       "timeout - immediate retry",
			attemptNum: 0,
			attempt: Attempt{
				Error: &ProviderError{Type: ErrorTypeTimeout},
			},
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:       "429 with Retry-After",
			attemptNum: 0,
			attempt: Attempt{
				StatusCode: 429,
				Error: &ProviderError{
					Type:       ErrorTypeRateLimit,
					RetryAfter: 3,
				},
			},
			wantMin: 3 * time.Second,
			wantMax: 3 * time.Second,
		},
		{
			name:       "429 without Retry-After",
			attemptNum: 0,
			attempt: Attempt{
				StatusCode: 429,
				Error:      &ProviderError{Type: ErrorTypeRateLimit},
			},
			wantMin: 1 * time.Second,
			wantMax: 1 * time.Second,
		},
		{
			name:       "500 first attempt - 1s backoff",
			attemptNum: 0,
			attempt: Attempt{
				StatusCode: 500,
				Error:      &ProviderError{Type: ErrorTypeServer},
			},
			wantMin: 1 * time.Second,
			wantMax: 1 * time.Second,
		},
		{
			name:       "500 second attempt - 2s backoff",
			attemptNum: 1,
			attempt: Attempt{
				StatusCode: 500,
				Error:      &ProviderError{Type: ErrorTypeServer},
			},
			wantMin: 2 * time.Second,
			wantMax: 2 * time.Second,
		},
		{
			name:       "500 third attempt - 4s backoff",
			attemptNum: 2,
			attempt: Attempt{
				StatusCode: 500,
				Error:      &ProviderError{Type: ErrorTypeServer},
			},
			wantMin: 4 * time.Second,
			wantMax: 4 * time.Second,
		},
		{
			name:       "500 fourth attempt - capped at 8s",
			attemptNum: 3,
			attempt: Attempt{
				StatusCode: 500,
				Error:      &ProviderError{Type: ErrorTypeServer},
			},
			wantMin: 8 * time.Second,
			wantMax: 8 * time.Second,
		},
		{
			name:       "429 with large Retry-After capped at MaxBackoff",
			attemptNum: 0,
			attempt: Attempt{
				StatusCode: 429,
				Error: &ProviderError{
					Type:       ErrorTypeRateLimit,
					RetryAfter: 120,
				},
			},
			wantMin: 8 * time.Second,
			wantMax: 8 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := re.computeBackoff(tt.attemptNum, &tt.attempt)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("expected backoff in [%v, %v], got %v", tt.wantMin, tt.wantMax, got)
			}
		})
	}
}

func TestRetryExecutor_ComputeBackoff_NoError(t *testing.T) {
	re := NewRetryExecutor(DefaultRetryConfig(), NewRegistry(), NewRouterState())

	got := re.computeBackoff(0, &Attempt{})
	if got != 0 {
		t.Errorf("expected 0 backoff for no error, got %v", got)
	}
}
