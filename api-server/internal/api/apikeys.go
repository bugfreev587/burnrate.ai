package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/middleware"
)

type createAPIKeyReq struct {
	Label     string     `json:"label"      binding:"required"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// handleCreateAPIKey creates a new API key for the authenticated user.
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

	kid, secret, err := s.apiKeySvc.CreateKey(c.Request.Context(), user.ID, req.Label, req.Scopes, req.ExpiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"key_id": kid,
		"secret": secret, // shown only once
		"label":  req.Label,
	})
}

// handleListAPIKeys lists all active API keys for the authenticated user.
// GET /v1/admin/api_keys
func (s *Server) handleListAPIKeys(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	keys, err := s.apiKeySvc.ListKeys(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Strip sensitive fields
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
	c.JSON(http.StatusOK, gin.H{"api_keys": out})
}

// handleRevokeAPIKey revokes an API key by key_id.
// DELETE /v1/admin/api_keys/:key_id
func (s *Server) handleRevokeAPIKey(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	keyID := c.Param("key_id")
	if err := s.apiKeySvc.RevokeKey(c.Request.Context(), user.ID, keyID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"revoked": keyID})
}
