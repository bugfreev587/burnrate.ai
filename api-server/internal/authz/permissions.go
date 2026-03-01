package authz

import "github.com/xiaoboyu/tokengate/api-server/internal/models"

// orgRolePermissions maps org roles to their allowed tenant-scoped actions.
var orgRolePermissions = map[string]map[string]bool{
	models.RoleOwner: {
		ActionOrgRead:               true,
		ActionOrgUpdate:             true,
		ActionOrgDelete:             true,
		ActionMemberList:            true,
		ActionMemberInvite:          true,
		ActionMemberUpdateRole:      true,
		ActionMemberRemove:          true,
		ActionProjectList:           true,
		ActionProjectCreate:         true,
		ActionProviderKeyList:       true,
		ActionProviderKeyRead:       true,
		ActionProviderKeyCreate:     true,
		ActionProviderKeyUpdate:     true,
		ActionProviderKeyDelete:     true,
		ActionUsageRead:             true,
		ActionReportGenerate:        true,
		ActionAuditRead:             true,
		ActionBillingRead:           true,
		ActionBillingUpdatePlan:     true,
		ActionBillingCancel:         true,
		ActionBillingDowngradeCheck: true,
	},
	models.RoleAdmin: {
		ActionOrgRead:               true,
		ActionOrgUpdate:             true,
		ActionMemberList:            true,
		ActionMemberInvite:          true,
		ActionMemberUpdateRole:      true,
		ActionMemberRemove:          true,
		ActionProjectList:           true,
		ActionProjectCreate:         true,
		ActionProviderKeyList:       true,
		ActionProviderKeyRead:       true,
		ActionProviderKeyCreate:     true,
		ActionProviderKeyUpdate:     true,
		ActionProviderKeyDelete:     true,
		ActionUsageRead:             true,
		ActionReportGenerate:        true,
		ActionAuditRead:             true,
		ActionBillingRead:           true,
		ActionBillingDowngradeCheck: true,
	},
	models.RoleEditor: {
		ActionProjectList: true,
		ActionUsageRead:   true,
	},
	models.RoleViewer: {
		ActionProjectList:  true,
		ActionUsageRead:    true,
		ActionBillingRead:  true, // plan_status only, enforced at handler level
	},
}

// projectRolePermissions maps project roles to their allowed project-scoped actions.
var projectRolePermissions = map[string]map[string]bool{
	models.ProjectRoleAdmin: {
		ActionProjectRead:             true,
		ActionProjectUpdate:           true,
		ActionProjectDelete:           true,
		ActionProjectMemberList:       true,
		ActionProjectMemberAdd:        true,
		ActionProjectMemberUpdateRole: true,
		ActionProjectMemberRemove:     true,
		ActionAPIKeyList:              true,
		ActionAPIKeyRead:              true,
		ActionAPIKeyCreate:            true,
		ActionAPIKeyUpdate:            true,
		ActionAPIKeyRevoke:            true,
		ActionLimitList:               true,
		ActionLimitRead:               true,
		ActionLimitCreate:             true,
		ActionLimitUpdate:             true,
		ActionLimitDelete:             true,
	},
	models.ProjectRoleEditor: {
		ActionProjectRead:   true,
		ActionProjectUpdate: true,
		ActionAPIKeyList:    true,
		ActionAPIKeyRead:    true,
		ActionAPIKeyCreate:  true,
		ActionAPIKeyUpdate:  true,
		ActionAPIKeyRevoke:  true,
		ActionLimitList:     true,
		ActionLimitRead:     true,
	},
	models.ProjectRoleViewer: {
		ActionProjectRead: true,
		ActionAPIKeyList:  true,
		ActionAPIKeyRead:  true,
		ActionLimitList:   true,
		ActionLimitRead:   true,
	},
}

// HasOrgPermission checks if the given org role has the specified tenant-scoped action.
func HasOrgPermission(orgRole, action string) bool {
	perms, ok := orgRolePermissions[orgRole]
	if !ok {
		return false
	}
	return perms[action]
}

// HasProjectPermission checks if the given project role has the specified project-scoped action.
func HasProjectPermission(projectRole, action string) bool {
	perms, ok := projectRolePermissions[projectRole]
	if !ok {
		return false
	}
	return perms[action]
}
