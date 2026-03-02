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
  category: string
  actor_type: string
  user_agent: string
  success: boolean
  ip_address: string
  before_json: string
  after_json: string
  metadata: string
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
  const [logCategory, setLogCategory] = useState('')
  const [logStartDate, setLogStartDate] = useState('')
  const [logEndDate, setLogEndDate] = useState('')
  const [logLimit, setLogLimit] = useState(50)
  const [logOffset, setLogOffset] = useState(0)
  const [logTotal, setLogTotal] = useState(0)
  const [selectedLog, setSelectedLog] = useState<AuditLogEntry | null>(null)

  const fetchAuditLogs = useCallback(async () => {
    setLogsLoading(true)
    setLogsError(null)
    try {
      const params = new URLSearchParams()
      if (logAction) params.set('action', logAction)
      if (logResourceType) params.set('resource_type', logResourceType)
      if (logCategory) params.set('category', logCategory)
      if (logStartDate) params.set('start_date', logStartDate)
      if (logEndDate) params.set('end_date', logEndDate)
      params.set('limit', String(logLimit))
      params.set('offset', String(logOffset))
      const res = await apiFetch(`/v1/audit-logs?${params.toString()}`)
      if (!res.ok) throw new Error('Failed to fetch audit logs')
      const data = await res.json()
      setAuditLogs(data.audit_logs ?? [])
      setLogTotal(data.total ?? 0)
    } catch (e) {
      setLogsError(e instanceof Error ? e.message : 'Failed to load audit logs')
    } finally {
      setLogsLoading(false)
    }
  }, [logAction, logResourceType, logCategory, logStartDate, logEndDate, logLimit, logOffset])

  useEffect(() => {
    if (isAdmin) fetchAuditLogs()
  }, [isAdmin, fetchAuditLogs])

  // Reset offset when filters change
  const resetOffset = () => setLogOffset(0)

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
    if (bytes === 0) return '\u2014'
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

  function prettyJSON(raw: string | undefined): string | null {
    if (!raw || raw === '{}' || raw === 'null') return null
    try {
      const obj = JSON.parse(raw)
      if (Object.keys(obj).length === 0) return null
      return JSON.stringify(obj, null, 2)
    } catch {
      return raw
    }
  }

  // ── Pagination helpers ───────────────────────────────────────────────
  const pageStart = logTotal === 0 ? 0 : logOffset + 1
  const pageEnd = Math.min(logOffset + logLimit, logTotal)
  const hasPrev = logOffset > 0
  const hasNext = logOffset + logLimit < logTotal

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
                  Category
                  <select className="audit-select" value={logCategory} onChange={e => { setLogCategory(e.target.value); resetOffset() }}>
                    <option value="">All Categories</option>
                    <option value="ACCESS">ACCESS</option>
                    <option value="TEAM">TEAM</option>
                    <option value="PROJECT">PROJECT</option>
                    <option value="CONFIG">CONFIG</option>
                    <option value="BILLING">BILLING</option>
                    <option value="OWNER">OWNER</option>
                    <option value="ADMIN">ADMIN</option>
                  </select>
                </label>
                <label className="audit-filter-label">
                  Action
                  <select className="audit-select" value={logAction} onChange={e => { setLogAction(e.target.value); resetOffset() }}>
                    <option value="">All Actions</option>
                    <optgroup label="Access">
                      <option value="API_KEY.CREATED">API Key Created</option>
                      <option value="API_KEY.REVOKED">API Key Revoked</option>
                      <option value="PROVIDER_KEY.CREATED">Provider Key Created</option>
                      <option value="PROVIDER_KEY.REVOKED">Provider Key Revoked</option>
                      <option value="PROVIDER_KEY.ACTIVATED">Provider Key Activated</option>
                      <option value="PROVIDER_KEY.ROTATED">Provider Key Rotated</option>
                    </optgroup>
                    <optgroup label="Team">
                      <option value="MEMBER.INVITED">Member Invited</option>
                      <option value="MEMBER.REMOVED">Member Removed</option>
                      <option value="MEMBER.ROLE_CHANGED">Member Role Changed</option>
                      <option value="MEMBER.SUSPENDED">Member Suspended</option>
                      <option value="MEMBER.UNSUSPENDED">Member Unsuspended</option>
                      <option value="MEMBER.PROMOTED">Member Promoted</option>
                      <option value="MEMBER.DEMOTED">Member Demoted</option>
                    </optgroup>
                    <optgroup label="Project">
                      <option value="PROJECT.CREATED">Project Created</option>
                      <option value="PROJECT.UPDATED">Project Updated</option>
                      <option value="PROJECT.DELETED">Project Deleted</option>
                      <option value="PROJECT_MEMBER.ADDED">Project Member Added</option>
                      <option value="PROJECT_MEMBER.ROLE_CHANGED">Project Member Role Changed</option>
                      <option value="PROJECT_MEMBER.REMOVED">Project Member Removed</option>
                    </optgroup>
                    <optgroup label="Config">
                      <option value="BUDGET.CREATED">Budget Created</option>
                      <option value="BUDGET.UPDATED">Budget Updated</option>
                      <option value="BUDGET.DELETED">Budget Deleted</option>
                      <option value="RATE_LIMIT.CREATED">Rate Limit Created</option>
                      <option value="RATE_LIMIT.UPDATED">Rate Limit Updated</option>
                      <option value="RATE_LIMIT.DELETED">Rate Limit Deleted</option>
                      <option value="NOTIFICATION_CHANNEL.CREATED">Notification Created</option>
                      <option value="NOTIFICATION_CHANNEL.UPDATED">Notification Updated</option>
                      <option value="NOTIFICATION_CHANNEL.DELETED">Notification Deleted</option>
                      <option value="PRICING_CONFIG.CREATED">Pricing Config Created</option>
                      <option value="PRICING_CONFIG.DELETED">Pricing Config Deleted</option>
                      <option value="PRICING_CONFIG.ASSIGNED">Pricing Config Assigned</option>
                      <option value="PRICING_CONFIG.UNASSIGNED">Pricing Config Unassigned</option>
                    </optgroup>
                    <optgroup label="Billing">
                      <option value="BILLING.CHECKOUT">Billing Checkout</option>
                      <option value="BILLING.PLAN_CHANGED">Billing Plan Changed</option>
                      <option value="BILLING.DOWNGRADED">Billing Downgraded</option>
                      <option value="BILLING.DOWNGRADE_CANCELED">Billing Downgrade Canceled</option>
                    </optgroup>
                    <optgroup label="Owner">
                      <option value="OWNERSHIP.TRANSFERRED">Ownership Transferred</option>
                      <option value="SETTINGS.UPDATED">Settings Updated</option>
                      <option value="ACCOUNT.DELETED">Account Deleted</option>
                    </optgroup>
                    <optgroup label="Admin">
                      <option value="SUPERADMIN.PLAN_CHANGED">Super Admin Plan Changed</option>
                      <option value="SUPERADMIN.STATUS_CHANGED">Super Admin Status Changed</option>
                    </optgroup>
                    <optgroup label="Legacy">
                      <option value="api_key:create">api_key:create</option>
                      <option value="api_key:revoke">api_key:revoke</option>
                      <option value="provider_key:create">provider_key:create</option>
                      <option value="provider_key:revoke">provider_key:revoke</option>
                      <option value="project:create">project:create</option>
                      <option value="project:update">project:update</option>
                      <option value="project:delete">project:delete</option>
                      <option value="member:invite">member:invite</option>
                      <option value="member:remove">member:remove</option>
                    </optgroup>
                  </select>
                </label>
                <label className="audit-filter-label">
                  Resource
                  <select className="audit-select" value={logResourceType} onChange={e => { setLogResourceType(e.target.value); resetOffset() }}>
                    <option value="">All Resources</option>
                    <option value="api_key">API Key</option>
                    <option value="provider_key">Provider Key</option>
                    <option value="project">Project</option>
                    <option value="membership">Membership</option>
                    <option value="project_membership">Project Membership</option>
                    <option value="budget_limit">Budget Limit</option>
                    <option value="rate_limit">Rate Limit</option>
                    <option value="notification_channel">Notification Channel</option>
                    <option value="pricing_config">Pricing Config</option>
                    <option value="billing">Billing</option>
                    <option value="tenant">Tenant</option>
                  </select>
                </label>
              </div>
              <div className="audit-filter-row" style={{ marginBottom: '0.75rem' }}>
                <label className="audit-filter-label">
                  Start Date
                  <input type="date" className="audit-date-input" value={logStartDate} onChange={e => { setLogStartDate(e.target.value); resetOffset() }} />
                </label>
                <label className="audit-filter-label">
                  End Date
                  <input type="date" className="audit-date-input" value={logEndDate} max={todayStr} onChange={e => { setLogEndDate(e.target.value); resetOffset() }} />
                </label>
                <label className="audit-filter-label">
                  Limit
                  <select className="audit-select" value={logLimit} onChange={e => { setLogLimit(parseInt(e.target.value, 10)); resetOffset() }}>
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
                        <th>Category</th>
                        <th>Resource</th>
                        <th>Resource ID</th>
                        <th>Result</th>
                        <th>IP Address</th>
                      </tr>
                    </thead>
                    <tbody>
                      {auditLogs.map(log => (
                        <tr key={log.id} onClick={() => setSelectedLog(log)} style={{ cursor: 'pointer' }}>
                          <td className="text-muted">{formatDateTime(log.created_at)}</td>
                          <td><span className="audit-format-badge">{log.action}</span></td>
                          <td>{log.category ? <span className="audit-category-badge">{log.category}</span> : '\u2014'}</td>
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
              {/* Pagination */}
              {logTotal > 0 && (
                <div className="audit-pagination">
                  <button className="btn btn-secondary btn-small" disabled={!hasPrev} onClick={() => setLogOffset(Math.max(0, logOffset - logLimit))}>
                    Previous
                  </button>
                  <span className="audit-pagination-info">{pageStart}&ndash;{pageEnd} of {logTotal}</span>
                  <button className="btn btn-secondary btn-small" disabled={!hasNext} onClick={() => setLogOffset(logOffset + logLimit)}>
                    Next
                  </button>
                </div>
              )}
            </section>
          )}

          {/* ── Detail Drawer ─────────────────────────────────── */}
          {selectedLog && (
            <div className="audit-detail-drawer" onClick={() => setSelectedLog(null)}>
              <div className="audit-detail-panel" onClick={e => e.stopPropagation()}>
                <div className="audit-detail-header">
                  <h3>Audit Log Detail</h3>
                  <button className="btn btn-secondary btn-small" onClick={() => setSelectedLog(null)}>Close</button>
                </div>
                <dl className="audit-detail-fields">
                  <dt>Action</dt>
                  <dd><span className="audit-format-badge">{selectedLog.action}</span></dd>

                  <dt>Category</dt>
                  <dd>{selectedLog.category ? <span className="audit-category-badge">{selectedLog.category}</span> : '\u2014'}</dd>

                  <dt>Actor</dt>
                  <dd>{selectedLog.actor_user_id || '\u2014'}{selectedLog.actor_type && selectedLog.actor_type !== 'user' ? ` (${selectedLog.actor_type})` : ''}</dd>

                  <dt>Resource</dt>
                  <dd>{selectedLog.resource_type} / {selectedLog.resource_id || '\u2014'}</dd>

                  <dt>Result</dt>
                  <dd>
                    <span className={`audit-status-badge audit-status-${selectedLog.success ? 'completed' : 'failed'}`}>
                      {selectedLog.success ? 'Success' : 'Failed'}
                    </span>
                  </dd>

                  <dt>IP Address</dt>
                  <dd>{selectedLog.ip_address || '\u2014'}</dd>

                  <dt>User Agent</dt>
                  <dd className="text-muted" style={{ wordBreak: 'break-all' }}>{selectedLog.user_agent || '\u2014'}</dd>

                  <dt>Time</dt>
                  <dd>{new Date(selectedLog.created_at).toLocaleString()}</dd>

                  {prettyJSON(selectedLog.before_json) && (
                    <>
                      <dt>Before</dt>
                      <dd><pre>{prettyJSON(selectedLog.before_json)}</pre></dd>
                    </>
                  )}

                  {prettyJSON(selectedLog.after_json) && (
                    <>
                      <dt>After</dt>
                      <dd><pre>{prettyJSON(selectedLog.after_json)}</pre></dd>
                    </>
                  )}

                  {prettyJSON(selectedLog.metadata) && (
                    <>
                      <dt>Metadata</dt>
                      <dd><pre>{prettyJSON(selectedLog.metadata)}</pre></dd>
                    </>
                  )}
                </dl>
              </div>
            </div>
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
                        <td>{formatDate(r.period_start)} &mdash; {formatDate(r.period_end)}</td>
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
                        <td>{r.status === 'COMPLETED' ? r.row_count.toLocaleString() : '\u2014'}</td>
                        <td>{r.status === 'COMPLETED' ? formatSize(r.artifact_size_bytes) : '\u2014'}</td>
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
