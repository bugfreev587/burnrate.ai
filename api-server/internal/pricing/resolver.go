package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/models"
)

const resolverCacheTTL = 5 * time.Minute

// PricingResolver resolves effective prices for a usage event.
type PricingResolver struct {
	db  *gorm.DB
	rdb *redis.Client
}

// NewPricingResolver returns a new PricingResolver.
func NewPricingResolver(db *gorm.DB, rdb *redis.Client) *PricingResolver {
	return &PricingResolver{db: db, rdb: rdb}
}

// Resolve returns the effective prices for a usage event.
// Contract overrides take precedence over standard versioned pricing.
// Returns ErrModelNotFound if the provider+model pair is unknown.
func (r *PricingResolver) Resolve(ctx context.Context, event UsageEvent) (*ResolvedPrices, error) {
	provider, err := r.resolveProvider(ctx, event.Provider)
	if err != nil {
		return nil, err
	}

	modelDef, err := r.resolveModel(ctx, provider.ID, event.Model)
	if err != nil {
		return nil, err
	}

	ts := event.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	resolved := &ResolvedPrices{
		ProviderID: provider.ID,
		ModelID:    modelDef.ID,
		Prices:     make(map[string]resolvedPrice),
	}

	// 1. Key-level pricing config (highest priority) — set by admin via PricingConfig
	if event.APIKeyRef != "" {
		var keyConfig models.APIKeyConfig
		if r.db.WithContext(ctx).Where("key_id = ?", event.APIKeyRef).First(&keyConfig).Error == nil {
			var rates []models.PricingConfigRate
			r.db.WithContext(ctx).Where("config_id = ? AND model_id = ?", keyConfig.ConfigID, modelDef.ID).Find(&rates)
			for _, rate := range rates {
				unitSize := rate.UnitSize
				if unitSize == 0 {
					unitSize = 1_000_000
				}
				resolved.Prices[rate.PriceType] = resolvedPrice{
					PricePerUnit: rate.PricePerUnit,
					UnitSize:     unitSize,
					PricingID:    rate.ID,
					Source:       "key_config",
				}
			}
		}
	}

	// 2. Load contract overrides (tenant-scoped) for price_types not already set
	var contracts []models.ContractPricing
	r.db.WithContext(ctx).
		Where("tenant_id = ? AND model_id = ? AND effective_from <= ? AND (effective_to IS NULL OR effective_to > ?)",
			event.TenantID, modelDef.ID, ts, ts).
		Find(&contracts)

	for _, cp := range contracts {
		if _, alreadySet := resolved.Prices[cp.PriceType]; alreadySet {
			continue // key_config wins
		}
		unitSize := cp.UnitSize
		if unitSize == 0 {
			unitSize = 1_000_000
		}
		resolved.Prices[cp.PriceType] = resolvedPrice{
			PricePerUnit: cp.PriceOverride,
			UnitSize:     unitSize,
			PricingID:    cp.ID,
			Source:       "contract",
		}
	}

	// 3. Load standard versioned pricing for any price_types not covered by contract
	var standardPrices []models.ModelPricing
	r.db.WithContext(ctx).
		Where("model_id = ? AND effective_from <= ? AND (effective_to IS NULL OR effective_to > ?)",
			modelDef.ID, ts, ts).
		Find(&standardPrices)

	for _, sp := range standardPrices {
		if _, alreadySet := resolved.Prices[sp.PriceType]; alreadySet {
			continue // contract override wins
		}
		unitSize := sp.UnitSize
		if unitSize == 0 {
			unitSize = 1_000_000
		}
		resolved.Prices[sp.PriceType] = resolvedPrice{
			PricePerUnit: sp.PricePerUnit,
			UnitSize:     unitSize,
			PricingID:    sp.ID,
			Source:       "standard",
		}
	}

	return resolved, nil
}

// resolveProvider looks up Provider by name (case-insensitive), cached in Redis.
func (r *PricingResolver) resolveProvider(ctx context.Context, providerName string) (*models.Provider, error) {
	cacheKey := fmt.Sprintf("pprovider:%s", strings.ToLower(providerName))

	if r.rdb != nil {
		if cached, err := r.rdb.Get(ctx, cacheKey).Bytes(); err == nil {
			var p models.Provider
			if json.Unmarshal(cached, &p) == nil {
				return &p, nil
			}
		}
	}

	var provider models.Provider
	result := r.db.WithContext(ctx).
		Where("LOWER(name) = LOWER(?)", providerName).
		First(&provider)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, &ErrModelNotFound{Provider: providerName, Model: ""}
		}
		return nil, result.Error
	}

	if r.rdb != nil {
		if b, err := json.Marshal(provider); err == nil {
			r.rdb.Set(ctx, cacheKey, b, resolverCacheTTL)
		}
	}

	return &provider, nil
}

// resolveModel looks up ModelDef by provider ID and model name (case-insensitive), cached in Redis.
func (r *PricingResolver) resolveModel(ctx context.Context, providerID uint, modelName string) (*models.ModelDef, error) {
	cacheKey := fmt.Sprintf("pmodel:%d:%s", providerID, strings.ToLower(modelName))

	if r.rdb != nil {
		if cached, err := r.rdb.Get(ctx, cacheKey).Bytes(); err == nil {
			var m models.ModelDef
			if json.Unmarshal(cached, &m) == nil {
				return &m, nil
			}
		}
	}

	var modelDef models.ModelDef
	result := r.db.WithContext(ctx).
		Where("provider_id = ? AND LOWER(model_name) = LOWER(?)", providerID, modelName).
		First(&modelDef)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, &ErrModelNotFound{Provider: fmt.Sprintf("provider_id=%d", providerID), Model: modelName}
		}
		return nil, result.Error
	}

	if r.rdb != nil {
		if b, err := json.Marshal(modelDef); err == nil {
			r.rdb.Set(ctx, cacheKey, b, resolverCacheTTL)
		}
	}

	return &modelDef, nil
}

