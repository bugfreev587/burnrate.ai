package models

import (
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

// Tenant is the top-level multi-tenant boundary.
// Every user, API key, provider key, and usage log belongs to exactly one tenant.
type Tenant struct {
	ID                   uint       `gorm:"primaryKey"`
	Name                 string
	Slug                 string     `gorm:"uniqueIndex;size:40"`   // path slug e.g. "acme"; 3–40 chars, ^[a-z0-9]([a-z0-9-]{1,38}[a-z0-9])?$
	Status               string     `gorm:"default:active"`        // active | suspended
	Plan                 string     `gorm:"default:free"` // free | pro | team | business
	MaxAPIKeys           int        `gorm:"default:1"`    // plan-derived; use GetPlanLimits for enforcement
	StripeCustomerID     string     `gorm:"column:stripe_customer_id;index"`
	StripeSubscriptionID string     `gorm:"column:stripe_subscription_id"`
	PlanStatus           string     `gorm:"column:plan_status;default:active"`
	CurrentPeriodEnd     *time.Time `gorm:"column:current_period_end"`
	BillingEmail         string     `gorm:"column:billing_email"`
	CreatedAt            time.Time
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

// PlanStatus constants (billing subscription status, independent from tenant Status).
const (
	PlanStatusActive     = "active"
	PlanStatusIncomplete = "incomplete"
	PlanStatusPastDue    = "past_due"
	PlanStatusCanceled   = "canceled"
)

// ProcessedStripeEvent records webhook events already handled (idempotency guard).
type ProcessedStripeEvent struct {
	ID          uint      `gorm:"primaryKey"`
	EventID     string    `gorm:"column:event_id;uniqueIndex;size:255"`
	ProcessedAt time.Time `gorm:"autoCreateTime"`
}

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

// API key modes
const (
	AnthropicModeClaudeCodePassthrough = "CLAUDE_CODE_PASSTHROUGH"
	AnthropicModeAPIBYOK               = "ANTHROPIC_API_BYOK"
	OpenAIModeCodexPassthrough         = "OPENAI_CODEX_PASSTHROUGH"
	OpenAIModeAPIBYOK                  = "OPENAI_API_BYOK"
)

// IsBYOKMode returns true when the mode is a Bring Your Own Key mode (any provider).
func IsBYOKMode(mode string) bool {
	return mode == AnthropicModeAPIBYOK || mode == OpenAIModeAPIBYOK
}

// IsPassthroughMode returns true when the mode is a passthrough mode (any provider).
func IsPassthroughMode(mode string) bool {
	return mode == AnthropicModeClaudeCodePassthrough || mode == OpenAIModeCodexPassthrough
}

// SupportedAPIKeyModes maps provider → valid modes.
var SupportedAPIKeyModes = map[string][]string{
	"anthropic": {AnthropicModeClaudeCodePassthrough, AnthropicModeAPIBYOK},
	"openai":    {OpenAIModeCodexPassthrough, OpenAIModeAPIBYOK},
}

// ValidAPIKeyMode returns true when mode is valid for provider.
func ValidAPIKeyMode(provider, mode string) bool {
	modes, ok := SupportedAPIKeyModes[provider]
	if !ok {
		return false
	}
	for _, m := range modes {
		if m == mode {
			return true
		}
	}
	return false
}

// APIKey is the machine-to-machine key used by the claude-code agent
// to authenticate with the TokenGate gateway. Scoped to a tenant.
type APIKey struct {
	ID         uint           `gorm:"primaryKey"`
	TenantID   uint           `gorm:"index"`
	KeyID      string         `gorm:"uniqueIndex;size:64"`
	Label      string
	Salt       []byte
	SecretHash []byte
	Scopes     pq.StringArray `gorm:"type:text[]"`
	Provider   string         `gorm:"not null;default:anthropic"`
	Mode       string         `gorm:"not null;default:CLAUDE_CODE_PASSTHROUGH"`
	Revoked    bool
	ExpiresAt  *time.Time
	LastSeenAt *time.Time
	CreatedAt  time.Time
}

// ProviderKey stores an upstream LLM provider API key using envelope encryption.
// The key is encrypted with a per-record DEK; the DEK is encrypted with the master key.
type ProviderKey struct {
	ID           uint      `gorm:"primaryKey"`
	TenantID     uint      `gorm:"index"`
	Provider     string    // "anthropic" | "openai"
	Label        string
	EncryptedKey []byte    `gorm:"column:encrypted_key"`
	KeyNonce     []byte    `gorm:"column:key_nonce"`
	EncryptedDEK []byte    `gorm:"column:encrypted_dek"`
	DEKNonce     []byte    `gorm:"column:dek_nonce"`
	Revoked      bool
	CreatedAt    time.Time
}

// TenantProviderSettings records which ProviderKey is active for a given tenant+provider pair.
// PolicyVersion is a monotonically-increasing counter bumped on every Activate or Rotate call.
// It is stored in the Redis TPS cache entry so that after a rotation any pod can detect a
// stale cached active_key_id without a DB round trip.
type TenantProviderSettings struct {
	ID            uint      `gorm:"primaryKey"`
	TenantID      uint      `gorm:"uniqueIndex:idx_tenant_provider"`
	Provider      string    `gorm:"uniqueIndex:idx_tenant_provider"`
	ActiveKeyID   uint
	PolicyVersion int       `gorm:"default:1"`
	UpdatedAt     time.Time
}

// UsageLog records one LLM request reported by the claude-code agent.
// request_id is an idempotency key; duplicate submissions are ignored.
// api_key_fingerprint is derived from the client's X-Api-Key header ("ak:<sha256-hex>")
// and used for stable cross-session audit attribution; raw key values are never stored.
type UsageLog struct {
	ID                  uint            `gorm:"primaryKey"                              json:"id"`
	TenantID            uint            `gorm:"index"                                   json:"tenant_id"`
	UserID              string          `gorm:"index"                                   json:"user_id"`
	Provider            string          `                                               json:"provider"`
	Model               string          `                                               json:"model"`
	PromptTokens        int64           `gorm:"column:prompt_tokens"                    json:"prompt_tokens"`
	CompletionTokens    int64           `gorm:"column:completion_tokens"                json:"completion_tokens"`
	CacheCreationTokens int64           `gorm:"column:cache_creation_tokens"            json:"cache_creation_tokens"`
	CacheReadTokens     int64           `gorm:"column:cache_read_tokens"                json:"cache_read_tokens"`
	ReasoningTokens     int64           `gorm:"column:reasoning_tokens"                 json:"reasoning_tokens"`
	Cost                decimal.Decimal `gorm:"type:numeric(20,8)"                      json:"cost"`
	RequestID           string          `gorm:"column:request_id;uniqueIndex"           json:"request_id"`
	APIKeyFingerprint   string          `gorm:"column:api_key_fingerprint;size:75;index" json:"api_key_fingerprint"`
	CreatedAt           time.Time       `gorm:"index"                                   json:"created_at"`
	APIUsageBilled      bool            `gorm:"column:api_usage_billed;not null;default:false;index" json:"api_usage_billed"`
}
