package api

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/models"
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
//  2. Pending invitation found by email → activate with invited role + tenant
//  3. Neither → create new tenant and make caller the owner
func (s *Server) handleAuthSync(c *gin.Context) {
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
		var tenant models.Tenant
		db.First(&tenant, user.TenantID)
		c.JSON(http.StatusOK, gin.H{
			"user_id":    user.ID,
			"tenant_id":  user.TenantID,
			"email":      user.Email,
			"name":       user.Name,
			"role":       user.Role,
			"status":     user.Status,
			"is_new_user": false,
		})
		return
	}

	// ── 2. Pending invitation by email ──────────────────────────────────────
	var pending models.User
	if err := db.Where("email = ? AND status = ?", req.Email, models.StatusPending).First(&pending).Error; err == nil {
		oldID := pending.ID
		pending.ID = req.ClerkUserID
		pending.Name = name
		pending.Status = models.StatusActive

		if err := db.Delete(&models.User{}, "id = ?", oldID).Error; err != nil {
			log.Printf("failed to delete pending user: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to activate invited user"})
			return
		}
		if err := db.Create(&pending).Error; err != nil {
			log.Printf("failed to create activated user: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to activate invited user"})
			return
		}
		log.Printf("activated invited user: email=%s tenant_id=%d role=%s", pending.Email, pending.TenantID, pending.Role)
		c.JSON(http.StatusOK, gin.H{
			"user_id":    pending.ID,
			"tenant_id":  pending.TenantID,
			"email":      pending.Email,
			"name":       pending.Name,
			"role":       pending.Role,
			"status":     pending.Status,
			"is_new_user": true,
		})
		return
	}

	// ── 3. New user — create tenant + owner ─────────────────────────────────
	tenant := models.Tenant{
		Name:      fmt.Sprintf("%s's Workspace", name),
		CreatedAt: time.Now(),
	}
	if err := db.Create(&tenant).Error; err != nil {
		log.Printf("failed to create tenant: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create tenant"})
		return
	}

	newUser := models.User{
		ID:        req.ClerkUserID,
		TenantID:  tenant.ID,
		Email:     req.Email,
		Name:      name,
		Role:      models.RoleOwner,
		Status:    models.StatusActive,
		CreatedAt: time.Now(),
	}
	if err := db.Create(&newUser).Error; err != nil {
		log.Printf("failed to create user: %v", err)
		db.Delete(&tenant)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	log.Printf("new tenant+owner: email=%s tenant_id=%d user_id=%s", newUser.Email, tenant.ID, newUser.ID)
	c.JSON(http.StatusCreated, gin.H{
		"user_id":    newUser.ID,
		"tenant_id":  tenant.ID,
		"email":      newUser.Email,
		"name":       newUser.Name,
		"role":       newUser.Role,
		"status":     newUser.Status,
		"is_new_user": true,
	})
}
