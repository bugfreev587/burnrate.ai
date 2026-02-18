package pricing

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// UsageEvent is the input to the pricing pipeline.
type UsageEvent struct {
	Provider            string
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	ReasoningTokens     int64
	RequestCount        int
	Timestamp           time.Time
	TenantID            uint
	UserID              string
	IdempotencyKey      string
	APIKeyRef           string // api_keys.key_id (UUID) — used for key-level pricing config lookup
}

// PricePoint is a snapshotted price for one dimension, serialised into the ledger.
type PricePoint struct {
	PricePerUnit string `json:"price_per_unit"` // decimal as string
	UnitSize     int64  `json:"unit_size"`
	PricingID    uint   `json:"pricing_id"`
	Source       string `json:"source"` // "contract" | "standard"
}

// PricingSnapshot captures the exact prices used to compute a cost, for audit.
type PricingSnapshot struct {
	Provider    string                `json:"provider"`
	Model       string                `json:"model"`
	PricePoints map[string]PricePoint `json:"price_points"` // keyed by price_type
	MarkupPct   string                `json:"markup_pct"`
	SnapshotAt  time.Time             `json:"snapshot_at"`
}

// PricingResult is returned by PricingEngine.Process.
type PricingResult struct {
	BaseCost     decimal.Decimal
	MarkupAmount decimal.Decimal
	FinalCost    decimal.Decimal
	Snapshot     PricingSnapshot
	ProviderID   uint
	ModelID      uint
}

// ErrBudgetExceeded is returned when a blocking budget limit is breached.
type ErrBudgetExceeded struct {
	TenantID     uint
	LimitAmount  decimal.Decimal
	CurrentSpend decimal.Decimal
	Period       string
}

func (e *ErrBudgetExceeded) Error() string {
	return fmt.Sprintf(
		"budget exceeded for tenant %d: period=%s limit=%s current=%s",
		e.TenantID, e.Period, e.LimitAmount.String(), e.CurrentSpend.String(),
	)
}

// ErrModelNotFound is returned by Resolver when the provider+model pair is unknown.
type ErrModelNotFound struct {
	Provider string
	Model    string
}

func (e *ErrModelNotFound) Error() string {
	return fmt.Sprintf("model not found in catalog: provider=%s model=%s", e.Provider, e.Model)
}

// resolvedPrice holds the effective price for one billing dimension.
type resolvedPrice struct {
	PricePerUnit decimal.Decimal
	UnitSize     int64
	PricingID    uint
	Source       string // "contract" | "standard"
}

// ResolvedPrices is the output of PricingResolver.Resolve.
type ResolvedPrices struct {
	ProviderID uint
	ModelID    uint
	Prices     map[string]resolvedPrice // keyed by price_type
}
