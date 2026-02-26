package events

import (
	"bytes"
	"context"
	"encoding/csv"
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

	var data []byte
	var rowCount int64
	var genErr error

	switch report.Format {
	case "CSV":
		data, rowCount, genErr = w.generateCSV(ctx, &report, &filters)
	case "PDF":
		data, rowCount, genErr = w.generatePDF(ctx, &report, &filters)
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

	// Store artifact.
	if err := w.auditSvc.StoreArtifact(ctx, reportID, data, int64(len(data)), rowCount); err != nil {
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
	q := w.db.WithContext(ctx).
		Table("usage_logs").
		Select("usage_logs.request_id, usage_logs.created_at, usage_logs.provider, usage_logs.model, usage_logs.key_id, COALESCE(api_keys.label, '') as key_label, usage_logs.prompt_tokens, usage_logs.completion_tokens, usage_logs.cache_creation_tokens, usage_logs.cache_read_tokens, usage_logs.reasoning_tokens, usage_logs.cost, usage_logs.api_usage_billed").
		Joins("LEFT JOIN api_keys ON api_keys.key_id = usage_logs.key_id").
		Where("usage_logs.tenant_id = ?", report.TenantID).
		Where("usage_logs.created_at >= ? AND usage_logs.created_at <= ?", report.PeriodStart, report.PeriodEnd)

	if len(filters.APIKeyIDs) > 0 {
		q = q.Where("usage_logs.key_id IN ?", filters.APIKeyIDs)
	}
	if filters.Provider != "" {
		q = q.Where("usage_logs.provider = ?", filters.Provider)
	}
	if len(filters.Models) > 0 {
		q = q.Where("usage_logs.model IN ?", filters.Models)
	}
	if filters.APIUsageBilled != nil {
		q = q.Where("usage_logs.api_usage_billed = ?", *filters.APIUsageBilled)
	}

	return q
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

func (w *ReportWorker) generatePDF(ctx context.Context, report *models.AuditReport, filters *models.AuditReportFilters) ([]byte, int64, error) {
	// ── Aggregation queries ──────────────────────────────────────────────

	// Summary totals (api_usage_billed only for cost)
	type summaryRow struct {
		TotalRequests int64
		TotalInput    int64
		TotalOutput   int64
		TotalCost     decimal.Decimal
	}
	var summary summaryRow
	if err := w.db.WithContext(ctx).
		Table("usage_logs").
		Joins("LEFT JOIN api_keys ON api_keys.key_id = usage_logs.key_id").
		Where("usage_logs.tenant_id = ?", report.TenantID).
		Where("usage_logs.created_at >= ? AND usage_logs.created_at <= ?", report.PeriodStart, report.PeriodEnd).
		Scopes(w.applyFilters(filters)).
		Select("COUNT(*) as total_requests, COALESCE(SUM(usage_logs.prompt_tokens),0) as total_input, COALESCE(SUM(usage_logs.completion_tokens),0) as total_output, COALESCE(SUM(CASE WHEN usage_logs.api_usage_billed THEN usage_logs.cost ELSE 0 END),0) as total_cost").
		Scan(&summary).Error; err != nil {
		return nil, 0, fmt.Errorf("summary query: %w", err)
	}

	// By model
	var byModel []modelAgg
	if err := w.db.WithContext(ctx).
		Table("usage_logs").
		Joins("LEFT JOIN api_keys ON api_keys.key_id = usage_logs.key_id").
		Where("usage_logs.tenant_id = ?", report.TenantID).
		Where("usage_logs.created_at >= ? AND usage_logs.created_at <= ?", report.PeriodStart, report.PeriodEnd).
		Scopes(w.applyFilters(filters)).
		Select("usage_logs.model, usage_logs.provider, COALESCE(SUM(usage_logs.cost),0) as total_cost, COALESCE(SUM(usage_logs.prompt_tokens),0) as input_tokens, COALESCE(SUM(usage_logs.completion_tokens),0) as output_tokens, COUNT(*) as requests").
		Group("usage_logs.model, usage_logs.provider").
		Order("total_cost DESC").
		Scan(&byModel).Error; err != nil {
		return nil, 0, fmt.Errorf("by model query: %w", err)
	}

	// By API key
	var byKey []keyAgg
	if err := w.db.WithContext(ctx).
		Table("usage_logs").
		Joins("LEFT JOIN api_keys ON api_keys.key_id = usage_logs.key_id").
		Where("usage_logs.tenant_id = ?", report.TenantID).
		Where("usage_logs.created_at >= ? AND usage_logs.created_at <= ?", report.PeriodStart, report.PeriodEnd).
		Scopes(w.applyFilters(filters)).
		Select("usage_logs.key_id, COALESCE(api_keys.label, '') as key_label, COALESCE(SUM(usage_logs.cost),0) as total_cost, COALESCE(SUM(usage_logs.prompt_tokens),0) as input_tokens, COALESCE(SUM(usage_logs.completion_tokens),0) as output_tokens, COUNT(*) as requests").
		Group("usage_logs.key_id, api_keys.label").
		Order("total_cost DESC").
		Scan(&byKey).Error; err != nil {
		return nil, 0, fmt.Errorf("by key query: %w", err)
	}

	// Recent requests (top 100)
	var recent []usageRow
	if err := w.buildQuery(ctx, report, filters).
		Order("usage_logs.created_at DESC").
		Limit(100).
		Find(&recent).Error; err != nil {
		return nil, 0, fmt.Errorf("recent query: %w", err)
	}

	// ── Build PDF ────────────────────────────────────────────────────────
	cfg := config.NewBuilder().
		WithPageNumber().
		WithLeftMargin(15).
		WithRightMargin(15).
		WithTopMargin(15).
		Build()

	m := maroto.New(cfg)

	// Cover page
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
				text.New(fmt.Sprintf("Period: %s to %s",
					report.PeriodStart.UTC().Format("2006-01-02"),
					report.PeriodEnd.UTC().Format("2006-01-02")),
					props.Text{Size: 11, Align: align.Center}),
			),
		),
	)
	m.AddRows(
		row.New(6).Add(
			col.New(12).Add(
				text.New(fmt.Sprintf("Generated: %s", time.Now().UTC().Format("2006-01-02 15:04 UTC")),
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

	// Summary section
	addSectionHeader(m, "Summary")
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
	)

	// By Model table
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

	// By API Key table
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

	// Recent Requests (top 100)
	if len(recent) > 0 {
		addSectionHeader(m, fmt.Sprintf("Recent Requests (top %d)", len(recent)))
		// Narrower columns for recent requests
		m.AddRows(
			row.New(7).Add(
				col.New(2).Add(text.New("Timestamp", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(2).Add(text.New("Provider", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(2).Add(text.New("Model", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(2).Add(text.New("Key Label", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("In Tokens", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(1).Add(text.New("Out Tokens", props.Text{Size: 7, Style: fontstyle.Bold})),
				col.New(2).Add(text.New("Cost", props.Text{Size: 7, Style: fontstyle.Bold})),
			),
		)
		for _, r := range recent {
			label := r.KeyLabel
			if label == "" {
				label = truncate(r.KeyID, 12)
			}
			m.AddRows(
				row.New(6).Add(
					col.New(2).Add(text.New(r.CreatedAt.UTC().Format("01-02 15:04"), props.Text{Size: 6})),
					col.New(2).Add(text.New(r.Provider, props.Text{Size: 6})),
					col.New(2).Add(text.New(truncate(r.Model, 20), props.Text{Size: 6})),
					col.New(2).Add(text.New(truncate(label, 12), props.Text{Size: 6})),
					col.New(1).Add(text.New(formatTokens(r.PromptTokens), props.Text{Size: 6})),
					col.New(1).Add(text.New(formatTokens(r.CompletionTokens), props.Text{Size: 6})),
					col.New(2).Add(text.New("$"+r.Cost.StringFixed(4), props.Text{Size: 6})),
				),
			)
		}
	}

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
