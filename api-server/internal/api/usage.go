package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/middleware"
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/models"
)

type reportUsageReq struct {
	Provider         string  `json:"provider"           binding:"required"`
	Model            string  `json:"model"              binding:"required"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	Cost             float64 `json:"cost"`
	RequestID        string  `json:"request_id"`
}

// handleReportUsage is called by the claude-code agent (API-key auth).
// POST /v1/agent/usage
func (s *Server) handleReportUsage(c *gin.Context) {
	ak, exists := c.Get(middleware.ContextKeyAPIKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	apiKey, ok := ak.(*models.APIKey)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req reportUsageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	entry := &models.UsageLog{
		TenantID:         apiKey.TenantID,
		Provider:         req.Provider,
		Model:            req.Model,
		PromptTokens:     req.PromptTokens,
		CompletionTokens: req.CompletionTokens,
		Cost:             req.Cost,
		RequestID:        req.RequestID,
	}
	if err := s.usageSvc.Create(c.Request.Context(), entry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"recorded": true})
}

// handleListUsage returns usage logs for the caller's tenant.
// GET /v1/usage
func (s *Server) handleListUsage(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	logs, err := s.usageSvc.ListByTenant(c.Request.Context(), tenantID, 500)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"usage_logs": logs})
}

// handleUsageSummary returns aggregated usage for the caller's tenant.
// GET /v1/usage/summary
func (s *Server) handleUsageSummary(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
