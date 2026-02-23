package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// ─── List Rate Limits ────────────────────────────────────────────────────────

// GET /v1/admin/rate-limits
func (s *Server) handleListRateLimits(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var limits []models.RateLimit
	if err := s.postgresDB.GetDB().
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Find(&limits).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Enrich with current usage from Redis counters
	type rateLimitView struct {
		models.RateLimit
		CurrentUsage int64 `json:"current_usage"`
	}

	views := make([]rateLimitView, len(limits))
	for i, l := range limits {
		var usage int64
		if s.rateLimiter != nil {
			usage = s.rateLimiter.GetCurrentUsage(c.Request.Context(), tenantID, l)
		}
		views[i] = rateLimitView{RateLimit: l, CurrentUsage: usage}
	}

	c.JSON(http.StatusOK, gin.H{"rate_limits": views})
}

// ─── Upsert Rate Limit ──────────────────────────────────────────────────────

type upsertRateLimitReq struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	ScopeType     string `json:"scope_type"`
	ScopeID       string `json:"scope_id"`
	Metric        string `json:"metric"       binding:"required"`
	LimitValue    int64  `json:"limit_value"  binding:"required"`
	WindowSeconds int    `json:"window_seconds"`
	Enabled       *bool  `json:"enabled"`
}

// PUT /v1/admin/rate-limits
func (s *Server) handleUpsertRateLimit(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req upsertRateLimitReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate metric
	switch req.Metric {
	case models.RateLimitMetricRPM, models.RateLimitMetricITPM, models.RateLimitMetricOTPM:
		// ok
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "metric must be rpm, itpm, or otpm"})
		return
	}

	if req.LimitValue <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit_value must be > 0"})
		return
	}

	// Check plan allows rate limits
	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load tenant"})
		return
	}
	planLimits := models.GetPlanLimits(tenant.Plan)
	if !planLimits.AllowRateLimits {
		c.JSON(http.StatusForbidden, gin.H{"error": "your plan does not support rate limits"})
		return
	}

	// Default scope
	scopeType := req.ScopeType
	if scopeType == "" {
		scopeType = models.BudgetScopeAccount
	}
	if scopeType == models.BudgetScopeAPIKey && !planLimits.AllowPerKeyRateLimit {
		c.JSON(http.StatusForbidden, gin.H{"error": "your plan does not support per-key rate limits"})
		return
	}

	windowSeconds := req.WindowSeconds
	if windowSeconds <= 0 {
		windowSeconds = 60
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	limit := models.RateLimit{
		TenantID:      tenantID,
		Provider:      req.Provider,
		Model:         req.Model,
		ScopeType:     scopeType,
		ScopeID:       req.ScopeID,
		Metric:        req.Metric,
		LimitValue:    req.LimitValue,
		WindowSeconds: windowSeconds,
		Enabled:       enabled,
	}

	// Upsert by unique key
	db := s.postgresDB.GetDB()
	var existing models.RateLimit
	err := db.Where(
		"tenant_id = ? AND provider = ? AND model = ? AND scope_type = ? AND scope_id = ? AND metric = ?",
		tenantID, req.Provider, req.Model, scopeType, req.ScopeID, req.Metric,
	).First(&existing).Error

	if err == nil {
		// Update existing
		existing.LimitValue = req.LimitValue
		existing.WindowSeconds = windowSeconds
		existing.Enabled = enabled
		if err := db.Save(&existing).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		limit = existing
	} else {
		// Create new
		if err := db.Create(&limit).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Invalidate Redis config cache
	if s.rateLimiter != nil {
		s.rateLimiter.InvalidateCache(c.Request.Context(), tenantID)
	}

	c.JSON(http.StatusOK, limit)
}

// ─── Delete Rate Limit ──────────────────────────────────────────────────────

// DELETE /v1/admin/rate-limits/:id
func (s *Server) handleDeleteRateLimit(c *gin.Context) {
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
	var limit models.RateLimit
	if err := db.First(&limit, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "rate limit not found"})
		return
	}

	if limit.TenantID != tenantID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if err := db.Delete(&limit).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Invalidate Redis config cache
	if s.rateLimiter != nil {
		s.rateLimiter.InvalidateCache(c.Request.Context(), tenantID)
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
