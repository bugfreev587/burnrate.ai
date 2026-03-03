import { useState, useCallback, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useUser } from '@clerk/clerk-react'
import Navbar from '../components/Navbar'
import { useDashboardSummary } from '../hooks/useDashboardSummary'
import type {
  BreakdownItem, SpendLimitEntry, RecentRequestRow,
  DatePreset, BillingModeFilter, TimeseriesPoint,
} from '../hooks/useDashboardSummary'
import { useDashboardConfig } from '../hooks/useDashboardConfig'
import { useProjects } from '../hooks/useProjects'
import './DashboardV2.css'

// ─── Helpers ──────────────────────────────────────────────────────────────────

function fmt$(v: string | number): string {
  const n = typeof v === 'string' ? parseFloat(v) : v
  if (isNaN(n)) return '$0.00'
  if (n >= 1000) return '$' + n.toFixed(2).replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  if (n >= 0.01) return '$' + n.toFixed(2)
  return '$' + n.toFixed(4)
}

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(2) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
  return n.toString()
}

function fmtNum(n: number): string {
  return n.toLocaleString()
}

function fmtMs(n: number): string {
  if (n >= 1000) return (n / 1000).toFixed(2) + 's'
  return Math.round(n) + 'ms'
}

function fmtPct(n: number): string {
  return n.toFixed(1) + '%'
}

// ─── Delta Badge ──────────────────────────────────────────────────────────────

function DeltaBadge({ pct, invert }: { pct: number; invert?: boolean }) {
  if (pct === 0) return <span className="dv2-delta dv2-delta-neutral">0%</span>
  const up = pct > 0
  const arrow = up ? '\u2191' : '\u2193'
  let cls: string
  if (invert) {
    cls = up ? 'dv2-delta dv2-delta-up-good' : 'dv2-delta dv2-delta-down-bad'
  } else {
    cls = up ? 'dv2-delta dv2-delta-up' : 'dv2-delta dv2-delta-down'
  }
  return <span className={cls}>{arrow} {Math.abs(pct).toFixed(1)}%</span>
}

// ─── Skeleton ─────────────────────────────────────────────────────────────────

function Skeleton({ size = 'lg' }: { size?: 'lg' | 'md' | 'sm' }) {
  return <div className={`dv2-skeleton dv2-skeleton-${size}`} />
}

// ─── KPI Card ─────────────────────────────────────────────────────────────────

function KpiCard({ label, value, sub, delta, invert, small }: {
  label: string; value: string; sub?: string; delta?: number; invert?: boolean; small?: boolean
}) {
  return (
    <div className="card dv2-kpi-card">
      <div className="dv2-kpi-label">{label}</div>
      <div className={`dv2-kpi-value${small ? ' dv2-kpi-value-sm' : ''}`}>{value}</div>
      {sub && <div className="dv2-kpi-sub">{sub}</div>}
      {delta !== undefined && <DeltaBadge pct={delta} invert={invert} />}
    </div>
  )
}

// ─── Collapsible Section ──────────────────────────────────────────────────────

function Section({ id, title, tags, children, defaultOpen = true }: {
  id: string; title: string; tags?: { label: string; cls: string }[]
  children: React.ReactNode; defaultOpen?: boolean
}) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className="dv2-section" id={id}>
      <div className="dv2-section-header" onClick={() => setOpen(!open)}>
        <div className="dv2-section-title-row">
          <h2 className="dv2-section-title">{title}</h2>
          {tags?.map((t, i) => (
            <span key={i} className={`dv2-section-tag ${t.cls}`}>{t.label}</span>
          ))}
        </div>
        <button className="dv2-section-toggle" onClick={e => { e.stopPropagation(); setOpen(!open) }}>
          {open ? 'Collapse' : 'Expand'}
        </button>
      </div>
      {open && <div className="dv2-section-body">{children}</div>}
    </div>
  )
}

// ─── Upgrade CTA ──────────────────────────────────────────────────────────────

function UpgradeCTA({ minPlan }: { minPlan: string }) {
  const navigate = useNavigate()
  return (
    <div className="dv2-upgrade-cta">
      <p>Available on {minPlan}+</p>
      <button className="btn btn-primary" onClick={() => navigate('/plan')}>
        View Plans
      </button>
    </div>
  )
}

// ─── Mini Line Chart ──────────────────────────────────────────────────────────

function MiniLineChart({ data, valueKey, formatter, title, note }: {
  data: TimeseriesPoint[]; valueKey: string; formatter: (v: number) => string
  title?: string; note?: string
}) {
  const [hoverIdx, setHoverIdx] = useState<number | null>(null)

  if (!data || data.length === 0) {
    return <div className="dv2-chart-empty">No data for the selected period.</div>
  }

  const values = data.map(d => {
    const raw = (d as unknown as Record<string, unknown>)[valueKey]
    return typeof raw === 'string' ? parseFloat(raw) : (typeof raw === 'number' ? raw : 0)
  })
  const maxVal = Math.max(...values, 0.0001)

  const W = 600, H = 140
  const PAD = { top: 10, right: 8, bottom: 26, left: 52 }
  const chartW = W - PAD.left - PAD.right
  const chartH = H - PAD.top - PAD.bottom

  const xOf = (i: number) => PAD.left + (i / (data.length - 1 || 1)) * chartW
  const yOf = (v: number) => PAD.top + chartH - (v / maxVal) * chartH

  const pts = values.map((v, i) => `${xOf(i)},${yOf(v)}`).join(' ')
  const areaPts = [
    `${PAD.left},${PAD.top + chartH}`,
    ...values.map((v, i) => `${xOf(i)},${yOf(v)}`),
    `${PAD.left + chartW},${PAD.top + chartH}`,
  ].join(' ')

  const labelStep = Math.ceil(data.length / 5)
  const yTicks = [
    { val: 0, label: formatter(0) },
    { val: maxVal / 2, label: formatter(maxVal / 2) },
    { val: maxVal, label: formatter(maxVal) },
  ]

  const hp = hoverIdx !== null ? data[hoverIdx] : null

  return (
    <div className="card dv2-chart-card">
      {title && <div className="dv2-chart-title">{title}</div>}
      <svg viewBox={`0 0 ${W} ${H}`} className="dv2-chart-svg" preserveAspectRatio="none">
        {/* Y-axis */}
        {yTicks.map((t, i) => (
          <g key={i}>
            <line x1={PAD.left} y1={yOf(t.val)} x2={W - PAD.right} y2={yOf(t.val)}
              stroke="rgba(255,255,255,0.06)" strokeWidth="1" />
            <text x={PAD.left - 6} y={yOf(t.val) + 3} textAnchor="end"
              fill="var(--color-text-muted)" fontSize="9">{t.label}</text>
          </g>
        ))}
        {/* Area + line */}
        <polygon points={areaPts} fill="url(#dv2Grad)" opacity="0.15" />
        <polyline points={pts} fill="none" stroke="var(--color-primary)" strokeWidth="2" />
        {/* X-axis labels */}
        {data.map((d, i) => i % labelStep === 0 ? (
          <text key={i} x={xOf(i)} y={H - 4} textAnchor="middle"
            fill="var(--color-text-muted)" fontSize="9">{d.ts?.slice(5) ?? ''}</text>
        ) : null)}
        {/* Hover rects */}
        {data.map((_, i) => (
          <rect key={i} x={xOf(i) - chartW / data.length / 2} y={PAD.top}
            width={chartW / data.length} height={chartH}
            fill="transparent" onMouseEnter={() => setHoverIdx(i)} onMouseLeave={() => setHoverIdx(null)} />
        ))}
        {/* Hover indicator */}
        {hoverIdx !== null && (
          <>
            <circle cx={xOf(hoverIdx)} cy={yOf(values[hoverIdx])} r="4" fill="var(--color-primary)" />
            <text x={xOf(hoverIdx)} y={yOf(values[hoverIdx]) - 10} textAnchor="middle"
              fill="var(--color-text)" fontSize="10" fontWeight="600">
              {formatter(values[hoverIdx])}
            </text>
          </>
        )}
        <defs>
          <linearGradient id="dv2Grad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--color-primary)" stopOpacity="0.4" />
            <stop offset="100%" stopColor="var(--color-primary)" stopOpacity="0" />
          </linearGradient>
        </defs>
      </svg>
      {note && <div className="dv2-chart-note">{note}</div>}
      {hp && (
        <div className="dv2-chart-note" style={{ fontWeight: 500 }}>
          {hp.ts}: {formatter(values[hoverIdx!])}
          {hp.requests !== undefined && ` | ${fmtNum(hp.requests)} requests`}
          {hp.tokens !== undefined && ` | ${fmtTokens(hp.tokens)} tokens`}
        </div>
      )}
    </div>
  )
}

// ─── Breakdown Table ──────────────────────────────────────────────────────────

function BreakdownTable({ items, showProvider }: { items: BreakdownItem[]; showProvider?: boolean }) {
  if (!items || items.length === 0) {
    return <div className="dv2-empty">No data for the selected period.</div>
  }
  return (
    <div className="dv2-table-wrap">
      <table className="dv2-breakdown-table">
        <thead>
          <tr>
            <th>Name</th>
            {showProvider && <th>Provider</th>}
            <th>Cost</th>
            <th>Tokens (In / Out)</th>
            <th>Requests</th>
            <th>% of Total</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item, i) => (
            <tr key={i}>
              <td>{item.name || '\u2014'}</td>
              {showProvider && <td style={{ textTransform: 'capitalize' }}>{item.provider || '\u2014'}</td>}
              <td>{fmt$(item.cost)}</td>
              <td>{fmtTokens(item.input_tokens)} / {fmtTokens(item.output_tokens)}</td>
              <td>{fmtNum(item.requests)}</td>
              <td>
                <span className="dv2-pct-bar" style={{ width: `${Math.min(item.pct_of_total, 100)}%`, maxWidth: 60 }} />
                {fmtPct(item.pct_of_total)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ─── Main Dashboard ───────────────────────────────────────────────────────────

export default function DashboardV2() {
  const { user } = useUser()
  const navigate = useNavigate()
  const { config } = useDashboardConfig()
  const { projects } = useProjects()
  const {
    summary, recentRequests, loading, error,
    filters, setFilters, applyPreset, refresh, fetchRecentRequests,
  } = useDashboardSummary()

  const [activeTab, setActiveTab] = useState<'api_key' | 'project' | 'model' | 'provider'>('api_key')
  const [recentExpanded, setRecentExpanded] = useState(false)
  const [recentLoading, setRecentLoading] = useState(false)

  const plan = summary?.plan ?? config?.plan ?? 'free'
  const isTeamPlus = plan === 'team' || plan === 'business'
  const isBusiness = plan === 'business'
  const isProPlus = plan !== 'free'

  // Load existing API keys for the dropdown
  const [apiKeys, setApiKeys] = useState<{ key_id: string; label: string }[]>([])
  useEffect(() => {
    import('../lib/api').then(({ apiFetch }) => {
      apiFetch('/v1/admin/api-keys').then(res => {
        if (res.ok) return res.json()
        return { api_keys: [] }
      }).then(data => {
        setApiKeys((data.api_keys ?? []).map((k: { key_id: string; label: string }) => ({
          key_id: k.key_id, label: k.label || k.key_id.slice(0, 12),
        })))
      }).catch(() => {})
    })
  }, [])

  const handleExpandRecent = useCallback(async () => {
    if (!recentExpanded) {
      setRecentExpanded(true)
      setRecentLoading(true)
      await fetchRecentRequests(100)
      setRecentLoading(false)
    }
  }, [recentExpanded, fetchRecentRequests])

  const handleShowMore = useCallback(async () => {
    setRecentLoading(true)
    await fetchRecentRequests(500)
    setRecentLoading(false)
  }, [fetchRecentRequests])

  const s = summary // shorthand

  // Retention hint
  const retentionHint = config
    ? config.retention.type === 'UNLIMITED'
      ? `Unlimited (since ${config.effective.min_start_date})`
      : `${config.retention.max_days}d (since ${config.effective.min_start_date})`
    : ''

  return (
    <div className="dv2-page">
      <Navbar />
      <div className="page-content">
        {/* ─── Header ────────────────────────────────────────────── */}
        <div style={{ marginBottom: '0.5rem' }}>
          <h1 style={{ fontSize: '1.75rem', fontWeight: 700, margin: '0 0 0.2rem' }}>Dashboard</h1>
          {user && (
            <p style={{ color: 'var(--color-text-muted)', fontSize: '0.875rem', margin: 0 }}>
              Welcome back, {user.firstName ?? 'there'}
            </p>
          )}
        </div>

        {/* ─── Sticky Toolbar ────────────────────────────────────── */}
        <div className="dv2-toolbar">
          {/* Date range */}
          <div className="dv2-toolbar-group">
            <span className="dv2-toolbar-label">Range</span>
            <select
              value={filters.preset}
              onChange={e => {
                const preset = e.target.value as DatePreset
                if (preset === 'custom') {
                  setFilters(f => ({ ...f, preset: 'custom' }))
                } else {
                  applyPreset(preset)
                }
              }}
            >
              <option value="1d">Last 24h</option>
              <option value="7d">Last 7 days</option>
              <option value="30d">Last 30 days</option>
              <option value="this_month">This month</option>
              <option value="last_month">Last month</option>
              <option value="custom">Custom</option>
            </select>
          </div>

          {filters.preset === 'custom' && (
            <div className="dv2-custom-dates">
              <input type="date" value={filters.from}
                min={config?.effective.min_start_date}
                max={filters.to}
                onChange={e => setFilters(f => ({ ...f, from: e.target.value }))} />
              <span className="dv2-toolbar-label">to</span>
              <input type="date" value={filters.to}
                min={filters.from}
                onChange={e => setFilters(f => ({ ...f, to: e.target.value }))} />
            </div>
          )}

          <div className="dv2-toolbar-sep" />

          {/* Billing mode */}
          <div className="dv2-toolbar-group">
            <span className="dv2-toolbar-label">Billing</span>
            <select
              value={filters.billing_mode}
              onChange={e => setFilters(f => ({ ...f, billing_mode: e.target.value as BillingModeFilter }))}
            >
              <option value="all">All</option>
              <option value="api_usage_billed">API Usage Billed</option>
              <option value="monthly_subscription">Monthly Subscription</option>
            </select>
          </div>

          <div className="dv2-toolbar-sep" />

          {/* Project filter (Team+) */}
          {isTeamPlus && (
            <div className="dv2-toolbar-group">
              <span className="dv2-toolbar-label">Project</span>
              <select
                value={filters.project_id}
                onChange={e => setFilters(f => ({ ...f, project_id: e.target.value }))}
              >
                <option value="">All projects</option>
                {projects.map(p => (
                  <option key={p.id} value={String(p.id)}>{p.name}</option>
                ))}
              </select>
            </div>
          )}

          {/* API key filter */}
          <div className="dv2-toolbar-group">
            <span className="dv2-toolbar-label">Key</span>
            <select
              value={filters.api_key_id}
              onChange={e => setFilters(f => ({ ...f, api_key_id: e.target.value }))}
            >
              <option value="">All keys</option>
              {apiKeys.map(k => (
                <option key={k.key_id} value={k.key_id}>{k.label}</option>
              ))}
            </select>
          </div>

          <div className="dv2-toolbar-sep" />

          <button className="btn btn-secondary" onClick={refresh}>Refresh</button>

          {retentionHint && (
            <span className="dv2-toolbar-retention">Retention: {retentionHint}</span>
          )}
        </div>

        {/* ─── Loading / Error ───────────────────────────────────── */}
        {loading && !s && (
          <div className="dv2-kpi-grid" style={{ marginBottom: '2rem' }}>
            {[1,2,3,4].map(i => (
              <div key={i} className="card dv2-kpi-card">
                <Skeleton size="sm" /><Skeleton size="lg" /><Skeleton size="sm" />
              </div>
            ))}
          </div>
        )}

        {error && (
          <div className="card" style={{ padding: '1.5rem', marginBottom: '1rem' }}>
            <p style={{ color: 'var(--color-danger)', margin: 0 }}>
              Failed to load dashboard: {error}
            </p>
            <button className="btn btn-secondary" style={{ marginTop: '0.5rem' }} onClick={refresh}>
              Retry
            </button>
          </div>
        )}

        {s && (
          <>
            {/* ═══ A. Executive Summary ═══════════════════════════════ */}
            <Section id="executive-summary" title="Executive Summary"
              tags={[{ label: 'All Plans', cls: 'dv2-tag-all' }]}>
              <div className="dv2-kpi-grid">
                <KpiCard
                  label="Total Spend"
                  value={fmt$(s.kpis.spend_total.value)}
                  sub={`${s.range.from} \u2013 ${s.range.to}`}
                  delta={s.kpis.spend_total.delta_pct}
                />
                <KpiCard
                  label="Projected Month-End"
                  value={fmt$(s.kpis.projected_month_end.value)}
                  sub={`${s.forecast.days_remaining}d remaining`}
                />
                <KpiCard
                  label="Budget Health"
                  value={s.kpis.budget_health.status === 'ok' ? 'OK' :
                    s.kpis.budget_health.status === 'warning' ? 'Warning' : 'Blocking Soon'}
                  sub={s.kpis.budget_health.message}
                  small
                />
                <KpiCard
                  label="Success Rate"
                  value={fmtPct(typeof s.kpis.success_rate.value === 'number' ? s.kpis.success_rate.value : parseFloat(String(s.kpis.success_rate.value)))}
                  invert
                />
              </div>

              {s.cost_note && (
                <div className="dv2-chart-note" style={{ marginBottom: '0.75rem' }}>
                  {s.cost_note}
                </div>
              )}

              {/* Mini insight row */}
              <div className="dv2-insight-strip">
                <div className="card dv2-insight-card">
                  <div className="dv2-insight-label">Spend Change</div>
                  <div className="dv2-insight-value">
                    <DeltaBadge pct={s.kpis.spend_total.delta_pct} />
                  </div>
                  <div className="dv2-insight-sub">vs previous period</div>
                </div>
                <div className="card dv2-insight-card">
                  <div className="dv2-insight-label">Requests Change</div>
                  <div className="dv2-insight-value">
                    <DeltaBadge pct={s.kpis.requests_total.delta_pct} invert />
                  </div>
                  <div className="dv2-insight-sub">vs previous period</div>
                </div>
                <div className="card dv2-insight-card">
                  <div className="dv2-insight-label">Avg Cost / Request</div>
                  <div className="dv2-insight-value">
                    {fmt$(s.kpis.avg_cost_per_request.value)}
                  </div>
                  <div className="dv2-insight-sub">
                    <DeltaBadge pct={s.kpis.avg_cost_per_request.delta_pct} />
                  </div>
                </div>
              </div>
            </Section>

            {/* ═══ B. Spend & Forecast ═══════════════════════════════ */}
            {isProPlus ? (
              <Section id="spend-forecast" title="Spend & Forecast"
                tags={[{ label: 'Pro+', cls: 'dv2-tag-api' }]}>
                <div className="dv2-forecast-grid">
                  <KpiCard label="Spend (Period)" value={fmt$(s.kpis.spend_total.value)} />
                  <KpiCard label="Spend This Month" value={fmt$(s.forecast.total_so_far)} />
                  <KpiCard label="Daily Average" value={fmt$(s.forecast.daily_average)} />
                  <KpiCard label="Projected Month-End" value={fmt$(s.forecast.forecast)}
                    sub={`${s.forecast.days_elapsed}d elapsed, ${s.forecast.days_remaining}d remaining`} />
                </div>

                {s.cost_note && (
                  <div className="dv2-chart-note" style={{ marginBottom: '0.75rem' }}>
                    {s.cost_note}
                  </div>
                )}

                <MiniLineChart
                  data={s.timeseries.daily_cost}
                  valueKey="cost"
                  formatter={v => fmt$(v)}
                  title="Daily Spend"
                />
              </Section>
            ) : (
              <Section id="spend-forecast" title="Spend & Forecast">
                <UpgradeCTA minPlan="Pro" />
              </Section>
            )}

            {/* ═══ C. Cost Attribution ═══════════════════════════════ */}
            <Section id="cost-attribution" title="Cost Attribution"
              tags={isTeamPlus ? [{ label: 'Team+', cls: 'dv2-tag-team' }] : [{ label: 'All Plans', cls: 'dv2-tag-all' }]}>

              {s.cost_note && (
                <div className="dv2-chart-note" style={{ marginBottom: '0.75rem' }}>
                  {s.cost_note}
                </div>
              )}

              {/* Cost Drivers insight panel */}
              {s.insights.cost_drivers.length > 0 && (
                <div className="card dv2-drivers">
                  <div className="dv2-drivers-title">Cost Drivers</div>
                  <ul>
                    {s.insights.cost_drivers.map((d, i) => (
                      <li key={i}>{d.text}</li>
                    ))}
                  </ul>
                </div>
              )}

              {/* Tabs */}
              <div className="dv2-tabs">
                <button className={`dv2-tab${activeTab === 'api_key' ? ' dv2-tab-active' : ''}`}
                  onClick={() => setActiveTab('api_key')}>By API Key</button>
                {isTeamPlus && (
                  <button className={`dv2-tab${activeTab === 'project' ? ' dv2-tab-active' : ''}`}
                    onClick={() => setActiveTab('project')}>By Project</button>
                )}
                <button className={`dv2-tab${activeTab === 'model' ? ' dv2-tab-active' : ''}`}
                  onClick={() => setActiveTab('model')}>By Model</button>
                <button className={`dv2-tab${activeTab === 'provider' ? ' dv2-tab-active' : ''}`}
                  onClick={() => setActiveTab('provider')}>By Provider</button>
              </div>

              {activeTab === 'api_key' && <BreakdownTable items={s.breakdowns.by_api_key} />}
              {activeTab === 'project' && isTeamPlus && <BreakdownTable items={s.breakdowns.by_project} />}
              {activeTab === 'model' && <BreakdownTable items={s.breakdowns.by_model} showProvider />}
              {activeTab === 'provider' && <BreakdownTable items={s.breakdowns.by_provider} />}
            </Section>

            {/* ═══ D. Limits & Risk ═════════════════════════════════ */}
            {isTeamPlus ? (
              <Section id="limits-risk" title="Limits & Risk"
                tags={[{ label: 'Team+', cls: 'dv2-tag-team' }]}>
                {/* Risk cards */}
                <div className="dv2-risk-grid">
                  <KpiCard label="Budget Utilization"
                    value={`${fmtPct(s.limits.budget_utilization_pct)}`} small />
                  <KpiCard label="Blocked (Budget)"
                    value={fmtNum(s.limits.blocked_requests)} small />
                  <KpiCard label="Rate Limited"
                    value={fmtNum(s.limits.rate_limited_requests)} small />
                  <KpiCard label="Spend Velocity"
                    value={s.kpis.budget_health.status === 'ok' ? 'Normal' : 'Elevated'}
                    sub={s.kpis.budget_health.message} small />
                </div>

                {/* Spend limits */}
                {s.limits.active_spend_limits.length > 0 && (
                  <>
                    <h3 style={{ fontSize: '0.85rem', fontWeight: 600, margin: '1rem 0 0.5rem', color: 'var(--color-text-muted)' }}>
                      Spend Limits
                    </h3>
                    <div className="dv2-limits-list">
                      {s.limits.active_spend_limits.map((l: SpendLimitEntry) => {
                        const fillColor = l.status === 'ok' ? '#22c55e' :
                          l.status === 'warning' ? '#f59e0b' : '#ef4444'
                        return (
                          <div key={l.id} className="card dv2-limit-card">
                            <div className="dv2-limit-info">
                              <div className="dv2-limit-scope">
                                {l.scope_type === 'api_key' ? (l.key_label || l.scope_id.slice(0, 12)) : 'Account'}
                                {l.provider && ` (${l.provider})`}
                              </div>
                              <div className="dv2-limit-details">
                                {l.period_type} | {fmt$(l.current_spend)} / {fmt$(l.limit_amount)} |{' '}
                                <span className={`dv2-status-chip dv2-status-${l.status}`}>{l.status}</span>
                                {' '}{l.action}
                              </div>
                            </div>
                            <div className="dv2-limit-bar">
                              <div className="dv2-limit-track">
                                <div className="dv2-limit-fill"
                                  style={{ width: `${Math.min(l.pct_used, 100)}%`, background: fillColor }} />
                              </div>
                              <div className="dv2-limit-pct" style={{ color: fillColor }}>
                                {fmtPct(l.pct_used)}
                              </div>
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  </>
                )}

                {/* Rate limits */}
                {s.limits.active_rate_limits.length > 0 && (
                  <>
                    <h3 style={{ fontSize: '0.85rem', fontWeight: 600, margin: '1rem 0 0.5rem', color: 'var(--color-text-muted)' }}>
                      Rate Limits
                    </h3>
                    <div className="dv2-table-wrap">
                      <table className="dv2-breakdown-table">
                        <thead>
                          <tr>
                            <th>Scope</th>
                            <th>Provider</th>
                            <th>Model</th>
                            <th>Metric</th>
                            <th>Limit</th>
                            <th>Window</th>
                          </tr>
                        </thead>
                        <tbody>
                          {s.limits.active_rate_limits.map((rl, i) => (
                            <tr key={i}>
                              <td>{rl.scope_type}{rl.scope_id ? ` (${rl.scope_id.slice(0, 8)})` : ''}</td>
                              <td>{rl.provider || 'All'}</td>
                              <td>{rl.model || 'All'}</td>
                              <td>{rl.metric.toUpperCase()}</td>
                              <td>{fmtNum(rl.limit_value)}</td>
                              <td>{rl.window_seconds}s</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </>
                )}

                {s.limits.active_spend_limits.length === 0 && s.limits.active_rate_limits.length === 0 && (
                  <div className="dv2-empty">No limits configured. <button className="btn-link" onClick={() => navigate('/limits')}>Set up limits</button></div>
                )}
              </Section>
            ) : (
              <Section id="limits-risk" title="Limits & Risk">
                <UpgradeCTA minPlan="Team" />
              </Section>
            )}

            {/* ═══ E. Performance & Reliability ═════════════════════ */}
            <Section id="performance" title="Performance & Reliability"
              tags={[{ label: 'All Plans', cls: 'dv2-tag-all' }]}>
              <div className="dv2-perf-grid">
                <KpiCard label="p50 Latency" value={fmtMs(s.latency.p50)} small />
                <KpiCard label="p95 Latency" value={fmtMs(s.latency.p95)} small />
                <KpiCard label="p99 Latency" value={fmtMs(s.latency.p99)} small />
                <KpiCard label="Avg Latency" value={fmtMs(s.latency.avg)} small />
                <KpiCard label="Samples" value={fmtNum(s.latency.sample_count)} small />
              </div>

              <MiniLineChart
                data={s.timeseries.daily_latency_p95}
                valueKey="ms"
                formatter={v => fmtMs(v)}
                title="Daily p95 Latency"
                note="Gateway processing overhead only (excludes upstream provider time)."
              />

              <MiniLineChart
                data={s.timeseries.outcomes}
                valueKey="success"
                formatter={v => fmtNum(Math.round(v))}
                title="Request Outcomes (Success)"
              />
            </Section>

            {/* ═══ F. Activity & Governance ═════════════════════════ */}
            {isTeamPlus ? (
              <Section id="governance" title="Activity & Governance"
                tags={[{ label: 'Team+', cls: 'dv2-tag-team' }]}>
                <div className="dv2-gov-grid">
                  <KpiCard label="Active API Keys" value={fmtNum(s.governance.active_api_keys)} small />
                  <KpiCard label="Active Projects" value={fmtNum(s.governance.active_projects)} small />
                  <KpiCard label="Audit Events (7d)" value={fmtNum(s.governance.audit_events_7d)} small />
                  <KpiCard label="Keys Revoked" value={fmtNum(s.governance.revoked_keys_period)} small />
                </div>

                {/* Business: Spend Concentration */}
                {isBusiness && (
                  <>
                    <h3 style={{ fontSize: '0.85rem', fontWeight: 600, margin: '0.5rem 0', color: 'var(--color-text-muted)' }}>
                      Spend Concentration Risk
                    </h3>
                    <div className="dv2-conc-grid">
                      <div className="card dv2-kpi-card">
                        <div className="dv2-kpi-label">Top API Key</div>
                        <div className="dv2-kpi-value dv2-kpi-value-sm">{fmtPct(s.insights.concentration.top_api_key_pct)}</div>
                        <div className={`dv2-conc-indicator ${s.insights.concentration.top_api_key_pct > 60 ? 'dv2-conc-high' : 'dv2-conc-ok'}`}>
                          {s.insights.concentration.top_api_key_pct > 60 ? 'High concentration' : 'Distributed'}
                        </div>
                      </div>
                      <div className="card dv2-kpi-card">
                        <div className="dv2-kpi-label">Top Project</div>
                        <div className="dv2-kpi-value dv2-kpi-value-sm">{fmtPct(s.insights.concentration.top_project_pct)}</div>
                        <div className={`dv2-conc-indicator ${s.insights.concentration.top_project_pct > 60 ? 'dv2-conc-high' : 'dv2-conc-ok'}`}>
                          {s.insights.concentration.top_project_pct > 60 ? 'High concentration' : 'Distributed'}
                        </div>
                      </div>
                      <div className="card dv2-kpi-card">
                        <div className="dv2-kpi-label">Top Model</div>
                        <div className="dv2-kpi-value dv2-kpi-value-sm">{fmtPct(s.insights.concentration.top_model_pct)}</div>
                        <div className={`dv2-conc-indicator ${s.insights.concentration.top_model_pct > 60 ? 'dv2-conc-high' : 'dv2-conc-ok'}`}>
                          {s.insights.concentration.top_model_pct > 60 ? 'High concentration' : 'Distributed'}
                        </div>
                      </div>
                    </div>
                  </>
                )}

                <div className="dv2-quick-links">
                  <button className="btn btn-secondary" onClick={() => navigate('/audit')}>View Audit Logs</button>
                  <button className="btn btn-secondary" onClick={() => navigate('/audit')}>Generate Audit Report</button>
                  <button className="btn btn-secondary" onClick={() => {
                    // Export CSV - trigger download
                    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone
                    const qs = new URLSearchParams({ from: filters.from, to: filters.to, billing_mode: filters.billing_mode, tz })
                    if (filters.project_id) qs.set('project_id', filters.project_id)
                    if (filters.api_key_id) qs.set('api_key_id', filters.api_key_id)
                    import('../lib/api').then(({ apiFetch }) => {
                      apiFetch(`/v1/dashboard/recent-requests?${qs}&limit=500`).then(res => res.json()).then(data => {
                        const rows = data.requests ?? []
                        if (rows.length === 0) return
                        const headers = ['timestamp', 'provider', 'model', 'key_label', 'prompt_tokens', 'completion_tokens', 'cost', 'latency_ms', 'result']
                        const csv = [headers.join(','), ...rows.map((r: RecentRequestRow) =>
                          [r.created_at, r.provider, r.model, r.key_label, r.prompt_tokens, r.completion_tokens, r.cost, r.latency_ms, r.result].join(',')
                        )].join('\n')
                        const blob = new Blob([csv], { type: 'text/csv' })
                        const url = URL.createObjectURL(blob)
                        const a = document.createElement('a')
                        a.href = url; a.download = `usage-export-${filters.from}-${filters.to}.csv`
                        a.click(); URL.revokeObjectURL(url)
                      })
                    })
                  }}>Export Usage CSV</button>
                </div>
              </Section>
            ) : (
              <Section id="governance" title="Activity & Governance">
                <UpgradeCTA minPlan="Team" />
              </Section>
            )}

            {/* ═══ Recent Requests (collapsed by default) ══════════ */}
            <Section id="recent-requests" title="Recent Requests"
              tags={[{ label: 'All Plans', cls: 'dv2-tag-all' }]}
              defaultOpen={false}>
              {!recentExpanded ? (
                <div className="dv2-empty">
                  <button className="btn btn-secondary" onClick={handleExpandRecent}>
                    Load Recent Requests
                  </button>
                </div>
              ) : recentLoading ? (
                <div className="dv2-empty">Loading requests...</div>
              ) : recentRequests.length === 0 ? (
                <div className="dv2-empty">No requests in this period.</div>
              ) : (
                <>
                  <div className="dv2-table-wrap">
                    <table className="dv2-requests-table">
                      <thead>
                        <tr>
                          <th>Timestamp</th>
                          <th>Provider</th>
                          <th>Model</th>
                          <th>API Key</th>
                          <th>Input Tokens</th>
                          <th>Output Tokens</th>
                          <th>Cost</th>
                          <th>Result</th>
                          <th>Latency</th>
                        </tr>
                      </thead>
                      <tbody>
                        {recentRequests.map(r => (
                          <tr key={r.id}>
                            <td style={{ whiteSpace: 'nowrap', fontSize: '0.75rem' }}>
                              {new Date(r.created_at).toLocaleString()}
                            </td>
                            <td style={{ textTransform: 'capitalize' }}>{r.provider}</td>
                            <td><code style={{ fontSize: '0.75rem' }}>{r.model}</code></td>
                            <td>{r.key_label || r.key_id?.slice(0, 8) || '\u2014'}</td>
                            <td style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>
                              {fmtTokens(r.prompt_tokens)}
                            </td>
                            <td style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>
                              {fmtTokens(r.completion_tokens)}
                            </td>
                            <td style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>
                              {fmt$(r.cost)}
                            </td>
                            <td>
                              <span className={`dv2-result-${r.result}`}>{r.result}</span>
                            </td>
                            <td style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>
                              {r.latency_ms > 0 ? fmtMs(r.latency_ms) : '\u2014'}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  <div className="dv2-requests-footer">
                    <span className="dv2-requests-count">{fmtNum(recentRequests.length)} requests</span>
                    {recentRequests.length <= 100 && (
                      <button className="btn-link" onClick={handleShowMore}>Show 500 requests</button>
                    )}
                  </div>
                </>
              )}
            </Section>
          </>
        )}
      </div>
    </div>
  )
}

