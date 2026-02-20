import { useUser } from '@clerk/clerk-react'
import Navbar from '../components/Navbar'
import { useUsageData } from '../hooks/useUsageData'
import type { BudgetStatus, DailyTrend, ModelBreakdown } from '../hooks/useUsageData'
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

function TrendChart({ data, mode }: { data: DailyTrend[]; mode: 'cost' | 'tokens' }) {
  if (data.length === 0) {
    return <div className="trend-empty">No data for the last 30 days.</div>
  }

  const values = data.map(d => mode === 'cost' ? parseFloat(d.cost) : d.tokens)
  const maxVal = Math.max(...values, 0.0001)

  const W = 600
  const H = 120
  const PAD = { top: 8, right: 4, bottom: 24, left: 4 }
  const chartW = W - PAD.left - PAD.right
  const chartH = H - PAD.top - PAD.bottom
  // Build SVG polyline points for line chart
  const pts = data.map((d, i) => {
    const x = PAD.left + (i / (data.length - 1 || 1)) * chartW
    const v = mode === 'cost' ? parseFloat(d.cost) : d.tokens
    const y = PAD.top + chartH - (v / maxVal) * chartH
    return `${x},${y}`
  }).join(' ')

  const areaBase = PAD.top + chartH
  const areaPts = [
    `${PAD.left},${areaBase}`,
    ...data.map((d, i) => {
      const x = PAD.left + (i / (data.length - 1 || 1)) * chartW
      const v = mode === 'cost' ? parseFloat(d.cost) : d.tokens
      const y = PAD.top + chartH - (v / maxVal) * chartH
      return `${x},${y}`
    }),
    `${PAD.left + chartW},${areaBase}`,
  ].join(' ')

  const labelStep = Math.ceil(data.length / 5)

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="trend-svg" preserveAspectRatio="none">
      <defs>
        <linearGradient id={`grad-${mode}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="var(--color-primary)" stopOpacity="0.3" />
          <stop offset="100%" stopColor="var(--color-primary)" stopOpacity="0.02" />
        </linearGradient>
      </defs>
      {/* Area fill */}
      <polygon points={areaPts} fill={`url(#grad-${mode})`} />
      {/* Line */}
      <polyline points={pts} fill="none" stroke="var(--color-primary)" strokeWidth="2" strokeLinejoin="round" />
      {/* Dots */}
      {data.map((d, i) => {
        const x = PAD.left + (i / (data.length - 1 || 1)) * chartW
        const v = mode === 'cost' ? parseFloat(d.cost) : d.tokens
        const y = PAD.top + chartH - (v / maxVal) * chartH
        return <circle key={i} cx={x} cy={y} r="3" fill="var(--color-primary)" />
      })}
      {/* X-axis labels */}
      {data.map((d, i) => {
        if (i % labelStep !== 0 && i !== data.length - 1) return null
        const x = PAD.left + (i / (data.length - 1 || 1)) * chartW
        const label = d.date.slice(5) // MM-DD
        return (
          <text key={i} x={x} y={H - 4} textAnchor="middle" fontSize="9" fill="var(--color-text-muted)">
            {label}
          </text>
        )
      })}
    </svg>
  )
}

// ─── Model Table ──────────────────────────────────────────────────────────────

function ModelTable({ models }: { models: ModelBreakdown[] }) {
  if (models.length === 0) {
    return <p className="model-empty">No model usage this month.</p>
  }
  const maxCost = Math.max(...models.map(m => parseFloat(m.cost)), 0.0001)
  return (
    <table className="model-table">
      <thead>
        <tr>
          <th>Model</th>
          <th>Cost (this month)</th>
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
  const { logs, summary, budgets, loading, error, refresh } = useUsageData()

  const s = summary

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
          <button className="btn btn-secondary refresh-btn" onClick={refresh}>
            Refresh
          </button>
        </div>

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
                <p className="summary-sub">cumulative</p>
              </div>
              <div className="card summary-card">
                <p className="summary-label">Output Tokens</p>
                <p className="summary-value summary-value-sm">{s ? fmtTokens(s.tokens.output_total) : '—'}</p>
                <p className="summary-sub">cumulative</p>
              </div>
              <div className="card summary-card">
                <p className="summary-label">Total Tokens</p>
                <p className="summary-value summary-value-sm">{s ? fmtTokens(s.tokens.total) : '—'}</p>
                <p className="summary-sub">cumulative</p>
              </div>
              <div className="card summary-card">
                <p className="summary-label">Avg / Request</p>
                <p className="summary-value summary-value-sm">{s ? fmtTokens(s.tokens.avg_per_request) : '—'}</p>
                <p className="summary-sub">tokens</p>
              </div>
            </div>

            {/* ── Model Breakdown + Daily Trend side-by-side ── */}
            <div className="dash-section-title">This Month</div>
            <div className="dash-split">
              <div className="card dash-split-left">
                <p className="card-subtitle">Breakdown by Model</p>
                <ModelTable models={s?.by_model ?? []} />
              </div>

              <div className="card dash-split-right">
                <p className="card-subtitle">Daily Cost — last 30 days</p>
                <TrendChart data={s?.daily_trend ?? []} mode="cost" />
                <p className="card-subtitle" style={{ marginTop: '1.5rem' }}>Daily Tokens — last 30 days</p>
                <TrendChart data={s?.daily_trend ?? []} mode="tokens" />
              </div>
            </div>

            {/* ── Recent Requests ── */}
            <div className="dash-section-title">Recent Requests</div>
            <div className="card">
              {logs.length === 0 ? (
                <div className="empty-state">
                  <p>No usage recorded yet.</p>
                  <p className="empty-hint">
                    Configure your Claude Code client to report usage via the gateway API.
                  </p>
                </div>
              ) : (
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
                      {logs.slice(0, 50).map(log => (
                        <tr key={log.id}>
                          <td className="text-muted">{new Date(log.created_at).toLocaleString()}</td>
                          <td><code className="model-code">{log.model}</code></td>
                          <td className="text-muted">{log.provider}</td>
                          <td className="num-cell">{(log.prompt_tokens ?? 0).toLocaleString()}</td>
                          <td className="num-cell">{(log.completion_tokens ?? 0).toLocaleString()}</td>
                          <td className="num-cell">{fmt$(log.cost)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
