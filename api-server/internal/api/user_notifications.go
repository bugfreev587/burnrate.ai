package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// ─── In-app notifications ───────────────────────────────────────────────────

// GET /v1/user/notifications
func (s *Server) handleListUserNotifications(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	var rows []models.UserNotification
	if err := s.postgresDB.GetDB().
		Where("user_id = ?", caller.ID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch notifications"})
		return
	}

	var unread int64
	s.postgresDB.GetDB().Model(&models.UserNotification{}).
		Where("user_id = ? AND status = ?", caller.ID, models.UserNotificationStatusUnread).
		Count(&unread)

	c.JSON(http.StatusOK, gin.H{
		"notifications": rows,
		"unread_count":  unread,
	})
}

// PATCH /v1/user/notifications/:id/read
func (s *Server) handleMarkUserNotificationRead(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	db := s.postgresDB.GetDB()
	now := time.Now()
	if err := db.Model(&models.UserNotification{}).
		Where("id = ? AND user_id = ?", id, caller.ID).
		Updates(map[string]interface{}{
			"status":  models.UserNotificationStatusRead,
			"read_at": &now,
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update notification"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// PATCH /v1/user/notifications/read-all
func (s *Server) handleMarkAllUserNotificationsRead(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	now := time.Now()
	if err := s.postgresDB.GetDB().Model(&models.UserNotification{}).
		Where("user_id = ? AND status = ?", caller.ID, models.UserNotificationStatusUnread).
		Updates(map[string]interface{}{
			"status":  models.UserNotificationStatusRead,
			"read_at": &now,
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update notifications"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── Personal channels ──────────────────────────────────────────────────────

type createUserNotificationChannelReq struct {
	ChannelType string          `json:"channel_type" binding:"required"`
	Name        string          `json:"name"`
	Config      json.RawMessage `json:"config" binding:"required"`
	EventTypes  []string        `json:"event_types" binding:"required"`
	Enabled     *bool           `json:"enabled"`
}

type updateUserNotificationChannelReq struct {
	ChannelType string          `json:"channel_type"`
	Name        string          `json:"name"`
	Config      json.RawMessage `json:"config"`
	EventTypes  []string        `json:"event_types"`
	Enabled     *bool           `json:"enabled"`
}

func validateUserEventType(et string) bool {
	switch et {
	case models.EventTeamInvitation:
		return true
	default:
		return false
	}
}

// GET /v1/user/notification-channels
func (s *Server) handleListUserNotificationChannels(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var channels []models.UserNotificationChannel
	if err := s.postgresDB.GetDB().
		Where("user_id = ?", caller.ID).
		Order("created_at DESC").
		Find(&channels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch channels"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"notification_channels": channels})
}

// POST /v1/user/notification-channels
func (s *Server) handleCreateUserNotificationChannel(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req createUserNotificationChannelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	switch req.ChannelType {
	case "email", "slack", "webhook":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "channel_type must be email, slack, or webhook"})
		return
	}
	for _, et := range req.EventTypes {
		if !validateUserEventType(et) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event_type: " + et})
			return
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	ch := models.UserNotificationChannel{
		UserID:      caller.ID,
		ChannelType: req.ChannelType,
		Name:        req.Name,
		Config:      string(req.Config),
		EventTypes:  pq.StringArray(req.EventTypes),
		Enabled:     enabled,
	}
	if err := s.postgresDB.GetDB().Create(&ch).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create channel"})
		return
	}
	c.JSON(http.StatusCreated, ch)
}

// PUT /v1/user/notification-channels/:id
func (s *Server) handleUpdateUserNotificationChannel(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req updateUserNotificationChannelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := s.postgresDB.GetDB()
	var ch models.UserNotificationChannel
	if err := db.First(&ch, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification channel not found"})
		return
	}
	if ch.UserID != caller.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if req.ChannelType != "" {
		switch req.ChannelType {
		case "email", "slack", "webhook":
			ch.ChannelType = req.ChannelType
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "channel_type must be email, slack, or webhook"})
			return
		}
	}
	if req.Name != "" {
		ch.Name = req.Name
	}
	if req.Config != nil {
		ch.Config = string(req.Config)
	}
	if req.EventTypes != nil {
		for _, et := range req.EventTypes {
			if !validateUserEventType(et) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event_type: " + et})
				return
			}
		}
		ch.EventTypes = pq.StringArray(req.EventTypes)
	}
	if req.Enabled != nil {
		ch.Enabled = *req.Enabled
	}

	if err := db.Save(&ch).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update channel"})
		return
	}
	c.JSON(http.StatusOK, ch)
}

// DELETE /v1/user/notification-channels/:id
func (s *Server) handleDeleteUserNotificationChannel(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	db := s.postgresDB.GetDB()
	var ch models.UserNotificationChannel
	if err := db.First(&ch, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification channel not found"})
		return
	}
	if ch.UserID != caller.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if err := db.Delete(&ch).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete channel"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// POST /v1/user/notification-channels/:id/test
func (s *Server) handleTestUserNotificationChannel(c *gin.Context) {
	caller, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	db := s.postgresDB.GetDB()
	var ch models.UserNotificationChannel
	if err := db.First(&ch, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification channel not found"})
		return
	}
	if ch.UserID != caller.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if s.notifWorker == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "notification worker not configured"})
		return
	}
	if err := s.notifWorker.SendTestUserNotificationChannel(ch); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
