package models

import (
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

// Tenant is the top-level multi-tenant boundary.
// Every user, API key, provider key, and usage log belongs to exactly one tenant.
type Tenant struct {
	ID        uint      `gorm:"primaryKey"`
	Name      string
	CreatedAt time.Time
}

// User represents a dashboard user synced from Clerk on first sign-in.
type User struct {
	ID        string    `gorm:"primaryKey;type:text"` // Clerk user ID e.g. user_2lXYZ…
	TenantID  uint      `gorm:"index"`
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

// APIKey is the machine-to-machine key used by the claude-code agent
// to authenticate with the burnrate gateway. Scoped to a tenant.
type APIKey struct {
	ID         uint           `gorm:"primaryKey"`
	TenantID   uint           `gorm:"index"`
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
	TenantID        uint      `gorm:"index"`
	Provider        string    // "anthropic" | "openai"
	EncryptedAPIKey []byte    `gorm:"column:encrypted_api_key"`
	Label           string
	Revoked         bool
	CreatedAt       time.Time
}

// UsageLog records one LLM request reported by the claude-code agent.
// request_id is an idempotency key; duplicate submissions are ignored.
type UsageLog struct {
	ID                  uint            `gorm:"primaryKey"`
	TenantID            uint            `gorm:"index"`
	UserID              string          `gorm:"index"`
	Provider            string
	Model               string
	PromptTokens        int64           `gorm:"column:prompt_tokens"`
	CompletionTokens    int64           `gorm:"column:completion_tokens"`
	CacheCreationTokens int64           `gorm:"column:cache_creation_tokens"`
	CacheReadTokens     int64           `gorm:"column:cache_read_tokens"`
	ReasoningTokens     int64           `gorm:"column:reasoning_tokens"`
	Cost                decimal.Decimal `gorm:"type:numeric(20,8)"`
	RequestID           string          `gorm:"column:request_id;uniqueIndex"`
	CreatedAt           time.Time       `gorm:"index"`
}
