package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

// handleCreateProviderKey stores an upstream LLM provider API key.
// POST /v1/admin/provider_keys
func (s *Server) handleCreateProviderKey(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")

	var req struct {
		Provider string `json:"provider" binding:"required"`
		Label    string `json:"label"    binding:"required"`
		APIKey   string `json:"api_key"  binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if strings.HasPrefix(req.APIKey, "tg_") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid api_key: a TokenGate API key cannot be stored as a provider key"})
		return
	}

	if err := services.ValidateProviderKeyFormat(req.Provider, req.APIKey); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Enforce plan-based provider key limit.
	var tenant models.Tenant
	s.postgresDB.GetDB().First(&tenant, tenantID)
	planLim := models.GetPlanLimits(tenant.Plan)
	if planLim.MaxProviderKeys != -1 {
		var activeCount int64
		s.postgresDB.GetDB().Model(&models.ProviderKey{}).
			Where("tenant_id = ? AND revoked = false", tenantID).
			Count(&activeCount)
		if int(activeCount) >= planLim.MaxProviderKeys {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "plan_limit_reached",
				"message": fmt.Sprintf("Your %s plan allows up to %d provider key(s). Upgrade to add more.", tenant.Plan, planLim.MaxProviderKeys),
				"limit":   planLim.MaxProviderKeys,
				"current": activeCount,
				"plan":    tenant.Plan,
			})
			return
		}
	}

	pk, err := s.providerKeySvc.Store(c.Request.Context(), tenantID, req.Provider, req.Label, req.APIKey)
	if err != nil {
		if errors.Is(err, services.ErrInvalidProviderKeyFormat) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store provider key"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         pk.ID,
		"provider":   pk.Provider,
		"label":      pk.Label,
		"created_at": pk.CreatedAt,
	})
}

// handleListProviderKeys lists stored provider keys (masked, no plaintext).
// GET /v1/admin/provider_keys
func (s *Server) handleListProviderKeys(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")

	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	keys, err := s.providerKeySvc.List(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list provider keys"})
		return
	}

	type keyResp struct {
		ID        uint      `json:"id"`
		Provider  string    `json:"provider"`
		Label     string    `json:"label"`
		IsActive  bool      `json:"is_active"`
		CreatedAt time.Time `json:"created_at"`
	}

	// Collect active key IDs per provider
	activeKeyIDs := map[string]uint{}
	providers := map[string]bool{}
	for _, k := range keys {
		providers[k.Provider] = true
	}
	for provider := range providers {
		settings, _ := s.providerKeySvc.GetActiveSettings(c.Request.Context(), tenantID, provider)
		if settings != nil {
			activeKeyIDs[provider] = settings.ActiveKeyID
		}
	}

	resp := make([]keyResp, 0, len(keys))
	for _, k := range keys {
		resp = append(resp, keyResp{
			ID:        k.ID,
			Provider:  k.Provider,
			Label:     k.Label,
			IsActive:  activeKeyIDs[k.Provider] == k.ID,
			CreatedAt: k.CreatedAt,
		})
	}

	// Fetch tenant for plan-aware limit display.
	var tenant models.Tenant
	s.postgresDB.GetDB().First(&tenant, tenantID)
	planLim := models.GetPlanLimits(tenant.Plan)

	var limitResp, slotsLeft interface{}
	if planLim.MaxProviderKeys != -1 {
		limitResp = planLim.MaxProviderKeys
		slotsLeft = planLim.MaxProviderKeys - len(resp)
	}

	c.JSON(http.StatusOK, gin.H{
		"provider_keys": resp,
		"limit":         limitResp,
		"slots_left":    slotsLeft,
		"plan":          tenant.Plan,
	})
}

// handleRevokeProviderKey revokes a provider key.
// DELETE /v1/admin/provider_keys/:key_id
func (s *Server) handleRevokeProviderKey(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")
	keyIDStr := c.Param("key_id")
	keyID64, err := strconv.ParseUint(keyIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key_id"})
		return
	}
	keyID := uint(keyID64)

	if err := s.providerKeySvc.Revoke(c.Request.Context(), tenantID, keyID); err != nil {
		if errors.Is(err, services.ErrProviderKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "provider key not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke provider key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "provider key revoked"})
}

// handleRotateProviderKey atomically stores a new key, activates it, and revokes the old one.
// POST /v1/admin/provider_keys/:key_id/rotate
func (s *Server) handleRotateProviderKey(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")
	keyIDStr := c.Param("key_id")
	keyID64, err := strconv.ParseUint(keyIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key_id"})
		return
	}
	oldKeyID := uint(keyID64)

	var req struct {
		Label  string `json:"label"   binding:"required"`
		APIKey string `json:"api_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newKey, err := s.providerKeySvc.Rotate(c.Request.Context(), tenantID, oldKeyID, req.Label, req.APIKey)
	if err != nil {
		if errors.Is(err, services.ErrProviderKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "provider key not found"})
			return
		}
		if errors.Is(err, services.ErrInvalidProviderKeyFormat) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         newKey.ID,
		"provider":   newKey.Provider,
		"label":      newKey.Label,
		"is_active":  true,
		"created_at": newKey.CreatedAt,
	})
}

// handleActivateProviderKey sets a provider key as the active key for its provider.
// PUT /v1/admin/provider_keys/:key_id/activate
func (s *Server) handleActivateProviderKey(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")
	keyIDStr := c.Param("key_id")
	keyID64, err := strconv.ParseUint(keyIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key_id"})
		return
	}
	keyID := uint(keyID64)

	if err := s.providerKeySvc.Activate(c.Request.Context(), tenantID, keyID); err != nil {
		if errors.Is(err, services.ErrProviderKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "provider key not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to activate provider key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "provider key activated"})
}
