package pricing

import (
	"log"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

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

	type modelEntry struct {
		name   string
		prices map[string]string // price_type -> $/1M
	}

	type providerEntry struct {
		name        string
		displayName string
		models      []modelEntry
	}

	seed := []providerEntry{
		{
			name:        "anthropic",
			displayName: "Anthropic",
			models: []modelEntry{
				{
					name: "claude-3-5-sonnet-20241022",
					prices: map[string]string{
						models.PriceTypeInput:         "3.00",
						models.PriceTypeOutput:        "15.00",
						models.PriceTypeCacheCreation: "3.75",
						models.PriceTypeCacheRead:     "0.30",
					},
				},
				{
					name: "claude-3-5-haiku-20241022",
					prices: map[string]string{
						models.PriceTypeInput:         "0.80",
						models.PriceTypeOutput:        "4.00",
						models.PriceTypeCacheCreation: "1.00",
						models.PriceTypeCacheRead:     "0.08",
					},
				},
				{
					name: "claude-3-opus-20240229",
					prices: map[string]string{
						models.PriceTypeInput:         "15.00",
						models.PriceTypeOutput:        "75.00",
						models.PriceTypeCacheCreation: "18.75",
						models.PriceTypeCacheRead:     "1.50",
					},
				},
				{
					name: "claude-sonnet-4-6",
					prices: map[string]string{
						models.PriceTypeInput:         "3.00",
						models.PriceTypeOutput:        "15.00",
						models.PriceTypeCacheCreation: "3.75",
						models.PriceTypeCacheRead:     "0.30",
					},
				},
				{
					name: "claude-opus-4-6",
					prices: map[string]string{
						models.PriceTypeInput:         "15.00",
						models.PriceTypeOutput:        "75.00",
						models.PriceTypeCacheCreation: "18.75",
						models.PriceTypeCacheRead:     "1.50",
					},
				},
			},
		},
		{
			name:        "openai",
			displayName: "OpenAI",
			models: []modelEntry{
				{name: "gpt-4o", prices: map[string]string{models.PriceTypeInput: "2.50", models.PriceTypeOutput: "10.00"}},
				{name: "gpt-4o-mini", prices: map[string]string{models.PriceTypeInput: "0.15", models.PriceTypeOutput: "0.60"}},
				{name: "gpt-4-turbo", prices: map[string]string{models.PriceTypeInput: "10.00", models.PriceTypeOutput: "30.00"}},
				{name: "o1", prices: map[string]string{models.PriceTypeInput: "15.00", models.PriceTypeOutput: "60.00"}},
				{name: "o1-mini", prices: map[string]string{models.PriceTypeInput: "3.00", models.PriceTypeOutput: "12.00"}},
			},
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
