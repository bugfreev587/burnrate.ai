package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/authz"
	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

type createAPIKeyReq struct {
	Label          string     `json:"label"           binding:"required"`
	ProjectID      uint       `json:"project_id"      binding:"required"`
	Provider       string     `json:"provider"        binding:"required"`
	AuthMethod     string     `json:"auth_method"     binding:"required"`
	BillingMode    string     `json:"billing_mode"    binding:"required"`
	Scopes         []string   `json:"scopes"`
	ExpiresAt      *time.Time `json:"expires_at"`
	ModelAllowlist []string   `json:"model_allowlist"` // optional list of allowed model names
}

// handleCreateAPIKey creates a new tenant-scoped API key bound to a project.
// POST /v1/admin/api_keys
func (s *Server) handleCreateAPIKey(c *gin.Context) {
	var req createAPIKeyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	tenantID, _ := middleware.GetTenantIDFromContext(c)

	// Validate project exists in this tenant.
	var project models.Project
	if err := s.postgresDB.GetDB().Where("id = ? AND tenant_id = ?", req.ProjectID, tenantID).First(&project).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project not found in this tenant"})
		return
	}

	// Authorize: editors/viewers must be a member of the target project.
	user, _ := middleware.GetUserFromContext(c)
	orgRole := middleware.GetOrgRoleFromContext(c)
	decision := authz.Authorize(s.postgresDB.GetDB(), user.ID, tenantID, orgRole, authz.ActionAPIKeyCreate, authz.Resource{ProjectID: req.ProjectID})
	if !decision.Allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": decision.Reason})
		return
	}

	// Enforce plan-based API key limit before hitting the service.
	var tenant models.Tenant
	s.postgresDB.GetDB().First(&tenant, tenantID)
	planLim := models.GetPlanLimits(tenant.Plan)
	if planLim.MaxAPIKeys != -1 {
		var activeCount int64
		s.postgresDB.GetDB().Model(&models.APIKey{}).
			Where("tenant_id = ? AND revoked = false AND (expires_at IS NULL OR expires_at > NOW())", tenantID).
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

	// BYOK keys require an active provider key for the same provider.
	if req.AuthMethod == models.AuthMethodBYOK {
		if s.providerKeySvc == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "provider key service not configured"})
			return
		}
		settings, err := s.providerKeySvc.GetActiveSettings(c.Request.Context(), tenantID, req.Provider)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check provider key status"})
			return
		}
		if settings == nil || settings.ActiveKeyID == 0 {
			// Check if there's an inactive (non-revoked) provider key
			var inactiveCount int64
			s.postgresDB.GetDB().Model(&models.ProviderKey{}).
				Where("tenant_id = ? AND provider = ? AND revoked = false", tenantID, req.Provider).
				Count(&inactiveCount)

			msg := fmt.Sprintf("Cannot create a BYOK API key for provider %q because no active provider key exists. Please add a %s provider key first.", req.Provider, req.Provider)
			if inactiveCount > 0 {
				msg = fmt.Sprintf("Cannot create a BYOK API key for provider %q because no active provider key exists. You have an inactive %s provider key — please activate it first.", req.Provider, req.Provider)
			}
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "no_active_provider_key",
				"message": msg,
			})
			return
		}
	}

	kid, secret, err := s.apiKeySvc.CreateKey(c.Request.Context(), tenantID, req.Label, req.Scopes, req.ExpiresAt, req.Provider, req.AuthMethod, req.BillingMode, req.ProjectID, user.ID, req.ModelAllowlist)
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

	s.recordAudit(c, "api_key:create", "api_key", kid)

	c.JSON(http.StatusCreated, gin.H{
		"key_id":       kid,
		"secret":       secret, // shown only once
		"label":        req.Label,
		"project_id":   req.ProjectID,
		"provider":     req.Provider,
		"auth_method":  req.AuthMethod,
		"billing_mode": req.BillingMode,
	})
}

// handleListAPIKeys lists active API keys for the caller's tenant,
// filtered by ownership: admins/owners see all keys, editors/viewers
// see only keys they created.
// GET /v1/admin/api_keys
func (s *Server) handleListAPIKeys(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	tenantID, _ := middleware.GetTenantIDFromContext(c)
	orgRole := middleware.GetOrgRoleFromContext(c)

	keys, err := s.apiKeySvc.ListKeysFiltered(c.Request.Context(), tenantID, orgRole, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fetch tenant for plan-aware limit display.
	var tenant models.Tenant
	s.postgresDB.GetDB().First(&tenant, tenantID)
	planLim := models.GetPlanLimits(tenant.Plan)

	type keyView struct {
		KeyID           string     `json:"key_id"`
		Label           string     `json:"label"`
		ProjectID       uint       `json:"project_id"`
		Provider        string     `json:"provider"`
		AuthMethod      string     `json:"auth_method"`
		BillingMode     string     `json:"billing_mode"`
		Scopes          []string   `json:"scopes"`
		ExpiresAt       *time.Time `json:"expires_at"`
		CreatedAt       time.Time  `json:"created_at"`
		LastSeenAt      *time.Time `json:"last_seen_at"`
		CreatedByUserID string     `json:"created_by_user_id"`
	}
	out := make([]keyView, len(keys))
	for i, k := range keys {
		out[i] = keyView{
			KeyID:           k.KeyID,
			Label:           k.Label,
			ProjectID:       k.ProjectID,
			Provider:        k.Provider,
			AuthMethod:      k.AuthMethod,
			BillingMode:     k.BillingMode,
			Scopes:          k.Scopes,
			ExpiresAt:       k.ExpiresAt,
			CreatedAt:       k.CreatedAt,
			LastSeenAt:      k.LastSeenAt,
			CreatedByUserID: k.CreatedByUserID,
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
// Owners/admins can revoke any key; editors can only revoke keys they created.
// DELETE /v1/admin/api_keys/:key_id
func (s *Server) handleRevokeAPIKey(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	tenantID, _ := middleware.GetTenantIDFromContext(c)
	orgRole := middleware.GetOrgRoleFromContext(c)
	keyID := c.Param("key_id")

	// For non-admin roles, verify the caller owns the key before revoking.
	if orgRole != models.RoleOwner && orgRole != models.RoleAdmin {
		var ak models.APIKey
		if err := s.postgresDB.GetDB().Where("key_id = ? AND tenant_id = ?", keyID, tenantID).First(&ak).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
			return
		}
		if ak.CreatedByUserID != user.ID {
			c.JSON(http.StatusForbidden, gin.H{"error": "you can only revoke keys you created"})
			return
		}
	}

	if err := s.apiKeySvc.RevokeKey(c.Request.Context(), tenantID, keyID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	s.recordAudit(c, "api_key:revoke", "api_key", keyID)

	c.JSON(http.StatusOK, gin.H{"revoked": keyID})
}
