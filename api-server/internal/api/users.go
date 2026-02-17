package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/middleware"
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/models"
)

// handleListUsers returns all users.
// GET /v1/admin/users
func (s *Server) handleListUsers(c *gin.Context) {
	var users []models.User
	if err := s.postgresDB.GetDB().Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

type updateRoleReq struct {
	Role string `json:"role" binding:"required"`
}

// handleUpdateUserRole changes a user's role (viewer/editor only; admin/owner via owner routes).
// PATCH /v1/admin/users/:user_id/role
func (s *Server) handleUpdateUserRole(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req updateRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Admins can only assign viewer or editor
	if req.Role != models.RoleViewer && req.Role != models.RoleEditor {
		c.JSON(http.StatusForbidden, gin.H{"error": "admins can only assign viewer or editor roles"})
		return
	}

	targetID := c.Param("user_id")
	if targetID == caller.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot change your own role"})
		return
	}

	// Cannot demote another admin/owner unless you are owner
	var target models.User
	if err := s.postgresDB.GetDB().First(&target, "id = ?", targetID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if models.RoleLevel(target.Role) >= models.RoleLevel(models.RoleAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot change role of admin or owner"})
		return
	}

	if err := s.postgresDB.GetDB().Model(&models.User{}).
		Where("id = ?", targetID).
		Update("role", req.Role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user_id": targetID, "role": req.Role})
}

// handleSuspendUser suspends a user account.
// PATCH /v1/admin/users/:user_id/suspend
func (s *Server) handleSuspendUser(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	targetID := c.Param("user_id")
	if targetID == caller.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot suspend yourself"})
		return
	}

	var target models.User
	if err := s.postgresDB.GetDB().First(&target, "id = ?", targetID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if target.Role == models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot suspend the owner"})
		return
	}
	// Admins cannot suspend other admins
	if target.Role == models.RoleAdmin && caller.Role != models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can suspend admins"})
		return
	}

	if err := s.postgresDB.GetDB().Model(&models.User{}).
		Where("id = ?", targetID).
		Update("status", models.StatusSuspended).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"suspended": targetID})
}

// handleUnsuspendUser reactivates a user account.
// PATCH /v1/admin/users/:user_id/unsuspend
func (s *Server) handleUnsuspendUser(c *gin.Context) {
	targetID := c.Param("user_id")
	if err := s.postgresDB.GetDB().Model(&models.User{}).
		Where("id = ?", targetID).
		Update("status", models.StatusActive).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"unsuspended": targetID})
}

// handleRemoveUser deletes a user record.
// DELETE /v1/admin/users/:user_id
func (s *Server) handleRemoveUser(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	targetID := c.Param("user_id")
	if targetID == caller.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot remove yourself"})
		return
	}

	var target models.User
	if err := s.postgresDB.GetDB().First(&target, "id = ?", targetID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if target.Role == models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot remove the owner"})
		return
	}
	if target.Role == models.RoleAdmin && caller.Role != models.RoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can remove admins"})
		return
	}

	if err := s.postgresDB.GetDB().Delete(&models.User{}, "id = ?", targetID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"removed": targetID})
}
