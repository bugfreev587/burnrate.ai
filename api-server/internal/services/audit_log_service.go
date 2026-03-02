package services

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// AuditLogService records and queries mutation audit logs.
type AuditLogService struct {
	db *gorm.DB
}

// NewAuditLogService creates a new AuditLogService.
func NewAuditLogService(db *gorm.DB) *AuditLogService {
	return &AuditLogService{db: db}
}

// Record persists a single audit log entry.
func (s *AuditLogService) Record(ctx context.Context, entry models.AuditLog) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	return s.db.WithContext(ctx).Create(&entry).Error
}

// AuditFilter specifies optional filters for listing audit logs.
type AuditFilter struct {
	Action          string
	ResourceType    string
	ResourceID      string
	ActorUserID     string
	Category        string
	ExcludeCategory string
	ScopeProjectID  string
	StartDate       *time.Time
	EndDate         *time.Time
	Limit           int
	Offset          int
}

// List returns audit log entries for a tenant with optional filters.
// Returns the matching logs, total count (before limit/offset), and any error.
func (s *AuditLogService) List(ctx context.Context, tenantID uint, filter AuditFilter) ([]models.AuditLog, int64, error) {
	q := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if filter.Action != "" {
		q = q.Where("action = ?", filter.Action)
	}
	if filter.ResourceType != "" {
		q = q.Where("resource_type = ?", filter.ResourceType)
	}
	if filter.ResourceID != "" {
		q = q.Where("resource_id = ?", filter.ResourceID)
	}
	if filter.ActorUserID != "" {
		q = q.Where("actor_user_id = ?", filter.ActorUserID)
	}
	if filter.ScopeProjectID != "" {
		q = q.Where("(resource_type = 'project' AND resource_id = ?) OR (resource_type = 'project_membership' AND resource_id LIKE ?)", filter.ScopeProjectID, filter.ScopeProjectID+"/%")
	}
	if filter.Category != "" {
		q = q.Where("category = ?", filter.Category)
	}
	if filter.ExcludeCategory != "" {
		q = q.Where("category != ? OR category IS NULL", filter.ExcludeCategory)
	}
	if filter.StartDate != nil {
		q = q.Where("created_at >= ?", *filter.StartDate)
	}
	if filter.EndDate != nil {
		q = q.Where("created_at <= ?", *filter.EndDate)
	}

	var total int64
	if err := q.Model(&models.AuditLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q = q.Order("created_at DESC").Limit(limit)
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}

	var logs []models.AuditLog
	if err := q.Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}
