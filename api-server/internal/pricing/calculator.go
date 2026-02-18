package pricing

import (
	"github.com/shopspring/decimal"
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/models"
)

// Calculator performs pure cost math with no database dependency.
type Calculator struct{}

// NewCalculator returns a new Calculator.
func NewCalculator() *Calculator {
	return &Calculator{}
}

// Calculate computes the base cost from token counts and resolved prices.
// Returns the base cost and the PricePoints map used (for snapshot).
func (c *Calculator) Calculate(event UsageEvent, prices map[string]resolvedPrice) (decimal.Decimal, map[string]PricePoint) {
	base := decimal.Zero
	points := make(map[string]PricePoint)

	dims := []struct {
		priceType string
		tokens    int64
	}{
		{models.PriceTypeInput, event.InputTokens},
		{models.PriceTypeOutput, event.OutputTokens},
		{models.PriceTypeCacheCreation, event.CacheCreationTokens},
		{models.PriceTypeCacheRead, event.CacheReadTokens},
		{models.PriceTypeReasoning, event.ReasoningTokens},
	}

	for _, dim := range dims {
		rp, ok := prices[dim.priceType]
		if !ok || dim.tokens == 0 {
			continue
		}
		unitSize := rp.UnitSize
		if unitSize == 0 {
			unitSize = 1_000_000
		}
		cost := decimal.NewFromInt(dim.tokens).
			Div(decimal.NewFromInt(unitSize)).
			Mul(rp.PricePerUnit)
		base = base.Add(cost)

		points[dim.priceType] = PricePoint{
			PricePerUnit: rp.PricePerUnit.String(),
			UnitSize:     unitSize,
			PricingID:    rp.PricingID,
			Source:       rp.Source,
		}
	}

	return base, points
}

// ApplyMarkups sums all markup percentages and applies them to the base cost.
// final = base * (1 + totalPct/100)
func (c *Calculator) ApplyMarkups(base decimal.Decimal, markups []models.PricingMarkup) decimal.Decimal {
	if len(markups) == 0 || base.IsZero() {
		return base
	}
	totalPct := decimal.Zero
	for _, m := range markups {
		totalPct = totalPct.Add(m.Percentage)
	}
	multiplier := decimal.NewFromInt(1).Add(totalPct.Div(decimal.NewFromInt(100)))
	return base.Mul(multiplier)
}
