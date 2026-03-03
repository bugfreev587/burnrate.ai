package authz

import (
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"gorm.io/gorm"
)

// Resource identifies the target of an authorization check.
// For project-scoped actions, ProjectID must be set.
type Resource struct {
	ProjectID uint
}

// Decision is the result of an authorization check.
type Decision struct {
	Allowed bool
	Reason  string
}

func Deny(reason string) Decision  { return Decision{Allowed: false, Reason: reason} }
func Allow() Decision              { return Decision{Allowed: true} }

// Authorize is the central authorization function.
// It checks whether userID has permission to perform action on resource within tenantID.
//
// For tenant-scoped actions: checks orgRolePermissions[orgRole] contains action.
// For project-scoped actions: owner/admin bypass; else checks project_membership exists
//   and projectRolePermissions[projectRole] contains action.
//
// Multi-user-only actions (member:invite, etc.) are denied on Free/Pro plans.
func Authorize(db *gorm.DB, userID string, tenantID uint, orgRole string, action string, resource Resource) Decision {
	// Check if this action exists in our scope map.
	scope, known := ActionScope[action]
	if !known {
		return Deny("unknown action")
	}

	// Multi-user actions require Team+ plans.
	if MultiUserActions[action] {
		var tenant models.Tenant
		if err := db.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
			return Deny("tenant not found")
		}
		if tenant.Plan == models.PlanFree || tenant.Plan == models.PlanPro {
			return Deny("multi-user actions require Team or Business plan")
		}
	}

	if scope == ScopeTenant {
		if HasOrgPermission(orgRole, action) {
			return Allow()
		}
		return Deny("insufficient org role for action " + action)
	}

	// Project-scoped action.
	// Owner and Admin have implicit access to all projects in their tenant.
	if orgRole == models.RoleOwner || orgRole == models.RoleAdmin {
		return Allow()
	}

	// For other roles, check project membership.
	if resource.ProjectID == 0 {
		return Deny("project_id required for project-scoped action")
	}

	var pm models.ProjectMembership
	err := db.Where("project_id = ? AND user_id = ?", resource.ProjectID, userID).First(&pm).Error
	if err != nil {
		return Deny("not a member of this project")
	}

	if HasProjectPermission(pm.ProjectRole, action) {
		return Allow()
	}
	return Deny(projectDenyMessage(pm.ProjectRole, action))
}

// projectDenyMessage returns a user-friendly message explaining why a project
// role is insufficient for the requested action and which role is required.
func projectDenyMessage(role, action string) string {
	required := minProjectRole(action)
	roleName := friendlyProjectRole(role)
	requiredName := friendlyProjectRole(required)

	actionLabel, ok := friendlyActionLabels[action]
	if !ok {
		actionLabel = action
	}

	return "Your project role is " + roleName + ", but " + actionLabel +
		" requires " + requiredName + " or above. Ask a project admin to upgrade your role."
}

func minProjectRole(action string) string {
	// Check from least privileged upward.
	if HasProjectPermission(models.ProjectRoleViewer, action) {
		return models.ProjectRoleViewer
	}
	if HasProjectPermission(models.ProjectRoleEditor, action) {
		return models.ProjectRoleEditor
	}
	return models.ProjectRoleAdmin
}

func friendlyProjectRole(role string) string {
	switch role {
	case models.ProjectRoleAdmin:
		return "Project Admin"
	case models.ProjectRoleEditor:
		return "Project Editor"
	case models.ProjectRoleViewer:
		return "Project Viewer"
	default:
		return role
	}
}

var friendlyActionLabels = map[string]string{
	ActionAPIKeyCreate:            "creating API keys",
	ActionAPIKeyUpdate:            "updating API keys",
	ActionAPIKeyRevoke:            "revoking API keys",
	ActionLimitCreate:             "creating limits",
	ActionLimitUpdate:             "updating limits",
	ActionLimitDelete:             "deleting limits",
	ActionProjectUpdate:           "updating the project",
	ActionProjectDelete:           "deleting the project",
	ActionProjectMemberList:       "listing project members",
	ActionProjectMemberAdd:        "adding project members",
	ActionProjectMemberUpdateRole: "changing member roles",
	ActionProjectMemberRemove:     "removing project members",
}
