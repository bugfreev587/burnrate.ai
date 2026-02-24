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

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
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

// PreCheckBudget checks whether the tenant's current spend already exceeds any
// blocking budget limit. It also returns the nearest-threshold BudgetStatus for
// informational headers. Call this before forwarding a proxy request.
//
// Returns (*BudgetStatus, *ErrBudgetExceeded) — the status is non-nil when any
// limit is at or above its alert threshold; the error is non-nil when a blocking
// limit is exceeded.
func (e *PricingEngine) PreCheckBudget(ctx context.Context, tenantID uint, keyID string, provider string, now time.Time) (*BudgetStatus, error) {
	var limits []models.BudgetLimit
	query := e.db.WithContext(ctx).
		Where("tenant_id = ? AND (scope_type = ? OR (scope_type = ? AND scope_id = ?))",
			tenantID,
			models.BudgetScopeAccount,
			models.BudgetScopeAPIKey, keyID,
		).
		Find(&limits)
	if query.Error != nil {
		return nil, nil // don't block on DB errors
	}

	var nearestStatus *BudgetStatus

	for _, limit := range limits {
		// Skip provider-scoped limits that don't match the current request's provider.
		if limit.Provider != "" && limit.Provider != provider {
			continue
		}

		spend := e.getSpend(ctx, tenantID, keyID, limit.ScopeType, limit.ScopeID, limit.PeriodType, limit.Provider, now)
		reserved := e.getReserved(ctx, tenantID, keyID, limit.ScopeType, limit.ScopeID, limit.PeriodType, now)
		effective := spend.Add(reserved)

		// Compute threshold amount
		thresholdAmount := limit.LimitAmount.Mul(limit.AlertThreshold).Div(decimal.NewFromInt(100))

		if (limit.Action == models.BudgetActionBlock || limit.Action == models.BudgetActionAlertBlock) && effective.GreaterThanOrEqual(limit.LimitAmount) {
			return &BudgetStatus{
				AtWarning:    true,
				Scope:        limit.ScopeType,
				Period:       limit.PeriodType,
				LimitAmount:  limit.LimitAmount,
				CurrentSpend: effective,
				Threshold:    limit.AlertThreshold,
			}, &ErrBudgetExceeded{
				TenantID:     tenantID,
				LimitAmount:  limit.LimitAmount,
				CurrentSpend: effective,
				Period:       limit.PeriodType,
			}
		}

		if effective.GreaterThanOrEqual(thresholdAmount) {
			if nearestStatus == nil || effective.Div(limit.LimitAmount).GreaterThan(nearestStatus.CurrentSpend.Div(nearestStatus.LimitAmount)) {
				nearestStatus = &BudgetStatus{
					AtWarning:    true,
					Scope:        limit.ScopeType,
					Period:       limit.PeriodType,
					LimitAmount:  limit.LimitAmount,
					CurrentSpend: effective,
					Threshold:    limit.AlertThreshold,
				}
			}
		}
	}

	return nearestStatus, nil
}

// ReserveSpend atomically reserves worst-case spend for a request in Redis.
// Returns the reserved amount. Call ReleaseReservation after the response completes.
func (e *PricingEngine) ReserveSpend(ctx context.Context, tenantID uint, keyID, provider, model string, maxTokens int) (decimal.Decimal, error) {
	if e.rdb == nil || maxTokens <= 0 {
		return decimal.Zero, nil
	}

	// Resolve output price for the model
	event := UsageEvent{
		Provider:  provider,
		Model:     model,
		Timestamp: time.Now(),
		TenantID:  tenantID,
		APIKeyRef: keyID,
	}
	resolved, err := e.resolver.Resolve(ctx, event)
	if err != nil {
		// If model not found, no reservation needed
		return decimal.Zero, nil
	}

	outputPrice, ok := resolved.Prices[models.PriceTypeOutput]
	if !ok || outputPrice.UnitSize == 0 {
		return decimal.Zero, nil
	}

	// Compute worst-case output cost: maxTokens * (pricePerUnit / unitSize)
	reservedAmount := outputPrice.PricePerUnit.
		Mul(decimal.NewFromInt(int64(maxTokens))).
		Div(decimal.NewFromInt(outputPrice.UnitSize))

	if reservedAmount.IsZero() {
		return decimal.Zero, nil
	}

	now := time.Now()
	amount := reservedAmount.InexactFloat64()

	// Increment reservation counters for all periods
	for _, period := range []string{models.PeriodMonthly, models.PeriodWeekly, models.PeriodDaily} {
		key := e.reservationRedisKey(tenantID, period, now)
		e.rdb.IncrByFloat(ctx, key, amount)
		e.rdb.ExpireAt(ctx, key, periodEnd(period, now))

		if keyID != "" {
			keyKey := e.reservationRedisKeyForKey(keyID, period, now)
			e.rdb.IncrByFloat(ctx, keyKey, amount)
			e.rdb.ExpireAt(ctx, keyKey, periodEnd(period, now))
		}
	}

	return reservedAmount, nil
}

// ReleaseReservation decrements the reservation counter after a request completes.
// The actual cost is already handled by incrementBudget in the worker pipeline.
func (e *PricingEngine) ReleaseReservation(ctx context.Context, tenantID uint, keyID string, reservedAmount decimal.Decimal) {
	if e.rdb == nil || reservedAmount.IsZero() {
		return
	}

	now := time.Now()
	amount := reservedAmount.InexactFloat64()

	for _, period := range []string{models.PeriodMonthly, models.PeriodWeekly, models.PeriodDaily} {
		key := e.reservationRedisKey(tenantID, period, now)
		e.rdb.IncrByFloat(ctx, key, -amount)

		if keyID != "" {
			keyKey := e.reservationRedisKeyForKey(keyID, period, now)
			e.rdb.IncrByFloat(ctx, keyKey, -amount)
		}
	}
}

// getReserved returns the current reservation amount from Redis.
func (e *PricingEngine) getReserved(ctx context.Context, tenantID uint, keyID, scopeType, scopeID, periodType string, t time.Time) decimal.Decimal {
	if e.rdb == nil {
		return decimal.Zero
	}

	var key string
	if scopeType == models.BudgetScopeAPIKey {
		key = e.reservationRedisKeyForKey(scopeID, periodType, t)
	} else {
		key = e.reservationRedisKey(tenantID, periodType, t)
	}

	if val, err := e.rdb.Get(ctx, key).Result(); err == nil {
		if reserved, err := decimal.NewFromString(val); err == nil && reserved.IsPositive() {
			return reserved
		}
	}
	return decimal.Zero
}

// reservationRedisKey returns the Redis key for a tenant's reservation counter.
func (e *PricingEngine) reservationRedisKey(tenantID uint, periodType string, t time.Time) string {
	switch periodType {
	case models.PeriodMonthly:
		return fmt.Sprintf("budget:reserved:%d:%d-%02d", tenantID, t.Year(), t.Month())
	case models.PeriodWeekly:
		year, week := t.ISOWeek()
		return fmt.Sprintf("budget:reserved:%d:w%d-%02d", tenantID, year, week)
	case models.PeriodDaily:
		return fmt.Sprintf("budget:reserved:%d:%d-%02d-%02d", tenantID, t.Year(), t.Month(), t.Day())
	default:
		return fmt.Sprintf("budget:reserved:%d:%s:%d-%02d", tenantID, periodType, t.Year(), t.Month())
	}
}

// reservationRedisKeyForKey returns the Redis key for an API key's reservation counter.
func (e *PricingEngine) reservationRedisKeyForKey(keyID, periodType string, t time.Time) string {
	switch periodType {
	case models.PeriodMonthly:
		return fmt.Sprintf("budget:reserved:key:%s:%d-%02d", keyID, t.Year(), t.Month())
	case models.PeriodWeekly:
		year, week := t.ISOWeek()
		return fmt.Sprintf("budget:reserved:key:%s:w%d-%02d", keyID, year, week)
	case models.PeriodDaily:
		return fmt.Sprintf("budget:reserved:key:%s:%d-%02d-%02d", keyID, t.Year(), t.Month(), t.Day())
	default:
		return fmt.Sprintf("budget:reserved:key:%s:%s:%d-%02d", keyID, periodType, t.Year(), t.Month())
	}
}

// getSpend returns the current spend for a budget limit scope from Redis (or DB fallback).
// When provider is non-empty, it reads from provider-scoped counters.
func (e *PricingEngine) getSpend(ctx context.Context, tenantID uint, keyID, scopeType, scopeID, periodType, provider string, t time.Time) decimal.Decimal {
	var key string
	if provider != "" {
		// Provider-scoped counters
		if scopeType == models.BudgetScopeAPIKey {
			key = e.budgetRedisKeyForKeyProvider(scopeID, provider, periodType, t)
		} else {
			key = e.budgetRedisKeyForProvider(tenantID, provider, periodType, t)
		}
	} else {
		if scopeType == models.BudgetScopeAPIKey {
			key = e.budgetRedisKeyForKey(scopeID, periodType, t)
		} else {
			key = e.budgetRedisKey(tenantID, periodType, t)
		}
	}

	if e.rdb != nil {
		if val, err := e.rdb.Get(ctx, key).Result(); err == nil {
			if spend, err := decimal.NewFromString(val); err == nil {
				return spend
			}
		}
	}

	// DB fallback (account-level only; key-level falls back to 0 if Redis missing)
	if scopeType == models.BudgetScopeAccount {
		return e.dbBudgetSpend(ctx, tenantID, periodType, provider, t)
	}
	return decimal.Zero
}

// checkBudget checks all blocking budget limits for the tenant (account + api_key scoped).
// Returns *ErrBudgetExceeded if any blocking limit is breached.
func (e *PricingEngine) checkBudget(ctx context.Context, event UsageEvent, finalCost decimal.Decimal) error {
	var limits []models.BudgetLimit
	e.db.WithContext(ctx).
		Where(`tenant_id = ? AND action = ? AND (scope_type = ? OR (scope_type = ? AND scope_id = ?))`,
			event.TenantID, models.BudgetActionBlock,
			models.BudgetScopeAccount,
			models.BudgetScopeAPIKey, event.APIKeyRef,
		).
		Find(&limits)

	for _, limit := range limits {
		// Skip provider-scoped limits that don't match this event's provider.
		if limit.Provider != "" && limit.Provider != event.Provider {
			continue
		}

		currentSpend := e.getSpend(ctx, event.TenantID, event.APIKeyRef, limit.ScopeType, limit.ScopeID, limit.PeriodType, limit.Provider, event.Timestamp)

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
// When provider is non-empty, it joins to filter by provider name.
func (e *PricingEngine) dbBudgetSpend(ctx context.Context, tenantID uint, periodType, provider string, t time.Time) decimal.Decimal {
	start := periodStart(periodType, t)
	var total decimal.Decimal
	q := e.db.WithContext(ctx).
		Model(&models.CostLedger{}).
		Where("tenant_id = ? AND timestamp >= ? AND api_usage_billed = ?", tenantID, start, true)
	if provider != "" {
		q = q.Where("provider_id IN (SELECT id FROM providers WHERE name = ?)", provider)
	}
	q.Select("COALESCE(SUM(final_cost), 0)").Scan(&total)
	return total
}

// incrementBudget increments Redis budget counters for all periods at both
// account scope and (if APIKeyRef is set) api_key scope.
// Uses INCRBYFLOAT (acceptable soft-cap precision; ledger is authoritative).
func (e *PricingEngine) incrementBudget(ctx context.Context, event UsageEvent, finalCost decimal.Decimal) {
	if e.rdb == nil {
		return
	}
	// Non-billable usage must not affect budget counters.
	if !event.APIUsageBilled {
		return
	}
	amount := finalCost.InexactFloat64()
	for _, period := range []string{models.PeriodMonthly, models.PeriodWeekly, models.PeriodDaily} {
		exp := periodEnd(period, event.Timestamp)

		// Account-level counter (all providers)
		key := e.budgetRedisKey(event.TenantID, period, event.Timestamp)
		e.rdb.IncrByFloat(ctx, key, amount)
		e.rdb.ExpireAt(ctx, key, exp)

		// Account-level counter (provider-scoped)
		if event.Provider != "" {
			provKey := e.budgetRedisKeyForProvider(event.TenantID, event.Provider, period, event.Timestamp)
			e.rdb.IncrByFloat(ctx, provKey, amount)
			e.rdb.ExpireAt(ctx, provKey, exp)
		}

		// API-key-level counter (if key ref is available)
		if event.APIKeyRef != "" {
			keyKey := e.budgetRedisKeyForKey(event.APIKeyRef, period, event.Timestamp)
			e.rdb.IncrByFloat(ctx, keyKey, amount)
			e.rdb.ExpireAt(ctx, keyKey, exp)

			// API-key-level counter (provider-scoped)
			if event.Provider != "" {
				keyProvKey := e.budgetRedisKeyForKeyProvider(event.APIKeyRef, event.Provider, period, event.Timestamp)
				e.rdb.IncrByFloat(ctx, keyProvKey, amount)
				e.rdb.ExpireAt(ctx, keyProvKey, exp)
			}
		}
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
		APIUsageBilled:      event.APIUsageBilled,
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

// budgetRedisKeyForKey returns the Redis key for an API key's budget counter.
func (e *PricingEngine) budgetRedisKeyForKey(keyID, periodType string, t time.Time) string {
	switch periodType {
	case models.PeriodMonthly:
		return fmt.Sprintf("budget:key:%s:%d-%02d", keyID, t.Year(), t.Month())
	case models.PeriodWeekly:
		year, week := t.ISOWeek()
		return fmt.Sprintf("budget:key:%s:w%d-%02d", keyID, year, week)
	case models.PeriodDaily:
		return fmt.Sprintf("budget:key:%s:%d-%02d-%02d", keyID, t.Year(), t.Month(), t.Day())
	default:
		return fmt.Sprintf("budget:key:%s:%s:%d-%02d", keyID, periodType, t.Year(), t.Month())
	}
}

// budgetRedisKeyForProvider returns the Redis key for a tenant's provider-scoped budget counter.
func (e *PricingEngine) budgetRedisKeyForProvider(tenantID uint, provider, periodType string, t time.Time) string {
	switch periodType {
	case models.PeriodMonthly:
		return fmt.Sprintf("budget:%d:%s:%d-%02d", tenantID, provider, t.Year(), t.Month())
	case models.PeriodWeekly:
		year, week := t.ISOWeek()
		return fmt.Sprintf("budget:%d:%s:w%d-%02d", tenantID, provider, year, week)
	case models.PeriodDaily:
		return fmt.Sprintf("budget:%d:%s:%d-%02d-%02d", tenantID, provider, t.Year(), t.Month(), t.Day())
	default:
		return fmt.Sprintf("budget:%d:%s:%s:%d-%02d", tenantID, provider, periodType, t.Year(), t.Month())
	}
}

// budgetRedisKeyForKeyProvider returns the Redis key for an API key's provider-scoped budget counter.
func (e *PricingEngine) budgetRedisKeyForKeyProvider(keyID, provider, periodType string, t time.Time) string {
	switch periodType {
	case models.PeriodMonthly:
		return fmt.Sprintf("budget:key:%s:%s:%d-%02d", keyID, provider, t.Year(), t.Month())
	case models.PeriodWeekly:
		year, week := t.ISOWeek()
		return fmt.Sprintf("budget:key:%s:%s:w%d-%02d", keyID, provider, year, week)
	case models.PeriodDaily:
		return fmt.Sprintf("budget:key:%s:%s:%d-%02d-%02d", keyID, provider, t.Year(), t.Month(), t.Day())
	default:
		return fmt.Sprintf("budget:key:%s:%s:%s:%d-%02d", keyID, provider, periodType, t.Year(), t.Month())
	}
}

// budgetRedisKey returns the Redis key for a tenant's account-level budget counter.
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
