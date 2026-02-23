package db

import (
	"fmt"
	"log"
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
		fmt.Printf("postgres not ready (attempt %d/10): %v\n", i, err)
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
	); err != nil {
		return nil, fmt.Errorf("automigrate: %w", err)
	}

	if err := pricing.SeedInitialData(db); err != nil {
		return nil, fmt.Errorf("seed pricing data: %w", err)
	}

	if err := pricing.EnsureMissingModels(db); err != nil {
		return nil, fmt.Errorf("ensure missing models: %w", err)
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
		log.Printf("backfill usage_logs costs: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("backfill usage_logs costs: updated %d rows", res.RowsAffected)
	}

	return &PostgresDB{db: db}, nil
}

func (p *PostgresDB) GetDB() *gorm.DB {
	return p.db
}

func (p *PostgresDB) Close() {
	if sqlDB, err := p.db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
