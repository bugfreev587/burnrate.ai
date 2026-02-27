package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// handleUsageMetrics returns gateway latency percentiles and activity metrics.
// GET /v1/usage/metrics?start_date=YYYY-MM-DD&end_date=YYYY-MM-DD&tz=America/Los_Angeles
func (s *Server) handleUsageMetrics(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	db := s.postgresDB.GetDB()
	loc := parseTimezone(c)
	now := time.Now().In(loc)

	// ── Resolve date range ──────────────────────────────────────────────────
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	var rangeStart, rangeEnd time.Time
	if startDateStr != "" && endDateStr != "" {
		rs, err1 := time.Parse("2006-01-02", startDateStr)
		re, err2 := time.Parse("2006-01-02", endDateStr)
		if err1 == nil && err2 == nil {
			var tenant models.Tenant
			db.First(&tenant, tenantID)
			lim := models.GetPlanLimits(tenant.Plan)
			effectiveMin := computeEffectiveMinStart(lim)

			rangeStart = time.Date(rs.Year(), rs.Month(), rs.Day(), 0, 0, 0, 0, loc)
			if rangeStart.Before(effectiveMin) {
				rangeStart = effectiveMin
			}
			rangeEnd = time.Date(re.Year(), re.Month(), re.Day(), 23, 59, 59, 999999999, loc)
			if rangeEnd.After(now) {
				rangeEnd = now
			}
		} else {
			rangeStart = now.AddDate(0, 0, -7)
			rangeEnd = now
		}
	} else {
		rangeStart = now.AddDate(0, 0, -7)
		rangeEnd = now
	}

	// ── Latency percentiles from usage_logs ─────────────────────────────────
	type latencyRow struct {
		P50         float64 `gorm:"column:p50"`
		P95         float64 `gorm:"column:p95"`
		P99         float64 `gorm:"column:p99"`
		Avg         float64 `gorm:"column:avg"`
		SampleCount int64   `gorm:"column:sample_count"`
	}
	var latency latencyRow
	db.Raw(`
		SELECT
			COALESCE(PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY latency_ms), 0) AS p50,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms), 0) AS p95,
			COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY latency_ms), 0) AS p99,
			COALESCE(AVG(latency_ms), 0) AS avg,
			COUNT(*) AS sample_count
		FROM usage_logs
		WHERE tenant_id = ? AND latency_ms > 0 AND created_at >= ? AND created_at <= ?
	`, tenantID, rangeStart, rangeEnd).Scan(&latency)

	// ── Activity counts ─────────────────────────────────────────────────────
	type activityRow struct {
		TotalRequests int64 `gorm:"column:total_requests"`
		ActiveAPIKeys int64 `gorm:"column:active_api_keys"`
	}
	var activity activityRow
	db.Raw(`
		SELECT
			COUNT(*) AS total_requests,
			COUNT(DISTINCT key_id) AS active_api_keys
		FROM usage_logs
		WHERE tenant_id = ? AND created_at >= ? AND created_at <= ?
	`, tenantID, rangeStart, rangeEnd).Scan(&activity)

	// Blocked counts from gateway_events
	type blockedRow struct {
		EventType string `gorm:"column:event_type"`
		Count     int64  `gorm:"column:cnt"`
	}
	var blockedRows []blockedRow
	db.Raw(`
		SELECT event_type, COUNT(*) AS cnt
		FROM gateway_events
		WHERE tenant_id = ? AND created_at >= ? AND created_at <= ?
		GROUP BY event_type
	`, tenantID, rangeStart, rangeEnd).Scan(&blockedRows)

	var blockedRateLimit, blockedBudget int64
	for _, r := range blockedRows {
		switch r.EventType {
		case "rate_limit_429":
			blockedRateLimit = r.Count
		case "budget_exceeded_402":
			blockedBudget = r.Count
		}
	}
	totalBlocked := blockedRateLimit + blockedBudget
	totalAll := activity.TotalRequests + totalBlocked
	var successRate float64
	if totalAll > 0 {
		successRate = float64(activity.TotalRequests) / float64(totalAll) * 100
	}

	// ── Daily activity ──────────────────────────────────────────────────────
	type dailyUsageRow struct {
		Date       string  `gorm:"column:date"`
		Requests   int64   `gorm:"column:requests"`
		ActiveKeys int64   `gorm:"column:active_keys"`
		P50Latency float64 `gorm:"column:p50_latency"`
		P95Latency float64 `gorm:"column:p95_latency"`
	}
	var dailyUsage []dailyUsageRow
	db.Raw(`
		SELECT
			TO_CHAR(created_at AT TIME ZONE ?, 'YYYY-MM-DD') AS date,
			COUNT(*) AS requests,
			COUNT(DISTINCT key_id) AS active_keys,
			COALESCE(PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE latency_ms > 0), 0) AS p50_latency,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE latency_ms > 0), 0) AS p95_latency
		FROM usage_logs
		WHERE tenant_id = ? AND created_at >= ? AND created_at <= ?
		GROUP BY date
		ORDER BY date
	`, loc.String(), tenantID, rangeStart, rangeEnd).Scan(&dailyUsage)

	// Blocked counts per day
	type dailyBlockedRow struct {
		Date    string `gorm:"column:date"`
		Blocked int64  `gorm:"column:blocked"`
	}
	var dailyBlocked []dailyBlockedRow
	db.Raw(`
		SELECT
			TO_CHAR(created_at AT TIME ZONE ?, 'YYYY-MM-DD') AS date,
			COUNT(*) AS blocked
		FROM gateway_events
		WHERE tenant_id = ? AND created_at >= ? AND created_at <= ?
		GROUP BY date
		ORDER BY date
	`, loc.String(), tenantID, rangeStart, rangeEnd).Scan(&dailyBlocked)

	// Merge daily usage + blocked
	blockedByDate := make(map[string]int64, len(dailyBlocked))
	for _, d := range dailyBlocked {
		blockedByDate[d.Date] = d.Blocked
	}
	type dailyActivityOut struct {
		Date       string  `json:"date"`
		Requests   int64   `json:"requests"`
		Blocked    int64   `json:"blocked"`
		ActiveKeys int64   `json:"active_keys"`
		P50Latency float64 `json:"p50_latency"`
		P95Latency float64 `json:"p95_latency"`
	}
	dailyActivity := make([]dailyActivityOut, 0, len(dailyUsage))
	for _, d := range dailyUsage {
		dailyActivity = append(dailyActivity, dailyActivityOut{
			Date:       d.Date,
			Requests:   d.Requests,
			Blocked:    blockedByDate[d.Date],
			ActiveKeys: d.ActiveKeys,
			P50Latency: d.P50Latency,
			P95Latency: d.P95Latency,
		})
	}

	// ── By model ────────────────────────────────────────────────────────────
	type byModelRow struct {
		Model      string  `gorm:"column:model"`
		Provider   string  `gorm:"column:provider"`
		Requests   int64   `gorm:"column:requests"`
		P50Latency float64 `gorm:"column:p50_latency"`
		P95Latency float64 `gorm:"column:p95_latency"`
	}
	var byModelUsage []byModelRow
	db.Raw(`
		SELECT
			model, provider,
			COUNT(*) AS requests,
			COALESCE(PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE latency_ms > 0), 0) AS p50_latency,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE latency_ms > 0), 0) AS p95_latency
		FROM usage_logs
		WHERE tenant_id = ? AND created_at >= ? AND created_at <= ?
		GROUP BY model, provider
		ORDER BY requests DESC
	`, tenantID, rangeStart, rangeEnd).Scan(&byModelUsage)

	// Blocked per model from gateway_events
	type modelBlockedRow struct {
		Model   string `gorm:"column:model"`
		Blocked int64  `gorm:"column:blocked"`
	}
	var modelBlocked []modelBlockedRow
	db.Raw(`
		SELECT model, COUNT(*) AS blocked
		FROM gateway_events
		WHERE tenant_id = ? AND created_at >= ? AND created_at <= ?
		GROUP BY model
	`, tenantID, rangeStart, rangeEnd).Scan(&modelBlocked)

	modelBlockedMap := make(map[string]int64, len(modelBlocked))
	for _, m := range modelBlocked {
		modelBlockedMap[m.Model] = m.Blocked
	}

	type byModelOut struct {
		Model      string  `json:"model"`
		Provider   string  `json:"provider"`
		Requests   int64   `json:"requests"`
		Blocked    int64   `json:"blocked"`
		P50Latency float64 `json:"p50_latency"`
		P95Latency float64 `json:"p95_latency"`
	}
	byModel := make([]byModelOut, 0, len(byModelUsage))
	for _, m := range byModelUsage {
		byModel = append(byModel, byModelOut{
			Model:      m.Model,
			Provider:   m.Provider,
			Requests:   m.Requests,
			Blocked:    modelBlockedMap[m.Model],
			P50Latency: m.P50Latency,
			P95Latency: m.P95Latency,
		})
	}

	// ── By API key ──────────────────────────────────────────────────────────
	type byKeyRow struct {
		KeyID      string  `gorm:"column:key_id"`
		Requests   int64   `gorm:"column:requests"`
		P50Latency float64 `gorm:"column:p50_latency"`
	}
	var byKeyUsage []byKeyRow
	db.Raw(`
		SELECT
			key_id,
			COUNT(*) AS requests,
			COALESCE(PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE latency_ms > 0), 0) AS p50_latency
		FROM usage_logs
		WHERE tenant_id = ? AND created_at >= ? AND created_at <= ?
		GROUP BY key_id
		ORDER BY requests DESC
	`, tenantID, rangeStart, rangeEnd).Scan(&byKeyUsage)

	// Blocked per key
	type keyBlockedRow struct {
		KeyID   string `gorm:"column:key_id"`
		Blocked int64  `gorm:"column:blocked"`
	}
	var keyBlocked []keyBlockedRow
	db.Raw(`
		SELECT key_id, COUNT(*) AS blocked
		FROM gateway_events
		WHERE tenant_id = ? AND created_at >= ? AND created_at <= ?
		GROUP BY key_id
	`, tenantID, rangeStart, rangeEnd).Scan(&keyBlocked)

	keyBlockedMap := make(map[string]int64, len(keyBlocked))
	for _, k := range keyBlocked {
		keyBlockedMap[k.KeyID] = k.Blocked
	}

	// Resolve key labels
	type keyLabelRow struct {
		KeyID string
		Label string
	}
	var keyLabels []keyLabelRow
	keyIDs := make([]string, 0, len(byKeyUsage))
	for _, k := range byKeyUsage {
		keyIDs = append(keyIDs, k.KeyID)
	}
	if len(keyIDs) > 0 {
		db.Raw(`SELECT key_id, label FROM api_keys WHERE key_id IN ?`, keyIDs).Scan(&keyLabels)
	}
	keyLabelMap := make(map[string]string, len(keyLabels))
	for _, kl := range keyLabels {
		keyLabelMap[kl.KeyID] = kl.Label
	}

	type byAPIKeyOut struct {
		KeyID      string  `json:"key_id"`
		Label      string  `json:"label"`
		Requests   int64   `json:"requests"`
		Blocked    int64   `json:"blocked"`
		P50Latency float64 `json:"p50_latency"`
	}
	byAPIKey := make([]byAPIKeyOut, 0, len(byKeyUsage))
	for _, k := range byKeyUsage {
		byAPIKey = append(byAPIKey, byAPIKeyOut{
			KeyID:      k.KeyID,
			Label:      keyLabelMap[k.KeyID],
			Requests:   k.Requests,
			Blocked:    keyBlockedMap[k.KeyID],
			P50Latency: k.P50Latency,
		})
	}

	// ── Response ────────────────────────────────────────────────────────────
	c.JSON(http.StatusOK, gin.H{
		"latency": gin.H{
			"p50":          latency.P50,
			"p95":          latency.P95,
			"p99":          latency.P99,
			"avg":          latency.Avg,
			"sample_count": latency.SampleCount,
		},
		"activity": gin.H{
			"active_api_keys":    activity.ActiveAPIKeys,
			"total_requests":     activity.TotalRequests,
			"total_blocked":      totalBlocked,
			"blocked_rate_limit": blockedRateLimit,
			"blocked_budget":     blockedBudget,
			"success_rate":       successRate,
		},
		"daily_activity": dailyActivity,
		"by_model":       byModel,
		"by_api_key":     byAPIKey,
	})
}
