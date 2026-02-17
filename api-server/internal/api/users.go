package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

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

// handleUpdateUserRole changes a user's role.
// PATCH /v1/admin/users/:user_id/role
func (s *Server) handleUpdateUserRole(c *gin.Context) {
	// TODO: implement
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// handleSuspendUser suspends a user account.
// PATCH /v1/admin/users/:user_id/suspend
func (s *Server) handleSuspendUser(c *gin.Context) {
	userID := c.Param("user_id")
	if err := s.postgresDB.GetDB().Model(&models.User{}).Where("id = ?", userID).Update("status", models.StatusSuspended).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"suspended": userID})
}

// handleUnsuspendUser reactivates a user account.
// PATCH /v1/admin/users/:user_id/unsuspend
func (s *Server) handleUnsuspendUser(c *gin.Context) {
	userID := c.Param("user_id")
	if err := s.postgresDB.GetDB().Model(&models.User{}).Where("id = ?", userID).Update("status", models.StatusActive).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"unsuspended": userID})
}
