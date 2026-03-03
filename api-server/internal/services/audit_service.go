package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
)

// AuditReportService provides CRUD operations for audit reports.
type AuditReportService struct {
	db       *gorm.DB
	objStore *ObjectStore
}

// NewAuditReportService creates a new AuditReportService.
func NewAuditReportService(db *gorm.DB, objStore *ObjectStore) *AuditReportService {
	return &AuditReportService{db: db, objStore: objStore}
}

// Create inserts a new audit report row.
func (s *AuditReportService) Create(ctx context.Context, report *models.AuditReport) error {
	return s.db.WithContext(ctx).Create(report).Error
}

// GetByID loads a report by ID (scoped to tenant), excluding the artifact data.
func (s *AuditReportService) GetByID(ctx context.Context, tenantID, reportID uint) (*models.AuditReport, error) {
	var report models.AuditReport
	err := s.db.WithContext(ctx).
		Select("id, tenant_id, created_by_user_id, created_by_email, period_start, period_end, timezone, filters_json, format, status, error_message, artifact_size_bytes, row_count, generated_checksum, created_at, updated_at").
		Where("id = ? AND tenant_id = ?", reportID, tenantID).
		First(&report).Error
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// GetWithArtifact loads a report including the artifact data (for download).
// Downloads the artifact from R2; falls back to the DB blob for legacy reports.
func (s *AuditReportService) GetWithArtifact(ctx context.Context, tenantID, reportID uint) (*models.AuditReport, error) {
	var report models.AuditReport
	err := s.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", reportID, tenantID).
		First(&report).Error
	if err != nil {
		return nil, err
	}
	if report.ArtifactKey != "" {
		data, err := s.objStore.Download(ctx, report.ArtifactKey)
		if err != nil {
			return nil, fmt.Errorf("download artifact from R2: %w", err)
		}
		report.ArtifactData = data
	}
	return &report, nil
}

// GetDownloadURL returns a short-lived presigned URL for downloading the report
// artifact directly from R2, bypassing the API server.
func (s *AuditReportService) GetDownloadURL(ctx context.Context, tenantID, reportID uint) (string, error) {
	var report models.AuditReport
	err := s.db.WithContext(ctx).
		Select("id, tenant_id, artifact_key, status").
		Where("id = ? AND tenant_id = ?", reportID, tenantID).
		First(&report).Error
	if err != nil {
		return "", err
	}
	if report.ArtifactKey == "" {
		return "", fmt.Errorf("report %d has no R2 artifact", reportID)
	}
	return s.objStore.PresignedGetURL(ctx, report.ArtifactKey, 15*time.Minute)
}

// ListByTenant returns reports for a tenant (most recent first), excluding artifact data.
func (s *AuditReportService) ListByTenant(ctx context.Context, tenantID uint, limit int) ([]models.AuditReport, error) {
	var reports []models.AuditReport
	q := s.db.WithContext(ctx).
		Select("id, tenant_id, created_by_user_id, created_by_email, period_start, period_end, timezone, filters_json, format, status, error_message, artifact_size_bytes, row_count, generated_checksum, created_at, updated_at").
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

// StoreArtifact saves the generated report artifact data to R2.
// Objects are stored under {tenant_id}/audit_report_{YYYYMMDD_HHmmss}.{format}.
func (s *AuditReportService) StoreArtifact(ctx context.Context, reportID uint, data []byte, size int64, rowCount int64, checksum string) error {
	var report models.AuditReport
	if err := s.db.WithContext(ctx).Select("id, tenant_id, format, created_at").First(&report, reportID).Error; err != nil {
		return fmt.Errorf("load report for R2 upload: %w", err)
	}
	ext := strings.ToLower(report.Format)
	ts := report.CreatedAt.UTC().Format("20060102_150405")
	key := fmt.Sprintf("%d/audit_report_%s.%s", report.TenantID, ts, ext)
	if err := s.objStore.Upload(ctx, key, data); err != nil {
		return fmt.Errorf("upload artifact to R2: %w", err)
	}
	slog.Info("artifact uploaded to R2", "report_id", reportID, "key", key, "size", size)
	return s.db.WithContext(ctx).
		Model(&models.AuditReport{}).
		Where("id = ?", reportID).
		Updates(map[string]interface{}{
			"artifact_key":        key,
			"artifact_size_bytes": size,
			"row_count":           rowCount,
			"generated_checksum":  checksum,
		}).Error
}

// Delete removes a report by ID (scoped to tenant) and its R2 artifact.
func (s *AuditReportService) Delete(ctx context.Context, tenantID, reportID uint) error {
	var report models.AuditReport
	if err := s.db.WithContext(ctx).Select("id, artifact_key").Where("id = ? AND tenant_id = ?", reportID, tenantID).First(&report).Error; err == nil && report.ArtifactKey != "" {
		if err := s.objStore.Delete(ctx, report.ArtifactKey); err != nil {
			slog.Warn("failed to delete R2 artifact", "report_id", reportID, "key", report.ArtifactKey, "error", err)
		}
	}
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
