package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v82"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// validateRedirectURL ensures a URL is valid for use as a Stripe redirect target.
// It requires a non-empty host and https scheme (http is allowed in non-production).
func (s *Server) validateRedirectURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}
	isProd := s.cfg.Environment == "production" || s.cfg.Environment == "prod"
	if isProd && u.Scheme != "https" {
		return fmt.Errorf("URL must use https in production")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("URL must use http or https scheme")
	}
	return nil
}

// ── GET /v1/billing/status ───────────────────────────────────────────────────

func (s *Server) handleBillingStatus(c *gin.Context) {
	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}

	resp := gin.H{
		"plan":               tenant.Plan,
		"plan_status":        tenant.PlanStatus,
		"has_subscription":   tenant.StripeSubscriptionID != "",
		"billing_email":      tenant.BillingEmail,
		"current_period_end": tenant.CurrentPeriodEnd,
		"stripe_configured":  s.stripeSvc.IsConfigured(),
		"pending_plan":       tenant.PendingPlan,
		"plan_effective_at":  tenant.PlanEffectiveAt,
	}

	// Fetch payment method info from Stripe if configured and subscribed
	if s.stripeSvc.IsConfigured() && tenant.StripeSubscriptionID != "" {
		subInfo, err := s.stripeSvc.GetSubscription(c.Request.Context(), tenantID)
		if err != nil {
			slog.Error("billing_fetch_subscription_failed", "tenant_id", tenantID, "error", err)
		} else if subInfo != nil {
			resp["cancel_at_period_end"] = subInfo.CancelAtPeriodEnd
			if subInfo.PaymentMethodLast4 != "" {
				resp["payment_method"] = gin.H{
					"brand":     subInfo.PaymentMethodBrand,
					"last4":     subInfo.PaymentMethodLast4,
					"exp_month": subInfo.PaymentMethodExpMonth,
					"exp_year":  subInfo.PaymentMethodExpYear,
				}
			}

			// Self-heal: if Stripe's actual plan differs from DB (e.g. Portal
			// upgrade webhook was missed or failed), correct it on read.
			if subInfo.DetectedPlan != "" && subInfo.DetectedPlan != tenant.Plan && tenant.PendingPlan == "" {
				if err := s.postgresDB.GetDB().Model(&tenant).Updates(map[string]any{
					"plan": subInfo.DetectedPlan,
				}).Error; err != nil {
					slog.Error("billing_plan_sync_failed", "tenant_id", tenantID, "error", err)
				} else {
					slog.Info("billing_plan_synced_from_stripe",
						"tenant_id", tenantID,
						"old_plan", tenant.Plan,
						"new_plan", subInfo.DetectedPlan,
					)
					resp["plan"] = subInfo.DetectedPlan
				}
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}

// ── POST /v1/billing/checkout ────────────────────────────────────────────────

type checkoutRequest struct {
	Plan       string `json:"plan" binding:"required"`
	SuccessURL string `json:"success_url" binding:"required"`
	CancelURL  string `json:"cancel_url" binding:"required"`
}

func (s *Server) handleBillingCheckout(c *gin.Context) {
	if !s.stripeSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stripe is not configured"})
		return
	}

	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	var req checkoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !models.ValidPlan(req.Plan) || req.Plan == models.PlanFree {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan; must be pro, team, or business"})
		return
	}

	if err := s.validateRedirectURL(req.SuccessURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid success_url: %v", err)})
		return
	}
	if err := s.validateRedirectURL(req.CancelURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid cancel_url: %v", err)})
		return
	}

	// Reject if already subscribed
	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}
	if tenant.StripeSubscriptionID != "" {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "already_subscribed",
			"message": "Use the customer portal to manage your existing subscription.",
		})
		return
	}

	url, err := s.stripeSvc.CreateCheckoutSession(c.Request.Context(), tenantID, req.Plan, req.SuccessURL, req.CancelURL)
	if err != nil {
		slog.Error("billing_checkout_error", "tenant_id", tenantID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create checkout session"})
		return
	}

	s.recordAuditEvent(c, models.AuditBillingCheckout, "billing", fmt.Sprintf("%d", tenantID), AuditOpts{
		Category: models.AuditCategoryBilling,
		Metadata: map[string]interface{}{
			"plan": req.Plan,
		},
	})

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// ── POST /v1/billing/checkout/verify ─────────────────────────────────────────

type verifyCheckoutRequest struct {
	SessionID string `json:"session_id" binding:"required"`
}

func (s *Server) handleBillingCheckoutVerify(c *gin.Context) {
	if !s.stripeSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stripe is not configured"})
		return
	}

	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	var req verifyCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.stripeSvc.VerifyCheckoutSession(c.Request.Context(), tenantID, req.SessionID); err != nil {
		slog.Error("billing_checkout_verify_error", "tenant_id", tenantID, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to verify checkout session"})
		return
	}

	// Return updated tenant info
	var tenant models.Tenant
	s.postgresDB.GetDB().First(&tenant, tenantID)
	c.JSON(http.StatusOK, gin.H{
		"plan":        tenant.Plan,
		"plan_status": tenant.PlanStatus,
	})
}

// ── POST /v1/billing/portal ──────────────────────────────────────────────────

type portalRequest struct {
	ReturnURL string `json:"return_url" binding:"required"`
}

func (s *Server) handleBillingPortal(c *gin.Context) {
	if !s.stripeSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stripe is not configured"})
		return
	}

	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	var req portalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.validateRedirectURL(req.ReturnURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid return_url: %v", err)})
		return
	}

	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}
	if tenant.StripeCustomerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no billing account found; subscribe first"})
		return
	}

	url, err := s.stripeSvc.CreatePortalSession(c.Request.Context(), tenantID, req.ReturnURL)
	if err != nil {
		slog.Error("billing_portal_error", "tenant_id", tenantID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create portal session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// ── POST /v1/billing/downgrade ───────────────────────────────────────────────

type downgradeRequest struct {
	Plan string `json:"plan" binding:"required"`
}

func (s *Server) handleBillingDowngrade(c *gin.Context) {
	if !s.stripeSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stripe is not configured"})
		return
	}

	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	var req downgradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !models.ValidPlan(req.Plan) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan"})
		return
	}

	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}

	if !models.IsDowngrade(tenant.Plan, req.Plan) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target plan is not a downgrade; use change-plan for upgrades"})
		return
	}

	if tenant.PendingPlan != "" {
		c.JSON(http.StatusConflict, gin.H{"error": "a downgrade is already scheduled; cancel it first"})
		return
	}

	if err := s.stripeSvc.ScheduleDowngrade(c.Request.Context(), tenantID, req.Plan); err != nil {
		slog.Error("billing_downgrade_error", "tenant_id", tenantID, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.recordAuditEvent(c, models.AuditBillingDowngraded, "billing", fmt.Sprintf("%d", tenantID), AuditOpts{
		Category: models.AuditCategoryBilling,
		Metadata: map[string]interface{}{
			"from_plan": tenant.Plan,
			"to_plan":   req.Plan,
		},
	})

	s.postgresDB.GetDB().First(&tenant, tenantID)
	c.JSON(http.StatusOK, gin.H{
		"plan":              tenant.Plan,
		"plan_status":       tenant.PlanStatus,
		"pending_plan":      tenant.PendingPlan,
		"plan_effective_at": tenant.PlanEffectiveAt,
		"message":           fmt.Sprintf("Downgrade to %s scheduled at end of billing period.", req.Plan),
	})
}

// ── POST /v1/billing/downgrade/cancel ───────────────────────────────────────

func (s *Server) handleBillingCancelDowngrade(c *gin.Context) {
	if !s.stripeSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stripe is not configured"})
		return
	}

	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	if err := s.stripeSvc.CancelScheduledDowngrade(c.Request.Context(), tenantID); err != nil {
		slog.Error("billing_cancel_downgrade_error", "tenant_id", tenantID, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.recordAuditEvent(c, models.AuditBillingDowngradeCanceled, "billing", fmt.Sprintf("%d", tenantID), AuditOpts{
		Category: models.AuditCategoryBilling,
	})

	var tenant models.Tenant
	s.postgresDB.GetDB().First(&tenant, tenantID)
	c.JSON(http.StatusOK, gin.H{
		"plan":        tenant.Plan,
		"plan_status": tenant.PlanStatus,
		"message":     "Scheduled downgrade has been canceled.",
	})
}

// ── POST /v1/billing/change-plan ─────────────────────────────────────────────

type changeBillingPlanRequest struct {
	Plan string `json:"plan" binding:"required"`
}

func (s *Server) handleBillingChangePlan(c *gin.Context) {
	if !s.stripeSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stripe is not configured"})
		return
	}

	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	var req changeBillingPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !models.ValidPlan(req.Plan) || req.Plan == models.PlanFree {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan; must be pro, team, or business"})
		return
	}

	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}

	if tenant.StripeSubscriptionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active subscription to change; use checkout instead"})
		return
	}

	if tenant.Plan == req.Plan {
		c.JSON(http.StatusBadRequest, gin.H{"error": "already on this plan"})
		return
	}

	if models.IsDowngrade(tenant.Plan, req.Plan) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "use POST /v1/billing/downgrade for downgrades"})
		return
	}

	oldPlan := tenant.Plan

	if err := s.stripeSvc.ChangeSubscriptionPlan(c.Request.Context(), tenantID, req.Plan); err != nil {
		slog.Error("billing_change_plan_error", "tenant_id", tenantID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to change plan"})
		return
	}

	s.recordAuditEvent(c, models.AuditBillingPlanChanged, "billing", fmt.Sprintf("%d", tenantID), AuditOpts{
		Category: models.AuditCategoryBilling,
		BeforeState: map[string]interface{}{
			"plan": oldPlan,
		},
		AfterState: map[string]interface{}{
			"plan": req.Plan,
		},
	})

	s.postgresDB.GetDB().First(&tenant, tenantID)
	c.JSON(http.StatusOK, gin.H{
		"plan":        tenant.Plan,
		"plan_status": tenant.PlanStatus,
	})
}

// ── GET /v1/billing/invoices ─────────────────────────────────────────────────

func (s *Server) handleBillingInvoices(c *gin.Context) {
	if !s.stripeSvc.IsConfigured() {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	invoices, err := s.stripeSvc.ListInvoices(c.Request.Context(), tenantID, 24)
	if err != nil {
		slog.Error("billing_invoices_error", "tenant_id", tenantID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invoices"})
		return
	}

	c.JSON(http.StatusOK, invoices)
}

// ── POST /v1/billing/webhook ─────────────────────────────────────────────────

func (s *Server) handleBillingWebhook(c *gin.Context) {
	if !s.stripeSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stripe is not configured"})
		return
	}
	if !s.stripeSvc.IsWebhookConfigured() {
		slog.Warn("billing_webhook_secret_not_configured")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook secret not configured"})
		return
	}

	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, 65536))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	sigHeader := c.GetHeader("Stripe-Signature")
	event, err := s.stripeSvc.ConstructEvent(payload, sigHeader)
	if err != nil {
		slog.Warn("billing_webhook_signature_failed", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid signature"})
		return
	}

	// Idempotency check
	isNew, err := s.stripeSvc.MarkEventProcessed(c.Request.Context(), event.ID)
	if err != nil {
		slog.Error("billing_idempotency_check_error", "event_id", event.ID, "error", err)
		// Continue processing — better to double-process than to drop
	}
	if !isNew && err == nil {
		slog.Debug("billing_event_already_processed", "event_id", event.ID)
		c.JSON(http.StatusOK, gin.H{"status": "already_processed"})
		return
	}

	ctx := c.Request.Context()

	switch event.Type {
	case "checkout.session.completed":
		var sess stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
			slog.Error("billing_webhook_parse_error", "event_type", "checkout.session.completed", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event data"})
			return
		}
		if err := s.stripeSvc.HandleCheckoutCompleted(ctx, &sess); err != nil {
			slog.Error("billing_webhook_handler_error", "handler", "HandleCheckoutCompleted", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "handler error"})
			return
		}

	case "customer.subscription.created", "customer.subscription.updated":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			slog.Error("billing_webhook_parse_error", "event_type", event.Type, "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event data"})
			return
		}
		if err := s.stripeSvc.HandleSubscriptionUpdated(ctx, &sub); err != nil {
			slog.Error("billing_webhook_handler_error", "handler", "HandleSubscriptionUpdated", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "handler error"})
			return
		}

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			slog.Error("billing_webhook_parse_error", "event_type", "customer.subscription.deleted", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event data"})
			return
		}
		if err := s.stripeSvc.HandleSubscriptionDeleted(ctx, &sub); err != nil {
			slog.Error("billing_webhook_handler_error", "handler", "HandleSubscriptionDeleted", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "handler error"})
			return
		}

	case "invoice.paid":
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			slog.Error("billing_webhook_parse_error", "event_type", "invoice.paid", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event data"})
			return
		}
		if err := s.stripeSvc.HandleInvoicePaid(ctx, &inv); err != nil {
			slog.Error("billing_webhook_handler_error", "handler", "HandleInvoicePaid", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "handler error"})
			return
		}

	case "invoice.payment_failed":
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			slog.Error("billing_webhook_parse_error", "event_type", "invoice.payment_failed", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event data"})
			return
		}
		if err := s.stripeSvc.HandleInvoicePaymentFailed(ctx, &inv); err != nil {
			slog.Error("billing_webhook_handler_error", "handler", "HandleInvoicePaymentFailed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "handler error"})
			return
		}

	default:
		slog.Debug("billing_webhook_unhandled_event", "event_type", event.Type)
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
