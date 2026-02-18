package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/models"
)

// PricingEngine orchestrates the full pricing pipeline:
// UsageEvent → Resolve → Calculate → Markup → Budget Check → Ledger Write
type PricingEngine struct {
	db         *gorm.DB
	rdb        *redis.Client
	resolver   *PricingResolver
	calculator *Calculator
}

// NewPricingEngine returns a new PricingEngine.
func NewPricingEngine(db *gorm.DB, rdb *redis.Client) *PricingEngine {
	return &PricingEngine{
		db:         db,
		rdb:        rdb,
		resolver:   NewPricingResolver(db, rdb),
		calculator: NewCalculator(),
	}
}

// Process runs the full pricing pipeline for a usage event.
// If the model is not found in the catalog, it returns a zero-cost result (not an error).
func (e *PricingEngine) Process(ctx context.Context, event UsageEvent) (*PricingResult, error) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// 1. Resolve prices
	resolved, err := e.resolver.Resolve(ctx, event)
	if err != nil {
		var notFound *ErrModelNotFound
		if errors.As(err, &notFound) {
			log.Printf("pricing: model not found (provider=%s model=%s), returning zero cost", event.Provider, event.Model)
			return &PricingResult{
				BaseCost:     decimal.Zero,
				MarkupAmount: decimal.Zero,
				FinalCost:    decimal.Zero,
				Snapshot: PricingSnapshot{
					Provider:    event.Provider,
					Model:       event.Model,
					PricePoints: map[string]PricePoint{},
					MarkupPct:   "0",
					SnapshotAt:  event.Timestamp,
				},
			}, nil
		}
		return nil, fmt.Errorf("pricing: resolve: %w", err)
	}

	// 2. Calculate base cost
	baseCost, pricePoints := e.calculator.Calculate(event, resolved.Prices)

	// 3. Resolve and apply markups
	markups := e.resolveMarkups(ctx, event, resolved)
	finalCost := e.calculator.ApplyMarkups(baseCost, markups)

	// Compute markup amount
	markupAmount := finalCost.Sub(baseCost)

	// Compute total markup percentage for snapshot
	markupPct := decimal.Zero
	for _, m := range markups {
		markupPct = markupPct.Add(m.Percentage)
	}

	result := &PricingResult{
		BaseCost:     baseCost,
		MarkupAmount: markupAmount,
		FinalCost:    finalCost,
		ProviderID:   resolved.ProviderID,
		ModelID:      resolved.ModelID,
		Snapshot: PricingSnapshot{
			Provider:    event.Provider,
			Model:       event.Model,
			PricePoints: pricePoints,
			MarkupPct:   markupPct.String(),
			SnapshotAt:  event.Timestamp,
		},
	}

	// 4. Budget check (blocking budgets only)
	if err := e.checkBudget(ctx, event, finalCost); err != nil {
		return nil, err
	}

	// 5. Write immutable ledger entry
	if err := e.writeLedger(ctx, event, result); err != nil {
		return nil, err
	}

	// 6. Increment Redis budget counters
	e.incrementBudget(ctx, event, finalCost)

	return result, nil
}

// resolveMarkups fetches all applicable markups for the tenant, sorted by priority.
func (e *PricingEngine) resolveMarkups(ctx context.Context, event UsageEvent, resolved *ResolvedPrices) []models.PricingMarkup {
	ts := event.Timestamp
	var markups []models.PricingMarkup
	e.db.WithContext(ctx).
		Where(`tenant_id = ?
			AND effective_from <= ?
			AND (effective_to IS NULL OR effective_to > ?)
			AND (provider_id IS NULL OR provider_id = ?)
			AND (model_id IS NULL OR model_id = ?)`,
			event.TenantID, ts, ts, resolved.ProviderID, resolved.ModelID).
		Order("priority DESC").
		Find(&markups)
	return markups
}

// checkBudget checks all blocking budget limits for the tenant.
// Returns *ErrBudgetExceeded if any blocking limit is breached.
func (e *PricingEngine) checkBudget(ctx context.Context, event UsageEvent, finalCost decimal.Decimal) error {
	var limits []models.BudgetLimit
	e.db.WithContext(ctx).
		Where("tenant_id = ? AND action = ?", event.TenantID, models.BudgetActionBlock).
		Find(&limits)

	for _, limit := range limits {
		key := e.budgetRedisKey(event.TenantID, limit.PeriodType, event.Timestamp)
		var currentSpend decimal.Decimal

		if e.rdb != nil {
			val, err := e.rdb.Get(ctx, key).Result()
			if err == nil {
				currentSpend, _ = decimal.NewFromString(val)
			} else {
				// Fall back to DB if Redis key missing
				currentSpend = e.dbBudgetSpend(ctx, event.TenantID, limit.PeriodType, event.Timestamp)
			}
		} else {
			currentSpend = e.dbBudgetSpend(ctx, event.TenantID, limit.PeriodType, event.Timestamp)
		}

		if currentSpend.Add(finalCost).GreaterThan(limit.LimitAmount) {
			return &ErrBudgetExceeded{
				TenantID:     event.TenantID,
				LimitAmount:  limit.LimitAmount,
				CurrentSpend: currentSpend,
				Period:       limit.PeriodType,
			}
		}
	}
	return nil
}

// dbBudgetSpend queries the cost ledger for spend in the current period.
func (e *PricingEngine) dbBudgetSpend(ctx context.Context, tenantID uint, periodType string, t time.Time) decimal.Decimal {
	start := periodStart(periodType, t)
	var total decimal.Decimal
	e.db.WithContext(ctx).
		Model(&models.CostLedger{}).
		Select("COALESCE(SUM(final_cost), 0)").
		Where("tenant_id = ? AND timestamp >= ?", tenantID, start).
		Scan(&total)
	return total
}

// incrementBudget increments the Redis budget counters for all periods.
// Uses INCRBYFLOAT (acceptable soft-cap precision; ledger is authoritative).
func (e *PricingEngine) incrementBudget(ctx context.Context, event UsageEvent, finalCost decimal.Decimal) {
	if e.rdb == nil {
		return
	}
	for _, period := range []string{models.PeriodMonthly, models.PeriodWeekly, models.PeriodDaily} {
		key := e.budgetRedisKey(event.TenantID, period, event.Timestamp)
		amount := finalCost.InexactFloat64()
		e.rdb.IncrByFloat(ctx, key, amount)
		// Set expiry to end of period
		expiry := periodEnd(period, event.Timestamp)
		e.rdb.ExpireAt(ctx, key, expiry)
	}
}

// writeLedger writes an immutable CostLedger record.
// Silently ignores duplicate idempotency key errors (already processed).
func (e *PricingEngine) writeLedger(ctx context.Context, event UsageEvent, result *PricingResult) error {
	snapshotJSON, err := json.Marshal(result.Snapshot)
	if err != nil {
		return fmt.Errorf("pricing: marshal snapshot: %w", err)
	}

	ledger := &models.CostLedger{
		TenantID:            event.TenantID,
		UserID:              event.UserID,
		ProviderID:          result.ProviderID,
		ModelID:             result.ModelID,
		InputTokens:         event.InputTokens,
		OutputTokens:        event.OutputTokens,
		CacheCreationTokens: event.CacheCreationTokens,
		CacheReadTokens:     event.CacheReadTokens,
		ReasoningTokens:     event.ReasoningTokens,
		BaseCost:            result.BaseCost,
		MarkupAmount:        result.MarkupAmount,
		FinalCost:           result.FinalCost,
		PricingSnapshot:     string(snapshotJSON),
		IdempotencyKey:      event.IdempotencyKey,
		Timestamp:           event.Timestamp,
	}

	if err := e.db.WithContext(ctx).Create(ledger).Error; err != nil {
		// Silently ignore duplicate idempotency_key (unique constraint violation)
		if isDuplicateKeyError(err) {
			log.Printf("pricing: duplicate idempotency_key=%s, skipping ledger write", event.IdempotencyKey)
			return nil
		}
		return fmt.Errorf("pricing: write ledger: %w", err)
	}
	return nil
}

// budgetRedisKey returns the Redis key for a tenant's budget counter.
func (e *PricingEngine) budgetRedisKey(tenantID uint, periodType string, t time.Time) string {
	switch periodType {
	case models.PeriodMonthly:
		return fmt.Sprintf("budget:%d:%d-%02d", tenantID, t.Year(), t.Month())
	case models.PeriodWeekly:
		year, week := t.ISOWeek()
		return fmt.Sprintf("budget:%d:w%d-%02d", tenantID, year, week)
	case models.PeriodDaily:
		return fmt.Sprintf("budget:%d:%d-%02d-%02d", tenantID, t.Year(), t.Month(), t.Day())
	default:
		return fmt.Sprintf("budget:%d:%s:%d-%02d", tenantID, periodType, t.Year(), t.Month())
	}
}

// periodStart returns the start of a budget period.
func periodStart(periodType string, t time.Time) time.Time {
	switch periodType {
	case models.PeriodMonthly:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	case models.PeriodWeekly:
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return t.AddDate(0, 0, -(weekday - 1)).Truncate(24 * time.Hour)
	case models.PeriodDaily:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	default:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	}
}

// periodEnd returns the expiry time (exclusive end) of a budget period.
func periodEnd(periodType string, t time.Time) time.Time {
	switch periodType {
	case models.PeriodMonthly:
		start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
		return start.AddDate(0, 1, 0)
	case models.PeriodWeekly:
		start := periodStart(models.PeriodWeekly, t)
		return start.AddDate(0, 0, 7)
	case models.PeriodDaily:
		return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
	default:
		start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
		return start.AddDate(0, 1, 0)
	}
}

// isDuplicateKeyError detects PostgreSQL unique constraint violations.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "duplicate key") || contains(errStr, "unique constraint") || contains(errStr, "23505")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
