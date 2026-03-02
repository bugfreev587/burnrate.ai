import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import Navbar from '../components/Navbar'
import { apiFetch } from '../lib/api'
import './ManagementPage.css'
import './SuperAdminPage.css'

// ── Types ────────────────────────────────────────────────────────────────────

interface PlatformStats {
  total_tenants: number
  tenants_by_plan: Record<string, number>
  total_users: number
  total_api_keys: number
  usage_count_30d: number
  total_cost_30d: number
}

interface TenantRow {
  id: number
  name: string
  plan: string
  status: string
  billing_email: string
  member_count: number
  api_key_count: number
  created_at: string
}

interface TenantDetail {
  tenant: {
    id: number
    name: string
    plan: string
    status: string
    billing_email: string
    stripe_customer_id: string
    stripe_subscription_id: string
    plan_status: string
    current_period_end: string | null
    pending_plan: string
    plan_effective_at: string | null
    created_at: string
  }
  plan_limits: Record<string, unknown>
  members: Array<{
    user_id: string
    email: string
    name: string
    org_role: string
    status: string
  }>
  api_key_count: number
  provider_key_count: number
  project_count: number
  usage_count_30d: number
  total_cost_30d: number
}

// ── Helpers ──────────────────────────────────────────────────────────────────

const PLANS = ['free', 'pro', 'team', 'business'] as const

function planBadge(plan: string) {
  return <span className={`sa-plan-badge sa-plan-${plan}`}>{plan}</span>
}

function statusBadge(status: string) {
  return <span className={`status-badge status-${status}`}>{status}</span>
}

function fmtCost(v: number | string): string {
  return '$' + Number(v).toFixed(2)
}

function fmtDate(iso: string): string {
  return new Date(iso).toLocaleDateString()
}

function fmtNum(n: number): string {
  return n.toLocaleString()
}

// ── Component ────────────────────────────────────────────────────────────────

export default function SuperAdminPage() {
  const navigate = useNavigate()

  // Auth gate
  const [authorized, setAuthorized] = useState<boolean | null>(null)

  // Stats
  const [stats, setStats] = useState<PlatformStats | null>(null)

  // Tenant list
  const [tenants, setTenants] = useState<TenantRow[]>([])
  const [total, setTotal] = useState(0)
  const [totalPages, setTotalPages] = useState(1)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [planFilter, setPlanFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')

  // Detail modal
  const [detail, setDetail] = useState<TenantDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)

  // Change plan modal
  const [changePlanTenant, setChangePlanTenant] = useState<{ id: number; name: string; currentPlan: string } | null>(null)
  const [selectedPlan, setSelectedPlan] = useState('')
  const [changePlanLoading, setChangePlanLoading] = useState(false)
  const [changePlanError, setChangePlanError] = useState('')

  // Flash
  const [flash, setFlash] = useState<{ type: 'success' | 'error'; msg: string } | null>(null)

  // ── Auth check ──────────────────────────────────────────────────────────

  useEffect(() => {
    apiFetch('/v1/superadmin/whoami')
      .then(res => {
        if (res.ok) {
          setAuthorized(true)
        } else {
          setAuthorized(false)
          navigate('/dashboard')
        }
      })
      .catch(() => {
        setAuthorized(false)
        navigate('/dashboard')
      })
  }, [navigate])

  // ── Load stats ──────────────────────────────────────────────────────────

  const loadStats = useCallback(async () => {
    try {
      const res = await apiFetch('/v1/superadmin/stats')
      if (res.ok) setStats(await res.json())
    } catch { /* silent */ }
  }, [])

  useEffect(() => {
    if (authorized) loadStats()
  }, [authorized, loadStats])

  // ── Load tenants ────────────────────────────────────────────────────────

  const loadTenants = useCallback(async () => {
    const params = new URLSearchParams({ page: String(page), per_page: '25' })
    if (search) params.set('search', search)
    if (planFilter) params.set('plan', planFilter)
    if (statusFilter) params.set('status', statusFilter)

    try {
      const res = await apiFetch(`/v1/superadmin/tenants?${params}`)
      if (res.ok) {
        const data = await res.json()
        setTenants(data.tenants || [])
        setTotal(data.total || 0)
        setTotalPages(data.total_pages || 1)
      }
    } catch { /* silent */ }
  }, [page, search, planFilter, statusFilter])

  useEffect(() => {
    if (authorized) loadTenants()
  }, [authorized, loadTenants])

  // ── View detail ─────────────────────────────────────────────────────────

  const openDetail = async (tenantId: number) => {
    setDetailLoading(true)
    try {
      const res = await apiFetch(`/v1/superadmin/tenants/${tenantId}`)
      if (res.ok) {
        setDetail(await res.json())
      }
    } catch { /* silent */ }
    setDetailLoading(false)
  }

  // ── Change plan ─────────────────────────────────────────────────────────

  const openChangePlan = (tenant: { id: number; name: string; plan: string }) => {
    setChangePlanTenant({ id: tenant.id, name: tenant.name, currentPlan: tenant.plan })
    setSelectedPlan(tenant.plan)
    setChangePlanError('')
  }

  const submitChangePlan = async () => {
    if (!changePlanTenant || selectedPlan === changePlanTenant.currentPlan) return
    setChangePlanLoading(true)
    setChangePlanError('')

    try {
      const res = await apiFetch(`/v1/superadmin/tenants/${changePlanTenant.id}/plan`, {
        method: 'PATCH',
        body: JSON.stringify({ plan: selectedPlan }),
      })
      const data = await res.json()
      if (res.ok) {
        setFlash({ type: 'success', msg: `Plan changed to "${selectedPlan}" for ${changePlanTenant.name}.` })
        setChangePlanTenant(null)
        loadTenants()
        loadStats()
        if (detail && detail.tenant.id === changePlanTenant.id) {
          openDetail(changePlanTenant.id)
        }
      } else {
        setChangePlanError(data.message || data.error || 'Failed to change plan')
      }
    } catch {
      setChangePlanError('Network error')
    }
    setChangePlanLoading(false)
  }

  // ── Update status ───────────────────────────────────────────────────────

  const toggleTenantStatus = async (tenantId: number, currentStatus: string) => {
    const newStatus = currentStatus === 'active' ? 'suspended' : 'active'
    try {
      const res = await apiFetch(`/v1/superadmin/tenants/${tenantId}/status`, {
        method: 'PATCH',
        body: JSON.stringify({ status: newStatus }),
      })
      if (res.ok) {
        setFlash({ type: 'success', msg: `Tenant ${newStatus === 'suspended' ? 'suspended' : 'unsuspended'} successfully.` })
        loadTenants()
        if (detail && detail.tenant.id === tenantId) {
          openDetail(tenantId)
        }
      } else {
        const data = await res.json()
        setFlash({ type: 'error', msg: data.message || data.error || 'Failed to update status' })
      }
    } catch {
      setFlash({ type: 'error', msg: 'Network error' })
    }
  }

  // ── Search debounce ─────────────────────────────────────────────────────

  const [searchInput, setSearchInput] = useState('')

  useEffect(() => {
    const timer = setTimeout(() => {
      setSearch(searchInput)
      setPage(1)
    }, 300)
    return () => clearTimeout(timer)
  }, [searchInput])

  // ── Render ──────────────────────────────────────────────────────────────

  if (authorized === null) {
    return (
      <>
        <Navbar />
        <main className="page-shell"><div className="loading-center"><div className="spinner" /></div></main>
      </>
    )
  }

  if (!authorized) return null

  return (
    <>
      <Navbar />
      <main className="page-shell">
        <div className="mgmt-container">
          <div className="mgmt-header">
            <h1>Super Admin</h1>
          </div>

          {flash && (
            <div className={`flash flash-${flash.type}`} onClick={() => setFlash(null)}>
              {flash.msg}
            </div>
          )}

          {/* ── Platform Overview ──────────────────────────────────────── */}
          <div className="sa-stats-grid">
            <div className="sa-stat-card">
              <p className="sa-stat-label">Total Tenants</p>
              <p className="sa-stat-value">{stats ? fmtNum(stats.total_tenants) : '...'}</p>
              {stats && (
                <p className="sa-stat-sub">
                  {Object.entries(stats.tenants_by_plan)
                    .filter(([, v]) => v > 0)
                    .map(([k, v]) => `${v} ${k}`)
                    .join(' / ')}
                </p>
              )}
            </div>
            <div className="sa-stat-card">
              <p className="sa-stat-label">Active Users</p>
              <p className="sa-stat-value">{stats ? fmtNum(stats.total_users) : '...'}</p>
            </div>
            <div className="sa-stat-card">
              <p className="sa-stat-label">Active API Keys</p>
              <p className="sa-stat-value">{stats ? fmtNum(stats.total_api_keys) : '...'}</p>
            </div>
            <div className="sa-stat-card">
              <p className="sa-stat-label">30-Day Usage</p>
              <p className="sa-stat-value">{stats ? fmtNum(stats.usage_count_30d) : '...'}</p>
              {stats && (
                <p className="sa-stat-sub">{fmtCost(stats.total_cost_30d)} total cost</p>
              )}
            </div>
          </div>

          {/* ── Tenant Management ─────────────────────────────────────── */}
          <div className="mgmt-section">
            <div className="section-hdr">
              <div>
                <h2>Tenant Management</h2>
                <p className="section-desc">View and manage all platform tenants.</p>
              </div>
              <span className="section-count">{total} tenant{total !== 1 ? 's' : ''}</span>
            </div>

            <div className="sa-filters">
              <input
                type="text"
                placeholder="Search by name or email..."
                value={searchInput}
                onChange={e => setSearchInput(e.target.value)}
              />
              <select value={planFilter} onChange={e => { setPlanFilter(e.target.value); setPage(1) }}>
                <option value="">All Plans</option>
                {PLANS.map(p => <option key={p} value={p}>{p}</option>)}
              </select>
              <select value={statusFilter} onChange={e => { setStatusFilter(e.target.value); setPage(1) }}>
                <option value="">All Statuses</option>
                <option value="active">Active</option>
                <option value="suspended">Suspended</option>
              </select>
            </div>

            <div className="table-scroll">
              <table className="mgmt-table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Name</th>
                    <th>Plan</th>
                    <th>Status</th>
                    <th>Members</th>
                    <th>API Keys</th>
                    <th>Created</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {tenants.length === 0 ? (
                    <tr><td colSpan={8} className="empty-cell">No tenants found.</td></tr>
                  ) : tenants.map(t => (
                    <tr key={t.id}>
                      <td className="text-muted">{t.id}</td>
                      <td>{t.name || <span className="text-muted">—</span>}</td>
                      <td>{planBadge(t.plan)}</td>
                      <td>{statusBadge(t.status)}</td>
                      <td>{t.member_count}</td>
                      <td>{t.api_key_count}</td>
                      <td className="text-muted">{fmtDate(t.created_at)}</td>
                      <td>
                        <div className="actions-cell">
                          <button className="btn btn-small" onClick={() => openDetail(t.id)}>View</button>
                          <button className="btn btn-small" onClick={() => openChangePlan({ id: t.id, name: t.name, plan: t.plan })}>
                            Change Plan
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {totalPages > 1 && (
              <div className="sa-pagination">
                <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}>Previous</button>
                <span>Page {page} of {totalPages}</span>
                <button disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>Next</button>
              </div>
            )}
          </div>
        </div>

        {/* ── Tenant Detail Modal ──────────────────────────────────────── */}
        {(detail || detailLoading) && (
          <div className="modal-overlay" onClick={() => { setDetail(null); setDetailLoading(false) }}>
            <div className="modal-box modal-lg" onClick={e => e.stopPropagation()}>
              {detailLoading && !detail ? (
                <div className="modal-body"><div className="loading-center"><div className="spinner" /></div></div>
              ) : detail && (
                <>
                  <div className="modal-hdr">
                    <h2>{detail.tenant.name || `Tenant #${detail.tenant.id}`}</h2>
                  </div>
                  <div className="modal-body">
                    {/* Tenant info */}
                    <div className="sa-detail-section">
                      <h3>Tenant Info</h3>
                      <div className="sa-detail-grid">
                        <div>
                          <div className="sa-detail-label">ID</div>
                          <div className="sa-detail-value">{detail.tenant.id}</div>
                        </div>
                        <div>
                          <div className="sa-detail-label">Plan</div>
                          <div className="sa-detail-value">{planBadge(detail.tenant.plan)}</div>
                        </div>
                        <div>
                          <div className="sa-detail-label">Status</div>
                          <div className="sa-detail-value">{statusBadge(detail.tenant.status)}</div>
                        </div>
                        <div>
                          <div className="sa-detail-label">Billing Email</div>
                          <div className="sa-detail-value">{detail.tenant.billing_email || '—'}</div>
                        </div>
                        <div>
                          <div className="sa-detail-label">Stripe Customer</div>
                          <div className="sa-detail-value">{detail.tenant.stripe_customer_id || '—'}</div>
                        </div>
                        <div>
                          <div className="sa-detail-label">Stripe Subscription</div>
                          <div className="sa-detail-value">{detail.tenant.stripe_subscription_id || '—'}</div>
                        </div>
                        <div>
                          <div className="sa-detail-label">Created</div>
                          <div className="sa-detail-value">{fmtDate(detail.tenant.created_at)}</div>
                        </div>
                        {detail.tenant.pending_plan && (
                          <div>
                            <div className="sa-detail-label">Pending Plan</div>
                            <div className="sa-detail-value">
                              {planBadge(detail.tenant.pending_plan)}
                              {detail.tenant.plan_effective_at && ` (${fmtDate(detail.tenant.plan_effective_at)})`}
                            </div>
                          </div>
                        )}
                      </div>
                    </div>

                    {/* Quick stats */}
                    <div className="sa-detail-section">
                      <h3>Quick Stats</h3>
                      <div className="sa-detail-grid">
                        <div>
                          <div className="sa-detail-label">API Keys</div>
                          <div className="sa-detail-value">{detail.api_key_count}</div>
                        </div>
                        <div>
                          <div className="sa-detail-label">Provider Keys</div>
                          <div className="sa-detail-value">{detail.provider_key_count}</div>
                        </div>
                        <div>
                          <div className="sa-detail-label">Projects</div>
                          <div className="sa-detail-value">{detail.project_count}</div>
                        </div>
                        <div>
                          <div className="sa-detail-label">30d Usage</div>
                          <div className="sa-detail-value">{fmtNum(detail.usage_count_30d)} ({fmtCost(detail.total_cost_30d)})</div>
                        </div>
                      </div>
                    </div>

                    {/* Members */}
                    <div className="sa-detail-section">
                      <h3>Members ({detail.members.length})</h3>
                      <div className="table-scroll">
                        <table className="mgmt-table">
                          <thead>
                            <tr>
                              <th>Email</th>
                              <th>Name</th>
                              <th>Role</th>
                              <th>Status</th>
                            </tr>
                          </thead>
                          <tbody>
                            {detail.members.length === 0 ? (
                              <tr><td colSpan={4} className="empty-cell">No members.</td></tr>
                            ) : detail.members.map(m => (
                              <tr key={m.user_id}>
                                <td>{m.email}</td>
                                <td>{m.name || '—'}</td>
                                <td><span className={`role-badge role-${m.org_role}`}>{m.org_role}</span></td>
                                <td>{statusBadge(m.status)}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </div>

                    {/* Actions */}
                    <div className="sa-detail-actions">
                      <button
                        className="btn btn-small"
                        onClick={() => {
                          setDetail(null)
                          openChangePlan({ id: detail.tenant.id, name: detail.tenant.name, plan: detail.tenant.plan })
                        }}
                      >
                        Change Plan
                      </button>
                      <button
                        className={`btn btn-small ${detail.tenant.status === 'active' ? 'btn-warning' : 'btn-primary'}`}
                        onClick={() => toggleTenantStatus(detail.tenant.id, detail.tenant.status)}
                      >
                        {detail.tenant.status === 'active' ? 'Suspend' : 'Unsuspend'}
                      </button>
                    </div>
                  </div>
                  <div className="modal-ftr">
                    <button className="btn" onClick={() => setDetail(null)}>Close</button>
                  </div>
                </>
              )}
            </div>
          </div>
        )}

        {/* ── Change Plan Modal ────────────────────────────────────────── */}
        {changePlanTenant && (
          <div className="modal-overlay" onClick={() => setChangePlanTenant(null)}>
            <div className="modal-box modal-md" onClick={e => e.stopPropagation()}>
              <div className="modal-hdr">
                <h2>Change Plan — {changePlanTenant.name || `Tenant #${changePlanTenant.id}`}</h2>
              </div>
              <div className="modal-body">
                <p style={{ margin: '0 0 0.5rem', fontSize: '0.875rem', color: 'var(--color-text-muted)' }}>
                  Current plan: {planBadge(changePlanTenant.currentPlan)}
                </p>

                <div className="sa-plan-options">
                  {PLANS.map(p => (
                    <label
                      key={p}
                      className={`sa-plan-option${selectedPlan === p ? ' selected' : ''}`}
                    >
                      <input
                        type="radio"
                        name="plan"
                        value={p}
                        checked={selectedPlan === p}
                        onChange={() => { setSelectedPlan(p); setChangePlanError('') }}
                      />
                      <span className="sa-plan-option-label">{p}</span>
                      {p === changePlanTenant.currentPlan && (
                        <span className="text-muted" style={{ fontSize: '0.75rem' }}>(current)</span>
                      )}
                    </label>
                  ))}
                </div>

                {changePlanError && (
                  <div className="flash flash-error" style={{ marginTop: '0.75rem' }}>
                    {changePlanError}
                  </div>
                )}
              </div>
              <div className="modal-ftr">
                <button className="btn" onClick={() => setChangePlanTenant(null)}>Cancel</button>
                <button
                  className="btn btn-primary"
                  disabled={changePlanLoading || selectedPlan === changePlanTenant.currentPlan}
                  onClick={submitChangePlan}
                >
                  {changePlanLoading ? 'Changing...' : 'Confirm'}
                </button>
              </div>
            </div>
          </div>
        )}
      </main>
    </>
  )
}
