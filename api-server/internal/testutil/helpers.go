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

	// Drop legacy columns that may exist from prior test runs.
	db.Exec("DROP INDEX IF EXISTS idx_users_tenant_id")
	db.Exec("ALTER TABLE users DROP COLUMN IF EXISTS tenant_id")
	db.Exec("ALTER TABLE users DROP COLUMN IF EXISTS role")
	db.Exec("ALTER TABLE tenants DROP COLUMN IF EXISTS max_api_keys")

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
		"audit_logs",
		"api_key_provider_key_bindings",
		"cost_ledgers", "usage_logs", "pricing_markups",
		"contract_pricings", "model_pricings", "model_defs", "providers",
		"api_key_configs", "pricing_config_rates", "pricing_configs",
		"budget_limits", "rate_limits",
		"tenant_provider_settings", "provider_keys",
		"project_memberships",
		"api_keys",
		"projects",
		"tenant_memberships",
		"notification_channels",
		"gateway_events",
		"users", "tenants",
		"processed_stripe_events", "audit_reports",
	}
	for _, tbl := range tables {
		db.Exec("DELETE FROM " + tbl)
	}
}

// SeedTenant creates a tenant, an owner user, a TenantMembership, and a default Project,
// returning the tenant and user.
func SeedTenant(t *testing.T, db *gorm.DB, tenantName, ownerID, ownerEmail string) (models.Tenant, models.User) {
	t.Helper()

	tenant := models.Tenant{Name: tenantName, Plan: models.PlanPro}
	if err := db.Create(&tenant).Error; err != nil {
		t.Fatalf("testutil: create tenant: %v", err)
	}

	// Create default project.
	project := models.Project{
		TenantID: tenant.ID,
		Name:     "Default",
		Status:   models.ProjectStatusActive,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("testutil: create default project: %v", err)
	}

	// Set default_project_id on tenant.
	db.Model(&tenant).Update("default_project_id", project.ID)
	tenant.DefaultProjectID = &project.ID

	user := models.User{
		ID:     ownerID,
		Email:  ownerEmail,
		Name:   "Test Owner",
		Status: models.StatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("testutil: create user: %v", err)
	}

	// Create TenantMembership.
	membership := models.TenantMembership{
		TenantID: tenant.ID,
		UserID:   user.ID,
		OrgRole:  models.RoleOwner,
		Status:   models.StatusActive,
	}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("testutil: create tenant membership: %v", err)
	}

	// Create ProjectMembership for default project.
	pm := models.ProjectMembership{
		ProjectID:   project.ID,
		UserID:      user.ID,
		ProjectRole: models.ProjectRoleAdmin,
	}
	if err := db.Create(&pm).Error; err != nil {
		t.Fatalf("testutil: create project membership: %v", err)
	}

	return tenant, user
}

// SeedUser creates an additional user in the given tenant with a TenantMembership.
func SeedUser(t *testing.T, db *gorm.DB, tenantID uint, id, email, role, status string) models.User {
	t.Helper()
	u := models.User{
		ID: id, Email: email,
		Name: "User " + role, Status: status,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("testutil: create user: %v", err)
	}

	// Create TenantMembership.
	membership := models.TenantMembership{
		TenantID: tenantID,
		UserID:   u.ID,
		OrgRole:  role,
		Status:   status,
	}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("testutil: create tenant membership: %v", err)
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
