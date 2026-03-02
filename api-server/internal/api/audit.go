package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// ── Request types ────────────────────────────────────────────────────────────

type createAuditReportReq struct {
	PeriodStart              string   `json:"period_start" binding:"required"`    // YYYY-MM-DD or YYYY-MM-DDTHH:mm
	PeriodEnd                string   `json:"period_end" binding:"required"`      // YYYY-MM-DD or YYYY-MM-DDTHH:mm
	Format                   string   `json:"format" binding:"required"`          // "PDF" | "CSV"
	Timezone                 string   `json:"timezone,omitempty"`                 // IANA timezone e.g. "America/Los_Angeles"
	Provider                 string   `json:"provider,omitempty"`                 // optional filter
	APIKeyIDs                []string `json:"api_key_ids,omitempty"`              // optional filter
	APIUsageBilled           *bool    `json:"api_usage_billed,omitempty"`         // optional filter
	ProjectIDs               []uint   `json:"project_ids,omitempty"`              // optional filter
	UserIDs                  []string `json:"user_ids,omitempty"`                 // optional filter
	BillingMode              string   `json:"billing_mode,omitempty"`             // "api_usage" | "subscription" | ""
	IncludeTopRequestsByCost *bool    `json:"include_top_requests_by_cost,omitempty"`
	TopRequestsLimit         *int     `json:"top_requests_limit,omitempty"`
}

// handleCreateAuditReport creates a new audit report generation job.
// POST /v1/audit/reports
func (s *Server) handleCreateAuditReport(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	tenantID, _ := middleware.GetTenantIDFromContext(c)

	// Fetch tenant + plan limits.
	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, tenantID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load tenant"})
		return
	}
	lim := models.GetPlanLimits(tenant.Plan)

	// Reject free plan.
	if !lim.AllowExport {
		c.JSON(http.StatusForbidden, gin.H{"error": "export not available on free plan"})
		return
	}

	var req createAuditReportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate format.
	if req.Format != "PDF" && req.Format != "CSV" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format must be PDF or CSV"})
		return
	}

	// Validate billing_mode.
	if req.BillingMode != "" && req.BillingMode != "api_usage" && req.BillingMode != "subscription" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "billing_mode must be api_usage, subscription, or empty"})
		return
	}

	// Validate timezone.
	tzName := req.Timezone
	if tzName == "" {
		tzName = "UTC"
	}
	loc, err := validateTimezone(tzName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid timezone: " + tzName})
		return
	}

	// Parse and validate dates (flexible: YYYY-MM-DD or YYYY-MM-DDTHH:mm or RFC3339).
	periodStartRaw, startHasTime, err := parseDatetimeFlexible(req.PeriodStart)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid period_start (expected YYYY-MM-DD or YYYY-MM-DDTHH:mm)"})
		return
	}
	periodEndRaw, endHasTime, err := parseDatetimeFlexible(req.PeriodEnd)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid period_end (expected YYYY-MM-DD or YYYY-MM-DDTHH:mm)"})
		return
	}

	// Convert to UTC using timezone: if date-only, use start-of-day / end-of-day.
	var periodStart, periodEnd time.Time
	if startHasTime {
		periodStart = time.Date(periodStartRaw.Year(), periodStartRaw.Month(), periodStartRaw.Day(),
			periodStartRaw.Hour(), periodStartRaw.Minute(), periodStartRaw.Second(), 0, loc).UTC()
	} else {
		periodStart = time.Date(periodStartRaw.Year(), periodStartRaw.Month(), periodStartRaw.Day(),
			0, 0, 0, 0, loc).UTC()
	}
	if endHasTime {
		periodEnd = time.Date(periodEndRaw.Year(), periodEndRaw.Month(), periodEndRaw.Day(),
			periodEndRaw.Hour(), periodEndRaw.Minute(), periodEndRaw.Second(), 0, loc).UTC()
	} else {
		periodEnd = time.Date(periodEndRaw.Year(), periodEndRaw.Month(), periodEndRaw.Day(),
			23, 59, 59, 999999999, loc).UTC()
	}

	// period_start must be before period_end.
	if !periodStart.Before(periodEnd) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "period_start must be before period_end"})
		return
	}

	// Enforce plan retention: period_start >= effective min start.
	effectiveMin := computeEffectiveMinStart(lim)
	if periodStart.Before(effectiveMin) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("period_start cannot be before %s (plan retention limit)",
				effectiveMin.Format("2006-01-02")),
		})
		return
	}

	// Cap max report duration: 90 days (Business: 365 days).
	maxDays := 90
	if tenant.Plan == models.PlanBusiness {
		maxDays = 365
	}
	if periodEnd.Sub(periodStart).Hours()/24 > float64(maxDays) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("report period cannot exceed %d days", maxDays),
		})
		return
	}

	// Validate TopRequestsLimit.
	includeTopRequests := false
	topRequestsLimit := 10
	if req.IncludeTopRequestsByCost != nil && *req.IncludeTopRequestsByCost {
		includeTopRequests = true
	}
	if req.TopRequestsLimit != nil {
		topRequestsLimit = *req.TopRequestsLimit
		if topRequestsLimit < 1 {
			topRequestsLimit = 1
		}
		if topRequestsLimit > 100 {
			topRequestsLimit = 100
		}
	}

	// Check concurrent limit.
	pending, err := s.auditSvc.CountPending(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check pending reports"})
		return
	}
	if pending >= 3 {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many pending reports (limit: 3)"})
		return
	}

	// Build filters JSON.
	filters := models.AuditReportFilters{
		APIKeyIDs:                req.APIKeyIDs,
		Provider:                 req.Provider,
		APIUsageBilled:           req.APIUsageBilled,
		ProjectIDs:               req.ProjectIDs,
		UserIDs:                  req.UserIDs,
		BillingMode:              req.BillingMode,
		IncludeTopRequestsByCost: includeTopRequests,
		TopRequestsLimit:         topRequestsLimit,
	}
	filtersJSON, _ := json.Marshal(filters)

	report := &models.AuditReport{
		TenantID:        tenantID,
		CreatedByUserID: user.ID,
		CreatedByEmail:  user.Email,
		PeriodStart:     periodStart,
		PeriodEnd:       periodEnd,
		Timezone:        tzName,
		FiltersJSON:     string(filtersJSON),
		Format:          req.Format,
		Status:          models.ReportStatusQueued,
	}
	if err := s.auditSvc.Create(c.Request.Context(), report); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create report"})
		return
	}

	// Publish to queue.
	if err := s.reportQueue.Publish(c.Request.Context(), report.ID); err != nil {
		// Mark as failed if we can't enqueue.
		_ = s.auditSvc.UpdateStatus(c.Request.Context(), report.ID, models.ReportStatusFailed, "failed to enqueue")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue report"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":     report.ID,
		"status": report.Status,
	})
}

// handleGetAuditReport returns metadata for a single report.
// GET /v1/audit/reports/:id
func (s *Server) handleGetAuditReport(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	reportID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report id"})
		return
	}

	report, err := s.auditSvc.GetByID(c.Request.Context(), tenantID, uint(reportID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "report not found"})
		return
	}

	c.JSON(http.StatusOK, report)
}

// handleListAuditReports returns all reports for the tenant.
// GET /v1/audit/reports
func (s *Server) handleListAuditReports(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	reports, err := s.auditSvc.ListByTenant(c.Request.Context(), tenantID, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list reports"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"reports": reports})
}

// handleDownloadAuditReport streams the report artifact to the client.
// GET /v1/audit/reports/:id/download
func (s *Server) handleDownloadAuditReport(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	reportID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report id"})
		return
	}

	report, err := s.auditSvc.GetWithArtifact(c.Request.Context(), tenantID, uint(reportID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "report not found"})
		return
	}

	if report.Status != models.ReportStatusCompleted {
		c.JSON(http.StatusConflict, gin.H{
			"error":  "report not ready",
			"status": report.Status,
		})
		return
	}

	// Determine content type and filename.
	var contentType, ext string
	switch report.Format {
	case "PDF":
		contentType = "application/pdf"
		ext = "pdf"
	case "CSV":
		contentType = "text/csv"
		ext = "csv"
	default:
		contentType = "application/octet-stream"
		ext = "bin"
	}

	filename := fmt.Sprintf("audit-report-%d-%s-to-%s.%s",
		report.ID,
		report.PeriodStart.UTC().Format("2006-01-02"),
		report.PeriodEnd.UTC().Format("2006-01-02"),
		ext,
	)

	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Header("Content-Length", strconv.FormatInt(int64(len(report.ArtifactData)), 10))
	c.Writer.Write(report.ArtifactData)
}

// handleDeleteAuditReport removes a report.
// DELETE /v1/audit/reports/:id
func (s *Server) handleDeleteAuditReport(c *gin.Context) {
	tenantID, ok := middleware.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	reportID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report id"})
		return
	}

	if err := s.auditSvc.Delete(c.Request.Context(), tenantID, uint(reportID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete report"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// parseDatetimeFlexible accepts YYYY-MM-DD, YYYY-MM-DDTHH:mm, or RFC3339.
// Returns the parsed time and whether the input included a time component.
func parseDatetimeFlexible(s string) (time.Time, bool, error) {
	// Try RFC3339 first (e.g. 2026-03-01T09:30:00Z)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true, nil
	}
	// Try datetime-local format (e.g. 2026-03-01T09:30)
	if t, err := time.Parse("2006-01-02T15:04", s); err == nil {
		return t, true, nil
	}
	// Try date-only (e.g. 2026-03-01)
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, false, nil
	}
	return time.Time{}, false, fmt.Errorf("unrecognized datetime format: %s", s)
}

// validateTimezone validates an IANA timezone string and returns the location.
func validateTimezone(tz string) (*time.Location, error) {
	return time.LoadLocation(tz)
}
