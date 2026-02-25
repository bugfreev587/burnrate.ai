package models

// Plan tier constants.
const (
	PlanFree     = "free"
	PlanPro      = "pro"
	PlanTeam     = "team"
	PlanBusiness = "business"
)

// PlanLimits defines what a given plan tier allows.
type PlanLimits struct {
	// MaxAPIKeys is the maximum number of active (non-revoked, non-expired) API keys.
	// -1 means unlimited.
	MaxAPIKeys int `json:"max_api_keys"`
	// MaxProviderKeys is the maximum number of active (non-revoked) provider keys.
	// -1 means unlimited.
	MaxProviderKeys int `json:"max_provider_keys"`
	// MaxMembers is the maximum number of users (owner + all others, including pending invites).
	// -1 means unlimited.
	MaxMembers int `json:"max_members"`
	// AllowedPeriods lists the budget period types the plan permits.
	AllowedPeriods []string `json:"allowed_periods"`
	// AllowBlockAction permits setting budget action="block" (hard limit → HTTP 402).
	AllowBlockAction bool `json:"allow_block_action"`
	// AllowPerKeyBudget permits budget limits scoped to individual API keys.
	AllowPerKeyBudget bool `json:"allow_per_key_budget"`
	// DataRetentionDays is how many days of usage / cost-ledger history are visible.
	// -1 means unlimited (all history retained).
	DataRetentionDays int `json:"data_retention_days"`
	// AllowRateLimits permits configuring model-scoped rate limits (RPM/ITPM/OTPM).
	AllowRateLimits bool `json:"allow_rate_limits"`
	// AllowPerKeyRateLimit permits rate limits scoped to individual API keys.
	AllowPerKeyRateLimit bool `json:"allow_per_key_rate_limit"`
}

var planLimitsMap = map[string]PlanLimits{
	PlanFree: {
		MaxAPIKeys:           1,
		MaxProviderKeys:      1,
		MaxMembers:           1,
		AllowedPeriods:       []string{"monthly"},
		AllowBlockAction:     false,
		AllowPerKeyBudget:    false,
		DataRetentionDays:    7,
		AllowRateLimits:      false,
		AllowPerKeyRateLimit: false,
	},
	PlanPro: {
		MaxAPIKeys:           5,
		MaxProviderKeys:      3,
		MaxMembers:           1,
		AllowedPeriods:       []string{"monthly", "weekly", "daily"},
		AllowBlockAction:     true,
		AllowPerKeyBudget:    false,
		DataRetentionDays:    90,
		AllowRateLimits:      true,
		AllowPerKeyRateLimit: false,
	},
	PlanTeam: {
		MaxAPIKeys:           -1,
		MaxProviderKeys:      5,
		MaxMembers:           10,
		AllowedPeriods:       []string{"monthly", "weekly", "daily"},
		AllowBlockAction:     true,
		AllowPerKeyBudget:    true,
		DataRetentionDays:    180,
		AllowRateLimits:      true,
		AllowPerKeyRateLimit: true,
	},
	PlanBusiness: {
		MaxAPIKeys:           -1,
		MaxProviderKeys:      20,
		MaxMembers:           -1,
		AllowedPeriods:       []string{"monthly", "weekly", "daily"},
		AllowBlockAction:     true,
		AllowPerKeyBudget:    true,
		DataRetentionDays:    -1,
		AllowRateLimits:      true,
		AllowPerKeyRateLimit: true,
	},
}

// GetPlanLimits returns the PlanLimits for the given plan name.
// Unknown or empty plans fall back to Free limits.
func GetPlanLimits(plan string) PlanLimits {
	if lim, ok := planLimitsMap[plan]; ok {
		return lim
	}
	return planLimitsMap[PlanFree]
}

// ValidPlan reports whether plan is one of the recognised tier names.
func ValidPlan(plan string) bool {
	_, ok := planLimitsMap[plan]
	return ok
}

// AllPlans returns all plan keys in ascending tier order.
func AllPlans() []string {
	return []string{PlanFree, PlanPro, PlanTeam, PlanBusiness}
}

// PlanLevel returns the numeric tier level for a plan (higher = more features).
// Used to determine whether a plan change is an upgrade or downgrade.
func PlanLevel(plan string) int {
	switch plan {
	case PlanFree:
		return 0
	case PlanPro:
		return 1
	case PlanTeam:
		return 2
	case PlanBusiness:
		return 3
	default:
		return 0
	}
}

// IsUpgrade returns true when changing from currentPlan to newPlan is an upgrade.
func IsUpgrade(currentPlan, newPlan string) bool {
	return PlanLevel(newPlan) > PlanLevel(currentPlan)
}

// IsDowngrade returns true when changing from currentPlan to newPlan is a downgrade.
func IsDowngrade(currentPlan, newPlan string) bool {
	return PlanLevel(newPlan) < PlanLevel(currentPlan)
}
