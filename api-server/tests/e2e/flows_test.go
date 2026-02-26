//go:build e2e

package e2e

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
	testPepper = []byte("e2e-test-pepper-32-bytes-long!!!")
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		fmt.Println("TEST_POSTGRES_DSN not set — skipping e2e tests")
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

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func parseJSON(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("parse json: %v (body: %s)", err, string(body))
	}
	return m
}

// ---------------------------------------------------------------------------
// Flow 1: Full user onboarding → API key lifecycle
//
// Steps:
//   1. Auth sync (new user) → creates tenant + owner
//   2. Create API key
//   3. List API keys → verify key appears
//   4. Revoke API key
//   5. List API keys → verify empty
// ---------------------------------------------------------------------------

func TestFlow_UserOnboarding_APIKeyLifecycle(t *testing.T) {
	cleanup(t)

	// Step 1: New user signs up via auth/sync
	syncBody := `{"clerk_user_id":"e2e_owner_1","email":"owner@e2e.test","first_name":"E2E","last_name":"Owner"}`
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync", syncBody, nil)
	if w.Code != 201 {
		t.Fatalf("auth/sync = %d; body: %s", w.Code, w.Body.String())
	}
	syncResp := testutil.ParseJSON(t, w)
	if syncResp["role"] != "owner" {
		t.Fatalf("expected owner role, got %v", syncResp["role"])
	}
	if syncResp["is_new_user"] != true {
		t.Fatalf("expected is_new_user=true")
	}

	headers := map[string]string{"X-User-ID": "e2e_owner_1"}

	// Step 2: Create an API key
	keyBody := `{
		"label":"e2e-test-key",
		"provider":"anthropic",
		"auth_method":"BROWSER_OAUTH",
		"billing_mode":"MONTHLY_SUBSCRIPTION"
	}`
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", keyBody, headers)
	if w.Code != 201 {
		t.Fatalf("create key = %d; body: %s", w.Code, w.Body.String())
	}
	createResp := testutil.ParseJSON(t, w)
	keyID, ok := createResp["key_id"].(string)
	if !ok || keyID == "" {
		t.Fatal("expected key_id in response")
	}
	if createResp["secret"] == nil || createResp["secret"] == "" {
		t.Fatal("expected secret in response")
	}
	if createResp["label"] != "e2e-test-key" {
		t.Errorf("label = %v, want e2e-test-key", createResp["label"])
	}

	// Step 3: List keys → should have 1
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", headers)
	if w.Code != 200 {
		t.Fatalf("list keys = %d", w.Code)
	}
	listResp := testutil.ParseJSON(t, w)
	if listResp["count"] != float64(1) {
		t.Errorf("count = %v, want 1", listResp["count"])
	}

	// Step 4: Revoke the key
	w = testutil.DoRequest(testRouter, "DELETE", "/v1/admin/api_keys/"+keyID, "", headers)
	if w.Code != 200 {
		t.Fatalf("revoke key = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 5: List keys → should be empty
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", headers)
	if w.Code != 200 {
		t.Fatalf("list keys after revoke = %d", w.Code)
	}
	listResp = testutil.ParseJSON(t, w)
	if listResp["count"] != float64(0) {
		t.Errorf("count after revoke = %v, want 0", listResp["count"])
	}
}

// ---------------------------------------------------------------------------
// Flow 2: Team management — invite user, change role, suspend, unsuspend
//
// Steps:
//   1. Auth sync owner
//   2. Invite a new user (editor)
//   3. List users → pending invite visible
//   4. Invited user signs up via auth/sync → joins as editor
//   5. Owner changes their role to viewer
//   6. Owner suspends them
//   7. Suspended user cannot access viewer endpoints
//   8. Owner unsuspends them
//   9. User can access viewer endpoints again
// ---------------------------------------------------------------------------

func TestFlow_TeamManagement(t *testing.T) {
	cleanup(t)

	// Step 1: Owner signs up
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_tm_owner","email":"tm-owner@e2e.test","first_name":"TM","last_name":"Owner"}`, nil)
	if w.Code != 201 {
		t.Fatalf("owner sync = %d; body: %s", w.Code, w.Body.String())
	}
	ownerHeaders := map[string]string{"X-User-ID": "e2e_tm_owner"}

	// Step 2: Invite a new team member as editor
	inviteBody := `{"email":"tm-editor@e2e.test","name":"Team Editor","role":"editor"}`
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/users/invite", inviteBody, ownerHeaders)
	if w.Code != 201 {
		t.Fatalf("invite = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 3: List users → should have owner + pending invite
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/users", "", ownerHeaders)
	if w.Code != 200 {
		t.Fatalf("list users = %d; body: %s", w.Code, w.Body.String())
	}
	listResp := testutil.ParseJSON(t, w)
	total, _ := listResp["total"].(float64)
	if total != 2 {
		t.Errorf("total users = %v, want 2 (owner + pending)", total)
	}

	// Step 4: Invited user signs up
	w = testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_tm_editor","email":"tm-editor@e2e.test","first_name":"Team","last_name":"Editor"}`, nil)
	if w.Code != 200 {
		t.Fatalf("editor sync = %d; body: %s", w.Code, w.Body.String())
	}
	editorSync := testutil.ParseJSON(t, w)
	if editorSync["role"] != "editor" {
		t.Errorf("editor role = %v, want editor", editorSync["role"])
	}

	editorHeaders := map[string]string{"X-User-ID": "e2e_tm_editor"}

	// Editor should be able to access admin endpoints
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", editorHeaders)
	if w.Code != 200 {
		t.Fatalf("editor access admin = %d, want 200", w.Code)
	}

	// Step 5: Owner changes editor's role to viewer
	w = testutil.DoRequest(testRouter, "PATCH", "/v1/admin/users/e2e_tm_editor/role",
		`{"role":"viewer"}`, ownerHeaders)
	if w.Code != 200 {
		t.Fatalf("change role = %d; body: %s", w.Code, w.Body.String())
	}

	// Viewer should NOT be able to access admin endpoints
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", editorHeaders)
	if w.Code != 403 {
		t.Fatalf("viewer access admin = %d, want 403", w.Code)
	}

	// Viewer CAN access viewer endpoints
	w = testutil.DoRequest(testRouter, "GET", "/v1/usage", "", editorHeaders)
	if w.Code != 200 {
		t.Fatalf("viewer access usage = %d, want 200", w.Code)
	}

	// Step 6: Owner suspends the user
	w = testutil.DoRequest(testRouter, "PATCH", "/v1/admin/users/e2e_tm_editor/suspend", "", ownerHeaders)
	if w.Code != 200 {
		t.Fatalf("suspend = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 7: Suspended user cannot access anything
	w = testutil.DoRequest(testRouter, "GET", "/v1/usage", "", editorHeaders)
	if w.Code != 403 {
		t.Fatalf("suspended user access = %d, want 403", w.Code)
	}

	// Step 8: Owner unsuspends the user
	w = testutil.DoRequest(testRouter, "PATCH", "/v1/admin/users/e2e_tm_editor/unsuspend", "", ownerHeaders)
	if w.Code != 200 {
		t.Fatalf("unsuspend = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 9: User can access again
	w = testutil.DoRequest(testRouter, "GET", "/v1/usage", "", editorHeaders)
	if w.Code != 200 {
		t.Fatalf("unsuspended user access = %d, want 200", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Flow 3: Usage tracking — create key, seed usage, view summary/ledger
//
// Steps:
//   1. Auth sync owner
//   2. Create API key
//   3. Seed usage data directly in DB
//   4. Get usage summary → verify totals
//   5. List usage → verify logs present
//   6. Get cost ledger
// ---------------------------------------------------------------------------

func TestFlow_UsageTracking(t *testing.T) {
	cleanup(t)

	// Step 1: Owner signs up
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_usage_owner","email":"usage@e2e.test","first_name":"Usage","last_name":"Test"}`, nil)
	if w.Code != 201 {
		t.Fatalf("owner sync = %d; body: %s", w.Code, w.Body.String())
	}
	headers := map[string]string{"X-User-ID": "e2e_usage_owner"}

	// Step 2: Create an API key
	keyBody := `{
		"label":"usage-track-key",
		"provider":"anthropic",
		"auth_method":"BROWSER_OAUTH",
		"billing_mode":"MONTHLY_SUBSCRIPTION"
	}`
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", keyBody, headers)
	if w.Code != 201 {
		t.Fatalf("create key = %d; body: %s", w.Code, w.Body.String())
	}
	createResp := testutil.ParseJSON(t, w)
	keyID := createResp["key_id"].(string)

	// Get the tenant ID from DB
	var user models.User
	testDB.Where("id = ?", "e2e_usage_owner").First(&user)
	tenantID := user.TenantID

	// Step 3: Seed usage data
	now := time.Now()
	logs := []models.UsageLog{
		{
			TenantID: tenantID, KeyID: keyID,
			Provider: "anthropic", Model: "claude-3-opus",
			PromptTokens: 1000, CompletionTokens: 500,
			RequestID: "e2e_req_1", APIUsageBilled: true,
			CreatedAt: now,
		},
		{
			TenantID: tenantID, KeyID: keyID,
			Provider: "anthropic", Model: "claude-3-sonnet",
			PromptTokens: 2000, CompletionTokens: 1000,
			RequestID: "e2e_req_2", APIUsageBilled: true,
			CreatedAt: now.Add(-1 * time.Hour),
		},
		{
			TenantID: tenantID, KeyID: keyID,
			Provider: "openai", Model: "gpt-4",
			PromptTokens: 500, CompletionTokens: 200,
			RequestID: "e2e_req_3", APIUsageBilled: true,
			CreatedAt: now.Add(-24 * time.Hour),
		},
	}
	for _, log := range logs {
		if err := testDB.Create(&log).Error; err != nil {
			t.Fatalf("seed usage: %v", err)
		}
	}

	// Step 4: Get usage summary
	w = testutil.DoRequest(testRouter, "GET", "/v1/usage/summary", "", headers)
	if w.Code != 200 {
		t.Fatalf("usage summary = %d; body: %s", w.Code, w.Body.String())
	}
	summary := testutil.ParseJSON(t, w)
	if summary["by_model"] == nil {
		t.Error("expected by_model in summary")
	}
	byModel, ok := summary["by_model"].([]interface{})
	if !ok {
		t.Fatal("by_model is not an array")
	}
	if len(byModel) < 2 {
		t.Errorf("expected at least 2 models in by_model, got %d", len(byModel))
	}

	// Step 5: List usage
	w = testutil.DoRequest(testRouter, "GET", "/v1/usage", "", headers)
	if w.Code != 200 {
		t.Fatalf("list usage = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 6: Cost ledger
	w = testutil.DoRequest(testRouter, "GET", "/v1/cost-ledger", "", headers)
	if w.Code != 200 {
		t.Fatalf("cost ledger = %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Flow 4: Budget management — create, read, update, delete
//
// Steps:
//   1. Auth sync owner
//   2. Get budget → empty
//   3. Create monthly budget
//   4. Get budget → verify limit
//   5. Update budget amount
//   6. Delete budget
//   7. Get budget → empty again
// ---------------------------------------------------------------------------

func TestFlow_BudgetManagement(t *testing.T) {
	cleanup(t)

	// Step 1: Owner signs up
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_budget_owner","email":"budget@e2e.test","first_name":"Budget","last_name":"Test"}`, nil)
	if w.Code != 201 {
		t.Fatalf("owner sync = %d; body: %s", w.Code, w.Body.String())
	}
	headers := map[string]string{"X-User-ID": "e2e_budget_owner"}

	// Step 2: Get budget → empty
	w = testutil.DoRequest(testRouter, "GET", "/v1/budget", "", headers)
	if w.Code != 200 {
		t.Fatalf("get budget = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 3: Create monthly budget
	budgetBody := `{
		"scope_type":"account",
		"period_type":"monthly",
		"limit_amount":"500.00",
		"alert_threshold":"80",
		"action":"alert"
	}`
	w = testutil.DoRequest(testRouter, "PUT", "/v1/admin/budget", budgetBody, headers)
	if w.Code != 200 {
		t.Fatalf("create budget = %d; body: %s", w.Code, w.Body.String())
	}
	budgetResp := testutil.ParseJSON(t, w)
	budgetID := budgetResp["id"]

	if budgetID == nil || budgetID == float64(0) {
		t.Fatal("expected budget id in response")
	}

	// Step 4: Get budget → verify limit
	w = testutil.DoRequest(testRouter, "GET", "/v1/budget", "", headers)
	if w.Code != 200 {
		t.Fatalf("get budget = %d; body: %s", w.Code, w.Body.String())
	}
	getBudget := testutil.ParseJSON(t, w)
	limits, ok := getBudget["budget_limits"].([]interface{})
	if !ok || len(limits) == 0 {
		t.Fatal("expected budget_limits in response")
	}

	// Step 5: Update budget amount
	updateBody := `{
		"scope_type":"account",
		"period_type":"monthly",
		"limit_amount":"1000.00",
		"alert_threshold":"90",
		"action":"block"
	}`
	w = testutil.DoRequest(testRouter, "PUT", "/v1/admin/budget", updateBody, headers)
	if w.Code != 200 {
		t.Fatalf("update budget = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 6: Delete budget
	idStr := fmt.Sprintf("%.0f", budgetID.(float64))
	w = testutil.DoRequest(testRouter, "DELETE", "/v1/admin/budget/"+idStr, "", headers)
	if w.Code != 200 {
		t.Fatalf("delete budget = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 7: Get budget → should be empty now
	w = testutil.DoRequest(testRouter, "GET", "/v1/budget", "", headers)
	if w.Code != 200 {
		t.Fatalf("get budget after delete = %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Flow 5: Rate limit CRUD
//
// Steps:
//   1. Auth sync owner
//   2. List rate limits → empty
//   3. Create RPM rate limit
//   4. List rate limits → 1 entry
//   5. Delete rate limit
//   6. List rate limits → empty
// ---------------------------------------------------------------------------

func TestFlow_RateLimitCRUD(t *testing.T) {
	cleanup(t)

	// Step 1: Owner signs up
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_rl_owner","email":"rl@e2e.test","first_name":"RL","last_name":"Test"}`, nil)
	if w.Code != 201 {
		t.Fatalf("owner sync = %d; body: %s", w.Code, w.Body.String())
	}
	headers := map[string]string{"X-User-ID": "e2e_rl_owner"}

	// Step 2: List rate limits → empty
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/rate-limits", "", headers)
	if w.Code != 200 {
		t.Fatalf("list rate limits = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 3: Create RPM rate limit
	rlBody := `{
		"provider":"anthropic",
		"model":"claude-3-opus",
		"scope_type":"account",
		"metric":"rpm",
		"limit_value":100,
		"window_seconds":60
	}`
	w = testutil.DoRequest(testRouter, "PUT", "/v1/admin/rate-limits", rlBody, headers)
	if w.Code != 200 {
		t.Fatalf("create rate limit = %d; body: %s", w.Code, w.Body.String())
	}
	rlResp := testutil.ParseJSON(t, w)
	rlID := rlResp["id"]
	if rlID == nil || rlID == float64(0) {
		t.Fatal("expected rate limit id in response")
	}

	// Step 4: List rate limits → 1 entry
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/rate-limits", "", headers)
	if w.Code != 200 {
		t.Fatalf("list rate limits = %d", w.Code)
	}
	listResp := testutil.ParseJSON(t, w)
	rls, ok := listResp["rate_limits"].([]interface{})
	if !ok || len(rls) != 1 {
		t.Errorf("expected 1 rate limit, got %d", len(rls))
	}

	// Step 5: Delete rate limit
	idStr := fmt.Sprintf("%.0f", rlID.(float64))
	w = testutil.DoRequest(testRouter, "DELETE", "/v1/admin/rate-limits/"+idStr, "", headers)
	if w.Code != 200 {
		t.Fatalf("delete rate limit = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 6: List rate limits → empty
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/rate-limits", "", headers)
	if w.Code != 200 {
		t.Fatalf("list rate limits after delete = %d", w.Code)
	}
	listResp = testutil.ParseJSON(t, w)
	rls, _ = listResp["rate_limits"].([]interface{})
	if len(rls) != 0 {
		t.Errorf("expected 0 rate limits after delete, got %d", len(rls))
	}
}

// ---------------------------------------------------------------------------
// Flow 6: RBAC escalation — viewers/editors cannot access owner routes
//
// Steps:
//   1. Auth sync owner
//   2. Invite editor and viewer
//   3. Editor and viewer sign up
//   4. Verify RBAC for each role across all route groups
// ---------------------------------------------------------------------------

func TestFlow_RBACEscalation(t *testing.T) {
	cleanup(t)

	// Owner signs up
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_rbac_owner","email":"rbac-owner@e2e.test","first_name":"RBAC","last_name":"Owner"}`, nil)
	if w.Code != 201 {
		t.Fatalf("owner sync = %d; body: %s", w.Code, w.Body.String())
	}
	ownerH := map[string]string{"X-User-ID": "e2e_rbac_owner"}

	// Invite editor
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/users/invite",
		`{"email":"rbac-editor@e2e.test","role":"editor"}`, ownerH)
	if w.Code != 201 {
		t.Fatalf("invite editor = %d; body: %s", w.Code, w.Body.String())
	}

	// Invite viewer
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/users/invite",
		`{"email":"rbac-viewer@e2e.test","role":"viewer"}`, ownerH)
	if w.Code != 201 {
		t.Fatalf("invite viewer = %d; body: %s", w.Code, w.Body.String())
	}

	// Editor signs up
	w = testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_rbac_editor","email":"rbac-editor@e2e.test"}`, nil)
	if w.Code != 200 {
		t.Fatalf("editor sync = %d; body: %s", w.Code, w.Body.String())
	}
	editorH := map[string]string{"X-User-ID": "e2e_rbac_editor"}

	// Viewer signs up
	w = testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_rbac_viewer","email":"rbac-viewer@e2e.test"}`, nil)
	if w.Code != 200 {
		t.Fatalf("viewer sync = %d; body: %s", w.Code, w.Body.String())
	}
	viewerH := map[string]string{"X-User-ID": "e2e_rbac_viewer"}

	// --- Viewer can access viewer routes ---
	viewerRoutes := []struct {
		method, path string
	}{
		{"GET", "/v1/usage"},
		{"GET", "/v1/usage/summary"},
		{"GET", "/v1/budget"},
		{"GET", "/v1/dashboard/config"},
		{"GET", "/v1/billing/status"},
	}
	for _, r := range viewerRoutes {
		w = testutil.DoRequest(testRouter, r.method, r.path, "", viewerH)
		if w.Code != 200 {
			t.Errorf("viewer %s %s = %d, want 200; body: %s", r.method, r.path, w.Code, w.Body.String())
		}
	}

	// --- Viewer CANNOT access editor/admin routes ---
	editorRoutes := []struct {
		method, path, body string
	}{
		{"GET", "/v1/admin/api_keys", ""},
		{"GET", "/v1/admin/users", ""},
		{"GET", "/v1/admin/rate-limits", ""},
	}
	for _, r := range editorRoutes {
		w = testutil.DoRequest(testRouter, r.method, r.path, r.body, viewerH)
		if w.Code != 403 {
			t.Errorf("viewer %s %s = %d, want 403", r.method, r.path, w.Code)
		}
	}

	// --- Editor CAN access editor/admin routes ---
	for _, r := range editorRoutes {
		w = testutil.DoRequest(testRouter, r.method, r.path, r.body, editorH)
		if w.Code != 200 {
			t.Errorf("editor %s %s = %d, want 200; body: %s", r.method, r.path, w.Code, w.Body.String())
		}
	}

	// --- Editor CANNOT access owner routes ---
	ownerRoutes := []struct {
		method, path string
	}{
		{"GET", "/v1/owner/settings"},
		{"DELETE", "/v1/owner/account"},
	}
	for _, r := range ownerRoutes {
		w = testutil.DoRequest(testRouter, r.method, r.path, "", editorH)
		if w.Code != 403 {
			t.Errorf("editor %s %s = %d, want 403", r.method, r.path, w.Code)
		}
	}

	// --- Owner CAN access owner routes ---
	w = testutil.DoRequest(testRouter, "GET", "/v1/owner/settings", "", ownerH)
	if w.Code != 200 {
		t.Errorf("owner GET /v1/owner/settings = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Flow 7: Multi-key management with plan limit enforcement
//
// Steps:
//   1. Auth sync owner (free plan → max 1 key)
//   2. Create first key → success
//   3. Create second key → 422 plan_limit_reached
//   4. Revoke first key
//   5. Create second key → success (slot freed)
// ---------------------------------------------------------------------------

func TestFlow_PlanLimitEnforcement(t *testing.T) {
	cleanup(t)

	// Owner signs up (default is Pro plan from SeedTenant, but auth/sync creates tenant directly)
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_plan_owner","email":"plan@e2e.test","first_name":"Plan","last_name":"Test"}`, nil)
	if w.Code != 201 {
		t.Fatalf("owner sync = %d; body: %s", w.Code, w.Body.String())
	}

	// Force the tenant to free plan with max_api_keys=1
	var user models.User
	testDB.Where("id = ?", "e2e_plan_owner").First(&user)
	testDB.Model(&models.Tenant{}).Where("id = ?", user.TenantID).
		Updates(map[string]interface{}{"plan": models.PlanFree, "max_api_keys": 1})

	headers := map[string]string{"X-User-ID": "e2e_plan_owner"}

	mkKey := func(label string) string {
		return fmt.Sprintf(`{
			"label":%q,
			"provider":"anthropic",
			"auth_method":"BROWSER_OAUTH",
			"billing_mode":"MONTHLY_SUBSCRIPTION"
		}`, label)
	}

	// Step 2: First key → success
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", mkKey("key-1"), headers)
	if w.Code != 201 {
		t.Fatalf("first key = %d; body: %s", w.Code, w.Body.String())
	}
	firstResp := testutil.ParseJSON(t, w)
	firstKeyID := firstResp["key_id"].(string)

	// Step 3: Second key → plan limit
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", mkKey("key-2"), headers)
	if w.Code != 422 {
		t.Fatalf("second key = %d, want 422; body: %s", w.Code, w.Body.String())
	}
	errResp := testutil.ParseJSON(t, w)
	if errResp["error"] != "plan_limit_reached" {
		t.Errorf("error = %v, want plan_limit_reached", errResp["error"])
	}

	// Step 4: Revoke first key
	w = testutil.DoRequest(testRouter, "DELETE", "/v1/admin/api_keys/"+firstKeyID, "", headers)
	if w.Code != 200 {
		t.Fatalf("revoke = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 5: Now second key should succeed
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", mkKey("key-2"), headers)
	if w.Code != 201 {
		t.Fatalf("second key after revoke = %d, want 201; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Flow 8: Dashboard + billing status flow
//
// Steps:
//   1. Auth sync owner
//   2. Get dashboard config → verify plan info
//   3. Get billing status → verify free plan has no subscription
//   4. Attempt billing checkout → 503 (Stripe not configured)
// ---------------------------------------------------------------------------

func TestFlow_DashboardAndBilling(t *testing.T) {
	cleanup(t)

	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_dash_owner","email":"dash@e2e.test","first_name":"Dash","last_name":"Test"}`, nil)
	if w.Code != 201 {
		t.Fatalf("owner sync = %d; body: %s", w.Code, w.Body.String())
	}
	headers := map[string]string{"X-User-ID": "e2e_dash_owner"}

	// Step 2: Dashboard config
	w = testutil.DoRequest(testRouter, "GET", "/v1/dashboard/config", "", headers)
	if w.Code != 200 {
		t.Fatalf("dashboard config = %d; body: %s", w.Code, w.Body.String())
	}
	dashResp := testutil.ParseJSON(t, w)
	if dashResp["plan"] == nil {
		t.Error("expected plan in dashboard config")
	}
	if dashResp["retention"] == nil {
		t.Error("expected retention in dashboard config")
	}
	if dashResp["preset_options"] == nil {
		t.Error("expected preset_options in dashboard config")
	}

	// Step 3: Billing status
	w = testutil.DoRequest(testRouter, "GET", "/v1/billing/status", "", headers)
	if w.Code != 200 {
		t.Fatalf("billing status = %d; body: %s", w.Code, w.Body.String())
	}
	billingResp := testutil.ParseJSON(t, w)
	if billingResp["has_subscription"] != false {
		t.Errorf("has_subscription = %v, want false", billingResp["has_subscription"])
	}
	if billingResp["stripe_configured"] != false {
		t.Errorf("stripe_configured = %v, want false (Stripe not configured in tests)", billingResp["stripe_configured"])
	}

	// Step 4: Billing checkout → 503 without Stripe
	checkoutBody := `{"plan":"pro","success_url":"https://example.com/ok","cancel_url":"https://example.com/cancel"}`
	w = testutil.DoRequest(testRouter, "POST", "/v1/billing/checkout", checkoutBody, headers)
	if w.Code != 503 {
		t.Fatalf("checkout without Stripe = %d, want 503; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Flow 9: Concurrent auth sync (idempotency)
//
// Steps:
//   1. Auth sync same user twice → first 201, second 200
//   2. Verify only one tenant/user created
// ---------------------------------------------------------------------------

func TestFlow_AuthSyncIdempotency(t *testing.T) {
	cleanup(t)

	body := `{"clerk_user_id":"e2e_idem_user","email":"idem@e2e.test","first_name":"Idem","last_name":"Test"}`

	// First call → 201 (created)
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync", body, nil)
	if w.Code != 201 {
		t.Fatalf("first sync = %d; body: %s", w.Code, w.Body.String())
	}
	first := testutil.ParseJSON(t, w)
	if first["is_new_user"] != true {
		t.Error("first sync should be is_new_user=true")
	}

	// Second call → 200 (existing)
	w = testutil.DoRequest(testRouter, "POST", "/v1/auth/sync", body, nil)
	if w.Code != 200 {
		t.Fatalf("second sync = %d; body: %s", w.Code, w.Body.String())
	}
	second := testutil.ParseJSON(t, w)
	if second["is_new_user"] != false {
		t.Error("second sync should be is_new_user=false")
	}

	// Verify exactly one tenant
	var tenantCount int64
	testDB.Model(&models.Tenant{}).Count(&tenantCount)
	if tenantCount != 1 {
		t.Errorf("tenant count = %d, want 1", tenantCount)
	}

	// Verify exactly one user
	var userCount int64
	testDB.Model(&models.User{}).Count(&userCount)
	if userCount != 1 {
		t.Errorf("user count = %d, want 1", userCount)
	}
}

// ---------------------------------------------------------------------------
// Flow 10: Cross-tenant isolation — users from different tenants cannot see
// each other's data
//
// Steps:
//   1. Create two tenants with owners
//   2. Each creates an API key
//   3. Seed usage for each
//   4. Verify each can only see their own keys, usage, budget
// ---------------------------------------------------------------------------

func TestFlow_CrossTenantIsolation(t *testing.T) {
	cleanup(t)

	// Tenant A
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_iso_a","email":"a@e2e.test","first_name":"Tenant","last_name":"A"}`, nil)
	if w.Code != 201 {
		t.Fatalf("tenant A sync = %d; body: %s", w.Code, w.Body.String())
	}
	headersA := map[string]string{"X-User-ID": "e2e_iso_a"}

	// Tenant B
	w = testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_iso_b","email":"b@e2e.test","first_name":"Tenant","last_name":"B"}`, nil)
	if w.Code != 201 {
		t.Fatalf("tenant B sync = %d; body: %s", w.Code, w.Body.String())
	}
	headersB := map[string]string{"X-User-ID": "e2e_iso_b"}

	keyBody := `{
		"label":"iso-key",
		"provider":"anthropic",
		"auth_method":"BROWSER_OAUTH",
		"billing_mode":"MONTHLY_SUBSCRIPTION"
	}`

	// Each creates a key
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", keyBody, headersA)
	if w.Code != 201 {
		t.Fatalf("tenant A key = %d; body: %s", w.Code, w.Body.String())
	}
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", keyBody, headersB)
	if w.Code != 201 {
		t.Fatalf("tenant B key = %d; body: %s", w.Code, w.Body.String())
	}

	// Seed usage for each tenant
	var userA, userB models.User
	testDB.Where("id = ?", "e2e_iso_a").First(&userA)
	testDB.Where("id = ?", "e2e_iso_b").First(&userB)

	testDB.Create(&models.UsageLog{
		TenantID: userA.TenantID, Provider: "anthropic", Model: "claude-3-opus",
		PromptTokens: 100, CompletionTokens: 50, RequestID: "iso_a_1",
		APIUsageBilled: true, CreatedAt: time.Now(),
	})
	testDB.Create(&models.UsageLog{
		TenantID: userB.TenantID, Provider: "anthropic", Model: "claude-3-opus",
		PromptTokens: 200, CompletionTokens: 100, RequestID: "iso_b_1",
		APIUsageBilled: true, CreatedAt: time.Now(),
	})

	// Tenant A lists keys → should only see 1
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", headersA)
	if w.Code != 200 {
		t.Fatalf("A list keys = %d", w.Code)
	}
	aKeys := testutil.ParseJSON(t, w)
	if aKeys["count"] != float64(1) {
		t.Errorf("tenant A key count = %v, want 1", aKeys["count"])
	}

	// Tenant B lists keys → should only see 1
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", headersB)
	if w.Code != 200 {
		t.Fatalf("B list keys = %d", w.Code)
	}
	bKeys := testutil.ParseJSON(t, w)
	if bKeys["count"] != float64(1) {
		t.Errorf("tenant B key count = %v, want 1", bKeys["count"])
	}

	// Tenant A lists users → only sees themselves
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/users", "", headersA)
	if w.Code != 200 {
		t.Fatalf("A list users = %d", w.Code)
	}
	aUsers := testutil.ParseJSON(t, w)
	if aUsers["total"] != float64(1) {
		t.Errorf("tenant A user count = %v, want 1", aUsers["total"])
	}

	// Tenant B lists users → only sees themselves
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/users", "", headersB)
	if w.Code != 200 {
		t.Fatalf("B list users = %d", w.Code)
	}
	bUsers := testutil.ParseJSON(t, w)
	if bUsers["total"] != float64(1) {
		t.Errorf("tenant B user count = %v, want 1", bUsers["total"])
	}
}
