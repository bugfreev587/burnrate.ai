package pricing

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

func decFromStr(s string) decimal.Decimal {
	return decimal.RequireFromString(s)
}

func TestIsDuplicateKeyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"duplicate key", errors.New("ERROR: duplicate key value violates unique constraint"), true},
		{"unique constraint", errors.New("unique constraint violation on column X"), true},
		{"postgres code 23505", errors.New("pq: error code 23505"), true},
		{"unrelated error", errors.New("connection refused"), false},
		{"empty error", errors.New(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDuplicateKeyError(tt.err); got != tt.want {
				t.Errorf("isDuplicateKeyError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		s, substr string
		want      bool
	}{
		{"hello world", "world", true},
		{"hello world", "hello", true},
		{"hello", "hello", true},
		{"hello", "hello world", false},
		{"", "", true},
		{"a", "", true},
		{"", "a", false},
		{"abc", "bc", true},
		{"abc", "cd", false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q_%q", tt.s, tt.substr), func(t *testing.T) {
			if got := contains(tt.s, tt.substr); got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

func TestBudgetRedisKey(t *testing.T) {
	e := &PricingEngine{}
	ts := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name       string
		periodType string
		want       string
	}{
		{"monthly", models.PeriodMonthly, "budget:42:2025-06"},
		{"weekly", models.PeriodWeekly, "budget:42:w2025-24"},
		{"daily", models.PeriodDaily, "budget:42:2025-06-15"},
		{"unknown defaults", "biweekly", "budget:42:biweekly:2025-06"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.budgetRedisKey(42, tt.periodType, ts)
			if got != tt.want {
				t.Errorf("budgetRedisKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBudgetRedisKeyForKey(t *testing.T) {
	e := &PricingEngine{}
	ts := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name       string
		periodType string
		want       string
	}{
		{"monthly", models.PeriodMonthly, "budget:key:tg_abc:2025-06"},
		{"weekly", models.PeriodWeekly, "budget:key:tg_abc:w2025-24"},
		{"daily", models.PeriodDaily, "budget:key:tg_abc:2025-06-15"},
		{"unknown", "biweekly", "budget:key:tg_abc:biweekly:2025-06"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.budgetRedisKeyForKey("tg_abc", tt.periodType, ts)
			if got != tt.want {
				t.Errorf("budgetRedisKeyForKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBudgetRedisKeyForProvider(t *testing.T) {
	e := &PricingEngine{}
	ts := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name       string
		periodType string
		want       string
	}{
		{"monthly", models.PeriodMonthly, "budget:42:anthropic:2025-06"},
		{"weekly", models.PeriodWeekly, "budget:42:anthropic:w2025-24"},
		{"daily", models.PeriodDaily, "budget:42:anthropic:2025-06-15"},
		{"unknown", "biweekly", "budget:42:anthropic:biweekly:2025-06"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.budgetRedisKeyForProvider(42, "anthropic", tt.periodType, ts)
			if got != tt.want {
				t.Errorf("budgetRedisKeyForProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBudgetRedisKeyForKeyProvider(t *testing.T) {
	e := &PricingEngine{}
	ts := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name       string
		periodType string
		want       string
	}{
		{"monthly", models.PeriodMonthly, "budget:key:tg_abc:anthropic:2025-06"},
		{"weekly", models.PeriodWeekly, "budget:key:tg_abc:anthropic:w2025-24"},
		{"daily", models.PeriodDaily, "budget:key:tg_abc:anthropic:2025-06-15"},
		{"unknown", "biweekly", "budget:key:tg_abc:anthropic:biweekly:2025-06"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.budgetRedisKeyForKeyProvider("tg_abc", "anthropic", tt.periodType, ts)
			if got != tt.want {
				t.Errorf("budgetRedisKeyForKeyProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReservationRedisKey(t *testing.T) {
	e := &PricingEngine{}
	ts := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name       string
		periodType string
		want       string
	}{
		{"monthly", models.PeriodMonthly, "budget:reserved:42:2025-06"},
		{"weekly", models.PeriodWeekly, "budget:reserved:42:w2025-24"},
		{"daily", models.PeriodDaily, "budget:reserved:42:2025-06-15"},
		{"unknown", "biweekly", "budget:reserved:42:biweekly:2025-06"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.reservationRedisKey(42, tt.periodType, ts)
			if got != tt.want {
				t.Errorf("reservationRedisKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReservationRedisKeyForKey(t *testing.T) {
	e := &PricingEngine{}
	ts := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name       string
		periodType string
		want       string
	}{
		{"monthly", models.PeriodMonthly, "budget:reserved:key:tg_abc:2025-06"},
		{"weekly", models.PeriodWeekly, "budget:reserved:key:tg_abc:w2025-24"},
		{"daily", models.PeriodDaily, "budget:reserved:key:tg_abc:2025-06-15"},
		{"unknown", "biweekly", "budget:reserved:key:tg_abc:biweekly:2025-06"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.reservationRedisKeyForKey("tg_abc", tt.periodType, ts)
			if got != tt.want {
				t.Errorf("reservationRedisKeyForKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrBudgetExceeded(t *testing.T) {
	err := &ErrBudgetExceeded{
		TenantID:     42,
		LimitAmount:  decFromStr("100.00"),
		CurrentSpend: decFromStr("105.50"),
		Period:       "monthly",
	}
	want := "budget exceeded for tenant 42: period=monthly limit=100 current=105.5"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrModelNotFound(t *testing.T) {
	err := &ErrModelNotFound{Provider: "anthropic", Model: "claude-3-opus"}
	want := "model not found in catalog: provider=anthropic model=claude-3-opus"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
