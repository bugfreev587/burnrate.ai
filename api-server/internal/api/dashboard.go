package api

import (
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// computeEffectiveMinStart returns the earliest date a tenant may query, taking
// into account both the plan's rolling data-retention window and the platform's
// availability floor (AVAILABILITY_MIN_START_DATE env var, default 2026-01-01).
func computeEffectiveMinStart(lim models.PlanLimits) time.Time {
	availStr := os.Getenv("AVAILABILITY_MIN_START_DATE")
	if availStr == "" {
		availStr = "2026-01-01"
	}
	avail, err := time.Parse("2006-01-02", availStr)
	if err != nil {
		avail = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	if lim.DataRetentionDays == -1 {
		// Unlimited retention: only the availability floor applies.
		return avail
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	retentionMin := today.AddDate(0, 0, -lim.DataRetentionDays)

	if retentionMin.After(avail) {
		return retentionMin
	}
	return avail
}

// handleDashboardConfig returns the date-range configuration for the current
// tenant: plan retention limits, platform availability floor, effective min
// start date, and which preset options are enabled.
// GET /v1/dashboard/config
func (s *Server) handleDashboardConfig(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var tenant models.Tenant
	s.postgresDB.GetDB().First(&tenant, tenantID)
	lim := models.GetPlanLimits(tenant.Plan)

	// Availability floor from env.
	availStr := os.Getenv("AVAILABILITY_MIN_START_DATE")
	if availStr == "" {
		availStr = "2026-01-01"
	}
	avail, err := time.Parse("2006-01-02", availStr)
	if err != nil {
		avail = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		availStr = avail.Format("2006-01-02")
	}

	effectiveMin := computeEffectiveMinStart(lim)

	// Retention descriptor.
	var retentionObj gin.H
	if lim.DataRetentionDays == -1 {
		retentionObj = gin.H{"type": "UNLIMITED", "max_days": -1}
	} else {
		retentionObj = gin.H{"type": "ROLLING", "max_days": lim.DataRetentionDays}
	}

	// Preset options.
	type presetDef struct {
		Key  string
		Days int // -1 signals "custom"
	}
	presets := []presetDef{
		{"1d", 1},
		{"3d", 3},
		{"7d", 7},
		{"14d", 14},
		{"30d", 30},
		{"90d", 90},
		{"custom", -1},
	}

	presetOptions := make([]gin.H, len(presets))
	for i, p := range presets {
		var enabled bool
		if p.Key == "custom" {
			enabled = lim.DataRetentionDays == -1 || lim.DataRetentionDays > 7
		} else {
			enabled = lim.DataRetentionDays == -1 || p.Days <= lim.DataRetentionDays
		}
		if p.Key == "custom" {
			presetOptions[i] = gin.H{"key": p.Key, "enabled": enabled}
		} else {
			presetOptions[i] = gin.H{"key": p.Key, "days": p.Days, "enabled": enabled}
		}
	}

	plan := tenant.Plan
	if plan == "" {
		plan = models.PlanFree
	}

	c.JSON(http.StatusOK, gin.H{
		"plan":      plan,
		"retention": retentionObj,
		"availability": gin.H{
			"min_start_date": availStr,
		},
		"effective": gin.H{
			"min_start_date": effectiveMin.Format("2006-01-02"),
		},
		"preset_options": presetOptions,
	})
}
