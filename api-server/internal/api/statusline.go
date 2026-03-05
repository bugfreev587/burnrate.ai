package api

import (
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// handleStatusLine returns a compact JSON payload designed for CLI status line
// rendering. Authenticated via X-TokenGate-Key (tenant auth middleware).
//
// GET /v1/statusline
func (s *Server) handleStatusLine(c *gin.Context) {
	tenantID := c.GetUint("tenant_id")
	keyID, _ := c.Get("key_id")
	keyIDStr, _ := keyID.(string)
	billingMode := c.GetString("billing_mode")

	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	db := s.postgresDB.GetDB()
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	weekStart := time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, time.UTC)

	apiUsageBilled := billingMode == models.BillingModeAPIUsage

	var wg sync.WaitGroup

	// ── Usage: today + month-to-date ────────────────────────────────────
	type usageRow struct {
		Cost         decimal.Decimal `gorm:"column:cost"`
		Requests     int64           `gorm:"column:requests"`
		InputTokens  int64           `gorm:"column:input_tokens"`
		OutputTokens int64           `gorm:"column:output_tokens"`
	}
	var todayUsage, mtdUsage usageRow

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Raw(`
			SELECT COALESCE(SUM(CASE WHEN api_usage_billed THEN cost ELSE 0 END), 0) AS cost,
			       COUNT(*) AS requests,
			       COALESCE(SUM(prompt_tokens), 0) AS input_tokens,
			       COALESCE(SUM(completion_tokens), 0) AS output_tokens
			FROM usage_logs
			WHERE tenant_id = ? AND key_id = ? AND created_at >= ?`,
			tenantID, keyIDStr, todayStart).Scan(&todayUsage)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Raw(`
			SELECT COALESCE(SUM(CASE WHEN api_usage_billed THEN cost ELSE 0 END), 0) AS cost,
			       COUNT(*) AS requests,
			       COALESCE(SUM(prompt_tokens), 0) AS input_tokens,
			       COALESCE(SUM(completion_tokens), 0) AS output_tokens
			FROM usage_logs
			WHERE tenant_id = ? AND key_id = ? AND created_at >= ?`,
			tenantID, keyIDStr, monthStart).Scan(&mtdUsage)
	}()

	// ── Budget limits ───────────────────────────────────────────────────
	var budgetLimits []models.BudgetLimit

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Where("tenant_id = ? AND enabled = ?", tenantID, true).Find(&budgetLimits)
	}()

	wg.Wait()

	// ── Build budget entries ────────────────────────────────────────────
	// For each budget limit, compute current spend scoped to this key (if
	// scope_type=api_key) or to the whole account.
	type budgetResult struct {
		Period  string  `json:"period"`
		Limit   string  `json:"limit"`
		Used    string  `json:"used"`
		Percent float64 `json:"percent"`
		Status  string  `json:"status"`
	}
	var budgets []budgetResult

	for _, bl := range budgetLimits {
		// Only include budgets that apply to this key
		if bl.ScopeType == "api_key" && bl.ScopeID != keyIDStr {
			continue
		}

		var periodStart time.Time
		switch bl.PeriodType {
		case "monthly":
			periodStart = monthStart
		case "weekly":
			periodStart = weekStart
		case "daily":
			periodStart = todayStart
		default:
			periodStart = monthStart
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
			pctUsed = math.Round(pctUsed*10) / 10
		}

		status := "ok"
		thresholdPct := bl.AlertThreshold.InexactFloat64()
		if pctUsed >= 100 {
			status = "exceeded"
		} else if pctUsed >= thresholdPct {
			status = "warning"
		}

		budgets = append(budgets, budgetResult{
			Period:  bl.PeriodType,
			Limit:   bl.LimitAmount.StringFixed(2),
			Used:    currentSpend.StringFixed(4),
			Percent: pctUsed,
			Status:  status,
		})
	}
	if budgets == nil {
		budgets = []budgetResult{}
	}

	// ── Flatten budgets into a map keyed by period for convenience ──────
	budgetMap := make(map[string]budgetResult)
	for _, b := range budgets {
		// If multiple budgets exist for the same period, pick the one with
		// the highest percent (worst case).
		if existing, ok := budgetMap[b.Period]; !ok || b.Percent > existing.Percent {
			budgetMap[b.Period] = b
		}
	}

	// Build the budgets response object with daily/weekly/monthly keys.
	budgetsResp := make(map[string]interface{})
	for _, period := range []string{"daily", "weekly", "monthly"} {
		if b, ok := budgetMap[period]; ok {
			budgetsResp[period] = b
		}
	}

	// ── Cost: zero out when not api_usage_billed ────────────────────────
	costToday := todayUsage.Cost
	costMTD := mtdUsage.Cost
	costNote := ""
	if !apiUsageBilled {
		costToday = decimal.Zero
		costMTD = decimal.Zero
		costNote = "Monthly subscription mode — cost is covered by user's provider plan."
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":  true,
		"now": now.Format(time.RFC3339),

		"billing_mode": billingMode,

		"cost": gin.H{
			"today":         costToday.StringFixed(4),
			"month_to_date": costMTD.StringFixed(4),
			"currency":      "USD",
			"note":          costNote,
		},

		"budgets": budgetsResp,

		"usage": gin.H{
			"tokens_in":    mtdUsage.InputTokens,
			"tokens_out":   mtdUsage.OutputTokens,
			"tokens_total": mtdUsage.InputTokens + mtdUsage.OutputTokens,
			"requests":     mtdUsage.Requests,
		},
	})
}

