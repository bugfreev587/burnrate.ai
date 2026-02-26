package models

import (
	"time"

	"github.com/lib/pq"
)

// Notification event type constants
const (
	EventBudgetBlocked     = "budget_blocked"
	EventBudgetWarning     = "budget_warning"
	EventRateLimitExceeded = "rate_limit_exceeded"
)

// NotificationChannel stores a tenant's notification channel configuration.
type NotificationChannel struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	TenantID    uint           `gorm:"index;not null" json:"tenant_id"`
	ChannelType string         `gorm:"not null" json:"channel_type"`            // "email" | "slack" | "webhook"
	Name        string         `json:"name"`                                    // user-friendly label
	Config      string         `gorm:"type:text" json:"config"`                 // JSON: {"email":"..."} or {"slack_webhook_url":"..."} or {"webhook_url":"...","signing_secret":"..."}
	EventTypes  pq.StringArray `gorm:"type:text[]" json:"event_types"`          // ["budget_blocked","budget_warning","rate_limit_exceeded"]
	Enabled     bool           `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}
