import { useEffect, useState } from 'react'
import { Navigate } from 'react-router-dom'
import Navbar from '../components/Navbar'
import { useUserSync } from '../hooks/useUserSync'
import './PlanPage.css'

const API_BASE = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

// ─── Types ────────────────────────────────────────────────────────────────

type PlanKey = 'free' | 'pro' | 'team' | 'business'

interface PlanLimits {
  max_api_keys: number
  max_members: number
  allowed_periods: string[]
  allow_block_action: boolean
  allow_per_key_budget: boolean
  data_retention_days: number
}

interface OwnerSettings {
  plan: PlanKey
  max_api_keys: number
  plan_limits: PlanLimits
}

interface PlanData {
  settings: OwnerSettings
  keyCount: number
  memberCount: number
}

// ─── Static plan comparison data ─────────────────────────────────────────

const PLANS: Array<{
  key: PlanKey
  label: string
  price: string
  maxKeys: string
  maxMembers: string
  periods: string
  block: boolean
  perKey: boolean
  retention: string
}> = [
  { key: 'free',     label: 'Free',     price: 'Free',    maxKeys: '1',  maxMembers: '1',  periods: 'Monthly', block: false, perKey: false, retention: '30 days'   },
  { key: 'pro',      label: 'Pro',      price: '$15/mo',  maxKeys: '5',  maxMembers: '1',  periods: 'All',     block: true,  perKey: false, retention: '90 days'   },
  { key: 'team',     label: 'Team',     price: '$29/mo',  maxKeys: '∞',  maxMembers: '10', periods: 'All',     block: true,  perKey: true,  retention: '1 year'    },
  { key: 'business', label: 'Business', price: 'Custom',  maxKeys: '∞',  maxMembers: '∞',  periods: 'All',     block: true,  perKey: true,  retention: 'Unlimited' },
]

// ─── Helpers ──────────────────────────────────────────────────────────────

function planLabel(plan: PlanKey): string {
  return plan.charAt(0).toUpperCase() + plan.slice(1) + ' Plan'
}

function formatRetention(days: number): string {
  if (days === -1) return 'Unlimited'
  if (days === 365) return '1 year'
  return `${days}d`
}

// ─── Sub-components ───────────────────────────────────────────────────────

function UsageMeter({ label, count, limit }: { label: string; count: number; limit: number }) {
  if (limit === -1) {
    return (
      <div className="usage-meter">
        <span className="usage-meter-label">{label}</span>
        <span className="usage-unlimited">Unlimited</span>
      </div>
    )
  }
  const pct = Math.min((count / limit) * 100, 100)
  const atLimit = count >= limit
  return (
    <div className="usage-meter">
      <span className="usage-meter-label">{label}</span>
      <div className="usage-bar-track">
        <div
          className={`usage-bar-fill${atLimit ? ' at-limit' : ''}`}
          style={{ '--fill-pct': `${pct}%` } as React.CSSProperties}
        />
      </div>
      <span className="usage-meter-count">{count} / {limit}</span>
    </div>
  )
}

function PlanBadge({ plan }: { plan: PlanKey }) {
  return (
    <span className={`plan-badge plan-badge-${plan}`}>
      {plan.toUpperCase()}
    </span>
  )
}

// ─── Main Page ────────────────────────────────────────────────────────────

export default function PlanPage() {
  const { role, userId, isSynced } = useUserSync()
  const [data, setData] = useState<PlanData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [switching, setSwitching] = useState<PlanKey | null>(null)
  const [planFlash, setPlanFlash] = useState<{ type: 'success' | 'error'; msg: string } | null>(null)
  const [refreshTick, setRefreshTick] = useState(0)

  // Auth guard: wait until synced, then redirect non-owners
  if (isSynced && role !== 'owner') {
    return <Navigate to="/dashboard" replace />
  }

  async function handleSwitchPlan(newPlan: PlanKey) {
    if (!userId) return
    setSwitching(newPlan)
    setPlanFlash(null)
    try {
      const res = await fetch(`${API_BASE}/v1/owner/plan`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json', 'X-User-ID': userId },
        body: JSON.stringify({ plan: newPlan }),
      })
      const d = await res.json()
      if (!res.ok) throw new Error(d.message ?? d.error ?? `HTTP ${res.status}`)
      setPlanFlash({ type: 'success', msg: `Switched to ${newPlan.charAt(0).toUpperCase() + newPlan.slice(1)} plan.` })
      setRefreshTick(t => t + 1)
    } catch (err) {
      setPlanFlash({ type: 'error', msg: err instanceof Error ? err.message : 'Failed to switch plan' })
    } finally {
      setSwitching(null)
    }
  }

  useEffect(() => {
    if (!isSynced || !userId) return

    const headers = { 'X-User-ID': userId }

    async function load() {
      setLoading(true)
      setError(null)
      try {
        const [settingsRes, keysRes, usersRes] = await Promise.all([
          fetch(`${API_BASE}/v1/owner/settings`, { headers }),
          fetch(`${API_BASE}/v1/admin/api_keys`, { headers }),
          fetch(`${API_BASE}/v1/admin/users`, { headers }),
        ])

        if (!settingsRes.ok) throw new Error(`Settings fetch failed: HTTP ${settingsRes.status}`)
        if (!keysRes.ok) throw new Error(`API keys fetch failed: HTTP ${keysRes.status}`)
        if (!usersRes.ok) throw new Error(`Users fetch failed: HTTP ${usersRes.status}`)

        const [settings, keysData, usersData] = await Promise.all([
          settingsRes.json(),
          keysRes.json(),
          usersRes.json(),
        ])

        setData({
          settings,
          keyCount: Array.isArray(keysData) ? keysData.length : (keysData.count ?? keysData.total ?? 0),
          memberCount: Array.isArray(usersData) ? usersData.length : (usersData.total ?? usersData.count ?? 0),
        })
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load plan data')
      } finally {
        setLoading(false)
      }
    }

    load()
  }, [isSynced, userId, refreshTick])

  const currentPlan = data?.settings.plan ?? 'free'
  const limits = data?.settings.plan_limits
  const isNotBusiness = currentPlan !== 'business'

  return (
    <div className="page-container">
      <Navbar />
      <div className="page-content">
        <div className="plan-page-header">
          <h1>Plan &amp; Billing</h1>
          {data && <PlanBadge plan={currentPlan} />}
        </div>

        {loading && (
          <div className="loading-center">
            <div className="spinner" />
          </div>
        )}

        {error && (
          <div className="flash flash-error">
            {error}
            <button
              className="btn btn-secondary"
              style={{ marginLeft: '1rem' }}
              onClick={() => { setError(null); setLoading(true) }}
            >
              Retry
            </button>
          </div>
        )}

        {planFlash && (
          <div className={`flash ${planFlash.type === 'success' ? 'flash-success' : 'flash-error'}`}
               style={{ marginBottom: '1rem' }}>
            {planFlash.msg}
          </div>
        )}

        {!loading && !error && data && (
          <>
            {/* ── Current Plan Card ── */}
            <div className="card plan-section">
              <div className="plan-hero">
                <h2>{planLabel(currentPlan)}</h2>
                <PlanBadge plan={currentPlan} />
              </div>

              <div className="usage-meters">
                <UsageMeter
                  label="API Keys"
                  count={data.keyCount}
                  limit={limits?.max_api_keys ?? data.settings.max_api_keys}
                />
                <UsageMeter
                  label="Members"
                  count={data.memberCount}
                  limit={limits?.max_members ?? 1}
                />
              </div>

              {limits && (
                <div className="plan-chips">
                  <span className={`plan-chip chip-active`}>
                    Periods: {limits.allowed_periods.length > 2 ? 'All' : limits.allowed_periods.join(', ')}
                  </span>
                  <span className={`plan-chip ${limits.allow_block_action ? 'chip-active' : 'chip-inactive'}`}>
                    Hard block
                  </span>
                  <span className={`plan-chip ${limits.allow_per_key_budget ? 'chip-active' : 'chip-inactive'}`}>
                    Per-key budget
                  </span>
                  <span className="plan-chip chip-active">
                    Retention: {formatRetention(limits.data_retention_days)}
                  </span>
                </div>
              )}
            </div>

            {/* ── Comparison Table ── */}
            <div className="card plan-section">
              <p className="plan-section-title">All Plans</p>
              <div className="plan-table-wrapper">
                <table className="plan-table">
                  <thead>
                    <tr>
                      <th>Feature</th>
                      {PLANS.map(p => (
                        <th key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}>
                          <div className="plan-col-current-header">
                            <span>{p.label}</span>
                            <span className="plan-col-price">{p.price}</span>
                            {p.key === currentPlan && <PlanBadge plan={p.key} />}
                          </div>
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <td>API Keys</td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}>
                          {p.maxKeys}
                        </td>
                      ))}
                    </tr>
                    <tr>
                      <td>Members</td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}>
                          {p.maxMembers}
                        </td>
                      ))}
                    </tr>
                    <tr>
                      <td>Budget periods</td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}>
                          {p.periods}
                        </td>
                      ))}
                    </tr>
                    <tr>
                      <td>Hard block</td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}>
                          {p.block
                            ? <span className="check">✓</span>
                            : <span className="dash">—</span>}
                        </td>
                      ))}
                    </tr>
                    <tr>
                      <td>Per-key budget</td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}>
                          {p.perKey
                            ? <span className="check">✓</span>
                            : <span className="dash">—</span>}
                        </td>
                      ))}
                    </tr>
                    <tr>
                      <td>Data retention</td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}>
                          {p.retention}
                        </td>
                      ))}
                    </tr>
                    <tr>
                      <td></td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}
                            style={{ paddingTop: '1rem', paddingBottom: '1rem' }}>
                          {p.key === currentPlan ? (
                            <span style={{ fontSize: '0.8rem', color: 'var(--color-text-muted)' }}>Current</span>
                          ) : p.key === 'business' ? (
                            <a href="mailto:sales@tokengate.to"
                               className="btn btn-secondary"
                               style={{ fontSize: '0.8rem', padding: '0.35rem 0.75rem', display: 'inline-block' }}>
                              Contact Sales
                            </a>
                          ) : (
                            <button
                              className="btn btn-primary"
                              style={{ fontSize: '0.8rem', padding: '0.35rem 0.75rem' }}
                              disabled={switching !== null}
                              onClick={() => handleSwitchPlan(p.key)}
                            >
                              {switching === p.key ? 'Switching…' : `Switch to ${p.label}`}
                            </button>
                          )}
                        </td>
                      ))}
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            {/* ── Upgrade CTA ── */}
            {isNotBusiness && (
              <div className="upgrade-cta">
                <div className="upgrade-cta-text">
                  <p>Need more capacity?</p>
                  <p>Upgrade your plan to unlock more API keys, team members, and advanced features.</p>
                </div>
                <a
                  href="mailto:sales@tokengate.to"
                  className="btn btn-primary"
                >
                  Contact Sales
                </a>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
