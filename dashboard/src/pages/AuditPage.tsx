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

type ScopeType = '' | 'api_key' | 'provider_key' | 'project'

interface ScopeAPIKey {
  key_id: string
  label: string
}

interface ScopeProviderKey {
  id: number
  label: string
}

interface ScopeProject {
  id: number
  name: string
}

interface UserOption {
  id: string
  email: string
  name: string
}

export default function AuditPage() {
  const { orgRole, isSynced } = useTenant()
  const role = (orgRole as UserRole) ?? null
  const { config } = useDashboardConfig()
  const { reports, loading, error, refresh, generate, deleteReport, downloadReport } = useAuditReports()

  const isAdmin = isSynced && hasPermission(role, 'admin')
  const isSuperAdmin = sessionStorage.getItem('is_super_admin') === 'true'

  // ── Audit Logs state ──────────────────────────────────────────────
  const [auditLogs, setAuditLogs] = useState<AuditLogEntry[]>([])
  const [logsLoading, setLogsLoading] = useState(false)
  const [logsError, setLogsError] = useState<string | null>(null)
  const [logAction, setLogAction] = useState('')
  const [logResourceType, setLogResourceType] = useState('')
  const [logCategory, setLogCategory] = useState('')
  const [logStartDate, setLogStartDate] = useState('')
  const [logEndDate, setLogEndDate] = useState('')
  const [logLimit, setLogLimit] = useState(10)
  const [logOffset, setLogOffset] = useState(0)
  const [logTotal, setLogTotal] = useState(0)
  const [selectedLog, setSelectedLog] = useState<AuditLogEntry | null>(null)

  // ── Entity scope state ──────────────────────────────────────────
  const [scopeType, setScopeType] = useState<ScopeType>('')
  const [scopeEntityId, setScopeEntityId] = useState('')
  const [apiKeys, setApiKeys] = useState<ScopeAPIKey[]>([])
  const [providerKeys, setProviderKeys] = useState<ScopeProviderKey[]>([])
  const [projects, setProjects] = useState<ScopeProject[]>([])
  const [users, setUsers] = useState<UserOption[]>([])

  // ── Report generation filter state ────────────────────────────────
  const [selectedProjectIds, setSelectedProjectIds] = useState<number[]>([])
  const [selectedUserIds, setSelectedUserIds] = useState<string[]>([])
  const [billingMode, setBillingMode] = useState('')

  // Read URL params on mount
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const st = params.get('scope_type') as ScopeType
    const sei = params.get('scope_entity_id') ?? ''
    const cat = params.get('category') ?? ''
    const act = params.get('action') ?? ''
    if (st && ['api_key', 'provider_key', 'project'].includes(st)) {
      setScopeType(st)
      setScopeEntityId(sei)
    }
    if (cat) setLogCategory(cat)
    if (act) setLogAction(act)
  }, [])

  // Fetch entity lists for scope dropdowns + report filters
  useEffect(() => {
    if (!isAdmin) return
    Promise.all([
      apiFetch('/v1/admin/api_keys').then(r => r.ok ? r.json() : null),
      apiFetch('/v1/admin/provider_keys').then(r => r.ok ? r.json() : null),
      apiFetch('/v1/projects').then(r => r.ok ? r.json() : null),
      apiFetch('/v1/admin/users').then(r => r.ok ? r.json() : null),
    ]).then(([akData, pkData, projData, usersData]) => {
      if (akData?.api_keys) setApiKeys(akData.api_keys.map((k: { key_id: string; label: string }) => ({ key_id: k.key_id, label: k.label })))
      if (pkData?.provider_keys) setProviderKeys(pkData.provider_keys.map((k: { id: number; label: string }) => ({ id: k.id, label: k.label })))
      if (projData?.projects) setProjects(projData.projects.map((p: { id: number; name: string }) => ({ id: p.id, name: p.name })))
      if (usersData?.users) setUsers(usersData.users.map((u: { id: string; email: string; name: string }) => ({ id: u.id, email: u.email, name: u.name })))
    })
  }, [isAdmin])

  // Update URL when scope filters change
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    if (scopeType) {
      params.set('scope_type', scopeType)
      params.set('scope_entity_id', scopeEntityId)
    } else {
      params.delete('scope_type')
      params.delete('scope_entity_id')
    }
    if (logCategory) params.set('category', logCategory)
    else params.delete('category')
    if (logAction) params.set('action', logAction)
    else params.delete('action')
    const qs = params.toString()
    const newUrl = window.location.pathname + (qs ? '?' + qs : '')
    window.history.replaceState(null, '', newUrl)
  }, [scopeType, scopeEntityId, logCategory, logAction])

  const fetchAuditLogs = useCallback(async () => {
    setLogsLoading(true)
    setLogsError(null)
    try {
      const params = new URLSearchParams()
      if (scopeType === 'project' && scopeEntityId) {
        params.set('scope_project_id', scopeEntityId)
      } else if (scopeType && scopeEntityId) {
        params.set('resource_type', scopeType)
        params.set('resource_id', scopeEntityId)
      } else {
        if (logResourceType) params.set('resource_type', logResourceType)
      }
      if (logAction) params.set('action', logAction)
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
  }, [scopeType, scopeEntityId, logAction, logResourceType, logCategory, logStartDate, logEndDate, logLimit, logOffset])

  useEffect(() => {
    if (isAdmin) fetchAuditLogs()
  }, [isAdmin, fetchAuditLogs])

  // Reset pagination when filters change
  const resetOffset = () => { setLogOffset(0); setLogLimit(10) }

  const plan = config?.plan || 'free'
  const canExport = plan !== 'free'

  // ── Filters ──────────────────────────────────────────────────────────
  const today = new Date()
  const todayStr = today.getFullYear() + '-' + String(today.getMonth() + 1).padStart(2, '0') + '-' + String(today.getDate()).padStart(2, '0')
  const defaultStart = new Date(today)
  defaultStart.setDate(defaultStart.getDate() - 29)
  const defaultStartStr = defaultStart.getFullYear() + '-' + String(defaultStart.getMonth() + 1).padStart(2, '0') + '-' + String(defaultStart.getDate()).padStart(2, '0')

  const [startDate, setStartDate] = useState(defaultStartStr + 'T00:00')
  const [endDate, setEndDate] = useState(todayStr + 'T23:59')
  const [timezone, setTimezone] = useState(Intl.DateTimeFormat().resolvedOptions().timeZone)
  const [includeTopRequestsByCost, setIncludeTopRequestsByCost] = useState(false)
  const [topRequestsLimit, setTopRequestsLimit] = useState(10)
  const [provider, setProvider] = useState('')
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
        timezone,
      }
      if (provider) req.provider = provider
      if (selectedProjectIds.length > 0) req.project_ids = selectedProjectIds
      if (selectedUserIds.length > 0) req.user_ids = selectedUserIds
      if (billingMode) req.billing_mode = billingMode
      if (includeTopRequestsByCost) {
        req.include_top_requests_by_cost = true
        req.top_requests_limit = topRequestsLimit
      }
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

  // ── Scope summary helper ────────────────────────────────────────────
  function formatScope(filtersJson: string): string {
    if (!filtersJson || filtersJson === '{}' || filtersJson === 'null') return 'All'
    try {
      const f = JSON.parse(filtersJson)
      const parts: string[] = []
      if (f.provider) parts.push(f.provider)
      if (f.project_ids?.length) parts.push(`${f.project_ids.length} project(s)`)
      if (f.user_ids?.length) parts.push(`${f.user_ids.length} user(s)`)
      if (f.api_key_ids?.length) parts.push(`${f.api_key_ids.length} key(s)`)
      if (f.billing_mode === 'api_usage') parts.push('API Usage')
      else if (f.billing_mode === 'subscription') parts.push('Subscription')
      else if (f.api_usage_billed === true) parts.push('Billed Only')
      else if (f.api_usage_billed === false) parts.push('Unbilled Only')
      return parts.length > 0 ? parts.join(', ') : 'All'
    } catch {
      return 'All'
    }
  }

  // ── Pagination helpers ───────────────────────────────────────────────
  const hasMoreLogs = auditLogs.length === logLimit && logOffset + logLimit < logTotal
  const [visibleReportsCount, setVisibleReportsCount] = useState(10)

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
              <div className="audit-scope-row" style={{ marginBottom: '0.75rem' }}>
                <label className="audit-filter-label">
                  Scope
                  <select className="audit-select" value={scopeType} onChange={e => {
                    const v = e.target.value as ScopeType
                    setScopeType(v)
                    setScopeEntityId('')
                    if (!v) setLogResourceType('')
                    resetOffset()
                  }}>
                    <option value="">All</option>
                    <option value="api_key">API Key</option>
                    <option value="provider_key">Provider Key</option>
                    <option value="project">Project</option>
                  </select>
                </label>
                {scopeType === 'api_key' && (
                  <label className="audit-filter-label">
                    API Key
                    <select className="audit-select" value={scopeEntityId} onChange={e => { setScopeEntityId(e.target.value); resetOffset() }}>
                      <option value="">Select API Key...</option>
                      {apiKeys.map(k => (
                        <option key={k.key_id} value={k.key_id}>{k.label || k.key_id}</option>
                      ))}
                    </select>
                  </label>
                )}
                {scopeType === 'provider_key' && (
                  <label className="audit-filter-label">
                    Provider Key
                    <select className="audit-select" value={scopeEntityId} onChange={e => { setScopeEntityId(e.target.value); resetOffset() }}>
                      <option value="">Select Provider Key...</option>
                      {providerKeys.map(k => (
                        <option key={k.id} value={String(k.id)}>{k.label || `Key #${k.id}`}</option>
                      ))}
                    </select>
                  </label>
                )}
                {scopeType === 'project' && (
                  <label className="audit-filter-label">
                    Project
                    <select className="audit-select" value={scopeEntityId} onChange={e => { setScopeEntityId(e.target.value); resetOffset() }}>
                      <option value="">Select Project...</option>
                      {projects.map(p => (
                        <option key={p.id} value={String(p.id)}>{p.name}</option>
                      ))}
                    </select>
                  </label>
                )}
                {scopeType && scopeEntityId && (
                  <button className="btn btn-secondary btn-small" style={{ alignSelf: 'flex-end' }} onClick={() => {
                    setScopeType('')
                    setScopeEntityId('')
                    setLogResourceType('')
                    resetOffset()
                  }}>
                    Clear Scope
                  </button>
                )}
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
                    {isSuperAdmin && <option value="ADMIN">ADMIN</option>}
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
                    {isSuperAdmin && (
                    <optgroup label="Admin">
                      <option value="SUPERADMIN.PLAN_CHANGED">Super Admin Plan Changed</option>
                      <option value="SUPERADMIN.STATUS_CHANGED">Super Admin Status Changed</option>
                    </optgroup>
                    )}
                  </select>
                </label>
                <label className="audit-filter-label">
                  Resource
                  <select className="audit-select" value={scopeType ? '' : logResourceType} disabled={!!scopeType} onChange={e => { setLogResourceType(e.target.value); resetOffset() }}>
                    <option value="">{scopeType ? '(Set by scope)' : 'All Resources'}</option>
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
              {hasMoreLogs && (
                <div className="audit-show-more">
                  <button className="btn btn-secondary" onClick={() => setLogLimit(prev => prev + 10)}>
                    Show more
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
                  Start Date/Time
                  <input
                    type="datetime-local"
                    className="audit-date-input"
                    value={startDate}
                    min={effectiveMin + 'T00:00'}
                    max={endDate}
                    onChange={e => setStartDate(e.target.value)}
                  />
                </label>
                <label className="audit-filter-label">
                  End Date/Time
                  <input
                    type="datetime-local"
                    className="audit-date-input"
                    value={endDate}
                    min={startDate}
                    max={todayStr + 'T23:59'}
                    onChange={e => setEndDate(e.target.value)}
                  />
                </label>
                <label className="audit-filter-label">
                  Timezone
                  <select
                    className="audit-select"
                    value={timezone}
                    onChange={e => setTimezone(e.target.value)}
                  >
                    <option value="UTC">UTC</option>
                    <option value="America/New_York">US Eastern</option>
                    <option value="America/Chicago">US Central</option>
                    <option value="America/Denver">US Mountain</option>
                    <option value="America/Los_Angeles">US Pacific</option>
                    <option value="America/Anchorage">US Alaska</option>
                    <option value="Pacific/Honolulu">US Hawaii</option>
                    <option value="Europe/London">Europe/London</option>
                    <option value="Europe/Berlin">Europe/Berlin</option>
                    <option value="Europe/Paris">Europe/Paris</option>
                    <option value="Asia/Tokyo">Asia/Tokyo</option>
                    <option value="Asia/Shanghai">Asia/Shanghai</option>
                    <option value="Asia/Kolkata">Asia/Kolkata</option>
                    <option value="Asia/Singapore">Asia/Singapore</option>
                    <option value="Australia/Sydney">Australia/Sydney</option>
                    <option value="Australia/Melbourne">Australia/Melbourne</option>
                  </select>
                </label>
              </div>
              <div className="audit-filter-row">
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
                <label className="audit-filter-label">
                  Project
                  <select
                    className="audit-select"
                    value={selectedProjectIds.length === 1 ? String(selectedProjectIds[0]) : ''}
                    onChange={e => {
                      const v = e.target.value
                      setSelectedProjectIds(v ? [Number(v)] : [])
                    }}
                  >
                    <option value="">All Projects</option>
                    {projects.map(p => (
                      <option key={p.id} value={String(p.id)}>{p.name}</option>
                    ))}
                  </select>
                </label>
                <label className="audit-filter-label">
                  User
                  <select
                    className="audit-select"
                    value={selectedUserIds.length === 1 ? selectedUserIds[0] : ''}
                    onChange={e => {
                      const v = e.target.value
                      setSelectedUserIds(v ? [v] : [])
                    }}
                  >
                    <option value="">All Users</option>
                    {users.map(u => (
                      <option key={u.id} value={u.id}>{u.name || u.email}</option>
                    ))}
                  </select>
                </label>
                <label className="audit-filter-label">
                  Billing Mode
                  <select
                    className="audit-select"
                    value={billingMode}
                    onChange={e => setBillingMode(e.target.value)}
                  >
                    <option value="">All</option>
                    <option value="api_usage">API Usage</option>
                    <option value="subscription">Monthly Subscription</option>
                  </select>
                </label>
              </div>
              <div className="audit-filter-row">
                <label className="audit-filter-label" style={{ flexDirection: 'row', alignItems: 'center', gap: '0.5rem' }}>
                  <input
                    type="checkbox"
                    checked={includeTopRequestsByCost}
                    onChange={e => setIncludeTopRequestsByCost(e.target.checked)}
                  />
                  Include Top Requests by Cost
                </label>
                {includeTopRequestsByCost && (
                  <label className="audit-filter-label">
                    Limit
                    <input
                      type="number"
                      className="audit-date-input"
                      value={topRequestsLimit}
                      min={1}
                      max={100}
                      style={{ width: '5rem' }}
                      onChange={e => setTopRequestsLimit(Math.max(1, Math.min(100, parseInt(e.target.value, 10) || 10)))}
                    />
                  </label>
                )}
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
                      <th>Scope</th>
                      <th>Status</th>
                      <th>Rows</th>
                      <th>Size</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {reports.slice(0, visibleReportsCount).map(r => (
                      <tr key={r.id}>
                        <td className="text-muted">{formatDateTime(r.created_at)}</td>
                        <td>{formatDate(r.period_start)} &mdash; {formatDate(r.period_end)}{r.timezone && r.timezone !== 'UTC' ? ` (${r.timezone})` : ''}</td>
                        <td><span className={`audit-format-badge audit-format-${r.format.toLowerCase()}`}>{r.format}</span></td>
                        <td className="text-muted">{formatScope(r.filters)}</td>
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
            {reports.length > visibleReportsCount && (
              <div className="audit-show-more">
                <button className="btn btn-secondary" onClick={() => setVisibleReportsCount(prev => prev + 10)}>
                  Show more
                </button>
              </div>
            )}
          </section>

        </div>
      </div>
    </div>
  )
}
