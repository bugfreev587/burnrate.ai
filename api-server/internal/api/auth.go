package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

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
func (s *Server) handleAuthSync(c *gin.Context) {
	var req authSyncReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := s.postgresDB.GetDB()

	// Check if user already exists (by Clerk ID or email)
	var user models.User
	err := db.First(&user, "id = ? OR email = ?", req.ClerkUserID, req.Email).Error
	if err == nil {
		// If found by email but with a different Clerk ID, update the ID
		if user.ID != req.ClerkUserID {
			db.Model(&user).Update("id", req.ClerkUserID)
			user.ID = req.ClerkUserID
		}
		if user.Status == models.StatusSuspended {
			c.JSON(http.StatusForbidden, gin.H{"error": "user_suspended"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"user_id":     user.ID,
			"email":       user.Email,
			"role":        user.Role,
			"status":      user.Status,
			"is_new_user": false,
		})
		return
	}

	if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	// New user — create record
	name := req.FirstName
	if req.LastName != "" {
		name += " " + req.LastName
	}

	// Check if this is the very first user — make them owner
	var count int64
	db.Model(&models.User{}).Count(&count)
	role := models.RoleViewer
	if count == 0 {
		role = models.RoleOwner
	}

	user = models.User{
		ID:     req.ClerkUserID,
		Email:  req.Email,
		Name:   name,
		Role:   role,
		Status: models.StatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"user_id":     user.ID,
		"email":       user.Email,
		"role":        user.Role,
		"status":      user.Status,
		"is_new_user": true,
	})
}
