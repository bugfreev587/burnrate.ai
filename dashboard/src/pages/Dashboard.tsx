import { useState, useRef, useCallback, useEffect } from 'react'
import { useUser } from '@clerk/clerk-react'
import Navbar from '../components/Navbar'
import DateRangeSelector from '../components/DateRangeSelector'
import { useUsageData } from '../hooks/useUsageData'
import type { DateRange, BudgetStatus, DailyTrend, ModelBreakdown, ApiKeyBreakdown, ForecastData, MetricsData, DailyActivity } from '../hooks/useUsageData'
import { useDashboardConfig } from '../hooks/useDashboardConfig'
import { useRateLimits } from '../hooks/useRateLimits'
import type { RateLimit } from '../hooks/useRateLimits'
import './Dashboard.css'
import './LimitsPage.css'

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
            <span style={{ color, fontSize: '0.85em' }}>{b.provider ? ` · ${b.provider.charAt(0).toUpperCase() + b.provider.slice(1)}` : ' · All Providers'}</span>
            {b.scope_type === 'api_key' && b.scope_id && (
              <span className="budget-scope"> · Key: {b.key_label || b.scope_id.slice(0, 8) + '…'}</span>
            )}
          </span>
          <span className="budget-action" style={{ color: b.action === 'block' || b.action === 'alert_block' ? 'var(--color-danger)' : '#f59e0b' }}>
            {b.action === 'alert_block' ? ' · Alert + Block' : b.action === 'block' ? ' · Block' : ' · Alert'}
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
  const [hoverIdx, setHoverIdx] = useState<number | null>(null)

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

  // Tooltip value for hovered point.
  const hoverPoint = hoverIdx !== null ? data[hoverIdx] : null
  const hoverVal = hoverPoint
    ? (mode === 'cost' ? fmt$(hoverPoint.cost) : fmtTokens(hoverPoint.tokens))
    : ''
  const hoverDate = hoverPoint ? hoverPoint.date : ''

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      className="trend-svg"
      preserveAspectRatio="none"
      onMouseLeave={() => setHoverIdx(null)}
    >
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

      {/* Vertical guide line on hover */}
      {hoverIdx !== null && (
        <line
          x1={xOf(hoverIdx)} y1={PAD.top}
          x2={xOf(hoverIdx)} y2={PAD.top + chartH}
          stroke="rgba(255,255,255,0.15)"
          strokeWidth="1"
          strokeDasharray="3,3"
        />
      )}

      {/* Dots */}
      {data.map((d, i) => {
        const v = mode === 'cost' ? parseFloat(d.cost) : d.tokens
        const isHovered = hoverIdx === i
        return (
          <circle
            key={i}
            cx={xOf(i)}
            cy={yOf(v)}
            r={isHovered ? 5 : 3}
            fill={isHovered ? '#fff' : 'var(--color-primary)'}
            stroke={isHovered ? 'var(--color-primary)' : 'none'}
            strokeWidth={isHovered ? 2 : 0}
          />
        )
      })}

      {/* Invisible wider hit areas for hover detection */}
      {data.map((d, i) => {
        const v = mode === 'cost' ? parseFloat(d.cost) : d.tokens
        return (
          <circle
            key={`hit-${i}`}
            cx={xOf(i)}
            cy={yOf(v)}
            r={10}
            fill="transparent"
            style={{ cursor: 'pointer' }}
            onMouseEnter={() => setHoverIdx(i)}
          />
        )
      })}

      {/* Tooltip */}
      {hoverIdx !== null && hoverPoint && (() => {
        const v = mode === 'cost' ? parseFloat(hoverPoint.cost) : hoverPoint.tokens
        const cx = xOf(hoverIdx)
        const cy = yOf(v)
        // Position tooltip above the point; flip if near the top.
        const tooltipY = cy - 12 > PAD.top ? cy - 12 : cy + 18
        // Anchor: prefer middle, shift left/right near edges.
        const anchor = cx < PAD.left + 60 ? 'start' : cx > PAD.left + chartW - 60 ? 'end' : 'middle'
        return (
          <g>
            <text
              x={cx}
              y={tooltipY - 9}
              textAnchor={anchor}
              fontSize="8"
              fill="var(--color-text-muted)"
            >
              {hoverDate}
            </text>
            <text
              x={cx}
              y={tooltipY}
              textAnchor={anchor}
              fontSize="9.5"
              fontWeight="600"
              fill="#fff"
            >
              {hoverVal}
            </text>
          </g>
        )
      })()}

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

// ─── Latency Trend Chart (dual-line: p50 + p95) ──────────────────────────────

function LatencyTrendChart({ data }: { data: DailyActivity[] }) {
  const [hoverIdx, setHoverIdx] = useState<number | null>(null)

  if (data.length === 0) {
    return <div className="trend-empty">No latency data for the selected period.</div>
  }

  const p50s = data.map(d => d.p50_latency)
  const p95s = data.map(d => d.p95_latency)
  const maxVal = Math.max(...p50s, ...p95s, 1)

  const W = 600
  const H = 132
  const PAD = { top: 10, right: 8, bottom: 26, left: 52 }
  const chartW = W - PAD.left - PAD.right
  const chartH = H - PAD.top - PAD.bottom

  const xOf = (i: number) => PAD.left + (i / (data.length - 1 || 1)) * chartW
  const yOf = (v: number) => PAD.top + chartH - (v / maxVal) * chartH

  const p50Pts = data.map((_, i) => `${xOf(i)},${yOf(p50s[i])}`).join(' ')
  const p95Pts = data.map((_, i) => `${xOf(i)},${yOf(p95s[i])}`).join(' ')

  const labelStep = Math.ceil(data.length / 5)

  const yTicks = [
    { val: 0,          label: '0ms' },
    { val: maxVal / 2, label: fmtMs(maxVal / 2) },
    { val: maxVal,     label: fmtMs(maxVal) },
  ]

  const axisTitleX = 10
  const axisTitleY = PAD.top + chartH / 2

  const hoverPoint = hoverIdx !== null ? data[hoverIdx] : null

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      className="trend-svg"
      preserveAspectRatio="none"
      onMouseLeave={() => setHoverIdx(null)}
    >
      {/* Y-axis title */}
      <text
        x={axisTitleX}
        y={axisTitleY}
        textAnchor="middle"
        fontSize="8.5"
        fill="var(--color-text-muted)"
        transform={`rotate(-90, ${axisTitleX}, ${axisTitleY})`}
      >
        Latency (ms)
      </text>

      {/* Y-axis ticks */}
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

      {/* Y-axis line */}
      <line
        x1={PAD.left} y1={PAD.top}
        x2={PAD.left} y2={PAD.top + chartH}
        stroke="rgba(255,255,255,0.1)"
        strokeWidth="1"
      />

      {/* p95 line (orange) */}
      <polyline points={p95Pts} fill="none" stroke="#f59e0b" strokeWidth="1.5" strokeLinejoin="round" strokeDasharray="4,3" />
      {/* p50 line (primary) */}
      <polyline points={p50Pts} fill="none" stroke="var(--color-primary)" strokeWidth="2" strokeLinejoin="round" />

      {/* Hover guide line */}
      {hoverIdx !== null && (
        <line
          x1={xOf(hoverIdx)} y1={PAD.top}
          x2={xOf(hoverIdx)} y2={PAD.top + chartH}
          stroke="rgba(255,255,255,0.15)"
          strokeWidth="1"
          strokeDasharray="3,3"
        />
      )}

      {/* Dots: p50 */}
      {data.map((_, i) => (
        <circle
          key={`p50-${i}`}
          cx={xOf(i)}
          cy={yOf(p50s[i])}
          r={hoverIdx === i ? 5 : 2.5}
          fill={hoverIdx === i ? '#fff' : 'var(--color-primary)'}
          stroke={hoverIdx === i ? 'var(--color-primary)' : 'none'}
          strokeWidth={hoverIdx === i ? 2 : 0}
        />
      ))}

      {/* Dots: p95 */}
      {data.map((_, i) => (
        <circle
          key={`p95-${i}`}
          cx={xOf(i)}
          cy={yOf(p95s[i])}
          r={hoverIdx === i ? 4 : 2}
          fill={hoverIdx === i ? '#fff' : '#f59e0b'}
          stroke={hoverIdx === i ? '#f59e0b' : 'none'}
          strokeWidth={hoverIdx === i ? 2 : 0}
        />
      ))}

      {/* Hit areas */}
      {data.map((_, i) => (
        <circle
          key={`hit-${i}`}
          cx={xOf(i)}
          cy={yOf((p50s[i] + p95s[i]) / 2)}
          r={10}
          fill="transparent"
          style={{ cursor: 'pointer' }}
          onMouseEnter={() => setHoverIdx(i)}
        />
      ))}

      {/* Tooltip */}
      {hoverIdx !== null && hoverPoint && (() => {
        const cx = xOf(hoverIdx)
        const cy = yOf(p50s[hoverIdx])
        const tooltipY = cy - 12 > PAD.top ? cy - 22 : cy + 24
        const anchor = cx < PAD.left + 60 ? 'start' : cx > PAD.left + chartW - 60 ? 'end' : 'middle'
        return (
          <g>
            <text x={cx} y={tooltipY - 9} textAnchor={anchor} fontSize="8" fill="var(--color-text-muted)">
              {hoverPoint.date}
            </text>
            <text x={cx} y={tooltipY} textAnchor={anchor} fontSize="9" fontWeight="600" fill="var(--color-primary)">
              p50: {fmtMs(hoverPoint.p50_latency)}
            </text>
            <text x={cx} y={tooltipY + 10} textAnchor={anchor} fontSize="9" fontWeight="600" fill="#f59e0b">
              p95: {fmtMs(hoverPoint.p95_latency)}
            </text>
          </g>
        )
      })()}

      {/* X-axis labels */}
      {data.map((d, i) => {
        if (i % labelStep !== 0 && i !== data.length - 1) return null
        return (
          <text key={i} x={xOf(i)} y={H - 4} textAnchor="middle" fontSize="9" fill="var(--color-text-muted)">
            {d.date.slice(5)}
          </text>
        )
      })}

      {/* Legend */}
      <circle cx={PAD.left + 8} cy={H - 14} r={3} fill="var(--color-primary)" />
      <text x={PAD.left + 14} y={H - 11} fontSize="8" fill="var(--color-text-muted)">p50</text>
      <circle cx={PAD.left + 38} cy={H - 14} r={3} fill="#f59e0b" />
      <text x={PAD.left + 44} y={H - 11} fontSize="8" fill="var(--color-text-muted)">p95</text>
    </svg>
  )
}

// ─── Request Outcome Chart (horizontal bar) ──────────────────────────────────

function RequestOutcomeChart({ metrics }: { metrics: MetricsData }) {
  const { total_requests, blocked_rate_limit, blocked_budget } = metrics.activity
  const success = total_requests
  const total = success + blocked_rate_limit + blocked_budget
  if (total === 0) {
    return <p className="model-empty">No request data for the selected period.</p>
  }

  const segments = [
    { label: 'Success', count: success, color: '#22c55e' },
    { label: 'Rate Limited', count: blocked_rate_limit, color: '#f59e0b' },
    { label: 'Budget Exceeded', count: blocked_budget, color: '#ef4444' },
  ].filter(s => s.count > 0)

  return (
    <>
      <div className="provider-bar-track">
        {segments.map(s => {
          const pct = (s.count / total) * 100
          return (
            <div
              key={s.label}
              className="provider-bar-segment"
              style={{ width: `${pct}%`, background: s.color }}
              title={`${s.label}: ${fmtNum(s.count)} (${pct.toFixed(1)}%)`}
            />
          )
        })}
      </div>
      <div className="provider-legend">
        {segments.map(s => (
          <div key={s.label} className="provider-legend-item">
            <div className="provider-legend-swatch" style={{ background: s.color }} />
            <div>
              <span className="provider-legend-name">{s.label}</span>
              <span className="provider-legend-detail"> {fmtNum(s.count)} ({((s.count / total) * 100).toFixed(1)}%)</span>
            </div>
          </div>
        ))}
      </div>
    </>
  )
}

// ─── Recent Requests ──────────────────────────────────────────────────────────

const RECENT_DEFAULT_LIMIT = 10

type BillingFilter = 'all' | 'api_billed' | 'subscription'

function RecentRequests({ logs, billingFilter }: { logs: ReturnType<typeof useUsageData>['logs']; billingFilter: BillingFilter }) {
  const [expanded, setExpanded] = useState(false)

  const filtered = logs.filter(l => {
    if (billingFilter === 'api_billed') return l.api_usage_billed
    if (billingFilter === 'subscription') return !l.api_usage_billed
    return true
  })

  if (filtered.length === 0) {
    return (
      <div className="empty-state">
        <p>No recent requests.</p>
        <p className="empty-hint">
          Configure your Claude Code client to report usage via the gateway API.
        </p>
      </div>
    )
  }

  const hasMore = filtered.length > RECENT_DEFAULT_LIMIT
  const visible = expanded ? filtered : filtered.slice(0, RECENT_DEFAULT_LIMIT)

  return (
    <>
      <div className="table-wrapper">
        <table className="usage-table">
          <thead>
            <tr>
              <th>Date</th>
              <th>Model</th>
              <th>Provider</th>
              <th>API Key</th>
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
                <td className="text-muted">{log.key_label || '—'}</td>
                <td className="num-cell">{(log.prompt_tokens ?? 0).toLocaleString()}</td>
                <td className="num-cell">{(log.completion_tokens ?? 0).toLocaleString()}</td>
                <td className="num-cell">{log.api_usage_billed ? fmt$(log.cost) : '$0.00'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {hasMore && (
        <div className="recent-footer">
          <span className="recent-count">
            Showing {visible.length} of {filtered.length}
          </span>
          <button
            className="btn-link"
            onClick={() => setExpanded(e => !e)}
          >
            {expanded ? '↑ Show less' : `↓ Show all ${filtered.length}`}
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

// ─── API Key Table ───────────────────────────────────────────────────────────

function ApiKeyTable({ keys }: { keys: ApiKeyBreakdown[] }) {
  if (keys.length === 0) {
    return <p className="model-empty">No API key usage for the selected period.</p>
  }
  const maxCost = Math.max(...keys.map(k => parseFloat(k.cost)), 0.0001)
  return (
    <table className="model-table">
      <thead>
        <tr>
          <th>API Key</th>
          <th>Cost</th>
          <th>Input</th>
          <th>Output</th>
          <th>Requests</th>
        </tr>
      </thead>
      <tbody>
        {keys.map(k => {
          const costVal = parseFloat(k.cost)
          const barPct = (costVal / maxCost) * 100
          return (
            <tr key={k.key_id}>
              <td>
                <div className="model-name-cell">
                  <code className="model-code">{k.label || k.key_id}</code>
                </div>
              </td>
              <td>
                <div className="model-cost-cell">
                  <span>{fmt$(k.cost)}</span>
                  <div className="model-cost-bar-track">
                    <div className="model-cost-bar-fill" style={{ width: `${barPct}%` }} />
                  </div>
                </div>
              </td>
              <td className="num-cell">{fmtTokens(k.input_tokens)}</td>
              <td className="num-cell">{fmtTokens(k.output_tokens)}</td>
              <td className="num-cell">{fmtNum(k.requests)}</td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

// ─── Provider Breakdown ──────────────────────────────────────────────────────

const PROVIDER_COLORS = ['#6366f1', '#22c55e', '#f59e0b', '#ef4444', '#8b5cf6', '#06b6d4', '#ec4899']

function ProviderBreakdown({ models }: { models: ModelBreakdown[] }) {
  if (models.length === 0) {
    return <p className="model-empty">No provider data for the selected period.</p>
  }

  // Group by provider
  const byProvider: Record<string, { cost: number; requests: number }> = {}
  for (const m of models) {
    const p = m.provider || 'unknown'
    if (!byProvider[p]) byProvider[p] = { cost: 0, requests: 0 }
    byProvider[p].cost += parseFloat(m.cost)
    byProvider[p].requests += m.requests
  }

  const entries = Object.entries(byProvider)
    .map(([name, data]) => ({ name, ...data }))
    .sort((a, b) => b.cost - a.cost)

  const totalCost = entries.reduce((sum, e) => sum + e.cost, 0)

  return (
    <>
      <div className="provider-bar-track">
        {entries.map((e, i) => {
          const pct = totalCost > 0 ? (e.cost / totalCost) * 100 : 0
          return (
            <div
              key={e.name}
              className="provider-bar-segment"
              style={{ width: `${pct}%`, background: PROVIDER_COLORS[i % PROVIDER_COLORS.length] }}
              title={`${e.name}: ${fmt$(e.cost)} (${pct.toFixed(1)}%)`}
            />
          )
        })}
      </div>
      <div className="provider-legend">
        {entries.map((e, i) => (
          <div key={e.name} className="provider-legend-item">
            <div className="provider-legend-swatch" style={{ background: PROVIDER_COLORS[i % PROVIDER_COLORS.length] }} />
            <div>
              <span className="provider-legend-name">{e.name}</span>
              <span className="provider-legend-detail"> {fmt$(e.cost)} · {fmtNum(e.requests)} reqs</span>
            </div>
          </div>
        ))}
      </div>
    </>
  )
}

// ─── Quick Health Hero ───────────────────────────────────────────────────────

function QuickHealth({ summary, forecast, budgets }: { summary: ReturnType<typeof useUsageData>['summary']; forecast: ForecastData | null; budgets: BudgetStatus[] }) {
  const s = summary
  // Worst-case budget usage
  let budgetPct: number | null = null
  let budgetColor = 'budget-status-ok'
  if (budgets.length > 0) {
    budgetPct = Math.max(...budgets.map(b => b.pct_used))
    if (budgetPct >= 100) budgetColor = 'budget-status-danger'
    else if (budgetPct >= parseFloat(budgets[0]?.alert_threshold ?? '80')) budgetColor = 'budget-status-warn'
  }

  return (
    <div className="quick-health">
      <div className="quick-health-inner">
        <div className="summary-grid summary-grid-3">
          <div className="card summary-card">
            <p className="summary-label">Today's Spend</p>
            <p className="summary-value">{s ? fmt$(s.cost.today) : '—'}</p>
            {s && <GrowthPill pct={growthPct(s.cost.today, s.cost.yesterday)} />}
            <p className="summary-sub">vs yesterday {s ? fmt$(s.cost.yesterday) : ''}</p>
          </div>
          <div className="card summary-card">
            <p className="summary-label">Projected This Month</p>
            <p className="summary-value">{forecast ? fmt$(forecast.forecast) : '—'}</p>
            <p className="summary-sub">{forecast ? `${forecast.days_elapsed}d elapsed · ${forecast.days_remaining}d left` : ''}</p>
          </div>
          <div className="card summary-card">
            <p className="summary-label">Budget Status</p>
            <p className={`summary-value ${budgetColor}`}>
              {budgetPct !== null ? `${budgetPct.toFixed(1)}%` : 'No budget'}
            </p>
            <p className="summary-sub">{budgetPct !== null ? 'worst-case usage across all budgets' : 'Set up a budget to track limits'}</p>
          </div>
        </div>
      </div>
    </div>
  )
}

// ─── Insights Strip ──────────────────────────────────────────────────────────

function InsightsStrip({ summary, apiKeys }: { summary: ReturnType<typeof useUsageData>['summary']; apiKeys: ApiKeyBreakdown[] }) {
  const s = summary

  // Top model by cost
  const topModel = s?.by_model?.length
    ? [...s.by_model].sort((a, b) => parseFloat(b.cost) - parseFloat(a.cost))[0]
    : null

  // Active keys (keys with at least 1 request)
  const activeKeys = apiKeys.filter(k => k.requests > 0).length

  return (
    <div className="insights-strip">
      <div className="card insight-card">
        <p className="insight-label">Top Model</p>
        <p className="insight-value">{topModel ? topModel.model : '—'}</p>
        <p className="insight-sub">{topModel ? `${fmt$(topModel.cost)} · ${fmtNum(topModel.requests)} reqs` : 'No usage data'}</p>
      </div>
      <div className="card insight-card">
        <p className="insight-label">Active Keys</p>
        <p className="insight-value">{activeKeys}</p>
        <p className="insight-sub">keys with usage this period</p>
      </div>
      <div className="card insight-card">
        <p className="insight-label">Avg Tokens / Request</p>
        <p className="insight-value">{s ? fmtTokens(s.tokens.avg_per_request) : '—'}</p>
        <p className="insight-sub">across all models</p>
      </div>
    </div>
  )
}

// ─── Rate Limit Card ──────────────────────────────────────────────────────────

const METRIC_LABELS: Record<string, string> = { rpm: 'RPM', itpm: 'ITPM', otpm: 'OTPM' }

function RateLimitCard({ rl }: { rl: RateLimit }) {
  const pct = rl.LimitValue > 0 ? (rl.current_usage / rl.LimitValue) * 100 : 0
  const color = pct >= 100 ? 'var(--color-danger)' : pct >= 80 ? '#f59e0b' : 'var(--color-accent, var(--color-primary))'
  const barClass = pct >= 100 ? 'usage-exceeded' : pct >= 80 ? 'usage-high' : ''

  return (
    <div className="rl-card card">
      <div className="rl-header">
        <span className="rl-header-label">
          {rl.Provider ? rl.Provider.charAt(0).toUpperCase() + rl.Provider.slice(1) : 'All Providers'}
          {rl.Model && <span className="rl-header-model"> · {rl.Model}</span>}
        </span>
        <span className={`metric-badge metric-${rl.Metric}`}>
          {METRIC_LABELS[rl.Metric] ?? rl.Metric.toUpperCase()}
        </span>
      </div>
      <div className="usage-bar">
        <div
          className={`usage-bar-fill ${barClass}`}
          style={{ width: `${Math.min(pct, 100)}%` }}
        />
      </div>
      <div className="rl-meta">
        <span className="rl-usage-text">
          {rl.current_usage.toLocaleString()} / {rl.LimitValue.toLocaleString()}
        </span>
        <span className="rl-pct" style={{ color }}>{pct.toFixed(1)}%</span>
      </div>
    </div>
  )
}

// ─── Dashboard ────────────────────────────────────────────────────────────────

export default function Dashboard() {
  const { user } = useUser()
  const { config } = useDashboardConfig()
  const { limits: rateLimits } = useRateLimits(5000)
  const [dateRange, setDateRange] = useState<DateRange>({ preset: '7d' })
  const [billingFilter, setBillingFilter] = useState<BillingFilter>('all')
  const [recentOpen, setRecentOpen] = useState(false)
  const pollInterval = recentOpen ? 5_000 : 300_000 // 5s when watching requests, 5min when collapsed
  const { logs, summary, budgets, forecast, metrics, appliedRange, loading, error, refresh } = useUsageData(dateRange, pollInterval)

  const recentRef = useRef<HTMLDivElement>(null)
  const scrollToRecentAfterLoad = useRef(false)

  const handleRecentRefresh = useCallback(() => {
    scrollToRecentAfterLoad.current = true
    refresh()
  }, [refresh])

  useEffect(() => {
    if (!loading && scrollToRecentAfterLoad.current) {
      scrollToRecentAfterLoad.current = false
      recentRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }, [loading])

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
    <div className="page-container dash-page">
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
            {/* ── Quick Health Hero ── */}
            <QuickHealth summary={s} forecast={forecast} budgets={budgets} />

            {/* ── Cost Overview ── */}
            <div className="dash-section-title">Cost Overview</div>
            <p className="section-note">
              Cost is calculated based on model usage, token consumption and for <span style={{ fontWeight: 'bold', color: '#ffffe0' }}>API usage billed users</span> only. Pricing varies by model.{' '}
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
                <p className="summary-label">Projected Spend</p>
                <p className="summary-value">{forecast ? fmt$(forecast.forecast) : '—'}</p>
                <p className="summary-sub">{forecast ? `${fmt$(forecast.daily_average)}/day avg` : ''}</p>
              </div>
              <div className="card summary-card">
                <p className="summary-label">Avg Daily Cost</p>
                <p className="summary-value">{forecast ? fmt$(forecast.daily_average) : '—'}</p>
                <p className="summary-sub">{forecast ? `over ${forecast.days_elapsed} days this month` : ''}</p>
              </div>
            </div>

            {/* ── Budget bars (moved after cost overview) ── */}
            {budgets.length > 0 && (
              <>
                <div className="dash-section-title">Budget Limits</div>
                <div className="budget-bars">
                  {budgets
                    .sort((a, b) => {
                      if (a.scope_type !== b.scope_type) {
                        return a.scope_type === 'account' ? -1 : 1
                      }
                      const periodOrder: Record<string, number> = { monthly: 0, weekly: 1, daily: 2 }
                      const pa = periodOrder[a.period_type] ?? 9
                      const pb = periodOrder[b.period_type] ?? 9
                      if (pa !== pb) return pa - pb
                      if (!a.provider && b.provider) return -1
                      if (a.provider && !b.provider) return 1
                      return a.provider.localeCompare(b.provider)
                    })
                    .map(b => <BudgetBar key={b.id} b={b} />)}
                </div>
              </>
            )}

            {/* ── Active Rate Limits ── */}
            {rateLimits.filter(rl => rl.Enabled).length > 0 && (
              <>
                <div className="dash-section-title">Active Rate Limits</div>
                <div className="rl-grid">
                  {rateLimits
                    .filter(rl => rl.Enabled)
                    .map(rl => <RateLimitCard key={rl.ID} rl={rl} />)}
                </div>
              </>
            )}

            {/* ── Token Summary ── */}
            <div className="dash-section-title">Token Usage</div>
            <div className="summary-grid summary-grid-4">
              <div className="card summary-card">
                <p className="summary-label">Input Tokens</p>
                <p className="summary-value summary-value-sm">{s ? fmtTokens(s.tokens.input_total) : '—'}</p>
                <p className="summary-sub">{periodLabel.toLowerCase()}</p>
                {s && s.tokens.billed_input_total > 0 && (
                  <p className="summary-footnote"><span className="summary-footnote-value">{fmtTokens(s.tokens.billed_input_total)}</span> API usage billed</p>
                )}
              </div>
              <div className="card summary-card">
                <p className="summary-label">Output Tokens</p>
                <p className="summary-value summary-value-sm">{s ? fmtTokens(s.tokens.output_total) : '—'}</p>
                <p className="summary-sub">{periodLabel.toLowerCase()}</p>
                {s && s.tokens.billed_output_total > 0 && (
                  <p className="summary-footnote"><span className="summary-footnote-value">{fmtTokens(s.tokens.billed_output_total)}</span> API usage billed</p>
                )}
              </div>
              <div className="card summary-card">
                <p className="summary-label">Total Tokens</p>
                <p className="summary-value summary-value-sm">{s ? fmtTokens(s.tokens.total) : '—'}</p>
                <p className="summary-sub">{periodLabel.toLowerCase()}</p>
                {s && s.tokens.billed_total > 0 && (
                  <p className="summary-footnote"><span className="summary-footnote-value">{fmtTokens(s.tokens.billed_total)}</span> API usage billed</p>
                )}
              </div>
              <div className="card summary-card">
                <p className="summary-label">Avg / Request</p>
                <p className="summary-value summary-value-sm">{s ? fmtTokens(s.tokens.avg_per_request) : '—'}</p>
                <p className="summary-sub">tokens</p>
                {s && s.tokens.billed_avg > 0 && (
                  <p className="summary-footnote"><span className="summary-footnote-value">{fmtTokens(s.tokens.billed_avg)}</span> API usage billed</p>
                )}
              </div>
            </div>

            {/* ── Breakdowns: Model + API Key side-by-side ── */}
            <div className="dash-section-title">{periodLabel}</div>
            <div className="dash-split">
              <div className="card dash-split-left">
                <p className="card-subtitle">Breakdown by Model</p>
                <ModelTable models={s?.by_model ?? []} />
              </div>
              <div className="card dash-split-right">
                <p className="card-subtitle">Breakdown by API Key</p>
                <ApiKeyTable keys={s?.by_api_key ?? []} />
              </div>
            </div>

            {/* ── Daily Cost Chart + Provider Breakdown side-by-side ── */}
            <div className="dash-split">
              <div className="card dash-split-left">
                <p className="card-subtitle">Daily Cost</p>
                <TrendChart data={s?.daily_trend ?? []} mode="cost" />
              </div>
              <div className="card dash-split-right">
                <p className="card-subtitle">Provider Breakdown</p>
                <ProviderBreakdown models={s?.by_model ?? []} />
              </div>
            </div>

            {/* ── Insights Strip ── */}
            <div className="dash-section-title">Insights</div>
            <InsightsStrip summary={s} apiKeys={s?.by_api_key ?? []} />

            {/* ── Gateway Performance ── */}
            {metrics && metrics.latency.sample_count > 0 && (
              <>
                <div className="dash-section-title">Gateway Performance</div>
                <div className="summary-grid summary-grid-4">
                  <div className="card summary-card">
                    <p className="summary-label">p50 Latency</p>
                    <p className="summary-value summary-value-sm">{fmtMs(metrics.latency.p50)}</p>
                    <p className="summary-sub">median response time</p>
                  </div>
                  <div className="card summary-card">
                    <p className="summary-label">p95 Latency</p>
                    <p className="summary-value summary-value-sm">{fmtMs(metrics.latency.p95)}</p>
                    <p className="summary-sub">95th percentile</p>
                  </div>
                  <div className="card summary-card">
                    <p className="summary-label">p99 Latency</p>
                    <p className="summary-value summary-value-sm">{fmtMs(metrics.latency.p99)}</p>
                    <p className="summary-sub">99th percentile</p>
                  </div>
                  <div className="card summary-card">
                    <p className="summary-label">Avg Latency</p>
                    <p className="summary-value summary-value-sm">{fmtMs(metrics.latency.avg)}</p>
                    <p className="summary-sub">{fmtNum(metrics.latency.sample_count)} samples</p>
                  </div>
                </div>
                <div className="dash-split">
                  <div className="card dash-split-left">
                    <p className="card-subtitle">Latency Trend</p>
                    <LatencyTrendChart data={metrics.daily_activity} />
                  </div>
                  <div className="card dash-split-right">
                    <p className="card-subtitle">Request Outcomes</p>
                    <RequestOutcomeChart metrics={metrics} />
                  </div>
                </div>
              </>
            )}

            {/* ── Activity ── */}
            {metrics && (
              <>
                <div className="dash-section-title">Activity</div>
                <div className="summary-grid summary-grid-4">
                  <div className="card summary-card">
                    <p className="summary-label">Total Requests</p>
                    <p className="summary-value summary-value-sm">{fmtNum(metrics.activity.total_requests)}</p>
                    <p className="summary-sub">{periodLabel.toLowerCase()}</p>
                  </div>
                  <div className="card summary-card">
                    <p className="summary-label">Active API Keys</p>
                    <p className="summary-value summary-value-sm">{fmtNum(metrics.activity.active_api_keys)}</p>
                    <p className="summary-sub">keys with traffic</p>
                  </div>
                  <div className="card summary-card">
                    <p className="summary-label">Blocked Requests</p>
                    <p className="summary-value summary-value-sm" style={{ color: metrics.activity.total_blocked > 0 ? 'var(--color-danger)' : undefined }}>
                      {fmtNum(metrics.activity.total_blocked)}
                    </p>
                    <p className="summary-sub">
                      {metrics.activity.blocked_rate_limit > 0 && `${fmtNum(metrics.activity.blocked_rate_limit)} rate limit`}
                      {metrics.activity.blocked_rate_limit > 0 && metrics.activity.blocked_budget > 0 && ' · '}
                      {metrics.activity.blocked_budget > 0 && `${fmtNum(metrics.activity.blocked_budget)} budget`}
                      {metrics.activity.total_blocked === 0 && 'none blocked'}
                    </p>
                  </div>
                  <div className="card summary-card">
                    <p className="summary-label">Success Rate</p>
                    <p className="summary-value summary-value-sm">{fmtPct(metrics.activity.success_rate)}</p>
                    <p className="summary-sub">requests completed</p>
                  </div>
                </div>
              </>
            )}

            {/* ── Recent Requests (collapsed by default) ── */}
            <div
              ref={recentRef}
              className="section-collapsible"
              onClick={() => setRecentOpen(o => !o)}
            >
              <div className="dash-section-title">Recent Requests</div>
              <button className="section-collapsible-toggle">
                {recentOpen ? '▲ Collapse' : `▼ Show ${logs.length} requests`}
              </button>
            </div>
            {recentOpen && (
              <>
                <div style={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center', gap: '0.5rem', marginBottom: '0.5rem' }}>
                  <button
                    onClick={handleRecentRefresh}
                    title="Refresh requests"
                    style={{
                      background: 'var(--color-bg)',
                      color: 'var(--color-text-muted)',
                      border: '1px solid var(--color-border)',
                      borderRadius: '4px',
                      padding: '0.25rem 0.5rem',
                      cursor: 'pointer',
                      fontSize: '0.75rem',
                      display: 'inline-flex',
                      alignItems: 'center',
                    }}
                  >
                    ↻ Refresh
                  </button>
                  <select
                    value={billingFilter}
                    onChange={e => setBillingFilter(e.target.value as BillingFilter)}
                    style={{
                      fontSize: '0.75rem',
                      background: 'var(--color-bg)',
                      color: 'var(--color-text-muted)',
                      border: '1px solid var(--color-border)',
                      borderRadius: '4px',
                      padding: '0.25rem 0.5rem',
                    }}
                  >
                    <option value="all">All Requests</option>
                    <option value="api_billed">API Usage Billed</option>
                    <option value="subscription">Monthly Subscription</option>
                  </select>
                </div>
                <div className="card">
                  <RecentRequests logs={logs} billingFilter={billingFilter} />
                </div>
              </>
            )}

          </>
        )}
      </div>
    </div>
  )
}
