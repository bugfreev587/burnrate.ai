import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useUserSync, hasPermission } from '../hooks/useUserSync'
import { useRateLimits } from '../hooks/useRateLimits'
import type { UpsertRateLimitReq } from '../hooks/useRateLimits'
import Navbar from '../components/Navbar'
import './RateLimitsPage.css'
import '../pages/ManagementPage.css'

const METRIC_OPTIONS = [
  { value: 'rpm', label: 'RPM', description: 'Requests per minute' },
  { value: 'itpm', label: 'ITPM', description: 'Input tokens per minute' },
  { value: 'otpm', label: 'OTPM', description: 'Output tokens per minute' },
]

const PROVIDER_OPTIONS = [
  { value: '', label: 'All Providers' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'google', label: 'Google' },
]

export default function RateLimitsPage() {
  const navigate = useNavigate()
  const { role, isSynced } = useUserSync()
  const { limits, loading, error, upsertLimit, deleteLimit } = useRateLimits()

  const [showModal, setShowModal] = useState(false)
  const [formProvider, setFormProvider] = useState('')
  const [formModel, setFormModel] = useState('')
  const [formMetric, setFormMetric] = useState('rpm')
  const [formLimitValue, setFormLimitValue] = useState('')
  const [formWindowSeconds, setFormWindowSeconds] = useState('60')
  const [formError, setFormError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [successMsg, setSuccessMsg] = useState<string | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  const isAdmin = isSynced && hasPermission(role, 'admin')

  useEffect(() => {
    if (isSynced && !isAdmin) {
      navigate('/dashboard')
    }
  }, [isSynced, isAdmin, navigate])

  const showSuccess = (msg: string) => {
    setSuccessMsg(msg)
    setTimeout(() => setSuccessMsg(null), 3000)
  }
  const showError = (msg: string) => {
    setErrorMsg(msg)
    setTimeout(() => setErrorMsg(null), 5000)
  }

  const resetForm = () => {
    setFormProvider('')
    setFormModel('')
    setFormMetric('rpm')
    setFormLimitValue('')
    setFormWindowSeconds('60')
    setFormError(null)
  }

  const handleSave = async () => {
    const limitVal = parseInt(formLimitValue, 10)
    if (!limitVal || limitVal <= 0) {
      setFormError('Limit value must be a positive number')
      return
    }
    const windowSec = parseInt(formWindowSeconds, 10) || 60

    setSaving(true)
    setFormError(null)
    try {
      const req: UpsertRateLimitReq = {
        provider: formProvider,
        model: formModel,
        scope_type: 'account',
        scope_id: '',
        metric: formMetric,
        limit_value: limitVal,
        window_seconds: windowSec,
        enabled: true,
      }
      await upsertLimit(req)
      showSuccess('Rate limit saved')
      setShowModal(false)
      resetForm()
    } catch (e) {
      setFormError(e instanceof Error ? e.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Delete this rate limit?')) return
    try {
      await deleteLimit(id)
      showSuccess('Rate limit deleted')
    } catch (e) {
      showError(e instanceof Error ? e.message : 'Failed to delete')
    }
  }

  const metricLabel = (m: string) => METRIC_OPTIONS.find(o => o.value === m)?.label ?? m.toUpperCase()

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
        <div className="rl-container">

          <div className="rl-header">
            <h1>Rate Limits</h1>
          </div>

          {successMsg && <div className="flash flash-success">{successMsg}</div>}
          {errorMsg && <div className="flash flash-error">{errorMsg}</div>}
          {error && <div className="flash flash-error">{error}</div>}

          <section className="mgmt-section">
            <div className="section-hdr">
              <div>
                <h2>Configured Limits</h2>
                <p className="section-desc">
                  Set per-model rate limits (requests per minute, input/output tokens per minute) to control usage.
                </p>
              </div>
              <button className="btn btn-primary" onClick={() => { resetForm(); setShowModal(true) }}>
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
                  {limits.length === 0 ? (
                    <tr>
                      <td colSpan={8} className="empty-cell">
                        <div className="empty-cta">
                          <p>No rate limits configured yet.</p>
                          <button className="btn btn-primary" onClick={() => { resetForm(); setShowModal(true) }}>
                            Add Your First Rate Limit
                          </button>
                        </div>
                      </td>
                    </tr>
                  ) : limits.map(l => {
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
                          <button className="btn btn-small btn-danger" onClick={() => handleDelete(l.ID)}>
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

      {/* ── Add/Edit Rate Limit Modal ─────────────────────────────────────── */}
      {showModal && (
        <div className="modal-overlay" onClick={() => { setShowModal(false); setFormError(null) }}>
          <div className="modal-box" onClick={e => e.stopPropagation()}>
            <div className="modal-hdr">
              <h2>Add Rate Limit</h2>
            </div>
            <div className="modal-body">
              <p className="modal-hint">
                Configure a rate limit for specific providers/models or across all traffic.
              </p>

              <div className="form-group">
                <label>Provider</label>
                <select value={formProvider} onChange={e => setFormProvider(e.target.value)}>
                  {PROVIDER_OPTIONS.map(o => (
                    <option key={o.value} value={o.value}>{o.label}</option>
                  ))}
                </select>
              </div>

              <div className="form-group">
                <label>Model <span className="optional">(leave empty for all models)</span></label>
                <input
                  type="text"
                  value={formModel}
                  onChange={e => setFormModel(e.target.value)}
                  placeholder="e.g. claude-sonnet-4-6"
                />
              </div>

              <div className="form-group">
                <label>Metric</label>
                <div className="role-select">
                  {METRIC_OPTIONS.map(m => (
                    <label key={m.value} className={`role-option ${formMetric === m.value ? 'selected' : ''}`}>
                      <input
                        type="radio"
                        name="metric"
                        value={m.value}
                        checked={formMetric === m.value}
                        onChange={() => setFormMetric(m.value)}
                      />
                      <div>
                        <strong>{m.label}</strong>
                        <span className="role-desc">{m.description}</span>
                      </div>
                    </label>
                  ))}
                </div>
              </div>

              <div className="form-group">
                <label>Limit Value <span className="required">*</span></label>
                <input
                  type="number"
                  value={formLimitValue}
                  onChange={e => setFormLimitValue(e.target.value)}
                  placeholder={formMetric === 'rpm' ? 'e.g. 100' : 'e.g. 1000000'}
                  min="1"
                  autoFocus
                />
              </div>

              <div className="form-group">
                <label>Window (seconds)</label>
                <input
                  type="number"
                  value={formWindowSeconds}
                  onChange={e => setFormWindowSeconds(e.target.value)}
                  placeholder="60"
                  min="1"
                />
              </div>
            </div>

            {formError && <div className="flash flash-error modal-flash">{formError}</div>}

            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => { setShowModal(false); setFormError(null) }}
                disabled={saving}>
                Cancel
              </button>
              <button className="btn btn-primary" onClick={handleSave}
                disabled={!formLimitValue || saving}>
                {saving ? 'Saving...' : 'Save'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
