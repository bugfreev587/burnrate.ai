package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

// handleListAuditLogs returns audit log entries for the tenant.
// GET /v1/audit-logs
func (s *Server) handleListAuditLogs(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	tenantID, _ := middleware.GetTenantIDFromContext(c)

	// Hide super-admin audit events from non-super-admin users.
	isSuperAdmin := s.superAdminEmails[strings.ToLower(user.Email)]

	filter := services.AuditFilter{
		Action:         c.Query("action"),
		ResourceType:   c.Query("resource_type"),
		ResourceID:     c.Query("resource_id"),
		ActorUserID:    c.Query("actor_user_id"),
		Category:       c.Query("category"),
		ScopeProjectID: c.Query("scope_project_id"),
	}
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Offset = n
		}
	}
	if v := c.Query("start_date"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			filter.StartDate = &t
		}
	}
	if v := c.Query("end_date"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			end := t.Add(24*time.Hour - time.Nanosecond)
			filter.EndDate = &end
		}
	}

	if !isSuperAdmin {
		filter.ExcludeCategory = models.AuditCategoryAdmin
	}

	logs, total, err := s.auditLogSvc.List(c.Request.Context(), tenantID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list audit logs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"audit_logs": logs, "count": len(logs), "total": total})
}
