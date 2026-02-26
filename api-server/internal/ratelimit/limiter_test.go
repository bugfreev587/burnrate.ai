package ratelimit

import (
	"testing"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

func TestCounterKey(t *testing.T) {
	l := &Limiter{}

	tests := []struct {
		name     string
		tenantID uint
		limit    models.RateLimit
		windowID int64
		want     string
	}{
		{
			"account-level RPM",
			42,
			models.RateLimit{
				Provider:  "anthropic",
				Model:     "claude-3-opus",
				ScopeType: "account",
				ScopeID:   "",
				Metric:    models.RateLimitMetricRPM,
			},
			12345,
			"rl:42:anthropic:claude-3-opus:account::rpm:12345",
		},
		{
			"api-key scoped ITPM",
			10,
			models.RateLimit{
				Provider:  "",
				Model:     "",
				ScopeType: "api_key",
				ScopeID:   "tg_abc",
				Metric:    models.RateLimitMetricITPM,
			},
			99,
			"rl:10:::api_key:tg_abc:itpm:99",
		},
		{
			"all-model OTPM",
			1,
			models.RateLimit{
				Provider:  "openai",
				Model:     "",
				ScopeType: "account",
				ScopeID:   "",
				Metric:    models.RateLimitMetricOTPM,
			},
			0,
			"rl:1:openai::account::otpm:0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := l.counterKey(tt.tenantID, tt.limit, tt.windowID)
			if got != tt.want {
				t.Errorf("counterKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRateLimitResult_Struct(t *testing.T) {
	r := RateLimitResult{
		Exceeded:     true,
		Metric:       "rpm",
		Limit:        100,
		Used:         95,
		RetryAfterMs: 5000,
	}
	if !r.Exceeded {
		t.Error("expected Exceeded=true")
	}
	if r.RetryAfterMs != 5000 {
		t.Errorf("RetryAfterMs = %d, want 5000", r.RetryAfterMs)
	}
}
