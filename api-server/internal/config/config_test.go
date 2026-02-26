package config

import (
	"os"
	"testing"
)

func TestApplyEnvOverrides(t *testing.T) {
	// Save and restore all env vars we'll modify
	envVars := []string{
		"API_KEY_PEPPER", "POSTGRES_DB_URL", "PROVIDER_KEY_ENCRYPTION_KEY",
		"STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET",
		"STRIPE_PRICE_PRO_MONTHLY", "STRIPE_PRICE_TEAM_MONTHLY", "STRIPE_PRICE_BUSINESS_MONTHLY",
		"STRIPE_PORTAL_CONFIGURATION_ID", "CORS_ORIGINS",
	}
	saved := make(map[string]string)
	for _, k := range envVars {
		saved[k] = os.Getenv(k)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	})

	// Clear all env vars first
	for _, k := range envVars {
		os.Unsetenv(k)
	}

	t.Run("no env vars — config unchanged", func(t *testing.T) {
		cfg := &Config{
			Security: SecurityCfg{APIKeyPepper: "original"},
			Postgres: PostgresCfg{DSN: "original-dsn"},
		}
		applyEnvOverrides(cfg)
		if cfg.Security.APIKeyPepper != "original" {
			t.Errorf("APIKeyPepper changed unexpectedly")
		}
		if cfg.Postgres.DSN != "original-dsn" {
			t.Errorf("DSN changed unexpectedly")
		}
	})

	t.Run("API_KEY_PEPPER override", func(t *testing.T) {
		os.Setenv("API_KEY_PEPPER", "env-pepper")
		defer os.Unsetenv("API_KEY_PEPPER")
		cfg := &Config{}
		applyEnvOverrides(cfg)
		if cfg.Security.APIKeyPepper != "env-pepper" {
			t.Errorf("APIKeyPepper = %q, want %q", cfg.Security.APIKeyPepper, "env-pepper")
		}
	})

	t.Run("POSTGRES_DB_URL override", func(t *testing.T) {
		os.Setenv("POSTGRES_DB_URL", "postgres://env")
		defer os.Unsetenv("POSTGRES_DB_URL")
		cfg := &Config{Postgres: PostgresCfg{DSN: "yaml-dsn"}}
		applyEnvOverrides(cfg)
		if cfg.Postgres.DSN != "postgres://env" {
			t.Errorf("DSN = %q, want %q", cfg.Postgres.DSN, "postgres://env")
		}
	})

	t.Run("STRIPE env vars override", func(t *testing.T) {
		os.Setenv("STRIPE_SECRET_KEY", "sk_test")
		os.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_test")
		os.Setenv("STRIPE_PRICE_PRO_MONTHLY", "price_pro")
		os.Setenv("STRIPE_PRICE_TEAM_MONTHLY", "price_team")
		os.Setenv("STRIPE_PRICE_BUSINESS_MONTHLY", "price_biz")
		os.Setenv("STRIPE_PORTAL_CONFIGURATION_ID", "bpc_123")
		defer func() {
			os.Unsetenv("STRIPE_SECRET_KEY")
			os.Unsetenv("STRIPE_WEBHOOK_SECRET")
			os.Unsetenv("STRIPE_PRICE_PRO_MONTHLY")
			os.Unsetenv("STRIPE_PRICE_TEAM_MONTHLY")
			os.Unsetenv("STRIPE_PRICE_BUSINESS_MONTHLY")
			os.Unsetenv("STRIPE_PORTAL_CONFIGURATION_ID")
		}()

		cfg := &Config{}
		applyEnvOverrides(cfg)

		if cfg.Stripe.SecretKey != "sk_test" {
			t.Errorf("SecretKey = %q, want %q", cfg.Stripe.SecretKey, "sk_test")
		}
		if cfg.Stripe.WebhookSecret != "whsec_test" {
			t.Errorf("WebhookSecret = %q, want %q", cfg.Stripe.WebhookSecret, "whsec_test")
		}
		if cfg.Stripe.PriceProMonthly != "price_pro" {
			t.Errorf("PriceProMonthly = %q, want %q", cfg.Stripe.PriceProMonthly, "price_pro")
		}
		if cfg.Stripe.PriceTeamMonthly != "price_team" {
			t.Errorf("PriceTeamMonthly = %q, want %q", cfg.Stripe.PriceTeamMonthly, "price_team")
		}
		if cfg.Stripe.PriceBusinessMonthly != "price_biz" {
			t.Errorf("PriceBusinessMonthly = %q, want %q", cfg.Stripe.PriceBusinessMonthly, "price_biz")
		}
		if cfg.Stripe.PortalConfigurationID != "bpc_123" {
			t.Errorf("PortalConfigurationID = %q, want %q", cfg.Stripe.PortalConfigurationID, "bpc_123")
		}
	})

	t.Run("CORS_ORIGINS comma-separated", func(t *testing.T) {
		os.Setenv("CORS_ORIGINS", "https://a.com, https://b.com , https://c.com")
		defer os.Unsetenv("CORS_ORIGINS")
		cfg := &Config{}
		applyEnvOverrides(cfg)
		if len(cfg.Server.CORSOrigins) != 3 {
			t.Fatalf("CORSOrigins length = %d, want 3", len(cfg.Server.CORSOrigins))
		}
		if cfg.Server.CORSOrigins[0] != "https://a.com" {
			t.Errorf("CORSOrigins[0] = %q, want %q", cfg.Server.CORSOrigins[0], "https://a.com")
		}
		if cfg.Server.CORSOrigins[1] != "https://b.com" {
			t.Errorf("CORSOrigins[1] = %q, want %q", cfg.Server.CORSOrigins[1], "https://b.com")
		}
		if cfg.Server.CORSOrigins[2] != "https://c.com" {
			t.Errorf("CORSOrigins[2] = %q, want %q", cfg.Server.CORSOrigins[2], "https://c.com")
		}
	})

	t.Run("CORS_ORIGINS wildcard", func(t *testing.T) {
		os.Setenv("CORS_ORIGINS", "*")
		defer os.Unsetenv("CORS_ORIGINS")
		cfg := &Config{}
		applyEnvOverrides(cfg)
		if len(cfg.Server.CORSOrigins) != 1 || cfg.Server.CORSOrigins[0] != "*" {
			t.Errorf("CORSOrigins = %v, want [*]", cfg.Server.CORSOrigins)
		}
	})

	t.Run("PROVIDER_KEY_ENCRYPTION_KEY override", func(t *testing.T) {
		os.Setenv("PROVIDER_KEY_ENCRYPTION_KEY", "enc-key-123")
		defer os.Unsetenv("PROVIDER_KEY_ENCRYPTION_KEY")
		cfg := &Config{}
		applyEnvOverrides(cfg)
		if cfg.Security.ProviderKeyEncryptionKey != "enc-key-123" {
			t.Errorf("ProviderKeyEncryptionKey = %q, want %q", cfg.Security.ProviderKeyEncryptionKey, "enc-key-123")
		}
	})
}
