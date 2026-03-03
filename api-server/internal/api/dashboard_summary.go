package api

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// ─── handleDashboardSummary ────────────────────────────────────────────────────
// GET /v1/dashboard/summary?from=YYYY-MM-DD&to=YYYY-MM-DD&billing_mode=all|api_usage_billed|monthly_subscription&project_id=N&api_key_id=UUID&tz=...
func (s *Server) handleDashboardSummary(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	db := s.postgresDB.GetDB()
	loc := parseTimezone(c)
	now := time.Now().In(loc)

	// ── Load tenant + plan limits ──────────────────────────────────────────
	var tenant models.Tenant
	db.First(&tenant, tenantID)
	lim := models.GetPlanLimits(tenant.Plan)
	plan := tenant.Plan
	if plan == "" {
		plan = models.PlanFree
	}

	// ── Parse query params ─────────────────────────────────────────────────
	fromStr := c.Query("from")
	toStr := c.Query("to")
	billingMode := c.DefaultQuery("billing_mode", "all")
	projectIDStr := c.Query("project_id")
	apiKeyID := c.Query("api_key_id")

	// Default range: last 7 days
	rangeStart := now.AddDate(0, 0, -7)
	rangeEnd := now

	if fromStr != "" && toStr != "" {
		if rs, err := time.Parse("2006-01-02", fromStr); err == nil {
			rangeStart = time.Date(rs.Year(), rs.Month(), rs.Day(), 0, 0, 0, 0, loc)
		}
		if re, err := time.Parse("2006-01-02", toStr); err == nil {
			rangeEnd = time.Date(re.Year(), re.Month(), re.Day(), 23, 59, 59, 999999999, loc)
		}
	}

	// Clamp to plan retention + availability
	effectiveMin := computeEffectiveMinStart(lim)
	if rangeStart.Before(effectiveMin) {
		rangeStart = effectiveMin
	}
	if rangeEnd.After(now) {
		rangeEnd = now
	}

	// Previous period: same duration immediately before current range
	duration := rangeEnd.Sub(rangeStart)
	prevEnd := rangeStart.Add(-time.Second)
	prevStart := prevEnd.Add(-duration)
	if prevStart.Before(effectiveMin) {
		prevStart = effectiveMin
	}

	// Optional project filter
	var projectID uint
	if projectIDStr != "" {
		if pid, err := strconv.ParseUint(projectIDStr, 10, 64); err == nil {
			projectID = uint(pid)
		}
	}

	// ── Shared WHERE builder ───────────────────────────────────────────────
	type filterCfg struct {
		billingMode string
		projectID   uint
		apiKeyID    string
	}
	filters := filterCfg{billingMode: billingMode, projectID: projectID, apiKeyID: apiKeyID}

	// Build base WHERE clause for usage_logs.
	// Pass a non-empty tableAlias (e.g. "ul") when the query joins other
	// tables that share column names (tenant_id, created_at, etc.).
	buildUsageWhere := func(f filterCfg, from, to time.Time, tableAlias ...string) (string, []interface{}) {
		pfx := ""
		if len(tableAlias) > 0 && tableAlias[0] != "" {
			pfx = tableAlias[0] + "."
		}
		where := fmt.Sprintf("%stenant_id = ? AND %screated_at >= ? AND %screated_at <= ?", pfx, pfx, pfx)
		args := []interface{}{tenantID, from, to}
		if f.billingMode == "api_usage_billed" {
			where += fmt.Sprintf(" AND %sapi_usage_billed = ?", pfx)
			args = append(args, true)
		} else if f.billingMode == "monthly_subscription" {
			where += fmt.Sprintf(" AND %sapi_usage_billed = ?", pfx)
			args = append(args, false)
		}
		if f.apiKeyID != "" {
			where += fmt.Sprintf(" AND %skey_id = ?", pfx)
			args = append(args, f.apiKeyID)
		}
		if f.projectID > 0 {
			where += fmt.Sprintf(" AND %sproject_id = ?", pfx)
			args = append(args, f.projectID)
		}
		return where, args
	}

	buildGatewayWhere := func(f filterCfg, from, to time.Time) (string, []interface{}) {
		where := "tenant_id = ? AND created_at >= ? AND created_at <= ?"
		args := []interface{}{tenantID, from, to}
		if f.apiKeyID != "" {
			where += " AND key_id = ?"
			args = append(args, f.apiKeyID)
		}
		return where, args
	}

	// ── Run all queries in parallel ────────────────────────────────────────
	var wg sync.WaitGroup

	// KPI: current period totals
	// Cost is ALWAYS aggregated from api_usage_billed rows only, regardless
	// of the billing_mode filter.  Token and request counts respect the filter.
	type kpiRow struct {
		SpendTotal     decimal.Decimal `gorm:"column:spend_total"`
		BilledRequests int64           `gorm:"column:billed_requests"`
		RequestsTotal  int64           `gorm:"column:requests_total"`
		InputTokens    int64           `gorm:"column:input_tokens"`
		OutputTokens   int64           `gorm:"column:output_tokens"`
	}
	var curKPI, prevKPI kpiRow

	wg.Add(1)
	go func() {
		defer wg.Done()
		w, a := buildUsageWhere(filters, rangeStart, rangeEnd)
		db.Raw(fmt.Sprintf(`
			SELECT COALESCE(SUM(CASE WHEN api_usage_billed THEN cost ELSE 0 END), 0) AS spend_total,
			       COUNT(CASE WHEN api_usage_billed THEN 1 END) AS billed_requests,
			       COUNT(*) AS requests_total,
			       COALESCE(SUM(prompt_tokens), 0) AS input_tokens,
			       COALESCE(SUM(completion_tokens), 0) AS output_tokens
			FROM usage_logs WHERE %s`, w), a...).Scan(&curKPI)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		w, a := buildUsageWhere(filters, prevStart, prevEnd)
		db.Raw(fmt.Sprintf(`
			SELECT COALESCE(SUM(CASE WHEN api_usage_billed THEN cost ELSE 0 END), 0) AS spend_total,
			       COUNT(CASE WHEN api_usage_billed THEN 1 END) AS billed_requests,
			       COUNT(*) AS requests_total,
			       COALESCE(SUM(prompt_tokens), 0) AS input_tokens,
			       COALESCE(SUM(completion_tokens), 0) AS output_tokens
			FROM usage_logs WHERE %s`, w), a...).Scan(&prevKPI)
	}()

	// KPI: blocked requests (current period)
	type blockedTotals struct {
		RateLimit int64
		Budget    int64
	}
	var curBlocked blockedTotals

	wg.Add(1)
	go func() {
		defer wg.Done()
		type blockedRow struct {
			EventType string `gorm:"column:event_type"`
			Count     int64  `gorm:"column:cnt"`
		}
		var rows []blockedRow
		w, a := buildGatewayWhere(filters, rangeStart, rangeEnd)
		db.Raw(fmt.Sprintf(`
			SELECT event_type, COUNT(*) AS cnt
			FROM gateway_events WHERE %s
			GROUP BY event_type`, w), a...).Scan(&rows)
		for _, r := range rows {
			switch r.EventType {
			case "rate_limit_429":
				curBlocked.RateLimit = r.Count
			case "budget_exceeded_402":
				curBlocked.Budget = r.Count
			}
		}
	}()

	// Forecast (current month)
	type forecastResult struct {
		TotalSoFar    decimal.Decimal
		DailyAverage  decimal.Decimal
		Forecast      decimal.Decimal
		DaysElapsed   int
		DaysRemaining int
	}
	var fc forecastResult

	wg.Add(1)
	go func() {
		defer wg.Done()
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		monthEnd := monthStart.AddDate(0, 1, 0)
		daysInMonth := int(monthEnd.Sub(monthStart).Hours() / 24)
		fc.DaysRemaining = daysInMonth - now.Day()

		// Forecast is always based on api_usage_billed cost only.
		// When billing_mode = monthly_subscription the forecast is $0.
		if billingMode == "monthly_subscription" {
			fc.DaysElapsed = 1
			return
		}

		// Build forecast filter: always api_usage_billed + project/key filters
		fcFilter := filterCfg{billingMode: "api_usage_billed", projectID: projectID, apiKeyID: apiKeyID}
		w, a := buildUsageWhere(fcFilter, monthStart, now)
		db.Raw(fmt.Sprintf(`SELECT COALESCE(SUM(cost), 0) AS spend_total FROM usage_logs WHERE %s`, w), a...).Scan(&fc.TotalSoFar)

		var daysWithData int64
		tzName := loc.String()
		db.Raw(fmt.Sprintf(`
			SELECT COUNT(DISTINCT DATE(created_at AT TIME ZONE '%s'))
			FROM usage_logs WHERE %s`, tzName, w), a...).Scan(&daysWithData)

		fc.DaysElapsed = int(daysWithData)
		if fc.DaysElapsed < 1 {
			fc.DaysElapsed = 1
		}
		fc.DailyAverage = fc.TotalSoFar.Div(decimal.NewFromInt(int64(fc.DaysElapsed)))
		fc.Forecast = fc.TotalSoFar.Add(fc.DailyAverage.Mul(decimal.NewFromInt(int64(fc.DaysRemaining))))
	}()

	// Budget health
	var budgetLimits []models.BudgetLimit
	type budgetEntry struct {
		ID           uint   `json:"id"`
		ScopeType    string `json:"scope_type"`
		ScopeID      string `json:"scope_id"`
		KeyLabel     string `json:"key_label,omitempty"`
		PeriodType   string `json:"period_type"`
		Provider     string `json:"provider"`
		LimitAmount  string `json:"limit_amount"`
		Threshold    string `json:"alert_threshold"`
		Action       string `json:"action"`
		CurrentSpend string `json:"current_spend"`
		PctUsed      float64 `json:"pct_used"`
		Status       string `json:"status"`
	}
	var spendLimitEntries []budgetEntry

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Where("tenant_id = ? AND enabled = ?", tenantID, true).Find(&budgetLimits)

		for _, bl := range budgetLimits {
			var periodStart time.Time
			switch bl.PeriodType {
			case "monthly":
				periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
			case "weekly":
				weekday := int(now.Weekday())
				if weekday == 0 {
					weekday = 7
				}
				periodStart = time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, loc)
			case "daily":
				periodStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
			default:
				periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
			}

			spendQ := db.Model(&models.UsageLog{}).Where("tenant_id = ? AND api_usage_billed = ? AND created_at >= ?", tenantID, true, periodStart)
			if bl.ScopeType == "api_key" && bl.ScopeID != "" {
				spendQ = spendQ.Where("key_id = ?", bl.ScopeID)
			}
			if bl.Provider != "" {
				spendQ = spendQ.Where("provider = ?", bl.Provider)
			}

			var currentSpend decimal.Decimal
			spendQ.Select("COALESCE(SUM(cost), 0)").Scan(&currentSpend)

			var pctUsed float64
			if !bl.LimitAmount.IsZero() {
				pctUsed = currentSpend.Div(bl.LimitAmount).Mul(decimal.NewFromInt(100)).InexactFloat64()
			}

			status := "ok"
			thresholdPct := bl.AlertThreshold.InexactFloat64()
			if pctUsed >= 100 {
				status = "blocking"
			} else if pctUsed >= thresholdPct {
				status = "warning"
			}

			var keyLabel string
			if bl.ScopeType == "api_key" && bl.ScopeID != "" {
				var ak models.APIKey
				if err := db.Where("key_id = ?", bl.ScopeID).First(&ak).Error; err == nil {
					keyLabel = ak.Label
				}
			}

			spendLimitEntries = append(spendLimitEntries, budgetEntry{
				ID:           bl.ID,
				ScopeType:    bl.ScopeType,
				ScopeID:      bl.ScopeID,
				KeyLabel:     keyLabel,
				PeriodType:   bl.PeriodType,
				Provider:     bl.Provider,
				LimitAmount:  bl.LimitAmount.StringFixed(2),
				Threshold:    bl.AlertThreshold.StringFixed(0),
				Action:       bl.Action,
				CurrentSpend: currentSpend.StringFixed(4),
				PctUsed:      math.Round(pctUsed*10) / 10,
				Status:       status,
			})
		}
	}()

	// Rate limits
	var rateLimits []models.RateLimit

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Where("tenant_id = ? AND enabled = ?", tenantID, true).Find(&rateLimits)
	}()

	// Latency percentiles
	type latencyRow struct {
		P50         float64 `gorm:"column:p50"`
		P95         float64 `gorm:"column:p95"`
		P99         float64 `gorm:"column:p99"`
		Avg         float64 `gorm:"column:avg"`
		SampleCount int64   `gorm:"column:sample_count"`
	}
	var latency latencyRow

	wg.Add(1)
	go func() {
		defer wg.Done()
		w, a := buildUsageWhere(filters, rangeStart, rangeEnd)
		db.Raw(fmt.Sprintf(`
			SELECT
				COALESCE(PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY latency_ms), 0) AS p50,
				COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms), 0) AS p95,
				COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY latency_ms), 0) AS p99,
				COALESCE(AVG(latency_ms), 0) AS avg,
				COUNT(*) AS sample_count
			FROM usage_logs WHERE %s AND latency_ms > 0`, w), a...).Scan(&latency)
	}()

	// Timeseries: daily cost
	type dailyCostRow struct {
		Ts       string          `gorm:"column:ts" json:"ts"`
		Cost     decimal.Decimal `gorm:"column:cost" json:"-"`
		CostStr  string          `gorm:"-" json:"cost"`
		Requests int64           `gorm:"column:requests" json:"requests"`
		Tokens   int64           `gorm:"column:tokens" json:"tokens"`
	}
	var dailyCost []dailyCostRow

	wg.Add(1)
	go func() {
		defer wg.Done()
		w, a := buildUsageWhere(filters, rangeStart, rangeEnd)
		tzName := loc.String()
		db.Raw(fmt.Sprintf(`
			SELECT
				TO_CHAR(created_at AT TIME ZONE '%s', 'YYYY-MM-DD') AS ts,
				COALESCE(SUM(CASE WHEN api_usage_billed THEN cost ELSE 0 END), 0) AS cost,
				COUNT(*) AS requests,
				COALESCE(SUM(prompt_tokens + completion_tokens), 0) AS tokens
			FROM usage_logs WHERE %s
			GROUP BY ts ORDER BY ts`, tzName, w), a...).Scan(&dailyCost)
	}()

	// Timeseries: daily latency p95
	type dailyLatRow struct {
		Ts string  `gorm:"column:ts" json:"ts"`
		Ms float64 `gorm:"column:ms" json:"ms"`
	}
	var dailyLatency []dailyLatRow

	wg.Add(1)
	go func() {
		defer wg.Done()
		w, a := buildUsageWhere(filters, rangeStart, rangeEnd)
		tzName := loc.String()
		db.Raw(fmt.Sprintf(`
			SELECT
				TO_CHAR(created_at AT TIME ZONE '%s', 'YYYY-MM-DD') AS ts,
				COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE latency_ms > 0), 0) AS ms
			FROM usage_logs WHERE %s
			GROUP BY ts ORDER BY ts`, tzName, w), a...).Scan(&dailyLatency)
	}()

	// Timeseries: outcomes (success/blocked/error per day)
	type dailyOutcomeRow struct {
		Ts      string `gorm:"column:ts" json:"ts"`
		Success int64  `gorm:"column:success" json:"success"`
		Blocked int64  `gorm:"column:blocked" json:"blocked"`
		Error   int64  `gorm:"column:error" json:"error"`
	}
	var dailyOutcomes []dailyOutcomeRow

	wg.Add(1)
	go func() {
		defer wg.Done()
		tzName := loc.String()

		// Success counts from usage_logs
		type usagePerDay struct {
			Ts      string `gorm:"column:ts"`
			Success int64  `gorm:"column:success"`
		}
		var successRows []usagePerDay
		w, a := buildUsageWhere(filters, rangeStart, rangeEnd)
		db.Raw(fmt.Sprintf(`
			SELECT TO_CHAR(created_at AT TIME ZONE '%s', 'YYYY-MM-DD') AS ts,
			       COUNT(*) AS success
			FROM usage_logs WHERE %s
			GROUP BY ts`, tzName, w), a...).Scan(&successRows)

		// Blocked counts from gateway_events
		type blockedPerDay struct {
			Ts      string `gorm:"column:ts"`
			Blocked int64  `gorm:"column:blocked"`
		}
		var blockedRows []blockedPerDay
		gw, ga := buildGatewayWhere(filters, rangeStart, rangeEnd)
		db.Raw(fmt.Sprintf(`
			SELECT TO_CHAR(created_at AT TIME ZONE '%s', 'YYYY-MM-DD') AS ts,
			       COUNT(*) AS blocked
			FROM gateway_events WHERE %s
			GROUP BY ts`, tzName, gw), ga...).Scan(&blockedRows)

		// Merge
		dateMap := make(map[string]*dailyOutcomeRow)
		for _, s := range successRows {
			dateMap[s.Ts] = &dailyOutcomeRow{Ts: s.Ts, Success: s.Success}
		}
		for _, b := range blockedRows {
			if r, exists := dateMap[b.Ts]; exists {
				r.Blocked = b.Blocked
			} else {
				dateMap[b.Ts] = &dailyOutcomeRow{Ts: b.Ts, Blocked: b.Blocked}
			}
		}
		for _, v := range dateMap {
			dailyOutcomes = append(dailyOutcomes, *v)
		}
	}()

	// Breakdowns: by_provider
	type breakdownRow struct {
		Name         string          `gorm:"column:name" json:"name"`
		Cost         decimal.Decimal `gorm:"column:cost" json:"-"`
		CostStr      string          `gorm:"-" json:"cost"`
		InputTokens  int64           `gorm:"column:input_tokens" json:"input_tokens"`
		OutputTokens int64           `gorm:"column:output_tokens" json:"output_tokens"`
		Requests     int64           `gorm:"column:requests" json:"requests"`
		PctOfTotal   float64         `gorm:"-" json:"pct_of_total"`
	}
	var byProvider []breakdownRow

	wg.Add(1)
	go func() {
		defer wg.Done()
		w, a := buildUsageWhere(filters, rangeStart, rangeEnd)
		db.Raw(fmt.Sprintf(`
			SELECT provider AS name,
			       COALESCE(SUM(CASE WHEN api_usage_billed THEN cost ELSE 0 END), 0) AS cost,
			       COALESCE(SUM(prompt_tokens), 0) AS input_tokens,
			       COALESCE(SUM(completion_tokens), 0) AS output_tokens,
			       COUNT(*) AS requests
			FROM usage_logs WHERE %s
			GROUP BY provider ORDER BY cost DESC`, w), a...).Scan(&byProvider)
	}()

	// Breakdowns: by_model
	type modelBreakdownRow struct {
		Name         string          `gorm:"column:name" json:"name"`
		Provider     string          `gorm:"column:provider" json:"provider"`
		Cost         decimal.Decimal `gorm:"column:cost" json:"-"`
		CostStr      string          `gorm:"-" json:"cost"`
		InputTokens  int64           `gorm:"column:input_tokens" json:"input_tokens"`
		OutputTokens int64           `gorm:"column:output_tokens" json:"output_tokens"`
		Requests     int64           `gorm:"column:requests" json:"requests"`
		PctOfTotal   float64         `gorm:"-" json:"pct_of_total"`
	}
	var byModel []modelBreakdownRow

	wg.Add(1)
	go func() {
		defer wg.Done()
		w, a := buildUsageWhere(filters, rangeStart, rangeEnd)
		db.Raw(fmt.Sprintf(`
			SELECT model AS name, provider,
			       COALESCE(SUM(CASE WHEN api_usage_billed THEN cost ELSE 0 END), 0) AS cost,
			       COALESCE(SUM(prompt_tokens), 0) AS input_tokens,
			       COALESCE(SUM(completion_tokens), 0) AS output_tokens,
			       COUNT(*) AS requests
			FROM usage_logs WHERE %s
			GROUP BY model, provider ORDER BY cost DESC`, w), a...).Scan(&byModel)
	}()

	// Breakdowns: by_api_key
	type apiKeyBreakdownRow struct {
		KeyID        string          `gorm:"column:key_id" json:"key_id"`
		Label        string          `gorm:"column:label" json:"name"`
		Cost         decimal.Decimal `gorm:"column:cost" json:"-"`
		CostStr      string          `gorm:"-" json:"cost"`
		InputTokens  int64           `gorm:"column:input_tokens" json:"input_tokens"`
		OutputTokens int64           `gorm:"column:output_tokens" json:"output_tokens"`
		Requests     int64           `gorm:"column:requests" json:"requests"`
		PctOfTotal   float64         `gorm:"-" json:"pct_of_total"`
	}
	var byAPIKey []apiKeyBreakdownRow

	wg.Add(1)
	go func() {
		defer wg.Done()
		w, a := buildUsageWhere(filters, rangeStart, rangeEnd, "ul")
		db.Raw(fmt.Sprintf(`
			SELECT ul.key_id,
			       COALESCE(ak.label, SUBSTRING(ul.key_id, 1, 12)) AS label,
			       COALESCE(SUM(CASE WHEN ul.api_usage_billed THEN ul.cost ELSE 0 END), 0) AS cost,
			       COALESCE(SUM(ul.prompt_tokens), 0) AS input_tokens,
			       COALESCE(SUM(ul.completion_tokens), 0) AS output_tokens,
			       COUNT(*) AS requests
			FROM usage_logs ul
			LEFT JOIN api_keys ak ON ak.key_id = ul.key_id
			WHERE %s
			GROUP BY ul.key_id, ak.label
			ORDER BY cost DESC`, w), a...).Scan(&byAPIKey)
	}()

	// Breakdowns: by_project
	type projectBreakdownRow struct {
		ProjectID    uint            `gorm:"column:project_id" json:"project_id"`
		Name         string          `gorm:"column:name" json:"name"`
		Cost         decimal.Decimal `gorm:"column:cost" json:"-"`
		CostStr      string          `gorm:"-" json:"cost"`
		InputTokens  int64           `gorm:"column:input_tokens" json:"input_tokens"`
		OutputTokens int64           `gorm:"column:output_tokens" json:"output_tokens"`
		Requests     int64           `gorm:"column:requests" json:"requests"`
		PctOfTotal   float64         `gorm:"-" json:"pct_of_total"`
	}
	var byProject []projectBreakdownRow

	wg.Add(1)
	go func() {
		defer wg.Done()
		w, a := buildUsageWhere(filters, rangeStart, rangeEnd, "ul")
		db.Raw(fmt.Sprintf(`
			SELECT ul.project_id,
			       COALESCE(p.name, 'Unassigned') AS name,
			       COALESCE(SUM(CASE WHEN ul.api_usage_billed THEN ul.cost ELSE 0 END), 0) AS cost,
			       COALESCE(SUM(ul.prompt_tokens), 0) AS input_tokens,
			       COALESCE(SUM(ul.completion_tokens), 0) AS output_tokens,
			       COUNT(*) AS requests
			FROM usage_logs ul
			LEFT JOIN projects p ON p.id = ul.project_id
			WHERE %s
			GROUP BY ul.project_id, p.name
			ORDER BY cost DESC`, w), a...).Scan(&byProject)
	}()

	// Governance
	type govData struct {
		ActiveAPIKeys    int64 `json:"active_api_keys"`
		ActiveProjects   int64 `json:"active_projects"`
		AuditEvents7d    int64 `json:"audit_events_7d"`
		RevokedKeysPeriod int64 `json:"revoked_keys_period"`
	}
	var gov govData

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Raw(`SELECT COUNT(*) FROM api_keys WHERE tenant_id = ? AND revoked = ?`, tenantID, false).Scan(&gov.ActiveAPIKeys)
		db.Raw(`SELECT COUNT(*) FROM projects WHERE tenant_id = ? AND status = ?`, tenantID, "active").Scan(&gov.ActiveProjects)
		week := now.AddDate(0, 0, -7)
		db.Raw(`SELECT COUNT(*) FROM audit_logs WHERE tenant_id = ? AND created_at >= ?`, tenantID, week).Scan(&gov.AuditEvents7d)
		db.Raw(`SELECT COUNT(*) FROM api_keys WHERE tenant_id = ? AND revoked = ? AND updated_at >= ? AND updated_at <= ?`, tenantID, true, rangeStart, rangeEnd).Scan(&gov.RevokedKeysPeriod)
	}()

	// Wait for all parallel queries
	wg.Wait()

	// ── Post-processing ────────────────────────────────────────────────────

	// Delta calculation helper
	deltaPct := func(cur, prev decimal.Decimal) float64 {
		if prev.IsZero() {
			if cur.IsZero() {
				return 0
			}
			return 100
		}
		return cur.Sub(prev).Div(prev).Mul(decimal.NewFromInt(100)).InexactFloat64()
	}
	deltaIntPct := func(cur, prev int64) float64 {
		if prev == 0 {
			if cur == 0 {
				return 0
			}
			return 100
		}
		return float64(cur-prev) / float64(prev) * 100
	}

	spendDelta := deltaPct(curKPI.SpendTotal, prevKPI.SpendTotal)
	requestsDelta := deltaIntPct(curKPI.RequestsTotal, prevKPI.RequestsTotal)

	// Avg cost per request uses only api_usage_billed rows for both
	// numerator (cost) and denominator (request count).
	var curAvgCost, prevAvgCost decimal.Decimal
	if curKPI.BilledRequests > 0 {
		curAvgCost = curKPI.SpendTotal.Div(decimal.NewFromInt(curKPI.BilledRequests))
	}
	if prevKPI.BilledRequests > 0 {
		prevAvgCost = prevKPI.SpendTotal.Div(decimal.NewFromInt(prevKPI.BilledRequests))
	}
	avgCostDelta := deltaPct(curAvgCost, prevAvgCost)

	// Success rate
	totalAll := curKPI.RequestsTotal + curBlocked.RateLimit + curBlocked.Budget
	var successRate float64
	if totalAll > 0 {
		successRate = float64(curKPI.RequestsTotal) / float64(totalAll) * 100
	}

	// Budget health (worst case across all limits)
	budgetHealthStatus := "ok"
	budgetHealthMsg := "All spend limits within thresholds"
	var worstPctUsed float64
	for _, entry := range spendLimitEntries {
		if entry.PctUsed > worstPctUsed {
			worstPctUsed = entry.PctUsed
		}
		if entry.Status == "blocking" && budgetHealthStatus != "blocking" {
			budgetHealthStatus = "blocking_soon"
			budgetHealthMsg = fmt.Sprintf("%s %s limit at %.0f%%", entry.ScopeType, entry.PeriodType, entry.PctUsed)
		} else if entry.Status == "warning" && budgetHealthStatus == "ok" {
			budgetHealthStatus = "warning"
			budgetHealthMsg = fmt.Sprintf("%s %s limit approaching threshold at %.0f%%", entry.ScopeType, entry.PeriodType, entry.PctUsed)
		}
	}
	if len(spendLimitEntries) == 0 {
		budgetHealthMsg = "No spend limits configured"
	}

	// Fill in missing dates for daily cost timeseries
	dailyCostMap := make(map[string]dailyCostRow, len(dailyCost))
	for _, d := range dailyCost {
		d.CostStr = d.Cost.StringFixed(4)
		dailyCostMap[d.Ts] = d
	}
	var filledDailyCost []gin.H
	for cursor := time.Date(rangeStart.Year(), rangeStart.Month(), rangeStart.Day(), 0, 0, 0, 0, loc); !cursor.After(time.Date(rangeEnd.Year(), rangeEnd.Month(), rangeEnd.Day(), 0, 0, 0, 0, loc)); cursor = cursor.AddDate(0, 0, 1) {
		ds := cursor.Format("2006-01-02")
		if row, ok := dailyCostMap[ds]; ok {
			filledDailyCost = append(filledDailyCost, gin.H{
				"ts": ds, "cost": row.Cost.StringFixed(4), "requests": row.Requests, "tokens": row.Tokens,
			})
		} else {
			filledDailyCost = append(filledDailyCost, gin.H{
				"ts": ds, "cost": "0.0000", "requests": int64(0), "tokens": int64(0),
			})
		}
	}
	if filledDailyCost == nil {
		filledDailyCost = []gin.H{}
	}

	// Fill in missing dates for daily latency timeseries
	dailyLatMap := make(map[string]dailyLatRow, len(dailyLatency))
	for _, d := range dailyLatency {
		dailyLatMap[d.Ts] = d
	}
	var filledDailyLatency []gin.H
	for cursor := time.Date(rangeStart.Year(), rangeStart.Month(), rangeStart.Day(), 0, 0, 0, 0, loc); !cursor.After(time.Date(rangeEnd.Year(), rangeEnd.Month(), rangeEnd.Day(), 0, 0, 0, 0, loc)); cursor = cursor.AddDate(0, 0, 1) {
		ds := cursor.Format("2006-01-02")
		if row, ok := dailyLatMap[ds]; ok {
			filledDailyLatency = append(filledDailyLatency, gin.H{"ts": ds, "ms": row.Ms})
		} else {
			filledDailyLatency = append(filledDailyLatency, gin.H{"ts": ds, "ms": float64(0)})
		}
	}
	if filledDailyLatency == nil {
		filledDailyLatency = []gin.H{}
	}

	// Fill in missing dates for daily outcomes timeseries
	dailyOutMap := make(map[string]dailyOutcomeRow, len(dailyOutcomes))
	for _, d := range dailyOutcomes {
		dailyOutMap[d.Ts] = d
	}
	var filledDailyOutcomes []gin.H
	for cursor := time.Date(rangeStart.Year(), rangeStart.Month(), rangeStart.Day(), 0, 0, 0, 0, loc); !cursor.After(time.Date(rangeEnd.Year(), rangeEnd.Month(), rangeEnd.Day(), 0, 0, 0, 0, loc)); cursor = cursor.AddDate(0, 0, 1) {
		ds := cursor.Format("2006-01-02")
		if row, ok := dailyOutMap[ds]; ok {
			filledDailyOutcomes = append(filledDailyOutcomes, gin.H{
				"ts": ds, "success": row.Success, "blocked": row.Blocked, "error": row.Error,
			})
		} else {
			filledDailyOutcomes = append(filledDailyOutcomes, gin.H{
				"ts": ds, "success": int64(0), "blocked": int64(0), "error": int64(0),
			})
		}
	}
	if filledDailyOutcomes == nil {
		filledDailyOutcomes = []gin.H{}
	}

	// Breakdown pct_of_total calculations
	byProviderOut := make([]gin.H, len(byProvider))
	for i, b := range byProvider {
		pct := float64(0)
		if !curKPI.SpendTotal.IsZero() {
			pct = b.Cost.Div(curKPI.SpendTotal).Mul(decimal.NewFromInt(100)).InexactFloat64()
		}
		byProviderOut[i] = gin.H{
			"name": b.Name, "cost": b.Cost.StringFixed(4),
			"input_tokens": b.InputTokens, "output_tokens": b.OutputTokens,
			"requests": b.Requests, "pct_of_total": math.Round(pct*10) / 10,
		}
	}

	byModelOut := make([]gin.H, len(byModel))
	for i, b := range byModel {
		pct := float64(0)
		if !curKPI.SpendTotal.IsZero() {
			pct = b.Cost.Div(curKPI.SpendTotal).Mul(decimal.NewFromInt(100)).InexactFloat64()
		}
		byModelOut[i] = gin.H{
			"name": b.Name, "provider": b.Provider, "cost": b.Cost.StringFixed(4),
			"input_tokens": b.InputTokens, "output_tokens": b.OutputTokens,
			"requests": b.Requests, "pct_of_total": math.Round(pct*10) / 10,
		}
	}

	byAPIKeyOut := make([]gin.H, len(byAPIKey))
	for i, b := range byAPIKey {
		pct := float64(0)
		if !curKPI.SpendTotal.IsZero() {
			pct = b.Cost.Div(curKPI.SpendTotal).Mul(decimal.NewFromInt(100)).InexactFloat64()
		}
		label := b.Label
		if label == "" && b.KeyID != "" {
			label = b.KeyID
			if len(label) > 12 {
				label = label[:12] + "…"
			}
		}
		byAPIKeyOut[i] = gin.H{
			"key_id": b.KeyID, "name": label, "cost": b.Cost.StringFixed(4),
			"input_tokens": b.InputTokens, "output_tokens": b.OutputTokens,
			"requests": b.Requests, "pct_of_total": math.Round(pct*10) / 10,
		}
	}

	byProjectOut := make([]gin.H, len(byProject))
	for i, b := range byProject {
		pct := float64(0)
		if !curKPI.SpendTotal.IsZero() {
			pct = b.Cost.Div(curKPI.SpendTotal).Mul(decimal.NewFromInt(100)).InexactFloat64()
		}
		byProjectOut[i] = gin.H{
			"project_id": b.ProjectID, "name": b.Name, "cost": b.Cost.StringFixed(4),
			"input_tokens": b.InputTokens, "output_tokens": b.OutputTokens,
			"requests": b.Requests, "pct_of_total": math.Round(pct*10) / 10,
		}
	}

	// Insights: cost drivers
	var costDrivers []gin.H
	if len(byAPIKeyOut) > 0 {
		topKeyPct := byAPIKeyOut[0]["pct_of_total"].(float64)
		if topKeyPct > 30 {
			costDrivers = append(costDrivers, gin.H{
				"type": "top_contributor",
				"text": fmt.Sprintf("Top API key accounts for %.0f%% of spend.", topKeyPct),
			})
		}
	}
	if len(byModelOut) > 0 {
		topModelPct := byModelOut[0]["pct_of_total"].(float64)
		if topModelPct > 30 {
			costDrivers = append(costDrivers, gin.H{
				"type": "top_model",
				"text": fmt.Sprintf("Top model is %s at %.0f%% of spend.", byModelOut[0]["name"], topModelPct),
			})
		}
	}
	if curKPI.OutputTokens > 0 && curKPI.InputTokens > 0 {
		ratio := float64(curKPI.OutputTokens) / float64(curKPI.InputTokens)
		if ratio > 3.0 {
			costDrivers = append(costDrivers, gin.H{
				"type": "token_ratio",
				"text": fmt.Sprintf("Output tokens are %.1f× input tokens (possible verbose responses).", ratio),
			})
		}
	}
	if costDrivers == nil {
		costDrivers = []gin.H{}
	}

	// Concentration metrics
	var topKeyPct, topProjectPct, topModelPct float64
	if len(byAPIKeyOut) > 0 {
		topKeyPct = byAPIKeyOut[0]["pct_of_total"].(float64)
	}
	if len(byProjectOut) > 0 {
		topProjectPct = byProjectOut[0]["pct_of_total"].(float64)
	}
	if len(byModelOut) > 0 {
		topModelPct = byModelOut[0]["pct_of_total"].(float64)
	}

	// Rate limits output
	rateLimitsOut := make([]gin.H, len(rateLimits))
	for i, rl := range rateLimits {
		rateLimitsOut[i] = gin.H{
			"id":             rl.ID,
			"scope_type":     rl.ScopeType,
			"scope_id":       rl.ScopeID,
			"provider":       rl.Provider,
			"model":          rl.Model,
			"metric":         rl.Metric,
			"limit_value":    rl.LimitValue,
			"window_seconds": rl.WindowSeconds,
		}
	}
	if rateLimitsOut == nil {
		rateLimitsOut = []gin.H{}
	}

	if spendLimitEntries == nil {
		spendLimitEntries = []budgetEntry{}
	}

	// ── Build cost scope note for the frontend ─────────────────────────────
	var costNote string
	switch billingMode {
	case "monthly_subscription":
		costNote = "Monthly subscription traffic is not billed per token; cost is excluded."
	case "all":
		costNote = "Cost includes API usage billed traffic only."
	default:
		costNote = ""
	}

	// ── Build response ─────────────────────────────────────────────────────
	c.JSON(http.StatusOK, gin.H{
		"plan":      plan,
		"cost_note": costNote,
		"range": gin.H{
			"from":      rangeStart.Format("2006-01-02"),
			"to":        rangeEnd.Format("2006-01-02"),
			"prev_from": prevStart.Format("2006-01-02"),
			"prev_to":   prevEnd.Format("2006-01-02"),
		},
		"filters": gin.H{
			"billing_mode": billingMode,
			"project_id":   projectID,
			"api_key_id":   apiKeyID,
		},
		"kpis": gin.H{
			"spend_total":          gin.H{"value": curKPI.SpendTotal.StringFixed(4), "delta_pct": math.Round(spendDelta*10) / 10},
			"projected_month_end":  gin.H{"value": fc.Forecast.StringFixed(4), "delta_pct": 0},
			"budget_health":        gin.H{"status": budgetHealthStatus, "message": budgetHealthMsg},
			"success_rate":         gin.H{"value": math.Round(successRate*10) / 10, "delta_pct": 0},
			"requests_total":       gin.H{"value": curKPI.RequestsTotal, "delta_pct": math.Round(requestsDelta*10) / 10},
			"avg_cost_per_request": gin.H{"value": curAvgCost.StringFixed(6), "delta_pct": math.Round(avgCostDelta*10) / 10},
		},
		"forecast": gin.H{
			"total_so_far":   fc.TotalSoFar.StringFixed(4),
			"daily_average":  fc.DailyAverage.StringFixed(4),
			"forecast":       fc.Forecast.StringFixed(4),
			"days_elapsed":   fc.DaysElapsed,
			"days_remaining": fc.DaysRemaining,
		},
		"timeseries": gin.H{
			"daily_cost":       filledDailyCost,
			"daily_latency_p95": filledDailyLatency,
			"outcomes":          filledDailyOutcomes,
		},
		"breakdowns": gin.H{
			"by_provider": byProviderOut,
			"by_model":    byModelOut,
			"by_api_key":  byAPIKeyOut,
			"by_project":  byProjectOut,
		},
		"limits": gin.H{
			"budget_utilization_pct": math.Round(worstPctUsed*10) / 10,
			"blocked_requests":       curBlocked.Budget,
			"rate_limited_requests":  curBlocked.RateLimit,
			"active_spend_limits":    spendLimitEntries,
			"active_rate_limits":     rateLimitsOut,
		},
		"governance": gov,
		"insights": gin.H{
			"cost_drivers": costDrivers,
			"concentration": gin.H{
				"top_api_key_pct": math.Round(topKeyPct*10) / 10,
				"top_project_pct": math.Round(topProjectPct*10) / 10,
				"top_model_pct":   math.Round(topModelPct*10) / 10,
			},
		},
		"latency": gin.H{
			"p50":          latency.P50,
			"p95":          latency.P95,
			"p99":          latency.P99,
			"avg":          latency.Avg,
			"sample_count": latency.SampleCount,
		},
		"recent_requests": gin.H{
			"default_limit": 100,
			"has_more":      curKPI.RequestsTotal > 100,
		},
	})
}

// ─── handleDashboardRecentRequests ─────────────────────────────────────────────
// GET /v1/dashboard/recent-requests?from=...&to=...&limit=100&billing_mode=...&project_id=...&api_key_id=...&tz=...
func (s *Server) handleDashboardRecentRequests(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	db := s.postgresDB.GetDB()
	loc := parseTimezone(c)
	now := time.Now().In(loc)

	var tenant models.Tenant
	db.First(&tenant, tenantID)
	lim := models.GetPlanLimits(tenant.Plan)

	fromStr := c.Query("from")
	toStr := c.Query("to")
	billingMode := c.DefaultQuery("billing_mode", "all")
	projectIDStr := c.Query("project_id")
	apiKeyID := c.Query("api_key_id")
	limitStr := c.DefaultQuery("limit", "100")

	limit := 100
	if l, err := strconv.Atoi(limitStr); err == nil && (l == 100 || l == 500) {
		limit = l
	}

	rangeStart := now.AddDate(0, 0, -7)
	rangeEnd := now

	if fromStr != "" && toStr != "" {
		if rs, err := time.Parse("2006-01-02", fromStr); err == nil {
			rangeStart = time.Date(rs.Year(), rs.Month(), rs.Day(), 0, 0, 0, 0, loc)
		}
		if re, err := time.Parse("2006-01-02", toStr); err == nil {
			rangeEnd = time.Date(re.Year(), re.Month(), re.Day(), 23, 59, 59, 999999999, loc)
		}
	}

	effectiveMin := computeEffectiveMinStart(lim)
	if rangeStart.Before(effectiveMin) {
		rangeStart = effectiveMin
	}
	if rangeEnd.After(now) {
		rangeEnd = now
	}

	q := db.Model(&models.UsageLog{}).
		Select("usage_logs.*, COALESCE(api_keys.label, '') as key_label").
		Joins("LEFT JOIN api_keys ON api_keys.key_id = usage_logs.key_id").
		Where("usage_logs.tenant_id = ?", tenantID).
		Where("usage_logs.created_at >= ? AND usage_logs.created_at <= ?", rangeStart, rangeEnd)

	if billingMode == "api_usage_billed" {
		q = q.Where("usage_logs.api_usage_billed = ?", true)
	} else if billingMode == "monthly_subscription" {
		q = q.Where("usage_logs.api_usage_billed = ?", false)
	}
	if apiKeyID != "" {
		q = q.Where("usage_logs.key_id = ?", apiKeyID)
	}
	if projectIDStr != "" {
		if pid, err := strconv.ParseUint(projectIDStr, 10, 64); err == nil {
			q = q.Where("usage_logs.project_id = ?", uint(pid))
		}
	}

	var logs []models.UsageLog
	q.Order("usage_logs.created_at DESC").Limit(limit + 1).Find(&logs)

	hasMore := len(logs) > limit
	if hasMore {
		logs = logs[:limit]
	}

	rows := make([]gin.H, len(logs))
	for i, log := range logs {
		// Cost is only meaningful for api_usage_billed rows.
		// Monthly subscription rows always show $0.
		costStr := "0.0000"
		if log.APIUsageBilled {
			costStr = log.Cost.StringFixed(4)
		}
		rows[i] = gin.H{
			"id":              log.ID,
			"provider":        log.Provider,
			"model":           log.Model,
			"key_id":          log.KeyID,
			"key_label":       log.KeyLabel,
			"prompt_tokens":   log.PromptTokens,
			"completion_tokens": log.CompletionTokens,
			"cost":            costStr,
			"latency_ms":      log.LatencyMs,
			"api_usage_billed": log.APIUsageBilled,
			"result":          "success",
			"created_at":      log.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"requests": rows,
		"has_more": hasMore,
	})
}
