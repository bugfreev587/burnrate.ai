package authz

// Action constants organized by resource.
// Each action is either tenant-scoped or project-scoped.

// Tenant-scoped actions
const (
	// Organization
	ActionOrgRead   = "org:read"
	ActionOrgUpdate = "org:update"
	ActionOrgDelete = "org:delete"

	// Member management
	ActionMemberList       = "member:list"
	ActionMemberInvite     = "member:invite"
	ActionMemberUpdateRole = "member:update_role"
	ActionMemberRemove     = "member:remove"

	// Project management (creating/listing projects is tenant-scoped)
	ActionProjectList   = "project:list"
	ActionProjectCreate = "project:create"

	// Provider key management
	ActionProviderKeyList   = "provider_key:list"
	ActionProviderKeyRead   = "provider_key:read"
	ActionProviderKeyCreate = "provider_key:create"
	ActionProviderKeyUpdate = "provider_key:update"
	ActionProviderKeyDelete = "provider_key:delete"

	// Usage & reporting
	ActionUsageRead      = "usage:read"
	ActionReportGenerate = "report:generate"

	// Audit
	ActionAuditRead = "audit:read"

	// Billing
	ActionBillingRead           = "billing:read"
	ActionBillingUpdatePlan     = "billing:update_plan"
	ActionBillingCancel         = "billing:cancel"
	ActionBillingDowngradeCheck = "billing:downgrade_check"
)

// Project-scoped actions
const (
	ActionProjectRead   = "project:read"
	ActionProjectUpdate = "project:update"
	ActionProjectDelete = "project:delete"

	// Project member management
	ActionProjectMemberList       = "project_member:list"
	ActionProjectMemberAdd        = "project_member:add"
	ActionProjectMemberUpdateRole = "project_member:update_role"
	ActionProjectMemberRemove     = "project_member:remove"

	// API key management (project-scoped)
	ActionAPIKeyList   = "api_key:list"
	ActionAPIKeyRead   = "api_key:read"
	ActionAPIKeyCreate = "api_key:create"
	ActionAPIKeyUpdate = "api_key:update"
	ActionAPIKeyRevoke = "api_key:revoke"

	// Limit management (project-scoped)
	ActionLimitList   = "limit:list"
	ActionLimitRead   = "limit:read"
	ActionLimitCreate = "limit:create"
	ActionLimitUpdate = "limit:update"
	ActionLimitDelete = "limit:delete"
)

// Scope identifies whether an action is checked at the tenant or project level.
type Scope int

const (
	ScopeTenant  Scope = iota
	ScopeProject
)

// ActionScope maps each action to its scope.
var ActionScope = map[string]Scope{
	// Tenant-scoped
	ActionOrgRead:               ScopeTenant,
	ActionOrgUpdate:             ScopeTenant,
	ActionOrgDelete:             ScopeTenant,
	ActionMemberList:            ScopeTenant,
	ActionMemberInvite:          ScopeTenant,
	ActionMemberUpdateRole:      ScopeTenant,
	ActionMemberRemove:          ScopeTenant,
	ActionProjectList:           ScopeTenant,
	ActionProjectCreate:         ScopeTenant,
	ActionProviderKeyList:       ScopeTenant,
	ActionProviderKeyRead:       ScopeTenant,
	ActionProviderKeyCreate:     ScopeTenant,
	ActionProviderKeyUpdate:     ScopeTenant,
	ActionProviderKeyDelete:     ScopeTenant,
	ActionUsageRead:             ScopeTenant,
	ActionReportGenerate:        ScopeTenant,
	ActionAuditRead:             ScopeTenant,
	ActionBillingRead:           ScopeTenant,
	ActionBillingUpdatePlan:     ScopeTenant,
	ActionBillingCancel:         ScopeTenant,
	ActionBillingDowngradeCheck: ScopeTenant,

	// Project-scoped
	ActionProjectRead:             ScopeProject,
	ActionProjectUpdate:           ScopeProject,
	ActionProjectDelete:           ScopeProject,
	ActionProjectMemberList:       ScopeProject,
	ActionProjectMemberAdd:        ScopeProject,
	ActionProjectMemberUpdateRole: ScopeProject,
	ActionProjectMemberRemove:     ScopeProject,
	ActionAPIKeyList:              ScopeProject,
	ActionAPIKeyRead:              ScopeProject,
	ActionAPIKeyCreate:            ScopeProject,
	ActionAPIKeyUpdate:            ScopeProject,
	ActionAPIKeyRevoke:            ScopeProject,
	ActionLimitList:               ScopeProject,
	ActionLimitRead:               ScopeProject,
	ActionLimitCreate:             ScopeProject,
	ActionLimitUpdate:             ScopeProject,
	ActionLimitDelete:             ScopeProject,
}

// MultiUserActions are actions that are only available on Team/Business plans.
var MultiUserActions = map[string]bool{
	ActionMemberInvite:     true,
	ActionMemberUpdateRole: true,
	ActionMemberRemove:     true,
}
