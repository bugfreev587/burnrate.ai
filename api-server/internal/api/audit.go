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
	PeriodStart    string   `json:"period_start" binding:"required"`    // YYYY-MM-DD
	PeriodEnd      string   `json:"period_end" binding:"required"`      // YYYY-MM-DD
	Format         string   `json:"format" binding:"required"`          // "PDF" | "CSV"
	Provider       string   `json:"provider,omitempty"`                 // optional filter
	APIKeyIDs      []string `json:"api_key_ids,omitempty"`              // optional filter
	APIUsageBilled *bool    `json:"api_usage_billed,omitempty"`         // optional filter
}

// handleCreateAuditReport creates a new audit report generation job.
// POST /v1/audit/reports
func (s *Server) handleCreateAuditReport(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Fetch tenant + plan limits.
	var tenant models.Tenant
	if err := s.postgresDB.GetDB().First(&tenant, user.TenantID).Error; err != nil {
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

	// Parse and validate dates.
	periodStart, err := time.Parse("2006-01-02", req.PeriodStart)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid period_start (expected YYYY-MM-DD)"})
		return
	}
	periodEnd, err := time.Parse("2006-01-02", req.PeriodEnd)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid period_end (expected YYYY-MM-DD)"})
		return
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

	// Check concurrent limit.
	pending, err := s.auditSvc.CountPending(c.Request.Context(), user.TenantID)
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
		APIKeyIDs:      req.APIKeyIDs,
		Provider:       req.Provider,
		APIUsageBilled: req.APIUsageBilled,
	}
	filtersJSON, _ := json.Marshal(filters)

	// Create the report row.
	startOfDay := time.Date(periodStart.Year(), periodStart.Month(), periodStart.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := time.Date(periodEnd.Year(), periodEnd.Month(), periodEnd.Day(), 23, 59, 59, 999999999, time.UTC)

	report := &models.AuditReport{
		TenantID:        user.TenantID,
		CreatedByUserID: user.ID,
		CreatedByEmail:  user.Email,
		PeriodStart:     startOfDay,
		PeriodEnd:       endOfDay,
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
