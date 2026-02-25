package api

import (
	"log"
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

	db := s.postgresDB.GetDB()

	// 1. Fetch tenant and validate confirm_name
	var tenant models.Tenant
	if err := db.First(&tenant, caller.TenantID).Error; err != nil {
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
			log.Printf("warning: failed to cancel Stripe subscription %s for tenant %d: %v", tenant.StripeSubscriptionID, tenant.ID, err)
		}
	}

	// 3. Revoke all API keys
	if err := db.Model(&models.APIKey{}).
		Where("tenant_id = ?", tenant.ID).
		Update("revoked", true).Error; err != nil {
		log.Printf("warning: failed to revoke API keys for tenant %d: %v", tenant.ID, err)
	}

	// 4. Revoke all provider keys
	if err := db.Model(&models.ProviderKey{}).
		Where("tenant_id = ?", tenant.ID).
		Update("revoked", true).Error; err != nil {
		log.Printf("warning: failed to revoke provider keys for tenant %d: %v", tenant.ID, err)
	}

	// 5. Delete all users from Clerk + DB
	var users []models.User
	db.Where("tenant_id = ?", tenant.ID).Find(&users)
	for _, u := range users {
		if err := s.deleteClerkUser(u.ID); err != nil {
			log.Printf("warning: failed to delete Clerk user %s: %v", u.ID, err)
		}
	}
	db.Where("tenant_id = ?", tenant.ID).Delete(&models.User{})

	// 6. Mark tenant as suspended + canceled
	db.Model(&tenant).Updates(map[string]any{
		"status":      models.StatusSuspended,
		"plan_status": models.PlanStatusCanceled,
	})

	log.Printf("account deleted: tenant_id=%d name=%q by=%s", tenant.ID, tenant.Name, caller.Email)
	c.JSON(http.StatusOK, gin.H{"message": "Account deleted successfully"})
}
