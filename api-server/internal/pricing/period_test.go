package pricing

import (
	"testing"
	"time"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

func TestPeriodStart(t *testing.T) {
	loc := time.UTC

	tests := []struct {
		name       string
		periodType string
		input      time.Time
		want       time.Time
	}{
		{
			name:       "monthly: mid-month → 1st of month midnight",
			periodType: models.PeriodMonthly,
			input:      time.Date(2025, 6, 15, 14, 30, 0, 0, loc),
			want:       time.Date(2025, 6, 1, 0, 0, 0, 0, loc),
		},
		{
			name:       "weekly: Wednesday → previous Monday",
			periodType: models.PeriodWeekly,
			input:      time.Date(2025, 6, 11, 10, 0, 0, 0, loc), // Wednesday
			want:       time.Date(2025, 6, 9, 0, 0, 0, 0, loc),   // Monday
		},
		{
			name:       "weekly: Sunday → previous Monday",
			periodType: models.PeriodWeekly,
			input:      time.Date(2025, 6, 15, 10, 0, 0, 0, loc), // Sunday
			want:       time.Date(2025, 6, 9, 0, 0, 0, 0, loc),   // Monday
		},
		{
			name:       "daily: mid-day → midnight of same day",
			periodType: models.PeriodDaily,
			input:      time.Date(2025, 6, 15, 18, 45, 30, 0, loc),
			want:       time.Date(2025, 6, 15, 0, 0, 0, 0, loc),
		},
		{
			name:       "unknown period type → defaults to monthly",
			periodType: "biweekly",
			input:      time.Date(2025, 6, 15, 14, 30, 0, 0, loc),
			want:       time.Date(2025, 6, 1, 0, 0, 0, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := periodStart(tt.periodType, tt.input)
			if !got.Equal(tt.want) {
				t.Errorf("periodStart(%q, %v) = %v, want %v", tt.periodType, tt.input, got, tt.want)
			}
		})
	}
}

func TestPeriodEnd(t *testing.T) {
	loc := time.UTC

	tests := []struct {
		name       string
		periodType string
		input      time.Time
		want       time.Time
	}{
		{
			name:       "monthly: any day → 1st of next month",
			periodType: models.PeriodMonthly,
			input:      time.Date(2025, 6, 15, 14, 30, 0, 0, loc),
			want:       time.Date(2025, 7, 1, 0, 0, 0, 0, loc),
		},
		{
			name:       "monthly: December → January next year",
			periodType: models.PeriodMonthly,
			input:      time.Date(2025, 12, 25, 0, 0, 0, 0, loc),
			want:       time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
		},
		{
			name:       "weekly: any day → 7 days after period start",
			periodType: models.PeriodWeekly,
			input:      time.Date(2025, 6, 11, 10, 0, 0, 0, loc), // Wednesday
			want:       time.Date(2025, 6, 16, 0, 0, 0, 0, loc),  // Monday + 7
		},
		{
			name:       "daily: any day → midnight of next day",
			periodType: models.PeriodDaily,
			input:      time.Date(2025, 6, 15, 18, 45, 0, 0, loc),
			want:       time.Date(2025, 6, 16, 0, 0, 0, 0, loc),
		},
		{
			name:       "unknown period type → defaults to monthly",
			periodType: "biweekly",
			input:      time.Date(2025, 6, 15, 14, 30, 0, 0, loc),
			want:       time.Date(2025, 7, 1, 0, 0, 0, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := periodEnd(tt.periodType, tt.input)
			if !got.Equal(tt.want) {
				t.Errorf("periodEnd(%q, %v) = %v, want %v", tt.periodType, tt.input, got, tt.want)
			}
		})
	}
}
