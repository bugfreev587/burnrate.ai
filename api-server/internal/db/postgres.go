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

	// One-time: replace api_key_fingerprint with provider_key_hint on usage_logs.
	// GORM AutoMigrate adds provider_key_hint; we just need to drop the old column and its index.
	db.Exec("DROP INDEX IF EXISTS idx_usage_logs_api_key_fingerprint")
	db.Exec("ALTER TABLE usage_logs DROP COLUMN IF EXISTS api_key_fingerprint")

	// ─── RBAC v2 one-time migration ────────────────────────────────────────────
	// Must run BEFORE AutoMigrate because AutoMigrate will add NOT NULL constraints
	// to api_keys.project_id which would fail on existing NULL rows.
	migrateRBACv2(db)

	// Auto-migrate schema (in dependency order)
	if err := db.AutoMigrate(
		&models.Tenant{},
		&models.User{},
		&models.TenantMembership{},
		&models.Project{},
		&models.ProjectMembership{},
		&models.APIKey{},
		&models.APIKeyProviderKeyBinding{},
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
		&models.AuditLog{},
	); err != nil {
		return nil, fmt.Errorf("automigrate: %w", err)
	}

	// One-time: reset latency data that included cold-cache-miss samples before
	// the >200ms suppression was added. Cold misses are now recorded as 0 and
	// automatically excluded from percentile metrics.
	var alreadyRan bool
	db.Raw(`SELECT EXISTS (SELECT 1 FROM processed_stripe_events WHERE event_id = 'migration:reset_latency_data_v3')`).Scan(&alreadyRan)
	if !alreadyRan {
		db.Exec(`UPDATE usage_logs SET latency_ms = 0 WHERE latency_ms > 0`)
		db.Exec(`DELETE FROM gateway_events`)
		db.Exec(`INSERT INTO processed_stripe_events (event_id, processed_at) VALUES ('migration:reset_latency_data_v3', NOW())`)
		slog.Info("one_time_migration: reset stale latency data (v3 – cold-miss suppression)")
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

// migrateRBACv2 handles the one-time data migration from the legacy single-tenant
// RBAC model to the new multi-tenant model with projects.
//
// It must run BEFORE AutoMigrate because AutoMigrate adds NOT NULL constraints
// to columns like api_keys.project_id that would fail on existing NULL rows.
//
// Steps:
//  1. Check if migration already ran (idempotency guard).
//  2. Create projects + tenant_memberships tables if they don't exist yet.
//  3. Migrate users.tenant_id/role → tenant_memberships (if old columns exist).
//  4. Create a default project for every tenant that has api_keys.
//  5. Backfill api_keys.project_id and usage_logs.project_id.
//  6. Drop legacy columns (users.tenant_id, users.role, tenants.max_api_keys).
func migrateRBACv2(db *gorm.DB) {
	// Idempotency: skip if already completed.
	var alreadyRan bool
	db.Raw(`SELECT EXISTS (SELECT 1 FROM processed_stripe_events WHERE event_id = 'migration:rbac_v2')`).Scan(&alreadyRan)
	if alreadyRan {
		return
	}

	// Check if there's actually existing data to migrate.
	var apiKeysTableExists bool
	db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'api_keys')`).Scan(&apiKeysTableExists)
	if !apiKeysTableExists {
		// Fresh database — no migration needed, AutoMigrate will create everything.
		// Still mark as done so we don't re-check on every startup.
		db.Exec(`INSERT INTO processed_stripe_events (event_id, processed_at) VALUES ('migration:rbac_v2', NOW()) ON CONFLICT DO NOTHING`)
		return
	}

	slog.Info("rbac_v2_migration: starting one-time data migration")

	// ── Step 1: Ensure projects table exists (raw DDL, before AutoMigrate) ──
	db.Exec(`
		CREATE TABLE IF NOT EXISTS projects (
			id BIGSERIAL PRIMARY KEY,
			tenant_id BIGINT NOT NULL,
			name VARCHAR(128) NOT NULL,
			description TEXT DEFAULT '',
			status VARCHAR(16) NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(tenant_id, name)
		)
	`)

	// ── Step 2: Ensure tenant_memberships table exists ──
	db.Exec(`
		CREATE TABLE IF NOT EXISTS tenant_memberships (
			tenant_id BIGINT NOT NULL,
			user_id TEXT NOT NULL,
			org_role VARCHAR(16) NOT NULL DEFAULT 'viewer',
			status VARCHAR(16) NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (tenant_id, user_id)
		)
	`)

	// ── Step 3: Ensure project_memberships table exists ──
	db.Exec(`
		CREATE TABLE IF NOT EXISTS project_memberships (
			project_id BIGINT NOT NULL,
			user_id TEXT NOT NULL,
			project_role VARCHAR(32) NOT NULL DEFAULT 'project_viewer',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (project_id, user_id)
		)
	`)

	// ── Step 4: Migrate users.tenant_id + users.role → tenant_memberships ──
	var hasTenantIDCol bool
	db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'tenant_id')`).Scan(&hasTenantIDCol)
	if hasTenantIDCol {
		slog.Info("rbac_v2_migration: migrating users.tenant_id → tenant_memberships")
		db.Exec(`
			INSERT INTO tenant_memberships (tenant_id, user_id, org_role, status, created_at, updated_at)
			SELECT u.tenant_id, u.id,
				COALESCE(NULLIF(u.role, ''), 'viewer'),
				u.status,
				u.created_at,
				NOW()
			FROM users u
			WHERE u.tenant_id IS NOT NULL
			  AND u.tenant_id != 0
			ON CONFLICT (tenant_id, user_id) DO NOTHING
		`)
	}

	// ── Step 5: Create default project for every tenant that has api_keys ──
	// Also handle tenants that exist but might not have api_keys yet.
	slog.Info("rbac_v2_migration: creating default projects for existing tenants")
	db.Exec(`
		INSERT INTO projects (tenant_id, name, description, status, created_at, updated_at)
		SELECT t.id, 'Default', 'Default project', 'active', NOW(), NOW()
		FROM tenants t
		WHERE NOT EXISTS (SELECT 1 FROM projects p WHERE p.tenant_id = t.id)
		ON CONFLICT (tenant_id, name) DO NOTHING
	`)

	// ── Step 6: Set tenants.default_project_id ──
	// Add the column first if it doesn't exist (AutoMigrate hasn't run yet).
	db.Exec(`ALTER TABLE tenants ADD COLUMN IF NOT EXISTS default_project_id BIGINT`)
	db.Exec(`
		UPDATE tenants t
		SET default_project_id = p.id
		FROM projects p
		WHERE p.tenant_id = t.id
		  AND p.name = 'Default'
		  AND t.default_project_id IS NULL
	`)

	// ── Step 7: Add project_id column to api_keys if missing, then backfill ──
	db.Exec(`ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS project_id BIGINT DEFAULT 0`)
	db.Exec(`ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS model_allowlist JSONB`)
	db.Exec(`ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS created_by_user_id VARCHAR(255) DEFAULT ''`)
	db.Exec(`
		UPDATE api_keys ak
		SET project_id = p.id
		FROM projects p
		WHERE p.tenant_id = ak.tenant_id
		  AND p.name = 'Default'
		  AND (ak.project_id IS NULL OR ak.project_id = 0)
	`)

	// ── Step 8: Add project_id column to usage_logs if missing, then backfill ──
	db.Exec(`ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS project_id BIGINT DEFAULT 0`)
	db.Exec(`
		UPDATE usage_logs ul
		SET project_id = p.id
		FROM api_keys ak
		JOIN projects p ON p.tenant_id = ak.tenant_id AND p.name = 'Default'
		WHERE ul.key_id = ak.key_id
		  AND (ul.project_id IS NULL OR ul.project_id = 0)
	`)

	// ── Step 9: Create project memberships for all tenant members ──
	db.Exec(`
		INSERT INTO project_memberships (project_id, user_id, project_role, created_at, updated_at)
		SELECT p.id, tm.user_id,
			CASE
				WHEN tm.org_role IN ('owner', 'admin') THEN 'project_admin'
				WHEN tm.org_role = 'editor' THEN 'project_editor'
				ELSE 'project_viewer'
			END,
			NOW(), NOW()
		FROM tenant_memberships tm
		JOIN projects p ON p.tenant_id = tm.tenant_id AND p.name = 'Default'
		ON CONFLICT (project_id, user_id) DO NOTHING
	`)

	// ── Step 10: Drop legacy columns ──
	db.Exec("DROP INDEX IF EXISTS idx_users_tenant_id")
	db.Exec("ALTER TABLE users DROP COLUMN IF EXISTS tenant_id")
	db.Exec("ALTER TABLE users DROP COLUMN IF EXISTS role")
	db.Exec("ALTER TABLE tenants DROP COLUMN IF EXISTS max_api_keys")

	// Mark migration as complete.
	db.Exec(`INSERT INTO processed_stripe_events (event_id, processed_at) VALUES ('migration:rbac_v2', NOW()) ON CONFLICT DO NOTHING`)
	slog.Info("rbac_v2_migration: completed successfully")
}
