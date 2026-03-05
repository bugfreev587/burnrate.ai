//go:build integration

package integration

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v82"

	"github.com/xiaoboyu/tokengate/api-server/internal/api"
	"github.com/xiaoboyu/tokengate/api-server/internal/config"
	"github.com/xiaoboyu/tokengate/api-server/internal/db"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/pricing"
	"github.com/xiaoboyu/tokengate/api-server/internal/ratelimit"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
	"github.com/xiaoboyu/tokengate/api-server/internal/testutil"
)

const testWebhookSecret = "whsec_test_secret_for_integration"

// webhookRouter creates a router with Stripe configured (webhook secret set).
func webhookRouter() http.Handler {
	cfg := &config.Config{
		Server: config.ServerCfg{
			Host:        "127.0.0.1",
			Port:        "0",
			CORSOrigins: []string{"*"},
		},
	}

	stripeSvc := services.NewStripeService(testDB, config.StripeCfg{
		SecretKey:     "sk_test_fake",
		WebhookSecret: testWebhookSecret,
	})
	apiKeySvc := services.NewAPIKeyService(testDB, testPepper, nil, 0)
	usageSvc := services.NewUsageLogService(testDB)
	pricingEngine := pricing.NewPricingEngine(testDB, nil)
	rateLimiter := ratelimit.NewLimiter(testDB, nil)
	pdb := db.NewFromDB(testDB)

	auditLogSvc := services.NewAuditLogService(testDB)
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
	return srv.Router()
}

// signPayload generates a valid Stripe webhook signature for the given payload.
func signPayload(t *testing.T, payload string) string {
	t.Helper()
	ts := fmt.Sprintf("%d", time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(testWebhookSecret))
	mac.Write([]byte(ts + "." + payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%s,v1=%s", ts, sig)
}

// buildInvoiceEvent builds a Stripe webhook event JSON for invoice events.
func buildInvoiceEvent(eventID, eventType, subscriptionID string) string {
	inv := map[string]interface{}{
		"id": "inv_test_123",
		"parent": map[string]interface{}{
			"type": "subscription",
			"subscription_details": map[string]interface{}{
				"subscription": map[string]interface{}{
					"id": subscriptionID,
				},
			},
		},
	}
	invJSON, _ := json.Marshal(inv)

	event := map[string]interface{}{
		"id":          eventID,
		"type":        eventType,
		"api_version": stripe.APIVersion,
		"data": map[string]interface{}{
			"object": json.RawMessage(invJSON),
		},
	}
	eventJSON, _ := json.Marshal(event)
	return string(eventJSON)
}

// seedTenantWithSubscription creates a tenant with a Stripe subscription ID.
func seedTenantWithSubscription(t *testing.T, name, ownerID, email, subscriptionID, plan string) models.Tenant {
	t.Helper()
	tenant, _ := testutil.SeedTenant(t, testDB, name, ownerID, email)
	testDB.Model(&tenant).Updates(map[string]interface{}{
		"stripe_subscription_id": subscriptionID,
		"stripe_customer_id":     "cus_test_" + ownerID,
		"plan":                   plan,
		"plan_status":            models.PlanStatusActive,
	})
	return tenant
}

// getTenantPlanStatus fetches the current plan_status for a tenant.
func getTenantPlanStatus(t *testing.T, tenantID uint) string {
	t.Helper()
	var tenant models.Tenant
	if err := testDB.First(&tenant, tenantID).Error; err != nil {
		t.Fatalf("fetch tenant %d: %v", tenantID, err)
	}
	return tenant.PlanStatus
}

// ---------------------------------------------------------------------------
// Test: invoice.payment_failed → plan_status becomes past_due
// ---------------------------------------------------------------------------

func TestWebhook_InvoicePaymentFailed(t *testing.T) {
	cleanup(t)
	router := webhookRouter()

	subID := "sub_fail_test"
	tenant := seedTenantWithSubscription(t, "FailCo", "user_wh1", "wh1@example.com", subID, models.PlanPro)

	// Verify starting state
	if status := getTenantPlanStatus(t, tenant.ID); status != models.PlanStatusActive {
		t.Fatalf("precondition: plan_status = %q, want %q", status, models.PlanStatusActive)
	}

	payload := buildInvoiceEvent("evt_fail_001", "invoice.payment_failed", subID)
	sig := signPayload(t, payload)

	w := testutil.DoRequest(router, "POST", "/v1/billing/webhook", payload, map[string]string{
		"Stripe-Signature": sig,
	})
	if w.Code != 200 {
		t.Fatalf("POST /v1/billing/webhook = %d; body: %s", w.Code, w.Body.String())
	}

	// Verify plan_status changed to past_due
	if status := getTenantPlanStatus(t, tenant.ID); status != models.PlanStatusPastDue {
		t.Errorf("after invoice.payment_failed: plan_status = %q, want %q", status, models.PlanStatusPastDue)
	}
}

// ---------------------------------------------------------------------------
// Test: invoice.paid → plan_status recovers to active
// ---------------------------------------------------------------------------

func TestWebhook_InvoicePaid_RecoverFromPastDue(t *testing.T) {
	cleanup(t)
	router := webhookRouter()

	subID := "sub_recover_test"
	tenant := seedTenantWithSubscription(t, "RecoverCo", "user_wh2", "wh2@example.com", subID, models.PlanPro)

	// Set tenant to past_due (simulating a prior failed payment)
	testDB.Model(&tenant).Update("plan_status", models.PlanStatusPastDue)
	if status := getTenantPlanStatus(t, tenant.ID); status != models.PlanStatusPastDue {
		t.Fatalf("precondition: plan_status = %q, want %q", status, models.PlanStatusPastDue)
	}

	payload := buildInvoiceEvent("evt_paid_001", "invoice.paid", subID)
	sig := signPayload(t, payload)

	w := testutil.DoRequest(router, "POST", "/v1/billing/webhook", payload, map[string]string{
		"Stripe-Signature": sig,
	})
	if w.Code != 200 {
		t.Fatalf("POST /v1/billing/webhook = %d; body: %s", w.Code, w.Body.String())
	}

	// Verify plan_status recovered to active
	if status := getTenantPlanStatus(t, tenant.ID); status != models.PlanStatusActive {
		t.Errorf("after invoice.paid: plan_status = %q, want %q", status, models.PlanStatusActive)
	}
}

// ---------------------------------------------------------------------------
// Test: Full lifecycle — active → past_due → active
// ---------------------------------------------------------------------------

func TestWebhook_InvoiceLifecycle(t *testing.T) {
	cleanup(t)
	router := webhookRouter()

	subID := "sub_lifecycle_test"
	tenant := seedTenantWithSubscription(t, "LifecycleCo", "user_wh3", "wh3@example.com", subID, models.PlanTeam)

	// Step 1: Verify starting state is active
	if status := getTenantPlanStatus(t, tenant.ID); status != models.PlanStatusActive {
		t.Fatalf("step 0: plan_status = %q, want %q", status, models.PlanStatusActive)
	}

	// Step 2: invoice.payment_failed → past_due
	payload1 := buildInvoiceEvent("evt_lc_fail", "invoice.payment_failed", subID)
	w1 := testutil.DoRequest(router, "POST", "/v1/billing/webhook", payload1, map[string]string{
		"Stripe-Signature": signPayload(t, payload1),
	})
	if w1.Code != 200 {
		t.Fatalf("step 1 (payment_failed): status %d; body: %s", w1.Code, w1.Body.String())
	}
	if status := getTenantPlanStatus(t, tenant.ID); status != models.PlanStatusPastDue {
		t.Errorf("step 1: plan_status = %q, want %q", status, models.PlanStatusPastDue)
	}

	// Step 3: invoice.paid → back to active
	payload2 := buildInvoiceEvent("evt_lc_paid", "invoice.paid", subID)
	w2 := testutil.DoRequest(router, "POST", "/v1/billing/webhook", payload2, map[string]string{
		"Stripe-Signature": signPayload(t, payload2),
	})
	if w2.Code != 200 {
		t.Fatalf("step 2 (paid): status %d; body: %s", w2.Code, w2.Body.String())
	}
	if status := getTenantPlanStatus(t, tenant.ID); status != models.PlanStatusActive {
		t.Errorf("step 2: plan_status = %q, want %q", status, models.PlanStatusActive)
	}
}

// ---------------------------------------------------------------------------
// Test: Idempotency — duplicate event is ignored
// ---------------------------------------------------------------------------

func TestWebhook_Idempotency(t *testing.T) {
	cleanup(t)
	router := webhookRouter()

	subID := "sub_idempotent_test"
	tenant := seedTenantWithSubscription(t, "IdempCo", "user_wh4", "wh4@example.com", subID, models.PlanPro)

	payload := buildInvoiceEvent("evt_dup_001", "invoice.payment_failed", subID)

	// First request → should process
	w1 := testutil.DoRequest(router, "POST", "/v1/billing/webhook", payload, map[string]string{
		"Stripe-Signature": signPayload(t, payload),
	})
	if w1.Code != 200 {
		t.Fatalf("first request: status %d; body: %s", w1.Code, w1.Body.String())
	}
	resp1 := testutil.ParseJSON(t, w1)
	if resp1["status"] != "ok" {
		t.Errorf("first request: status = %v, want ok", resp1["status"])
	}
	if status := getTenantPlanStatus(t, tenant.ID); status != models.PlanStatusPastDue {
		t.Fatalf("after first: plan_status = %q, want %q", status, models.PlanStatusPastDue)
	}

	// Manually reset to active to detect if second request processes
	testDB.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Update("plan_status", models.PlanStatusActive)

	// Second request (same event ID) → should be idempotent
	w2 := testutil.DoRequest(router, "POST", "/v1/billing/webhook", payload, map[string]string{
		"Stripe-Signature": signPayload(t, payload),
	})
	if w2.Code != 200 {
		t.Fatalf("second request: status %d; body: %s", w2.Code, w2.Body.String())
	}
	resp2 := testutil.ParseJSON(t, w2)
	if resp2["status"] != "already_processed" {
		t.Errorf("second request: status = %v, want already_processed", resp2["status"])
	}

	// plan_status should still be active (not re-set to past_due)
	if status := getTenantPlanStatus(t, tenant.ID); status != models.PlanStatusActive {
		t.Errorf("after duplicate: plan_status = %q, want %q (should not have re-processed)", status, models.PlanStatusActive)
	}
}

// ---------------------------------------------------------------------------
// Test: Invalid signature → 400
// ---------------------------------------------------------------------------

func TestWebhook_InvalidSignature(t *testing.T) {
	cleanup(t)
	router := webhookRouter()

	payload := buildInvoiceEvent("evt_badsig", "invoice.paid", "sub_any")
	w := testutil.DoRequest(router, "POST", "/v1/billing/webhook", payload, map[string]string{
		"Stripe-Signature": "t=123,v1=invalidsignature",
	})
	if w.Code != 400 {
		t.Errorf("invalid signature: status %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test: Unknown subscription → 200 OK (no-op, no error)
// ---------------------------------------------------------------------------

func TestWebhook_UnknownSubscription(t *testing.T) {
	cleanup(t)
	router := webhookRouter()

	payload := buildInvoiceEvent("evt_unknown_sub", "invoice.payment_failed", "sub_nonexistent")
	w := testutil.DoRequest(router, "POST", "/v1/billing/webhook", payload, map[string]string{
		"Stripe-Signature": signPayload(t, payload),
	})
	if w.Code != 200 {
		t.Errorf("unknown subscription: status %d, want 200; body: %s", w.Code, w.Body.String())
	}
}
