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
}

// BudgetLimit enforces per-tenant spend limits for a given period.
type BudgetLimit struct {
	ID             uint            `gorm:"primaryKey"`
	TenantID       uint            `gorm:"uniqueIndex:idx_budget_tenant_period"`
	PeriodType     string          `gorm:"uniqueIndex:idx_budget_tenant_period"` // monthly|weekly|daily
	LimitAmount    decimal.Decimal `gorm:"type:numeric(20,8)"`
	AlertThreshold decimal.Decimal `gorm:"type:numeric(5,2);default:80"` // percentage
	Action         string          `gorm:"default:alert"`                // alert|block
	CreatedAt      time.Time
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
	BudgetActionAlert = "alert"
	BudgetActionBlock = "block"
)
