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
	testDB     *gorm.DB
	testRouter http.Handler
	testPepper = []byte("integration-test-pepper-32bytes!")
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

	if err := gormDB.AutoMigrate(
		&models.Tenant{}, &models.User{}, &models.APIKey{},
		&models.ProviderKey{}, &models.TenantProviderSettings{},
		&models.UsageLog{}, &models.Provider{}, &models.ModelDef{},
		&models.ModelPricing{}, &models.ContractPricing{},
		&models.PricingMarkup{}, &models.CostLedger{},
		&models.BudgetLimit{}, &models.PricingConfig{},
		&models.PricingConfigRate{}, &models.APIKeyConfig{},
		&models.RateLimit{}, &models.ProcessedStripeEvent{},
		&models.AuditReport{},
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
	pdb := db.NewFromDB(testDB)

	srv := api.NewServer(
		cfg, pdb,
		apiKeySvc, usageSvc, pricingEngine,
		nil, // providerKeySvc
		nil, // proxyHandler
		rateLimiter,
		stripeSvc,
		nil, // auditSvc
		nil, // reportQueue
	)
	testRouter = srv.Router()

	os.Exit(m.Run())
}

func cleanup(t *testing.T) {
	t.Helper()
	testutil.TruncateAll(t, testDB)
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
	if resp["role"] != "owner" {
		t.Errorf("role = %v, want owner", resp["role"])
	}
	if resp["is_new_user"] != true {
		t.Errorf("is_new_user = %v, want true", resp["is_new_user"])
	}

	// Verify tenant was created in DB
	var count int64
	testDB.Model(&models.Tenant{}).Count(&count)
	if count != 1 {
		t.Errorf("tenant count = %d, want 1", count)
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
}

func TestAuthSync_SuspendedUser(t *testing.T) {
	cleanup(t)
	tenant := models.Tenant{Name: "Suspended Co", Plan: models.PlanFree, MaxAPIKeys: 1}
	testDB.Create(&tenant)
	testDB.Create(&models.User{
		ID: "user_susp", TenantID: tenant.ID, Email: "susp@example.com",
		Role: models.RoleOwner, Status: models.StatusSuspended,
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
	tenant := models.Tenant{Name: "Invite Co", Plan: models.PlanPro, MaxAPIKeys: 5}
	testDB.Create(&tenant)
	// Owner
	testDB.Create(&models.User{
		ID: "user_owner_inv", TenantID: tenant.ID, Email: "owner@invite.com",
		Role: models.RoleOwner, Status: models.StatusActive,
	})
	// Pending invite
	testDB.Create(&models.User{
		ID: "pending_placeholder", TenantID: tenant.ID, Email: "invited@invite.com",
		Role: models.RoleEditor, Status: models.StatusPending,
	})

	// The invited user signs up via Clerk with a real ID
	body := `{"clerk_user_id":"user_real_invite","email":"invited@invite.com","first_name":"Invited"}`
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync", body, nil)
	if w.Code != 200 {
		t.Fatalf("POST /v1/auth/sync invited = %d; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	if resp["role"] != "editor" {
		t.Errorf("role = %v, want editor (from invite)", resp["role"])
	}
	if resp["is_new_user"] != true {
		t.Errorf("is_new_user = %v, want true", resp["is_new_user"])
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
		"X-User-ID": "user_nonexistent",
	})
	if w.Code != 401 {
		t.Fatalf("GET /v1/usage unknown user = %d, want 401", w.Code)
	}
}

func TestRBAC_SuspendedUser(t *testing.T) {
	cleanup(t)
	tenant := models.Tenant{Name: "T", Plan: models.PlanFree, MaxAPIKeys: 1}
	testDB.Create(&tenant)
	testDB.Create(&models.User{
		ID: "user_susp2", TenantID: tenant.ID, Email: "susp2@example.com",
		Role: models.RoleViewer, Status: models.StatusSuspended,
	})

	w := testutil.DoRequest(testRouter, "GET", "/v1/usage", "", map[string]string{
		"X-User-ID": "user_susp2",
	})
	if w.Code != 403 {
		t.Fatalf("GET /v1/usage suspended = %d, want 403", w.Code)
	}
}

func TestRBAC_ViewerCannotAccessAdmin(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_v1", "viewer@example.com")
	testDB.Model(&models.User{}).Where("id = ?", "user_v1").Update("role", models.RoleViewer)
	_ = tenant

	w := testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", map[string]string{
		"X-User-ID": "user_v1",
	})
	if w.Code != 403 {
		t.Fatalf("GET /v1/admin/api_keys as viewer = %d, want 403", w.Code)
	}
}

func TestRBAC_EditorCanAccessAdmin(t *testing.T) {
	cleanup(t)
	testutil.SeedTenant(t, testDB, "T", "user_ed", "editor@example.com")
	testDB.Model(&models.User{}).Where("id = ?", "user_ed").Update("role", models.RoleEditor)

	w := testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", map[string]string{
		"X-User-ID": "user_ed",
	})
	if w.Code != 200 {
		t.Fatalf("GET /v1/admin/api_keys as editor = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestRBAC_OwnerRoute_AdminDenied(t *testing.T) {
	cleanup(t)
	tenant, _ := testutil.SeedTenant(t, testDB, "T", "user_own_1", "own@example.com")
	testutil.SeedUser(t, testDB, tenant.ID, "user_adm_1", "adm@example.com", models.RoleAdmin, models.StatusActive)

	w := testutil.DoRequest(testRouter, "GET", "/v1/owner/settings", "", map[string]string{
		"X-User-ID": "user_adm_1",
	})
	if w.Code != 403 {
		t.Fatalf("GET /v1/owner/settings as admin = %d, want 403", w.Code)
	}
}

// ---------------------------------------------------------------------------
// API key CRUD
// ---------------------------------------------------------------------------

func TestCreateAPIKey_Success(t *testing.T) {
	cleanup(t)
	testutil.SeedTenant(t, testDB, "KeyTest Co", "user_ak1", "ak1@example.com")

	body := `{
		"label":"test-key",
		"provider":"anthropic",
		"auth_method":"BROWSER_OAUTH",
		"billing_mode":"MONTHLY_SUBSCRIPTION"
	}`
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", body, map[string]string{
		"X-User-ID": "user_ak1",
	})
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
}

func TestCreateAPIKey_BadPayload(t *testing.T) {
	cleanup(t)
	testutil.SeedTenant(t, testDB, "T", "user_bp", "bp@example.com")

	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", `{"label":"x"}`, map[string]string{
		"X-User-ID": "user_bp",
	})
	if w.Code != 400 {
		t.Fatalf("POST /v1/admin/api_keys bad payload = %d, want 400", w.Code)
	}
}

func TestCreateAPIKey_InvalidAuthBillingCombo(t *testing.T) {
	cleanup(t)
	testutil.SeedTenant(t, testDB, "T", "user_inv", "inv@example.com")

	body := `{
		"label":"bad-combo",
		"provider":"anthropic",
		"auth_method":"BYOK",
		"billing_mode":"MONTHLY_SUBSCRIPTION"
	}`
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", body, map[string]string{
		"X-User-ID": "user_inv",
	})
	// BYOK + MONTHLY_SUBSCRIPTION is invalid → service returns error → 500
	if w.Code != 500 {
		t.Fatalf("POST invalid combo = %d, want 500; body: %s", w.Code, w.Body.String())
	}
}

func TestListAPIKeys_Empty(t *testing.T) {
	cleanup(t)
	testutil.SeedTenant(t, testDB, "T", "user_le", "le@example.com")

	w := testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", map[string]string{
		"X-User-ID": "user_le",
	})
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
	testutil.SeedTenant(t, testDB, "T", "user_lk", "lk@example.com")

	body := `{
		"label":"list-test",
		"provider":"anthropic",
		"auth_method":"BROWSER_OAUTH",
		"billing_mode":"MONTHLY_SUBSCRIPTION"
	}`
	testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", body, map[string]string{
		"X-User-ID": "user_lk",
	})

	w := testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", map[string]string{
		"X-User-ID": "user_lk",
	})
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
	testutil.SeedTenant(t, testDB, "T", "user_rv", "rv@example.com")

	body := `{
		"label":"revoke-me",
		"provider":"anthropic",
		"auth_method":"BROWSER_OAUTH",
		"billing_mode":"MONTHLY_SUBSCRIPTION"
	}`
	createW := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", body, map[string]string{
		"X-User-ID": "user_rv",
	})
	createResp := testutil.ParseJSON(t, createW)
	keyID := createResp["key_id"].(string)

	w := testutil.DoRequest(testRouter, "DELETE", "/v1/admin/api_keys/"+keyID, "", map[string]string{
		"X-User-ID": "user_rv",
	})
	if w.Code != 200 {
		t.Fatalf("DELETE api_keys/%s = %d, want 200; body: %s", keyID, w.Code, w.Body.String())
	}

	// List should be empty
	listW := testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", map[string]string{
		"X-User-ID": "user_rv",
	})
	listResp := testutil.ParseJSON(t, listW)
	if listResp["count"] != float64(0) {
		t.Errorf("count after revoke = %v, want 0", listResp["count"])
	}
}

func TestRevokeAPIKey_NotFound(t *testing.T) {
	cleanup(t)
	testutil.SeedTenant(t, testDB, "T", "user_rnf", "rnf@example.com")

	w := testutil.DoRequest(testRouter, "DELETE", "/v1/admin/api_keys/nonexistent", "", map[string]string{
		"X-User-ID": "user_rnf",
	})
	if w.Code != 404 {
		t.Fatalf("DELETE nonexistent key = %d, want 404", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Plan limit enforcement
// ---------------------------------------------------------------------------

func TestCreateAPIKey_PlanLimitReached(t *testing.T) {
	cleanup(t)
	tenant := models.Tenant{Name: "Free Co", Plan: models.PlanFree, MaxAPIKeys: 1}
	testDB.Create(&tenant)
	testDB.Create(&models.User{
		ID: "user_pl", TenantID: tenant.ID, Email: "pl@example.com",
		Role: models.RoleOwner, Status: models.StatusActive,
	})

	mkBody := func(label string) string {
		return fmt.Sprintf(`{
			"label":%q,
			"provider":"anthropic",
			"auth_method":"BROWSER_OAUTH",
			"billing_mode":"MONTHLY_SUBSCRIPTION"
		}`, label)
	}
	headers := map[string]string{"X-User-ID": "user_pl"}

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
// Billing
// ---------------------------------------------------------------------------

func TestBillingStatus_FreePlan(t *testing.T) {
	cleanup(t)
	testutil.SeedTenant(t, testDB, "Free Co", "user_bs", "bs@example.com")
	testDB.Model(&models.Tenant{}).Where("name = ?", "Free Co").Update("plan", models.PlanFree)

	w := testutil.DoRequest(testRouter, "GET", "/v1/billing/status", "", map[string]string{
		"X-User-ID": "user_bs",
	})
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
	testutil.SeedTenant(t, testDB, "T", "user_bc", "bc@example.com")
	testDB.Model(&models.User{}).Where("id = ?", "user_bc").Update("role", models.RoleAdmin)

	body := `{"plan":"pro","success_url":"https://example.com/ok","cancel_url":"https://example.com/cancel"}`
	w := testutil.DoRequest(testRouter, "POST", "/v1/billing/checkout", body, map[string]string{
		"X-User-ID": "user_bc",
	})
	if w.Code != 503 {
		t.Fatalf("POST /v1/billing/checkout without Stripe = %d, want 503; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Dashboard config
// ---------------------------------------------------------------------------

func TestDashboardConfig(t *testing.T) {
	cleanup(t)
	testutil.SeedTenant(t, testDB, "Dash Co", "user_dc", "dc@example.com")

	w := testutil.DoRequest(testRouter, "GET", "/v1/dashboard/config", "", map[string]string{
		"X-User-ID": "user_dc",
	})
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
	testutil.SeedTenant(t, testDB, "Usage Co", "user_ul", "ul@example.com")

	w := testutil.DoRequest(testRouter, "GET", "/v1/usage", "", map[string]string{
		"X-User-ID": "user_ul",
	})
	if w.Code != 200 {
		t.Fatalf("GET /v1/usage = %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUsageSummary(t *testing.T) {
	cleanup(t)
	_, owner := testutil.SeedTenant(t, testDB, "Summary Co", "user_us", "us@example.com")

	testDB.Create(&models.UsageLog{
		TenantID:         owner.TenantID,
		Provider:         "anthropic",
		Model:            "claude-3-opus",
		PromptTokens:     1000,
		CompletionTokens: 500,
		RequestID:        "req_summary_1",
		APIUsageBilled:   true,
		CreatedAt:        time.Now(),
	})

	w := testutil.DoRequest(testRouter, "GET", "/v1/usage/summary", "", map[string]string{
		"X-User-ID": "user_us",
	})
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
	testutil.SeedTenant(t, testDB, "Ledger Co", "user_cl", "cl@example.com")

	w := testutil.DoRequest(testRouter, "GET", "/v1/cost-ledger", "", map[string]string{
		"X-User-ID": "user_cl",
	})
	if w.Code != 200 {
		t.Fatalf("GET /v1/cost-ledger = %d; body: %s", w.Code, w.Body.String())
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
