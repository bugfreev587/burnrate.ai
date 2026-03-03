package events

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/xiaoboyu/tokengate/api-server/internal/models"
	"github.com/xiaoboyu/tokengate/api-server/internal/services"
)

const reportConsumerGroup = "tokengate:report:workers"
const reportConsumerName = "report-worker-1"

// ReportWorker consumes report generation jobs from Redis Streams.
type ReportWorker struct {
	rdb      *redis.Client
	db       *gorm.DB
	auditSvc *services.AuditReportService
}

// NewReportWorker creates a new ReportWorker.
func NewReportWorker(rdb *redis.Client, db *gorm.DB, auditSvc *services.AuditReportService) *ReportWorker {
	return &ReportWorker{rdb: rdb, db: db, auditSvc: auditSvc}
}

// Run starts the Redis Streams consumer loop. It blocks until ctx is cancelled.
func (w *ReportWorker) Run(ctx context.Context) {
	err := w.rdb.XGroupCreateMkStream(ctx, reportStreamName, reportConsumerGroup, "$").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		slog.Error("reportworker_xgroup_create_failed", "error", err)
	}

	slog.Info("reportworker_started", "stream", reportStreamName, "group", reportConsumerGroup)

	for {
		select {
		case <-ctx.Done():
			slog.Info("reportworker_stopping", "reason", "context_cancelled")
			return
		default:
		}

		streams, err := w.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    reportConsumerGroup,
			Consumer: reportConsumerName,
			Streams:  []string{reportStreamName, ">"},
			Count:    1,
			Block:    5 * time.Second,
		}).Result()
		if err != nil {
			if err == redis.Nil || err.Error() == "redis: nil" {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			slog.Error("reportworker_xreadgroup_error", "error", err)
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				if err := w.processMessage(ctx, msg); err != nil {
					slog.Error("reportworker_process_failed", "msg_id", msg.ID, "error", err)
					continue
				}
				if err := w.rdb.XAck(ctx, reportStreamName, reportConsumerGroup, msg.ID).Err(); err != nil {
					slog.Error("reportworker_xack_failed", "msg_id", msg.ID, "error", err)
				}
			}
		}
	}
}

func (w *ReportWorker) processMessage(ctx context.Context, msg redis.XMessage) error {
	reportIDStr := fmt.Sprintf("%v", msg.Values["report_id"])
	reportID64, err := strconv.ParseUint(reportIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("parse report_id: %w", err)
	}
	return w.processReport(ctx, uint(reportID64))
}

func (w *ReportWorker) processReport(ctx context.Context, reportID uint) error {
	// Load the report.
	var report models.AuditReport
	if err := w.db.WithContext(ctx).First(&report, reportID).Error; err != nil {
		return fmt.Errorf("load report: %w", err)
	}

	// Set status to RUNNING.
	if err := w.auditSvc.UpdateStatus(ctx, reportID, models.ReportStatusRunning, ""); err != nil {
		return fmt.Errorf("set running: %w", err)
	}

	// Parse filters.
	var filters models.AuditReportFilters
	if report.FiltersJSON != "" && report.FiltersJSON != "{}" {
		if err := json.Unmarshal([]byte(report.FiltersJSON), &filters); err != nil {
			_ = w.auditSvc.UpdateStatus(ctx, reportID, models.ReportStatusFailed, "invalid filters: "+err.Error())
			return nil
		}
	}

	// Load timezone location.
	loc := time.UTC
	if report.Timezone != "" {
		if parsed, err := time.LoadLocation(report.Timezone); err == nil {
			loc = parsed
		}
	}

	var data []byte
	var rowCount int64
	var genErr error

	switch report.Format {
	case "CSV":
		data, rowCount, genErr = w.generateCSV(ctx, &report, &filters)
	case "PDF":
		data, rowCount, genErr = w.generatePDF(ctx, &report, &filters, loc)
	default:
		_ = w.auditSvc.UpdateStatus(ctx, reportID, models.ReportStatusFailed, "unsupported format: "+report.Format)
		return nil
	}

	if genErr != nil {
		_ = w.auditSvc.UpdateStatus(ctx, reportID, models.ReportStatusFailed, genErr.Error())
		return nil
	}

	// Enforce 10 MB cap.
	if len(data) > 10*1024*1024 {
		_ = w.auditSvc.UpdateStatus(ctx, reportID, models.ReportStatusFailed, "report exceeds 10 MB limit")
		return nil
	}

	// Compute SHA-256 checksum.
	hash := sha256.Sum256(data)
	checksum := hex.EncodeToString(hash[:])

	// Store artifact.
	if err := w.auditSvc.StoreArtifact(ctx, reportID, data, int64(len(data)), rowCount, checksum); err != nil {
		_ = w.auditSvc.UpdateStatus(ctx, reportID, models.ReportStatusFailed, "store artifact: "+err.Error())
		return nil
	}

	// Set completed.
	if err := w.auditSvc.UpdateStatus(ctx, reportID, models.ReportStatusCompleted, ""); err != nil {
		return fmt.Errorf("set completed: %w", err)
	}

	slog.Info("reportworker_report_completed", "report_id", reportID, "format", report.Format, "row_count", rowCount, "size_bytes", len(data))
	return nil
}

// usageRow is used to scan usage_log rows for report generation.
type usageRow struct {
	RequestID           string
	CreatedAt           time.Time
	Provider            string
	Model               string
	KeyID               string
	KeyLabel            string
	ProjectID           uint
	UserID              string
	PromptTokens        int64
	CompletionTokens    int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	ReasoningTokens     int64
	Cost                decimal.Decimal
	APIUsageBilled      bool
}

// buildQuery constructs the base GORM query for usage_logs with filters applied.
func (w *ReportWorker) buildQuery(ctx context.Context, report *models.AuditReport, filters *models.AuditReportFilters) *gorm.DB {
	return w.db.WithContext(ctx).
		Table("usage_logs").
		Select("usage_logs.request_id, usage_logs.created_at, usage_logs.provider, usage_logs.model, usage_logs.key_id, COALESCE(api_keys.label, '') as key_label, usage_logs.project_id, usage_logs.user_id, usage_logs.prompt_tokens, usage_logs.completion_tokens, usage_logs.cache_creation_tokens, usage_logs.cache_read_tokens, usage_logs.reasoning_tokens, usage_logs.cost, usage_logs.api_usage_billed").
		Joins("LEFT JOIN api_keys ON api_keys.key_id = usage_logs.key_id").
		Where("usage_logs.tenant_id = ?", report.TenantID).
		Where("usage_logs.created_at >= ? AND usage_logs.created_at <= ?", report.PeriodStart, report.PeriodEnd).
		Scopes(w.applyFilters(filters))
}

func (w *ReportWorker) generateCSV(ctx context.Context, report *models.AuditReport, filters *models.AuditReportFilters) ([]byte, int64, error) {
	var rows []usageRow
	q := w.buildQuery(ctx, report, filters).Order("usage_logs.created_at ASC")
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("query usage: %w", err)
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Header
	_ = writer.Write([]string{
		"request_id", "created_at", "provider", "model", "key_id", "key_label",
		"project_id", "user_id",
		"prompt_tokens", "completion_tokens", "cache_creation_tokens", "cache_read_tokens",
		"reasoning_tokens", "cost", "api_usage_billed",
	})

	for _, r := range rows {
		_ = writer.Write([]string{
			r.RequestID,
			r.CreatedAt.UTC().Format(time.RFC3339),
			r.Provider,
			r.Model,
			r.KeyID,
			r.KeyLabel,
			strconv.FormatUint(uint64(r.ProjectID), 10),
			r.UserID,
			strconv.FormatInt(r.PromptTokens, 10),
			strconv.FormatInt(r.CompletionTokens, 10),
			strconv.FormatInt(r.CacheCreationTokens, 10),
			strconv.FormatInt(r.CacheReadTokens, 10),
			strconv.FormatInt(r.ReasoningTokens, 10),
			r.Cost.StringFixed(6),
			strconv.FormatBool(r.APIUsageBilled),
		})
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, 0, fmt.Errorf("csv write: %w", err)
	}

	return buf.Bytes(), int64(len(rows)), nil
}

// aggregation types for PDF summary tables.
type modelAgg struct {
	Model        string
	Provider     string
	TotalCost    decimal.Decimal
	InputTokens  int64
	OutputTokens int64
	Requests     int64
}

type keyAgg struct {
	KeyID        string
	KeyLabel     string
	TotalCost    decimal.Decimal
	InputTokens  int64
	OutputTokens int64
	Requests     int64
}

type billingModeAgg struct {
	BillingMode string
	TotalCost   decimal.Decimal
	Requests    int64
}

type projectAgg struct {
	ProjectID   uint
	ProjectName string
	TotalCost   decimal.Decimal
	Requests    int64
}

type providerAgg struct {
	Provider     string
	TotalCost    decimal.Decimal
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	Requests     int64
}

type dailyAgg struct {
	Day       string
	TotalCost decimal.Decimal
	Requests  int64
}

type auditEventRow struct {
	ID          uint
	CreatedAt   time.Time
	Action      string
	Category    string
	ResourceType string
	ResourceID  string
	ActorUserID string
	Success     bool
	IPAddress   string
}

func (w *ReportWorker) generatePDF(ctx context.Context, report *models.AuditReport, filters *models.AuditReportFilters, loc *time.Location) ([]byte, int64, error) {
	// ── Tenant lookup ────────────────────────────────────────────────────
	var tenant models.Tenant
	w.db.WithContext(ctx).Select("id, name").First(&tenant, report.TenantID)
	tenantName := tenant.Name
	if tenantName == "" {
		tenantName = fmt.Sprintf("Tenant %d", report.TenantID)
	}

	// ── Aggregation queries ──────────────────────────────────────────────

	// Helper: base query with filters for usage_logs
	baseQuery := func() *gorm.DB {
		return w.db.WithContext(ctx).
			Table("usage_logs").
			Joins("LEFT JOIN api_keys ON api_keys.key_id = usage_logs.key_id").
			Where("usage_logs.tenant_id = ?", report.TenantID).
			Where("usage_logs.created_at >= ? AND usage_logs.created_at <= ?", report.PeriodStart, report.PeriodEnd).
			Scopes(w.applyFilters(filters))
	}

	// Summary totals (api_usage_billed only for cost)
	type summaryRow struct {
		TotalRequests  int64
		TotalInput     int64
		TotalOutput    int64
		TotalCost      decimal.Decimal
		UniqueKeys     int64
		UniqueUsers    int64
		UniqueProjects int64
	}
	var summary summaryRow
	if err := baseQuery().
		Select("COUNT(*) as total_requests, COALESCE(SUM(usage_logs.prompt_tokens),0) as total_input, COALESCE(SUM(usage_logs.completion_tokens),0) as total_output, COALESCE(SUM(CASE WHEN usage_logs.api_usage_billed THEN usage_logs.cost ELSE 0 END),0) as total_cost, COUNT(DISTINCT usage_logs.key_id) as unique_keys, COUNT(DISTINCT usage_logs.user_id) as unique_users, COUNT(DISTINCT usage_logs.project_id) as unique_projects").
		Scan(&summary).Error; err != nil {
		return nil, 0, fmt.Errorf("summary query: %w", err)
	}

	// By provider (NEW)
	var byProvider []providerAgg
	if err := baseQuery().
		Select("usage_logs.provider, COALESCE(SUM(usage_logs.cost),0) as total_cost, COALESCE(SUM(usage_logs.prompt_tokens),0) as input_tokens, COALESCE(SUM(usage_logs.completion_tokens),0) as output_tokens, COALESCE(SUM(usage_logs.prompt_tokens + usage_logs.completion_tokens),0) as total_tokens, COUNT(*) as requests").
		Group("usage_logs.provider").
		Order("total_cost DESC").
		Scan(&byProvider).Error; err != nil {
		return nil, 0, fmt.Errorf("by provider query: %w", err)
	}

	// By model
	var byModel []modelAgg
	if err := baseQuery().
		Select("usage_logs.model, usage_logs.provider, COALESCE(SUM(usage_logs.cost),0) as total_cost, COALESCE(SUM(usage_logs.prompt_tokens),0) as input_tokens, COALESCE(SUM(usage_logs.completion_tokens),0) as output_tokens, COUNT(*) as requests").
		Group("usage_logs.model, usage_logs.provider").
		Order("total_cost DESC").
		Scan(&byModel).Error; err != nil {
		return nil, 0, fmt.Errorf("by model query: %w", err)
	}

	// By API key
	var byKey []keyAgg
	if err := baseQuery().
		Select("usage_logs.key_id, COALESCE(api_keys.label, '') as key_label, COALESCE(SUM(usage_logs.cost),0) as total_cost, COALESCE(SUM(usage_logs.prompt_tokens),0) as input_tokens, COALESCE(SUM(usage_logs.completion_tokens),0) as output_tokens, COUNT(*) as requests").
		Group("usage_logs.key_id, api_keys.label").
		Order("total_cost DESC").
		Scan(&byKey).Error; err != nil {
		return nil, 0, fmt.Errorf("by key query: %w", err)
	}

	// By project (renamed from "Cost by Project")
	var byProject []projectAgg
	if err := w.db.WithContext(ctx).
		Table("usage_logs").
		Joins("LEFT JOIN api_keys ON api_keys.key_id = usage_logs.key_id").
		Joins("LEFT JOIN projects ON projects.id = usage_logs.project_id").
		Where("usage_logs.tenant_id = ?", report.TenantID).
		Where("usage_logs.created_at >= ? AND usage_logs.created_at <= ?", report.PeriodStart, report.PeriodEnd).
		Scopes(w.applyFilters(filters)).
		Select("usage_logs.project_id, COALESCE(projects.name, 'Unassigned') as project_name, COALESCE(SUM(usage_logs.cost),0) as total_cost, COUNT(*) as requests").
		Group("usage_logs.project_id, projects.name").
		Order("total_cost DESC").
		Scan(&byProject).Error; err != nil {
		return nil, 0, fmt.Errorf("by project query: %w", err)
	}

	// Cost by Billing Mode
	var byBilling []billingModeAgg
	if err := baseQuery().
		Select("CASE WHEN usage_logs.api_usage_billed THEN 'API Usage' ELSE 'Subscription' END as billing_mode, COALESCE(SUM(usage_logs.cost),0) as total_cost, COUNT(*) as requests").
		Group("usage_logs.api_usage_billed").
		Order("total_cost DESC").
		Scan(&byBilling).Error; err != nil {
		return nil, 0, fmt.Errorf("by billing mode query: %w", err)
	}

	// Daily Cost Rollup
	var daily []dailyAgg
	if err := baseQuery().
		Select("TO_CHAR(usage_logs.created_at, 'YYYY-MM-DD') as day, COALESCE(SUM(usage_logs.cost),0) as total_cost, COUNT(*) as requests").
		Group("TO_CHAR(usage_logs.created_at, 'YYYY-MM-DD')").
		Order("day ASC").
		Scan(&daily).Error; err != nil {
		return nil, 0, fmt.Errorf("daily rollup query: %w", err)
	}

	// Top Requests by Cost (conditional)
	var topByCost []usageRow
	if filters.IncludeTopRequestsByCost {
		limit := filters.TopRequestsLimit
		if limit <= 0 {
			limit = 10
		}
		if err := w.buildQuery(ctx, report, filters).
			Order("usage_logs.cost DESC").
			Limit(limit).
			Find(&topByCost).Error; err != nil {
			return nil, 0, fmt.Errorf("top by cost query: %w", err)
		}
	}

	// Security Events (from audit_logs, only tenant_id + date range, no usage filters)
	var securityEvents []auditEventRow
	if err := w.db.WithContext(ctx).
		Table("audit_logs").
		Where("tenant_id = ?", report.TenantID).
		Where("created_at >= ? AND created_at <= ?", report.PeriodStart, report.PeriodEnd).
		Where("category = ?", "ACCESS").
		Select("id, created_at, action, category, resource_type, resource_id, actor_user_id, success, ip_address").
		Order("created_at DESC").
		Limit(50).
		Scan(&securityEvents).Error; err != nil {
		return nil, 0, fmt.Errorf("security events query: %w", err)
	}

	// Admin & Configuration Actions (from audit_logs)
	var adminActions []auditEventRow
	if err := w.db.WithContext(ctx).
		Table("audit_logs").
		Where("tenant_id = ?", report.TenantID).
		Where("created_at >= ? AND created_at <= ?", report.PeriodStart, report.PeriodEnd).
		Where("category IN ?", []string{"ADMIN", "OWNER", "CONFIG", "TEAM"}).
		Select("id, created_at, action, category, resource_type, resource_id, actor_user_id, success, ip_address").
		Order("created_at DESC").
		Limit(50).
		Scan(&adminActions).Error; err != nil {
		return nil, 0, fmt.Errorf("admin actions query: %w", err)
	}

	// Recent Requests (conditional)
	var recentRequests []usageRow
	if filters.IncludeRecentRequests {
		limit := filters.RecentRequestsLimit
		if limit <= 0 {
			limit = 100
		}
		if err := w.buildQuery(ctx, report, filters).
			Order("usage_logs.created_at DESC").
			Limit(limit).
			Find(&recentRequests).Error; err != nil {
			return nil, 0, fmt.Errorf("recent requests query: %w", err)
		}
	}

	// ── Build PDF ────────────────────────────────────────────────────────
	cfg := config.NewBuilder().
		WithPageNumber().
		WithLeftMargin(15).
		WithRightMargin(15).
		WithTopMargin(15).
		Build()

	m := maroto.New(cfg)

	// Timezone display name
	tzDisplay := loc.String()

	// ── A. Cover Page ────────────────────────────────────────────────────
	m.AddRows(
		row.New(30).Add(
			col.New(12).Add(
				text.New("Audit Report", props.Text{
					Size:  24,
					Style: fontstyle.Bold,
					Align: align.Center,
					Top:   10,
				}),
			),
		),
	)
	m.AddRows(
		row.New(8).Add(
			col.New(12).Add(
				text.New(tenantName, props.Text{Size: 14, Align: align.Center, Style: fontstyle.Bold}),
			),
		),
	)
	m.AddRows(
		row.New(8).Add(
			col.New(12).Add(
				text.New(fmt.Sprintf("Period: %s to %s (%s)",
					report.PeriodStart.In(loc).Format("2006-01-02 15:04"),
					report.PeriodEnd.In(loc).Format("2006-01-02 15:04"),
					tzDisplay),
					props.Text{Size: 11, Align: align.Center}),
			),
		),
	)
	m.AddRows(
		row.New(6).Add(
			col.New(12).Add(
				text.New(fmt.Sprintf("Report ID: %d", report.ID),
					props.Text{Size: 9, Align: align.Center, Color: &props.Color{Red: 120, Green: 120, Blue: 120}}),
			),
		),
	)
	m.AddRows(
		row.New(6).Add(
			col.New(12).Add(
				text.New(fmt.Sprintf("Generated: %s", time.Now().In(loc).Format("2006-01-02 15:04 MST")),
					props.Text{Size: 9, Align: align.Center, Color: &props.Color{Red: 120, Green: 120, Blue: 120}}),
			),
		),
	)
	m.AddRows(
		row.New(6).Add(
			col.New(12).Add(
				text.New(fmt.Sprintf("Generated by: %s", report.CreatedByEmail),
					props.Text{Size: 9, Align: align.Center, Color: &props.Color{Red: 120, Green: 120, Blue: 120}}),
			),
		),
	)

	// ── B. Executive Summary ─────────────────────────────────────────────
	addSectionHeader(m, "Executive Summary")
	m.AddRows(
		row.New(7).Add(
			col.New(6).Add(text.New("Total Requests:", props.Text{Size: 10, Style: fontstyle.Bold})),
			col.New(6).Add(text.New(fmt.Sprintf("%d", summary.TotalRequests), props.Text{Size: 10})),
		),
		row.New(7).Add(
			col.New(6).Add(text.New("Total Input Tokens:", props.Text{Size: 10, Style: fontstyle.Bold})),
			col.New(6).Add(text.New(formatTokens(summary.TotalInput), props.Text{Size: 10})),
		),
		row.New(7).Add(
			col.New(6).Add(text.New("Total Output Tokens:", props.Text{Size: 10, Style: fontstyle.Bold})),
			col.New(6).Add(text.New(formatTokens(summary.TotalOutput), props.Text{Size: 10})),
		),
		row.New(7).Add(
			col.New(6).Add(text.New("Total Cost (billed):", props.Text{Size: 10, Style: fontstyle.Bold})),
			col.New(6).Add(text.New("$"+summary.TotalCost.StringFixed(4), props.Text{Size: 10})),
		),
		row.New(7).Add(
			col.New(6).Add(text.New("Unique API Keys:", props.Text{Size: 10, Style: fontstyle.Bold})),
			col.New(6).Add(text.New(fmt.Sprintf("%d", summary.UniqueKeys), props.Text{Size: 10})),
		),
		row.New(7).Add(
			col.New(6).Add(text.New("Unique Users:", props.Text{Size: 10, Style: fontstyle.Bold})),
			col.New(6).Add(text.New(fmt.Sprintf("%d", summary.UniqueUsers), props.Text{Size: 10})),
		),
		row.New(7).Add(
			col.New(6).Add(text.New("Unique Projects:", props.Text{Size: 10, Style: fontstyle.Bold})),
			col.New(6).Add(text.New(fmt.Sprintf("%d", summary.UniqueProjects), props.Text{Size: 10})),
		),
	)

	// ── C1. Usage by Provider ────────────────────────────────────────────
	if len(byProvider) > 0 {
		addSectionHeader(m, "Usage by Provider")
		m.AddRows(tableHeaderRow("Provider", "Cost", "Input Tokens", "Output Tokens", "Total Tokens", "Requests"))
		for _, r := range byProvider {
			m.AddRows(tableDataRow(
				r.Provider,
				"$"+r.TotalCost.StringFixed(4),
				formatTokens(r.InputTokens),
				formatTokens(r.OutputTokens),
				formatTokens(r.TotalTokens),
				fmt.Sprintf("%d", r.Requests),
			))
		}
	}

	// ── C2. Usage by Model ───────────────────────────────────────────────
	if len(byModel) > 0 {
		addSectionHeader(m, "Usage by Model")
		m.AddRows(tableHeaderRow("Model", "Provider", "Cost", "Input Tokens", "Output Tokens", "Requests"))
		for _, r := range byModel {
			m.AddRows(tableDataRow(
				r.Model, r.Provider,
				"$"+r.TotalCost.StringFixed(4),
				formatTokens(r.InputTokens),
				formatTokens(r.OutputTokens),
				fmt.Sprintf("%d", r.Requests),
			))
		}
	}

	// ── C3. Usage by API Key ─────────────────────────────────────────────
	if len(byKey) > 0 {
		addSectionHeader(m, "Usage by API Key")
		m.AddRows(tableHeaderRow("Key Label", "Key ID", "Cost", "Input Tokens", "Output Tokens", "Requests"))
		for _, r := range byKey {
			label := r.KeyLabel
			if label == "" {
				label = truncate(r.KeyID, 16)
			}
			m.AddRows(tableDataRow(
				label, truncate(r.KeyID, 16),
				"$"+r.TotalCost.StringFixed(4),
				formatTokens(r.InputTokens),
				formatTokens(r.OutputTokens),
				fmt.Sprintf("%d", r.Requests),
			))
		}
	}

	// ── C4. Usage by Project ─────────────────────────────────────────────
	if len(byProject) > 0 {
		addSectionHeader(m, "Usage by Project")
		m.AddRows(tableHeaderRow("Project", "Cost", "Requests"))
		for _, r := range byProject {
			name := r.ProjectName
			if name == "" {
				name = "Unassigned"
			}
			m.AddRows(tableDataRow(
				truncate(name, 30),
				"$"+r.TotalCost.StringFixed(4),
				fmt.Sprintf("%d", r.Requests),
			))
		}
	}

	// ── D1. Cost by Billing Mode ─────────────────────────────────────────
	if len(byBilling) > 0 {
		addSectionHeader(m, "Cost by Billing Mode")
		m.AddRows(tableHeaderRow("Billing Mode", "Cost", "Requests"))
		for _, r := range byBilling {
			m.AddRows(tableDataRow(
				r.BillingMode,
				"$"+r.TotalCost.StringFixed(4),
				fmt.Sprintf("%d", r.Requests),
			))
		}
	}

	// ── D2. Daily Cost Rollup ────────────────────────────────────────────
	if len(daily) > 0 {
		addSectionHeader(m, "Daily Cost Rollup")
		m.AddRows(tableHeaderRow("Date", "Cost", "Requests"))
		for _, r := range daily {
			m.AddRows(tableDataRow(
				r.Day,
				"$"+r.TotalCost.StringFixed(4),
				fmt.Sprintf("%d", r.Requests),
			))
		}
	}

	// ── D3. Top Requests by Cost (conditional) ───────────────────────────
	if len(topByCost) > 0 {
		addSectionHeader(m, fmt.Sprintf("Top Requests by Cost (top %d)", len(topByCost)))
		m.AddRows(
			row.New(7).Add(
				col.New(2).Add(text.New("Timestamp", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("API Key", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("Project", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("Provider", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(2).Add(text.New("Model", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("Tokens", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("Cost", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(3).Add(text.New("Request ID", props.Text{Size: 7, Style: fontstyle.Bold})),
			),
		)
		for _, r := range topByCost {
			label := r.KeyLabel
			if label == "" {
				label = truncate(r.KeyID, 10)
			}
			totalTokens := r.PromptTokens + r.CompletionTokens
			m.AddRows(
				row.New(6).Add(
					col.New(2).Add(text.New(r.CreatedAt.In(loc).Format("01-02 15:04"), props.Text{Size: 6})),
					col.New(1).Add(text.New(truncate(label, 10), props.Text{Size: 6})),
					col.New(1).Add(text.New(fmt.Sprintf("%d", r.ProjectID), props.Text{Size: 6})),
					col.New(1).Add(text.New(r.Provider, props.Text{Size: 6})),
					col.New(2).Add(text.New(truncate(r.Model, 18), props.Text{Size: 6})),
					col.New(1).Add(text.New(formatTokens(totalTokens), props.Text{Size: 6})),
					col.New(1).Add(text.New("$"+r.Cost.StringFixed(4), props.Text{Size: 6})),
					col.New(3).Add(text.New(truncate(r.RequestID, 28), props.Text{Size: 5})),
				),
			)
		}
	}

	// ── E1. Security Events ──────────────────────────────────────────────
	if len(securityEvents) > 0 {
		addSectionHeader(m, fmt.Sprintf("Security Events (latest %d)", len(securityEvents)))
		m.AddRows(
			row.New(7).Add(
				col.New(2).Add(text.New("Time", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(3).Add(text.New("Action", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(2).Add(text.New("Resource", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(2).Add(text.New("Actor", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("Result", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(2).Add(text.New("IP", props.Text{Size: 7, Style: fontstyle.Bold})),
			),
		)
		for _, e := range securityEvents {
			result := "OK"
			if !e.Success {
				result = "FAIL"
			}
			m.AddRows(
				row.New(6).Add(
					col.New(2).Add(text.New(e.CreatedAt.In(loc).Format("01-02 15:04"), props.Text{Size: 6})),
					col.New(3).Add(text.New(truncate(e.Action, 28), props.Text{Size: 6})),
					col.New(2).Add(text.New(truncate(e.ResourceType, 16), props.Text{Size: 6})),
					col.New(2).Add(text.New(truncate(e.ActorUserID, 16), props.Text{Size: 6})),
					col.New(1).Add(text.New(result, props.Text{Size: 6})),
					col.New(2).Add(text.New(e.IPAddress, props.Text{Size: 6})),
				),
			)
		}
	}

	// ── E2. Admin & Configuration Actions ────────────────────────────────
	if len(adminActions) > 0 {
		addSectionHeader(m, fmt.Sprintf("Admin & Configuration Actions (latest %d)", len(adminActions)))
		m.AddRows(
			row.New(7).Add(
				col.New(2).Add(text.New("Time", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(3).Add(text.New("Action", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(2).Add(text.New("Category", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(2).Add(text.New("Resource", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("Result", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(2).Add(text.New("Actor", props.Text{Size: 7, Style: fontstyle.Bold})),
			),
		)
		for _, e := range adminActions {
			result := "OK"
			if !e.Success {
				result = "FAIL"
			}
			m.AddRows(
				row.New(6).Add(
					col.New(2).Add(text.New(e.CreatedAt.In(loc).Format("01-02 15:04"), props.Text{Size: 6})),
					col.New(3).Add(text.New(truncate(e.Action, 28), props.Text{Size: 6})),
					col.New(2).Add(text.New(e.Category, props.Text{Size: 6})),
					col.New(2).Add(text.New(truncate(e.ResourceType, 16), props.Text{Size: 6})),
					col.New(1).Add(text.New(result, props.Text{Size: 6})),
					col.New(2).Add(text.New(truncate(e.ActorUserID, 16), props.Text{Size: 6})),
				),
			)
		}
	}

	// ── F. Recent Requests ──────────────────────────────────────────────
	if len(recentRequests) > 0 {
		addSectionHeader(m, fmt.Sprintf("Recent Requests (latest %d)", len(recentRequests)))
		m.AddRows(
			row.New(7).Add(
				col.New(2).Add(text.New("Timestamp", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("API Key", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("Provider", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(2).Add(text.New("Model", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("Input", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("Output", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("Cost", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(3).Add(text.New("Request ID", props.Text{Size: 7, Style: fontstyle.Bold})),
			),
		)
		for _, r := range recentRequests {
			label := r.KeyLabel
			if label == "" {
				label = truncate(r.KeyID, 10)
			}
			m.AddRows(
				row.New(6).Add(
					col.New(2).Add(text.New(r.CreatedAt.In(loc).Format("01-02 15:04"), props.Text{Size: 6})),
					col.New(1).Add(text.New(truncate(label, 10), props.Text{Size: 6})),
					col.New(1).Add(text.New(r.Provider, props.Text{Size: 6})),
					col.New(2).Add(text.New(truncate(r.Model, 18), props.Text{Size: 6})),
					col.New(1).Add(text.New(formatTokens(r.PromptTokens), props.Text{Size: 6})),
					col.New(1).Add(text.New(formatTokens(r.CompletionTokens), props.Text{Size: 6})),
					col.New(1).Add(text.New("$"+r.Cost.StringFixed(4), props.Text{Size: 6})),
					col.New(3).Add(text.New(truncate(r.RequestID, 28), props.Text{Size: 5})),
				),
			)
		}
	}

	// ── G. Definitions & Methodology ─────────────────────────────────────
	addSectionHeader(m, "Definitions & Methodology")
	addDefinitionRow(m, "API Usage Billed", "Requests billed directly via API usage metering (pay-per-token).")
	addDefinitionRow(m, "Subscription", "Requests covered under a monthly subscription plan allocation.")
	addDefinitionRow(m, "Input Tokens", "Tokens sent in the prompt (including system messages and context).")
	addDefinitionRow(m, "Output Tokens", "Tokens generated in the model's response (completion tokens).")
	addDefinitionRow(m, "Cost", "Computed cost in USD based on provider pricing at the time of the request.")
	m.AddRows(row.New(4)) // spacer
	addDefinitionRow(m, "Data Integrity", "This report includes a SHA-256 checksum for tamper detection. Cost figures are computed from provider-reported token counts and pricing at request time.")
	addDefinitionRow(m, "Security Events", "ACCESS-category audit log entries (API key/provider key creation, revocation, rotation).")
	addDefinitionRow(m, "Admin Actions", "ADMIN, OWNER, CONFIG, and TEAM-category audit log entries (role changes, settings, billing, team management).")

	// ── Compliance footer ────────────────────────────────────────────────
	m.AddRows(row.New(8)) // spacer
	m.AddRows(
		row.New(6).Add(
			col.New(12).Add(
				text.New("This report is generated for compliance and auditing purposes. All timestamps are displayed in "+tzDisplay+". Data integrity is ensured via SHA-256 checksum verification.",
					props.Text{Size: 7, Color: &props.Color{Red: 100, Green: 100, Blue: 100}}),
			),
		),
	)

	doc, err := m.Generate()
	if err != nil {
		return nil, 0, fmt.Errorf("generate pdf: %w", err)
	}

	pdfBytes := doc.GetBytes()
	return pdfBytes, summary.TotalRequests, nil
}

// applyFilters returns a GORM scope function that applies filter conditions.
func (w *ReportWorker) applyFilters(filters *models.AuditReportFilters) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if len(filters.APIKeyIDs) > 0 {
			db = db.Where("usage_logs.key_id IN ?", filters.APIKeyIDs)
		}
		if filters.Provider != "" {
			db = db.Where("usage_logs.provider = ?", filters.Provider)
		}
		if len(filters.Models) > 0 {
			db = db.Where("usage_logs.model IN ?", filters.Models)
		}
		if filters.APIUsageBilled != nil {
			db = db.Where("usage_logs.api_usage_billed = ?", *filters.APIUsageBilled)
		}
		if len(filters.ProjectIDs) > 0 {
			db = db.Where("usage_logs.project_id IN ?", filters.ProjectIDs)
		}
		if len(filters.UserIDs) > 0 {
			db = db.Where("usage_logs.user_id IN ?", filters.UserIDs)
		}
		if filters.BillingMode == "api_usage" {
			db = db.Where("usage_logs.api_usage_billed = ?", true)
		} else if filters.BillingMode == "subscription" {
			db = db.Where("usage_logs.api_usage_billed = ?", false)
		}
		return db
	}
}

// ── PDF helper functions ─────────────────────────────────────────────────────

func addSectionHeader(m core.Maroto, title string) {
	m.AddRows(row.New(5)) // spacer
	m.AddRows(
		row.New(8).Add(
			col.New(12).Add(
				text.New(title, props.Text{Size: 14, Style: fontstyle.Bold, Top: 2}),
			),
		),
	)
}

func tableHeaderRow(cols ...string) core.Row {
	components := make([]core.Col, len(cols))
	colSize := 12 / len(cols)
	for i, c := range cols {
		components[i] = col.New(colSize).Add(text.New(c, props.Text{Size: 8, Style: fontstyle.Bold}))
	}
	return row.New(7).Add(components...)
}

func tableDataRow(cols ...string) core.Row {
	components := make([]core.Col, len(cols))
	colSize := 12 / len(cols)
	for i, c := range cols {
		components[i] = col.New(colSize).Add(text.New(c, props.Text{Size: 8}))
	}
	return row.New(6).Add(components...)
}

func formatTokens(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "\u2026"
}

func addDefinitionRow(m core.Maroto, term, definition string) {
	m.AddRows(
		row.New(7).Add(
			col.New(3).Add(text.New(term, props.Text{Size: 8, Style: fontstyle.Bold})),
			col.New(9).Add(text.New(definition, props.Text{Size: 8})),
		),
	)
}
