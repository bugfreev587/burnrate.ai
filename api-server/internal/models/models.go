package models

import (
	"time"

	"github.com/lib/pq"
)

// User represents a dashboard user (synced from Clerk).
type User struct {
	ID        string    `gorm:"primaryKey;type:text"` // Clerk user ID
	Email     string    `gorm:"uniqueIndex"`
	Name      string
	Role      string    `gorm:"default:viewer"` // owner | admin | editor | viewer
	Status    string    `gorm:"default:active"` // active | suspended | pending
	CreatedAt time.Time
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

// ProviderKey stores an upstream LLM provider API key for a user.
type ProviderKey struct {
	ID         uint      `gorm:"primaryKey"`
	UserID     string    `gorm:"index;type:text"` // references users.id
	Provider   string    // e.g. "anthropic", "openai"
	KeyID      string    `gorm:"uniqueIndex;size:36"` // UUID
	Salt       []byte
	SecretHash []byte
	Label      string
	Revoked    bool
	ExpiresAt  *time.Time
	CreatedAt  time.Time
}

// UsageLog records token usage events reported by the claude-code agent.
type UsageLog struct {
	ID           uint      `gorm:"primaryKey"`
	UserID       string    `gorm:"index;type:text"`
	Provider     string    // "anthropic"
	Model        string    // e.g. "claude-sonnet-4-6"
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
	RequestedAt  time.Time `gorm:"index"`
	CreatedAt    time.Time
}

// APIKey is the machine-to-machine key used by the claude-code agent
// (or CI pipelines) to authenticate with the burnrate gateway.
type APIKey struct {
	ID         uint           `gorm:"primaryKey"`
	UserID     string         `gorm:"index;type:text"`
	KeyID      string         `gorm:"uniqueIndex;size:36"`
	Salt       []byte
	SecretHash []byte
	Label      string
	Scopes     pq.StringArray `gorm:"type:text[]"`
	Revoked    bool
	ExpiresAt  *time.Time
	CreatedAt  time.Time
}
