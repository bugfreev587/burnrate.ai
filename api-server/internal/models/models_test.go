package models

import "testing"

func TestRoleLevel(t *testing.T) {
	tests := []struct {
		role string
		want int
	}{
		{RoleOwner, 4},
		{RoleAdmin, 3},
		{RoleEditor, 2},
		{RoleViewer, 1},
		{"unknown", 0},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			if got := RoleLevel(tt.role); got != tt.want {
				t.Errorf("RoleLevel(%q) = %d, want %d", tt.role, got, tt.want)
			}
		})
	}
}

func TestHasPermission(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		required string
		want     bool
	}{
		{"owner can do anything", RoleOwner, RoleViewer, true},
		{"owner has owner perm", RoleOwner, RoleOwner, true},
		{"admin has admin perm", RoleAdmin, RoleAdmin, true},
		{"admin has editor perm", RoleAdmin, RoleEditor, true},
		{"admin lacks owner perm", RoleAdmin, RoleOwner, false},
		{"editor has viewer perm", RoleEditor, RoleViewer, true},
		{"editor lacks admin perm", RoleEditor, RoleAdmin, false},
		{"viewer has viewer perm", RoleViewer, RoleViewer, true},
		{"viewer lacks editor perm", RoleViewer, RoleEditor, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &User{Role: tt.role}
			if got := u.HasPermission(tt.required); got != tt.want {
				t.Errorf("User{Role:%q}.HasPermission(%q) = %v, want %v", tt.role, tt.required, got, tt.want)
			}
		})
	}
}

func TestIsActive(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{StatusActive, true},
		{StatusSuspended, false},
		{StatusPending, false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			u := &User{Status: tt.status}
			if got := u.IsActive(); got != tt.want {
				t.Errorf("User{Status:%q}.IsActive() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestValidAuthBillingCombo(t *testing.T) {
	tests := []struct {
		name        string
		provider    string
		authMethod  string
		billingMode string
		want        bool
	}{
		{"anthropic browser+monthly", "anthropic", AuthMethodBrowserOAuth, BillingModeMonthlySubscription, true},
		{"anthropic browser+api", "anthropic", AuthMethodBrowserOAuth, BillingModeAPIUsage, true},
		{"anthropic byok+api", "anthropic", AuthMethodBYOK, BillingModeAPIUsage, true},
		{"anthropic byok+monthly invalid", "anthropic", AuthMethodBYOK, BillingModeMonthlySubscription, false},
		{"openai browser+monthly", "openai", AuthMethodBrowserOAuth, BillingModeMonthlySubscription, true},
		{"openai byok+monthly invalid", "openai", AuthMethodBYOK, BillingModeMonthlySubscription, false},
		{"unknown provider", "google", AuthMethodBrowserOAuth, BillingModeMonthlySubscription, false},
		{"unknown auth method", "anthropic", "unknown", BillingModeAPIUsage, false},
		{"unknown billing mode", "anthropic", AuthMethodBrowserOAuth, "unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidAuthBillingCombo(tt.provider, tt.authMethod, tt.billingMode); got != tt.want {
				t.Errorf("ValidAuthBillingCombo(%q,%q,%q) = %v, want %v", tt.provider, tt.authMethod, tt.billingMode, got, tt.want)
			}
		})
	}
}

func TestIsBillableMode(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{BillingModeAPIUsage, true},
		{BillingModeMonthlySubscription, false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			if got := IsBillableMode(tt.mode); got != tt.want {
				t.Errorf("IsBillableMode(%q) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}
