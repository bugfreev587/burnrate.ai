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
	return Deny("insufficient project role for action " + action)
}
