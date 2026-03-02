package models

import "time"

// AuditReport status constants.
const (
	ReportStatusQueued    = "QUEUED"
	ReportStatusRunning   = "RUNNING"
	ReportStatusCompleted = "COMPLETED"
	ReportStatusFailed    = "FAILED"
)

// AuditReportFilters holds optional filters applied when generating a report.
type AuditReportFilters struct {
	APIKeyIDs      []string `json:"api_key_ids,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	Models         []string `json:"models,omitempty"`
	APIUsageBilled *bool    `json:"api_usage_billed,omitempty"`
	ProjectIDs     []uint   `json:"project_ids,omitempty"`
	UserIDs        []string `json:"user_ids,omitempty"`
	BillingMode    string   `json:"billing_mode,omitempty"` // "api_usage" | "subscription" | ""
}

// AuditReport tracks an async report generation job and its resulting artifact.
type AuditReport struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	TenantID          uint      `gorm:"index" json:"tenant_id"`
	CreatedByUserID   string    `json:"created_by_user_id"`
	CreatedByEmail    string    `json:"created_by_email"`
	PeriodStart       time.Time `json:"period_start"`
	PeriodEnd         time.Time `json:"period_end"`
	FiltersJSON       string    `gorm:"type:jsonb" json:"filters"`
	Format            string    `json:"format"`                              // "PDF" | "CSV"
	Status            string    `gorm:"default:QUEUED" json:"status"`        // QUEUED | RUNNING | COMPLETED | FAILED
	ErrorMessage      string    `json:"error_message,omitempty"`
	ArtifactData      []byte    `gorm:"type:bytea" json:"-"`                 // never serialized in JSON responses
	ArtifactSizeBytes int64     `json:"artifact_size_bytes"`
	RowCount          int64     `json:"row_count"`
	GeneratedChecksum string    `json:"generated_checksum,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
