package services

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

type UsageLogService struct {
	db *gorm.DB
}

func NewUsageLogService(db *gorm.DB) *UsageLogService {
	return &UsageLogService{db: db}
}

func (s *UsageLogService) Create(ctx context.Context, log *models.UsageLog) error {
	return s.db.Create(log).Error
}

func (s *UsageLogService) ListByTenant(ctx context.Context, tenantID uint, limit int) ([]models.UsageLog, error) {
	return s.ListByTenantSince(ctx, tenantID, limit, nil)
}

// ListByTenantSince lists usage logs for a tenant. If since is non-nil, only logs
// created at or after that time are returned (used for plan-based data retention).
func (s *UsageLogService) ListByTenantSince(ctx context.Context, tenantID uint, limit int, since *time.Time) ([]models.UsageLog, error) {
	var logs []models.UsageLog
	q := s.db.Where("tenant_id = ? AND api_usage_billed = ?", tenantID, true).Order("created_at DESC")
	if since != nil {
		q = q.Where("created_at >= ?", *since)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	return logs, q.Find(&logs).Error
}

// ListByTenantBetween lists usage logs for a tenant between two timestamps (inclusive).
func (s *UsageLogService) ListByTenantBetween(ctx context.Context, tenantID uint, limit int, from, to time.Time) ([]models.UsageLog, error) {
	var logs []models.UsageLog
	q := s.db.Where("tenant_id = ? AND api_usage_billed = ? AND created_at >= ? AND created_at <= ?", tenantID, true, from, to).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	return logs, q.Find(&logs).Error
}
