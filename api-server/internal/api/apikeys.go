package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

type createAPIKeyReq struct {
	Label     string     `json:"label"      binding:"required"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// handleCreateAPIKey creates a new tenant-scoped API key.
// POST /v1/admin/api_keys
func (s *Server) handleCreateAPIKey(c *gin.Context) {
	var req createAPIKeyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Enforce plan-based API key limit before hitting the service.
	var tenant models.Tenant
	s.postgresDB.GetDB().First(&tenant, user.TenantID)
	planLim := models.GetPlanLimits(tenant.Plan)
	if planLim.MaxAPIKeys != -1 {
		var activeCount int64
		s.postgresDB.GetDB().Model(&models.APIKey{}).
			Where("tenant_id = ? AND revoked = false AND (expires_at IS NULL OR expires_at > NOW())", user.TenantID).
			Count(&activeCount)
		if int(activeCount) >= planLim.MaxAPIKeys {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":       "plan_limit_reached",
				"message":     fmt.Sprintf("Your %s plan allows up to %d API key(s). Upgrade to add more.", tenant.Plan, planLim.MaxAPIKeys),
				"limit":       planLim.MaxAPIKeys,
				"active_keys": activeCount,
				"plan":        tenant.Plan,
			})
			return
		}
	}

	kid, secret, err := s.apiKeySvc.CreateKey(c.Request.Context(), user.TenantID, req.Label, req.Scopes, req.ExpiresAt)
	if err != nil {
		var limitErr *services.ErrAPIKeyLimitReached
		if errors.As(err, &limitErr) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":       "api_key_limit_reached",
				"message":     "You have reached your API key limit. Revoke an existing key or upgrade your plan.",
				"limit":       limitErr.Limit,
				"active_keys": limitErr.Current,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"key_id": kid,
		"secret": secret, // shown only once
		"label":  req.Label,
	})
}

// handleListAPIKeys lists all active API keys for the caller's tenant,
// including the tenant's key limit for display in the dashboard.
// GET /v1/admin/api_keys
func (s *Server) handleListAPIKeys(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	keys, err := s.apiKeySvc.ListKeys(c.Request.Context(), user.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fetch tenant for plan-aware limit display.
	var tenant models.Tenant
	s.postgresDB.GetDB().First(&tenant, user.TenantID)
	planLim := models.GetPlanLimits(tenant.Plan)

	type keyView struct {
		KeyID     string     `json:"key_id"`
		Label     string     `json:"label"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expires_at"`
		CreatedAt time.Time  `json:"created_at"`
	}
	out := make([]keyView, len(keys))
	for i, k := range keys {
		out[i] = keyView{
			KeyID:     k.KeyID,
			Label:     k.Label,
			Scopes:    k.Scopes,
			ExpiresAt: k.ExpiresAt,
			CreatedAt: k.CreatedAt,
		}
	}

	// For unlimited plans, limit and slots_left are null.
	var limitResp, slotsLeft interface{}
	if planLim.MaxAPIKeys != -1 {
		limitResp = planLim.MaxAPIKeys
		slotsLeft = planLim.MaxAPIKeys - len(out)
	}

	c.JSON(http.StatusOK, gin.H{
		"api_keys":   out,
		"count":      len(out),
		"limit":      limitResp,
		"slots_left": slotsLeft,
		"plan":       tenant.Plan,
	})
}

// handleRevokeAPIKey revokes a tenant API key by key_id.
// DELETE /v1/admin/api_keys/:key_id
func (s *Server) handleRevokeAPIKey(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	keyID := c.Param("key_id")
	if err := s.apiKeySvc.RevokeKey(c.Request.Context(), user.TenantID, keyID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"revoked": keyID})
}
