package api

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// recordAudit is a convenience wrapper that writes a single audit log entry.
// It extracts tenant ID, user ID, and IP from the gin context and logs a
// warning on failure (non-blocking so handlers are unaffected).
func (s *Server) recordAudit(c *gin.Context, action, resourceType, resourceID string) {
	tenantID, _ := middleware.GetTenantIDFromContext(c)
	var userID string
	if u, ok := middleware.GetUserFromContext(c); ok {
		userID = u.ID
	}

	err := s.auditLogSvc.Record(c.Request.Context(), models.AuditLog{
		TenantID:     tenantID,
		ActorUserID:  userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Success:      true,
		IPAddress:    c.ClientIP(),
	})
	if err != nil {
		slog.Warn("audit_log_record_failed",
			"action", action,
			"resource_type", resourceType,
			"resource_id", resourceID,
			"error", err,
		)
	}
}
