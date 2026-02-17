package models

import (
	"time"

	"github.com/lib/pq"
)

// User represents a dashboard user synced from Clerk on first sign-in.
type User struct {
	ID              string    `gorm:"primaryKey;type:text"` // Clerk user ID e.g. user_2lXYZ…
	Email           string    `gorm:"uniqueIndex"`
	Name            string
	Role            string    `gorm:"default:viewer"` // owner | admin | editor | viewer
	Status          string    `gorm:"default:active"` // active | suspended | pending
	BurnrateAPIKey  string    `gorm:"column:burnrate_api_key"` // key_id of the user's primary gateway key
	CreatedAt       time.Time
}

// Role constants
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// Status constants
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusPending   = "pending"
)

func RoleLevel(role string) int {
	switch role {
	case RoleOwner:
		return 4
	case RoleAdmin:
		return 3
	case RoleEditor:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

func (u *User) HasPermission(required string) bool {
	return RoleLevel(u.Role) >= RoleLevel(required)
}

func (u *User) IsActive() bool {
	return u.Status == StatusActive
}

// APIKey is the machine-to-machine key used by the claude-code agent
// to authenticate with the burnrate gateway.
// Pattern mirrors kubernetes-cost-monitor api_keys table.
type APIKey struct {
	ID         uint           `gorm:"primaryKey"`
	UserID     string         `gorm:"index;type:text"`
	KeyID      string         `gorm:"uniqueIndex;size:36"`
	Label      string
	Salt       []byte
	SecretHash []byte
	Scopes     pq.StringArray `gorm:"type:text[]"`
	Revoked    bool
	ExpiresAt  *time.Time
	CreatedAt  time.Time
}

// ProviderKey stores an upstream LLM provider API key encrypted at the
// application layer (AES-256-GCM) before being written to the database.
type ProviderKey struct {
	ID              uint      `gorm:"primaryKey"`
	UserID          string    `gorm:"index;type:text"`
	Provider        string    // "anthropic" | "openai"
	EncryptedAPIKey []byte    `gorm:"column:encrypted_api_key"`
	Label           string
	Revoked         bool
	CreatedAt       time.Time
}

// UsageLog records one LLM request reported by the claude-code agent.
// request_id is an idempotency key; duplicate submissions are ignored.
type UsageLog struct {
	ID               uint      `gorm:"primaryKey"`
	UserID           string    `gorm:"index;type:text"`
	Provider         string
	Model            string
	PromptTokens     int64     `gorm:"column:prompt_tokens"`
	CompletionTokens int64     `gorm:"column:completion_tokens"`
	Cost             float64   `gorm:"type:numeric(14,8)"`
	RequestID        string    `gorm:"column:request_id;uniqueIndex"`
	CreatedAt        time.Time `gorm:"index"`
}
