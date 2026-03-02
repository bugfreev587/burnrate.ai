import { useState, useEffect, useRef, useCallback } from 'react'
import { hasPermission, type UserRole } from '../hooks/useUserSync'
import { useTenant } from '../contexts/TenantContext'
import { useDashboardConfig } from '../hooks/useDashboardConfig'
import { useAuditReports } from '../hooks/useAuditReports'
import { apiFetch } from '../lib/api'
import type { CreateReportRequest } from '../hooks/useAuditReports'
import Navbar from '../components/Navbar'
import './AuditPage.css'

interface AuditLogEntry {
  id: number
  tenant_id: number
  actor_user_id: string
  action: string
  resource_type: string
  resource_id: string
  success: boolean
  ip_address: string
  created_at: string
}

export default function AuditPage() {
  const { orgRole, isSynced } = useTenant()
  const role = (orgRole as UserRole) ?? null
  const { config } = useDashboardConfig()
  const { reports, loading, error, refresh, generate, deleteReport, downloadReport } = useAuditReports()

  const isAdmin = isSynced && hasPermission(role, 'admin')

  // ── Audit Logs state ──────────────────────────────────────────────
  const [auditLogs, setAuditLogs] = useState<AuditLogEntry[]>([])
  const [logsLoading, setLogsLoading] = useState(false)
  const [logsError, setLogsError] = useState<string | null>(null)
  const [logAction, setLogAction] = useState('')
  const [logResourceType, setLogResourceType] = useState('')
  const [logLimit, setLogLimit] = useState(50)

  const fetchAuditLogs = useCallback(async () => {
    setLogsLoading(true)
    setLogsError(null)
    try {
      const params = new URLSearchParams()
      if (logAction) params.set('action', logAction)
      if (logResourceType) params.set('resource_type', logResourceType)
      params.set('limit', String(logLimit))
      const res = await apiFetch(`/v1/audit-logs?${params.toString()}`)
      if (!res.ok) throw new Error('Failed to fetch audit logs')
      const data = await res.json()
      setAuditLogs(data.audit_logs ?? [])
    } catch (e) {
      setLogsError(e instanceof Error ? e.message : 'Failed to load audit logs')
    } finally {
      setLogsLoading(false)
    }
  }, [logAction, logResourceType, logLimit])

  useEffect(() => {
    if (isAdmin) fetchAuditLogs()
  }, [isAdmin, fetchAuditLogs])
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

          {/* ── Audit Logs (Admin only) ────────────────────────── */}
          {isAdmin && (
            <section className="audit-section">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.75rem' }}>
                <h2>Activity Log</h2>
                <button className="btn btn-secondary" onClick={fetchAuditLogs} disabled={logsLoading}>
                  {logsLoading ? 'Loading...' : 'Refresh'}
                </button>
              </div>
              <div className="audit-filter-row" style={{ marginBottom: '0.75rem' }}>
                <label className="audit-filter-label">
                  Action
                  <select className="audit-select" value={logAction} onChange={e => setLogAction(e.target.value)}>
                    <option value="">All Actions</option>
                    <option value="api_key:create">API Key Create</option>
                    <option value="api_key:revoke">API Key Revoke</option>
                    <option value="provider_key:create">Provider Key Create</option>
                    <option value="provider_key:revoke">Provider Key Revoke</option>
                    <option value="project:create">Project Create</option>
                    <option value="project:update">Project Update</option>
                    <option value="project:delete">Project Delete</option>
                    <option value="member:invite">Member Invite</option>
                    <option value="member:remove">Member Remove</option>
                    <option value="billing:update_plan">Plan Change</option>
                  </select>
                </label>
                <label className="audit-filter-label">
                  Resource
                  <select className="audit-select" value={logResourceType} onChange={e => setLogResourceType(e.target.value)}>
                    <option value="">All Resources</option>
                    <option value="api_key">API Key</option>
                    <option value="provider_key">Provider Key</option>
                    <option value="project">Project</option>
                    <option value="membership">Membership</option>
                    <option value="budget_limit">Budget Limit</option>
                    <option value="rate_limit">Rate Limit</option>
                    <option value="billing">Billing</option>
                  </select>
                </label>
                <label className="audit-filter-label">
                  Limit
                  <select className="audit-select" value={logLimit} onChange={e => setLogLimit(parseInt(e.target.value, 10))}>
                    <option value="25">25</option>
                    <option value="50">50</option>
                    <option value="100">100</option>
                  </select>
                </label>
              </div>
              {logsError && <div className="flash flash-error">{logsError}</div>}
              {auditLogs.length === 0 && !logsLoading ? (
                <p className="audit-empty">No audit log entries found.</p>
              ) : (
                <div className="table-scroll">
                  <table className="audit-table">
                    <thead>
                      <tr>
                        <th>Time</th>
                        <th>Action</th>
                        <th>Resource</th>
                        <th>Resource ID</th>
                        <th>Result</th>
                        <th>IP Address</th>
                      </tr>
                    </thead>
                    <tbody>
                      {auditLogs.map(log => (
                        <tr key={log.id}>
                          <td className="text-muted">{formatDateTime(log.created_at)}</td>
                          <td><span className="audit-format-badge">{log.action}</span></td>
                          <td>{log.resource_type}</td>
                          <td className="text-muted">{log.resource_id?.slice(0, 12) || '\u2014'}</td>
                          <td>
                            <span className={`audit-status-badge audit-status-${log.success ? 'completed' : 'failed'}`}>
                              {log.success ? 'Success' : 'Failed'}
                            </span>
                          </td>
                          <td className="text-muted">{log.ip_address || '\u2014'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </section>
          )}

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
