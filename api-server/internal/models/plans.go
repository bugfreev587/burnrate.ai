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
}

var planLimitsMap = map[string]PlanLimits{
	PlanFree: {
		MaxAPIKeys:        1,
		MaxMembers:        1,
		AllowedPeriods:    []string{"monthly"},
		AllowBlockAction:  false,
		AllowPerKeyBudget: false,
		DataRetentionDays: 30,
	},
	PlanPro: {
		MaxAPIKeys:        5,
		MaxMembers:        1,
		AllowedPeriods:    []string{"monthly", "weekly", "daily"},
		AllowBlockAction:  true,
		AllowPerKeyBudget: false,
		DataRetentionDays: 90,
	},
	PlanTeam: {
		MaxAPIKeys:        -1,
		MaxMembers:        10,
		AllowedPeriods:    []string{"monthly", "weekly", "daily"},
		AllowBlockAction:  true,
		AllowPerKeyBudget: true,
		DataRetentionDays: 365,
	},
	PlanBusiness: {
		MaxAPIKeys:        -1,
		MaxMembers:        -1,
		AllowedPeriods:    []string{"monthly", "weekly", "daily"},
		AllowBlockAction:  true,
		AllowPerKeyBudget: true,
		DataRetentionDays: -1,
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
