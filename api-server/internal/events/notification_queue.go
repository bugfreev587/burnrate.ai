package events

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-redis/redis/v8"
)

const notificationStreamName = "tokengate:notification:events"

// NotificationQueue is a Redis Streams producer for notification events.
type NotificationQueue struct {
	rdb *redis.Client
}

// NewNotificationQueue creates a new NotificationQueue.
func NewNotificationQueue(rdb *redis.Client) *NotificationQueue {
	return &NotificationQueue{rdb: rdb}
}

// NotificationEventMsg is the payload published to the notification stream.
type NotificationEventMsg struct {
	TenantID  uint
	EventType string // "budget_blocked" | "budget_warning" | "rate_limit_exceeded"
	KeyID     string
	Provider  string
	Model     string
	Details   string // JSON with event-specific fields
}

// Publish XADD tokengate:notification:events * field value ...
// Fire-and-forget: errors are logged but not propagated to the caller.
func (q *NotificationQueue) Publish(ctx context.Context, msg NotificationEventMsg) error {
	values := map[string]interface{}{
		"tenant_id":  strconv.FormatUint(uint64(msg.TenantID), 10),
		"event_type": msg.EventType,
		"key_id":     msg.KeyID,
		"provider":   msg.Provider,
		"model":      msg.Model,
		"details":    msg.Details,
	}
	err := q.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: notificationStreamName,
		Values: values,
	}).Err()
	if err != nil {
		return fmt.Errorf("notificationqueue: XADD %s: %w", notificationStreamName, err)
	}
	return nil
}
