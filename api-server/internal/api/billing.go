package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v82"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// ── GET /v1/billing/status ───────────────────────────────────────────────────

func (s *Server) handleBillingStatus(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, caller.TenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}

	resp := gin.H{
		"plan":             tenant.Plan,
		"plan_status":      tenant.PlanStatus,
		"has_subscription": tenant.StripeSubscriptionID != "",
		"billing_email":    tenant.BillingEmail,
		"current_period_end": tenant.CurrentPeriodEnd,
		"stripe_configured":  s.stripeSvc.IsConfigured(),
	}

	// Fetch payment method info from Stripe if configured and subscribed
	if s.stripeSvc.IsConfigured() && tenant.StripeSubscriptionID != "" {
		subInfo, err := s.stripeSvc.GetSubscription(c.Request.Context(), caller.TenantID)
		if err != nil {
			log.Printf("billing: failed to fetch subscription for tenant %d: %v", caller.TenantID, err)
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

	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req checkoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !models.ValidPlan(req.Plan) || req.Plan == models.PlanFree {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan; must be pro, team, or business"})
		return
	}

	// Reject if already subscribed
	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, caller.TenantID).Error; err != nil {
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

	url, err := s.stripeSvc.CreateCheckoutSession(c.Request.Context(), caller.TenantID, req.Plan, req.SuccessURL, req.CancelURL)
	if err != nil {
		log.Printf("billing: checkout error for tenant %d: %v", caller.TenantID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create checkout session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
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

	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req portalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, caller.TenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}
	if tenant.StripeCustomerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no billing account found; subscribe first"})
		return
	}

	url, err := s.stripeSvc.CreatePortalSession(c.Request.Context(), caller.TenantID, req.ReturnURL)
	if err != nil {
		log.Printf("billing: portal error for tenant %d: %v", caller.TenantID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create portal session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// ── GET /v1/billing/invoices ─────────────────────────────────────────────────

func (s *Server) handleBillingInvoices(c *gin.Context) {
	if !s.stripeSvc.IsConfigured() {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	invoices, err := s.stripeSvc.ListInvoices(c.Request.Context(), caller.TenantID, 24)
	if err != nil {
		log.Printf("billing: invoices error for tenant %d: %v", caller.TenantID, err)
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

	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, 65536))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	sigHeader := c.GetHeader("Stripe-Signature")
	event, err := s.stripeSvc.ConstructEvent(payload, sigHeader)
	if err != nil {
		log.Printf("billing: webhook signature verification failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid signature"})
		return
	}

	// Idempotency check
	isNew, err := s.stripeSvc.MarkEventProcessed(c.Request.Context(), event.ID)
	if err != nil {
		log.Printf("billing: idempotency check error for event %s: %v", event.ID, err)
		// Continue processing — better to double-process than to drop
	}
	if !isNew && err == nil {
		log.Printf("billing: skipping already-processed event %s", event.ID)
		c.JSON(http.StatusOK, gin.H{"status": "already_processed"})
		return
	}

	ctx := c.Request.Context()

	switch event.Type {
	case "checkout.session.completed":
		var sess stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
			log.Printf("billing: failed to parse checkout.session.completed: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event data"})
			return
		}
		if err := s.stripeSvc.HandleCheckoutCompleted(ctx, &sess); err != nil {
			log.Printf("billing: HandleCheckoutCompleted error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "handler error"})
			return
		}

	case "customer.subscription.created", "customer.subscription.updated":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			log.Printf("billing: failed to parse subscription event: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event data"})
			return
		}
		if err := s.stripeSvc.HandleSubscriptionUpdated(ctx, &sub); err != nil {
			log.Printf("billing: HandleSubscriptionUpdated error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "handler error"})
			return
		}

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			log.Printf("billing: failed to parse subscription.deleted event: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event data"})
			return
		}
		if err := s.stripeSvc.HandleSubscriptionDeleted(ctx, &sub); err != nil {
			log.Printf("billing: HandleSubscriptionDeleted error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "handler error"})
			return
		}

	case "invoice.paid":
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			log.Printf("billing: failed to parse invoice.paid event: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event data"})
			return
		}
		if err := s.stripeSvc.HandleInvoicePaid(ctx, &inv); err != nil {
			log.Printf("billing: HandleInvoicePaid error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "handler error"})
			return
		}

	case "invoice.payment_failed":
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			log.Printf("billing: failed to parse invoice.payment_failed event: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event data"})
			return
		}
		if err := s.stripeSvc.HandleInvoicePaymentFailed(ctx, &inv); err != nil {
			log.Printf("billing: HandleInvoicePaymentFailed error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "handler error"})
			return
		}

	default:
		log.Printf("billing: unhandled webhook event type: %s", event.Type)
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
