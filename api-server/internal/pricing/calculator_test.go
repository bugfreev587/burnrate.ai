package pricing

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

func TestCalculate(t *testing.T) {
	calc := NewCalculator()

	// Helper to build a resolvedPrice with default 1M unit size.
	rp := func(pricePerUnit string, unitSize int64) resolvedPrice {
		return resolvedPrice{
			PricePerUnit: decimal.RequireFromString(pricePerUnit),
			UnitSize:     unitSize,
			PricingID:    1,
			Source:       "standard",
		}
	}

	tests := []struct {
		name        string
		event       UsageEvent
		prices      map[string]resolvedPrice
		wantCost    string
		wantPoints  int // expected number of price point entries
	}{
		{
			name:  "input tokens only",
			event: UsageEvent{InputTokens: 1_000_000},
			prices: map[string]resolvedPrice{
				models.PriceTypeInput: rp("3.00", 1_000_000),
			},
			wantCost:   "3",
			wantPoints: 1,
		},
		{
			name:  "input + output tokens sum correctly",
			event: UsageEvent{InputTokens: 1_000_000, OutputTokens: 500_000},
			prices: map[string]resolvedPrice{
				models.PriceTypeInput:  rp("3.00", 1_000_000),
				models.PriceTypeOutput: rp("15.00", 1_000_000),
			},
			wantCost:   "10.5", // 3 + 7.5
			wantPoints: 2,
		},
		{
			name: "all five dimensions",
			event: UsageEvent{
				InputTokens:         1_000_000,
				OutputTokens:        1_000_000,
				CacheCreationTokens: 1_000_000,
				CacheReadTokens:     1_000_000,
				ReasoningTokens:     1_000_000,
			},
			prices: map[string]resolvedPrice{
				models.PriceTypeInput:         rp("1.00", 1_000_000),
				models.PriceTypeOutput:        rp("2.00", 1_000_000),
				models.PriceTypeCacheCreation: rp("3.00", 1_000_000),
				models.PriceTypeCacheRead:     rp("4.00", 1_000_000),
				models.PriceTypeReasoning:     rp("5.00", 1_000_000),
			},
			wantCost:   "15", // 1+2+3+4+5
			wantPoints: 5,
		},
		{
			name:  "zero tokens for a dimension — skipped",
			event: UsageEvent{InputTokens: 0, OutputTokens: 1_000_000},
			prices: map[string]resolvedPrice{
				models.PriceTypeInput:  rp("3.00", 1_000_000),
				models.PriceTypeOutput: rp("15.00", 1_000_000),
			},
			wantCost:   "15",
			wantPoints: 1, // only output
		},
		{
			name:  "missing price for a dimension — skipped",
			event: UsageEvent{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			prices: map[string]resolvedPrice{
				models.PriceTypeInput: rp("3.00", 1_000_000),
				// no output price
			},
			wantCost:   "3",
			wantPoints: 1,
		},
		{
			name:       "empty prices map — zero cost",
			event:      UsageEvent{InputTokens: 1_000_000},
			prices:     map[string]resolvedPrice{},
			wantCost:   "0",
			wantPoints: 0,
		},
		{
			name:  "zero tokens across all dimensions",
			event: UsageEvent{},
			prices: map[string]resolvedPrice{
				models.PriceTypeInput:  rp("3.00", 1_000_000),
				models.PriceTypeOutput: rp("15.00", 1_000_000),
			},
			wantCost:   "0",
			wantPoints: 0,
		},
		{
			name:  "custom unit size — correct division",
			event: UsageEvent{InputTokens: 500},
			prices: map[string]resolvedPrice{
				models.PriceTypeInput: rp("10.00", 1_000), // $10 per 1K tokens
			},
			wantCost:   "5", // 500/1000 * 10
			wantPoints: 1,
		},
		{
			name:  "unit size of zero — defaults to 1,000,000",
			event: UsageEvent{InputTokens: 1_000_000},
			prices: map[string]resolvedPrice{
				models.PriceTypeInput: rp("3.00", 0),
			},
			wantCost:   "3",
			wantPoints: 1,
		},
		{
			name:  "large token count — correct precision",
			event: UsageEvent{InputTokens: 123_456_789},
			prices: map[string]resolvedPrice{
				models.PriceTypeInput: rp("3.00", 1_000_000),
			},
			wantCost:   "370.370367", // 123456789/1000000 * 3
			wantPoints: 1,
		},
		{
			name:  "price points snapshot records source and ID",
			event: UsageEvent{InputTokens: 100},
			prices: map[string]resolvedPrice{
				models.PriceTypeInput: {
					PricePerUnit: decimal.RequireFromString("5.00"),
					UnitSize:     1_000_000,
					PricingID:    42,
					Source:       "contract",
				},
			},
			wantCost:   "0.0005", // 100/1000000 * 5
			wantPoints: 1,
		},
		{
			name:  "fractional token cost",
			event: UsageEvent{InputTokens: 1},
			prices: map[string]resolvedPrice{
				models.PriceTypeInput: rp("3.00", 1_000_000),
			},
			wantCost:   "0.000003",
			wantPoints: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, points := calc.Calculate(tt.event, tt.prices)

			want := decimal.RequireFromString(tt.wantCost)
			if !cost.Equal(want) {
				t.Errorf("cost = %s, want %s", cost.String(), want.String())
			}
			if len(points) != tt.wantPoints {
				t.Errorf("got %d price points, want %d", len(points), tt.wantPoints)
			}
		})
	}
}

func TestCalculate_PricePointSnapshot(t *testing.T) {
	calc := NewCalculator()
	event := UsageEvent{InputTokens: 100}
	prices := map[string]resolvedPrice{
		models.PriceTypeInput: {
			PricePerUnit: decimal.RequireFromString("5.00"),
			UnitSize:     1_000_000,
			PricingID:    42,
			Source:       "contract",
		},
	}

	_, points := calc.Calculate(event, prices)

	pt, ok := points[models.PriceTypeInput]
	if !ok {
		t.Fatal("expected input price point")
	}
	if pt.PricePerUnit != "5" {
		t.Errorf("PricePerUnit = %q, want %q", pt.PricePerUnit, "5")
	}
	if pt.UnitSize != 1_000_000 {
		t.Errorf("UnitSize = %d, want %d", pt.UnitSize, 1_000_000)
	}
	if pt.PricingID != 42 {
		t.Errorf("PricingID = %d, want %d", pt.PricingID, 42)
	}
	if pt.Source != "contract" {
		t.Errorf("Source = %q, want %q", pt.Source, "contract")
	}
}

func TestApplyMarkups(t *testing.T) {
	calc := NewCalculator()

	mkup := func(pct string) models.PricingMarkup {
		return models.PricingMarkup{Percentage: decimal.RequireFromString(pct)}
	}

	tests := []struct {
		name    string
		base    string
		markups []models.PricingMarkup
		want    string
	}{
		{
			name:    "no markups — returns base unchanged",
			base:    "100",
			markups: nil,
			want:    "100",
		},
		{
			name:    "single 10% markup",
			base:    "100",
			markups: []models.PricingMarkup{mkup("10")},
			want:    "110",
		},
		{
			name:    "multiple markups sum (10% + 5% = 15%)",
			base:    "100",
			markups: []models.PricingMarkup{mkup("10"), mkup("5")},
			want:    "115",
		},
		{
			name:    "zero base cost — returns zero regardless of markups",
			base:    "0",
			markups: []models.PricingMarkup{mkup("50")},
			want:    "0",
		},
		{
			name:    "zero percent markup — returns base unchanged",
			base:    "100",
			markups: []models.PricingMarkup{mkup("0")},
			want:    "100",
		},
		{
			name:    "large markup percentage (200%)",
			base:    "100",
			markups: []models.PricingMarkup{mkup("200")},
			want:    "300",
		},
		{
			name:    "fractional markup percentage (2.5%)",
			base:    "200",
			markups: []models.PricingMarkup{mkup("2.5")},
			want:    "205",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := decimal.RequireFromString(tt.base)
			got := calc.ApplyMarkups(base, tt.markups)
			want := decimal.RequireFromString(tt.want)
			if !got.Equal(want) {
				t.Errorf("ApplyMarkups(%s) = %s, want %s", tt.base, got.String(), want.String())
			}
		})
	}
}
