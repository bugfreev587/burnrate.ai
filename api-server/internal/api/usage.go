package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/middleware"
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/models"
)

type reportUsageReq struct {
	Provider     string  `json:"provider"      binding:"required"`
	Model        string  `json:"model"         binding:"required"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
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

	log := &models.UsageLog{
		UserID:       apiKey.UserID,
		Provider:     req.Provider,
		Model:        req.Model,
		InputTokens:  req.InputTokens,
		OutputTokens: req.OutputTokens,
		CostUSD:      req.CostUSD,
	}
	if err := s.usageSvc.Create(c.Request.Context(), log); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"recorded": true})
}

// handleListUsage returns usage logs for the authenticated dashboard user.
// GET /v1/usage
func (s *Server) handleListUsage(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	logs, err := s.usageSvc.ListByUser(c.Request.Context(), user.ID, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"usage_logs": logs})
}

// handleUsageSummary returns aggregated usage for the authenticated user.
// GET /v1/usage/summary
func (s *Server) handleUsageSummary(c *gin.Context) {
	// TODO: implement aggregations
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
