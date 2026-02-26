package models

import "testing"

func TestGetPlanLimits(t *testing.T) {
	tests := []struct {
		plan       string
		wantMaxAPI int
	}{
		{PlanFree, 1},
		{PlanPro, 5},
		{PlanTeam, -1},
		{PlanBusiness, -1},
		{"unknown", 1},  // falls back to free
		{"", 1},          // falls back to free
	}
	for _, tt := range tests {
		t.Run(tt.plan, func(t *testing.T) {
			lim := GetPlanLimits(tt.plan)
			if lim.MaxAPIKeys != tt.wantMaxAPI {
				t.Errorf("GetPlanLimits(%q).MaxAPIKeys = %d, want %d", tt.plan, lim.MaxAPIKeys, tt.wantMaxAPI)
			}
		})
	}
}

func TestGetPlanLimits_DataRetention(t *testing.T) {
	tests := []struct {
		plan string
		want int
	}{
		{PlanFree, 7},
		{PlanPro, 90},
		{PlanTeam, 180},
		{PlanBusiness, -1},
	}
	for _, tt := range tests {
		t.Run(tt.plan, func(t *testing.T) {
			lim := GetPlanLimits(tt.plan)
			if lim.DataRetentionDays != tt.want {
				t.Errorf("GetPlanLimits(%q).DataRetentionDays = %d, want %d", tt.plan, lim.DataRetentionDays, tt.want)
			}
		})
	}
}

func TestValidPlan(t *testing.T) {
	tests := []struct {
		plan string
		want bool
	}{
		{PlanFree, true},
		{PlanPro, true},
		{PlanTeam, true},
		{PlanBusiness, true},
		{"enterprise", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.plan, func(t *testing.T) {
			if got := ValidPlan(tt.plan); got != tt.want {
				t.Errorf("ValidPlan(%q) = %v, want %v", tt.plan, got, tt.want)
			}
		})
	}
}

func TestPlanLevel(t *testing.T) {
	tests := []struct {
		plan string
		want int
	}{
		{PlanFree, 0},
		{PlanPro, 1},
		{PlanTeam, 2},
		{PlanBusiness, 3},
		{"unknown", 0},
	}
	for _, tt := range tests {
		t.Run(tt.plan, func(t *testing.T) {
			if got := PlanLevel(tt.plan); got != tt.want {
				t.Errorf("PlanLevel(%q) = %d, want %d", tt.plan, got, tt.want)
			}
		})
	}
}

func TestIsUpgrade(t *testing.T) {
	tests := []struct {
		name    string
		current string
		new     string
		want    bool
	}{
		{"free to pro", PlanFree, PlanPro, true},
		{"pro to team", PlanPro, PlanTeam, true},
		{"team to business", PlanTeam, PlanBusiness, true},
		{"free to business", PlanFree, PlanBusiness, true},
		{"pro to free is not upgrade", PlanPro, PlanFree, false},
		{"same plan is not upgrade", PlanPro, PlanPro, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUpgrade(tt.current, tt.new); got != tt.want {
				t.Errorf("IsUpgrade(%q,%q) = %v, want %v", tt.current, tt.new, got, tt.want)
			}
		})
	}
}

func TestIsDowngrade(t *testing.T) {
	tests := []struct {
		name    string
		current string
		new     string
		want    bool
	}{
		{"pro to free", PlanPro, PlanFree, true},
		{"team to pro", PlanTeam, PlanPro, true},
		{"business to free", PlanBusiness, PlanFree, true},
		{"free to pro is not downgrade", PlanFree, PlanPro, false},
		{"same plan is not downgrade", PlanPro, PlanPro, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDowngrade(tt.current, tt.new); got != tt.want {
				t.Errorf("IsDowngrade(%q,%q) = %v, want %v", tt.current, tt.new, got, tt.want)
			}
		})
	}
}

func TestAllPlans(t *testing.T) {
	plans := AllPlans()
	if len(plans) != 4 {
		t.Fatalf("AllPlans() returned %d plans, want 4", len(plans))
	}
	// Verify ascending order
	for i := 1; i < len(plans); i++ {
		if PlanLevel(plans[i]) <= PlanLevel(plans[i-1]) {
			t.Errorf("AllPlans() not in ascending order: %q (level %d) after %q (level %d)",
				plans[i], PlanLevel(plans[i]), plans[i-1], PlanLevel(plans[i-1]))
		}
	}
}
