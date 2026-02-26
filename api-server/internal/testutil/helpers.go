package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// MustConnectTestDB connects to the test Postgres database. The caller must
// provide a valid DSN (typically from the TEST_POSTGRES_DSN env var).
// It runs AutoMigrate for all models and returns a ready-to-use *gorm.DB.
func MustConnectTestDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("testutil: connect to test db: %v", err)
	}

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
	); err != nil {
		t.Fatalf("testutil: automigrate: %v", err)
	}

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	return db
}

// TruncateAll removes all rows from the core tables in dependency-safe order.
func TruncateAll(t *testing.T, db *gorm.DB) {
	t.Helper()
	tables := []string{
		"cost_ledgers", "usage_logs", "pricing_markups",
		"contract_pricings", "model_pricings", "model_defs", "providers",
		"api_key_configs", "pricing_config_rates", "pricing_configs",
		"budget_limits", "rate_limits",
		"tenant_provider_settings", "provider_keys",
		"api_keys", "users", "tenants",
		"processed_stripe_events", "audit_reports",
	}
	for _, tbl := range tables {
		db.Exec("DELETE FROM " + tbl)
	}
}

// SeedTenant creates a tenant and an owner user, returning both.
func SeedTenant(t *testing.T, db *gorm.DB, tenantName, ownerID, ownerEmail string) (models.Tenant, models.User) {
	t.Helper()
	tenant := models.Tenant{Name: tenantName, Plan: models.PlanPro, MaxAPIKeys: 5}
	if err := db.Create(&tenant).Error; err != nil {
		t.Fatalf("testutil: create tenant: %v", err)
	}
	user := models.User{
		ID:       ownerID,
		TenantID: tenant.ID,
		Email:    ownerEmail,
		Name:     "Test Owner",
		Role:     models.RoleOwner,
		Status:   models.StatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("testutil: create user: %v", err)
	}
	return tenant, user
}

// SeedUser creates an additional user in the given tenant.
func SeedUser(t *testing.T, db *gorm.DB, tenantID uint, id, email, role, status string) models.User {
	t.Helper()
	u := models.User{
		ID: id, TenantID: tenantID, Email: email,
		Name: "User " + role, Role: role, Status: status,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("testutil: create user: %v", err)
	}
	return u
}

// DoRequest performs an HTTP request against a gin.Engine and returns the
// recorded response.
func DoRequest(router http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ParseJSON unmarshals a response body into a map.
func ParseJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("testutil: parse json: %v (body: %s)", err, w.Body.String())
	}
	return m
}
