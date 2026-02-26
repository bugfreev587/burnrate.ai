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
	Status               string     `gorm:"default:active"`        // active | suspended
	Plan                 string     `gorm:"default:free"` // free | pro | team | business
	MaxAPIKeys           int        `gorm:"default:1"`    // plan-derived; use GetPlanLimits for enforcement
	StripeCustomerID     string     `gorm:"column:stripe_customer_id;index"`
	StripeSubscriptionID string     `gorm:"column:stripe_subscription_id"`
	PlanStatus           string     `gorm:"column:plan_status;default:active"`
	CurrentPeriodEnd     *time.Time `gorm:"column:current_period_end"`
	BillingEmail         string     `gorm:"column:billing_email"`
	PendingPlan          string     `gorm:"column:pending_plan"`           // scheduled downgrade target (empty = no pending change)
	PlanEffectiveAt      *time.Time `gorm:"column:plan_effective_at"`      // when the pending plan change takes effect
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

// Auth method constants
const (
	AuthMethodBrowserOAuth = "BROWSER_OAUTH"
	AuthMethodBYOK         = "BYOK"
)

// Billing mode constants
const (
	BillingModeMonthlySubscription = "MONTHLY_SUBSCRIPTION"
	BillingModeAPIUsage            = "API_USAGE"
)

// ValidAuthBillingCombo returns true when the auth_method + billing_mode combination
// is valid for the given provider. BYOK + MONTHLY_SUBSCRIPTION is never valid.
func ValidAuthBillingCombo(provider, authMethod, billingMode string) bool {
	if provider != "anthropic" && provider != "openai" {
		return false
	}
	if authMethod != AuthMethodBrowserOAuth && authMethod != AuthMethodBYOK {
		return false
	}
	if billingMode != BillingModeMonthlySubscription && billingMode != BillingModeAPIUsage {
		return false
	}
	// BYOK implies API-level access — monthly subscription makes no sense.
	if authMethod == AuthMethodBYOK && billingMode == BillingModeMonthlySubscription {
		return false
	}
	return true
}

// IsBillableMode returns true when the billing mode indicates billable API usage.
func IsBillableMode(billingMode string) bool {
	return billingMode == BillingModeAPIUsage
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
	Provider    string         `gorm:"not null;default:anthropic"`
	AuthMethod  string         `gorm:"not null;default:BROWSER_OAUTH"`
	BillingMode string         `gorm:"not null;default:MONTHLY_SUBSCRIPTION"`
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
	KeyID               string          `gorm:"column:key_id;size:64;index"              json:"key_id"`
	APIKeyFingerprint   string          `gorm:"column:api_key_fingerprint;size:75;index" json:"api_key_fingerprint"`
	CreatedAt           time.Time       `gorm:"index"                                   json:"created_at"`
	APIUsageBilled      bool            `gorm:"column:api_usage_billed;not null;default:false;index" json:"api_usage_billed"`
	KeyLabel            string          `gorm:"-"                                                   json:"key_label"`
}
