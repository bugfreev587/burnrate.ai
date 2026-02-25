package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/stripe/stripe-go/v82"
	billingportalsession "github.com/stripe/stripe-go/v82/billingportal/session"
	checkoutsession "github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/customer"
	"github.com/stripe/stripe-go/v82/invoice"
	"github.com/stripe/stripe-go/v82/subscription"
	"github.com/stripe/stripe-go/v82/webhook"
	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/config"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// StripeService encapsulates all Stripe operations.
type StripeService struct {
	db  *gorm.DB
	cfg config.StripeCfg
}

// NewStripeService creates a new StripeService and sets the global Stripe key.
func NewStripeService(db *gorm.DB, cfg config.StripeCfg) *StripeService {
	if cfg.SecretKey != "" {
		stripe.Key = cfg.SecretKey
	}
	return &StripeService{db: db, cfg: cfg}
}

// IsConfigured returns true when a Stripe secret key has been provided.
func (s *StripeService) IsConfigured() bool {
	return s.cfg.SecretKey != ""
}

// CancelSubscriptionImmediately cancels a Stripe subscription immediately.
// No-ops when subscriptionID is empty (e.g. free-tier tenants).
func (s *StripeService) CancelSubscriptionImmediately(subscriptionID string) error {
	if subscriptionID == "" {
		return nil
	}
	_, err := subscription.Cancel(subscriptionID, nil)
	if err != nil {
		return fmt.Errorf("cancel subscription %s: %w", subscriptionID, err)
	}
	log.Printf("stripe: subscription %s canceled immediately", subscriptionID)
	return nil
}

// ── Price ↔ Plan mapping ─────────────────────────────────────────────────────

// PriceIDForPlan returns the Stripe Price ID for a plan tier.
func (s *StripeService) PriceIDForPlan(plan string) (string, error) {
	switch plan {
	case models.PlanPro:
		if s.cfg.PriceProMonthly == "" {
			return "", errors.New("STRIPE_PRICE_PRO_MONTHLY not configured")
		}
		return s.cfg.PriceProMonthly, nil
	case models.PlanTeam:
		if s.cfg.PriceTeamMonthly == "" {
			return "", errors.New("STRIPE_PRICE_TEAM_MONTHLY not configured")
		}
		return s.cfg.PriceTeamMonthly, nil
	case models.PlanBusiness:
		if s.cfg.PriceBusinessMonthly == "" {
			return "", errors.New("STRIPE_PRICE_BUSINESS_MONTHLY not configured")
		}
		return s.cfg.PriceBusinessMonthly, nil
	default:
		return "", fmt.Errorf("no Stripe price for plan %q", plan)
	}
}

// PlanForPriceID returns the plan tier for a Stripe Price ID.
func (s *StripeService) PlanForPriceID(priceID string) string {
	switch priceID {
	case s.cfg.PriceProMonthly:
		return models.PlanPro
	case s.cfg.PriceTeamMonthly:
		return models.PlanTeam
	case s.cfg.PriceBusinessMonthly:
		return models.PlanBusiness
	default:
		return ""
	}
}

// ── Customer management ──────────────────────────────────────────────────────

// EnsureCustomer lazily creates a Stripe Customer for the given tenant.
// If one already exists, it returns the existing ID.
func (s *StripeService) EnsureCustomer(ctx context.Context, tenantID uint) (string, error) {
	var tenant models.Tenant
	if err := s.db.First(&tenant, tenantID).Error; err != nil {
		return "", fmt.Errorf("fetch tenant: %w", err)
	}

	if tenant.StripeCustomerID != "" {
		return tenant.StripeCustomerID, nil
	}

	// Determine email: prefer billing_email, fall back to owner's email
	email := tenant.BillingEmail
	if email == "" {
		var owner models.User
		if err := s.db.Where("tenant_id = ? AND role = ?", tenantID, models.RoleOwner).First(&owner).Error; err == nil {
			email = owner.Email
		}
	}

	params := &stripe.CustomerParams{
		Name:  stripe.String(tenant.Name),
		Email: stripe.String(email),
	}
	params.AddMetadata("tenant_id", fmt.Sprintf("%d", tenant.ID))
	params.Context = ctx

	cust, err := customer.New(params)
	if err != nil {
		return "", fmt.Errorf("create stripe customer: %w", err)
	}

	if err := s.db.Model(&tenant).Update("stripe_customer_id", cust.ID).Error; err != nil {
		return "", fmt.Errorf("save stripe_customer_id: %w", err)
	}

	log.Printf("stripe: created customer %s for tenant %d", cust.ID, tenantID)
	return cust.ID, nil
}

// ── Checkout & Portal ────────────────────────────────────────────────────────

// CreateCheckoutSession creates a Stripe Checkout session for subscribing to a paid plan.
func (s *StripeService) CreateCheckoutSession(ctx context.Context, tenantID uint, plan, successURL, cancelURL string) (string, error) {
	customerID, err := s.EnsureCustomer(ctx, tenantID)
	if err != nil {
		return "", err
	}

	priceID, err := s.PriceIDForPlan(plan)
	if err != nil {
		return "", err
	}

	params := &stripe.CheckoutSessionParams{
		Customer: stripe.String(customerID),
		Mode:     stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: map[string]string{
				"tenant_id": fmt.Sprintf("%d", tenantID),
				"plan":      plan,
			},
		},
	}
	params.AddMetadata("tenant_id", fmt.Sprintf("%d", tenantID))
	params.AddMetadata("plan", plan)
	params.Context = ctx

	sess, err := checkoutsession.New(params)
	if err != nil {
		return "", fmt.Errorf("create checkout session: %w", err)
	}

	return sess.URL, nil
}

// VerifyCheckoutSession fetches a completed Checkout session from Stripe and
// applies the plan change to the tenant. This is the synchronous fallback that
// runs when the user is redirected back after payment, ensuring the plan is
// updated even if the webhook hasn't arrived yet.
func (s *StripeService) VerifyCheckoutSession(ctx context.Context, tenantID uint, sessionID string) error {
	params := &stripe.CheckoutSessionParams{}
	params.AddExpand("subscription")
	params.Context = ctx

	sess, err := checkoutsession.Get(sessionID, params)
	if err != nil {
		return fmt.Errorf("fetch checkout session: %w", err)
	}

	if sess.Status != stripe.CheckoutSessionStatusComplete {
		return fmt.Errorf("checkout session not complete (status=%s)", sess.Status)
	}

	// Verify this session belongs to the requesting tenant
	if sess.Metadata["tenant_id"] != fmt.Sprintf("%d", tenantID) {
		return fmt.Errorf("session tenant_id mismatch")
	}

	// Apply via the same handler used by the webhook
	return s.HandleCheckoutCompleted(ctx, sess)
}

// ScheduleDowngrade schedules a downgrade to take effect at the end of the current
// billing period. The user keeps their current plan features until then.
//
// For paid → free: sets cancel_at_period_end=true on the Stripe subscription.
// For paid → lower paid: records pending_plan locally; the actual Stripe price swap
// happens when the period ends (via webhook or a scheduled job).
func (s *StripeService) ScheduleDowngrade(ctx context.Context, tenantID uint, targetPlan string) error {
	var tenant models.Tenant
	if err := s.db.First(&tenant, tenantID).Error; err != nil {
		return fmt.Errorf("fetch tenant: %w", err)
	}

	if tenant.StripeSubscriptionID == "" {
		return errors.New("tenant has no active subscription")
	}

	if !models.IsDowngrade(tenant.Plan, targetPlan) {
		return fmt.Errorf("plan %q → %q is not a downgrade", tenant.Plan, targetPlan)
	}

	// Determine the current period end from Stripe
	getParams := &stripe.SubscriptionParams{}
	getParams.Context = ctx
	sub, err := subscription.Get(tenant.StripeSubscriptionID, getParams)
	if err != nil {
		return fmt.Errorf("get subscription: %w", err)
	}

	periodEnd := currentPeriodEndFromSub(sub)
	var effectiveAt *time.Time
	if periodEnd > 0 {
		t := time.Unix(periodEnd, 0)
		effectiveAt = &t
	}

	if targetPlan == models.PlanFree {
		// Paid → Free: tell Stripe to cancel at period end
		updateParams := &stripe.SubscriptionParams{
			CancelAtPeriodEnd: stripe.Bool(true),
		}
		updateParams.Context = ctx
		if _, err := subscription.Update(tenant.StripeSubscriptionID, updateParams); err != nil {
			return fmt.Errorf("set cancel_at_period_end: %w", err)
		}
	} else {
		// Paid → lower Paid: we schedule a price swap at period end via Stripe subscription schedule,
		// but for simplicity we use cancel_at_period_end=false and record the pending plan locally.
		// The webhook for subscription.updated at period renewal will trigger the price change.
		// For now, just record the intent in our DB.
	}

	// Record the pending downgrade in our DB
	updates := map[string]any{
		"pending_plan":      targetPlan,
		"plan_effective_at": effectiveAt,
	}
	if err := s.db.WithContext(ctx).Model(&tenant).Updates(updates).Error; err != nil {
		return fmt.Errorf("save pending downgrade: %w", err)
	}

	log.Printf("stripe: scheduled downgrade for tenant %d: %s → %s (effective %v)", tenantID, tenant.Plan, targetPlan, effectiveAt)
	return nil
}

// CancelScheduledDowngrade cancels a pending downgrade, restoring the current plan.
func (s *StripeService) CancelScheduledDowngrade(ctx context.Context, tenantID uint) error {
	var tenant models.Tenant
	if err := s.db.First(&tenant, tenantID).Error; err != nil {
		return fmt.Errorf("fetch tenant: %w", err)
	}

	if tenant.PendingPlan == "" {
		return errors.New("no pending downgrade to cancel")
	}

	if tenant.StripeSubscriptionID == "" {
		return errors.New("tenant has no active subscription")
	}

	// If downgrading to free, we set cancel_at_period_end=true, so undo that
	if tenant.PendingPlan == models.PlanFree {
		updateParams := &stripe.SubscriptionParams{
			CancelAtPeriodEnd: stripe.Bool(false),
		}
		updateParams.Context = ctx
		if _, err := subscription.Update(tenant.StripeSubscriptionID, updateParams); err != nil {
			return fmt.Errorf("unset cancel_at_period_end: %w", err)
		}
	}

	// Clear the pending downgrade in our DB
	if err := s.db.WithContext(ctx).Model(&tenant).Updates(map[string]any{
		"pending_plan":      "",
		"plan_effective_at": nil,
	}).Error; err != nil {
		return fmt.Errorf("clear pending downgrade: %w", err)
	}

	log.Printf("stripe: canceled scheduled downgrade for tenant %d (was pending %s)", tenantID, tenant.PendingPlan)
	return nil
}

// ChangeSubscriptionPlan changes the price on an existing Stripe subscription.
// This handles immediate upgrades (with proration) and also clears any pending downgrade.
func (s *StripeService) ChangeSubscriptionPlan(ctx context.Context, tenantID uint, newPlan string) error {
	var tenant models.Tenant
	if err := s.db.First(&tenant, tenantID).Error; err != nil {
		return fmt.Errorf("fetch tenant: %w", err)
	}

	if tenant.StripeSubscriptionID == "" {
		return errors.New("tenant has no active subscription")
	}

	newPriceID, err := s.PriceIDForPlan(newPlan)
	if err != nil {
		return err
	}

	// If there was a pending downgrade to free (cancel_at_period_end), undo it
	if tenant.PendingPlan == models.PlanFree {
		undoParams := &stripe.SubscriptionParams{
			CancelAtPeriodEnd: stripe.Bool(false),
		}
		undoParams.Context = ctx
		if _, err := subscription.Update(tenant.StripeSubscriptionID, undoParams); err != nil {
			log.Printf("stripe: warning: failed to unset cancel_at_period_end during upgrade for tenant %d: %v", tenantID, err)
		}
	}

	// Fetch the current subscription to get the item ID
	getParams := &stripe.SubscriptionParams{}
	getParams.Context = ctx
	sub, err := subscription.Get(tenant.StripeSubscriptionID, getParams)
	if err != nil {
		return fmt.Errorf("get subscription: %w", err)
	}

	if sub.Items == nil || len(sub.Items.Data) == 0 {
		return errors.New("subscription has no items")
	}

	itemID := sub.Items.Data[0].ID

	// Update the subscription with the new price (immediate, with proration for upgrades)
	updateParams := &stripe.SubscriptionParams{
		Items: []*stripe.SubscriptionItemsParams{
			{
				ID:    stripe.String(itemID),
				Price: stripe.String(newPriceID),
			},
		},
		ProrationBehavior: stripe.String("create_prorations"),
	}
	updateParams.AddMetadata("plan", newPlan)
	updateParams.Context = ctx

	updatedSub, err := subscription.Update(tenant.StripeSubscriptionID, updateParams)
	if err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}

	// Update our DB — apply plan immediately and clear any pending downgrade
	newLimits := models.GetPlanLimits(newPlan)
	dbUpdates := map[string]any{
		"plan":              newPlan,
		"plan_status":       s.mapStripeStatus(updatedSub.Status),
		"max_api_keys":      newLimits.MaxAPIKeys,
		"pending_plan":      "",
		"plan_effective_at": nil,
	}
	if periodEnd := currentPeriodEndFromSub(updatedSub); periodEnd > 0 {
		t := time.Unix(periodEnd, 0)
		dbUpdates["current_period_end"] = &t
	}
	if err := s.db.WithContext(ctx).Model(&tenant).Updates(dbUpdates).Error; err != nil {
		return fmt.Errorf("update tenant plan: %w", err)
	}

	log.Printf("stripe: subscription %s upgraded to plan=%s for tenant %d", tenant.StripeSubscriptionID, newPlan, tenantID)
	return nil
}

// CreatePortalSession creates a Stripe Customer Portal session for managing the subscription.
func (s *StripeService) CreatePortalSession(ctx context.Context, tenantID uint, returnURL string) (string, error) {
	var tenant models.Tenant
	if err := s.db.First(&tenant, tenantID).Error; err != nil {
		return "", fmt.Errorf("fetch tenant: %w", err)
	}

	if tenant.StripeCustomerID == "" {
		return "", errors.New("tenant has no Stripe customer")
	}

	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(tenant.StripeCustomerID),
		ReturnURL: stripe.String(returnURL),
	}
	if s.cfg.PortalConfigurationID != "" {
		params.Configuration = stripe.String(s.cfg.PortalConfigurationID)
	}
	params.Context = ctx

	sess, err := billingportalsession.New(params)
	if err != nil {
		return "", fmt.Errorf("create portal session: %w", err)
	}

	return sess.URL, nil
}

// ── Subscription & Invoice queries ───────────────────────────────────────────

// SubscriptionInfo holds display-friendly subscription details.
type SubscriptionInfo struct {
	ID                    string `json:"id"`
	Status                string `json:"status"`
	CurrentPeriodEnd      int64  `json:"current_period_end"`
	CancelAtPeriodEnd     bool   `json:"cancel_at_period_end"`
	PaymentMethodBrand    string `json:"payment_method_brand,omitempty"`
	PaymentMethodLast4    string `json:"payment_method_last4,omitempty"`
	PaymentMethodExpMonth int64  `json:"payment_method_exp_month,omitempty"`
	PaymentMethodExpYear  int64  `json:"payment_method_exp_year,omitempty"`
}

// GetSubscription fetches the current subscription details from Stripe.
func (s *StripeService) GetSubscription(ctx context.Context, tenantID uint) (*SubscriptionInfo, error) {
	var tenant models.Tenant
	if err := s.db.First(&tenant, tenantID).Error; err != nil {
		return nil, fmt.Errorf("fetch tenant: %w", err)
	}

	if tenant.StripeSubscriptionID == "" {
		return nil, nil
	}

	params := &stripe.SubscriptionParams{}
	params.AddExpand("default_payment_method")
	params.Context = ctx

	sub, err := subscription.Get(tenant.StripeSubscriptionID, params)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}

	info := &SubscriptionInfo{
		ID:                sub.ID,
		Status:            string(sub.Status),
		CurrentPeriodEnd:  currentPeriodEndFromSub(sub),
		CancelAtPeriodEnd: sub.CancelAtPeriodEnd,
	}

	if sub.DefaultPaymentMethod != nil && sub.DefaultPaymentMethod.Card != nil {
		card := sub.DefaultPaymentMethod.Card
		info.PaymentMethodBrand = string(card.Brand)
		info.PaymentMethodLast4 = card.Last4
		info.PaymentMethodExpMonth = card.ExpMonth
		info.PaymentMethodExpYear = card.ExpYear
	}

	return info, nil
}

// InvoiceInfo holds display-friendly invoice data.
type InvoiceInfo struct {
	ID          string `json:"id"`
	Number      string `json:"number"`
	Status      string `json:"status"`
	AmountDue   int64  `json:"amount_due"`
	AmountPaid  int64  `json:"amount_paid"`
	Currency    string `json:"currency"`
	PeriodStart int64  `json:"period_start"`
	PeriodEnd   int64  `json:"period_end"`
	Created     int64  `json:"created"`
	PDFURL      string `json:"pdf_url"`
	HostedURL   string `json:"hosted_url"`
}

// ListInvoices returns recent invoices for the tenant's Stripe customer.
func (s *StripeService) ListInvoices(ctx context.Context, tenantID uint, limit int64) ([]InvoiceInfo, error) {
	var tenant models.Tenant
	if err := s.db.First(&tenant, tenantID).Error; err != nil {
		return nil, fmt.Errorf("fetch tenant: %w", err)
	}

	if tenant.StripeCustomerID == "" {
		return []InvoiceInfo{}, nil
	}

	params := &stripe.InvoiceListParams{
		Customer: stripe.String(tenant.StripeCustomerID),
	}
	params.Filters.AddFilter("limit", "", fmt.Sprintf("%d", limit))
	params.Context = ctx

	var invoices []InvoiceInfo
	iter := invoice.List(params)
	for iter.Next() {
		inv := iter.Invoice()
		invoices = append(invoices, InvoiceInfo{
			ID:          inv.ID,
			Number:      inv.Number,
			Status:      string(inv.Status),
			AmountDue:   inv.AmountDue,
			AmountPaid:  inv.AmountPaid,
			Currency:    string(inv.Currency),
			PeriodStart: inv.PeriodStart,
			PeriodEnd:   inv.PeriodEnd,
			Created:     inv.Created,
			PDFURL:      inv.InvoicePDF,
			HostedURL:   inv.HostedInvoiceURL,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}

	return invoices, nil
}

// ── Webhook helpers ──────────────────────────────────────────────────────────

// ConstructEvent verifies the webhook signature and parses the event.
func (s *StripeService) ConstructEvent(payload []byte, sigHeader string) (stripe.Event, error) {
	return webhook.ConstructEvent(payload, sigHeader, s.cfg.WebhookSecret)
}

// MarkEventProcessed attempts to insert the event ID for idempotency.
// Returns true if this is a new event, false if already processed.
func (s *StripeService) MarkEventProcessed(ctx context.Context, eventID string) (bool, error) {
	evt := models.ProcessedStripeEvent{EventID: eventID}
	result := s.db.WithContext(ctx).Create(&evt)
	if result.Error != nil {
		// unique index violation → already processed
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return false, nil
		}
		// Some drivers surface unique violation differently
		if result.RowsAffected == 0 {
			return false, nil
		}
		return false, result.Error
	}
	return true, nil
}

// ── Webhook event handlers ───────────────────────────────────────────────────

// HandleCheckoutCompleted processes a checkout.session.completed event.
func (s *StripeService) HandleCheckoutCompleted(ctx context.Context, sess *stripe.CheckoutSession) error {
	tenantIDStr := sess.Metadata["tenant_id"]
	plan := sess.Metadata["plan"]
	if tenantIDStr == "" || plan == "" {
		log.Printf("stripe webhook: checkout.session.completed missing metadata (tenant_id=%q, plan=%q)", tenantIDStr, plan)
		return nil
	}

	var tenantID uint
	if _, err := fmt.Sscanf(tenantIDStr, "%d", &tenantID); err != nil {
		return fmt.Errorf("parse tenant_id: %w", err)
	}

	updates := map[string]any{
		"plan":                   plan,
		"plan_status":            models.PlanStatusActive,
		"stripe_subscription_id": "",
		"max_api_keys":           models.GetPlanLimits(plan).MaxAPIKeys,
		"pending_plan":           "",
		"plan_effective_at":      nil,
	}

	if sess.Subscription != nil {
		updates["stripe_subscription_id"] = sess.Subscription.ID
	}

	if sess.CustomerDetails != nil && sess.CustomerDetails.Email != "" {
		updates["billing_email"] = sess.CustomerDetails.Email
	}

	if err := s.db.WithContext(ctx).Model(&models.Tenant{}).Where("id = ?", tenantID).Updates(updates).Error; err != nil {
		return fmt.Errorf("update tenant after checkout: %w", err)
	}

	log.Printf("stripe: checkout completed for tenant %d → plan=%s", tenantID, plan)
	return nil
}

// HandleSubscriptionUpdated processes customer.subscription.updated and customer.subscription.created events.
func (s *StripeService) HandleSubscriptionUpdated(ctx context.Context, sub *stripe.Subscription) error {
	tenantID, err := s.tenantIDFromSubscription(sub)
	if err != nil {
		return err
	}
	if tenantID == 0 {
		return nil
	}

	// Fetch the current tenant to check for pending downgrade
	var tenant models.Tenant
	if err := s.db.First(&tenant, tenantID).Error; err != nil {
		return fmt.Errorf("fetch tenant: %w", err)
	}

	plan := s.planFromSubscription(sub)

	updates := map[string]any{
		"plan_status":            s.mapStripeStatus(sub.Status),
		"stripe_subscription_id": sub.ID,
	}

	// Only update the plan if there's no pending downgrade.
	// When a downgrade is scheduled, we don't want the webhook to overwrite
	// the current plan — the user keeps their current plan until period end.
	if tenant.PendingPlan == "" && plan != "" {
		updates["plan"] = plan
		updates["max_api_keys"] = models.GetPlanLimits(plan).MaxAPIKeys
	}

	if periodEnd := currentPeriodEndFromSub(sub); periodEnd > 0 {
		t := time.Unix(periodEnd, 0)
		updates["current_period_end"] = &t
	}

	if err := s.db.WithContext(ctx).Model(&models.Tenant{}).Where("id = ?", tenantID).Updates(updates).Error; err != nil {
		return fmt.Errorf("update tenant from subscription: %w", err)
	}

	log.Printf("stripe: subscription %s updated for tenant %d (status=%s, plan=%s, pending=%s)", sub.ID, tenantID, sub.Status, plan, tenant.PendingPlan)
	return nil
}

// HandleSubscriptionDeleted processes customer.subscription.deleted events.
// If there's a pending downgrade to free, this finalizes it. Otherwise, it's a forced cancellation.
func (s *StripeService) HandleSubscriptionDeleted(ctx context.Context, sub *stripe.Subscription) error {
	tenantID, err := s.tenantIDFromSubscription(sub)
	if err != nil {
		return err
	}
	if tenantID == 0 {
		return nil
	}

	// Downgrade to free and clear any pending plan state
	targetPlan := models.PlanFree
	targetLimits := models.GetPlanLimits(targetPlan)

	updates := map[string]any{
		"plan":                   targetPlan,
		"plan_status":            models.PlanStatusCanceled,
		"stripe_subscription_id": "",
		"current_period_end":     nil,
		"max_api_keys":           targetLimits.MaxAPIKeys,
		"pending_plan":           "",
		"plan_effective_at":      nil,
	}

	if err := s.db.WithContext(ctx).Model(&models.Tenant{}).Where("id = ?", tenantID).Updates(updates).Error; err != nil {
		return fmt.Errorf("downgrade tenant after subscription deleted: %w", err)
	}

	log.Printf("stripe: subscription %s deleted for tenant %d → downgraded to free", sub.ID, tenantID)
	return nil
}

// HandleInvoicePaid processes invoice.paid events — recovers plan_status to active.
func (s *StripeService) HandleInvoicePaid(ctx context.Context, inv *stripe.Invoice) error {
	subID := subscriptionIDFromInvoice(inv)
	if subID == "" {
		return nil
	}

	var tenant models.Tenant
	if err := s.db.WithContext(ctx).Where("stripe_subscription_id = ?", subID).First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("stripe: invoice.paid for unknown subscription %s", subID)
			return nil
		}
		return err
	}

	if err := s.db.WithContext(ctx).Model(&tenant).Update("plan_status", models.PlanStatusActive).Error; err != nil {
		return fmt.Errorf("update plan_status to active: %w", err)
	}

	log.Printf("stripe: invoice paid for tenant %d → plan_status=active", tenant.ID)
	return nil
}

// HandleInvoicePaymentFailed processes invoice.payment_failed events.
func (s *StripeService) HandleInvoicePaymentFailed(ctx context.Context, inv *stripe.Invoice) error {
	subID := subscriptionIDFromInvoice(inv)
	if subID == "" {
		return nil
	}

	var tenant models.Tenant
	if err := s.db.WithContext(ctx).Where("stripe_subscription_id = ?", subID).First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("stripe: invoice.payment_failed for unknown subscription %s", subID)
			return nil
		}
		return err
	}

	if err := s.db.WithContext(ctx).Model(&tenant).Update("plan_status", models.PlanStatusPastDue).Error; err != nil {
		return fmt.Errorf("update plan_status to past_due: %w", err)
	}

	log.Printf("stripe: invoice payment failed for tenant %d → plan_status=past_due", tenant.ID)
	return nil
}

// ── Internal helpers ─────────────────────────────────────────────────────────

// tenantIDFromSubscription extracts the tenant ID from subscription metadata or DB lookup.
func (s *StripeService) tenantIDFromSubscription(sub *stripe.Subscription) (uint, error) {
	if idStr, ok := sub.Metadata["tenant_id"]; ok && idStr != "" {
		var id uint
		if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil {
			return id, nil
		}
	}

	// Fall back to DB lookup by stripe_subscription_id
	var tenant models.Tenant
	if err := s.db.Where("stripe_subscription_id = ?", sub.ID).First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if sub.Customer != nil {
				if err := s.db.Where("stripe_customer_id = ?", sub.Customer.ID).First(&tenant).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						log.Printf("stripe: no tenant found for subscription %s / customer %s", sub.ID, sub.Customer.ID)
						return 0, nil
					}
					return 0, err
				}
				return tenant.ID, nil
			}
			log.Printf("stripe: no tenant found for subscription %s", sub.ID)
			return 0, nil
		}
		return 0, err
	}
	return tenant.ID, nil
}

// planFromSubscription determines the plan from the subscription's line items.
func (s *StripeService) planFromSubscription(sub *stripe.Subscription) string {
	if sub.Items != nil {
		for _, item := range sub.Items.Data {
			if item.Price != nil {
				if plan := s.PlanForPriceID(item.Price.ID); plan != "" {
					return plan
				}
			}
		}
	}
	if plan, ok := sub.Metadata["plan"]; ok {
		return plan
	}
	return ""
}

// currentPeriodEndFromSub extracts the current_period_end from the first subscription item.
// In stripe-go v82, CurrentPeriodEnd was moved from Subscription to SubscriptionItem.
func currentPeriodEndFromSub(sub *stripe.Subscription) int64 {
	if sub.Items != nil && len(sub.Items.Data) > 0 {
		return sub.Items.Data[0].CurrentPeriodEnd
	}
	return 0
}

// subscriptionIDFromInvoice extracts the subscription ID from an invoice.
// In stripe-go v82, Invoice.Subscription was replaced by Invoice.Parent.SubscriptionDetails.
func subscriptionIDFromInvoice(inv *stripe.Invoice) string {
	if inv.Parent != nil && inv.Parent.SubscriptionDetails != nil && inv.Parent.SubscriptionDetails.Subscription != nil {
		return inv.Parent.SubscriptionDetails.Subscription.ID
	}
	return ""
}

// mapStripeStatus maps Stripe subscription status to PlanStatus.
func (s *StripeService) mapStripeStatus(status stripe.SubscriptionStatus) string {
	switch status {
	case stripe.SubscriptionStatusActive, stripe.SubscriptionStatusTrialing:
		return models.PlanStatusActive
	case stripe.SubscriptionStatusIncomplete, stripe.SubscriptionStatusIncompleteExpired:
		return models.PlanStatusIncomplete
	case stripe.SubscriptionStatusPastDue:
		return models.PlanStatusPastDue
	case stripe.SubscriptionStatusCanceled, stripe.SubscriptionStatusUnpaid:
		return models.PlanStatusCanceled
	default:
		return models.PlanStatusActive
	}
}
