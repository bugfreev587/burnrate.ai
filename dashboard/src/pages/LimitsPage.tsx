import { useState, useEffect, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { useUserSync, hasPermission } from '../hooks/useUserSync'
import { useRateLimits } from '../hooks/useRateLimits'
import { useSpendLimits } from '../hooks/useSpendLimits'
import { usePricingConfig } from '../hooks/usePricingConfig'
import { useDashboardConfig } from '../hooks/useDashboardConfig'
import type { RateLimit, UpsertRateLimitReq } from '../hooks/useRateLimits'
import type { SpendLimit, UpsertSpendLimitReq } from '../hooks/useSpendLimits'
import Navbar from '../components/Navbar'
import './LimitsPage.css'
import './ManagementPage.css'

const METRIC_OPTIONS = [
  { value: 'rpm', label: 'RPM', description: 'Requests per minute' },
  { value: 'itpm', label: 'ITPM', description: 'Input tokens per minute' },
  { value: 'otpm', label: 'OTPM', description: 'Output tokens per minute' },
]

const PERIOD_OPTIONS = [
  { value: 'monthly', label: 'Monthly' },
  { value: 'weekly', label: 'Weekly' },
  { value: 'daily', label: 'Daily' },
]

const ACTION_OPTIONS = [
  { value: 'alert', label: 'Alert', description: 'Warn via headers when threshold is reached' },
  { value: 'block', label: 'Hard Block', description: 'Reject requests (HTTP 402) when limit is exceeded' },
]

export default function LimitsPage() {
  const navigate = useNavigate()
  const { role, isSynced } = useUserSync()
  const { limits: rateLimits, loading: rlLoading, error: rlError, upsertLimit: upsertRateLimit, deleteLimit: deleteRateLimit } = useRateLimits(5000)
  const { limits: spendLimits, loading: slLoading, error: slError, upsertLimit: upsertSpendLimit, deleteLimit: deleteSpendLimit } = useSpendLimits()
  const { catalog, activeKeys } = usePricingConfig()
  const { config } = useDashboardConfig()
  const isFreePlan = config?.plan === 'free'

  // ── Provider / model options from catalog ─────────────────────────────────
  const providerOptions = useMemo(() => {
    const seen = new Map<string, string>()
    for (const entry of catalog) {
      if (!seen.has(entry.provider)) {
        seen.set(entry.provider, entry.provider_display || entry.provider)
      }
    }
    return [
      { value: '', label: 'All Providers' },
      ...Array.from(seen.entries()).map(([value, display]) => ({ value, label: display })),
    ]
  }, [catalog])

  // ── Rate limit modal state ────────────────────────────────────────────────
  const [showRLModal, setShowRLModal] = useState(false)
  const [rlProvider, setRlProvider] = useState('')
  const [rlModel, setRlModel] = useState('')
  const [rlMetric, setRlMetric] = useState('rpm')
  const [rlLimitValue, setRlLimitValue] = useState('')
  const [rlWindowSeconds, setRlWindowSeconds] = useState('60')
  const [rlFormError, setRlFormError] = useState<string | null>(null)
  const [rlSaving, setRlSaving] = useState(false)

  const modelOptions = useMemo(() => {
    const models = catalog
      .filter(entry => !rlProvider || entry.provider === rlProvider)
      .map(entry => entry.model_name)
    const unique = Array.from(new Set(models)).sort()
    return [{ value: '', label: 'All Models' }, ...unique.map(m => ({ value: m, label: m }))]
  }, [catalog, rlProvider])

  // ── Spend limit modal state ───────────────────────────────────────────────
  const [showSLModal, setShowSLModal] = useState(false)
  const [slScope, setSlScope] = useState<'account' | 'api_key'>('account')
  const [slKeyId, setSlKeyId] = useState('')
  const [slProvider, setSlProvider] = useState('')
  const [slPeriod, setSlPeriod] = useState('monthly')
  const [slLimitAmount, setSlLimitAmount] = useState('')
  const [slThreshold, setSlThreshold] = useState('80')
  const [slActions, setSlActions] = useState<string[]>(['alert'])
  const [slFormError, setSlFormError] = useState<string | null>(null)
  const [slSaving, setSlSaving] = useState(false)

  // ── Editing state ─────────────────────────────────────────────────────────
  const [editingSL, setEditingSL] = useState<SpendLimit | null>(null)
  const [editingRL, setEditingRL] = useState<RateLimit | null>(null)

  // ── Shared state ──────────────────────────────────────────────────────────
  const [successMsg, setSuccessMsg] = useState<string | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  const canAccess = isSynced && hasPermission(role, 'editor')

  useEffect(() => {
    if (isSynced && !canAccess) navigate('/dashboard')
  }, [isSynced, canAccess, navigate])

  const showSuccess = (msg: string) => { setSuccessMsg(msg); setTimeout(() => setSuccessMsg(null), 3000) }
  const showError = (msg: string) => { setErrorMsg(msg); setTimeout(() => setErrorMsg(null), 5000) }

  // ── Rate limit handlers ───────────────────────────────────────────────────
  const resetRLForm = () => {
    setRlProvider(''); setRlModel(''); setRlMetric('rpm')
    setRlLimitValue(''); setRlWindowSeconds('60'); setRlFormError(null)
  }

  const handleSaveRL = async () => {
    const limitVal = parseInt(rlLimitValue, 10)
    if (!limitVal || limitVal <= 0) { setRlFormError('Limit value must be a positive number'); return }
    const windowSec = parseInt(rlWindowSeconds, 10) || 60
    setRlSaving(true); setRlFormError(null)
    try {
      const req: UpsertRateLimitReq = {
        provider: rlProvider, model: rlModel, scope_type: 'account', scope_id: '',
        metric: rlMetric, limit_value: limitVal, window_seconds: windowSec, enabled: true,
      }
      await upsertRateLimit(req)
      showSuccess('Rate limit saved'); setShowRLModal(false); setEditingRL(null); resetRLForm()
    } catch (e) { setRlFormError(e instanceof Error ? e.message : 'Failed to save') }
    finally { setRlSaving(false) }
  }

  const handleDeleteRL = async (id: number) => {
    if (!confirm('Delete this rate limit?')) return
    try { await deleteRateLimit(id); showSuccess('Rate limit deleted') }
    catch (e) { showError(e instanceof Error ? e.message : 'Failed to delete') }
  }

  // ── Spend limit handlers ──────────────────────────────────────────────────
  const resetSLForm = () => {
    setSlScope('account'); setSlKeyId(''); setSlProvider(''); setSlPeriod('monthly')
    setSlLimitAmount(''); setSlThreshold('80'); setSlActions(['alert']); setSlFormError(null)
  }

  const handleSaveSL = async () => {
    const amount = parseFloat(slLimitAmount)
    if (!amount || amount <= 0) { setSlFormError('Limit amount must be a positive number'); return }
    const threshold = parseFloat(slThreshold)
    if (isNaN(threshold) || threshold < 0 || threshold > 100) { setSlFormError('Alert threshold must be 0-100'); return }
    if (slActions.length === 0) { setSlFormError('Select at least one action'); return }
    if (slScope === 'api_key' && !slKeyId) { setSlFormError('Please select an API key'); return }
    const action = slActions.includes('alert') && slActions.includes('block')
      ? 'alert_block'
      : slActions[0]
    setSlSaving(true); setSlFormError(null)
    try {
      const req: UpsertSpendLimitReq = {
        scope_type: slScope, scope_id: slScope === 'api_key' ? slKeyId : '',
        period_type: slPeriod, provider: slProvider,
        limit_amount: slLimitAmount, alert_threshold: String(threshold), action,
      }
      await upsertSpendLimit(req)
      showSuccess('Spend limit saved'); setShowSLModal(false); setEditingSL(null); resetSLForm()
    } catch (e) { setSlFormError(e instanceof Error ? e.message : 'Failed to save') }
    finally { setSlSaving(false) }
  }

  const handleDeleteSL = async (id: number) => {
    if (!confirm('Delete this spend limit?')) return
    try { await deleteSpendLimit(id); showSuccess('Spend limit deleted') }
    catch (e) { showError(e instanceof Error ? e.message : 'Failed to delete') }
  }

  const handleEditSL = (l: SpendLimit) => {
    setEditingSL(l)
    setSlScope(l.scope_type as 'account' | 'api_key')
    setSlKeyId(l.scope_id || '')
    setSlProvider(l.provider || '')
    setSlPeriod(l.period_type)
    setSlLimitAmount(l.limit_amount)
    setSlThreshold(l.alert_threshold || '80')
    const actions: string[] = l.action === 'alert_block' ? ['alert', 'block'] : [l.action]
    setSlActions(actions)
    setSlFormError(null)
    setShowSLModal(true)
  }

  const handleEditRL = (l: RateLimit) => {
    setEditingRL(l)
    setRlProvider(l.Provider || '')
    setRlModel(l.Model || '')
    setRlMetric(l.Metric)
    setRlLimitValue(String(l.LimitValue))
    setRlWindowSeconds(String(l.WindowSeconds))
    setRlFormError(null)
    setShowRLModal(true)
  }

  // ── Helpers ───────────────────────────────────────────────────────────────
  const metricLabel = (m: string) => METRIC_OPTIONS.find(o => o.value === m)?.label ?? m.toUpperCase()

  const loading = rlLoading || slLoading

  if (!isSynced || loading) {
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
        <div className="limits-container">

          <div className="limits-header">
            <h1>Limits</h1>
          </div>

          {successMsg && <div className="flash flash-success">{successMsg}</div>}
          {errorMsg && <div className="flash flash-error">{errorMsg}</div>}
          {rlError && <div className="flash flash-error">{rlError}</div>}
          {slError && <div className="flash flash-error">{slError}</div>}

          {/* ── Spend Limits Section ──────────────────────────────────────── */}
          <section className="mgmt-section">
            <div className="section-hdr">
              <div>
                <h2>Spend Limits</h2>
                <p className="section-desc">
                  Set budget caps to control spending. Alert-only limits warn via response headers;
                  hard-block limits reject requests (HTTP 402) when exceeded.
                </p>
              </div>
              <button className="btn btn-primary" onClick={() => { setEditingSL(null); resetSLForm(); setShowSLModal(true) }} disabled={isFreePlan} title={isFreePlan ? 'Upgrade to unlock' : undefined}>
                {isFreePlan && <svg className="lock-icon" viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M4 7V5a4 4 0 1 1 8 0v2h1a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V8a1 1 0 0 1 1-1h1zm2-2a2 2 0 1 1 4 0v2H6V5z"/></svg>}
                Add Spend Limit
              </button>
            </div>

            <div className="table-scroll">
              <table className="mgmt-table">
                <thead>
                  <tr>
                    <th>Scope</th>
                    <th>Provider</th>
                    <th>Period</th>
                    <th>Limit (USD)</th>
                    <th>Alert At</th>
                    <th>Action</th>
                    <th>Current Spend</th>
                    <th>Usage</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {spendLimits.length === 0 ? (
                    <tr>
                      <td colSpan={9} className="empty-cell">
                        <div className="empty-cta">
                          <p>No spend limits configured yet.</p>
                          <button className="btn btn-primary" onClick={() => { setEditingSL(null); resetSLForm(); setShowSLModal(true) }} disabled={isFreePlan} title={isFreePlan ? 'Upgrade to unlock' : undefined}>
                            {isFreePlan && <svg className="lock-icon" viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M4 7V5a4 4 0 1 1 8 0v2h1a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V8a1 1 0 0 1 1-1h1zm2-2a2 2 0 1 1 4 0v2H6V5z"/></svg>}
                            Add Your First Spend Limit
                          </button>
                        </div>
                      </td>
                    </tr>
                  ) : [...spendLimits].sort((a, b) => {
                    // Account-level first, then per-key
                    if (a.scope_type !== b.scope_type) return a.scope_type === 'account' ? -1 : 1
                    const periodOrder: Record<string, number> = { monthly: 0, weekly: 1, daily: 2 }
                    const pa = periodOrder[a.period_type] ?? 9
                    const pb = periodOrder[b.period_type] ?? 9
                    if (pa !== pb) return pa - pb
                    if (!a.provider && b.provider) return -1
                    if (a.provider && !b.provider) return 1
                    return a.provider.localeCompare(b.provider)
                  }).map(l => {
                    const pct = l.pct_used
                    const thresholdNum = parseFloat(l.alert_threshold) || 80
                    const barClass = pct >= 100 ? 'usage-exceeded' : pct >= thresholdNum ? 'usage-high' : ''
                    return (
                      <tr key={l.id}>
                        <td>
                          {l.scope_type === 'api_key' ? (
                            <span className="mode-badge" title={l.scope_id}>
                              {l.key_label || l.scope_id?.slice(0, 8) + '…'}
                            </span>
                          ) : (
                            <span className="provider-badge">Account</span>
                          )}
                        </td>
                        <td>
                          <span className="provider-badge">
                            {l.provider ? l.provider.charAt(0).toUpperCase() + l.provider.slice(1) : 'All'}
                          </span>
                        </td>
                        <td>
                          <span className={`metric-badge metric-${l.period_type === 'monthly' ? 'rpm' : l.period_type === 'weekly' ? 'itpm' : 'otpm'}`}>
                            {l.period_type.charAt(0).toUpperCase() + l.period_type.slice(1)}
                          </span>
                        </td>
                        <td>${parseFloat(l.limit_amount).toFixed(2)}</td>
                        <td>{l.alert_threshold}%</td>
                        <td>
                          {l.action === 'alert_block' ? (
                            <>
                              <span className="action-badge action-alert">Alert</span>
                              {' '}
                              <span className="action-badge action-block">Hard Block</span>
                            </>
                          ) : (
                            <span className={`action-badge action-${l.action}`}>
                              {l.action === 'block' ? 'Hard Block' : 'Alert'}
                            </span>
                          )}
                        </td>
                        <td>${parseFloat(l.current_spend).toFixed(2)}</td>
                        <td>
                          <div className="usage-bar-container">
                            <div className="usage-bar">
                              <div
                                className={`usage-bar-fill ${barClass}`}
                                style={{ width: `${Math.min(pct, 100)}%` }}
                              />
                            </div>
                            <span className="usage-text">{pct.toFixed(1)}%</span>
                          </div>
                        </td>
                        <td>
                          <button className="btn btn-small btn-secondary" onClick={() => handleEditSL(l)} disabled={isFreePlan} title={isFreePlan ? 'Upgrade to unlock' : undefined}>
                            {isFreePlan && <svg className="lock-icon" viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M4 7V5a4 4 0 1 1 8 0v2h1a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V8a1 1 0 0 1 1-1h1zm2-2a2 2 0 1 1 4 0v2H6V5z"/></svg>}
                            Edit
                          </button>
                          <button className="btn btn-small btn-danger" onClick={() => handleDeleteSL(l.id)}>
                            Delete
                          </button>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </section>

          {/* ── Rate Limits Section ───────────────────────────────────────── */}
          <section className="mgmt-section">
            <div className="section-hdr">
              <div>
                <h2>Rate Limits</h2>
                <p className="section-desc">
                  Set per-model rate limits (requests per minute, input/output tokens per minute) to control usage.
                </p>
              </div>
              <button className="btn btn-primary" onClick={() => { setEditingRL(null); resetRLForm(); setShowRLModal(true) }} disabled={isFreePlan} title={isFreePlan ? 'Upgrade to unlock' : undefined}>
                {isFreePlan && <svg className="lock-icon" viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M4 7V5a4 4 0 1 1 8 0v2h1a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V8a1 1 0 0 1 1-1h1zm2-2a2 2 0 1 1 4 0v2H6V5z"/></svg>}
                Add Rate Limit
              </button>
            </div>

            <div className="table-scroll">
              <table className="mgmt-table">
                <thead>
                  <tr>
                    <th>Provider</th>
                    <th>Model</th>
                    <th>Metric</th>
                    <th>Limit</th>
                    <th>Window</th>
                    <th>Status</th>
                    <th>Current Usage</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {rateLimits.length === 0 ? (
                    <tr>
                      <td colSpan={8} className="empty-cell">
                        <div className="empty-cta">
                          <p>No rate limits configured yet.</p>
                          <button className="btn btn-primary" onClick={() => { setEditingRL(null); resetRLForm(); setShowRLModal(true) }} disabled={isFreePlan} title={isFreePlan ? 'Upgrade to unlock' : undefined}>
                            {isFreePlan && <svg className="lock-icon" viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M4 7V5a4 4 0 1 1 8 0v2h1a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V8a1 1 0 0 1 1-1h1zm2-2a2 2 0 1 1 4 0v2H6V5z"/></svg>}
                            Add Your First Rate Limit
                          </button>
                        </div>
                      </td>
                    </tr>
                  ) : rateLimits.map(l => {
                    const pct = l.LimitValue > 0 ? (l.current_usage / l.LimitValue) * 100 : 0
                    const barClass = pct >= 100 ? 'usage-exceeded' : pct >= 80 ? 'usage-high' : ''
                    return (
                      <tr key={l.ID}>
                        <td><span className="provider-badge">{l.Provider || 'All'}</span></td>
                        <td>{l.Model || 'All models'}</td>
                        <td><span className={`metric-badge metric-${l.Metric}`}>{metricLabel(l.Metric)}</span></td>
                        <td>{l.LimitValue.toLocaleString()}</td>
                        <td>{l.WindowSeconds}s</td>
                        <td>
                          <span className={`enabled-dot ${l.Enabled ? 'on' : 'off'}`} />
                          {l.Enabled ? 'Active' : 'Disabled'}
                        </td>
                        <td>
                          <div className="usage-bar-container">
                            <div className="usage-bar">
                              <div
                                className={`usage-bar-fill ${barClass}`}
                                style={{ width: `${Math.min(pct, 100)}%` }}
                              />
                            </div>
                            <span className="usage-text">
                              {l.current_usage.toLocaleString()}/{l.LimitValue.toLocaleString()}
                            </span>
                          </div>
                        </td>
                        <td>
                          <button className="btn btn-small btn-secondary" onClick={() => handleEditRL(l)} disabled={isFreePlan} title={isFreePlan ? 'Upgrade to unlock' : undefined}>
                            {isFreePlan && <svg className="lock-icon" viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M4 7V5a4 4 0 1 1 8 0v2h1a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V8a1 1 0 0 1 1-1h1zm2-2a2 2 0 1 1 4 0v2H6V5z"/></svg>}
                            Edit
                          </button>
                          <button className="btn btn-small btn-danger" onClick={() => handleDeleteRL(l.ID)}>
                            Delete
                          </button>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </section>

        </div>
      </div>

      {/* ── Add Spend Limit Modal ──────────────────────────────────────────── */}
      {showSLModal && (
        <div className="modal-overlay" onClick={() => { setShowSLModal(false); setEditingSL(null); setSlFormError(null) }}>
          <div className="modal-box" onClick={e => e.stopPropagation()}>
            <div className="modal-hdr">
              <h2>{editingSL ? 'Edit Spend Limit' : 'Add Spend Limit'}</h2>
            </div>
            <div className="modal-body">
              <p className="modal-hint">
                Set a budget cap for your account or a specific API key. When the limit is reached,
                the configured action (alert or block) will take effect.
              </p>

              <div className="form-group">
                <label>Scope <span className="required">*</span></label>
                <div className="role-select">
                  <label className={`role-option ${slScope === 'account' ? 'selected' : ''} ${editingSL ? 'disabled' : ''}`}>
                    <input type="radio" name="sl-scope" value="account"
                      checked={slScope === 'account'}
                      disabled={!!editingSL}
                      onChange={() => { setSlScope('account'); setSlKeyId('') }} />
                    <div>
                      <strong>Account</strong>
                      <span className="role-desc">Applies to all usage across the account</span>
                    </div>
                  </label>
                  <label className={`role-option ${slScope === 'api_key' ? 'selected' : ''} ${editingSL ? 'disabled' : ''}`}>
                    <input type="radio" name="sl-scope" value="api_key"
                      checked={slScope === 'api_key'}
                      disabled={!!editingSL}
                      onChange={() => { setSlScope('api_key'); setSlKeyId(activeKeys[0]?.key_id ?? ''); setSlProvider('') }} />
                    <div>
                      <strong>Per API Key</strong>
                      <span className="role-desc">Applies to a single API key (Team+ plans)</span>
                    </div>
                  </label>
                </div>
              </div>

              {slScope === 'api_key' && (
                <div className="form-group">
                  <label>API Key <span className="required">*</span></label>
                  {activeKeys.length === 0 ? (
                    <p className="text-muted">No active API keys. Create one in Management first.</p>
                  ) : (
                    <select value={slKeyId} onChange={e => setSlKeyId(e.target.value)} disabled={!!editingSL}>
                      {activeKeys.map(k => (
                        <option key={k.key_id} value={k.key_id}>
                          {k.label} ({k.key_id.slice(0, 8)}…)
                        </option>
                      ))}
                    </select>
                  )}
                </div>
              )}

              {slScope === 'account' && (
                <div className="form-group">
                  <label>Provider <span className="required">*</span></label>
                  <select value={slProvider} onChange={e => setSlProvider(e.target.value)} disabled={!!editingSL}>
                    <option value="">All Providers</option>
                    <option value="anthropic">Anthropic</option>
                    <option value="openai">OpenAI</option>
                  </select>
                </div>
              )}

              <div className="form-group">
                <label>Period <span className="required">*</span></label>
                <select value={slPeriod} onChange={e => setSlPeriod(e.target.value)} disabled={!!editingSL}>
                  {PERIOD_OPTIONS.map(p => (
                    <option key={p.value} value={p.value}>{p.label}</option>
                  ))}
                </select>
              </div>

              <div className="form-group">
                <label>Limit Amount (USD) <span className="required">*</span></label>
                <input
                  type="number" value={slLimitAmount}
                  onChange={e => setSlLimitAmount(e.target.value)}
                  placeholder="e.g. 100.00" min="0.01" step="0.01" autoFocus
                />
              </div>

              <div className="form-group">
                <label>Alert Threshold (%)</label>
                <input
                  type="number" value={slThreshold}
                  onChange={e => setSlThreshold(e.target.value)}
                  placeholder="80" min="0" max="100"
                />
                <span className="form-hint">Warn via response headers when spend reaches this percentage of the limit.</span>
              </div>

              <div className="form-group">
                <label>Action</label>
                <div className="role-select">
                  {ACTION_OPTIONS.map(a => {
                    const checked = slActions.includes(a.value)
                    return (
                      <label key={a.value} className={`role-option ${checked ? 'selected' : ''}`}>
                        <input
                          type="checkbox" value={a.value}
                          checked={checked}
                          onChange={() => setSlActions(prev =>
                            prev.includes(a.value) ? prev.filter(v => v !== a.value) : [...prev, a.value]
                          )}
                        />
                        <div>
                          <strong>{a.label}</strong>
                          <span className="role-desc">{a.description}</span>
                        </div>
                      </label>
                    )
                  })}
                </div>
              </div>
            </div>

            {slFormError && <div className="flash flash-error modal-flash">{slFormError}</div>}

            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => { setShowSLModal(false); setEditingSL(null); setSlFormError(null) }}
                disabled={slSaving}>Cancel</button>
              <button className="btn btn-primary" onClick={handleSaveSL}
                disabled={!slLimitAmount || slSaving}>
                {slSaving ? 'Saving...' : 'Save'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── Add Rate Limit Modal ───────────────────────────────────────────── */}
      {showRLModal && (
        <div className="modal-overlay" onClick={() => { setShowRLModal(false); setEditingRL(null); setRlFormError(null) }}>
          <div className="modal-box" onClick={e => e.stopPropagation()}>
            <div className="modal-hdr">
              <h2>{editingRL ? 'Edit Rate Limit' : 'Add Rate Limit'}</h2>
            </div>
            <div className="modal-body">
              <p className="modal-hint">
                Configure a rate limit for specific providers/models or across all traffic.
              </p>

              <div className="form-group">
                <label>Provider</label>
                <select value={rlProvider} onChange={e => { setRlProvider(e.target.value); setRlModel('') }} disabled={!!editingRL}>
                  {providerOptions.map(o => (
                    <option key={o.value} value={o.value}>{o.label}</option>
                  ))}
                </select>
              </div>

              <div className="form-group">
                <label>Model</label>
                <select value={rlModel} onChange={e => setRlModel(e.target.value)} disabled={!!editingRL}>
                  {modelOptions.map(o => (
                    <option key={o.value} value={o.value}>{o.label}</option>
                  ))}
                </select>
              </div>

              <div className="form-group">
                <label>Metric</label>
                <select value={rlMetric} onChange={e => setRlMetric(e.target.value)} disabled={!!editingRL}>
                  {METRIC_OPTIONS.map(m => (
                    <option key={m.value} value={m.value}>{m.label} — {m.description}</option>
                  ))}
                </select>
              </div>

              <div className="form-group">
                <label>Limit Value <span className="required">*</span></label>
                <input
                  type="number" value={rlLimitValue}
                  onChange={e => setRlLimitValue(e.target.value)}
                  placeholder={rlMetric === 'rpm' ? 'e.g. 100' : 'e.g. 1000000'}
                  min="1" autoFocus
                />
              </div>

              <div className="form-group">
                <label>Window (seconds)</label>
                <input
                  type="number" value={rlWindowSeconds}
                  onChange={e => setRlWindowSeconds(e.target.value)}
                  placeholder="60" min="1"
                />
              </div>
            </div>

            {rlFormError && <div className="flash flash-error modal-flash">{rlFormError}</div>}

            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => { setShowRLModal(false); setEditingRL(null); setRlFormError(null) }}
                disabled={rlSaving}>Cancel</button>
              <button className="btn btn-primary" onClick={handleSaveRL}
                disabled={!rlLimitValue || rlSaving}>
                {rlSaving ? 'Saving...' : 'Save'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
