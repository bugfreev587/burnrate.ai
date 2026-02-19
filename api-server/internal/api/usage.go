package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

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
// Results are bounded by the tenant's plan data retention window.
// GET /v1/usage
func (s *Server) handleListUsage(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var tenant models.Tenant
	s.postgresDB.GetDB().First(&tenant, tenantID)
	lim := models.GetPlanLimits(tenant.Plan)

	var since *time.Time
	if lim.DataRetentionDays > 0 {
		t := time.Now().AddDate(0, 0, -lim.DataRetentionDays)
		since = &t
	}

	logs, err := s.usageSvc.ListByTenantSince(c.Request.Context(), tenantID, 500, since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"usage_logs": logs})
}

// handleUsageSummary returns aggregated usage for the caller's tenant.
// GET /v1/usage/summary
func (s *Server) handleUsageSummary(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	db := s.postgresDB.GetDB()
	now := time.Now().UTC()

	// ── Time windows ────────────────────────────────────────────────────────
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastMonthStart := monthStart.AddDate(0, -1, 0)
	trend30Start := todayStart.AddDate(0, 0, -29)

	// ── Helper to query a single period ─────────────────────────────────────
	type periodRow struct {
		TotalCost decimal.Decimal
		Requests  int64
	}
	periodStats := func(from, to *time.Time) periodRow {
		q := db.Model(&models.UsageLog{}).Where("tenant_id = ?", tenantID)
		if from != nil {
			q = q.Where("created_at >= ?", *from)
		}
		if to != nil {
			q = q.Where("created_at < ?", *to)
		}
		var row periodRow
		q.Select("COALESCE(SUM(cost), 0) as total_cost, COUNT(*) as requests").Scan(&row)
		return row
	}

	today := periodStats(&todayStart, nil)
	yesterday := periodStats(&yesterdayStart, &todayStart)
	thisMonth := periodStats(&monthStart, nil)
	lastMonth := periodStats(&lastMonthStart, &monthStart)
	cumulative := periodStats(nil, nil)

	// ── Token totals (cumulative) ────────────────────────────────────────────
	type tokenRow struct {
		InputTotal  int64
		OutputTotal int64
	}
	var tokens tokenRow
	db.Model(&models.UsageLog{}).
		Where("tenant_id = ?", tenantID).
		Select("COALESCE(SUM(prompt_tokens), 0) as input_total, COALESCE(SUM(completion_tokens), 0) as output_total").
		Scan(&tokens)

	totalTokens := tokens.InputTotal + tokens.OutputTotal
	var avgTokensPerRequest int64
	if cumulative.Requests > 0 {
		avgTokensPerRequest = totalTokens / cumulative.Requests
	}

	// ── By-model breakdown (this month) ─────────────────────────────────────
	type modelRow struct {
		Model        string
		Provider     string
		TotalCost    decimal.Decimal
		InputTokens  int64
		OutputTokens int64
		Requests     int64
	}
	var byModel []modelRow
	db.Model(&models.UsageLog{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, monthStart).
		Select("model, provider, COALESCE(SUM(cost),0) as total_cost, COALESCE(SUM(prompt_tokens),0) as input_tokens, COALESCE(SUM(completion_tokens),0) as output_tokens, COUNT(*) as requests").
		Group("model, provider").
		Order("total_cost DESC").
		Scan(&byModel)

	byModelOut := make([]gin.H, len(byModel))
	for i, m := range byModel {
		byModelOut[i] = gin.H{
			"model":         m.Model,
			"provider":      m.Provider,
			"cost":          m.TotalCost.StringFixed(4),
			"input_tokens":  m.InputTokens,
			"output_tokens": m.OutputTokens,
			"requests":      m.Requests,
		}
	}

	// ── Daily trend (last 30 days, UTC date) ────────────────────────────────
	type dailyRow struct {
		Date    string
		Cost    decimal.Decimal
		Tokens  int64
	}
	var daily []dailyRow
	db.Model(&models.UsageLog{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, trend30Start).
		Select("TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') as date, COALESCE(SUM(cost),0) as cost, COALESCE(SUM(prompt_tokens)+SUM(completion_tokens),0) as tokens").
		Group("TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD')").
		Order("date ASC").
		Scan(&daily)

	dailyOut := make([]gin.H, len(daily))
	for i, d := range daily {
		dailyOut[i] = gin.H{
			"date":   d.Date,
			"cost":   d.Cost.StringFixed(4),
			"tokens": d.Tokens,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"cost": gin.H{
			"today":       today.TotalCost.StringFixed(4),
			"yesterday":   yesterday.TotalCost.StringFixed(4),
			"this_month":  thisMonth.TotalCost.StringFixed(4),
			"last_month":  lastMonth.TotalCost.StringFixed(4),
			"cumulative":  cumulative.TotalCost.StringFixed(4),
		},
		"requests": gin.H{
			"today":      today.Requests,
			"yesterday":  yesterday.Requests,
			"this_month": thisMonth.Requests,
			"last_month": lastMonth.Requests,
			"cumulative": cumulative.Requests,
		},
		"tokens": gin.H{
			"input_total":       tokens.InputTotal,
			"output_total":      tokens.OutputTotal,
			"total":             totalTokens,
			"avg_per_request":   avgTokensPerRequest,
		},
		"by_model":    byModelOut,
		"daily_trend": dailyOut,
	})
}
