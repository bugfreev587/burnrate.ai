import { useState } from 'react'
import { useUser } from '@clerk/clerk-react'
import Navbar from '../components/Navbar'
import DateRangeSelector from '../components/DateRangeSelector'
import { useUsageData } from '../hooks/useUsageData'
import type { DateRange, BudgetStatus, DailyTrend, ModelBreakdown } from '../hooks/useUsageData'
import { useDashboardConfig } from '../hooks/useDashboardConfig'
import './Dashboard.css'

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

function growthPct(current: string, prior: string): number | null {
  const c = parseFloat(current)
  const p = parseFloat(prior)
  if (isNaN(c) || isNaN(p) || p === 0) return null
  return ((c - p) / p) * 100
}

function GrowthPill({ pct }: { pct: number | null }) {
  if (pct === null) return null
  const up = pct >= 0
  return (
    <span className={`growth-pill ${up ? 'growth-up' : 'growth-down'}`}>
      {up ? '↑' : '↓'} {Math.abs(pct).toFixed(1)}%
    </span>
  )
}

// ─── Budget Bar ───────────────────────────────────────────────────────────────

function BudgetBar({ b }: { b: BudgetStatus }) {
  const pct = Math.min(b.pct_used, 100)
  const atAlert = b.pct_used >= parseFloat(b.alert_threshold)
  const atLimit = b.pct_used >= 100
  const color = atLimit ? 'var(--color-danger)' : atAlert ? '#f59e0b' : 'var(--color-primary)'
  const remaining = (parseFloat(b.limit_amount) - parseFloat(b.current_spend)).toFixed(2)

  return (
    <div className="budget-bar-card card">
      <div className="budget-bar-header">
        <div>
          <span className="budget-label">
            {b.period_type.charAt(0).toUpperCase() + b.period_type.slice(1)} Budget
            {b.scope_type === 'api_key' && b.scope_id && (
              <span className="budget-scope"> · key {b.scope_id.slice(0, 8)}…</span>
            )}
          </span>
          <span className="budget-action" style={{ color: b.action === 'block' ? 'var(--color-danger)' : '#f59e0b' }}>
            {b.action === 'block' ? ' · Hard Block' : ' · Alert only'}
          </span>
        </div>
        <span className="budget-amounts">
          <span style={{ color }}>{fmt$(b.current_spend)}</span>
          <span className="budget-sep"> / </span>
          <span>${parseFloat(b.limit_amount).toFixed(2)}</span>
        </span>
      </div>
      <div className="budget-track">
        <div
          className="budget-fill"
          style={{ width: `${pct}%`, background: color }}
        />
      </div>
      <div className="budget-bar-footer">
        <span className="budget-pct" style={{ color }}>{pct.toFixed(1)}% used</span>
        <span className="budget-remaining">
          {parseFloat(remaining) >= 0
            ? `${fmt$(remaining)} remaining`
            : `${fmt$(Math.abs(parseFloat(remaining)))} over limit`}
        </span>
      </div>
    </div>
  )
}

// ─── Daily Trend SVG Chart ────────────────────────────────────────────────────

function fmtYTick(val: number, mode: 'cost' | 'tokens'): string {
  if (mode === 'cost') {
    if (val === 0) return '$0'
    if (val >= 1000) return '$' + (val / 1000).toFixed(1) + 'K'
    if (val >= 1) return '$' + val.toFixed(2)
    if (val >= 0.01) return '$' + val.toFixed(3)
    return '$' + val.toFixed(4)
  } else {
    if (val === 0) return '0'
    if (val >= 1_000_000) return (val / 1_000_000).toFixed(1) + 'M'
    if (val >= 1_000) return (val / 1_000).toFixed(1) + 'K'
    return Math.round(val).toString()
  }
}

function fmtDate(s: string | null | undefined): string {
  if (!s) return '—'
  const d = new Date(s)
  if (isNaN(d.getTime())) return '—'
  return d.toLocaleString()
}

function TrendChart({ data, mode }: { data: DailyTrend[]; mode: 'cost' | 'tokens' }) {
  if (data.length === 0) {
    return <div className="trend-empty">No data for the selected period.</div>
  }

  const values = data.map(d => mode === 'cost' ? parseFloat(d.cost) : d.tokens)
  const maxVal = Math.max(...values, 0.0001)

  const W = 600
  const H = 132
  // Left padding is wide enough for Y-axis tick labels; bottom for X-axis labels.
  const PAD = { top: 10, right: 8, bottom: 26, left: 52 }
  const chartW = W - PAD.left - PAD.right
  const chartH = H - PAD.top - PAD.bottom

  const xOf = (i: number) => PAD.left + (i / (data.length - 1 || 1)) * chartW
  const yOf = (v: number) => PAD.top + chartH - (v / maxVal) * chartH

  const pts = data.map((d, i) => {
    const v = mode === 'cost' ? parseFloat(d.cost) : d.tokens
    return `${xOf(i)},${yOf(v)}`
  }).join(' ')

  const areaBase = PAD.top + chartH
  const areaPts = [
    `${PAD.left},${areaBase}`,
    ...data.map((d, i) => {
      const v = mode === 'cost' ? parseFloat(d.cost) : d.tokens
      return `${xOf(i)},${yOf(v)}`
    }),
    `${PAD.left + chartW},${areaBase}`,
  ].join(' ')

  const labelStep = Math.ceil(data.length / 5)

  // Three Y-axis ticks: 0, half, max.
  const yTicks = [
    { val: 0,          label: fmtYTick(0, mode) },
    { val: maxVal / 2, label: fmtYTick(maxVal / 2, mode) },
    { val: maxVal,     label: fmtYTick(maxVal, mode) },
  ]

  // Rotated axis title mid-point.
  const axisTitleX = 10
  const axisTitleY = PAD.top + chartH / 2

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="trend-svg" preserveAspectRatio="none">
      <defs>
        <linearGradient id={`grad-${mode}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="var(--color-primary)" stopOpacity="0.3" />
          <stop offset="100%" stopColor="var(--color-primary)" stopOpacity="0.02" />
        </linearGradient>
      </defs>

      {/* Y-axis title (rotated) */}
      <text
        x={axisTitleX}
        y={axisTitleY}
        textAnchor="middle"
        fontSize="8.5"
        fill="var(--color-text-muted)"
        transform={`rotate(-90, ${axisTitleX}, ${axisTitleY})`}
      >
        {mode === 'cost' ? 'Cost (USD)' : 'Tokens'}
      </text>

      {/* Y-axis ticks: gridlines + labels */}
      {yTicks.map((tick, i) => {
        const y = yOf(tick.val)
        return (
          <g key={i}>
            <line
              x1={PAD.left} y1={y}
              x2={PAD.left + chartW} y2={y}
              stroke="rgba(255,255,255,0.06)"
              strokeWidth="1"
            />
            <text
              x={PAD.left - 4}
              y={y + 3}
              textAnchor="end"
              fontSize="8"
              fill="var(--color-text-muted)"
            >
              {tick.label}
            </text>
          </g>
        )
      })}

      {/* Y-axis vertical line */}
      <line
        x1={PAD.left} y1={PAD.top}
        x2={PAD.left} y2={PAD.top + chartH}
        stroke="rgba(255,255,255,0.1)"
        strokeWidth="1"
      />

      {/* Area fill */}
      <polygon points={areaPts} fill={`url(#grad-${mode})`} />
      {/* Line */}
      <polyline points={pts} fill="none" stroke="var(--color-primary)" strokeWidth="2" strokeLinejoin="round" />
      {/* Dots */}
      {data.map((d, i) => {
        const v = mode === 'cost' ? parseFloat(d.cost) : d.tokens
        return <circle key={i} cx={xOf(i)} cy={yOf(v)} r="3" fill="var(--color-primary)" />
      })}

      {/* X-axis labels */}
      {data.map((d, i) => {
        if (i % labelStep !== 0 && i !== data.length - 1) return null
        const label = d.date.slice(5) // MM-DD
        return (
          <text key={i} x={xOf(i)} y={H - 4} textAnchor="middle" fontSize="9" fill="var(--color-text-muted)">
            {label}
          </text>
        )
      })}
    </svg>
  )
}

// ─── Recent Requests ──────────────────────────────────────────────────────────

const RECENT_DEFAULT_LIMIT = 10

function RecentRequests({ logs }: { logs: ReturnType<typeof useUsageData>['logs'] }) {
  const [expanded, setExpanded] = useState(false)

  if (logs.length === 0) {
    return (
      <div className="empty-state">
        <p>No recent requests.</p>
        <p className="empty-hint">
          Configure your Claude Code client to report usage via the gateway API.
        </p>
      </div>
    )
  }

  const hasMore = logs.length > RECENT_DEFAULT_LIMIT
  const visible = expanded ? logs : logs.slice(0, RECENT_DEFAULT_LIMIT)

  return (
    <>
      <div className="table-wrapper">
        <table className="usage-table">
          <thead>
            <tr>
              <th>Date</th>
              <th>Model</th>
              <th>Provider</th>
              <th>Input</th>
              <th>Output</th>
              <th>Cost</th>
            </tr>
          </thead>
          <tbody>
            {visible.map(log => (
              <tr key={log.id}>
                <td className="text-muted">{fmtDate(log.created_at)}</td>
                <td><code className="model-code">{log.model || '—'}</code></td>
                <td className="text-muted">{log.provider || '—'}</td>
                <td className="num-cell">{(log.prompt_tokens ?? 0).toLocaleString()}</td>
                <td className="num-cell">{(log.completion_tokens ?? 0).toLocaleString()}</td>
                <td className="num-cell">{fmt$(log.cost)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {hasMore && (
        <div className="recent-footer">
          <span className="recent-count">
            Showing {visible.length} of {logs.length}
          </span>
          <button
            className="btn-link"
            onClick={() => setExpanded(e => !e)}
          >
            {expanded ? '↑ Show less' : `↓ Show all ${logs.length}`}
          </button>
        </div>
      )}
    </>
  )
}

// ─── Model Table ──────────────────────────────────────────────────────────────

function ModelTable({ models }: { models: ModelBreakdown[] }) {
  if (models.length === 0) {
    return <p className="model-empty">No model usage for the selected period.</p>
  }
  const maxCost = Math.max(...models.map(m => parseFloat(m.cost)), 0.0001)
  return (
    <table className="model-table">
      <thead>
        <tr>
          <th>Model</th>
          <th>Cost</th>
          <th>Input</th>
          <th>Output</th>
          <th>Requests</th>
        </tr>
      </thead>
      <tbody>
        {models.map(m => {
          const costVal = parseFloat(m.cost)
          const barPct = (costVal / maxCost) * 100
          return (
            <tr key={`${m.provider}/${m.model}`}>
              <td>
                <div className="model-name-cell">
                  <code className="model-code">{m.model}</code>
                  <span className="model-provider">{m.provider}</span>
                </div>
              </td>
              <td>
                <div className="model-cost-cell">
                  <span>{fmt$(m.cost)}</span>
                  <div className="model-cost-bar-track">
                    <div className="model-cost-bar-fill" style={{ width: `${barPct}%` }} />
                  </div>
                </div>
              </td>
              <td className="num-cell">{fmtTokens(m.input_tokens)}</td>
              <td className="num-cell">{fmtTokens(m.output_tokens)}</td>
              <td className="num-cell">{fmtNum(m.requests)}</td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

// ─── Dashboard ────────────────────────────────────────────────────────────────

export default function Dashboard() {
  const { user } = useUser()
  const { config } = useDashboardConfig()
  const [dateRange, setDateRange] = useState<DateRange>({ preset: '30d' })
  const { logs, summary, budgets, appliedRange, loading, error, refresh } = useUsageData(dateRange)

  const s = summary

  // Derive a human-readable label for the selected period section.
  const PRESET_LABELS: Record<string, string> = {
    '1d': 'Last 24 Hours',
    '3d': 'Last 3 Days',
    '7d': 'Last 7 Days',
    '14d': 'Last 14 Days',
    '30d': 'Last 30 Days',
    '90d': 'Last 90 Days',
    'custom': 'Custom Period',
  }
  const periodLabel = dateRange.preset ? (PRESET_LABELS[dateRange.preset] ?? 'Selected Period') : 'Selected Period'

  return (
    <div className="page-container">
      <Navbar />
      <div className="page-content">

        {/* ── Header ── */}
        <div className="dash-header">
          <div>
            <h1>Dashboard</h1>
            {user?.firstName && (
              <p className="dash-welcome">Welcome back, {user.firstName}!</p>
            )}
          </div>
          <div className="dash-controls">
            <DateRangeSelector
              config={config}
              value={dateRange}
              onChange={setDateRange}
            />
            <button className="btn btn-secondary refresh-btn" onClick={refresh}>
              Refresh
            </button>
          </div>
        </div>

        {appliedRange && (
          <p className="applied-range-note">
            Showing data from {appliedRange.start} to {appliedRange.end}
          </p>
        )}

        {loading && (
          <div className="loading-center">
            <div className="spinner" />
          </div>
        )}

        {error && (
          <div className="flash flash-error" style={{ marginBottom: '1.5rem' }}>
            {error}
            <button className="btn btn-secondary" style={{ marginLeft: '1rem' }} onClick={refresh}>
              Retry
            </button>
          </div>
        )}

        {!loading && (
          <>
            {/* ── Budget bars ── */}
            {budgets.filter(b => b.scope_type === 'account').length > 0 && (
              <div className="budget-bars">
                {budgets
                  .filter(b => b.scope_type === 'account')
                  .map(b => <BudgetBar key={b.id} b={b} />)}
              </div>
            )}

            {/* ── Cost Overview ── */}
            <div className="dash-section-title">Cost Overview</div>
            <p className="section-note">
              Cost is calculated based on model usage, token consumption and for API usage billed users only. Pricing varies by model.{' '}
              <a href="/pricing-config" className="section-note-link">Refer to the Pricing Config page</a>{' '}
              for detailed rates. These figures reflect token usage costs only and do not represent
              TokenGate subscription fees, invoices, or Stripe billing totals.
            </p>
            <div className="summary-grid summary-grid-4">
              <div className="card summary-card">
                <p className="summary-label">Today</p>
                <p className="summary-value">{s ? fmt$(s.cost.today) : '—'}</p>
                {s && <GrowthPill pct={growthPct(s.cost.today, s.cost.yesterday)} />}
                <p className="summary-sub">vs yesterday {s ? fmt$(s.cost.yesterday) : ''}</p>
              </div>
              <div className="card summary-card">
                <p className="summary-label">This Month</p>
                <p className="summary-value">{s ? fmt$(s.cost.this_month) : '—'}</p>
                {s && <GrowthPill pct={growthPct(s.cost.this_month, s.cost.last_month)} />}
                <p className="summary-sub">last month {s ? fmt$(s.cost.last_month) : ''}</p>
              </div>
              <div className="card summary-card">
                <p className="summary-label">Last Month</p>
                <p className="summary-value">{s ? fmt$(s.cost.last_month) : '—'}</p>
              </div>
              <div className="card summary-card">
                <p className="summary-label">Cumulative</p>
                <p className="summary-value">{s ? fmt$(s.cost.cumulative) : '—'}</p>
                <p className="summary-sub">{s ? fmtNum(s.requests.cumulative) : '—'} requests total</p>
              </div>
            </div>

            {/* ── Token Summary ── */}
            <div className="dash-section-title">Token Usage</div>
            <div className="summary-grid summary-grid-4">
              <div className="card summary-card">
                <p className="summary-label">Input Tokens</p>
                <p className="summary-value summary-value-sm">{s ? fmtTokens(s.tokens.input_total) : '—'}</p>
                <p className="summary-sub">{periodLabel.toLowerCase()}</p>
              </div>
              <div className="card summary-card">
                <p className="summary-label">Output Tokens</p>
                <p className="summary-value summary-value-sm">{s ? fmtTokens(s.tokens.output_total) : '—'}</p>
                <p className="summary-sub">{periodLabel.toLowerCase()}</p>
              </div>
              <div className="card summary-card">
                <p className="summary-label">Total Tokens</p>
                <p className="summary-value summary-value-sm">{s ? fmtTokens(s.tokens.total) : '—'}</p>
                <p className="summary-sub">{periodLabel.toLowerCase()}</p>
              </div>
              <div className="card summary-card">
                <p className="summary-label">Avg / Request</p>
                <p className="summary-value summary-value-sm">{s ? fmtTokens(s.tokens.avg_per_request) : '—'}</p>
                <p className="summary-sub">tokens</p>
              </div>
            </div>

            {/* ── Model Breakdown + Daily Trend side-by-side ── */}
            <div className="dash-section-title">{periodLabel}</div>
            <div className="dash-split">
              <div className="card dash-split-left">
                <p className="card-subtitle">Breakdown by Model</p>
                <ModelTable models={s?.by_model ?? []} />
              </div>

              <div className="card dash-split-right">
                <p className="card-subtitle">Daily Cost — {periodLabel.toLowerCase()}</p>
                <TrendChart data={s?.daily_trend ?? []} mode="cost" />
                <p className="card-subtitle" style={{ marginTop: '1.5rem' }}>Daily Tokens — {periodLabel.toLowerCase()}</p>
                <TrendChart data={s?.daily_trend ?? []} mode="tokens" />
              </div>
            </div>

            {/* ── Recent Requests ── */}
            <div className="dash-section-title">Recent Requests</div>
            <div className="card">
              <RecentRequests logs={logs} />
            </div>
          </>
        )}
      </div>
    </div>
  )
}
