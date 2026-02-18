package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/middleware"
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/models"
	"github.com/xiaoboyu/burnrate-ai/api-server/internal/pricing"
)

type reportUsageReq struct {
	Provider            string `json:"provider"              binding:"required"`
	Model               string `json:"model"                 binding:"required"`
	PromptTokens        int64  `json:"prompt_tokens"`         // legacy alias for input_tokens
	CompletionTokens    int64  `json:"completion_tokens"`     // legacy alias for output_tokens
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens"`
	ReasoningTokens     int64  `json:"reasoning_tokens"`
	RequestID           string `json:"request_id"`
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

	// Normalize legacy token aliases
	inputTokens := req.InputTokens
	if inputTokens == 0 {
		inputTokens = req.PromptTokens
	}
	outputTokens := req.OutputTokens
	if outputTokens == 0 {
		outputTokens = req.CompletionTokens
	}

	ctx := c.Request.Context()

	event := pricing.UsageEvent{
		Provider:            req.Provider,
		Model:               req.Model,
		InputTokens:         inputTokens,
		OutputTokens:        outputTokens,
		CacheCreationTokens: req.CacheCreationTokens,
		CacheReadTokens:     req.CacheReadTokens,
		ReasoningTokens:     req.ReasoningTokens,
		RequestCount:        1,
		TenantID:            apiKey.TenantID,
		IdempotencyKey:      req.RequestID,
		APIKeyRef:           apiKey.KeyID,
	}

	result, err := s.pricingEngine.Process(ctx, event)
	if err != nil {
		var budgetErr *pricing.ErrBudgetExceeded
		if errors.As(err, &budgetErr) {
			c.JSON(http.StatusPaymentRequired, gin.H{
				"error":         "budget_exceeded",
				"period":        budgetErr.Period,
				"limit_amount":  budgetErr.LimitAmount.String(),
				"current_spend": budgetErr.CurrentSpend.String(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Record in usage_logs for backward compatibility
	entry := &models.UsageLog{
		TenantID:            apiKey.TenantID,
		Provider:            req.Provider,
		Model:               req.Model,
		PromptTokens:        inputTokens,
		CompletionTokens:    outputTokens,
		CacheCreationTokens: req.CacheCreationTokens,
		CacheReadTokens:     req.CacheReadTokens,
		ReasoningTokens:     req.ReasoningTokens,
		Cost:                result.FinalCost,
		RequestID:           req.RequestID,
	}
	if err := s.usageSvc.Create(ctx, entry); err != nil {
		// Duplicate request_id → idempotent success
		if isDuplicateRequestID(err) {
			c.JSON(http.StatusOK, gin.H{
				"recorded":   true,
				"idempotent": true,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"recorded":      true,
		"base_cost":     result.BaseCost.String(),
		"markup_amount": result.MarkupAmount.String(),
		"final_cost":    result.FinalCost.String(),
	})
}

// isDuplicateRequestID detects unique constraint errors on request_id.
func isDuplicateRequestID(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return containsSubstr(s, "duplicate key") || containsSubstr(s, "unique constraint") || containsSubstr(s, "23505")
}

func containsSubstr(s, sub string) bool {
	if len(sub) == 0 || len(s) < len(sub) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
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
