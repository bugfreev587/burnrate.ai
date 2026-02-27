import { useEffect, useState } from 'react'
import { Navigate } from 'react-router-dom'
import Navbar from '../components/Navbar'
import { useUserSync, hasPermission } from '../hooks/useUserSync'
import './PlanPage.css'

const API_BASE = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

// ─── Types ────────────────────────────────────────────────────────────────

type PlanKey = 'free' | 'pro' | 'team' | 'business'

const PLAN_LEVELS: Record<PlanKey, number> = { free: 0, pro: 1, team: 2, business: 3 }

interface PlanLimits {
  max_api_keys: number
  max_provider_keys: number
  max_members: number
  allowed_periods: string[]
  allow_block_action: boolean
  allow_per_key_budget: boolean
  data_retention_days: number
  allow_rate_limits: boolean
  allow_per_key_rate_limit: boolean
  allow_export: boolean
  max_budget_limits: number
  max_rate_limits: number
  max_notification_channels: number
}

interface OwnerSettings {
  plan: PlanKey
  max_api_keys: number
  plan_limits: PlanLimits
}

interface BillingStatus {
  plan: string
  pending_plan: string
  plan_effective_at: string | null
  has_subscription: boolean
}

interface PlanData {
  settings: OwnerSettings
  keyCount: number
  providerKeyCount: number
  memberCount: number
  billing: BillingStatus | null
}

// ─── Static plan comparison data ─────────────────────────────────────────

const PLANS: Array<{
  key: PlanKey
  label: string
  price: string
  maxKeys: string
  maxProviderKeys: string
  maxMembers: string
  perKey: boolean
  retention: string
  spendLimits: string
  rateLimits: string
  notifications: string
  export: boolean
  webhooks: boolean
}> = [
  { key: 'free',     label: 'Free',     price: 'Free',    maxKeys: '1',  maxProviderKeys: '1',  maxMembers: '1',  perKey: false, retention: '7 days',    spendLimits: '1', rateLimits: '1', notifications: '1', export: false, webhooks: false },
  { key: 'pro',      label: 'Pro',      price: '$15/mo',  maxKeys: '5',  maxProviderKeys: '3',  maxMembers: '1',  perKey: false, retention: '90 days',   spendLimits: '∞', rateLimits: '∞', notifications: '∞', export: true,  webhooks: false },
  { key: 'team',     label: 'Team',     price: '$39/mo',  maxKeys: '∞',  maxProviderKeys: '5',  maxMembers: '10', perKey: true,  retention: '180 days',  spendLimits: '∞', rateLimits: '∞', notifications: '∞', export: true,  webhooks: true  },
  { key: 'business', label: 'Business', price: 'Contact', maxKeys: '∞',  maxProviderKeys: '20', maxMembers: '∞',  perKey: true,  retention: 'Unlimited', spendLimits: '∞', rateLimits: '∞', notifications: '∞', export: true,  webhooks: true  },
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

function formatDate(ts: string | null | undefined): string {
  if (!ts) return '—'
  const d = new Date(ts)
  return d.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })
}

function isUpgrade(from: PlanKey, to: PlanKey): boolean {
  return PLAN_LEVELS[to] > PLAN_LEVELS[from]
}

function isDowngrade(from: PlanKey, to: PlanKey): boolean {
  return PLAN_LEVELS[to] < PLAN_LEVELS[from]
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

function useCountdown(targetDate: string | null) {
  const [remaining, setRemaining] = useState<{ days: number; hours: number; minutes: number } | null>(null)

  useEffect(() => {
    if (!targetDate) { setRemaining(null); return }

    function compute() {
      const diff = new Date(targetDate!).getTime() - Date.now()
      if (diff <= 0) { setRemaining(null); return }
      const days = Math.floor(diff / 86_400_000)
      const hours = Math.floor((diff % 86_400_000) / 3_600_000)
      const minutes = Math.floor((diff % 3_600_000) / 60_000)
      setRemaining({ days, hours, minutes })
    }

    compute()
    const id = setInterval(compute, 60_000)
    return () => clearInterval(id)
  }, [targetDate])

  if (!remaining) return null
  const parts: string[] = []
  if (remaining.days > 0) parts.push(`${remaining.days} day${remaining.days !== 1 ? 's' : ''}`)
  if (remaining.hours > 0) parts.push(`${remaining.hours} hour${remaining.hours !== 1 ? 's' : ''}`)
  if (remaining.days === 0 && remaining.minutes > 0) parts.push(`${remaining.minutes} minute${remaining.minutes !== 1 ? 's' : ''}`)
  return parts.length > 0 ? parts.join(', ') + ' remaining' : null
}

function PendingDowngradeCard({ currentPlan, pendingPlan, planEffectiveAt, switching, onCancel }: {
  currentPlan: PlanKey
  pendingPlan: PlanKey
  planEffectiveAt: string | null
  switching: boolean
  onCancel: () => void
}) {
  const countdown = useCountdown(planEffectiveAt)

  return (
    <div className="pending-downgrade-card">
      <div className="pending-downgrade-plans">
        <PlanBadge plan={currentPlan} />
        <span className="pending-downgrade-arrow">→</span>
        <PlanBadge plan={pendingPlan} />
      </div>
      <p className="pending-downgrade-message">
        Your <strong>{planLabel(currentPlan)}</strong> features remain active until <strong>{formatDate(planEffectiveAt)}</strong>.
        After that, your plan will switch to <strong>{planLabel(pendingPlan)}</strong>.
      </p>
      {countdown && (
        <p className="pending-downgrade-countdown">{countdown}</p>
      )}
      <button
        className="btn btn-secondary"
        disabled={switching}
        onClick={onCancel}
      >
        Cancel Downgrade
      </button>
    </div>
  )
}

// ─── Main Page ────────────────────────────────────────────────────────────

export default function PlanPage() {
  const { role, userId, isSynced } = useUserSync()
  const [data, setData] = useState<PlanData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [switching, setSwitching] = useState<PlanKey | null>(null)
  const [resultModal, setResultModal] = useState<{ type: 'success' | 'error' | 'warning'; title: string; message: string } | null>(null)
  const [confirmDowngrade, setConfirmDowngrade] = useState<PlanKey | null>(null)

  // Auth guard: wait until synced, then redirect non-admins
  if (isSynced && !hasPermission(role, 'admin')) {
    return <Navigate to="/dashboard" replace />
  }

  const [refreshTrigger, setRefreshTrigger] = useState(0)

  const currentPlan = (data?.settings.plan ?? 'free') as PlanKey
  const pendingPlan = data?.billing?.pending_plan || ''
  const planEffectiveAt = data?.billing?.plan_effective_at || null

  async function handleSwitchPlan(newPlan: PlanKey) {
    if (!userId) return
    setSwitching(newPlan)
    setResultModal(null)

    try {
      if (isDowngrade(currentPlan, newPlan)) {
        // Schedule downgrade at period end
        const res = await fetch(`${API_BASE}/v1/billing/downgrade`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-User-ID': userId },
          body: JSON.stringify({ plan: newPlan }),
        })
        const d = await res.json()
        if (!res.ok) throw new Error(d.message ?? d.error ?? `HTTP ${res.status}`)
        const effectiveDate = d.plan_effective_at ? formatDate(d.plan_effective_at) : 'end of billing period'
        setResultModal({ type: 'success', title: 'Downgrade Scheduled', message: `Downgrade to ${newPlan.charAt(0).toUpperCase() + newPlan.slice(1)} scheduled for ${effectiveDate}. You keep your current features until then.` })
        setRefreshTrigger(t => t + 1)
      } else if (currentPlan === 'free' && newPlan !== 'free') {
        // Free → Paid: Stripe Checkout
        const res = await fetch(`${API_BASE}/v1/billing/checkout`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-User-ID': userId },
          body: JSON.stringify({
            plan: newPlan,
            success_url: `${window.location.origin}/billing?session_id={CHECKOUT_SESSION_ID}`,
            cancel_url: `${window.location.origin}/plan`,
          }),
        })
        const d = await res.json()
        if (!res.ok) throw new Error(d.message ?? d.error ?? `HTTP ${res.status}`)
        window.location.href = d.url
        return // navigating away
      } else if (isUpgrade(currentPlan, newPlan)) {
        // Paid → higher Paid: immediate upgrade via API
        const res = await fetch(`${API_BASE}/v1/billing/change-plan`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-User-ID': userId },
          body: JSON.stringify({ plan: newPlan }),
        })
        const d = await res.json()
        if (!res.ok) throw new Error(d.message ?? d.error ?? `HTTP ${res.status}`)
        setResultModal({ type: 'success', title: 'Plan Upgraded', message: `Upgraded to ${newPlan.charAt(0).toUpperCase() + newPlan.slice(1)} plan.` })
        setRefreshTrigger(t => t + 1)
      }
    } catch (err) {
      setResultModal({ type: 'error', title: 'Something Went Wrong', message: err instanceof Error ? err.message : 'Failed to switch plan' })
    } finally {
      setSwitching(null)
      setConfirmDowngrade(null)
    }
  }

  async function handleCancelDowngrade() {
    if (!userId) return
    setSwitching('free' as PlanKey) // just to show loading state
    setResultModal(null)

    try {
      const res = await fetch(`${API_BASE}/v1/billing/downgrade/cancel`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-ID': userId },
      })
      const d = await res.json()
      if (!res.ok) throw new Error(d.message ?? d.error ?? `HTTP ${res.status}`)
      setResultModal({ type: 'success', title: 'Downgrade Canceled', message: 'Scheduled downgrade has been canceled. You will stay on your current plan.' })
      setRefreshTrigger(t => t + 1)
    } catch (err) {
      setResultModal({ type: 'error', title: 'Something Went Wrong', message: err instanceof Error ? err.message : 'Failed to cancel downgrade' })
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
        const [settingsRes, keysRes, providerKeysRes, usersRes, billingRes] = await Promise.all([
          fetch(`${API_BASE}/v1/owner/settings`, { headers }),
          fetch(`${API_BASE}/v1/admin/api_keys`, { headers }),
          fetch(`${API_BASE}/v1/admin/provider_keys`, { headers }),
          fetch(`${API_BASE}/v1/admin/users`, { headers }),
          fetch(`${API_BASE}/v1/billing/status`, { headers }),
        ])

        if (!settingsRes.ok) throw new Error(`Settings fetch failed: HTTP ${settingsRes.status}`)
        if (!keysRes.ok) throw new Error(`API keys fetch failed: HTTP ${keysRes.status}`)
        if (!usersRes.ok) throw new Error(`Users fetch failed: HTTP ${usersRes.status}`)

        const [settings, keysData, providerKeysData, usersData] = await Promise.all([
          settingsRes.json(),
          keysRes.json(),
          providerKeysRes.ok ? providerKeysRes.json() : { provider_keys: [] },
          usersRes.json(),
        ])

        const billingData = billingRes.ok ? await billingRes.json() : null

        const pkArr = providerKeysData.provider_keys ?? []

        setData({
          settings,
          keyCount: Array.isArray(keysData) ? keysData.length : (keysData.count ?? keysData.total ?? 0),
          providerKeyCount: Array.isArray(pkArr) ? pkArr.length : 0,
          memberCount: Array.isArray(usersData) ? usersData.length : (usersData.total ?? usersData.count ?? 0),
          billing: billingData,
        })
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load plan data')
      } finally {
        setLoading(false)
      }
    }

    load()
  }, [isSynced, userId, refreshTrigger])

  const limits = data?.settings.plan_limits
  const isNotBusiness = currentPlan !== 'business'

  // Button renderer for the comparison table
  function renderPlanButton(planKey: PlanKey) {
    if (planKey === currentPlan && !pendingPlan) {
      return <span style={{ fontSize: '0.8rem', color: 'var(--color-text-muted)' }}>Current</span>
    }

    if (planKey === 'business') {
      return (
        <a href="mailto:sales@tokengate.to"
           className="btn btn-secondary"
           style={{ fontSize: '0.8rem', padding: '0.35rem 0.75rem', display: 'inline-block' }}>
          Contact Sales
        </a>
      )
    }

    // If this plan is the pending downgrade target
    if (pendingPlan && planKey === pendingPlan) {
      return (
        <span style={{ fontSize: '0.8rem', color: 'var(--color-warning, #fb923c)' }}>
          Switching {formatDate(planEffectiveAt)}
        </span>
      )
    }

    // If there's a pending downgrade and this is the current plan
    if (pendingPlan && planKey === currentPlan) {
      return <span style={{ fontSize: '0.8rem', color: 'var(--color-text-muted)' }}>Current (until {formatDate(planEffectiveAt)})</span>
    }

    // Determine if upgrade or downgrade
    const isDown = isDowngrade(currentPlan, planKey)

    if (isDown) {
      // Downgrade button — requires confirmation
      return (
        <button
          className="btn btn-secondary"
          style={{ fontSize: '0.8rem', padding: '0.35rem 0.75rem' }}
          disabled={switching !== null || !!pendingPlan}
          onClick={() => setConfirmDowngrade(planKey)}
        >
          {switching === planKey ? 'Scheduling…' : `Downgrade to ${PLANS.find(p => p.key === planKey)!.label}`}
        </button>
      )
    }

    // Upgrade button
    return (
      <button
        className="btn btn-primary"
        style={{ fontSize: '0.8rem', padding: '0.35rem 0.75rem' }}
        disabled={switching !== null}
        onClick={() => handleSwitchPlan(planKey)}
      >
        {switching === planKey ? 'Upgrading…' : `Upgrade to ${PLANS.find(p => p.key === planKey)!.label}`}
      </button>
    )
  }

  return (
    <div className="page-container">
      <Navbar />
      <div className="page-content">
        <div className="plan-page-header">
          <h1>Plan</h1>
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

        {/* ── Result Modal ── */}
        {resultModal && (
          <div className="modal-overlay" onClick={() => setResultModal(null)}>
            <div className="modal-box plan-modal" onClick={e => e.stopPropagation()}>
              <div className={`plan-modal-icon plan-modal-icon-${resultModal.type}`}>
                {resultModal.type === 'success' ? '✓' : resultModal.type === 'error' ? '✕' : '⚠'}
              </div>
              <h2 className="plan-modal-title">{resultModal.title}</h2>
              <p className="plan-modal-message">{resultModal.message}</p>
              <button className="btn btn-primary" onClick={() => setResultModal(null)}>
                OK
              </button>
            </div>
          </div>
        )}

        {/* ── Downgrade Confirmation Modal ── */}
        {confirmDowngrade && (
          <div className="modal-overlay" onClick={() => setConfirmDowngrade(null)}>
            <div className="modal-box plan-modal" onClick={e => e.stopPropagation()}>
              <div className="plan-modal-icon plan-modal-icon-warning">⚠</div>
              <h2 className="plan-modal-title">Confirm Downgrade</h2>
              <p className="plan-modal-message">
                Your {planLabel(currentPlan)} features remain active until the end of your billing period.
                After that, you'll switch to {planLabel(confirmDowngrade as PlanKey)}.
              </p>
              <div className="plan-modal-actions">
                <button className="btn btn-secondary" onClick={() => setConfirmDowngrade(null)}>
                  Cancel
                </button>
                <button
                  className="btn btn-primary"
                  disabled={switching !== null}
                  onClick={() => handleSwitchPlan(confirmDowngrade)}
                >
                  {switching === confirmDowngrade ? 'Scheduling…' : 'Confirm Downgrade'}
                </button>
              </div>
            </div>
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
                  label="Provider Keys"
                  count={data.providerKeyCount}
                  limit={limits?.max_provider_keys ?? 1}
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

            {/* ── Pending Downgrade Card ── */}
            {pendingPlan && (
              <PendingDowngradeCard
                currentPlan={currentPlan}
                pendingPlan={pendingPlan as PlanKey}
                planEffectiveAt={planEffectiveAt}
                switching={switching !== null}
                onCancel={handleCancelDowngrade}
              />
            )}

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
                      <td>Provider Keys</td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}>
                          {p.maxProviderKeys}
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
                      <td>Spend limits</td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}>
                          {p.spendLimits}
                        </td>
                      ))}
                    </tr>
                    <tr>
                      <td>Rate limits</td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}>
                          {p.rateLimits}
                        </td>
                      ))}
                    </tr>
                    <tr>
                      <td>Notifications</td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}>
                          {p.notifications}
                        </td>
                      ))}
                    </tr>
                    <tr>
                      <td>Audit export</td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}>
                          {p.export
                            ? <span className="check">✓</span>
                            : <span className="dash">—</span>}
                        </td>
                      ))}
                    </tr>
                    <tr>
                      <td>Webhooks</td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}>
                          {p.webhooks
                            ? <span className="check">✓</span>
                            : <span className="dash">—</span>}
                        </td>
                      ))}
                    </tr>
                    <tr>
                      <td></td>
                      {PLANS.map(p => (
                        <td key={p.key} className={p.key === currentPlan ? 'plan-col-current' : ''}
                            style={{ paddingTop: '1rem', paddingBottom: '1rem' }}>
                          {renderPlanButton(p.key)}
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
