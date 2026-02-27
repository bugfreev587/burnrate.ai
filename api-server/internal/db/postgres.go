package db

import (
	"fmt"
	"log"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/pricing"
)

type PostgresDB struct {
	db *gorm.DB
}

func InitPostgres(dsn string) (*PostgresDB, error) {
	gormLogger := logger.New(
		log.New(log.Writer(), "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             500 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	var db *gorm.DB
	var err error

	for i := 1; i <= 10; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			PrepareStmt: true,
			Logger:      gormLogger,
		})
		if err == nil {
			if sqlDB, e := db.DB(); e == nil && sqlDB.Ping() == nil {
				break
			}
		}
		slog.Warn("postgres_not_ready", "attempt", i, "max_attempts", 10, "error", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres connection failed: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(60 * time.Minute)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	// One-time: drop legacy budget unique indexes that lack the provider column.
	// GORM AutoMigrate won't drop/recreate existing indexes, so we must do it explicitly.
	for _, idx := range []string{"idx_budget_tenant_period", "idx_budget_tenant_scope"} {
		db.Exec(fmt.Sprintf("DROP INDEX IF EXISTS %s", idx))
	}

	// One-time: drop unused slug column and its unique index from tenants table.
	db.Exec("DROP INDEX IF EXISTS idx_tenants_slug")
	db.Exec("ALTER TABLE tenants DROP COLUMN IF EXISTS slug")

	// Auto-migrate schema (in dependency order)
	if err := db.AutoMigrate(
		&models.Tenant{},
		&models.User{},
		&models.APIKey{},
		&models.ProviderKey{},
		&models.TenantProviderSettings{},
		&models.UsageLog{},
		&models.Provider{},
		&models.ModelDef{},
		&models.ModelPricing{},
		&models.ContractPricing{},
		&models.PricingMarkup{},
		&models.CostLedger{},
		&models.BudgetLimit{},
		&models.PricingConfig{},
		&models.PricingConfigRate{},
		&models.APIKeyConfig{},
		&models.RateLimit{},
		&models.ProcessedStripeEvent{},
		&models.AuditReport{},
		&models.NotificationChannel{},
		&models.GatewayEvent{},
	); err != nil {
		return nil, fmt.Errorf("automigrate: %w", err)
	}

	// One-time: reset latency_ms data recorded before the Redis caching + pipeline
	// optimisation so dashboards start fresh with accurate numbers.
	// Uses a sentinel row in processed_stripe_events to ensure it runs only once.
	var alreadyRan bool
	db.Raw(`SELECT EXISTS (SELECT 1 FROM processed_stripe_events WHERE event_id = 'migration:reset_latency_data_v2')`).Scan(&alreadyRan)
	if !alreadyRan {
		db.Exec(`UPDATE usage_logs SET latency_ms = 0 WHERE latency_ms > 0`)
		db.Exec(`DELETE FROM gateway_events`)
		db.Exec(`INSERT INTO processed_stripe_events (event_id, processed_at) VALUES ('migration:reset_latency_data_v2', NOW())`)
		slog.Info("one_time_migration: reset stale latency data (v2 – post caching/pipeline)")
	}

	// Composite indexes for metrics queries.
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_usage_logs_latency ON usage_logs (tenant_id, created_at, latency_ms) WHERE latency_ms > 0`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_gateway_events_type ON gateway_events (tenant_id, event_type, created_at)`)

	if err := pricing.SeedInitialData(db); err != nil {
		return nil, fmt.Errorf("seed pricing data: %w", err)
	}

	if err := pricing.EnsureMissingModels(db); err != nil {
		return nil, fmt.Errorf("ensure missing models: %w", err)
	}

	// One-time migration: populate auth_method + billing_mode from legacy mode column.
	// Only run if the legacy "mode" column still exists.
	var modeColExists bool
	db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'api_keys' AND column_name = 'mode')`).Scan(&modeColExists)
	if modeColExists {
		// Passthrough modes → BROWSER_OAUTH + MONTHLY_SUBSCRIPTION
		db.Exec(`UPDATE api_keys SET auth_method = 'BROWSER_OAUTH', billing_mode = 'MONTHLY_SUBSCRIPTION' WHERE mode IN ('CLAUDE_CODE_PASSTHROUGH', 'OPENAI_CODEX_PASSTHROUGH')`)

		// BYOK modes → BYOK + API_USAGE
		if res := db.Exec(`UPDATE api_keys SET auth_method = 'BYOK', billing_mode = 'API_USAGE' WHERE mode IN ('ANTHROPIC_API_BYOK', 'OPENAI_API_BYOK', 'API_BYOK')`); res.Error != nil {
			slog.Error("migrate_api_key_auth_billing_failed", "error", res.Error)
		} else if res.RowsAffected > 0 {
			slog.Info("migrate_api_key_auth_billing", "rows_updated", res.RowsAffected)
		}

		// Drop the legacy mode column now that auth_method + billing_mode are populated.
		db.Exec("ALTER TABLE api_keys DROP COLUMN IF EXISTS mode")
	}

	// One-time backfill: copy final_cost from cost_ledgers into usage_logs
	// where cost was never set (zero). Safe to run repeatedly — the WHERE
	// clause ensures already-backfilled rows are skipped.
	backfillSQL := `
		UPDATE usage_logs
		SET cost = cl.final_cost
		FROM cost_ledgers cl
		WHERE usage_logs.request_id = cl.idempotency_key
		  AND usage_logs.cost = 0
		  AND cl.final_cost > 0`
	if res := db.Exec(backfillSQL); res.Error != nil {
		slog.Error("backfill_usage_logs_costs_failed", "error", res.Error)
	} else if res.RowsAffected > 0 {
		slog.Info("backfill_usage_logs_costs", "rows_updated", res.RowsAffected)
	}

	return &PostgresDB{db: db}, nil
}

// NewFromDB wraps an existing *gorm.DB instance. Used in tests to avoid
// the full InitPostgres bootstrap (retry loop, one-time migrations, seeding).
func NewFromDB(gormDB *gorm.DB) *PostgresDB {
	return &PostgresDB{db: gormDB}
}

func (p *PostgresDB) GetDB() *gorm.DB {
	return p.db
}

func (p *PostgresDB) Close() {
	if sqlDB, err := p.db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
