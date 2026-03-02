package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// ─── List Notification Channels ─────────────────────────────────────────────

// GET /v1/admin/notifications
func (s *Server) handleListNotificationChannels(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var channels []models.NotificationChannel
	if err := s.postgresDB.GetDB().
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Find(&channels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"notification_channels": channels})
}

// ─── Create Notification Channel ────────────────────────────────────────────

type createNotificationChannelReq struct {
	ChannelType string          `json:"channel_type" binding:"required"`
	Name        string          `json:"name"`
	Config      json.RawMessage `json:"config" binding:"required"`
	EventTypes  []string        `json:"event_types" binding:"required"`
	Enabled     *bool           `json:"enabled"`
}

// POST /v1/admin/notifications
func (s *Server) handleCreateNotificationChannel(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req createNotificationChannelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate channel type
	switch req.ChannelType {
	case "email", "slack", "webhook":
		// ok
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "channel_type must be email, slack, or webhook"})
		return
	}

	// Validate event types
	for _, et := range req.EventTypes {
		switch et {
		case models.EventBudgetBlocked, models.EventBudgetWarning, models.EventRateLimitExceeded:
			// ok
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event_type: " + et})
			return
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Enforce notification channel count.
	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant"})
		return
	}
	planLim := models.GetPlanLimits(tenant.Plan)
	if planLim.MaxNotificationChannels != -1 {
		var channelCount int64
		s.postgresDB.GetDB().Model(&models.NotificationChannel{}).Where("tenant_id = ?", tenantID).Count(&channelCount)
		if int(channelCount) >= planLim.MaxNotificationChannels {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "plan_limit_reached",
				"message": fmt.Sprintf("Your %s plan allows up to %d notification channel(s). Upgrade to add more.", tenant.Plan, planLim.MaxNotificationChannels),
				"limit":   planLim.MaxNotificationChannels,
				"current": channelCount,
				"plan":    tenant.Plan,
			})
			return
		}
	}

	channel := models.NotificationChannel{
		TenantID:    tenantID,
		ChannelType: req.ChannelType,
		Name:        req.Name,
		Config:      string(req.Config),
		EventTypes:  pq.StringArray(req.EventTypes),
		Enabled:     enabled,
	}

	if err := s.postgresDB.GetDB().Create(&channel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.recordAuditEvent(c, models.AuditNotificationChannelCreated, "notification_channel", fmt.Sprintf("%d", channel.ID), AuditOpts{
		Category: models.AuditCategoryConfig,
		AfterState: map[string]interface{}{
			"type": channel.ChannelType,
			"name": channel.Name,
		},
	})

	c.JSON(http.StatusCreated, channel)
}

// ─── Update Notification Channel ────────────────────────────────────────────

type updateNotificationChannelReq struct {
	ChannelType string          `json:"channel_type"`
	Name        string          `json:"name"`
	Config      json.RawMessage `json:"config"`
	EventTypes  []string        `json:"event_types"`
	Enabled     *bool           `json:"enabled"`
}

// PUT /v1/admin/notifications/:id
func (s *Server) handleUpdateNotificationChannel(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	db := s.postgresDB.GetDB()
	var channel models.NotificationChannel
	if err := db.First(&channel, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification channel not found"})
		return
	}

	if channel.TenantID != tenantID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	var req updateNotificationChannelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ChannelType != "" {
		switch req.ChannelType {
		case "email", "slack", "webhook":
			channel.ChannelType = req.ChannelType
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "channel_type must be email, slack, or webhook"})
			return
		}
	}

	if req.Name != "" {
		channel.Name = req.Name
	}

	if req.Config != nil {
		channel.Config = string(req.Config)
	}

	if req.EventTypes != nil {
		for _, et := range req.EventTypes {
			switch et {
			case models.EventBudgetBlocked, models.EventBudgetWarning, models.EventRateLimitExceeded:
				// ok
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event_type: " + et})
				return
			}
		}
		channel.EventTypes = pq.StringArray(req.EventTypes)
	}

	if req.Enabled != nil {
		channel.Enabled = *req.Enabled
	}

	if err := db.Save(&channel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.recordAuditEvent(c, models.AuditNotificationChannelUpdated, "notification_channel", fmt.Sprintf("%d", channel.ID), AuditOpts{
		Category: models.AuditCategoryConfig,
	})

	c.JSON(http.StatusOK, channel)
}

// ─── Delete Notification Channel ────────────────────────────────────────────

// DELETE /v1/admin/notifications/:id
func (s *Server) handleDeleteNotificationChannel(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	db := s.postgresDB.GetDB()
	var channel models.NotificationChannel
	if err := db.First(&channel, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification channel not found"})
		return
	}

	if channel.TenantID != tenantID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if err := db.Delete(&channel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.recordAuditEvent(c, models.AuditNotificationChannelDeleted, "notification_channel", c.Param("id"), AuditOpts{
		Category: models.AuditCategoryConfig,
	})

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// ─── Test Notification Channel ──────────────────────────────────────────────

// POST /v1/admin/notifications/:id/test
func (s *Server) handleTestNotificationChannel(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	db := s.postgresDB.GetDB()
	var channel models.NotificationChannel
	if err := db.First(&channel, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification channel not found"})
		return
	}

	if channel.TenantID != tenantID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if s.notifWorker == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "notification worker not configured"})
		return
	}

	if err := s.notifWorker.SendTestNotification(c.Request.Context(), channel); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
