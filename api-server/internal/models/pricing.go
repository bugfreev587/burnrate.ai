package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// Provider represents an LLM provider (anthropic, openai, google, azure, mistral).
type Provider struct {
	ID          uint      `gorm:"primaryKey"`
	Name        string    `gorm:"uniqueIndex;size:64"`
	DisplayName string
	Currency    string    `gorm:"default:USD"`
	CreatedAt   time.Time
}

// ModelDef represents an LLM model belonging to a provider.
type ModelDef struct {
	ID              uint      `gorm:"primaryKey"`
	ProviderID      uint      `gorm:"uniqueIndex:idx_model_provider_name"`
	ModelName       string    `gorm:"size:128;uniqueIndex:idx_model_provider_name"`
	BillingUnitType string    `gorm:"default:token"` // token|image|second|request
	CreatedAt       time.Time
}

// ModelPricing stores versioned official pricing for a model.
type ModelPricing struct {
	ID            uint            `gorm:"primaryKey"`
	ModelID       uint            `gorm:"index"`
	PriceType     string          // input|output|cache_creation|cache_read|reasoning
	PricePerUnit  decimal.Decimal `gorm:"type:numeric(20,8)"`
	UnitSize      int64           `gorm:"default:1000000"` // per 1M tokens
	EffectiveFrom time.Time       `gorm:"index"`
	EffectiveTo   *time.Time
}

// ContractPricing stores enterprise pricing overrides scoped to a tenant.
type ContractPricing struct {
	ID            uint            `gorm:"primaryKey"`
	TenantID      uint            `gorm:"index"`
	ModelID       uint            `gorm:"index"`
	PriceType     string
	PriceOverride decimal.Decimal `gorm:"type:numeric(20,8)"`
	UnitSize      int64           `gorm:"default:1000000"`
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
}

// PricingMarkup is the monetization lever applied per tenant, with optional
// provider/model granularity. Higher Priority = more specific, wins over lower.
type PricingMarkup struct {
	ID            uint            `gorm:"primaryKey"`
	TenantID      uint            `gorm:"index"`
	ProviderID    *uint           // NULL = all providers
	ModelID       *uint           // NULL = all models
	Percentage    decimal.Decimal `gorm:"type:numeric(8,4)"`
	Priority      int             `gorm:"default:0"` // higher = more specific
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
}

// CostLedger is an immutable financial record for each processed usage event.
type CostLedger struct {
	ID                  uint            `gorm:"primaryKey"`
	TenantID            uint            `gorm:"index"`
	UserID              string          `gorm:"index"`
	ProviderID          uint
	ModelID             uint
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	ReasoningTokens     int64
	BaseCost            decimal.Decimal `gorm:"type:numeric(20,8)"`
	MarkupAmount        decimal.Decimal `gorm:"type:numeric(20,8)"`
	FinalCost           decimal.Decimal `gorm:"type:numeric(20,8)"`
	PricingSnapshot     string          `gorm:"type:jsonb"`  // snapshotted prices for audit
	IdempotencyKey      string          `gorm:"uniqueIndex"` // = UsageLog.request_id
	Timestamp           time.Time       `gorm:"index"`
	CreatedAt           time.Time
	APIUsageBilled      bool            `gorm:"column:api_usage_billed;not null;default:false;index" json:"api_usage_billed"`
}

// BudgetLimit enforces spend limits scoped to a tenant, optionally further
// scoped to a specific API key. ScopeType = "account" applies to the whole
// tenant; ScopeType = "api_key" applies to one key (ScopeID = key_id).
type BudgetLimit struct {
	ID             uint            `gorm:"primaryKey"`
	TenantID       uint            `gorm:"uniqueIndex:idx_budget_tenant_scope"`
	ScopeType      string          `gorm:"uniqueIndex:idx_budget_tenant_scope;size:16;default:account"` // account|api_key
	ScopeID        string          `gorm:"uniqueIndex:idx_budget_tenant_scope;size:64"`                 // "" for account, key_id for api_key
	PeriodType     string          `gorm:"uniqueIndex:idx_budget_tenant_scope"`                         // monthly|weekly|daily
	Provider       string          `gorm:"uniqueIndex:idx_budget_tenant_scope;size:32;default:''"`      // "" = all, "anthropic", "openai"
	LimitAmount    decimal.Decimal `gorm:"type:numeric(20,8)"`
	AlertThreshold decimal.Decimal `gorm:"type:numeric(5,2);default:80"` // percentage, e.g. 80 = warn at 80%
	Action         string          `gorm:"default:alert"`                // alert|block
	CreatedAt      time.Time
}

// PricingConfig is a named set of model pricing overrides scoped to a tenant.
// A config is associated with at most one API key via APIKeyConfig.
type PricingConfig struct {
	ID          uint      `gorm:"primaryKey"`
	TenantID    uint      `gorm:"index"`
	Name        string    `gorm:"size:128"`
	Description string
	CreatedAt   time.Time
}

// PricingConfigRate is a single price override belonging to a PricingConfig.
type PricingConfigRate struct {
	ID           uint            `gorm:"primaryKey"`
	ConfigID     uint            `gorm:"index"`
	ModelID      uint
	PriceType    string          // input|output|cache_creation|cache_read|reasoning
	PricePerUnit decimal.Decimal `gorm:"type:numeric(20,8)"`
	UnitSize     int64           `gorm:"default:1000000"`
}

// APIKeyConfig associates one PricingConfig with one API key (by key_id UUID).
// Deleted automatically when the key is revoked.
type APIKeyConfig struct {
	ID        uint      `gorm:"primaryKey"`
	TenantID  uint      `gorm:"index"`
	KeyID     string    `gorm:"uniqueIndex;size:64"` // references api_keys.key_id
	ConfigID  uint      `gorm:"index"`
	CreatedAt time.Time
}

// Price type constants
const (
	PriceTypeInput         = "input"
	PriceTypeOutput        = "output"
	PriceTypeCacheCreation = "cache_creation"
	PriceTypeCacheRead     = "cache_read"
	PriceTypeReasoning     = "reasoning"
)

// Period type constants
const (
	PeriodMonthly = "monthly"
	PeriodWeekly  = "weekly"
	PeriodDaily   = "daily"
)

// Budget action constants
const (
	BudgetActionAlert      = "alert"
	BudgetActionBlock      = "block"
	BudgetActionAlertBlock = "alert_block" // alert at threshold AND block at limit
)

// Budget scope constants
const (
	BudgetScopeAccount = "account"
	BudgetScopeAPIKey  = "api_key"
	BudgetScopeProject = "project"
)

// RateLimit defines a tenant-aware, model-scoped rate limit.
// The unique index ensures at most one limit per (tenant, provider, model, scope, metric).
type RateLimit struct {
	ID            uint      `gorm:"primaryKey"`
	TenantID      uint      `gorm:"uniqueIndex:idx_ratelimit_unique"`
	Provider      string    `gorm:"uniqueIndex:idx_ratelimit_unique;size:32;default:''"` // "" = all
	Model         string    `gorm:"uniqueIndex:idx_ratelimit_unique;size:128;default:''"` // "" = all
	ScopeType     string    `gorm:"uniqueIndex:idx_ratelimit_unique;size:16;default:account"` // account|api_key
	ScopeID       string    `gorm:"uniqueIndex:idx_ratelimit_unique;size:64;default:''"`
	Metric        string    `gorm:"uniqueIndex:idx_ratelimit_unique;size:16"` // rpm|itpm|otpm
	LimitValue    int64     `gorm:"not null"`
	WindowSeconds int       `gorm:"not null;default:60"`
	Enabled       bool      `gorm:"not null;default:true"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Rate limit metric constants
const (
	RateLimitMetricRPM  = "rpm"
	RateLimitMetricITPM = "itpm"
	RateLimitMetricOTPM = "otpm"
)

// Rate limit scope constants
const (
	RateLimitScopeAccount = "account"
	RateLimitScopeAPIKey  = "api_key"
	RateLimitScopeProject = "project"
)
