package api

import (
	"encoding/json"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/xiaoboyu/tokengate/api-server/internal/middleware"
	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// AuditOpts holds optional parameters for recording an audit event.
type AuditOpts struct {
	Category    string
	ActorType   string
	BeforeState map[string]interface{}
	AfterState  map[string]interface{}
	Metadata    map[string]interface{}
	Success     *bool
}

// recordAuditEvent writes a rich audit log entry with optional metadata,
// before/after state, category, and actor type.
func (s *Server) recordAuditEvent(c *gin.Context, action, resourceType, resourceID string, opts AuditOpts) {
	tenantID, _ := middleware.GetTenantIDFromContext(c)
	var userID string
	if u, ok := middleware.GetUserFromContext(c); ok {
		userID = u.ID
	}

	actorType := opts.ActorType
	if actorType == "" {
		actorType = models.AuditActorUser
	}

	success := true
	if opts.Success != nil {
		success = *opts.Success
	}

	beforeJSON := "{}"
	if opts.BeforeState != nil {
		if b, err := json.Marshal(opts.BeforeState); err == nil {
			beforeJSON = string(b)
		}
	}

	afterJSON := "{}"
	if opts.AfterState != nil {
		if b, err := json.Marshal(opts.AfterState); err == nil {
			afterJSON = string(b)
		}
	}

	metadata := "{}"
	if opts.Metadata != nil {
		if b, err := json.Marshal(opts.Metadata); err == nil {
			metadata = string(b)
		}
	}

	err := s.auditLogSvc.Record(c.Request.Context(), models.AuditLog{
		TenantID:    tenantID,
		ActorUserID: userID,
		Action:      action,
		ResourceType: resourceType,
		ResourceID:  resourceID,
		Category:    opts.Category,
		ActorType:   actorType,
		UserAgent:   c.Request.UserAgent(),
		Success:     success,
		IPAddress:   c.ClientIP(),
		BeforeJSON:  beforeJSON,
		AfterJSON:   afterJSON,
		Metadata:    metadata,
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
