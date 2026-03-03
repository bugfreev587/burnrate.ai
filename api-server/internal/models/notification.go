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
	EventTeamInvitation    = "team_invitation"
	EventMemberUpdated    = "member_updated"
)

// NotificationChannel stores a tenant's notification channel configuration.
type NotificationChannel struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	TenantID    uint           `gorm:"index;not null" json:"tenant_id"`
	ChannelType string         `gorm:"not null" json:"channel_type"`   // "email" | "slack" | "webhook"
	Name        string         `json:"name"`                           // user-friendly label
	Config      string         `gorm:"type:text" json:"config"`        // JSON: {"email":"..."} or {"slack_webhook_url":"..."} or {"webhook_url":"...","signing_secret":"..."}
	EventTypes  pq.StringArray `gorm:"type:text[]" json:"event_types"` // ["budget_blocked","budget_warning","rate_limit_exceeded"]
	Enabled     bool           `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// UserNotification stores in-app notifications for a specific user.
type UserNotification struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    string     `gorm:"type:text;index;not null" json:"user_id"`
	TenantID  *uint      `gorm:"index" json:"tenant_id,omitempty"` // related tenant (if applicable)
	Type      string     `gorm:"not null;index" json:"type"`       // e.g. "team_invitation"
	Title     string     `gorm:"not null" json:"title"`
	Body      string     `gorm:"type:text" json:"body"`
	Payload   string     `gorm:"type:jsonb" json:"payload"` // arbitrary structured payload
	Status    string     `gorm:"not null;default:unread;index" json:"status"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// UserNotificationChannel stores personal outbound notification channels for a user.
// These channels are independent from tenant-level channels and can be used for
// personal events (for example, team invitations).
type UserNotificationChannel struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	UserID      string         `gorm:"type:text;index;not null" json:"user_id"`
	ChannelType string         `gorm:"not null" json:"channel_type"`   // "email" | "slack" | "webhook"
	Name        string         `json:"name"`                           // user-friendly label
	Config      string         `gorm:"type:text" json:"config"`        // JSON config
	EventTypes  pq.StringArray `gorm:"type:text[]" json:"event_types"` // e.g. ["team_invitation"]
	Enabled     bool           `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

const (
	UserNotificationStatusUnread = "unread"
	UserNotificationStatusRead   = "read"
)
