//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/xiaoboyu/tokengate/api-server/internal/api"
	"github.com/xiaoboyu/tokengate/api-server/internal/config"
	"github.com/xiaoboyu/tokengate/api-server/internal/db"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/pricing"
	"github.com/xiaoboyu/tokengate/api-server/internal/ratelimit"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
	"github.com/xiaoboyu/tokengate/api-server/internal/testutil"
)

var (
	testDB             *gorm.DB
	testRouter         http.Handler
	testPepper         = []byte("integration-test-pepper-32bytes!")
	testProviderKeySvc *services.ProviderKeyService
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		fmt.Println("TEST_POSTGRES_DSN not set — skipping integration tests")
		os.Exit(0)
	}

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect test db: %v\n", err)
		os.Exit(1)
	}

	// Drop legacy columns that may exist from prior test runs.
	gormDB.Exec("DROP INDEX IF EXISTS idx_users_tenant_id")
	gormDB.Exec("ALTER TABLE users DROP COLUMN IF EXISTS tenant_id")
	gormDB.Exec("ALTER TABLE users DROP COLUMN IF EXISTS role")
	gormDB.Exec("ALTER TABLE tenants DROP COLUMN IF EXISTS max_api_keys")

	if err := gormDB.AutoMigrate(
		&models.Tenant{}, &models.User{},
		&models.TenantMembership{}, &models.Project{}, &models.ProjectMembership{},
		&models.APIKey{}, &models.APIKeyProviderKeyBinding{},
		&models.ProviderKey{}, &models.TenantProviderSettings{},
		&models.UsageLog{}, &models.Provider{}, &models.ModelDef{},
		&models.ModelPricing{}, &models.ContractPricing{},
		&models.PricingMarkup{}, &models.CostLedger{},
		&models.BudgetLimit{}, &models.PricingConfig{},
		&models.PricingConfigRate{}, &models.APIKeyConfig{},
		&models.RateLimit{}, &models.ProcessedStripeEvent{},
		&models.AuditReport{}, &models.NotificationChannel{},
		&models.GatewayEvent{}, &models.AuditLog{},
	); err != nil {
		fmt.Fprintf(os.Stderr, "automigrate: %v\n", err)
		os.Exit(1)
	}

	testDB = gormDB

	cfg := &config.Config{
		Server: config.ServerCfg{
			Host:        "127.0.0.1",
			Port:        "0",
			CORSOrigins: []string{"*"},
		},
	}

	apiKeySvc := services.NewAPIKeyService(testDB, testPepper, nil, 0)
	usageSvc := services.NewUsageLogService(testDB)
	pricingEngine := pricing.NewPricingEngine(testDB, nil)
	stripeSvc := services.NewStripeService(testDB, config.StripeCfg{})
	rateLimiter := ratelimit.NewLimiter(testDB, nil)
	auditLogSvc := services.NewAuditLogService(testDB)
	pdb := db.NewFromDB(testDB)

	// 32-byte hex key for provider key encryption in tests.
	providerKeySvc, err := services.NewProviderKeyService(testDB, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "provider key svc: %v\n", err)
		os.Exit(1)
	}
	testProviderKeySvc = providerKeySvc

	srv := api.NewServer(
		cfg, pdb,
		nil, // rdb
		apiKeySvc, usageSvc, pricingEngine,
		providerKeySvc,
		nil, // proxyHandler
		rateLimiter,
		stripeSvc,
		nil, // auditSvc
		auditLogSvc,
		nil, // reportQueue
		nil, // notifWorker
	)
	testRouter = srv.Router()

	os.Exit(m.Run())
}

func cleanup(t *testing.T) {
	t.Helper()
	testutil.TruncateAll(t, testDB)
}

// userHeaders returns the standard auth headers for an authenticated request.
func userHeaders(userID string, tenantID uint) map[string]string {
	return map[string]string{
		"X-User-ID":   userID,
		"X-Tenant-Id": fmt.Sprintf("%d", tenantID),
	}
}

// getFirstMembershipTenantID extracts the tenant_id from the first membership
// in an auth sync response.
func getFirstMembershipTenantID(t *testing.T, resp map[string]interface{}) uint {
	t.Helper()
	memberships, ok := resp["memberships"].([]interface{})
	if !ok || len(memberships) == 0 {
		t.Fatal("no memberships in auth sync response")
	}
	first := memberships[0].(map[string]interface{})
	return uint(first["tenant_id"].(float64))
}

// getDefaultProjectID looks up the default project for a tenant.
func getDefaultProjectID(t *testing.T, tenantID uint) uint {
	t.Helper()
	var project models.Project
	if err := testDB.Where("tenant_id = ? AND name = ?", tenantID, "Default").First(&project).Error; err != nil {
		t.Fatalf("default project not found for tenant %d: %v", tenantID, err)
	}
	return project.ID
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

func TestHealth(t *testing.T) {
	w := testutil.DoRequest(testRouter, "GET", "/health", "", nil)
	if w.Code != 200 {
		t.Fatalf("GET /health = %d, want 200", w.Code)
	}
	body := testutil.ParseJSON(t, w)
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if body["service"] != "tokengate-api" {
		t.Errorf("service = %v, want tokengate-api", body["service"])
	}
}

func TestHealthV1(t *testing.T) {
	w := testutil.DoRequest(testRouter, "GET", "/v1/health", "", nil)
	if w.Code != 200 {
		t.Fatalf("GET /v1/health = %d, want 200", w.Code)
	}
}

func TestNoRoute(t *testing.T) {
	w := testutil.DoRequest(testRouter, "GET", "/v1/nonexistent", "", nil)
	if w.Code != 404 {
		t.Fatalf("GET /v1/nonexistent = %d, want 404", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Auth sync
// ---------------------------------------------------------------------------

func TestAuthSync_NewUser(t *testing.T) {
	cleanup(t)
	body := `{"clerk_user_id":"user_new1","email":"new@example.com","first_name":"New","last_name":"User"}`
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync", body, nil)
	if w.Code != 201 {
		t.Fatalf("POST /v1/auth/sync = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	if resp["user_id"] != "user_new1" {
		t.Errorf("user_id = %v, want user_new1", resp["user_id"])
	}
	if resp["is_new_user"] != true {
		t.Errorf("is_new_user = %v, want true", resp["is_new_user"])
	}

	// Verify memberships array exists with owner role.
	memberships, ok := resp["memberships"].([]interface{})
	if !ok || len(memberships) == 0 {
		t.Fatal("expected non-empty memberships array")
	}
	first := memberships[0].(map[string]interface{})
	if first["org_role"] != "owner" {
		t.Errorf("org_role = %v, want owner", first["org_role"])
	}

	// Verify tenant + membership + project created.
	var tenantCount int64
	testDB.Model(&models.Tenant{}).Count(&tenantCount)
	if tenantCount != 1 {
		t.Errorf("tenant count = %d, want 1", tenantCount)
	}
	var membershipCount int64
	testDB.Model(&models.TenantMembership{}).Count(&membershipCount)
	if membershipCount != 1 {
		t.Errorf("membership count = %d, want 1", membershipCount)
	}
	var projectCount int64
	testDB.Model(&models.Project{}).Count(&projectCount)
	if projectCount != 1 {
		t.Errorf("project count = %d, want 1", projectCount)
	}
}

func TestAuthSync_ExistingUser(t *testing.T) {
	cleanup(t)
	testutil.SeedTenant(t, testDB, "Test Co", "user_exist1", "exist@example.com")

	body := `{"clerk_user_id":"user_exist1","email":"exist@example.com"}`
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync", body, nil)
	if w.Code != 200 {
		t.Fatalf("POST /v1/auth/sync = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	if resp["is_new_user"] != false {
		t.Errorf("is_new_user = %v, want false", resp["is_new_user"])
	}
	// Verify memberships returned.
	memberships, ok := resp["memberships"].([]interface{})
	if !ok || len(memberships) == 0 {
		t.Fatal("expected memberships in response")
	}
}

func TestAuthSync_SuspendedUser(t *testing.T) {
	cleanup(t)
	// Create a suspended user (no tenant needed for suspension check).
	testDB.Create(&models.User{
		ID: "user_susp", Email: "susp@example.com",
		Name: "Susp", Status: models.StatusSuspended,
	})

	body := `{"clerk_user_id":"user_susp","email":"susp@example.com"}`
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync", body, nil)
	if w.Code != 403 {
		t.Fatalf("POST /v1/auth/sync suspended = %d, want 403", w.Code)
	}
}

func TestAuthSync_BadPayload(t *testing.T) {
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync", `{}`, nil)
	if w.Code != 400 {
		t.Fatalf("POST /v1/auth/sync empty = %d, want 400", w.Code)
	}
}

func TestAuthSync_InvalidJSON(t *testing.T) {
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync", `not json`, nil)
	if w.Code != 400 {
		t.Fatalf("POST /v1/auth/sync invalid json = %d, want 400", w.Code)
	}
}

func TestAuthSync_PendingInvite(t *testing.T) {
	cleanup(t)
	// Create tenant with owner.
	tenant := models.Tenant{Name: "Invite Co", Plan: models.PlanPro}
	testDB.Create(&tenant)
	project := models.Project{TenantID: tenant.ID, Name: "Default", Status: models.ProjectStatusActive}
	testDB.Create(&project)
	testDB.Model(&tenant).Update("default_project_id", project.ID)

	testDB.Create(&models.User{ID: "user_owner_inv", Email: "owner@invite.com", Name: "Owner", Status: models.StatusActive})
	testDB.Create(&models.TenantMembership{TenantID: tenant.ID, UserID: "user_owner_inv", OrgRole: models.RoleOwner, Status: models.StatusActive})

	// Create pending invite.
	testDB.Create(&models.User{ID: "pending_placeholder", Email: "invited@invite.com", Name: "Invited", Status: models.StatusPending})
	testDB.Create(&models.TenantMembership{TenantID: tenant.ID, UserID: "pending_placeholder", OrgRole: models.RoleEditor, Status: models.StatusPending})

	// Invited user signs up via Clerk with a real ID.
	body := `{"clerk_user_id":"user_real_invite","email":"invited@invite.com","first_name":"Invited"}`
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync", body, nil)
	if w.Code != 200 {
		t.Fatalf("POST /v1/auth/sync invited = %d; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	if resp["is_new_user"] != true {
		t.Errorf("is_new_user = %v, want true", resp["is_new_user"])
	}

	// Should have memberships: invited tenant (editor) + personal tenant (owner).
	memberships, ok := resp["memberships"].([]interface{})
	if !ok || len(memberships) < 2 {
		t.Fatalf("expected at least 2 memberships, got %d", len(memberships))
	}

	// Find the invited tenant membership.
	var foundEditor bool
	for _, m := range memberships {
		mem := m.(map[string]interface{})
		if uint(mem["tenant_id"].(float64)) == tenant.ID {
			if mem["org_role"] != "editor" {
				t.Errorf("invited tenant org_role = %v, want editor", mem["org_role"])
			}
			foundEditor = true
		}
	}
	if !foundEditor {
		t.Error("expected membership in inviting tenant with editor role")
	}
}

// ---------------------------------------------------------------------------
// RBAC enforcement
// ---------------------------------------------------------------------------

func TestRBAC_MissingUserHeader(t *testing.T) {
	w := testutil.DoRequest(testRouter, "GET", "/v1/usage", "", nil)
	if w.Code != 401 {
		t.Fatalf("GET /v1/usage without auth = %d, want 401", w.Code)
	}
}

func TestRBAC_UnknownUser(t *testing.T) {
	cleanup(t)
	w := testutil.DoRequest(testRouter, "GET", "/v1/usage", "", map[string]string{
		"X-User-ID":   "user_nonexistent",
		"X-Tenant-Id": "1",
	})
	if w.Code != 401 {
		t.Fatalf("GET /v1/usage unknown user = %d, want 401", w.Code)
	}
}

func TestRBAC_MissingTenantHeader(t *testing.T) {
	cleanup(t)
	testutil.SeedTenant(t, testDB, "T", "user_mt", "mt@example.com")

	// X-User-ID present but no X-Tenant-Id → 400
	w := testutil.DoRequest(testRouter, "GET", "/v1/usage", "", map[string]string{
		"X-User-ID": "user_mt",
	})
	if w.Code != 400 {
		t.Fatalf("GET /v1/usage without tenant = %d, want 400", w.Code)
	}
}

func TestRBAC_SuspendedUser(t *testing.T) {
	cleanup(t)
	// Suspended user is caught before tenant check.
	testDB.Create(&models.User{
		ID: "user_susp2", Email: "susp2@example.com",
		Name: "Susp2", Status: models.StatusSuspended,
	})

	w := testutil.DoRequest(testRouter, "GET", "/v1/usage", "", map[string]string{
		"X-User-ID":   "user_susp2",
		"X-Tenant-Id": "1",
	})
	if w.Code != 403 {
		t.Fatalf("GET /v1/usage suspended = %d, want 403", w.Code)
	}
}

func TestRBAC_NonMemberTenant(t *testing.T) {
	cleanup(t)
	// User exists but is NOT a member of the specified tenant.
	tenant := models.Tenant{Name: "Other Tenant", Plan: models.PlanFree}
	testDB.Create(&tenant)
	testDB.Create(&models.User{ID: "user_nm", Email: "nm@example.com", Name: "NM", Status: models.StatusActive})
	// No TenantMembership created for this user in this tenant.

	w := testutil.DoRequest(testRouter, "GET", "/v1/usage", "", userHeaders("user_nm", tenant.ID))
	if w.Code != 403 {
		t.Fatalf("GET /v1/usage non-member = %d, want 403", w.Code)
	}
}

func TestRBAC_SuspendedMembership(t *testing.T) {
	cleanup(t)
	tenant := models.Tenant{Name: "SM Tenant", Plan: models.PlanFree}
	testDB.Create(&tenant)
	testDB.Create(&models.User{ID: "user_sm", Email: "sm@example.com", Name: "SM", Status: models.StatusActive})
	testDB.Create(&models.TenantMembership{TenantID: tenant.ID, UserID: "user_sm", OrgRole: models.RoleViewer, Status: models.StatusSuspended})

	w := testutil.DoRequest(testRouter, "GET", "/v1/usage", "", userHeaders("user_sm", tenant.ID))
	if w.Code != 403 {
		t.Fatalf("GET /v1/usage suspended membership = %d, want 403", w.Code)
	}
}

func TestRBAC_ViewerCannotAccessAdmin(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_v1", "viewer@example.com")
	// SeedTenant creates owner; change membership to viewer.
	testDB.Model(&models.TenantMembership{}).Where("tenant_id = ? AND user_id = ?", tenant.ID, "user_v1").
		Update("org_role", models.RoleViewer)

	w := testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", userHeaders("user_v1", tenant.ID))
	if w.Code != 403 {
		t.Fatalf("GET /v1/admin/api_keys as viewer = %d, want 403", w.Code)
	}
}

func TestRBAC_EditorCanAccessAdmin(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_ed", "editor@example.com")
	testDB.Model(&models.TenantMembership{}).Where("tenant_id = ? AND user_id = ?", tenant.ID, "user_ed").
		Update("org_role", models.RoleEditor)

	w := testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", userHeaders("user_ed", tenant.ID))
	if w.Code != 200 {
		t.Fatalf("GET /v1/admin/api_keys as editor = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestRBAC_OwnerRoute_AdminDenied(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_own_1", "own@example.com")
	testutil.SeedUser(t, testDB, tenant.ID, "user_adm_1", "adm@example.com", models.RoleAdmin, models.StatusActive)

	w := testutil.DoRequest(testRouter, "GET", "/v1/owner/settings", "", userHeaders("user_adm_1", tenant.ID))
	if w.Code != 403 {
		t.Fatalf("GET /v1/owner/settings as admin = %d, want 403", w.Code)
	}
}

// ---------------------------------------------------------------------------
// API key CRUD
// ---------------------------------------------------------------------------

func TestCreateAPIKey_Success(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "KeyTest Co", "user_ak1", "ak1@example.com")
	projectID := getDefaultProjectID(t, tenant.ID)

	body := fmt.Sprintf(`{
		"label":"test-key",
		"project_id":%d,
		"provider":"anthropic",
		"auth_method":"BROWSER_OAUTH",
		"billing_mode":"MONTHLY_SUBSCRIPTION"
	}`, projectID)
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", body, userHeaders("user_ak1", tenant.ID))
	if w.Code != 201 {
		t.Fatalf("POST /v1/admin/api_keys = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	if resp["key_id"] == nil || resp["key_id"] == "" {
		t.Error("expected key_id in response")
	}
	if resp["secret"] == nil || resp["secret"] == "" {
		t.Error("expected secret in response")
	}
	if resp["label"] != "test-key" {
		t.Errorf("label = %v, want test-key", resp["label"])
	}
	if resp["project_id"] != float64(projectID) {
		t.Errorf("project_id = %v, want %d", resp["project_id"], projectID)
	}
}

func TestCreateAPIKey_BadPayload(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_bp", "bp@example.com")

	// Missing required project_id and provider fields.
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", `{"label":"x"}`, userHeaders("user_bp", tenant.ID))
	if w.Code != 400 {
		t.Fatalf("POST /v1/admin/api_keys bad payload = %d, want 400", w.Code)
	}
}

func TestCreateAPIKey_InvalidAuthBillingCombo(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_inv", "inv@example.com")
	projectID := getDefaultProjectID(t, tenant.ID)

	body := fmt.Sprintf(`{
		"label":"bad-combo",
		"project_id":%d,
		"provider":"anthropic",
		"auth_method":"BYOK",
		"billing_mode":"MONTHLY_SUBSCRIPTION"
	}`, projectID)
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", body, userHeaders("user_inv", tenant.ID))
	// BYOK + MONTHLY_SUBSCRIPTION is invalid → service returns error → 500
	if w.Code != 500 {
		t.Fatalf("POST invalid combo = %d, want 500; body: %s", w.Code, w.Body.String())
	}
}

func TestListAPIKeys_Empty(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_le", "le@example.com")

	w := testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", userHeaders("user_le", tenant.ID))
	if w.Code != 200 {
		t.Fatalf("GET /v1/admin/api_keys = %d, want 200", w.Code)
	}
	resp := testutil.ParseJSON(t, w)
	if resp["count"] != float64(0) {
		t.Errorf("count = %v, want 0", resp["count"])
	}
}

func TestListAPIKeys_WithKeys(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_lk", "lk@example.com")
	projectID := getDefaultProjectID(t, tenant.ID)

	body := fmt.Sprintf(`{
		"label":"list-test",
		"project_id":%d,
		"provider":"anthropic",
		"auth_method":"BROWSER_OAUTH",
		"billing_mode":"MONTHLY_SUBSCRIPTION"
	}`, projectID)
	testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", body, userHeaders("user_lk", tenant.ID))

	w := testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", userHeaders("user_lk", tenant.ID))
	if w.Code != 200 {
		t.Fatalf("GET /v1/admin/api_keys = %d", w.Code)
	}
	resp := testutil.ParseJSON(t, w)
	if resp["count"] != float64(1) {
		t.Errorf("count = %v, want 1", resp["count"])
	}
}

func TestRevokeAPIKey_Success(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_rv", "rv@example.com")
	projectID := getDefaultProjectID(t, tenant.ID)
	headers := userHeaders("user_rv", tenant.ID)

	body := fmt.Sprintf(`{
		"label":"revoke-me",
		"project_id":%d,
		"provider":"anthropic",
		"auth_method":"BROWSER_OAUTH",
		"billing_mode":"MONTHLY_SUBSCRIPTION"
	}`, projectID)
	createW := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", body, headers)
	createResp := testutil.ParseJSON(t, createW)
	keyID := createResp["key_id"].(string)

	w := testutil.DoRequest(testRouter, "DELETE", "/v1/admin/api_keys/"+keyID, "", headers)
	if w.Code != 200 {
		t.Fatalf("DELETE api_keys/%s = %d, want 200; body: %s", keyID, w.Code, w.Body.String())
	}

	// List should be empty.
	listW := testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", headers)
	listResp := testutil.ParseJSON(t, listW)
	if listResp["count"] != float64(0) {
		t.Errorf("count after revoke = %v, want 0", listResp["count"])
	}
}

func TestRevokeAPIKey_NotFound(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_rnf", "rnf@example.com")

	w := testutil.DoRequest(testRouter, "DELETE", "/v1/admin/api_keys/nonexistent", "", userHeaders("user_rnf", tenant.ID))
	if w.Code != 404 {
		t.Fatalf("DELETE nonexistent key = %d, want 404", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Plan limit enforcement
// ---------------------------------------------------------------------------

func TestCreateAPIKey_PlanLimitReached(t *testing.T) {
	cleanup(t)
	// SeedTenant creates a Pro tenant (MaxAPIKeys=5). Switch to Free (MaxAPIKeys=1).
	tenant, _ := testutil.SeedTenant(t, testDB, "Free Co", "user_pl", "pl@example.com")
	testDB.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Update("plan", models.PlanFree)
	projectID := getDefaultProjectID(t, tenant.ID)
	headers := userHeaders("user_pl", tenant.ID)

	mkBody := func(label string) string {
		return fmt.Sprintf(`{
			"label":%q,
			"project_id":%d,
			"provider":"anthropic",
			"auth_method":"BROWSER_OAUTH",
			"billing_mode":"MONTHLY_SUBSCRIPTION"
		}`, label, projectID)
	}

	w1 := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", mkBody("key-1"), headers)
	if w1.Code != 201 {
		t.Fatalf("first key = %d, want 201; body: %s", w1.Code, w1.Body.String())
	}

	w2 := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", mkBody("key-2"), headers)
	if w2.Code != 422 {
		t.Fatalf("second key = %d, want 422; body: %s", w2.Code, w2.Body.String())
	}
	resp := testutil.ParseJSON(t, w2)
	if resp["error"] != "plan_limit_reached" {
		t.Errorf("error = %v, want plan_limit_reached", resp["error"])
	}
}

// ---------------------------------------------------------------------------
// BYOK provider key enforcement
// ---------------------------------------------------------------------------

func TestCreateAPIKey_BYOK_NoProviderKey(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "BYOK Co", "user_byok1", "byok1@example.com")
	projectID := getDefaultProjectID(t, tenant.ID)

	body := fmt.Sprintf(`{
		"label":"byok-no-key",
		"project_id":%d,
		"provider":"anthropic",
		"auth_method":"BYOK",
		"billing_mode":"API_USAGE"
	}`, projectID)
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", body, userHeaders("user_byok1", tenant.ID))
	if w.Code != 422 {
		t.Fatalf("POST BYOK without provider key = %d, want 422; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	if resp["error"] != "no_active_provider_key" {
		t.Errorf("error = %v, want no_active_provider_key", resp["error"])
	}
}

func TestCreateAPIKey_BYOK_WithProviderKey(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "BYOK Co", "user_byok2", "byok2@example.com")
	projectID := getDefaultProjectID(t, tenant.ID)

	// Store a provider key so the BYOK check passes.
	_, err := testProviderKeySvc.Store(
		t.Context(), tenant.ID, "anthropic", "test-key",
		"sk-ant-api03-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
	)
	if err != nil {
		t.Fatalf("store provider key: %v", err)
	}

	body := fmt.Sprintf(`{
		"label":"byok-with-key",
		"project_id":%d,
		"provider":"anthropic",
		"auth_method":"BYOK",
		"billing_mode":"API_USAGE"
	}`, projectID)
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", body, userHeaders("user_byok2", tenant.ID))
	if w.Code != 201 {
		t.Fatalf("POST BYOK with provider key = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	if resp["key_id"] == nil || resp["key_id"] == "" {
		t.Error("expected key_id in response")
	}
}

// ---------------------------------------------------------------------------
// Billing
// ---------------------------------------------------------------------------

func TestBillingStatus_FreePlan(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "Free Co", "user_bs", "bs@example.com")
	testDB.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Update("plan", models.PlanFree)

	w := testutil.DoRequest(testRouter, "GET", "/v1/billing/status", "", userHeaders("user_bs", tenant.ID))
	if w.Code != 200 {
		t.Fatalf("GET /v1/billing/status = %d; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	if resp["has_subscription"] != false {
		t.Errorf("has_subscription = %v, want false", resp["has_subscription"])
	}
	if resp["stripe_configured"] != false {
		t.Errorf("stripe_configured = %v, want false", resp["stripe_configured"])
	}
}

func TestBillingCheckout_StripeNotConfigured(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_bc", "bc@example.com")

	body := `{"plan":"pro","success_url":"https://example.com/ok","cancel_url":"https://example.com/cancel"}`
	w := testutil.DoRequest(testRouter, "POST", "/v1/billing/checkout", body, userHeaders("user_bc", tenant.ID))
	if w.Code != 503 {
		t.Fatalf("POST /v1/billing/checkout without Stripe = %d, want 503; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Dashboard config
// ---------------------------------------------------------------------------

func TestDashboardConfig(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "Dash Co", "user_dc", "dc@example.com")

	w := testutil.DoRequest(testRouter, "GET", "/v1/dashboard/config", "", userHeaders("user_dc", tenant.ID))
	if w.Code != 200 {
		t.Fatalf("GET /v1/dashboard/config = %d; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	if resp["plan"] == nil {
		t.Error("expected plan in response")
	}
	if resp["retention"] == nil {
		t.Error("expected retention in response")
	}
	if resp["preset_options"] == nil {
		t.Error("expected preset_options in response")
	}
}

// ---------------------------------------------------------------------------
// Usage endpoints
// ---------------------------------------------------------------------------

func TestUsageList_Empty(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "Usage Co", "user_ul", "ul@example.com")

	w := testutil.DoRequest(testRouter, "GET", "/v1/usage", "", userHeaders("user_ul", tenant.ID))
	if w.Code != 200 {
		t.Fatalf("GET /v1/usage = %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUsageSummary(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "Summary Co", "user_us", "us@example.com")

	testDB.Create(&models.UsageLog{
		TenantID:         tenant.ID,
		Provider:         "anthropic",
		Model:            "claude-3-opus",
		PromptTokens:     1000,
		CompletionTokens: 500,
		RequestID:        "req_summary_1",
		APIUsageBilled:   true,
		CreatedAt:        time.Now(),
	})

	w := testutil.DoRequest(testRouter, "GET", "/v1/usage/summary", "", userHeaders("user_us", tenant.ID))
	if w.Code != 200 {
		t.Fatalf("GET /v1/usage/summary = %d; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	if resp["cost"] == nil {
		t.Error("expected cost in response")
	}
	if resp["by_model"] == nil {
		t.Error("expected by_model in response")
	}
	if resp["daily_trend"] == nil {
		t.Error("expected daily_trend in response")
	}
}

func TestCostLedger_Empty(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "Ledger Co", "user_cl", "cl@example.com")

	w := testutil.DoRequest(testRouter, "GET", "/v1/cost-ledger", "", userHeaders("user_cl", tenant.ID))
	if w.Code != 200 {
		t.Fatalf("GET /v1/cost-ledger = %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Project CRUD
// ---------------------------------------------------------------------------

func TestProjectCRUD(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "Proj Co", "user_proj", "proj@example.com")
	// Upgrade to Pro to allow more projects (Free allows 1, already have Default).
	testDB.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Update("plan", models.PlanTeam)
	headers := userHeaders("user_proj", tenant.ID)

	// List projects → should have 1 (Default).
	w := testutil.DoRequest(testRouter, "GET", "/v1/projects", "", headers)
	if w.Code != 200 {
		t.Fatalf("list projects = %d; body: %s", w.Code, w.Body.String())
	}
	listResp := testutil.ParseJSON(t, w)
	if listResp["count"] != float64(1) {
		t.Errorf("initial project count = %v, want 1", listResp["count"])
	}

	// Create a new project.
	w = testutil.DoRequest(testRouter, "POST", "/v1/projects", `{"name":"Backend","description":"Backend services"}`, headers)
	if w.Code != 201 {
		t.Fatalf("create project = %d; body: %s", w.Code, w.Body.String())
	}
	createResp := testutil.ParseJSON(t, w)
	projectID := uint(createResp["id"].(float64))
	if createResp["name"] != "Backend" {
		t.Errorf("name = %v, want Backend", createResp["name"])
	}
	if createResp["is_default"] != false {
		t.Errorf("is_default = %v, want false", createResp["is_default"])
	}

	// Get project.
	w = testutil.DoRequest(testRouter, "GET", fmt.Sprintf("/v1/projects/%d", projectID), "", headers)
	if w.Code != 200 {
		t.Fatalf("get project = %d; body: %s", w.Code, w.Body.String())
	}

	// Update project name.
	w = testutil.DoRequest(testRouter, "PATCH", fmt.Sprintf("/v1/projects/%d", projectID),
		`{"name":"Backend API"}`, headers)
	if w.Code != 200 {
		t.Fatalf("update project = %d; body: %s", w.Code, w.Body.String())
	}
	updateResp := testutil.ParseJSON(t, w)
	if updateResp["name"] != "Backend API" {
		t.Errorf("updated name = %v, want Backend API", updateResp["name"])
	}

	// List projects → should have 2.
	w = testutil.DoRequest(testRouter, "GET", "/v1/projects", "", headers)
	listResp = testutil.ParseJSON(t, w)
	if listResp["count"] != float64(2) {
		t.Errorf("project count after create = %v, want 2", listResp["count"])
	}

	// Delete the non-default project.
	w = testutil.DoRequest(testRouter, "DELETE", fmt.Sprintf("/v1/projects/%d", projectID), "", headers)
	if w.Code != 200 {
		t.Fatalf("delete project = %d; body: %s", w.Code, w.Body.String())
	}

	// List projects → back to 1.
	w = testutil.DoRequest(testRouter, "GET", "/v1/projects", "", headers)
	listResp = testutil.ParseJSON(t, w)
	if listResp["count"] != float64(1) {
		t.Errorf("project count after delete = %v, want 1", listResp["count"])
	}
}

func TestProjectDelete_DefaultProtected(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_pd", "pd@example.com")
	headers := userHeaders("user_pd", tenant.ID)
	defaultProjectID := getDefaultProjectID(t, tenant.ID)

	w := testutil.DoRequest(testRouter, "DELETE", fmt.Sprintf("/v1/projects/%d", defaultProjectID), "", headers)
	if w.Code != 409 {
		t.Fatalf("DELETE default project = %d, want 409; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	if resp["error"] != "cannot_delete_default" {
		t.Errorf("error = %v, want cannot_delete_default", resp["error"])
	}
}

func TestProjectDelete_HasActiveKeys(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_phk", "phk@example.com")
	testDB.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Update("plan", models.PlanTeam)
	headers := userHeaders("user_phk", tenant.ID)

	// Create a second project.
	w := testutil.DoRequest(testRouter, "POST", "/v1/projects", `{"name":"Extra"}`, headers)
	if w.Code != 201 {
		t.Fatalf("create project = %d; body: %s", w.Code, w.Body.String())
	}
	createResp := testutil.ParseJSON(t, w)
	projectID := uint(createResp["id"].(float64))

	// Create an API key bound to this project.
	keyBody := fmt.Sprintf(`{
		"label":"bound-key",
		"project_id":%d,
		"provider":"anthropic",
		"auth_method":"BROWSER_OAUTH",
		"billing_mode":"MONTHLY_SUBSCRIPTION"
	}`, projectID)
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", keyBody, headers)
	if w.Code != 201 {
		t.Fatalf("create key = %d; body: %s", w.Code, w.Body.String())
	}

	// Try to delete the project → should fail (has active keys).
	w = testutil.DoRequest(testRouter, "DELETE", fmt.Sprintf("/v1/projects/%d", projectID), "", headers)
	if w.Code != 409 {
		t.Fatalf("DELETE project with keys = %d, want 409; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	if resp["error"] != "has_active_keys" {
		t.Errorf("error = %v, want has_active_keys", resp["error"])
	}
}

func TestProjectMembers(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "Mem Co", "user_pm_owner", "pm_owner@example.com")
	testDB.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Update("plan", models.PlanTeam)
	headers := userHeaders("user_pm_owner", tenant.ID)

	// Add a second user to the tenant.
	testutil.SeedUser(t, testDB, tenant.ID, "user_pm_member", "pm_member@example.com", models.RoleEditor, models.StatusActive)
	defaultProjectID := getDefaultProjectID(t, tenant.ID)

	// Create another project.
	w := testutil.DoRequest(testRouter, "POST", "/v1/projects", `{"name":"Team Project"}`, headers)
	if w.Code != 201 {
		t.Fatalf("create project = %d; body: %s", w.Code, w.Body.String())
	}
	createResp := testutil.ParseJSON(t, w)
	projectID := uint(createResp["id"].(float64))

	// List members of default project (should have owner).
	w = testutil.DoRequest(testRouter, "GET", fmt.Sprintf("/v1/projects/%d/members", defaultProjectID), "", headers)
	if w.Code != 200 {
		t.Fatalf("list default project members = %d; body: %s", w.Code, w.Body.String())
	}
	membersResp := testutil.ParseJSON(t, w)
	if membersResp["count"] != float64(1) {
		t.Errorf("default project member count = %v, want 1", membersResp["count"])
	}

	// Add the second user to the new project.
	addBody := `{"user_id":"user_pm_member","project_role":"project_editor"}`
	w = testutil.DoRequest(testRouter, "POST", fmt.Sprintf("/v1/projects/%d/members", projectID), addBody, headers)
	if w.Code != 201 {
		t.Fatalf("add project member = %d; body: %s", w.Code, w.Body.String())
	}

	// List project members → should have 2 (owner auto-added + new member).
	w = testutil.DoRequest(testRouter, "GET", fmt.Sprintf("/v1/projects/%d/members", projectID), "", headers)
	if w.Code != 200 {
		t.Fatalf("list project members = %d; body: %s", w.Code, w.Body.String())
	}
	membersResp = testutil.ParseJSON(t, w)
	if membersResp["count"] != float64(2) {
		t.Errorf("project member count = %v, want 2", membersResp["count"])
	}

	// Update member role.
	w = testutil.DoRequest(testRouter, "PATCH", fmt.Sprintf("/v1/projects/%d/members/user_pm_member", projectID),
		`{"project_role":"project_viewer"}`, headers)
	if w.Code != 200 {
		t.Fatalf("update member role = %d; body: %s", w.Code, w.Body.String())
	}

	// Remove member.
	w = testutil.DoRequest(testRouter, "DELETE", fmt.Sprintf("/v1/projects/%d/members/user_pm_member", projectID), "", headers)
	if w.Code != 200 {
		t.Fatalf("remove member = %d; body: %s", w.Code, w.Body.String())
	}

	// Member count should be 1.
	w = testutil.DoRequest(testRouter, "GET", fmt.Sprintf("/v1/projects/%d/members", projectID), "", headers)
	membersResp = testutil.ParseJSON(t, w)
	if membersResp["count"] != float64(1) {
		t.Errorf("project member count after remove = %v, want 1", membersResp["count"])
	}
}

// ---------------------------------------------------------------------------
// Audit Logs
// ---------------------------------------------------------------------------

func TestAuditLogs_AdminOnly(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "Audit Co", "user_al_owner", "al_owner@example.com")
	testDB.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Update("plan", models.PlanTeam)

	// Create a viewer.
	testutil.SeedUser(t, testDB, tenant.ID, "user_al_viewer", "al_viewer@example.com", models.RoleViewer, models.StatusActive)

	// Viewer cannot access audit logs.
	w := testutil.DoRequest(testRouter, "GET", "/v1/audit-logs", "", userHeaders("user_al_viewer", tenant.ID))
	if w.Code != 403 {
		t.Fatalf("viewer audit logs = %d, want 403", w.Code)
	}

	// Owner (admin+) can access audit logs.
	w = testutil.DoRequest(testRouter, "GET", "/v1/audit-logs", "", userHeaders("user_al_owner", tenant.ID))
	if w.Code != 200 {
		t.Fatalf("owner audit logs = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	if resp["audit_logs"] == nil {
		t.Error("expected audit_logs in response")
	}
}

func TestAuditLogs_WithData(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "Audit Data Co", "user_ald", "ald@example.com")
	headers := userHeaders("user_ald", tenant.ID)

	// Seed audit log entries directly.
	testDB.Create(&models.AuditLog{
		TenantID:     tenant.ID,
		ActorUserID:  "user_ald",
		Action:       models.AuditAPIKeyCreated,
		ResourceType: "api_key",
		ResourceID:   "key_123",
		Category:     models.AuditCategoryAccess,
		ActorType:    models.AuditActorUser,
		Success:      true,
		CreatedAt:    time.Now(),
	})
	testDB.Create(&models.AuditLog{
		TenantID:     tenant.ID,
		ActorUserID:  "user_ald",
		Action:       models.AuditProjectCreated,
		ResourceType: "project",
		ResourceID:   "proj_1",
		Category:     models.AuditCategoryProject,
		ActorType:    models.AuditActorUser,
		Success:      true,
		CreatedAt:    time.Now(),
	})

	// List all audit logs.
	w := testutil.DoRequest(testRouter, "GET", "/v1/audit-logs", "", headers)
	if w.Code != 200 {
		t.Fatalf("audit logs = %d; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	logs, ok := resp["audit_logs"].([]interface{})
	if !ok {
		t.Fatal("audit_logs not an array")
	}
	if len(logs) != 2 {
		t.Errorf("audit log count = %d, want 2", len(logs))
	}

	// Filter by action.
	w = testutil.DoRequest(testRouter, "GET", "/v1/audit-logs?action=API_KEY.CREATED", "", headers)
	if w.Code != 200 {
		t.Fatalf("filtered audit logs = %d; body: %s", w.Code, w.Body.String())
	}
	resp = testutil.ParseJSON(t, w)
	logs = resp["audit_logs"].([]interface{})
	if len(logs) != 1 {
		t.Errorf("filtered audit log count = %d, want 1", len(logs))
	}
}

// ---------------------------------------------------------------------------
// Internal API
// ---------------------------------------------------------------------------

func TestInternalAPI_NoSecret_Returns503Or401(t *testing.T) {
	w := testutil.DoRequest(testRouter, "PATCH", "/v1/internal/tenants/1/plan", `{"plan":"pro"}`, nil)
	// INTERNAL_ADMIN_SECRET is not set → returns 503
	if w.Code != 503 {
		t.Fatalf("PATCH internal without secret = %d, want 503", w.Code)
	}
}

// Placeholder to satisfy the import of "json" (used in mustJSON).
func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
