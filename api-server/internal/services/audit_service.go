package services

import (
	"context"

	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// AuditReportService provides CRUD operations for audit reports.
type AuditReportService struct {
	db *gorm.DB
}

// NewAuditReportService creates a new AuditReportService.
func NewAuditReportService(db *gorm.DB) *AuditReportService {
	return &AuditReportService{db: db}
}

// Create inserts a new audit report row.
func (s *AuditReportService) Create(ctx context.Context, report *models.AuditReport) error {
	return s.db.WithContext(ctx).Create(report).Error
}

// GetByID loads a report by ID (scoped to tenant), excluding the artifact data.
func (s *AuditReportService) GetByID(ctx context.Context, tenantID, reportID uint) (*models.AuditReport, error) {
	var report models.AuditReport
	err := s.db.WithContext(ctx).
		Select("id, tenant_id, created_by_user_id, created_by_email, period_start, period_end, filters_json, format, status, error_message, artifact_size_bytes, row_count, created_at, updated_at").
		Where("id = ? AND tenant_id = ?", reportID, tenantID).
		First(&report).Error
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// GetWithArtifact loads a report including the artifact data (for download).
func (s *AuditReportService) GetWithArtifact(ctx context.Context, tenantID, reportID uint) (*models.AuditReport, error) {
	var report models.AuditReport
	err := s.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", reportID, tenantID).
		First(&report).Error
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// ListByTenant returns reports for a tenant (most recent first), excluding artifact data.
func (s *AuditReportService) ListByTenant(ctx context.Context, tenantID uint, limit int) ([]models.AuditReport, error) {
	var reports []models.AuditReport
	q := s.db.WithContext(ctx).
		Select("id, tenant_id, created_by_user_id, created_by_email, period_start, period_end, filters_json, format, status, error_message, artifact_size_bytes, row_count, created_at, updated_at").
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&reports).Error; err != nil {
		return nil, err
	}
	return reports, nil
}

// UpdateStatus sets the status (and optional error message) of a report.
func (s *AuditReportService) UpdateStatus(ctx context.Context, reportID uint, status string, errMsg string) error {
	return s.db.WithContext(ctx).
		Model(&models.AuditReport{}).
		Where("id = ?", reportID).
		Updates(map[string]interface{}{
			"status":        status,
			"error_message": errMsg,
		}).Error
}

// StoreArtifact saves the generated report artifact data.
func (s *AuditReportService) StoreArtifact(ctx context.Context, reportID uint, data []byte, size int64, rowCount int64) error {
	return s.db.WithContext(ctx).
		Model(&models.AuditReport{}).
		Where("id = ?", reportID).
		Updates(map[string]interface{}{
			"artifact_data":       data,
			"artifact_size_bytes": size,
			"row_count":           rowCount,
		}).Error
}

// Delete removes a report by ID (scoped to tenant).
func (s *AuditReportService) Delete(ctx context.Context, tenantID, reportID uint) error {
	return s.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", reportID, tenantID).
		Delete(&models.AuditReport{}).Error
}

// CountPending returns the number of QUEUED or RUNNING reports for a tenant.
func (s *AuditReportService) CountPending(ctx context.Context, tenantID uint) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&models.AuditReport{}).
		Where("tenant_id = ? AND status IN ?", tenantID, []string{models.ReportStatusQueued, models.ReportStatusRunning}).
		Count(&count).Error
	return count, err
}
