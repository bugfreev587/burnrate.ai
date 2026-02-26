package ratelimit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// RateLimitResult is returned when a rate limit is exceeded.
type RateLimitResult struct {
	Exceeded     bool
	Metric       string
	Limit        int64
	Used         int64
	RetryAfterMs int64
}

// NotificationPublisher is the interface used to publish notification events.
type NotificationPublisher interface {
	Publish(ctx context.Context, msg NotificationEventMsg) error
}

// NotificationEventMsg mirrors events.NotificationEventMsg to avoid import cycles.
type NotificationEventMsg struct {
	TenantID  uint
	EventType string
	KeyID     string
	Provider  string
	Model     string
	Details   string
}

// Limiter enforces tenant-aware, model-scoped rate limits using Redis fixed-window counters.
type Limiter struct {
	db         *gorm.DB
	rdb        *redis.Client
	notifQueue NotificationPublisher
}

// NewLimiter creates a new rate limit enforcer.
func NewLimiter(db *gorm.DB, rdb *redis.Client) *Limiter {
	return &Limiter{db: db, rdb: rdb}
}

// SetNotificationQueue sets the notification publisher for rate limit events.
func (l *Limiter) SetNotificationQueue(nq NotificationPublisher) {
	l.notifQueue = nq
}

// Check evaluates all applicable rate limits for the request.
// Returns a *RateLimitResult if any limit is exceeded, or nil if all pass.
func (l *Limiter) Check(ctx context.Context, tenantID uint, keyID, provider, model string, estimatedInputTokens int64, maxTokens int) (*RateLimitResult, error) {
	if l.rdb == nil {
		return nil, nil
	}

	limits, err := l.loadLimits(ctx, tenantID)
	if err != nil {
		slog.Error("ratelimit_load_limits_failed", "tenant_id", tenantID, "error", err)
		return nil, nil // don't block on config errors
	}

	if len(limits) == 0 {
		return nil, nil
	}

	now := time.Now()

	for _, limit := range limits {
		if !limit.Enabled {
			continue
		}

		// Check provider match
		if limit.Provider != "" && limit.Provider != provider {
			continue
		}

		// Check model match
		if limit.Model != "" && limit.Model != model {
			continue
		}

		// Check scope match
		if limit.ScopeType == models.BudgetScopeAPIKey && limit.ScopeID != keyID {
			continue
		}

		// Determine increment amount based on metric
		var amount int64
		switch limit.Metric {
		case models.RateLimitMetricRPM:
			amount = 1
		case models.RateLimitMetricITPM:
			amount = estimatedInputTokens
		case models.RateLimitMetricOTPM:
			amount = int64(maxTokens) // worst-case reservation
		default:
			continue
		}

		if amount <= 0 {
			continue
		}

		windowSec := limit.WindowSeconds
		if windowSec <= 0 {
			windowSec = 60
		}

		windowID := now.Unix() / int64(windowSec)
		key := l.counterKey(tenantID, limit, windowID)

		// Atomically increment and check
		newVal, err := l.rdb.IncrBy(ctx, key, amount).Result()
		if err != nil {
			slog.Error("ratelimit_redis_incrby_error", "error", err)
			continue // don't block on Redis errors
		}

		// Set expiry on first increment (2x window to handle edge cases)
		if newVal == amount {
			l.rdb.Expire(ctx, key, time.Duration(windowSec*2)*time.Second)
		}

		if newVal > limit.LimitValue {
			// Exceeded — compute retry-after
			windowEnd := (windowID + 1) * int64(windowSec)
			retryAfterMs := (windowEnd - now.Unix()) * 1000

			// Roll back the increment since we're rejecting the request
			l.rdb.DecrBy(ctx, key, amount)

			slog.Warn("rate_limit_exceeded",
				"tenant_id", tenantID, "key_id", keyID,
				"provider", provider, "model", model,
				"metric", limit.Metric, "limit", limit.LimitValue,
				"used", newVal-amount, "retry_after_ms", retryAfterMs,
			)
			if l.notifQueue != nil {
				_ = l.notifQueue.Publish(ctx, NotificationEventMsg{
					TenantID:  tenantID,
					EventType: models.EventRateLimitExceeded,
					KeyID:     keyID,
					Provider:  provider,
					Model:     model,
					Details:   fmt.Sprintf(`{"metric":"%s","limit":%d,"used":%d}`, limit.Metric, limit.LimitValue, newVal-amount),
				})
			}

			return &RateLimitResult{
				Exceeded:     true,
				Metric:       limit.Metric,
				Limit:        limit.LimitValue,
				Used:         newVal - amount,
				RetryAfterMs: retryAfterMs,
			}, nil
		}
	}

	return nil, nil
}

// Reconcile adjusts OTPM counters after response when actual output tokens are known.
// It decrements by (maxTokens - actualOutputTokens) to release the reservation.
func (l *Limiter) Reconcile(ctx context.Context, tenantID uint, keyID, provider, model string, actualOutputTokens int64, maxTokens int) {
	if l.rdb == nil || maxTokens <= 0 {
		return
	}

	diff := int64(maxTokens) - actualOutputTokens
	if diff <= 0 {
		return
	}

	slog.Debug("rate_limit_reconciled",
		"tenant_id", tenantID, "key_id", keyID,
		"provider", provider, "model", model,
		"actual_output_tokens", actualOutputTokens, "max_tokens", maxTokens,
		"delta", diff,
	)

	limits, err := l.loadLimits(ctx, tenantID)
	if err != nil {
		return
	}

	now := time.Now()

	for _, limit := range limits {
		if !limit.Enabled || limit.Metric != models.RateLimitMetricOTPM {
			continue
		}
		if limit.Provider != "" && limit.Provider != provider {
			continue
		}
		if limit.Model != "" && limit.Model != model {
			continue
		}
		if limit.ScopeType == models.BudgetScopeAPIKey && limit.ScopeID != keyID {
			continue
		}

		windowSec := limit.WindowSeconds
		if windowSec <= 0 {
			windowSec = 60
		}

		windowID := now.Unix() / int64(windowSec)
		key := l.counterKey(tenantID, limit, windowID)

		l.rdb.DecrBy(ctx, key, diff)
	}
}

// counterKey returns the Redis key for a rate limit counter.
func (l *Limiter) counterKey(tenantID uint, limit models.RateLimit, windowID int64) string {
	return fmt.Sprintf("rl:%d:%s:%s:%s:%s:%s:%d",
		tenantID, limit.Provider, limit.Model,
		limit.ScopeType, limit.ScopeID,
		limit.Metric, windowID)
}

// loadLimits loads rate limit config for a tenant, with 60s Redis cache.
func (l *Limiter) loadLimits(ctx context.Context, tenantID uint) ([]models.RateLimit, error) {
	cacheKey := fmt.Sprintf("rl:config:%d", tenantID)

	// Try cache first
	if val, err := l.rdb.Get(ctx, cacheKey).Result(); err == nil {
		var limits []models.RateLimit
		if json.Unmarshal([]byte(val), &limits) == nil {
			return limits, nil
		}
	}

	// Load from DB
	var limits []models.RateLimit
	if err := l.db.WithContext(ctx).
		Where("tenant_id = ? AND enabled = ?", tenantID, true).
		Find(&limits).Error; err != nil {
		return nil, err
	}

	// Cache for 60s
	if data, err := json.Marshal(limits); err == nil {
		l.rdb.Set(ctx, cacheKey, string(data), 60*time.Second)
	}

	return limits, nil
}

// InvalidateCache removes the cached rate limit config for a tenant.
func (l *Limiter) InvalidateCache(ctx context.Context, tenantID uint) {
	if l.rdb == nil {
		return
	}
	cacheKey := fmt.Sprintf("rl:config:%d", tenantID)
	l.rdb.Del(ctx, cacheKey)
}

// GetCurrentUsage returns the current counter value for a specific rate limit.
func (l *Limiter) GetCurrentUsage(ctx context.Context, tenantID uint, limit models.RateLimit) int64 {
	if l.rdb == nil {
		return 0
	}

	now := time.Now()
	windowSec := limit.WindowSeconds
	if windowSec <= 0 {
		windowSec = 60
	}
	windowID := now.Unix() / int64(windowSec)
	key := l.counterKey(tenantID, limit, windowID)

	val, err := l.rdb.Get(ctx, key).Int64()
	if err != nil {
		return 0
	}
	return val
}
