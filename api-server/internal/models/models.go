package models

import (
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

// Tenant is the top-level multi-tenant boundary.
// Every API key, provider key, and usage log belongs to exactly one tenant.
type Tenant struct {
	ID                   uint `gorm:"primaryKey"`
	Name                 string
	Status               string     `gorm:"default:active"`            // active | suspended
	Plan                 string     `gorm:"default:free"`              // free | pro | team | business
	DefaultProjectID     *uint      `gorm:"column:default_project_id"` // FK to projects.id; nullable until first project is created
	StripeCustomerID     string     `gorm:"column:stripe_customer_id;index"`
	StripeSubscriptionID string     `gorm:"column:stripe_subscription_id"`
	PlanStatus           string     `gorm:"column:plan_status;default:active"`
	CurrentPeriodEnd     *time.Time `gorm:"column:current_period_end"`
	BillingEmail         string     `gorm:"column:billing_email"`
	PendingPlan          string     `gorm:"column:pending_plan"`      // scheduled downgrade target (empty = no pending change)
	PlanEffectiveAt      *time.Time `gorm:"column:plan_effective_at"` // when the pending plan change takes effect
	CreatedAt            time.Time
}

// User represents a dashboard user synced from Clerk on first sign-in.
// Users can belong to multiple tenants via TenantMembership.
type User struct {
	ID                       string `gorm:"primaryKey;type:text"` // Clerk user ID e.g. user_2lXYZ…
	Email                    string `gorm:"uniqueIndex"`
	Name                     string
	Status                   string `gorm:"default:active"` // active | suspended | pending
	DismissedIntegrationHint bool   `gorm:"column:dismissed_integration_hint;default:false"`
	DismissedAvatarHint      bool   `gorm:"column:dismissed_avatar_hint;default:false"`
	CreatedAt                time.Time
}

// TenantMembership associates a user with a tenant and defines their org-level role.
type TenantMembership struct {
	TenantID  uint   `gorm:"primaryKey;autoIncrement:false"`
	UserID    string `gorm:"primaryKey;type:text;autoIncrement:false"`
	OrgRole   string `gorm:"not null;default:viewer"` // owner | admin | editor | viewer
	Status    string `gorm:"not null;default:active"` // active | suspended | pending
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Project represents a logical grouping within a tenant. API keys are bound to projects.
type Project struct {
	ID          uint   `gorm:"primaryKey"`
	TenantID    uint   `gorm:"index;uniqueIndex:idx_project_tenant_name"`
	Name        string `gorm:"size:128;uniqueIndex:idx_project_tenant_name"`
	Description string
	Status      string `gorm:"not null;default:active"` // active | archived
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ProjectMembership associates a user with a project and defines their project-level role.
type ProjectMembership struct {
	ProjectID   uint   `gorm:"primaryKey;autoIncrement:false"`
	UserID      string `gorm:"primaryKey;type:text;autoIncrement:false"`
	ProjectRole string `gorm:"not null;default:project_viewer"` // project_admin | project_editor | project_viewer
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// APIKeyProviderKeyBinding allows a per-API-key override of the tenant's default provider key.
type APIKeyProviderKeyBinding struct {
	APIKeyID      uint `gorm:"primaryKey;autoIncrement:false"`
	ProviderKeyID uint `gorm:"not null"`
	CreatedAt     time.Time
}

// AuditLog records security-relevant mutations for compliance and debugging.
type AuditLog struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	TenantID      uint      `gorm:"index;index:idx_audit_tenant_created" json:"tenant_id"`
	ActorUserID   string    `gorm:"size:255" json:"actor_user_id"`
	ActorAPIKeyID string    `gorm:"size:64" json:"actor_api_key_id"`
	Action        string    `gorm:"size:64;index" json:"action"`         // e.g. "API_KEY.CREATED", "MEMBER.INVITED"
	ResourceType  string    `gorm:"size:64;index;index:idx_audit_resource" json:"resource_type"`  // e.g. "api_key", "project", "membership"
	ResourceID    string    `gorm:"size:255;index:idx_audit_resource" json:"resource_id"`
	Category      string    `gorm:"size:32;index" json:"category"`
	ActorType     string    `gorm:"size:16;default:user" json:"actor_type"`
	UserAgent     string    `gorm:"size:512" json:"user_agent,omitempty"`
	Success       bool      `gorm:"not null;default:true" json:"success"`
	IPAddress     string    `gorm:"size:45" json:"ip_address"`
	BeforeJSON    string    `gorm:"type:jsonb" json:"before_json,omitempty"`
	AfterJSON     string    `gorm:"type:jsonb" json:"after_json,omitempty"`
	Metadata      string    `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt     time.Time `gorm:"index;index:idx_audit_tenant_created" json:"created_at"`
}

// Role constants (org-level)
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// Project role constants
const (
	ProjectRoleAdmin  = "project_admin"
	ProjectRoleEditor = "project_editor"
	ProjectRoleViewer = "project_viewer"
)

// Status constants
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusPending   = "pending"
)

// Project status constants
const (
	ProjectStatusActive   = "active"
	ProjectStatusArchived = "archived"
)

// Audit action constants (DOMAIN.VERB format)
const (
	AuditAPIKeyCreated  = "API_KEY.CREATED"
	AuditAPIKeyRevoked  = "API_KEY.REVOKED"

	AuditProviderKeyCreated   = "PROVIDER_KEY.CREATED"
	AuditProviderKeyRevoked   = "PROVIDER_KEY.REVOKED"
	AuditProviderKeyActivated = "PROVIDER_KEY.ACTIVATED"
	AuditProviderKeyRotated   = "PROVIDER_KEY.ROTATED"

	AuditMemberInvited     = "MEMBER.INVITED"
	AuditMemberRemoved     = "MEMBER.REMOVED"
	AuditMemberRoleChanged = "MEMBER.ROLE_CHANGED"
	AuditMemberSuspended   = "MEMBER.SUSPENDED"
	AuditMemberUnsuspended = "MEMBER.UNSUSPENDED"
	AuditMemberPromoted    = "MEMBER.PROMOTED"
	AuditMemberDemoted     = "MEMBER.DEMOTED"

	AuditProjectCreated           = "PROJECT.CREATED"
	AuditProjectUpdated           = "PROJECT.UPDATED"
	AuditProjectDeleted           = "PROJECT.DELETED"
	AuditProjectMemberAdded       = "PROJECT_MEMBER.ADDED"
	AuditProjectMemberRoleChanged = "PROJECT_MEMBER.ROLE_CHANGED"
	AuditProjectMemberRemoved     = "PROJECT_MEMBER.REMOVED"

	AuditBudgetCreated = "BUDGET.CREATED"
	AuditBudgetUpdated = "BUDGET.UPDATED"
	AuditBudgetDeleted = "BUDGET.DELETED"

	AuditRateLimitCreated = "RATE_LIMIT.CREATED"
	AuditRateLimitUpdated = "RATE_LIMIT.UPDATED"
	AuditRateLimitDeleted = "RATE_LIMIT.DELETED"

	AuditBillingCheckoutInitiated = "BILLING.CHECKOUT_INITIATED"
	AuditBillingPlanChanged      = "BILLING.PLAN_CHANGED"
	AuditBillingDowngraded       = "BILLING.DOWNGRADED"
	AuditBillingDowngradeCanceled = "BILLING.DOWNGRADE_CANCELED"
	AuditBillingWebhookProcessed = "BILLING.WEBHOOK_PROCESSED"

	AuditOwnershipTransferred = "OWNERSHIP.TRANSFERRED"
	AuditSettingsUpdated      = "SETTINGS.UPDATED"
	AuditAccountDeleted       = "ACCOUNT.DELETED"

	AuditNotificationChannelCreated = "NOTIFICATION_CHANNEL.CREATED"
	AuditNotificationChannelUpdated = "NOTIFICATION_CHANNEL.UPDATED"
	AuditNotificationChannelDeleted = "NOTIFICATION_CHANNEL.DELETED"

	AuditPricingConfigCreated    = "PRICING_CONFIG.CREATED"
	AuditPricingConfigDeleted    = "PRICING_CONFIG.DELETED"
	AuditPricingConfigAssigned   = "PRICING_CONFIG.ASSIGNED"
	AuditPricingConfigUnassigned = "PRICING_CONFIG.UNASSIGNED"

	AuditSuperAdminPlanChanged   = "SUPERADMIN.PLAN_CHANGED"
	AuditSuperAdminStatusChanged = "SUPERADMIN.STATUS_CHANGED"
)

// Audit log category constants
const (
	AuditCategoryAccess  = "ACCESS"
	AuditCategoryTeam    = "TEAM"
	AuditCategoryProject = "PROJECT"
	AuditCategoryConfig  = "CONFIG"
	AuditCategoryBilling = "BILLING"
	AuditCategoryOwner   = "OWNER"
	AuditCategoryAdmin   = "ADMIN"
)

// Audit actor type constants
const (
	AuditActorUser       = "user"
	AuditActorSystem     = "system"
	AuditActorSuperAdmin = "superadmin"
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

func ProjectRoleLevel(role string) int {
	switch role {
	case ProjectRoleAdmin:
		return 3
	case ProjectRoleEditor:
		return 2
	case ProjectRoleViewer:
		return 1
	default:
		return 0
	}
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
// to authenticate with the TokenGate gateway. Scoped to a tenant and bound to a project.
type APIKey struct {
	ID              uint   `gorm:"primaryKey"`
	TenantID        uint   `gorm:"index"`
	ProjectID       uint   `gorm:"not null;index"` // FK to projects.id; every key belongs to exactly one project
	KeyID           string `gorm:"uniqueIndex;size:64"`
	Label           string
	Salt            []byte
	SecretHash      []byte
	Scopes          pq.StringArray `gorm:"type:text[]"`
	Provider        string         `gorm:"not null;default:anthropic"`
	AuthMethod      string         `gorm:"not null;default:BROWSER_OAUTH"`
	BillingMode     string         `gorm:"not null;default:MONTHLY_SUBSCRIPTION"`
	ModelAllowlist  *string        `gorm:"type:jsonb"` // JSON array of allowed model strings, nil = all
	CreatedByUserID string         `gorm:"size:255"`   // Clerk user ID of the creator
	Revoked         bool
	ExpiresAt       *time.Time
	LastSeenAt      *time.Time
	CreatedAt       time.Time
}

// ProviderKey stores an upstream LLM provider API key using envelope encryption.
// The key is encrypted with a per-record DEK; the DEK is encrypted with the master key.
type ProviderKey struct {
	ID           uint   `gorm:"primaryKey"`
	TenantID     uint   `gorm:"index"`
	Provider     string // "anthropic" | "openai"
	Label        string
	EncryptedKey []byte `gorm:"column:encrypted_key"`
	KeyNonce     []byte `gorm:"column:key_nonce"`
	EncryptedDEK []byte `gorm:"column:encrypted_dek"`
	DEKNonce     []byte `gorm:"column:dek_nonce"`
	Revoked      bool
	CreatedAt    time.Time
}

// TenantProviderSettings records which ProviderKey is active for a given tenant+provider pair.
// PolicyVersion is a monotonically-increasing counter bumped on every Activate or Rotate call.
// It is stored in the Redis TPS cache entry so that after a rotation any pod can detect a
// stale cached active_key_id without a DB round trip.
type TenantProviderSettings struct {
	ID            uint   `gorm:"primaryKey"`
	TenantID      uint   `gorm:"uniqueIndex:idx_tenant_provider"`
	Provider      string `gorm:"uniqueIndex:idx_tenant_provider"`
	ActiveKeyID   uint
	PolicyVersion int `gorm:"default:1"`
	UpdatedAt     time.Time
}

// MaskKey returns a masked version of an API key for display purposes.
// For keys >= 22 chars: first 15 + "..." + last 4. Otherwise: first 4 + "..." + last 4.
// Returns empty string for empty input.
func MaskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) >= 22 {
		return key[:15] + "..." + key[len(key)-4:]
	}
	if len(key) <= 8 {
		return key[:1] + "..." + key[len(key)-1:]
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// UsageLog records one LLM request reported by the claude-code agent.
// request_id is an idempotency key; duplicate submissions are ignored.
type UsageLog struct {
	ID                  uint            `gorm:"primaryKey"                              json:"id"`
	TenantID            uint            `gorm:"index"                                   json:"tenant_id"`
	ProjectID           uint            `gorm:"index"                                   json:"project_id"`
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
	ProviderKeyHint     string          `gorm:"column:provider_key_hint;size:30"         json:"provider_key_hint"`
	CreatedAt           time.Time       `gorm:"index"                                   json:"created_at"`
	LatencyMs           int64           `gorm:"column:latency_ms;default:0"                         json:"latency_ms"`
	APIUsageBilled      bool            `gorm:"column:api_usage_billed;not null;default:false;index" json:"api_usage_billed"`
	KeyLabel            string          `gorm:"-"                                                   json:"key_label"`
}

// ModelGroupConfig persists a model group's routing configuration to the database.
// Each model group maps a virtual model name to one or more provider deployments.
type ModelGroupConfig struct {
	ID          uint      `gorm:"primaryKey"                                       json:"id"`
	TenantID    uint      `gorm:"index;uniqueIndex:idx_mg_tenant_name"             json:"tenant_id"`
	Name        string    `gorm:"size:128;uniqueIndex:idx_mg_tenant_name"          json:"name"`
	Strategy    string    `gorm:"size:32;not null;default:fallback"                json:"strategy"` // fallback | round-robin | lowest-latency | cost-optimized
	Description string    `gorm:"size:512"                                         json:"description,omitempty"`
	Enabled     bool      `gorm:"not null;default:true"                            json:"enabled"`
	CreatedAt   time.Time `                                                        json:"created_at"`
	UpdatedAt   time.Time `                                                        json:"updated_at"`

	Deployments []ModelGroupDeployment `gorm:"foreignKey:ModelGroupID;constraint:OnDelete:CASCADE" json:"deployments,omitempty"`
}

func (ModelGroupConfig) TableName() string { return "model_group_configs" }

// ModelGroupDeployment is a single provider deployment within a model group.
type ModelGroupDeployment struct {
	ID              uint    `gorm:"primaryKey"                          json:"id"`
	ModelGroupID    uint    `gorm:"index;not null"                     json:"model_group_id"`
	Provider        string  `gorm:"size:32;not null"                   json:"provider"`  // openai | anthropic | deepseek | mistral
	Model           string  `gorm:"size:128;not null"                  json:"model"`     // actual model name at provider
	ProviderKeyID   *uint   `gorm:"column:provider_key_id"             json:"provider_key_id,omitempty"` // FK to provider_keys.id; nil = use tenant's active key
	Priority        int     `gorm:"not null;default:0"                 json:"priority"`
	Weight          int     `gorm:"not null;default:1"                 json:"weight"`
	CostPer1KInput  float64 `gorm:"column:cost_per_1k_input;default:0" json:"cost_per_1k_input"`
	CostPer1KOutput float64 `gorm:"column:cost_per_1k_output;default:0" json:"cost_per_1k_output"`
	Enabled         bool    `gorm:"not null;default:true"              json:"enabled"`
	CreatedAt       time.Time `                                        json:"created_at"`
}

func (ModelGroupDeployment) TableName() string { return "model_group_deployments" }

// Audit action constants for model group management
const (
	AuditModelGroupCreated = "MODEL_GROUP.CREATED"
	AuditModelGroupUpdated = "MODEL_GROUP.UPDATED"
	AuditModelGroupDeleted = "MODEL_GROUP.DELETED"
)

// GatewayEvent records blocked requests (rate limit 429, budget exceeded 402)
// that return early before usage events are published, so they never appear in usage_logs.
type GatewayEvent struct {
	ID         uint      `gorm:"primaryKey"                           json:"id"`
	TenantID   uint      `gorm:"index"                                json:"tenant_id"`
	KeyID      string    `gorm:"column:key_id;size:64;index"          json:"key_id"`
	Provider   string    `gorm:"size:32"                              json:"provider"`
	Model      string    `gorm:"size:128"                             json:"model"`
	EventType  string    `gorm:"size:32;index"                        json:"event_type"` // "rate_limit_429" | "budget_exceeded_402"
	StatusCode int       `                                            json:"status_code"`
	LatencyMs  int64     `gorm:"column:latency_ms;default:0"          json:"latency_ms"`
	Details    string    `gorm:"type:text"                            json:"details"`
	CreatedAt  time.Time `gorm:"index"                                json:"created_at"`
}
