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

// membershipResponse is the JSON shape for each entry in the memberships array.
type membershipResponse struct {
	TenantID   uint   `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	OrgRole    string `json:"org_role"`
}

// loadMemberships returns all active tenant memberships for the given user,
// joined with the tenants table to include the tenant name.
func loadMemberships(db *gorm.DB, userID string) []membershipResponse {
	var results []membershipResponse
	db.Table("tenant_memberships").
		Select("tenant_memberships.tenant_id, tenants.name AS tenant_name, tenant_memberships.org_role").
		Joins("JOIN tenants ON tenants.id = tenant_memberships.tenant_id").
		Where("tenant_memberships.user_id = ? AND tenant_memberships.status = ?", userID, models.StatusActive).
		Scan(&results)
	if results == nil {
		results = []membershipResponse{}
	}
	return results
}

// tenantHasAPIKeys returns true if any of the user's tenants has at least one (non-revoked) API key.
func tenantHasAPIKeys(db *gorm.DB, userID string) bool {
	var count int64
	db.Model(&models.APIKey{}).
		Where("revoked = false AND tenant_id IN (?)",
			db.Table("tenant_memberships").Select("tenant_id").
				Where("user_id = ? AND status = ?", userID, models.StatusActive),
		).Count(&count)
	return count > 0
}

// handleAuthSync syncs a Clerk-authenticated user with the local database.
// POST /v1/auth/sync
//
// Flow:
//  1. User exists by Clerk ID → return existing user + memberships
//  2. User exists by email (different Clerk ID) → update ID and return
//  3. Pending invitation found by email → activate invited memberships + create personal tenant
//  4. Neither → create new tenant, project, and memberships (in a transaction)
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
		memberships := loadMemberships(db, user.ID)
		c.JSON(http.StatusOK, gin.H{
			"user_id":      user.ID,
			"email":        user.Email,
			"name":         user.Name,
			"status":       user.Status,
			"is_new_user":  false,
			"has_api_keys": tenantHasAPIKeys(db, user.ID),
			"memberships":  memberships,
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
		oldID := byEmail.ID
		// Update the Clerk ID to the new one so future lookups hit step 1.
		db.Model(&byEmail).Update("id", req.ClerkUserID)
		// Update the user_id in tenant_memberships to match the new Clerk ID.
		db.Model(&models.TenantMembership{}).Where("user_id = ?", oldID).Update("user_id", req.ClerkUserID)
		// Update the user_id in project_memberships to match the new Clerk ID.
		db.Model(&models.ProjectMembership{}).Where("user_id = ?", oldID).Update("user_id", req.ClerkUserID)

		memberships := loadMemberships(db, req.ClerkUserID)
		c.JSON(http.StatusOK, gin.H{
			"user_id":      req.ClerkUserID,
			"email":        byEmail.Email,
			"name":         byEmail.Name,
			"status":       byEmail.Status,
			"is_new_user":  false,
			"has_api_keys": tenantHasAPIKeys(db, req.ClerkUserID),
			"memberships":  memberships,
		})
		return
	}

	// ── 3. Pending invitation by email ──────────────────────────────────────
	// Invitations create a User with status=pending and TenantMembership(s) with status=pending.
	var pending models.User
	if err := db.Where("email = ? AND status = ?", req.Email, models.StatusPending).First(&pending).Error; err == nil {
		oldID := pending.ID

		txErr := db.Transaction(func(tx *gorm.DB) error {
			// Delete the placeholder pending user and create the real one with the Clerk ID.
			if err := tx.Delete(&models.User{}, "id = ?", oldID).Error; err != nil {
				return fmt.Errorf("delete pending user: %w", err)
			}

			newUser := models.User{
				ID:        req.ClerkUserID,
				Email:     req.Email,
				Name:      name,
				Status:    models.StatusActive,
				CreatedAt: time.Now(),
			}
			if err := tx.Create(&newUser).Error; err != nil {
				return fmt.Errorf("create activated user: %w", err)
			}

			// Activate all pending TenantMembership(s) and re-point them to the new user ID.
			if err := tx.Model(&models.TenantMembership{}).
				Where("user_id = ? AND status = ?", oldID, models.StatusPending).
				Updates(map[string]interface{}{
					"user_id":    req.ClerkUserID,
					"status":     models.StatusActive,
					"updated_at": time.Now(),
				}).Error; err != nil {
				return fmt.Errorf("activate pending memberships: %w", err)
			}

			// Also migrate any other memberships that may exist for the old placeholder ID.
			tx.Model(&models.ProjectMembership{}).Where("user_id = ?", oldID).Update("user_id", req.ClerkUserID)

			// Create the user's personal Free tenant + default project + owner membership.
			personalTenant := models.Tenant{
				Name:      fmt.Sprintf("%s's Workspace", name),
				Plan:      models.PlanFree,
				CreatedAt: time.Now(),
			}
			if err := tx.Create(&personalTenant).Error; err != nil {
				return fmt.Errorf("create personal tenant: %w", err)
			}

			defaultProject := models.Project{
				TenantID:  personalTenant.ID,
				Name:      "Default",
				Status:    models.ProjectStatusActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			if err := tx.Create(&defaultProject).Error; err != nil {
				return fmt.Errorf("create default project: %w", err)
			}

			if err := tx.Model(&personalTenant).Update("default_project_id", defaultProject.ID).Error; err != nil {
				return fmt.Errorf("set default project: %w", err)
			}

			ownerMembership := models.TenantMembership{
				TenantID:  personalTenant.ID,
				UserID:    req.ClerkUserID,
				OrgRole:   models.RoleOwner,
				Status:    models.StatusActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			if err := tx.Create(&ownerMembership).Error; err != nil {
				return fmt.Errorf("create owner membership: %w", err)
			}

			projectMembership := models.ProjectMembership{
				ProjectID:   defaultProject.ID,
				UserID:      req.ClerkUserID,
				ProjectRole: models.ProjectRoleAdmin,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			if err := tx.Create(&projectMembership).Error; err != nil {
				return fmt.Errorf("create project membership: %w", err)
			}

			return nil
		})

		if txErr != nil {
			slog.Error("auth_sync_invitation_activation_failed", "email", req.Email, "error", txErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to activate invited user"})
			return
		}

		slog.Info("auth_sync_user_activated", "email", req.Email, "user_id", req.ClerkUserID)
		memberships := loadMemberships(db, req.ClerkUserID)
		c.JSON(http.StatusOK, gin.H{
			"user_id":      req.ClerkUserID,
			"email":        req.Email,
			"name":         name,
			"status":       models.StatusActive,
			"is_new_user":  true,
			"has_api_keys": tenantHasAPIKeys(db, req.ClerkUserID),
			"memberships":  memberships,
		})
		return
	}

	// ── 4. Brand-new user — create tenant + project + memberships in a transaction ─
	var newUser models.User
	var newTenant models.Tenant

	txErr := db.Transaction(func(tx *gorm.DB) error {
		newTenant = models.Tenant{
			Name:      fmt.Sprintf("%s's Workspace", name),
			Plan:      models.PlanFree,
			CreatedAt: time.Now(),
		}
		if err := tx.Create(&newTenant).Error; err != nil {
			return fmt.Errorf("create tenant: %w", err)
		}

		defaultProject := models.Project{
			TenantID:  newTenant.ID,
			Name:      "Default",
			Status:    models.ProjectStatusActive,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := tx.Create(&defaultProject).Error; err != nil {
			return fmt.Errorf("create default project: %w", err)
		}

		if err := tx.Model(&newTenant).Update("default_project_id", defaultProject.ID).Error; err != nil {
			return fmt.Errorf("set default project: %w", err)
		}

		newUser = models.User{
			ID:        req.ClerkUserID,
			Email:     req.Email,
			Name:      name,
			Status:    models.StatusActive,
			CreatedAt: time.Now(),
		}
		if err := tx.Create(&newUser).Error; err != nil {
			return fmt.Errorf("create user: %w", err)
		}

		ownerMembership := models.TenantMembership{
			TenantID:  newTenant.ID,
			UserID:    req.ClerkUserID,
			OrgRole:   models.RoleOwner,
			Status:    models.StatusActive,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := tx.Create(&ownerMembership).Error; err != nil {
			return fmt.Errorf("create owner membership: %w", err)
		}

		projectMembership := models.ProjectMembership{
			ProjectID:   defaultProject.ID,
			UserID:      req.ClerkUserID,
			ProjectRole: models.ProjectRoleAdmin,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := tx.Create(&projectMembership).Error; err != nil {
			return fmt.Errorf("create project membership: %w", err)
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
			memberships := loadMemberships(db, recovered.ID)
			c.JSON(http.StatusOK, gin.H{
				"user_id":      recovered.ID,
				"email":        recovered.Email,
				"name":         recovered.Name,
				"status":       recovered.Status,
				"is_new_user":  false,
				"has_api_keys": tenantHasAPIKeys(db, recovered.ID),
				"memberships":  memberships,
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	slog.Info("auth_sync_new_tenant", "email", newUser.Email, "tenant_id", newTenant.ID, "user_id", newUser.ID)
	memberships := loadMemberships(db, newUser.ID)
	c.JSON(http.StatusCreated, gin.H{
		"user_id":      newUser.ID,
		"email":        newUser.Email,
		"name":         newUser.Name,
		"status":       newUser.Status,
		"is_new_user":  true,
		"has_api_keys": false,
		"memberships":  memberships,
	})
}
