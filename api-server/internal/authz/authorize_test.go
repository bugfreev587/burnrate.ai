package authz

import (
	"testing"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

func TestHasOrgPermission(t *testing.T) {
	tests := []struct {
		role    string
		action  string
		allowed bool
	}{
		// Owner can do everything
		{models.RoleOwner, ActionOrgRead, true},
		{models.RoleOwner, ActionOrgUpdate, true},
		{models.RoleOwner, ActionOrgDelete, true},
		{models.RoleOwner, ActionMemberInvite, true},
		{models.RoleOwner, ActionProviderKeyCreate, true},
		{models.RoleOwner, ActionBillingUpdatePlan, true},
		{models.RoleOwner, ActionBillingCancel, true},
		{models.RoleOwner, ActionAuditRead, true},
		{models.RoleOwner, ActionUsageRead, true},

		// Admin can do most things except billing mutations
		{models.RoleAdmin, ActionOrgRead, true},
		{models.RoleAdmin, ActionOrgUpdate, true},
		{models.RoleAdmin, ActionMemberInvite, true},
		{models.RoleAdmin, ActionProviderKeyCreate, true},
		{models.RoleAdmin, ActionAuditRead, true},
		{models.RoleAdmin, ActionBillingRead, true},
		{models.RoleAdmin, ActionBillingUpdatePlan, false},
		{models.RoleAdmin, ActionBillingCancel, false},
		{models.RoleAdmin, ActionOrgDelete, false},

		// Editor has limited tenant-scoped access
		{models.RoleEditor, ActionProjectList, true},
		{models.RoleEditor, ActionUsageRead, true},
		{models.RoleEditor, ActionOrgRead, false},
		{models.RoleEditor, ActionMemberInvite, false},
		{models.RoleEditor, ActionProviderKeyCreate, false},
		{models.RoleEditor, ActionAuditRead, false},

		// Viewer has minimal tenant-scoped access
		{models.RoleViewer, ActionProjectList, true},
		{models.RoleViewer, ActionUsageRead, true},
		{models.RoleViewer, ActionBillingRead, true},
		{models.RoleViewer, ActionOrgRead, false},
		{models.RoleViewer, ActionMemberInvite, false},
		{models.RoleViewer, ActionProviderKeyCreate, false},
	}

	for _, tt := range tests {
		t.Run(tt.role+"_"+tt.action, func(t *testing.T) {
			got := HasOrgPermission(tt.role, tt.action)
			if got != tt.allowed {
				t.Errorf("HasOrgPermission(%q, %q) = %v, want %v", tt.role, tt.action, got, tt.allowed)
			}
		})
	}
}

func TestHasProjectPermission(t *testing.T) {
	tests := []struct {
		role    string
		action  string
		allowed bool
	}{
		// Project admin
		{models.ProjectRoleAdmin, ActionProjectRead, true},
		{models.ProjectRoleAdmin, ActionProjectUpdate, true},
		{models.ProjectRoleAdmin, ActionProjectDelete, true},
		{models.ProjectRoleAdmin, ActionProjectMemberAdd, true},
		{models.ProjectRoleAdmin, ActionAPIKeyCreate, true},
		{models.ProjectRoleAdmin, ActionLimitCreate, true},

		// Project editor
		{models.ProjectRoleEditor, ActionProjectRead, true},
		{models.ProjectRoleEditor, ActionProjectUpdate, true},
		{models.ProjectRoleEditor, ActionAPIKeyCreate, true},
		{models.ProjectRoleEditor, ActionAPIKeyList, true},
		{models.ProjectRoleEditor, ActionProjectDelete, false},
		{models.ProjectRoleEditor, ActionProjectMemberAdd, false},
		{models.ProjectRoleEditor, ActionLimitCreate, false},

		// Project viewer
		{models.ProjectRoleViewer, ActionProjectRead, true},
		{models.ProjectRoleViewer, ActionAPIKeyList, true},
		{models.ProjectRoleViewer, ActionAPIKeyRead, true},
		{models.ProjectRoleViewer, ActionLimitList, true},
		{models.ProjectRoleViewer, ActionProjectUpdate, false},
		{models.ProjectRoleViewer, ActionAPIKeyCreate, false},
		{models.ProjectRoleViewer, ActionLimitCreate, false},
	}

	for _, tt := range tests {
		t.Run(tt.role+"_"+tt.action, func(t *testing.T) {
			got := HasProjectPermission(tt.role, tt.action)
			if got != tt.allowed {
				t.Errorf("HasProjectPermission(%q, %q) = %v, want %v", tt.role, tt.action, got, tt.allowed)
			}
		})
	}
}

func TestActionScopeComplete(t *testing.T) {
	// Ensure every action listed in permission maps is in ActionScope.
	for role, perms := range orgRolePermissions {
		for action := range perms {
			if _, ok := ActionScope[action]; !ok {
				t.Errorf("org role %q has action %q not in ActionScope", role, action)
			}
		}
	}
	for role, perms := range projectRolePermissions {
		for action := range perms {
			if _, ok := ActionScope[action]; !ok {
				t.Errorf("project role %q has action %q not in ActionScope", role, action)
			}
		}
	}
}
