import { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { hasPermission, type UserRole } from '../hooks/useUserSync'
import { useTenant } from '../contexts/TenantContext'
import { apiFetch } from '../lib/api'
import Navbar from '../components/Navbar'
import './SmartRoutingPage.css'
import '../pages/ManagementPage.css'

// ─── Types ──────────────────────────────────────────────────────────────────

interface Deployment {
  id?: number
  provider: string
  model: string
  provider_key_id: number | null
  priority: number
  weight: number
  cost_per_1k_input: number
  cost_per_1k_output: number
  enabled: boolean
}

interface ModelGroup {
  id: number
  name: string
  strategy: string
  description: string
  enabled: boolean
  deployments: Deployment[]
  created_at: string
  updated_at: string
}

interface ProviderKey {
  id: number
  provider: string
  label: string
  is_active: boolean
}

interface DeploymentHealth {
  deployment_id: string
  provider: string
  model: string
  healthy: boolean
  avg_latency_ms: number
}

// ─── Constants ──────────────────────────────────────────────────────────────

const STRATEGIES: { value: string; label: string; description: string }[] = [
  { value: 'fallback', label: 'Fallback', description: 'Try deployments in priority order; fail over to the next on error.' },
  { value: 'round-robin', label: 'Round Robin', description: 'Distribute requests evenly across all deployments by weight.' },
  { value: 'lowest-latency', label: 'Lowest Latency', description: 'Route to the deployment with the best average response time.' },
  { value: 'cost-optimized', label: 'Cost Optimized', description: 'Route to the cheapest deployment that is healthy.' },
]

const PROVIDERS = ['anthropic', 'openai', 'deepseek', 'mistral']

function emptyDeployment(): Deployment {
  return { provider: 'anthropic', model: '', provider_key_id: null, priority: 0, weight: 1, cost_per_1k_input: 0, cost_per_1k_output: 0, enabled: true }
}

// ─── Component ──────────────────────────────────────────────────────────────

export default function SmartRoutingPage() {
  const navigate = useNavigate()
  const { orgRole, isSynced } = useTenant()
  const role = (orgRole as UserRole) ?? null

  // Data
  const [groups, setGroups] = useState<ModelGroup[]>([])
  const [providerKeys, setProviderKeys] = useState<ProviderKey[]>([])
  const [loading, setLoading] = useState(true)
  const [successMsg, setSuccessMsg] = useState<string | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  // Create/Edit modal
  const [showModal, setShowModal] = useState(false)
  const [editingGroup, setEditingGroup] = useState<ModelGroup | null>(null)
  const [formName, setFormName] = useState('')
  const [formStrategy, setFormStrategy] = useState('fallback')
  const [formDescription, setFormDescription] = useState('')
  const [formEnabled, setFormEnabled] = useState(true)
  const [formDeployments, setFormDeployments] = useState<Deployment[]>([emptyDeployment()])
  const [saving, setSaving] = useState(false)
  const [modalError, setModalError] = useState<string | null>(null)

  // Strategy dropdown
  const [strategyOpen, setStrategyOpen] = useState(false)
  const [strategyFocus, setStrategyFocus] = useState(-1)
  const strategyRef = useRef<HTMLDivElement>(null)

  // Health modal
  const [healthGroup, setHealthGroup] = useState<ModelGroup | null>(null)
  const [healthData, setHealthData] = useState<DeploymentHealth[] | null>(null)
  const [healthLoading, setHealthLoading] = useState(false)

  // ─── Flash auto-dismiss ─────────────────────────────────────────────────

  useEffect(() => {
    if (!successMsg) return
    const t = setTimeout(() => setSuccessMsg(null), 4000)
    return () => clearTimeout(t)
  }, [successMsg])

  useEffect(() => {
    if (!errorMsg) return
    const t = setTimeout(() => setErrorMsg(null), 6000)
    return () => clearTimeout(t)
  }, [errorMsg])

  // ─── Permission guard ───────────────────────────────────────────────────

  useEffect(() => {
    if (isSynced && !hasPermission(role, 'editor')) {
      navigate('/dashboard', { replace: true })
    }
  }, [isSynced, role, navigate])

  // ─── Data loading ───────────────────────────────────────────────────────

  const loadGroups = useCallback(async () => {
    try {
      const res = await apiFetch('/v1/admin/model-groups')
      if (!res.ok) throw new Error('Failed to load model groups')
      const data = await res.json()
      setGroups(data.data || [])
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : 'Failed to load model groups')
    }
  }, [])

  const loadProviderKeys = useCallback(async () => {
    try {
      const res = await apiFetch('/v1/admin/provider_keys')
      if (!res.ok) return
      const data = await res.json()
      setProviderKeys(data.data || [])
    } catch {
      // non-critical
    }
  }, [])

  useEffect(() => {
    if (!isSynced) return
    Promise.all([loadGroups(), loadProviderKeys()]).finally(() => setLoading(false))
  }, [isSynced, loadGroups, loadProviderKeys])

  // ─── Close strategy dropdown on outside click ──────────────────────────

  useEffect(() => {
    if (!strategyOpen) return
    function handleClick(e: MouseEvent) {
      if (strategyRef.current && !strategyRef.current.contains(e.target as Node)) {
        setStrategyOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [strategyOpen])

  // ─── CRUD helpers ───────────────────────────────────────────────────────

  const openCreate = () => {
    setEditingGroup(null)
    setFormName('')
    setFormStrategy('fallback')
    setFormDescription('')
    setFormEnabled(true)
    setFormDeployments([emptyDeployment()])
    setModalError(null)
    setShowModal(true)
  }

  const openEdit = (g: ModelGroup) => {
    setEditingGroup(g)
    setFormName(g.name)
    setFormStrategy(g.strategy)
    setFormDescription(g.description)
    setFormEnabled(g.enabled)
    setFormDeployments(
      g.deployments.length > 0
        ? g.deployments.map(d => ({ ...d }))
        : [emptyDeployment()]
    )
    setModalError(null)
    setShowModal(true)
  }

  const handleSave = async () => {
    if (!formName.trim()) { setModalError('Name is required'); return }
    const validDeps = formDeployments.filter(d => d.model.trim())
    if (validDeps.length === 0) { setModalError('At least one deployment with a model is required'); return }

    setSaving(true)
    setModalError(null)
    try {
      const body = {
        name: formName.trim(),
        strategy: formStrategy,
        description: formDescription.trim(),
        enabled: formEnabled,
        deployments: validDeps.map(d => ({
          provider: d.provider,
          model: d.model.trim(),
          provider_key_id: d.provider_key_id || undefined,
          priority: d.priority,
          weight: d.weight,
          cost_per_1k_input: d.cost_per_1k_input,
          cost_per_1k_output: d.cost_per_1k_output,
          enabled: d.enabled,
        })),
      }

      const url = editingGroup
        ? `/v1/admin/model-groups/${editingGroup.id}`
        : '/v1/admin/model-groups'
      const method = editingGroup ? 'PUT' : 'POST'

      const res = await apiFetch(url, { method, body: JSON.stringify(body) })
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Request failed' }))
        throw new Error(err.error || 'Request failed')
      }

      setShowModal(false)
      setSuccessMsg(editingGroup ? 'Model group updated' : 'Model group created')
      await loadGroups()
    } catch (err) {
      setModalError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (g: ModelGroup) => {
    if (!window.confirm(`Delete model group "${g.name}"? This cannot be undone.`)) return
    try {
      const res = await apiFetch(`/v1/admin/model-groups/${g.id}`, { method: 'DELETE' })
      if (!res.ok) throw new Error('Failed to delete')
      setSuccessMsg('Model group deleted')
      await loadGroups()
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : 'Failed to delete')
    }
  }

  const handleHealthCheck = async (g: ModelGroup) => {
    setHealthGroup(g)
    setHealthData(null)
    setHealthLoading(true)
    try {
      const res = await apiFetch(`/v1/admin/model-groups/${g.id}/health`)
      if (!res.ok) throw new Error('Failed to fetch health')
      const data = await res.json()
      setHealthData(data.deployments || [])
    } catch {
      setHealthData([])
    } finally {
      setHealthLoading(false)
    }
  }

  // ─── Deployment list helpers ────────────────────────────────────────────

  const updateDeployment = (idx: number, patch: Partial<Deployment>) => {
    setFormDeployments(prev => prev.map((d, i) => i === idx ? { ...d, ...patch } : d))
  }

  const removeDeployment = (idx: number) => {
    setFormDeployments(prev => prev.length > 1 ? prev.filter((_, i) => i !== idx) : prev)
  }

  const addDeployment = () => {
    setFormDeployments(prev => [...prev, emptyDeployment()])
  }

  // ─── Strategy dropdown keyboard nav ─────────────────────────────────────

  const handleStrategyKeyDown = (e: React.KeyboardEvent) => {
    if (!strategyOpen) {
      if (e.key === 'Enter' || e.key === ' ' || e.key === 'ArrowDown') {
        e.preventDefault()
        setStrategyOpen(true)
        setStrategyFocus(STRATEGIES.findIndex(s => s.value === formStrategy))
      }
      return
    }
    if (e.key === 'Escape') { setStrategyOpen(false); return }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setStrategyFocus(f => Math.min(f + 1, STRATEGIES.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setStrategyFocus(f => Math.max(f - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (strategyFocus >= 0 && strategyFocus < STRATEGIES.length) {
        setFormStrategy(STRATEGIES[strategyFocus].value)
        setStrategyOpen(false)
      }
    }
  }

  // ─── Render ─────────────────────────────────────────────────────────────

  if (!isSynced) return <div className="loading-center"><div className="spinner" /></div>

  const strategyLabel = STRATEGIES.find(s => s.value === formStrategy)?.label ?? formStrategy

  return (
    <div className="page-container">
      <Navbar />
      <div className="page-content">
        <div className="mgmt-container">

          {successMsg && <div className="flash flash-success">{successMsg}</div>}
          {errorMsg && <div className="flash flash-error">{errorMsg}</div>}

          {/* ── Model Groups section ──────────────────────────────────── */}
          <div className="mgmt-section">
            <div className="section-hdr">
              <div>
                <h2>
                  Model Groups
                  <span className="section-count">({groups.length})</span>
                </h2>
                <p className="section-desc">
                  Route requests across multiple providers with smart routing strategies.
                </p>
              </div>
              <button className="btn btn-primary btn-small" onClick={openCreate}>
                Create Model Group
              </button>
            </div>

            {loading ? (
              <div className="loading-center"><div className="spinner" /></div>
            ) : groups.length === 0 ? (
              <div className="empty-cta">
                <p className="text-muted">No model groups yet. Create one to enable smart routing across providers.</p>
                <button className="btn btn-primary btn-small" onClick={openCreate}>Create Model Group</button>
              </div>
            ) : (
              <div className="table-scroll">
                <table className="mgmt-table">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Strategy</th>
                      <th>Deployments</th>
                      <th>Status</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {groups.map(g => (
                      <tr key={g.id}>
                        <td>
                          <strong>{g.name}</strong>
                          {g.description && <div className="text-muted" style={{ fontSize: '0.75rem' }}>{g.description}</div>}
                        </td>
                        <td>
                          <span className={`role-badge strategy-${g.strategy}`}>{g.strategy}</span>
                        </td>
                        <td>{g.deployments?.length ?? 0} deployment{(g.deployments?.length ?? 0) !== 1 ? 's' : ''}</td>
                        <td>
                          <span className={`status-badge ${g.enabled ? 'status-active' : 'status-suspended'}`}>
                            {g.enabled ? 'Enabled' : 'Disabled'}
                          </span>
                        </td>
                        <td>
                          <div className="actions-cell">
                            <button className="btn btn-secondary btn-small" onClick={() => openEdit(g)}>Edit</button>
                            <button className="btn btn-secondary btn-small" onClick={() => handleHealthCheck(g)}>Health</button>
                            <button className="btn btn-secondary btn-small btn-danger-text" onClick={() => handleDelete(g)}>Delete</button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* ── Create / Edit Modal ──────────────────────────────────────────── */}
      {showModal && (
        <div className="modal-overlay" onClick={() => setShowModal(false)}>
          <div className="modal-box modal-lg" onClick={e => e.stopPropagation()}>
            <div className="modal-hdr">
              <h2>{editingGroup ? 'Edit Model Group' : 'Create Model Group'}</h2>
            </div>
            <div className="modal-body">
              {modalError && <div className="flash flash-error" style={{ marginBottom: '1rem' }}>{modalError}</div>}

              <div className="form-group">
                <label>Name <span className="required">*</span></label>
                <input type="text" value={formName} onChange={e => setFormName(e.target.value)} placeholder="e.g. claude-ha" />
              </div>

              <div className="form-group">
                <label>Strategy <span className="required">*</span></label>
                <div className={`rich-dropdown${strategyOpen ? ' open' : ''}`} ref={strategyRef}>
                  <button
                    type="button"
                    className="rich-dropdown-trigger"
                    onClick={() => setStrategyOpen(v => !v)}
                    onKeyDown={handleStrategyKeyDown}
                  >
                    <span className="trigger-label">{strategyLabel}</span>
                    <span className="trigger-chevron">&#9660;</span>
                  </button>
                  {strategyOpen && (
                    <div className="rich-dropdown-panel">
                      {STRATEGIES.map((s, i) => (
                        <div
                          key={s.value}
                          className={`rich-dropdown-option${s.value === formStrategy ? ' selected' : ''}${i === strategyFocus ? ' focused' : ''}`}
                          onClick={() => { setFormStrategy(s.value); setStrategyOpen(false) }}
                        >
                          <span className="option-title">{s.label}</span>
                          <span className="option-desc">{s.description}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </div>

              <div className="form-group">
                <label>Description <span className="optional">(optional)</span></label>
                <input type="text" value={formDescription} onChange={e => setFormDescription(e.target.value)} placeholder="Short description" />
              </div>

              <div className="form-group">
                <label className="sr-checkbox-label">
                  <input type="checkbox" checked={formEnabled} onChange={e => setFormEnabled(e.target.checked)} />
                  Enabled
                </label>
              </div>

              {/* ── Deployments ──────────────────────────────────────────── */}
              <div className="sr-deployments-section">
                <div className="sr-deployments-header">
                  <label>Deployments <span className="required">*</span></label>
                  <button type="button" className="btn btn-secondary btn-small" onClick={addDeployment}>+ Add</button>
                </div>

                {formDeployments.map((dep, idx) => (
                  <div key={idx} className="sr-deployment-card">
                    <div className="sr-deployment-card-header">
                      <span className="sr-deployment-num">#{idx + 1}</span>
                      {formDeployments.length > 1 && (
                        <button type="button" className="btn btn-secondary btn-small btn-danger-text" onClick={() => removeDeployment(idx)}>Remove</button>
                      )}
                    </div>

                    <div className="sr-deployment-grid">
                      <div className="form-group">
                        <label>Provider</label>
                        <select value={dep.provider} onChange={e => updateDeployment(idx, { provider: e.target.value })}>
                          {PROVIDERS.map(p => <option key={p} value={p}>{p}</option>)}
                        </select>
                      </div>

                      <div className="form-group">
                        <label>Model</label>
                        <input type="text" value={dep.model} onChange={e => updateDeployment(idx, { model: e.target.value })} placeholder="e.g. claude-sonnet-4-20250514" />
                      </div>

                      <div className="form-group">
                        <label>Provider Key</label>
                        <select
                          value={dep.provider_key_id ?? ''}
                          onChange={e => updateDeployment(idx, { provider_key_id: e.target.value ? Number(e.target.value) : null })}
                        >
                          <option value="">Auto (default)</option>
                          {providerKeys
                            .filter(k => k.provider === dep.provider && k.is_active)
                            .map(k => <option key={k.id} value={k.id}>{k.label}</option>)}
                        </select>
                      </div>

                      <div className="form-group">
                        <label>Priority</label>
                        <input type="number" value={dep.priority} onChange={e => updateDeployment(idx, { priority: Number(e.target.value) })} />
                      </div>

                      <div className="form-group">
                        <label>Weight</label>
                        <input type="number" value={dep.weight} min={1} onChange={e => updateDeployment(idx, { weight: Number(e.target.value) || 1 })} />
                      </div>

                      <div className="form-group">
                        <label>Cost/1K input</label>
                        <input type="number" value={dep.cost_per_1k_input} step="0.001" min={0} onChange={e => updateDeployment(idx, { cost_per_1k_input: Number(e.target.value) })} />
                      </div>

                      <div className="form-group">
                        <label>Cost/1K output</label>
                        <input type="number" value={dep.cost_per_1k_output} step="0.001" min={0} onChange={e => updateDeployment(idx, { cost_per_1k_output: Number(e.target.value) })} />
                      </div>

                      <div className="form-group">
                        <label className="sr-checkbox-label">
                          <input type="checkbox" checked={dep.enabled} onChange={e => updateDeployment(idx, { enabled: e.target.checked })} />
                          Enabled
                        </label>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => setShowModal(false)} disabled={saving}>Cancel</button>
              <button className="btn btn-primary" onClick={handleSave} disabled={saving}>
                {saving ? 'Saving...' : editingGroup ? 'Save Changes' : 'Create'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── Health Modal ─────────────────────────────────────────────────── */}
      {healthGroup && (
        <div className="modal-overlay" onClick={() => setHealthGroup(null)}>
          <div className="modal-box modal-md" onClick={e => e.stopPropagation()}>
            <div className="modal-hdr">
              <h2>Health — {healthGroup.name}</h2>
            </div>
            <div className="modal-body">
              {healthLoading ? (
                <div className="loading-center"><div className="spinner" /></div>
              ) : !healthData || healthData.length === 0 ? (
                <p className="text-muted">No deployment health data available.</p>
              ) : (
                <div className="table-scroll">
                  <table className="mgmt-table">
                    <thead>
                      <tr>
                        <th>Provider</th>
                        <th>Model</th>
                        <th>Healthy</th>
                        <th>Avg Latency</th>
                      </tr>
                    </thead>
                    <tbody>
                      {healthData.map((h, i) => (
                        <tr key={i}>
                          <td>{h.provider}</td>
                          <td>{h.model}</td>
                          <td>
                            <span className={`status-badge ${h.healthy ? 'status-active' : 'status-suspended'}`}>
                              {h.healthy ? 'Healthy' : 'Unhealthy'}
                            </span>
                          </td>
                          <td>{h.avg_latency_ms > 0 ? `${h.avg_latency_ms}ms` : '—'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => setHealthGroup(null)}>Close</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
