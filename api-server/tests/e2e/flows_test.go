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

	srv := api.NewServer(
		cfg, pdb,
		nil, // rdb
		apiKeySvc, usageSvc, pricingEngine,
		nil, // providerKeySvc
		nil, // proxyHandler
		rateLimiter,
		stripeSvc,
		nil, // sandboxStripeSvc
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

// userHeaders returns the standard auth headers for authenticated requests.
func userHeaders(userID string, tenantID uint) map[string]string {
	return map[string]string{
		"X-User-ID":   userID,
		"X-Tenant-Id": fmt.Sprintf("%d", tenantID),
	}
}

// syncAndGetTenantID performs auth sync for a user and returns the tenant_id
// from the first membership in the response.
func syncAndGetTenantID(t *testing.T, body string) (map[string]interface{}, uint) {
	t.Helper()
	w := testutil.DoRequest(testRouter, "POST", "/v1/auth/sync", body, nil)
	if w.Code != 200 && w.Code != 201 {
		t.Fatalf("auth/sync = %d; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	memberships, ok := resp["memberships"].([]interface{})
	if !ok || len(memberships) == 0 {
		t.Fatal("no memberships in auth sync response")
	}
	first := memberships[0].(map[string]interface{})
	return resp, uint(first["tenant_id"].(float64))
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

// mkKeyBody returns a JSON body for creating an API key in the given project.
func mkKeyBody(label string, projectID uint) string {
	return fmt.Sprintf(`{
		"label":%q,
		"project_id":%d,
		"provider":"anthropic",
		"auth_method":"BROWSER_OAUTH",
		"billing_mode":"MONTHLY_SUBSCRIPTION"
	}`, label, projectID)
}

// ---------------------------------------------------------------------------
// Flow 1: Full user onboarding → API key lifecycle
//
// Steps:
//   1. Auth sync (new user) → creates tenant + owner + default project
//   2. Create API key (with project_id)
//   3. List API keys → verify key appears
//   4. Revoke API key
//   5. List API keys → verify empty
// ---------------------------------------------------------------------------

func TestFlow_UserOnboarding_APIKeyLifecycle(t *testing.T) {
	cleanup(t)

	// Step 1: New user signs up via auth/sync
	syncResp, tenantID := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_owner_1","email":"owner@e2e.test","first_name":"E2E","last_name":"Owner"}`)
	if syncResp["is_new_user"] != true {
		t.Fatalf("expected is_new_user=true")
	}
	memberships := syncResp["memberships"].([]interface{})
	firstMem := memberships[0].(map[string]interface{})
	if firstMem["org_role"] != "owner" {
		t.Fatalf("expected owner role, got %v", firstMem["org_role"])
	}

	headers := userHeaders("e2e_owner_1", tenantID)
	projectID := getDefaultProjectID(t, tenantID)

	// Step 2: Create an API key bound to the default project.
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", mkKeyBody("e2e-test-key", projectID), headers)
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
	if createResp["project_id"] != float64(projectID) {
		t.Errorf("project_id = %v, want %d", createResp["project_id"], projectID)
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
//   2. Upgrade to team plan
//   3. Invite a new user (editor)
//   4. List users → pending invite visible
//   5. Invited user signs up via auth/sync → joins as editor
//   6. Owner changes their role to viewer
//   7. Owner suspends them
//   8. Suspended user cannot access viewer endpoints
//   9. Owner unsuspends them
//  10. User can access viewer endpoints again
// ---------------------------------------------------------------------------

func TestFlow_TeamManagement(t *testing.T) {
	cleanup(t)

	// Step 1: Owner signs up
	_, tenantID := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_tm_owner","email":"tm-owner@e2e.test","first_name":"TM","last_name":"Owner"}`)
	ownerHeaders := userHeaders("e2e_tm_owner", tenantID)

	// Step 2: Upgrade tenant to team plan (free only allows 1 member)
	testDB.Model(&models.Tenant{}).Where("id = ?", tenantID).Update("plan", models.PlanTeam)

	// Step 3: Invite a new team member as editor
	inviteBody := `{"email":"tm-editor@e2e.test","name":"Team Editor","role":"editor"}`
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/users/invite", inviteBody, ownerHeaders)
	if w.Code != 201 {
		t.Fatalf("invite = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 4: List users → should have owner + pending invite
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/users", "", ownerHeaders)
	if w.Code != 200 {
		t.Fatalf("list users = %d; body: %s", w.Code, w.Body.String())
	}
	listResp := testutil.ParseJSON(t, w)
	total, _ := listResp["total"].(float64)
	if total != 2 {
		t.Errorf("total users = %v, want 2 (owner + pending)", total)
	}

	// Step 5: Invited user signs up — gets their memberships
	w = testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_tm_editor","email":"tm-editor@e2e.test","first_name":"Team","last_name":"Editor"}`, nil)
	if w.Code != 200 {
		t.Fatalf("editor sync = %d; body: %s", w.Code, w.Body.String())
	}
	editorSync := testutil.ParseJSON(t, w)
	// Should have at least 2 memberships: invited tenant (editor) + personal tenant (owner)
	editorMemberships := editorSync["memberships"].([]interface{})
	if len(editorMemberships) < 2 {
		t.Fatalf("expected at least 2 memberships, got %d", len(editorMemberships))
	}
	// Find the editor role in the team tenant
	var editorRole string
	for _, m := range editorMemberships {
		mem := m.(map[string]interface{})
		if uint(mem["tenant_id"].(float64)) == tenantID {
			editorRole = mem["org_role"].(string)
		}
	}
	if editorRole != "editor" {
		t.Errorf("editor role in team tenant = %v, want editor", editorRole)
	}

	editorHeaders := userHeaders("e2e_tm_editor", tenantID)

	// Editor should be able to access admin endpoints
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", editorHeaders)
	if w.Code != 200 {
		t.Fatalf("editor access admin = %d, want 200", w.Code)
	}

	// Step 6: Owner changes editor's role to viewer
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

	// Step 7: Owner suspends the user
	w = testutil.DoRequest(testRouter, "PATCH", "/v1/admin/users/e2e_tm_editor/suspend", "", ownerHeaders)
	if w.Code != 200 {
		t.Fatalf("suspend = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 8: Suspended user cannot access anything
	w = testutil.DoRequest(testRouter, "GET", "/v1/usage", "", editorHeaders)
	if w.Code != 403 {
		t.Fatalf("suspended user access = %d, want 403", w.Code)
	}

	// Step 9: Owner unsuspends the user
	w = testutil.DoRequest(testRouter, "PATCH", "/v1/admin/users/e2e_tm_editor/unsuspend", "", ownerHeaders)
	if w.Code != 200 {
		t.Fatalf("unsuspend = %d; body: %s", w.Code, w.Body.String())
	}

	// Step 10: User can access again
	w = testutil.DoRequest(testRouter, "GET", "/v1/usage", "", editorHeaders)
	if w.Code != 200 {
		t.Fatalf("unsuspended user access = %d, want 200", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Flow 3: Usage tracking — create key, seed usage, view summary/ledger
// ---------------------------------------------------------------------------

func TestFlow_UsageTracking(t *testing.T) {
	cleanup(t)

	// Step 1: Owner signs up
	_, tenantID := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_usage_owner","email":"usage@e2e.test","first_name":"Usage","last_name":"Test"}`)
	headers := userHeaders("e2e_usage_owner", tenantID)
	projectID := getDefaultProjectID(t, tenantID)

	// Step 2: Create an API key
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", mkKeyBody("usage-track-key", projectID), headers)
	if w.Code != 201 {
		t.Fatalf("create key = %d; body: %s", w.Code, w.Body.String())
	}
	createResp := testutil.ParseJSON(t, w)
	keyID := createResp["key_id"].(string)

	// Step 3: Seed usage data
	now := time.Now()
	logs := []models.UsageLog{
		{
			TenantID: tenantID, KeyID: keyID, ProjectID: projectID,
			Provider: "anthropic", Model: "claude-3-opus",
			PromptTokens: 1000, CompletionTokens: 500,
			RequestID: "e2e_req_1", APIUsageBilled: true,
			CreatedAt: now,
		},
		{
			TenantID: tenantID, KeyID: keyID, ProjectID: projectID,
			Provider: "anthropic", Model: "claude-3-sonnet",
			PromptTokens: 2000, CompletionTokens: 1000,
			RequestID: "e2e_req_2", APIUsageBilled: true,
			CreatedAt: now.Add(-1 * time.Hour),
		},
		{
			TenantID: tenantID, KeyID: keyID, ProjectID: projectID,
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
// ---------------------------------------------------------------------------

func TestFlow_BudgetManagement(t *testing.T) {
	cleanup(t)

	_, tenantID := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_budget_owner","email":"budget@e2e.test","first_name":"Budget","last_name":"Test"}`)
	headers := userHeaders("e2e_budget_owner", tenantID)

	// Get budget → empty
	w := testutil.DoRequest(testRouter, "GET", "/v1/budget", "", headers)
	if w.Code != 200 {
		t.Fatalf("get budget = %d; body: %s", w.Code, w.Body.String())
	}

	// Create monthly budget
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
	budgetID := budgetResp["ID"]
	if budgetID == nil || budgetID == float64(0) {
		t.Fatal("expected budget ID in response")
	}

	// Get budget → verify limit
	w = testutil.DoRequest(testRouter, "GET", "/v1/budget", "", headers)
	if w.Code != 200 {
		t.Fatalf("get budget = %d; body: %s", w.Code, w.Body.String())
	}
	getBudget := testutil.ParseJSON(t, w)
	limits, ok := getBudget["budget_limits"].([]interface{})
	if !ok || len(limits) == 0 {
		t.Fatal("expected budget_limits in response")
	}

	// Update budget amount
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

	// Delete budget
	idStr := fmt.Sprintf("%.0f", budgetID.(float64))
	w = testutil.DoRequest(testRouter, "DELETE", "/v1/admin/budget/"+idStr, "", headers)
	if w.Code != 200 {
		t.Fatalf("delete budget = %d; body: %s", w.Code, w.Body.String())
	}

	// Get budget → should be empty now
	w = testutil.DoRequest(testRouter, "GET", "/v1/budget", "", headers)
	if w.Code != 200 {
		t.Fatalf("get budget after delete = %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Flow 5: Rate limit CRUD
// ---------------------------------------------------------------------------

func TestFlow_RateLimitCRUD(t *testing.T) {
	cleanup(t)

	_, tenantID := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_rl_owner","email":"rl@e2e.test","first_name":"RL","last_name":"Test"}`)
	headers := userHeaders("e2e_rl_owner", tenantID)

	// List rate limits → empty
	w := testutil.DoRequest(testRouter, "GET", "/v1/admin/rate-limits", "", headers)
	if w.Code != 200 {
		t.Fatalf("list rate limits = %d; body: %s", w.Code, w.Body.String())
	}

	// Create RPM rate limit
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
	rlID := rlResp["ID"]
	if rlID == nil || rlID == float64(0) {
		t.Fatal("expected rate limit ID in response")
	}

	// List rate limits → 1 entry
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/rate-limits", "", headers)
	if w.Code != 200 {
		t.Fatalf("list rate limits = %d", w.Code)
	}
	listResp := testutil.ParseJSON(t, w)
	rls, ok := listResp["rate_limits"].([]interface{})
	if !ok || len(rls) != 1 {
		t.Errorf("expected 1 rate limit, got %d", len(rls))
	}

	// Delete rate limit
	idStr := fmt.Sprintf("%.0f", rlID.(float64))
	w = testutil.DoRequest(testRouter, "DELETE", "/v1/admin/rate-limits/"+idStr, "", headers)
	if w.Code != 200 {
		t.Fatalf("delete rate limit = %d; body: %s", w.Code, w.Body.String())
	}

	// List rate limits → empty
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
// ---------------------------------------------------------------------------

func TestFlow_RBACEscalation(t *testing.T) {
	cleanup(t)

	// Owner signs up
	_, tenantID := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_rbac_owner","email":"rbac-owner@e2e.test","first_name":"RBAC","last_name":"Owner"}`)
	ownerH := userHeaders("e2e_rbac_owner", tenantID)

	// Upgrade tenant to team plan (free only allows 1 member)
	testDB.Model(&models.Tenant{}).Where("id = ?", tenantID).Update("plan", models.PlanTeam)

	// Invite editor
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/users/invite",
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
	editorH := userHeaders("e2e_rbac_editor", tenantID)

	// Viewer signs up
	w = testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_rbac_viewer","email":"rbac-viewer@e2e.test"}`, nil)
	if w.Code != 200 {
		t.Fatalf("viewer sync = %d; body: %s", w.Code, w.Body.String())
	}
	viewerH := userHeaders("e2e_rbac_viewer", tenantID)

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
// ---------------------------------------------------------------------------

func TestFlow_PlanLimitEnforcement(t *testing.T) {
	cleanup(t)

	_, tenantID := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_plan_owner","email":"plan@e2e.test","first_name":"Plan","last_name":"Test"}`)
	// Auth/sync creates free plan tenant. Free plan allows max 1 API key.
	projectID := getDefaultProjectID(t, tenantID)
	headers := userHeaders("e2e_plan_owner", tenantID)

	// First key → success
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", mkKeyBody("key-1", projectID), headers)
	if w.Code != 201 {
		t.Fatalf("first key = %d; body: %s", w.Code, w.Body.String())
	}
	firstResp := testutil.ParseJSON(t, w)
	firstKeyID := firstResp["key_id"].(string)

	// Second key → plan limit
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", mkKeyBody("key-2", projectID), headers)
	if w.Code != 422 {
		t.Fatalf("second key = %d, want 422; body: %s", w.Code, w.Body.String())
	}
	errResp := testutil.ParseJSON(t, w)
	if errResp["error"] != "plan_limit_reached" {
		t.Errorf("error = %v, want plan_limit_reached", errResp["error"])
	}

	// Revoke first key
	w = testutil.DoRequest(testRouter, "DELETE", "/v1/admin/api_keys/"+firstKeyID, "", headers)
	if w.Code != 200 {
		t.Fatalf("revoke = %d; body: %s", w.Code, w.Body.String())
	}

	// Now second key should succeed
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", mkKeyBody("key-2", projectID), headers)
	if w.Code != 201 {
		t.Fatalf("second key after revoke = %d, want 201; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Flow 8: Dashboard + billing status flow
// ---------------------------------------------------------------------------

func TestFlow_DashboardAndBilling(t *testing.T) {
	cleanup(t)

	_, tenantID := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_dash_owner","email":"dash@e2e.test","first_name":"Dash","last_name":"Test"}`)
	headers := userHeaders("e2e_dash_owner", tenantID)

	// Dashboard config
	w := testutil.DoRequest(testRouter, "GET", "/v1/dashboard/config", "", headers)
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

	// Billing status
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

	// Billing checkout → 503 without Stripe
	checkoutBody := `{"plan":"pro","success_url":"https://example.com/ok","cancel_url":"https://example.com/cancel"}`
	w = testutil.DoRequest(testRouter, "POST", "/v1/billing/checkout", checkoutBody, headers)
	if w.Code != 503 {
		t.Fatalf("checkout without Stripe = %d, want 503; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Flow 9: Auth sync idempotency
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

	// Verify exactly one membership
	var membershipCount int64
	testDB.Model(&models.TenantMembership{}).Count(&membershipCount)
	if membershipCount != 1 {
		t.Errorf("membership count = %d, want 1", membershipCount)
	}
}

// ---------------------------------------------------------------------------
// Flow 10: Cross-tenant isolation
// ---------------------------------------------------------------------------

func TestFlow_CrossTenantIsolation(t *testing.T) {
	cleanup(t)

	// Tenant A
	_, tenantIDA := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_iso_a","email":"a@e2e.test","first_name":"Tenant","last_name":"A"}`)
	headersA := userHeaders("e2e_iso_a", tenantIDA)
	projectIDA := getDefaultProjectID(t, tenantIDA)

	// Tenant B
	_, tenantIDB := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_iso_b","email":"b@e2e.test","first_name":"Tenant","last_name":"B"}`)
	headersB := userHeaders("e2e_iso_b", tenantIDB)
	projectIDB := getDefaultProjectID(t, tenantIDB)

	// Each creates a key
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", mkKeyBody("iso-key", projectIDA), headersA)
	if w.Code != 201 {
		t.Fatalf("tenant A key = %d; body: %s", w.Code, w.Body.String())
	}
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", mkKeyBody("iso-key", projectIDB), headersB)
	if w.Code != 201 {
		t.Fatalf("tenant B key = %d; body: %s", w.Code, w.Body.String())
	}

	// Seed usage for each tenant
	testDB.Create(&models.UsageLog{
		TenantID: tenantIDA, ProjectID: projectIDA, Provider: "anthropic", Model: "claude-3-opus",
		PromptTokens: 100, CompletionTokens: 50, RequestID: "iso_a_1",
		APIUsageBilled: true, CreatedAt: time.Now(),
	})
	testDB.Create(&models.UsageLog{
		TenantID: tenantIDB, ProjectID: projectIDB, Provider: "anthropic", Model: "claude-3-opus",
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

	// Tenant A cannot access Tenant B's data via X-Tenant-Id spoofing
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", userHeaders("e2e_iso_a", tenantIDB))
	if w.Code != 403 {
		t.Fatalf("cross-tenant access = %d, want 403", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Flow 11: Project lifecycle with API key binding
//
// Steps:
//   1. Owner signs up, upgrade to team plan
//   2. Create a new project
//   3. Create an API key bound to the new project
//   4. List keys → key shows correct project_id
//   5. Try to delete project → blocked by active keys
//   6. Revoke the key
//   7. Delete the project → success
//   8. Verify default project cannot be deleted
// ---------------------------------------------------------------------------

func TestFlow_ProjectLifecycle(t *testing.T) {
	cleanup(t)

	_, tenantID := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_proj_owner","email":"proj@e2e.test","first_name":"Proj","last_name":"Owner"}`)
	testDB.Model(&models.Tenant{}).Where("id = ?", tenantID).Update("plan", models.PlanTeam)
	headers := userHeaders("e2e_proj_owner", tenantID)

	// List projects → should have 1 (Default)
	w := testutil.DoRequest(testRouter, "GET", "/v1/projects", "", headers)
	if w.Code != 200 {
		t.Fatalf("list projects = %d; body: %s", w.Code, w.Body.String())
	}
	listResp := testutil.ParseJSON(t, w)
	if listResp["count"] != float64(1) {
		t.Errorf("initial project count = %v, want 1", listResp["count"])
	}

	// Create a new project
	w = testutil.DoRequest(testRouter, "POST", "/v1/projects",
		`{"name":"Backend","description":"Backend services"}`, headers)
	if w.Code != 201 {
		t.Fatalf("create project = %d; body: %s", w.Code, w.Body.String())
	}
	projResp := testutil.ParseJSON(t, w)
	projectID := uint(projResp["id"].(float64))
	if projResp["name"] != "Backend" {
		t.Errorf("project name = %v, want Backend", projResp["name"])
	}
	if projResp["is_default"] != false {
		t.Errorf("is_default = %v, want false", projResp["is_default"])
	}

	// Create an API key bound to the new project
	w = testutil.DoRequest(testRouter, "POST", "/v1/admin/api_keys", mkKeyBody("proj-key", projectID), headers)
	if w.Code != 201 {
		t.Fatalf("create key = %d; body: %s", w.Code, w.Body.String())
	}
	keyResp := testutil.ParseJSON(t, w)
	keyID := keyResp["key_id"].(string)
	if keyResp["project_id"] != float64(projectID) {
		t.Errorf("key project_id = %v, want %d", keyResp["project_id"], projectID)
	}

	// Try to delete the project → blocked (active key)
	w = testutil.DoRequest(testRouter, "DELETE", fmt.Sprintf("/v1/projects/%d", projectID), "", headers)
	if w.Code != 409 {
		t.Fatalf("delete project with key = %d, want 409; body: %s", w.Code, w.Body.String())
	}

	// Revoke the key
	w = testutil.DoRequest(testRouter, "DELETE", "/v1/admin/api_keys/"+keyID, "", headers)
	if w.Code != 200 {
		t.Fatalf("revoke key = %d; body: %s", w.Code, w.Body.String())
	}

	// Now delete the project → success
	w = testutil.DoRequest(testRouter, "DELETE", fmt.Sprintf("/v1/projects/%d", projectID), "", headers)
	if w.Code != 200 {
		t.Fatalf("delete project = %d; body: %s", w.Code, w.Body.String())
	}

	// Verify default project cannot be deleted
	defaultProjectID := getDefaultProjectID(t, tenantID)
	w = testutil.DoRequest(testRouter, "DELETE", fmt.Sprintf("/v1/projects/%d", defaultProjectID), "", headers)
	if w.Code != 409 {
		t.Fatalf("delete default project = %d, want 409; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Flow 12: Multi-tenant membership — user belongs to multiple tenants
//
// Steps:
//   1. Owner A signs up (gets personal tenant)
//   2. Owner B signs up (gets personal tenant)
//   3. Owner A upgrades to team, invites Owner B
//   4. Owner B syncs → now has 2 memberships
//   5. Owner B can access tenant A's data with X-Tenant-Id = A
//   6. Owner B can access their own data with X-Tenant-Id = B
//   7. Owner B cannot access tenant A as owner (they are editor in A)
// ---------------------------------------------------------------------------

func TestFlow_MultiTenantMembership(t *testing.T) {
	cleanup(t)

	// Owner A signs up
	_, tenantIDA := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_mt_a","email":"mt-a@e2e.test","first_name":"MT","last_name":"A"}`)
	testDB.Model(&models.Tenant{}).Where("id = ?", tenantIDA).Update("plan", models.PlanTeam)
	headersA := userHeaders("e2e_mt_a", tenantIDA)

	// Owner B signs up
	syncRespB, tenantIDB := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_mt_b","email":"mt-b@e2e.test","first_name":"MT","last_name":"B"}`)
	_ = syncRespB

	// Owner A invites Owner B as editor
	w := testutil.DoRequest(testRouter, "POST", "/v1/admin/users/invite",
		`{"email":"mt-b@e2e.test","role":"editor"}`, headersA)
	if w.Code != 201 {
		t.Fatalf("invite = %d; body: %s", w.Code, w.Body.String())
	}

	// Owner B re-syncs to pick up the activated membership
	w = testutil.DoRequest(testRouter, "POST", "/v1/auth/sync",
		`{"clerk_user_id":"e2e_mt_b","email":"mt-b@e2e.test"}`, nil)
	if w.Code != 200 {
		t.Fatalf("re-sync = %d; body: %s", w.Code, w.Body.String())
	}
	reSyncResp := testutil.ParseJSON(t, w)
	memberships := reSyncResp["memberships"].([]interface{})
	if len(memberships) < 2 {
		t.Fatalf("expected at least 2 memberships, got %d", len(memberships))
	}

	// Owner B can access tenant A's admin keys (as editor)
	headersBinA := userHeaders("e2e_mt_b", tenantIDA)
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", headersBinA)
	if w.Code != 200 {
		t.Fatalf("B in A list keys = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	// Owner B can access their own tenant
	headersBinB := userHeaders("e2e_mt_b", tenantIDB)
	w = testutil.DoRequest(testRouter, "GET", "/v1/admin/api_keys", "", headersBinB)
	if w.Code != 200 {
		t.Fatalf("B in B list keys = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	// Owner B cannot access owner routes in tenant A (they are editor, not owner)
	w = testutil.DoRequest(testRouter, "GET", "/v1/owner/settings", "", headersBinA)
	if w.Code != 403 {
		t.Fatalf("B owner route in A = %d, want 403", w.Code)
	}

	// But Owner B CAN access owner routes in their own tenant B (they are owner)
	w = testutil.DoRequest(testRouter, "GET", "/v1/owner/settings", "", headersBinB)
	if w.Code != 200 {
		t.Fatalf("B owner route in B = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Flow 13: Audit log tracking
//
// Steps:
//   1. Owner signs up
//   2. Seed audit log entries
//   3. Query audit logs (admin only)
//   4. Verify filtering works
// ---------------------------------------------------------------------------

func TestFlow_AuditLogTracking(t *testing.T) {
	cleanup(t)

	_, tenantID := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_audit_owner","email":"audit@e2e.test","first_name":"Audit","last_name":"Owner"}`)
	headers := userHeaders("e2e_audit_owner", tenantID)

	// Seed audit log entries directly
	testDB.Create(&models.AuditLog{
		TenantID: tenantID, ActorUserID: "e2e_audit_owner",
		Action: models.AuditAPIKeyCreated, ResourceType: "api_key", ResourceID: "key_1",
		Category: models.AuditCategoryAccess, ActorType: models.AuditActorUser,
		Success: true, CreatedAt: time.Now(),
	})
	testDB.Create(&models.AuditLog{
		TenantID: tenantID, ActorUserID: "e2e_audit_owner",
		Action: models.AuditProjectCreated, ResourceType: "project", ResourceID: "proj_1",
		Category: models.AuditCategoryProject, ActorType: models.AuditActorUser,
		Success: true, CreatedAt: time.Now().Add(-1 * time.Hour),
	})
	testDB.Create(&models.AuditLog{
		TenantID: tenantID, ActorUserID: "e2e_audit_owner",
		Action: models.AuditAPIKeyRevoked, ResourceType: "api_key", ResourceID: "key_1",
		Category: models.AuditCategoryAccess, ActorType: models.AuditActorUser,
		Success: true, CreatedAt: time.Now().Add(-2 * time.Hour),
	})

	// Query all audit logs
	w := testutil.DoRequest(testRouter, "GET", "/v1/audit-logs", "", headers)
	if w.Code != 200 {
		t.Fatalf("audit logs = %d; body: %s", w.Code, w.Body.String())
	}
	resp := testutil.ParseJSON(t, w)
	logs, ok := resp["audit_logs"].([]interface{})
	if !ok {
		t.Fatal("expected audit_logs array")
	}
	if len(logs) != 3 {
		t.Errorf("audit log count = %d, want 3", len(logs))
	}

	// Filter by action
	w = testutil.DoRequest(testRouter, "GET", "/v1/audit-logs?action=API_KEY.CREATED", "", headers)
	if w.Code != 200 {
		t.Fatalf("filtered audit = %d; body: %s", w.Code, w.Body.String())
	}
	resp = testutil.ParseJSON(t, w)
	logs = resp["audit_logs"].([]interface{})
	if len(logs) != 1 {
		t.Errorf("filtered API_KEY.CREATED count = %d, want 1", len(logs))
	}

	// Filter by resource_type
	w = testutil.DoRequest(testRouter, "GET", "/v1/audit-logs?resource_type=project", "", headers)
	if w.Code != 200 {
		t.Fatalf("filtered by resource = %d; body: %s", w.Code, w.Body.String())
	}
	resp = testutil.ParseJSON(t, w)
	logs = resp["audit_logs"].([]interface{})
	if len(logs) != 1 {
		t.Errorf("filtered project count = %d, want 1", len(logs))
	}

	// Limit
	w = testutil.DoRequest(testRouter, "GET", "/v1/audit-logs?limit=2", "", headers)
	if w.Code != 200 {
		t.Fatalf("limited audit = %d; body: %s", w.Code, w.Body.String())
	}
	resp = testutil.ParseJSON(t, w)
	logs = resp["audit_logs"].([]interface{})
	if len(logs) != 2 {
		t.Errorf("limited count = %d, want 2", len(logs))
	}
}

// ---------------------------------------------------------------------------
// Flow 14: Project plan limit enforcement
//
// Steps:
//   1. Owner signs up (free plan → max 1 project, already has Default)
//   2. Try to create another project → 422 plan_limit_reached
//   3. Upgrade to team plan
//   4. Create project → success
// ---------------------------------------------------------------------------

func TestFlow_ProjectPlanLimit(t *testing.T) {
	cleanup(t)

	_, tenantID := syncAndGetTenantID(t,
		`{"clerk_user_id":"e2e_pp_owner","email":"pp@e2e.test","first_name":"PP","last_name":"Owner"}`)
	headers := userHeaders("e2e_pp_owner", tenantID)

	// Free plan: already have Default project (max 1).
	w := testutil.DoRequest(testRouter, "POST", "/v1/projects", `{"name":"Extra"}`, headers)
	if w.Code != 422 {
		t.Fatalf("create project on free = %d, want 422; body: %s", w.Code, w.Body.String())
	}
	errResp := testutil.ParseJSON(t, w)
	if errResp["error"] != "plan_limit_reached" {
		t.Errorf("error = %v, want plan_limit_reached", errResp["error"])
	}

	// Upgrade to team plan
	testDB.Model(&models.Tenant{}).Where("id = ?", tenantID).Update("plan", models.PlanTeam)

	// Now can create a project
	w = testutil.DoRequest(testRouter, "POST", "/v1/projects", `{"name":"Extra"}`, headers)
	if w.Code != 201 {
		t.Fatalf("create project after upgrade = %d; body: %s", w.Code, w.Body.String())
	}
}
