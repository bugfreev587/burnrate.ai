package services

import (
	"testing"

	"github.com/stripe/stripe-go/v82"

	"github.com/xiaoboyu/tokengate/api-server/internal/config"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

func newTestStripeService() *StripeService {
	return &StripeService{
		cfg: config.StripeCfg{
			PriceProMonthly:      "price_pro_123",
			PriceTeamMonthly:     "price_team_456",
			PriceBusinessMonthly: "price_biz_789",
		},
	}
}

func TestPriceIDForPlan(t *testing.T) {
	svc := newTestStripeService()

	tests := []struct {
		name    string
		plan    string
		want    string
		wantErr bool
	}{
		{"pro plan", models.PlanPro, "price_pro_123", false},
		{"team plan", models.PlanTeam, "price_team_456", false},
		{"business plan", models.PlanBusiness, "price_biz_789", false},
		{"free plan — no price", models.PlanFree, "", true},
		{"unknown plan", "enterprise", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.PriceIDForPlan(tt.plan)
			if (err != nil) != tt.wantErr {
				t.Fatalf("PriceIDForPlan(%q) error = %v, wantErr %v", tt.plan, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("PriceIDForPlan(%q) = %q, want %q", tt.plan, got, tt.want)
			}
		})
	}
}

func TestPriceIDForPlan_MissingConfig(t *testing.T) {
	svc := &StripeService{cfg: config.StripeCfg{}} // all empty

	for _, plan := range []string{models.PlanPro, models.PlanTeam, models.PlanBusiness} {
		t.Run(plan, func(t *testing.T) {
			_, err := svc.PriceIDForPlan(plan)
			if err == nil {
				t.Errorf("PriceIDForPlan(%q) expected error for missing config", plan)
			}
		})
	}
}

func TestPlanForPriceID(t *testing.T) {
	svc := newTestStripeService()

	tests := []struct {
		name    string
		priceID string
		want    string
	}{
		{"pro price ID", "price_pro_123", models.PlanPro},
		{"team price ID", "price_team_456", models.PlanTeam},
		{"business price ID", "price_biz_789", models.PlanBusiness},
		{"unknown price ID", "price_unknown", ""},
		{"empty price ID", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.PlanForPriceID(tt.priceID)
			if got != tt.want {
				t.Errorf("PlanForPriceID(%q) = %q, want %q", tt.priceID, got, tt.want)
			}
		})
	}
}

func TestMapStripeStatus(t *testing.T) {
	svc := newTestStripeService()

	tests := []struct {
		name   string
		status stripe.SubscriptionStatus
		want   string
	}{
		{"active", stripe.SubscriptionStatusActive, models.PlanStatusActive},
		{"trialing", stripe.SubscriptionStatusTrialing, models.PlanStatusActive},
		{"incomplete", stripe.SubscriptionStatusIncomplete, models.PlanStatusIncomplete},
		{"incomplete_expired", stripe.SubscriptionStatusIncompleteExpired, models.PlanStatusIncomplete},
		{"past_due", stripe.SubscriptionStatusPastDue, models.PlanStatusPastDue},
		{"canceled", stripe.SubscriptionStatusCanceled, models.PlanStatusCanceled},
		{"unpaid", stripe.SubscriptionStatusUnpaid, models.PlanStatusCanceled},
		{"unknown status defaults to active", stripe.SubscriptionStatus("paused"), models.PlanStatusActive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.mapStripeStatus(tt.status)
			if got != tt.want {
				t.Errorf("mapStripeStatus(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestCurrentPeriodEndFromSub(t *testing.T) {
	tests := []struct {
		name string
		sub  *stripe.Subscription
		want int64
	}{
		{
			"has items with period end",
			&stripe.Subscription{
				Items: &stripe.SubscriptionItemList{
					Data: []*stripe.SubscriptionItem{
						{CurrentPeriodEnd: 1700000000},
					},
				},
			},
			1700000000,
		},
		{
			"nil items",
			&stripe.Subscription{Items: nil},
			0,
		},
		{
			"empty items list",
			&stripe.Subscription{
				Items: &stripe.SubscriptionItemList{Data: []*stripe.SubscriptionItem{}},
			},
			0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := currentPeriodEndFromSub(tt.sub)
			if got != tt.want {
				t.Errorf("currentPeriodEndFromSub() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSubscriptionIDFromInvoice(t *testing.T) {
	tests := []struct {
		name string
		inv  *stripe.Invoice
		want string
	}{
		{
			"has subscription details",
			&stripe.Invoice{
				Parent: &stripe.InvoiceParent{
					SubscriptionDetails: &stripe.InvoiceParentSubscriptionDetails{
						Subscription: &stripe.Subscription{ID: "sub_abc123"},
					},
				},
			},
			"sub_abc123",
		},
		{
			"nil parent",
			&stripe.Invoice{Parent: nil},
			"",
		},
		{
			"nil subscription details",
			&stripe.Invoice{
				Parent: &stripe.InvoiceParent{SubscriptionDetails: nil},
			},
			"",
		},
		{
			"nil subscription",
			&stripe.Invoice{
				Parent: &stripe.InvoiceParent{
					SubscriptionDetails: &stripe.InvoiceParentSubscriptionDetails{
						Subscription: nil,
					},
				},
			},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := subscriptionIDFromInvoice(tt.inv)
			if got != tt.want {
				t.Errorf("subscriptionIDFromInvoice() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsConfigured(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"with secret key", "sk_test_123", true},
		{"empty key", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &StripeService{cfg: config.StripeCfg{SecretKey: tt.key}}
			if got := svc.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}
