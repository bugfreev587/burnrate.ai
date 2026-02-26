package events

import (
	"context"

	"github.com/xiaoboyu/tokengate/api-server/internal/pricing"
	"github.com/xiaoboyu/tokengate/api-server/internal/ratelimit"
)

// PricingNotificationAdapter adapts NotificationQueue to satisfy pricing.NotificationPublisher.
type PricingNotificationAdapter struct {
	q *NotificationQueue
}

// NewPricingNotificationAdapter creates an adapter for the pricing engine.
func NewPricingNotificationAdapter(q *NotificationQueue) *PricingNotificationAdapter {
	return &PricingNotificationAdapter{q: q}
}

// Publish converts pricing.NotificationEventMsg to events.NotificationEventMsg and publishes.
func (a *PricingNotificationAdapter) Publish(ctx context.Context, msg pricing.NotificationEventMsg) error {
	return a.q.Publish(ctx, NotificationEventMsg{
		TenantID:  msg.TenantID,
		EventType: msg.EventType,
		KeyID:     msg.KeyID,
		Provider:  msg.Provider,
		Model:     msg.Model,
		Details:   msg.Details,
	})
}

// RateLimitNotificationAdapter adapts NotificationQueue to satisfy ratelimit.NotificationPublisher.
type RateLimitNotificationAdapter struct {
	q *NotificationQueue
}

// NewRateLimitNotificationAdapter creates an adapter for the rate limiter.
func NewRateLimitNotificationAdapter(q *NotificationQueue) *RateLimitNotificationAdapter {
	return &RateLimitNotificationAdapter{q: q}
}

// Publish converts ratelimit.NotificationEventMsg to events.NotificationEventMsg and publishes.
func (a *RateLimitNotificationAdapter) Publish(ctx context.Context, msg ratelimit.NotificationEventMsg) error {
	return a.q.Publish(ctx, NotificationEventMsg{
		TenantID:  msg.TenantID,
		EventType: msg.EventType,
		KeyID:     msg.KeyID,
		Provider:  msg.Provider,
		Model:     msg.Model,
		Details:   msg.Details,
	})
}
