package services

import (
	"context"

	"gorm.io/gorm"

	"github.com/xiaoboyu/burnrate-ai/api-server/internal/models"
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
	var logs []models.UsageLog
	q := s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	return logs, q.Find(&logs).Error
}
