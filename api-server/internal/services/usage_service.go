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

func (s *UsageLogService) ListByUser(ctx context.Context, userID string, limit int) ([]models.UsageLog, error) {
	var logs []models.UsageLog
	q := s.db.Where("user_id = ?", userID).Order("requested_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	return logs, q.Find(&logs).Error
}
