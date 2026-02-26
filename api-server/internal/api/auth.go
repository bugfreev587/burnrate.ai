package api

import (
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

type authSyncReq struct {
	ClerkUserID string `json:"clerk_user_id" binding:"required"`
	Email       string `json:"email"         binding:"required"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
}

// handleAuthSync syncs a Clerk-authenticated user with the local database.
// POST /v1/auth/sync
//
// Flow:
//  1. User exists by Clerk ID → return existing user + tenant
//  2. User exists by email (different Clerk ID) → update ID and return
//  3. Pending invitation found by email → activate with invited role + tenant
//  4. Neither → create new tenant and make caller the owner (in a transaction
//     so concurrent duplicate calls don't leave orphan tenants)
func (s *Server) handleAuthSync(c *gin.Context) {
	// If an auth sync secret is configured, require it in the request header.
	if secret := s.cfg.Security.AuthSyncSecret; secret != "" {
		provided := c.GetHeader("X-Auth-Sync-Secret")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or missing auth sync secret"})
			return
		}
	}

	var req authSyncReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := s.postgresDB.GetDB()

	name := req.FirstName
	if req.LastName != "" {
		name += " " + req.LastName
	}
	if name == "" || name == " " {
		name = req.Email
	}

	// ── 1. Existing user by Clerk ID ────────────────────────────────────────
	var user models.User
	if err := db.Where("id = ?", req.ClerkUserID).First(&user).Error; err == nil {
		if user.Status == models.StatusSuspended {
			c.JSON(http.StatusForbidden, gin.H{"error": "user_suspended"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"user_id":     user.ID,
			"tenant_id":   user.TenantID,
			"email":       user.Email,
			"name":        user.Name,
			"role":        user.Role,
			"status":      user.Status,
			"is_new_user": false,
		})
		return
	}

	// ── 2. Existing active user by email (e.g. Clerk ID changed) ────────────
	var byEmail models.User
	if err := db.Where("email = ? AND status != ?", req.Email, models.StatusPending).First(&byEmail).Error; err == nil {
		if byEmail.Status == models.StatusSuspended {
			c.JSON(http.StatusForbidden, gin.H{"error": "user_suspended"})
			return
		}
		// Update the Clerk ID to the new one so future lookups hit step 1.
		db.Model(&byEmail).Update("id", req.ClerkUserID)
		c.JSON(http.StatusOK, gin.H{
			"user_id":     req.ClerkUserID,
			"tenant_id":   byEmail.TenantID,
			"email":       byEmail.Email,
			"name":        byEmail.Name,
			"role":        byEmail.Role,
			"status":      byEmail.Status,
			"is_new_user": false,
		})
		return
	}

	// ── 3. Pending invitation by email ──────────────────────────────────────
	var pending models.User
	if err := db.Where("email = ? AND status = ?", req.Email, models.StatusPending).First(&pending).Error; err == nil {
		oldID := pending.ID
		pending.ID = req.ClerkUserID
		pending.Name = name
		pending.Status = models.StatusActive

		if err := db.Delete(&models.User{}, "id = ?", oldID).Error; err != nil {
			slog.Error("auth_sync_delete_pending_user_failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to activate invited user"})
			return
		}
		if err := db.Create(&pending).Error; err != nil {
			slog.Error("auth_sync_create_activated_user_failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to activate invited user"})
			return
		}
		slog.Info("auth_sync_user_activated", "email", pending.Email, "tenant_id", pending.TenantID, "role", pending.Role)
		c.JSON(http.StatusOK, gin.H{
			"user_id":     pending.ID,
			"tenant_id":   pending.TenantID,
			"email":       pending.Email,
			"name":        pending.Name,
			"role":        pending.Role,
			"status":      pending.Status,
			"is_new_user": true,
		})
		return
	}

	// ── 4. Brand-new user — create tenant + owner in a transaction ───────────
	// Wrapped in a transaction so that if a concurrent duplicate request also
	// reaches this point, only one succeeds and the other rolls back cleanly
	// instead of leaving an orphan tenant behind.
	var newUser models.User
	var newTenant models.Tenant

	txErr := db.Transaction(func(tx *gorm.DB) error {
		freeLimits := models.GetPlanLimits(models.PlanFree)
		newTenant = models.Tenant{
			Name:       fmt.Sprintf("%s's Workspace", name),
			Plan:       models.PlanFree,
			MaxAPIKeys: freeLimits.MaxAPIKeys,
			CreatedAt:  time.Now(),
		}
		if err := tx.Create(&newTenant).Error; err != nil {
			return fmt.Errorf("create tenant: %w", err)
		}

		newUser = models.User{
			ID:        req.ClerkUserID,
			TenantID:  newTenant.ID,
			Email:     req.Email,
			Name:      name,
			Role:      models.RoleOwner,
			Status:    models.StatusActive,
			CreatedAt: time.Now(),
		}
		if err := tx.Create(&newUser).Error; err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		return nil
	})

	if txErr != nil {
		slog.Error("auth_sync_transaction_failed", "email", req.Email, "clerk_user_id", req.ClerkUserID, "error", txErr)

		// The transaction rolled back. Another concurrent request may have
		// succeeded. Look up the user one more time before giving up.
		var recovered models.User
		if err := db.Where("id = ? OR email = ?", req.ClerkUserID, req.Email).First(&recovered).Error; err == nil {
			if recovered.Status == models.StatusSuspended {
				c.JSON(http.StatusForbidden, gin.H{"error": "user_suspended"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"user_id":     recovered.ID,
				"tenant_id":   recovered.TenantID,
				"email":       recovered.Email,
				"name":        recovered.Name,
				"role":        recovered.Role,
				"status":      recovered.Status,
				"is_new_user": false,
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	slog.Info("auth_sync_new_tenant", "email", newUser.Email, "tenant_id", newTenant.ID, "user_id", newUser.ID)
	c.JSON(http.StatusCreated, gin.H{
		"user_id":     newUser.ID,
		"tenant_id":   newTenant.ID,
		"email":       newUser.Email,
		"name":        newUser.Name,
		"role":        newUser.Role,
		"status":      newUser.Status,
		"is_new_user": true,
	})
}
