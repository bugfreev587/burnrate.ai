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

	plan := s.planFromSubscription(sub)

	updates := map[string]any{
		"plan_status":            s.mapStripeStatus(sub.Status),
		"stripe_subscription_id": sub.ID,
	}

	if plan != "" {
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

	log.Printf("stripe: subscription %s updated for tenant %d (status=%s, plan=%s)", sub.ID, tenantID, sub.Status, plan)
	return nil
}

// HandleSubscriptionDeleted processes customer.subscription.deleted events (downgrade to free).
func (s *StripeService) HandleSubscriptionDeleted(ctx context.Context, sub *stripe.Subscription) error {
	tenantID, err := s.tenantIDFromSubscription(sub)
	if err != nil {
		return err
	}
	if tenantID == 0 {
		return nil
	}

	updates := map[string]any{
		"plan":                   models.PlanFree,
		"plan_status":            models.PlanStatusCanceled,
		"stripe_subscription_id": "",
		"current_period_end":     nil,
		"max_api_keys":           models.GetPlanLimits(models.PlanFree).MaxAPIKeys,
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
