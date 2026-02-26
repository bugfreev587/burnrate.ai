import { useState, useEffect, useRef } from 'react'
import { useUserSync, hasPermission } from '../hooks/useUserSync'
import { useDashboardConfig } from '../hooks/useDashboardConfig'
import { useAuditReports } from '../hooks/useAuditReports'
import type { CreateReportRequest } from '../hooks/useAuditReports'
import Navbar from '../components/Navbar'
import './AuditPage.css'

export default function AuditPage() {
  const { role, isSynced } = useUserSync()
  const { config } = useDashboardConfig()
  const { reports, loading, error, refresh, generate, deleteReport, downloadReport } = useAuditReports()

  const isAdmin = isSynced && hasPermission(role, 'admin')
  const plan = config?.plan || 'free'
  const canExport = plan !== 'free'

  // ── Filters ──────────────────────────────────────────────────────────
  const today = new Date()
  const todayStr = today.getFullYear() + '-' + String(today.getMonth() + 1).padStart(2, '0') + '-' + String(today.getDate()).padStart(2, '0')
  const defaultStart = new Date(today)
  defaultStart.setDate(defaultStart.getDate() - 29)
  const defaultStartStr = defaultStart.getFullYear() + '-' + String(defaultStart.getMonth() + 1).padStart(2, '0') + '-' + String(defaultStart.getDate()).padStart(2, '0')

  const [startDate, setStartDate] = useState(defaultStartStr)
  const [endDate, setEndDate] = useState(todayStr)
  const [provider, setProvider] = useState('')
  const [billedOnly, setBilledOnly] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [flashMsg, setFlashMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const effectiveMin = config?.effective.min_start_date ?? '2026-01-01'

  // ── Auto-poll when reports are pending ────────────────────────────
  const hasPending = reports.some(r => r.status === 'QUEUED' || r.status === 'RUNNING')
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    if (hasPending) {
      pollRef.current = setInterval(() => refresh(), 5000)
    }
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [hasPending, refresh])

  // ── Generate ─────────────────────────────────────────────────────────
  const handleGenerate = async (format: 'PDF' | 'CSV') => {
    if (!canExport || !isAdmin) return
    setGenerating(true)
    setFlashMsg(null)
    try {
      const req: CreateReportRequest = {
        period_start: startDate,
        period_end: endDate,
        format,
      }
      if (provider) req.provider = provider
      if (billedOnly) req.api_usage_billed = true
      await generate(req)
      setFlashMsg({ type: 'success', text: `${format} report queued successfully` })
    } catch (e: unknown) {
      setFlashMsg({ type: 'error', text: e instanceof Error ? e.message : 'Failed to generate report' })
    } finally {
      setGenerating(false)
    }
  }

  // ── Download ─────────────────────────────────────────────────────────
  const handleDownload = async (id: number, format: string) => {
    try {
      await downloadReport(id, format)
    } catch (e: unknown) {
      setFlashMsg({ type: 'error', text: e instanceof Error ? e.message : 'Download failed' })
    }
  }

  // ── Delete ───────────────────────────────────────────────────────────
  const handleDelete = async (id: number) => {
    if (!confirm('Delete this report?')) return
    const ok = await deleteReport(id)
    if (ok) {
      setFlashMsg({ type: 'success', text: 'Report deleted' })
    } else {
      setFlashMsg({ type: 'error', text: 'Failed to delete report' })
    }
  }

  // ── Format helpers ───────────────────────────────────────────────────
  function formatSize(bytes: number): string {
    if (bytes === 0) return '—'
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  }

  function formatDate(iso: string): string {
    const d = new Date(iso)
    return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
  }

  function formatDateTime(iso: string): string {
    const d = new Date(iso)
    return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  }

  // ── Loading ──────────────────────────────────────────────────────────
  if (!isSynced) {
    return (
      <div className="page-container">
        <Navbar />
        <div className="page-content">
          <div className="loading-center"><div className="spinner" /></div>
        </div>
      </div>
    )
  }

  return (
    <div className="page-container">
      <Navbar />
      <div className="page-content">
        <div className="audit-container">

          {/* Header */}
          <div className="audit-header">
            <h1>Audit Reports</h1>
            <button className="btn btn-secondary" onClick={refresh} disabled={loading}>
              {loading ? 'Refreshing...' : 'Refresh'}
            </button>
          </div>

          {/* Flash */}
          {flashMsg && (
            <div className={`flash flash-${flashMsg.type}`}>{flashMsg.text}</div>
          )}
          {error && <div className="flash flash-error">{error}</div>}

          {/* Filters */}
          <section className="audit-section">
            <h2>Generate Report</h2>
            <div className="audit-filters">
              <div className="audit-filter-row">
                <label className="audit-filter-label">
                  Start Date
                  <input
                    type="date"
                    className="audit-date-input"
                    value={startDate}
                    min={effectiveMin}
                    max={endDate}
                    onChange={e => setStartDate(e.target.value)}
                  />
                </label>
                <label className="audit-filter-label">
                  End Date
                  <input
                    type="date"
                    className="audit-date-input"
                    value={endDate}
                    min={startDate}
                    max={todayStr}
                    onChange={e => setEndDate(e.target.value)}
                  />
                </label>
                <label className="audit-filter-label">
                  Provider
                  <select
                    className="audit-select"
                    value={provider}
                    onChange={e => setProvider(e.target.value)}
                  >
                    <option value="">All Providers</option>
                    <option value="anthropic">Anthropic</option>
                    <option value="openai">OpenAI</option>
                  </select>
                </label>
              </div>
              <div className="audit-filter-row">
                <label className="audit-toggle-label">
                  <input
                    type="checkbox"
                    checked={billedOnly}
                    onChange={e => setBilledOnly(e.target.checked)}
                  />
                  API Usage Billed Only
                </label>
              </div>
              <div className="audit-actions">
                {canExport && isAdmin ? (
                  <>
                    <button
                      className="btn btn-primary"
                      onClick={() => handleGenerate('PDF')}
                      disabled={generating}
                    >
                      {generating ? 'Generating...' : 'Generate PDF'}
                    </button>
                    <button
                      className="btn btn-secondary"
                      onClick={() => handleGenerate('CSV')}
                      disabled={generating}
                    >
                      {generating ? 'Generating...' : 'Export CSV'}
                    </button>
                  </>
                ) : (
                  <>
                    <button
                      className="btn btn-primary btn-disabled"
                      disabled
                      title={!canExport ? 'Upgrade to Pro to export reports' : 'Admin role required'}
                    >
                      Generate PDF
                    </button>
                    <button
                      className="btn btn-secondary btn-disabled"
                      disabled
                      title={!canExport ? 'Upgrade to Pro to export reports' : 'Admin role required'}
                    >
                      Export CSV
                    </button>
                    <span className="audit-upgrade-hint">
                      {!canExport ? 'Upgrade to Pro to export reports' : 'Admin role required to generate reports'}
                    </span>
                  </>
                )}
              </div>
            </div>
          </section>

          {/* Report History */}
          <section className="audit-section">
            <h2>Report History</h2>
            {reports.length === 0 && !loading ? (
              <p className="audit-empty">No reports generated yet.</p>
            ) : (
              <div className="table-scroll">
                <table className="audit-table">
                  <thead>
                    <tr>
                      <th>Created</th>
                      <th>Period</th>
                      <th>Format</th>
                      <th>Status</th>
                      <th>Rows</th>
                      <th>Size</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {reports.map(r => (
                      <tr key={r.id}>
                        <td className="text-muted">{formatDateTime(r.created_at)}</td>
                        <td>{formatDate(r.period_start)} — {formatDate(r.period_end)}</td>
                        <td><span className={`audit-format-badge audit-format-${r.format.toLowerCase()}`}>{r.format}</span></td>
                        <td>
                          <span className={`audit-status-badge audit-status-${r.status.toLowerCase()}`}>
                            {r.status === 'RUNNING' && <span className="audit-spinner" />}
                            {r.status}
                          </span>
                          {r.status === 'FAILED' && r.error_message && (
                            <span className="audit-error-hint" title={r.error_message}>!</span>
                          )}
                        </td>
                        <td>{r.status === 'COMPLETED' ? r.row_count.toLocaleString() : '—'}</td>
                        <td>{r.status === 'COMPLETED' ? formatSize(r.artifact_size_bytes) : '—'}</td>
                        <td className="actions-cell">
                          {r.status === 'COMPLETED' && (
                            <button
                              className="btn btn-small btn-primary"
                              onClick={() => handleDownload(r.id, r.format)}
                            >
                              Download
                            </button>
                          )}
                          {isAdmin && (
                            <button
                              className="btn btn-small btn-danger"
                              onClick={() => handleDelete(r.id)}
                            >
                              Delete
                            </button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>

        </div>
      </div>
    </div>
  )
}
