package pricing

import (
	"log"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// modelEntry describes a model and its pricing per 1M tokens.
type modelEntry struct {
	name   string
	prices map[string]string // price_type -> $/1M
}

// anthropicModels is the canonical list of Anthropic models and their pricing.
// Both dated IDs and aliases are listed so the resolver can match either form.
var anthropicModels = []modelEntry{
	// Claude 3 family
	{name: "claude-3-opus-20240229", prices: map[string]string{
		models.PriceTypeInput: "15.00", models.PriceTypeOutput: "75.00",
		models.PriceTypeCacheCreation: "18.75", models.PriceTypeCacheRead: "1.50",
	}},
	{name: "claude-3-5-sonnet-20241022", prices: map[string]string{
		models.PriceTypeInput: "3.00", models.PriceTypeOutput: "15.00",
		models.PriceTypeCacheCreation: "3.75", models.PriceTypeCacheRead: "0.30",
	}},
	{name: "claude-3-5-haiku-20241022", prices: map[string]string{
		models.PriceTypeInput: "0.80", models.PriceTypeOutput: "4.00",
		models.PriceTypeCacheCreation: "1.00", models.PriceTypeCacheRead: "0.08",
	}},
	// Claude 4
	{name: "claude-sonnet-4-20250514", prices: map[string]string{
		models.PriceTypeInput: "3.00", models.PriceTypeOutput: "15.00",
		models.PriceTypeCacheCreation: "3.75", models.PriceTypeCacheRead: "0.30",
	}},
	{name: "claude-sonnet-4-0", prices: map[string]string{
		models.PriceTypeInput: "3.00", models.PriceTypeOutput: "15.00",
		models.PriceTypeCacheCreation: "3.75", models.PriceTypeCacheRead: "0.30",
	}},
	{name: "claude-opus-4-20250514", prices: map[string]string{
		models.PriceTypeInput: "15.00", models.PriceTypeOutput: "75.00",
		models.PriceTypeCacheCreation: "18.75", models.PriceTypeCacheRead: "1.50",
	}},
	{name: "claude-opus-4-0", prices: map[string]string{
		models.PriceTypeInput: "15.00", models.PriceTypeOutput: "75.00",
		models.PriceTypeCacheCreation: "18.75", models.PriceTypeCacheRead: "1.50",
	}},
	// Claude 4.1
	{name: "claude-opus-4-1-20250805", prices: map[string]string{
		models.PriceTypeInput: "15.00", models.PriceTypeOutput: "75.00",
		models.PriceTypeCacheCreation: "18.75", models.PriceTypeCacheRead: "1.50",
	}},
	{name: "claude-opus-4-1", prices: map[string]string{
		models.PriceTypeInput: "15.00", models.PriceTypeOutput: "75.00",
		models.PriceTypeCacheCreation: "18.75", models.PriceTypeCacheRead: "1.50",
	}},
	// Claude 4.5
	{name: "claude-haiku-4-5-20251001", prices: map[string]string{
		models.PriceTypeInput: "1.00", models.PriceTypeOutput: "5.00",
		models.PriceTypeCacheCreation: "1.25", models.PriceTypeCacheRead: "0.10",
	}},
	{name: "claude-haiku-4-5", prices: map[string]string{
		models.PriceTypeInput: "1.00", models.PriceTypeOutput: "5.00",
		models.PriceTypeCacheCreation: "1.25", models.PriceTypeCacheRead: "0.10",
	}},
	{name: "claude-sonnet-4-5-20250929", prices: map[string]string{
		models.PriceTypeInput: "3.00", models.PriceTypeOutput: "15.00",
		models.PriceTypeCacheCreation: "3.75", models.PriceTypeCacheRead: "0.30",
	}},
	{name: "claude-sonnet-4-5", prices: map[string]string{
		models.PriceTypeInput: "3.00", models.PriceTypeOutput: "15.00",
		models.PriceTypeCacheCreation: "3.75", models.PriceTypeCacheRead: "0.30",
	}},
	{name: "claude-opus-4-5-20251101", prices: map[string]string{
		models.PriceTypeInput: "5.00", models.PriceTypeOutput: "25.00",
		models.PriceTypeCacheCreation: "6.25", models.PriceTypeCacheRead: "0.50",
	}},
	{name: "claude-opus-4-5", prices: map[string]string{
		models.PriceTypeInput: "5.00", models.PriceTypeOutput: "25.00",
		models.PriceTypeCacheCreation: "6.25", models.PriceTypeCacheRead: "0.50",
	}},
	// Claude 4.6
	{name: "claude-sonnet-4-6", prices: map[string]string{
		models.PriceTypeInput: "3.00", models.PriceTypeOutput: "15.00",
		models.PriceTypeCacheCreation: "3.75", models.PriceTypeCacheRead: "0.30",
	}},
	{name: "claude-opus-4-6", prices: map[string]string{
		models.PriceTypeInput: "5.00", models.PriceTypeOutput: "25.00",
		models.PriceTypeCacheCreation: "6.25", models.PriceTypeCacheRead: "0.50",
	}},
}

// openaiModels is the canonical list of OpenAI models and their pricing.
var openaiModels = []modelEntry{
	// GPT-4o family
	{name: "gpt-4o", prices: map[string]string{models.PriceTypeInput: "2.50", models.PriceTypeOutput: "10.00"}},
	{name: "gpt-4o-mini", prices: map[string]string{models.PriceTypeInput: "0.15", models.PriceTypeOutput: "0.60"}},
	{name: "gpt-4-turbo", prices: map[string]string{models.PriceTypeInput: "10.00", models.PriceTypeOutput: "30.00"}},
	{name: "gpt-3.5-turbo", prices: map[string]string{models.PriceTypeInput: "0.50", models.PriceTypeOutput: "1.50"}},
	// GPT-4.1 family
	{name: "gpt-4.1", prices: map[string]string{models.PriceTypeInput: "2.00", models.PriceTypeOutput: "8.00"}},
	{name: "gpt-4.1-mini", prices: map[string]string{models.PriceTypeInput: "0.40", models.PriceTypeOutput: "1.60"}},
	{name: "gpt-4.1-nano", prices: map[string]string{models.PriceTypeInput: "0.10", models.PriceTypeOutput: "0.40"}},
	// GPT-5 family
	{name: "gpt-5", prices: map[string]string{models.PriceTypeInput: "1.25", models.PriceTypeOutput: "10.00"}},
	{name: "gpt-5-mini", prices: map[string]string{models.PriceTypeInput: "0.25", models.PriceTypeOutput: "2.00"}},
	{name: "gpt-5-nano", prices: map[string]string{models.PriceTypeInput: "0.05", models.PriceTypeOutput: "0.40"}},
	{name: "gpt-5-codex", prices: map[string]string{models.PriceTypeInput: "1.25", models.PriceTypeOutput: "10.00"}},
	{name: "gpt-5-codex-mini", prices: map[string]string{models.PriceTypeInput: "0.25", models.PriceTypeOutput: "2.00"}},
	// GPT-5.1 family
	{name: "gpt-5.1", prices: map[string]string{models.PriceTypeInput: "1.25", models.PriceTypeOutput: "10.00"}},
	{name: "gpt-5.1-codex", prices: map[string]string{models.PriceTypeInput: "1.25", models.PriceTypeOutput: "10.00"}},
	{name: "gpt-5.1-codex-mini", prices: map[string]string{models.PriceTypeInput: "0.25", models.PriceTypeOutput: "2.00"}},
	{name: "gpt-5.1-codex-max", prices: map[string]string{models.PriceTypeInput: "1.25", models.PriceTypeOutput: "10.00"}},
	// GPT-5.2 family
	{name: "gpt-5.2", prices: map[string]string{models.PriceTypeInput: "1.75", models.PriceTypeOutput: "14.00"}},
	{name: "gpt-5.2-codex", prices: map[string]string{models.PriceTypeInput: "1.75", models.PriceTypeOutput: "14.00"}},
	{name: "gpt-5.2-pro", prices: map[string]string{models.PriceTypeInput: "21.00", models.PriceTypeOutput: "168.00"}},
	// GPT-5.3 family
	{name: "gpt-5.3-codex", prices: map[string]string{models.PriceTypeInput: "1.75", models.PriceTypeOutput: "14.00"}},
	{name: "gpt-5.3-codex-spark", prices: map[string]string{models.PriceTypeInput: "1.75", models.PriceTypeOutput: "14.00"}},
	// O-series reasoning models
	{name: "o1", prices: map[string]string{models.PriceTypeInput: "15.00", models.PriceTypeOutput: "60.00"}},
	{name: "o1-mini", prices: map[string]string{models.PriceTypeInput: "3.00", models.PriceTypeOutput: "12.00"}},
	{name: "o1-pro", prices: map[string]string{models.PriceTypeInput: "150.00", models.PriceTypeOutput: "600.00"}},
	{name: "o3", prices: map[string]string{models.PriceTypeInput: "2.00", models.PriceTypeOutput: "8.00"}},
	{name: "o3-mini", prices: map[string]string{models.PriceTypeInput: "1.10", models.PriceTypeOutput: "4.40"}},
	{name: "o4-mini", prices: map[string]string{models.PriceTypeInput: "1.10", models.PriceTypeOutput: "4.40"}},
	// ChatGPT models
	{name: "chatgpt-4o-latest", prices: map[string]string{models.PriceTypeInput: "5.00", models.PriceTypeOutput: "15.00"}},
}

// SeedInitialData seeds providers, models, and pricing into the DB.
// It is a no-op if providers already exist.
func SeedInitialData(db *gorm.DB) error {
	var count int64
	db.Model(&models.Provider{}).Count(&count)
	if count > 0 {
		log.Println("pricing: seed data already present, skipping")
		return nil
	}

	effectiveFrom := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	type providerEntry struct {
		name        string
		displayName string
		models      []modelEntry
	}

	seed := []providerEntry{
		{
			name:        "anthropic",
			displayName: "Anthropic",
			models:      anthropicModels,
		},
		{
			name:        "openai",
			displayName: "OpenAI",
			models:      openaiModels,
		},
		{
			name:        "google",
			displayName: "Google",
			models: []modelEntry{
				{name: "gemini-1.5-pro", prices: map[string]string{models.PriceTypeInput: "1.25", models.PriceTypeOutput: "5.00"}},
				{name: "gemini-1.5-flash", prices: map[string]string{models.PriceTypeInput: "0.075", models.PriceTypeOutput: "0.30"}},
				{name: "gemini-2.0-flash", prices: map[string]string{models.PriceTypeInput: "0.10", models.PriceTypeOutput: "0.40"}},
			},
		},
		{
			name:        "azure",
			displayName: "Azure OpenAI",
			models: []modelEntry{
				{name: "gpt-4o", prices: map[string]string{models.PriceTypeInput: "2.50", models.PriceTypeOutput: "10.00"}},
			},
		},
		{
			name:        "mistral",
			displayName: "Mistral AI",
			models: []modelEntry{
				{name: "mistral-large", prices: map[string]string{models.PriceTypeInput: "2.00", models.PriceTypeOutput: "6.00"}},
			},
		},
	}

	for _, pe := range seed {
		provider := models.Provider{
			Name:        pe.name,
			DisplayName: pe.displayName,
			Currency:    "USD",
		}
		if err := db.Create(&provider).Error; err != nil {
			return err
		}

		for _, me := range pe.models {
			modelDef := models.ModelDef{
				ProviderID:      provider.ID,
				ModelName:       me.name,
				BillingUnitType: "token",
			}
			if err := db.Create(&modelDef).Error; err != nil {
				return err
			}

			for priceType, priceStr := range me.prices {
				price, err := decimal.NewFromString(priceStr)
				if err != nil {
					return err
				}
				mp := models.ModelPricing{
					ModelID:       modelDef.ID,
					PriceType:     priceType,
					PricePerUnit:  price,
					UnitSize:      1_000_000,
					EffectiveFrom: effectiveFrom,
				}
				if err := db.Create(&mp).Error; err != nil {
					return err
				}
			}
		}
	}

	log.Println("pricing: seed data inserted successfully")
	return nil
}

// EnsureMissingModels adds any models that are not yet in the DB
// and corrects pricing for existing models whose prices have changed.
// Safe to call on every startup — it only inserts/updates where needed.
func EnsureMissingModels(db *gorm.DB) error {
	providers := []struct {
		name   string
		models []modelEntry
	}{
		{"anthropic", anthropicModels},
		{"openai", openaiModels},
	}

	effectiveFrom := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, p := range providers {
		if err := ensureModelsForProvider(db, p.name, p.models, effectiveFrom); err != nil {
			return err
		}
	}
	return nil
}

// ensureModelsForProvider adds/updates models for a single provider.
func ensureModelsForProvider(db *gorm.DB, providerName string, entries []modelEntry, effectiveFrom time.Time) error {
	var provider models.Provider
	if err := db.Where("name = ?", providerName).First(&provider).Error; err != nil {
		// Provider doesn't exist yet (fresh DB); SeedInitialData will handle it.
		return nil
	}

	for _, me := range entries {
		var modelDef models.ModelDef
		result := db.Where("provider_id = ? AND model_name = ?", provider.ID, me.name).First(&modelDef)

		if result.Error == gorm.ErrRecordNotFound {
			// Model missing — create it with pricing.
			modelDef = models.ModelDef{
				ProviderID:      provider.ID,
				ModelName:       me.name,
				BillingUnitType: "token",
			}
			if err := db.Create(&modelDef).Error; err != nil {
				return err
			}
			for priceType, priceStr := range me.prices {
				price, _ := decimal.NewFromString(priceStr)
				mp := models.ModelPricing{
					ModelID:       modelDef.ID,
					PriceType:     priceType,
					PricePerUnit:  price,
					UnitSize:      1_000_000,
					EffectiveFrom: effectiveFrom,
				}
				if err := db.Create(&mp).Error; err != nil {
					return err
				}
			}
			log.Printf("pricing: added missing model %s (%s)", me.name, providerName)
			continue
		} else if result.Error != nil {
			return result.Error
		}

		// Model exists — check each price type and update if different.
		for priceType, priceStr := range me.prices {
			wantPrice, _ := decimal.NewFromString(priceStr)

			var existing models.ModelPricing
			err := db.Where("model_id = ? AND price_type = ? AND (effective_to IS NULL OR effective_to > NOW())",
				modelDef.ID, priceType).
				Order("effective_from DESC").
				First(&existing).Error

			if err == gorm.ErrRecordNotFound {
				// Price type row missing — insert it.
				mp := models.ModelPricing{
					ModelID:       modelDef.ID,
					PriceType:     priceType,
					PricePerUnit:  wantPrice,
					UnitSize:      1_000_000,
					EffectiveFrom: effectiveFrom,
				}
				if err := db.Create(&mp).Error; err != nil {
					return err
				}
				log.Printf("pricing: added %s pricing for %s", priceType, me.name)
			} else if err != nil {
				return err
			} else if !existing.PricePerUnit.Equal(wantPrice) {
				// Price changed — update in place.
				existing.PricePerUnit = wantPrice
				if err := db.Save(&existing).Error; err != nil {
					return err
				}
				log.Printf("pricing: updated %s %s pricing to %s", me.name, priceType, priceStr)
			}
		}
	}

	return nil
}
