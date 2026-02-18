import { useState, useMemo } from 'react'
import { usePricingConfig, CatalogEntry, PricingConfigView } from '../hooks/usePricingConfig'
import './PricingConfigPage.css'

const PRICE_TYPES = ['input', 'output', 'cache_creation', 'cache_read', 'reasoning'] as const
type PriceType = typeof PRICE_TYPES[number]

const PRICE_TYPE_LABELS: Record<PriceType, string> = {
  input: 'Input',
  output: 'Output',
  cache_creation: 'Cache Write',
  cache_read: 'Cache Read',
  reasoning: 'Reasoning',
}

function fmt(val: string | undefined) {
  if (!val) return '—'
  const n = parseFloat(val)
  if (isNaN(n)) return '—'
  return `$${n.toFixed(4)}`
}

function shortKey(keyId: string) {
  return keyId.slice(0, 8) + '…'
}

// ── Provider badge colour ─────────────────────────────────────────────────────
const PROVIDER_COLORS: Record<string, string> = {
  anthropic: '#c96442',
  openai: '#10a37f',
  google: '#4285f4',
  azure: '#0078d4',
  mistral: '#ff7000',
}

function ProviderBadge({ name }: { name: string }) {
  const color = PROVIDER_COLORS[name.toLowerCase()] || '#6366f1'
  return (
    <span className="provider-badge" style={{ background: color + '22', color, borderColor: color + '55' }}>
      {name}
    </span>
  )
}

// ── Group catalog by provider ─────────────────────────────────────────────────
function groupByProvider(catalog: CatalogEntry[]) {
  const map: Record<string, CatalogEntry[]> = {}
  for (const entry of catalog) {
    const key = entry.provider_display || entry.provider
    if (!map[key]) map[key] = []
    map[key].push(entry)
  }
  return map
}

// ═══════════════════════════════════════════════════════════════════════════════
export default function PricingConfigPage() {
  const {
    catalog, configs, activeKeys,
    loading, error,
    createConfig, deleteConfig,
    addRate, deleteRate,
    assignKey, unassignKey,
  } = usePricingConfig()

  const [flash, setFlash] = useState<{ type: 'success' | 'error'; msg: string } | null>(null)
  const showFlash = (type: 'success' | 'error', msg: string) => {
    setFlash({ type, msg })
    setTimeout(() => setFlash(null), 4000)
  }

  // ── Expanded config ───────────────────────────────────────────────────────
  const [expandedId, setExpandedId] = useState<number | null>(null)

  // ── Create config modal ───────────────────────────────────────────────────
  const [showCreate, setShowCreate] = useState(false)
  const [createName, setCreateName] = useState('')
  const [createDesc, setCreateDesc] = useState('')
  const [createLoading, setCreateLoading] = useState(false)

  async function handleCreate() {
    if (!createName.trim()) return
    setCreateLoading(true)
    try {
      const cfg = await createConfig(createName.trim(), createDesc.trim())
      setShowCreate(false)
      setCreateName('')
      setCreateDesc('')
      setExpandedId(cfg.id)
      showFlash('success', `Config "${cfg.name}" created.`)
    } catch (e: unknown) {
      showFlash('error', e instanceof Error ? e.message : 'Failed to create config')
    } finally {
      setCreateLoading(false)
    }
  }

  // ── Add rate modal ────────────────────────────────────────────────────────
  const [addRateFor, setAddRateFor] = useState<PricingConfigView | null>(null)
  const [rateModelId, setRateModelId] = useState<number>(0)
  const [ratePriceType, setRatePriceType] = useState<PriceType>('input')
  const [ratePrice, setRatePrice] = useState('')
  const [rateLoading, setRateLoading] = useState(false)

  const selectedModelEntry = useMemo(
    () => catalog.find(e => e.model_id === rateModelId),
    [catalog, rateModelId]
  )
  const defaultPriceHint = selectedModelEntry?.prices[ratePriceType]?.price_per_unit

  function openAddRate(cfg: PricingConfigView) {
    setAddRateFor(cfg)
    setRateModelId(catalog[0]?.model_id ?? 0)
    setRatePriceType('input')
    setRatePrice('')
  }

  async function handleAddRate() {
    if (!addRateFor || !rateModelId || !ratePrice.trim()) return
    setRateLoading(true)
    try {
      await addRate(addRateFor.id, rateModelId, ratePriceType, ratePrice.trim())
      showFlash('success', 'Rate override saved.')
      setAddRateFor(null)
    } catch (e: unknown) {
      showFlash('error', e instanceof Error ? e.message : 'Failed to add rate')
    } finally {
      setRateLoading(false)
    }
  }

  // ── Assign key modal ──────────────────────────────────────────────────────
  const [assignFor, setAssignFor] = useState<PricingConfigView | null>(null)
  const [assignKeyId, setAssignKeyId] = useState('')
  const [assignLoading, setAssignLoading] = useState(false)

  function openAssign(cfg: PricingConfigView) {
    setAssignFor(cfg)
    setAssignKeyId(activeKeys[0]?.key_id ?? '')
  }

  async function handleAssign() {
    if (!assignFor || !assignKeyId) return
    setAssignLoading(true)
    try {
      const key = activeKeys.find(k => k.key_id === assignKeyId)
      await assignKey(assignFor.id, assignKeyId, key?.label ?? assignKeyId)
      showFlash('success', 'Config assigned to API key.')
      setAssignFor(null)
    } catch (e: unknown) {
      showFlash('error', e instanceof Error ? e.message : 'Failed to assign')
    } finally {
      setAssignLoading(false)
    }
  }

  // ── Delete confirm ────────────────────────────────────────────────────────
  const [deleteFor, setDeleteFor] = useState<PricingConfigView | null>(null)
  const [deleteLoading, setDeleteLoading] = useState(false)

  async function handleDelete() {
    if (!deleteFor) return
    setDeleteLoading(true)
    try {
      await deleteConfig(deleteFor.id)
      if (expandedId === deleteFor.id) setExpandedId(null)
      showFlash('success', `Config "${deleteFor.name}" deleted.`)
      setDeleteFor(null)
    } catch (e: unknown) {
      showFlash('error', e instanceof Error ? e.message : 'Failed to delete config')
    } finally {
      setDeleteLoading(false)
    }
  }

  // ── Grouped catalog ───────────────────────────────────────────────────────
  const grouped = useMemo(() => groupByProvider(catalog), [catalog])

  // ── Render ────────────────────────────────────────────────────────────────
  return (
    <div className="page-container">
      <div className="pc-page">

        {/* Header */}
        <div className="pc-header">
          <div>
            <h1 className="pc-title">Pricing Config</h1>
            <p className="pc-subtitle">
              View default model pricing and create custom overrides per API key.
            </p>
          </div>
          <button className="btn btn-primary" onClick={() => setShowCreate(true)}>
            + New Config
          </button>
        </div>

        {/* Flash */}
        {flash && (
          <div className={`flash flash-${flash.type}`}>{flash.msg}</div>
        )}

        {loading && <div className="loading-spinner" />}
        {error && <div className="flash flash-error">{error}</div>}

        {!loading && !error && (
          <>
            {/* ── Section 1: Default Pricing Catalog ─────────────────────── */}
            <section className="pc-section">
              <div className="pc-section-header">
                <h2>Default Model Pricing</h2>
                <span className="pc-section-note">per 1M tokens · read-only</span>
              </div>

              {Object.entries(grouped).map(([providerDisplay, entries]) => (
                <div key={providerDisplay} className="pc-provider-block">
                  <div className="pc-provider-label">
                    <ProviderBadge name={entries[0].provider} />
                    <span>{providerDisplay}</span>
                  </div>
                  <div className="pc-table-wrap">
                    <table className="pc-table">
                      <thead>
                        <tr>
                          <th>Model</th>
                          {PRICE_TYPES.map(pt => (
                            <th key={pt}>{PRICE_TYPE_LABELS[pt]}</th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {entries.map(entry => (
                          <tr key={entry.model_id}>
                            <td><code className="model-name">{entry.model_name}</code></td>
                            {PRICE_TYPES.map(pt => (
                              <td key={pt} className={entry.prices[pt] ? '' : 'pc-na'}>
                                {entry.prices[pt] ? fmt(entry.prices[pt].price_per_unit) : '—'}
                              </td>
                            ))}
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              ))}
            </section>

            {/* ── Section 2: Pricing Configs ────────────────────────────── */}
            <section className="pc-section">
              <div className="pc-section-header">
                <h2>My Pricing Configs</h2>
                <span className="pc-section-note">
                  {configs.length} config{configs.length !== 1 ? 's' : ''}
                </span>
              </div>

              {configs.length === 0 && (
                <div className="pc-empty">
                  No configs yet. Create one to override default pricing for a specific API key.
                </div>
              )}

              <div className="pc-configs-list">
                {configs.map(cfg => (
                  <div key={cfg.id} className="pc-config-card">

                    {/* Config header row */}
                    <div
                      className="pc-config-header"
                      onClick={() => setExpandedId(expandedId === cfg.id ? null : cfg.id)}
                    >
                      <div className="pc-config-title-group">
                        <span className="pc-chevron">{expandedId === cfg.id ? '▾' : '▸'}</span>
                        <strong className="pc-config-name">{cfg.name}</strong>
                        {cfg.description && (
                          <span className="pc-config-desc">{cfg.description}</span>
                        )}
                      </div>

                      <div className="pc-config-meta" onClick={e => e.stopPropagation()}>
                        {cfg.assigned_key ? (
                          <span className="pc-key-badge">
                            🔑 {cfg.assigned_key.label}
                            <code>{shortKey(cfg.assigned_key.key_id)}</code>
                            <button
                              className="pc-unassign"
                              title="Remove assignment"
                              onClick={async () => {
                                try {
                                  await unassignKey(cfg.id)
                                  showFlash('success', 'Assignment removed.')
                                } catch (e: unknown) {
                                  showFlash('error', e instanceof Error ? e.message : 'Failed')
                                }
                              }}
                            >×</button>
                          </span>
                        ) : (
                          <span className="pc-no-key">No API key assigned</span>
                        )}
                        <div className="pc-config-actions">
                          <button
                            className="btn btn-secondary pc-btn-sm"
                            onClick={() => openAddRate(cfg)}
                          >+ Override</button>
                          <button
                            className="btn btn-secondary pc-btn-sm"
                            onClick={() => openAssign(cfg)}
                            disabled={activeKeys.length === 0}
                            title={activeKeys.length === 0 ? 'No active API keys' : 'Assign to API key'}
                          >Assign Key</button>
                          <button
                            className="btn btn-danger pc-btn-sm"
                            onClick={() => setDeleteFor(cfg)}
                          >Delete</button>
                        </div>
                      </div>
                    </div>

                    {/* Expanded rates table */}
                    {expandedId === cfg.id && (
                      <div className="pc-rates-body">
                        {cfg.rates.length === 0 ? (
                          <p className="pc-rates-empty">
                            No overrides yet. Click <strong>+ Override</strong> to add a price override for a specific model.
                          </p>
                        ) : (
                          <table className="pc-table pc-rates-table">
                            <thead>
                              <tr>
                                <th>Provider</th>
                                <th>Model</th>
                                <th>Price Type</th>
                                <th>Override (per 1M)</th>
                                <th></th>
                              </tr>
                            </thead>
                            <tbody>
                              {cfg.rates.map(rate => (
                                <tr key={rate.id}>
                                  <td><ProviderBadge name={rate.provider} /></td>
                                  <td><code className="model-name">{rate.model_name}</code></td>
                                  <td>
                                    <span className="pc-ptype-badge">
                                      {PRICE_TYPE_LABELS[rate.price_type as PriceType] || rate.price_type}
                                    </span>
                                  </td>
                                  <td className="pc-price-cell">{fmt(rate.price_per_unit)}</td>
                                  <td>
                                    <button
                                      className="pc-delete-rate"
                                      title="Remove override"
                                      onClick={async () => {
                                        try {
                                          await deleteRate(cfg.id, rate.id)
                                          showFlash('success', 'Override removed.')
                                        } catch (e: unknown) {
                                          showFlash('error', e instanceof Error ? e.message : 'Failed')
                                        }
                                      }}
                                    >×</button>
                                  </td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        )}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </section>
          </>
        )}
      </div>

      {/* ── Modal: Create Config ──────────────────────────────────────────── */}
      {showCreate && (
        <div className="modal-overlay" onClick={() => setShowCreate(false)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h3>New Pricing Config</h3>
            </div>
            <div className="modal-body">
              <div className="form-group">
                <label>Name <span className="required">*</span></label>
                <input
                  className="form-input"
                  placeholder="e.g. Enterprise Plan"
                  value={createName}
                  onChange={e => setCreateName(e.target.value)}
                  autoFocus
                />
              </div>
              <div className="form-group">
                <label>Description</label>
                <input
                  className="form-input"
                  placeholder="Optional description"
                  value={createDesc}
                  onChange={e => setCreateDesc(e.target.value)}
                />
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn btn-secondary" onClick={() => setShowCreate(false)}>Cancel</button>
              <button
                className="btn btn-primary"
                onClick={handleCreate}
                disabled={!createName.trim() || createLoading}
              >
                {createLoading ? 'Creating…' : 'Create Config'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── Modal: Add Rate Override ──────────────────────────────────────── */}
      {addRateFor && (
        <div className="modal-overlay" onClick={() => setAddRateFor(null)}>
          <div className="modal modal-wide" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Add Price Override</h3>
              <p className="modal-subtitle">Config: <strong>{addRateFor.name}</strong></p>
            </div>
            <div className="modal-body">
              <div className="form-group">
                <label>Model <span className="required">*</span></label>
                <select
                  className="form-input"
                  value={rateModelId}
                  onChange={e => {
                    setRateModelId(Number(e.target.value))
                    setRatePrice('')
                  }}
                >
                  {catalog.map(entry => (
                    <option key={entry.model_id} value={entry.model_id}>
                      {entry.provider_display} / {entry.model_name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="form-group">
                <label>Price Type <span className="required">*</span></label>
                <div className="pc-ptype-radios">
                  {PRICE_TYPES.map(pt => {
                    const hasDefault = !!selectedModelEntry?.prices[pt]
                    return (
                      <label
                        key={pt}
                        className={`pc-ptype-radio ${ratePriceType === pt ? 'selected' : ''} ${!hasDefault ? 'unavailable' : ''}`}
                      >
                        <input
                          type="radio"
                          name="price_type"
                          value={pt}
                          checked={ratePriceType === pt}
                          onChange={() => { setRatePriceType(pt); setRatePrice('') }}
                        />
                        <span>{PRICE_TYPE_LABELS[pt]}</span>
                        {hasDefault && (
                          <span className="pc-default-hint">
                            default: {fmt(selectedModelEntry!.prices[pt].price_per_unit)}
                          </span>
                        )}
                      </label>
                    )
                  })}
                </div>
              </div>
              <div className="form-group">
                <label>
                  Override Price per 1M tokens <span className="required">*</span>
                  {defaultPriceHint && (
                    <span className="form-hint">
                      default: {fmt(defaultPriceHint)}
                      <button
                        className="pc-use-default"
                        type="button"
                        onClick={() => setRatePrice(defaultPriceHint)}
                      >use</button>
                    </span>
                  )}
                </label>
                <input
                  className="form-input"
                  type="number"
                  min="0"
                  step="0.001"
                  placeholder="e.g. 2.50"
                  value={ratePrice}
                  onChange={e => setRatePrice(e.target.value)}
                />
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn btn-secondary" onClick={() => setAddRateFor(null)}>Cancel</button>
              <button
                className="btn btn-primary"
                onClick={handleAddRate}
                disabled={!rateModelId || !ratePrice.trim() || rateLoading}
              >
                {rateLoading ? 'Saving…' : 'Save Override'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── Modal: Assign Key ─────────────────────────────────────────────── */}
      {assignFor && (
        <div className="modal-overlay" onClick={() => setAssignFor(null)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Assign to API Key</h3>
              <p className="modal-subtitle">Config: <strong>{assignFor.name}</strong></p>
            </div>
            <div className="modal-body">
              {activeKeys.length === 0 ? (
                <p className="pc-empty">No active API keys. Create one first.</p>
              ) : (
                <div className="form-group">
                  <label>API Key <span className="required">*</span></label>
                  <select
                    className="form-input"
                    value={assignKeyId}
                    onChange={e => setAssignKeyId(e.target.value)}
                  >
                    {activeKeys.map(k => (
                      <option key={k.key_id} value={k.key_id}>
                        {k.label} ({shortKey(k.key_id)})
                      </option>
                    ))}
                  </select>
                  <p className="form-hint" style={{ marginTop: 8 }}>
                    If this key already has a config assigned, it will be replaced.
                  </p>
                </div>
              )}
            </div>
            <div className="modal-footer">
              <button className="btn btn-secondary" onClick={() => setAssignFor(null)}>Cancel</button>
              <button
                className="btn btn-primary"
                onClick={handleAssign}
                disabled={!assignKeyId || assignLoading || activeKeys.length === 0}
              >
                {assignLoading ? 'Assigning…' : 'Assign'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── Modal: Delete Confirm ─────────────────────────────────────────── */}
      {deleteFor && (
        <div className="modal-overlay" onClick={() => setDeleteFor(null)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Delete Config</h3>
            </div>
            <div className="modal-body">
              <div className="warning-box">
                <p>
                  Delete <strong>"{deleteFor.name}"</strong>? This will remove all rate overrides and
                  unassign it from any API key. This cannot be undone.
                </p>
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn btn-secondary" onClick={() => setDeleteFor(null)}>Cancel</button>
              <button
                className="btn btn-danger"
                onClick={handleDelete}
                disabled={deleteLoading}
              >
                {deleteLoading ? 'Deleting…' : 'Delete Config'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
