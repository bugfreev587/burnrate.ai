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
	q := s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC")
	if since != nil {
		q = q.Where("created_at >= ?", *since)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&logs).Error; err != nil {
		return nil, err
	}
	s.populateKeyLabels(logs)
	return logs, nil
}

// ListByTenantBetween lists usage logs for a tenant between two timestamps (inclusive).
func (s *UsageLogService) ListByTenantBetween(ctx context.Context, tenantID uint, limit int, from, to time.Time) ([]models.UsageLog, error) {
	var logs []models.UsageLog
	q := s.db.Where("tenant_id = ? AND created_at >= ? AND created_at <= ?", tenantID, from, to).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&logs).Error; err != nil {
		return nil, err
	}
	s.populateKeyLabels(logs)
	return logs, nil
}

// populateKeyLabels looks up API key labels for the key_ids present in the
// given logs and sets the KeyLabel field on each entry.
func (s *UsageLogService) populateKeyLabels(logs []models.UsageLog) {
	if len(logs) == 0 {
		return
	}

	// Collect unique key_ids.
	seen := make(map[string]struct{})
	for _, l := range logs {
		if l.KeyID != "" {
			seen[l.KeyID] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return
	}

	keyIDs := make([]string, 0, len(seen))
	for k := range seen {
		keyIDs = append(keyIDs, k)
	}

	// Query labels from api_keys table.
	var keys []models.APIKey
	s.db.Select("key_id", "label").Where("key_id IN ?", keyIDs).Find(&keys)

	labelMap := make(map[string]string, len(keys))
	for _, k := range keys {
		labelMap[k.KeyID] = k.Label
	}

	// Assign labels to log entries.
	for i := range logs {
		if label, ok := labelMap[logs[i].KeyID]; ok {
			logs[i].KeyLabel = label
		}
	}
}
