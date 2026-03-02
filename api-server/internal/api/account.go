package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

type deleteAccountReq struct {
	ConfirmName string `json:"confirm_name" binding:"required"`
}

// handleDeleteAccount permanently deletes the tenant account.
// Requires the owner to type the workspace name to confirm (like GitHub repo deletion).
// DELETE /v1/owner/account
func (s *Server) handleDeleteAccount(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req deleteAccountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID, _ := middleware.GetTenantIDFromContext(c)
	db := s.postgresDB.GetDB()

	// 1. Fetch tenant and validate confirm_name
	var tenant models.Tenant
	if err := db.First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}

	if req.ConfirmName != tenant.Name {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "name_mismatch",
			"message": "The workspace name you entered does not match. Please type the exact workspace name to confirm.",
		})
		return
	}

	// 2. Cancel Stripe subscription immediately (best-effort)
	if tenant.StripeSubscriptionID != "" {
		if err := s.stripeSvc.CancelSubscriptionImmediately(tenant.StripeSubscriptionID); err != nil {
			slog.Warn("account_delete_cancel_stripe_failed", "subscription_id", tenant.StripeSubscriptionID, "tenant_id", tenant.ID, "error", err)
		}
	}

	// 3. Revoke all API keys
	if err := db.Model(&models.APIKey{}).
		Where("tenant_id = ?", tenant.ID).
		Update("revoked", true).Error; err != nil {
		slog.Warn("account_delete_revoke_api_keys_failed", "tenant_id", tenant.ID, "error", err)
	}

	// 4. Revoke all provider keys
	if err := db.Model(&models.ProviderKey{}).
		Where("tenant_id = ?", tenant.ID).
		Update("revoked", true).Error; err != nil {
		slog.Warn("account_delete_revoke_provider_keys_failed", "tenant_id", tenant.ID, "error", err)
	}

	// 5. Delete all tenant members from Clerk + DB
	var memberships []models.TenantMembership
	db.Where("tenant_id = ?", tenant.ID).Find(&memberships)
	var clerkErrors []string
	for _, m := range memberships {
		// Only delete from Clerk if user has no other tenant memberships.
		var otherCount int64
		db.Model(&models.TenantMembership{}).Where("user_id = ? AND tenant_id != ?", m.UserID, tenant.ID).Count(&otherCount)
		if otherCount == 0 {
			if err := s.deleteClerkUser(m.UserID); err != nil {
				slog.Error("account_delete_clerk_user_failed", "user_id", m.UserID, "tenant_id", tenant.ID, "error", err)
				clerkErrors = append(clerkErrors, fmt.Sprintf("%s: %v", m.UserID, err))
			}
		}
	}
	if len(clerkErrors) > 0 {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "clerk_deletion_failed",
			"message": fmt.Sprintf("Failed to delete %d user(s) from Clerk. Account cleanup was not completed. Please try again or contact support.", len(clerkErrors)),
		})
		return
	}

	// Delete project memberships for projects in this tenant.
	db.Exec("DELETE FROM project_memberships WHERE project_id IN (SELECT id FROM projects WHERE tenant_id = ?)", tenant.ID)
	// Delete projects.
	db.Where("tenant_id = ?", tenant.ID).Delete(&models.Project{})
	// Delete tenant memberships.
	db.Where("tenant_id = ?", tenant.ID).Delete(&models.TenantMembership{})
	// Delete users who have no remaining memberships.
	for _, m := range memberships {
		var remaining int64
		db.Model(&models.TenantMembership{}).Where("user_id = ?", m.UserID).Count(&remaining)
		if remaining == 0 {
			db.Delete(&models.User{}, "id = ?", m.UserID)
		}
	}

	// 6. Mark tenant as suspended + canceled
	db.Model(&tenant).Updates(map[string]any{
		"status":      models.StatusSuspended,
		"plan_status": models.PlanStatusCanceled,
	})

	s.recordAuditEvent(c, models.AuditAccountDeleted, "tenant", fmt.Sprintf("%d", tenant.ID), AuditOpts{
		Category: models.AuditCategoryOwner,
		Metadata: map[string]interface{}{
			"tenant_name": tenant.Name,
		},
	})

	slog.Info("account_deleted", "tenant_id", tenant.ID, "name", tenant.Name, "by", caller.Email)
	c.JSON(http.StatusOK, gin.H{"message": "Account deleted successfully"})
}
