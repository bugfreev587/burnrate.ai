package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/pricing"
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
		APIUsageBilled:      false, // agent-reported usage is non-billable
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
		APIUsageBilled:      false, // agent-reported usage is non-billable
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

// parseTimezone reads the "tz" query parameter (an IANA timezone name such as
// "America/Los_Angeles") and returns the corresponding *time.Location.
// Falls back to time.UTC when the parameter is missing or invalid.
func parseTimezone(c *gin.Context) *time.Location {
	tz := c.Query("tz")
	if tz == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

// handleListUsage returns usage logs for the caller's tenant.
// Results are bounded by the tenant's plan data retention window, or by
// optional start_date / end_date query params (YYYY-MM-DD) when provided.
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
	loc := parseTimezone(c)

	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	if startDateStr != "" && endDateStr != "" {
		rangeStart, err1 := time.Parse("2006-01-02", startDateStr)
		rangeEnd, err2 := time.Parse("2006-01-02", endDateStr)
		if err1 == nil && err2 == nil {
			effectiveMin := computeEffectiveMinStart(lim)

			appliedStart := time.Date(rangeStart.Year(), rangeStart.Month(), rangeStart.Day(), 0, 0, 0, 0, loc)
			if appliedStart.Before(effectiveMin) {
				appliedStart = effectiveMin
			}

			now := time.Now().In(loc)
			appliedEnd := time.Date(rangeEnd.Year(), rangeEnd.Month(), rangeEnd.Day(), 23, 59, 59, 999999999, loc)
			if appliedEnd.After(now) {
				appliedEnd = now
			}

			logs, err := s.usageSvc.ListByTenantBetween(c.Request.Context(), tenantID, 500, appliedStart, appliedEnd)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"usage_logs": logs})
			return
		}
	}

	// Fallback: existing retention-window behavior.
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
// Accepts optional start_date / end_date query params (YYYY-MM-DD) to scope
// the by_model breakdown and daily_trend sections.  Fixed-window cards
// (today, yesterday, this_month, last_month, cumulative) are unaffected.
// GET /v1/usage/summary
func (s *Server) handleUsageSummary(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	db := s.postgresDB.GetDB()
	loc := parseTimezone(c)
	now := time.Now().In(loc)

	// ── Time windows (in caller's local timezone) ───────────────────────────
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	lastMonthStart := monthStart.AddDate(0, -1, 0)
	trend30Start := todayStart.AddDate(0, 0, -29)

	// ── Optional date range params ───────────────────────────────────────────
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	var rangeQueryStart, rangeQueryEnd *time.Time
	var appliedRangeOut *gin.H

	if startDateStr != "" && endDateStr != "" {
		rangeStart, err1 := time.Parse("2006-01-02", startDateStr)
		rangeEnd, err2 := time.Parse("2006-01-02", endDateStr)
		if err1 == nil && err2 == nil {
			var tenant models.Tenant
			db.First(&tenant, tenantID)
			lim := models.GetPlanLimits(tenant.Plan)
			effectiveMin := computeEffectiveMinStart(lim)

			appliedStart := time.Date(rangeStart.Year(), rangeStart.Month(), rangeStart.Day(), 0, 0, 0, 0, loc)
			if appliedStart.Before(effectiveMin) {
				appliedStart = effectiveMin
			}

			appliedEnd := time.Date(rangeEnd.Year(), rangeEnd.Month(), rangeEnd.Day(), 23, 59, 59, 999999999, loc)
			if appliedEnd.After(now) {
				appliedEnd = now
			}

			rangeQueryStart = &appliedStart
			rangeQueryEnd = &appliedEnd

			ar := gin.H{
				"start": appliedStart.Format("2006-01-02"),
				"end":   rangeEnd.Format("2006-01-02"),
			}
			appliedRangeOut = &ar
		}
	}

	// ── Helper to query a single period ─────────────────────────────────────
	type periodRow struct {
		TotalCost decimal.Decimal
		Requests  int64
	}
	periodStats := func(from, to *time.Time) periodRow {
		q := db.Model(&models.UsageLog{}).Where("tenant_id = ? AND api_usage_billed = ?", tenantID, true)
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

	// ── Token totals (scoped to date range when provided, cumulative otherwise) ─
	type tokenRow struct {
		InputTotal  int64
		OutputTotal int64
		Requests    int64
	}
	var tokens tokenRow
	tokenQ := db.Model(&models.UsageLog{}).Where("tenant_id = ? AND api_usage_billed = ?", tenantID, true)
	if rangeQueryStart != nil {
		tokenQ = tokenQ.Where("created_at >= ? AND created_at <= ?", *rangeQueryStart, *rangeQueryEnd)
	}
	tokenQ.Select("COALESCE(SUM(prompt_tokens), 0) as input_total, COALESCE(SUM(completion_tokens), 0) as output_total, COUNT(*) as requests").
		Scan(&tokens)

	totalTokens := tokens.InputTotal + tokens.OutputTotal
	var avgTokensPerRequest int64
	if tokens.Requests > 0 {
		avgTokensPerRequest = totalTokens / tokens.Requests
	}

	// ── By-model breakdown ───────────────────────────────────────────────────
	type modelRow struct {
		Model        string
		Provider     string
		TotalCost    decimal.Decimal
		InputTokens  int64
		OutputTokens int64
		Requests     int64
	}
	var byModel []modelRow
	byModelQ := db.Model(&models.UsageLog{}).Where("tenant_id = ? AND api_usage_billed = ?", tenantID, true)
	if rangeQueryStart != nil {
		byModelQ = byModelQ.Where("created_at >= ? AND created_at <= ?", *rangeQueryStart, *rangeQueryEnd)
	} else {
		byModelQ = byModelQ.Where("created_at >= ?", monthStart)
	}
	byModelQ.
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

	// ── By-API-key breakdown ────────────────────────────────────────────────
	type apiKeyRow struct {
		KeyID        string
		Label        string
		TotalCost    decimal.Decimal
		InputTokens  int64
		OutputTokens int64
		Requests     int64
	}
	var byAPIKey []apiKeyRow
	byKeyQ := db.Model(&models.UsageLog{}).
		Select("usage_logs.key_id, COALESCE(api_keys.label, '') as label, COALESCE(SUM(usage_logs.cost),0) as total_cost, COALESCE(SUM(usage_logs.prompt_tokens),0) as input_tokens, COALESCE(SUM(usage_logs.completion_tokens),0) as output_tokens, COUNT(*) as requests").
		Joins("LEFT JOIN api_keys ON api_keys.key_id = usage_logs.key_id").
		Where("usage_logs.tenant_id = ? AND usage_logs.api_usage_billed = ?", tenantID, true)
	if rangeQueryStart != nil {
		byKeyQ = byKeyQ.Where("usage_logs.created_at >= ? AND usage_logs.created_at <= ?", *rangeQueryStart, *rangeQueryEnd)
	} else {
		byKeyQ = byKeyQ.Where("usage_logs.created_at >= ?", monthStart)
	}
	byKeyQ.
		Group("usage_logs.key_id, api_keys.label").
		Order("total_cost DESC").
		Scan(&byAPIKey)

	byAPIKeyOut := make([]gin.H, len(byAPIKey))
	for i, k := range byAPIKey {
		displayLabel := k.Label
		if displayLabel == "" && k.KeyID != "" {
			// Show first 12 chars of key_id as fallback
			displayLabel = k.KeyID
			if len(displayLabel) > 12 {
				displayLabel = displayLabel[:12] + "…"
			}
		}
		byAPIKeyOut[i] = gin.H{
			"key_id":       k.KeyID,
			"label":        displayLabel,
			"cost":         k.TotalCost.StringFixed(4),
			"input_tokens":  k.InputTokens,
			"output_tokens": k.OutputTokens,
			"requests":      k.Requests,
		}
	}

	// ── Daily trend ──────────────────────────────────────────────────────────
	type dailyRow struct {
		Date   string
		Cost   decimal.Decimal
		Tokens int64
	}
	var daily []dailyRow

	var trendStart time.Time
	var trendEnd time.Time
	if rangeQueryStart != nil {
		trendStart = *rangeQueryStart
		trendEnd = *rangeQueryEnd
	} else {
		trendStart = trend30Start
		trendEnd = now
	}

	dailyQ := db.Model(&models.UsageLog{}).Where("tenant_id = ? AND api_usage_billed = ?", tenantID, true)
	dailyQ = dailyQ.Where("created_at >= ? AND created_at <= ?", trendStart, trendEnd)
	tzName := loc.String()
	dailyQ.
		Select(fmt.Sprintf("TO_CHAR(created_at AT TIME ZONE '%s', 'YYYY-MM-DD') as date, COALESCE(SUM(cost),0) as cost, COALESCE(SUM(prompt_tokens)+SUM(completion_tokens),0) as tokens", tzName)).
		Group(fmt.Sprintf("TO_CHAR(created_at AT TIME ZONE '%s', 'YYYY-MM-DD')", tzName)).
		Order("date ASC").
		Scan(&daily)

	// Build a map of existing daily data, then fill in missing dates with zeros.
	dailyMap := make(map[string]dailyRow, len(daily))
	for _, d := range daily {
		dailyMap[d.Date] = d
	}

	var dailyOut []gin.H
	for cursor := time.Date(trendStart.Year(), trendStart.Month(), trendStart.Day(), 0, 0, 0, 0, loc); !cursor.After(time.Date(trendEnd.Year(), trendEnd.Month(), trendEnd.Day(), 0, 0, 0, 0, loc)); cursor = cursor.AddDate(0, 0, 1) {
		dateStr := cursor.Format("2006-01-02")
		if row, ok := dailyMap[dateStr]; ok {
			dailyOut = append(dailyOut, gin.H{
				"date":   row.Date,
				"cost":   row.Cost.StringFixed(4),
				"tokens": row.Tokens,
			})
		} else {
			dailyOut = append(dailyOut, gin.H{
				"date":   dateStr,
				"cost":   "0.0000",
				"tokens": int64(0),
			})
		}
	}
	if dailyOut == nil {
		dailyOut = []gin.H{}
	}

	resp := gin.H{
		"cost": gin.H{
			"today":      today.TotalCost.StringFixed(4),
			"yesterday":  yesterday.TotalCost.StringFixed(4),
			"this_month": thisMonth.TotalCost.StringFixed(4),
			"last_month": lastMonth.TotalCost.StringFixed(4),
			"cumulative": cumulative.TotalCost.StringFixed(4),
		},
		"requests": gin.H{
			"today":      today.Requests,
			"yesterday":  yesterday.Requests,
			"this_month": thisMonth.Requests,
			"last_month": lastMonth.Requests,
			"cumulative": cumulative.Requests,
		},
		"tokens": gin.H{
			"input_total":     tokens.InputTotal,
			"output_total":    tokens.OutputTotal,
			"total":           totalTokens,
			"avg_per_request": avgTokensPerRequest,
		},
		"by_model":    byModelOut,
		"by_api_key":  byAPIKeyOut,
		"daily_trend": dailyOut,
	}

	if appliedRangeOut != nil {
		resp["applied_range"] = *appliedRangeOut
	}

	c.JSON(http.StatusOK, resp)
}
