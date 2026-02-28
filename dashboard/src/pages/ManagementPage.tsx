import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { useUserSync, hasPermission } from '../hooks/useUserSync'
import Navbar from '../components/Navbar'
import './ManagementPage.css'

const API_SERVER_URL = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

interface APIKey {
  key_id: string
  label: string
  provider: string
  auth_method: string
  billing_mode: string
  scopes: string[] | null
  expires_at: string | null
  created_at: string
  last_seen_at: string | null
}

const AUTH_METHODS: { value: string; label: string; description: string }[] = [
  {
    value: 'BROWSER_OAUTH',
    label: 'Browser OAuth',
    description: 'CLI tool authenticates via browser login. Forwards the client\'s own credentials.',
  },
  {
    value: 'BYOK',
    label: 'Bring Your Own Key',
    description: 'Gateway injects your stored provider key. User never handles raw credentials.',
  },
]

const BILLING_MODES: { value: string; label: string; description: string }[] = [
  {
    value: 'MONTHLY_SUBSCRIPTION',
    label: 'Monthly Subscription',
    description: 'Usage billed through the user\'s existing provider subscription.',
  },
  {
    value: 'API_USAGE',
    label: 'API Usage',
    description: 'Usage billed per token through the stored or client-provided API key.',
  },
]

interface ProviderKey {
  id: number
  provider: string
  label: string
  is_active: boolean
  created_at: string
}

export default function ManagementPage() {
  const navigate = useNavigate()
  const { role, userId, isSynced } = useUserSync()

  const [apiKeys, setApiKeys] = useState<APIKey[]>([])
  const [providerKeys, setProviderKeys] = useState<ProviderKey[]>([])
  const [keyLimit, setKeyLimit] = useState<number | null>(null)
  const [providerKeyLimit, setProviderKeyLimit] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)
  const [successMsg, setSuccessMsg] = useState<string | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  // Add provider key modal
  const [showAddKeyModal, setShowAddKeyModal] = useState(false)
  const [addKeyProvider, setAddKeyProvider] = useState<'anthropic' | 'openai'>('anthropic')
  const [addKeyLabel, setAddKeyLabel] = useState('')
  const [addKeyValue, setAddKeyValue] = useState('')
  const [addingKey, setAddingKey] = useState(false)

  // Create API key modal
  const [showCreateKeyModal, setShowCreateKeyModal] = useState(false)
  const [showNewKeyModal, setShowNewKeyModal] = useState(false)
  const [newKeyLabel, setNewKeyLabel] = useState('')
  const [newKeyProvider, setNewKeyProvider] = useState<string>('anthropic')
  const [newAuthMethod, setNewAuthMethod] = useState<string>('BROWSER_OAUTH')
  const [newBillingMode, setNewBillingMode] = useState<string>('MONTHLY_SUBSCRIPTION')
  const [createdAuthMethod, setCreatedAuthMethod] = useState<string>('')
  const [createdBillingMode, setCreatedBillingMode] = useState<string>('')
  const [createdProvider, setCreatedProvider] = useState<string>('')
  const [newKeySecret, setNewKeySecret] = useState<string | null>(null)
  const [copiedID, setCopiedID] = useState<string | null>(null)
  const [createKeyError, setCreateKeyError] = useState<string | null>(null)

  // Limit-reached modal
  const [limitModal, setLimitModal] = useState<{ type: 'keys' | 'provider_keys' } | null>(null)

  const canAccess = hasPermission(role, 'editor')

  const headers = useCallback(() => ({
    'Content-Type': 'application/json',
    'X-User-ID': userId ?? '',
  }), [userId])

  const showSuccess = (msg: string) => {
    setSuccessMsg(msg)
    setTimeout(() => setSuccessMsg(null), 3000)
  }
  const showError = (msg: string) => {
    setErrorMsg(msg)
    setTimeout(() => setErrorMsg(null), 5000)
  }

  const fetchAPIKeys = useCallback(async () => {
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/api_keys`, { headers: headers() })
      if (res.ok) {
        const data = await res.json()
        setApiKeys(data.api_keys ?? [])
        setKeyLimit(data.limit ?? null)
      }
    } catch (err) {
      console.error('fetch api keys:', err)
    }
  }, [headers])

  const fetchProviderKeys = useCallback(async () => {
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/provider_keys`, { headers: headers() })
      if (res.ok) {
        const data = await res.json()
        setProviderKeys(data.provider_keys ?? [])
        setProviderKeyLimit(data.limit ?? null)
      }
    } catch (err) {
      console.error('fetch provider keys:', err)
    }
  }, [headers])

  useEffect(() => {
    if (!isSynced) return
    if (!canAccess) {
      navigate('/dashboard')
      return
    }
    const load = async () => {
      setLoading(true)
      await Promise.all([fetchAPIKeys(), fetchProviderKeys()])
      setLoading(false)
    }
    load()
  }, [isSynced, canAccess, navigate, fetchAPIKeys, fetchProviderKeys])

  // ── API Key actions ──────────────────────────────────────────────────────

  const handleCreateAPIKey = async () => {
    if (!newKeyLabel.trim()) {
      setCreateKeyError('Please enter a label')
      return
    }
    setCreateKeyError(null)
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/api_keys`, {
        method: 'POST',
        headers: headers(),
        body: JSON.stringify({ label: newKeyLabel.trim(), provider: newKeyProvider, auth_method: newAuthMethod, billing_mode: newBillingMode, scopes: ['*'] }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.message ?? d.error ?? 'Failed to create API key')
      }
      const data = await res.json()
      setCreatedAuthMethod(newAuthMethod)
      setCreatedBillingMode(newBillingMode)
      setCreatedProvider(newKeyProvider)
      setNewKeySecret(`${data.key_id}:${data.secret}`)
      setShowCreateKeyModal(false)
      setCreateKeyError(null)
      setShowNewKeyModal(true)
      setNewKeyLabel('')
      fetchAPIKeys()
    } catch (err) {
      setCreateKeyError(err instanceof Error ? err.message : 'Failed to create API key')
    }
  }

  const handleRevokeAPIKey = async (keyID: string) => {
    if (!confirm('Revoke this API key? Any agents using it will stop working.')) return
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/api_keys/${keyID}`, {
        method: 'DELETE',
        headers: headers(),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error ?? 'Failed to revoke key')
      }
      showSuccess('API key revoked')
      fetchAPIKeys()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to revoke API key')
    }
  }

  // ── Provider Key actions ─────────────────────────────────────────────────

  const handleAddProviderKey = async () => {
    if (!addKeyLabel.trim() || !addKeyValue.trim()) {
      showError('Label and API key are required')
      return
    }
    setAddingKey(true)
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/provider_keys`, {
        method: 'POST',
        headers: headers(),
        body: JSON.stringify({ provider: addKeyProvider, label: addKeyLabel.trim(), api_key: addKeyValue.trim() }),
      })
      const data = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(data.error ?? 'Failed to add provider key')
      showSuccess(`Provider key "${addKeyLabel}" added successfully`)
      setShowAddKeyModal(false)
      setAddKeyLabel('')
      setAddKeyValue('')
      setAddKeyProvider('anthropic')
      fetchProviderKeys()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to add provider key')
    } finally {
      setAddingKey(false)
    }
  }

  const handleActivateKey = async (id: number) => {
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/provider_keys/${id}/activate`, {
        method: 'PUT',
        headers: headers(),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error ?? 'Failed to activate key')
      }
      showSuccess('Provider key activated')
      fetchProviderKeys()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to activate provider key')
    }
  }

  const handleRevokeProviderKey = async (id: number) => {
    if (!confirm('Revoke this provider key? Any proxied requests using it will fail.')) return
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/provider_keys/${id}`, {
        method: 'DELETE',
        headers: headers(),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error ?? 'Failed to revoke key')
      }
      showSuccess('Provider key revoked')
      fetchProviderKeys()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to revoke provider key')
    }
  }

  const copy = async (text: string, id: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopiedID(id)
      setTimeout(() => setCopiedID(null), 2000)
    } catch {
      showError('Failed to copy')
    }
  }

  // ── Loading / redirect ────────────────────────────────────────────────────

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
        <div className="mgmt-container">

          {/* Header */}
          <div className="mgmt-header">
            <h1>Management</h1>
            <span className={`role-badge role-${role}`}>
              {role?.charAt(0).toUpperCase()}{role?.slice(1)}
            </span>
          </div>

          {/* Flash messages */}
          {successMsg && <div className="flash flash-success">{successMsg}</div>}
          {errorMsg   && <div className="flash flash-error">{errorMsg}</div>}

          {/* ── Gateway API Keys ─────────────────────────────────────────── */}
          <section className="mgmt-section">
            <div className="section-hdr">
              <div>
                <h2>
                  Gateway API Keys{' '}
                  <span className="section-count">
                    {apiKeys.length}/{keyLimit === null ? '∞' : keyLimit}
                  </span>
                </h2>
                <p className="section-desc">
                  Keys used by the AI code agent to report usage through the TokenGate gateway.
                </p>
              </div>
              <button className="btn btn-primary" onClick={() => {
                if (keyLimit !== null && apiKeys.length >= keyLimit) {
                  setLimitModal({ type: 'keys' })
                  return
                }
                setNewKeyLabel(''); setCreateKeyError(null); setShowCreateKeyModal(true)
              }}>
                Create Key
              </button>
            </div>
            <div className="table-scroll">
              <table className="mgmt-table">
                <thead>
                  <tr>
                    <th>Key ID</th>
                    <th>Label</th>
                    <th>Provider</th>
                    <th>Auth</th>
                    <th>Billing</th>
                    <th>Created</th>
                    <th>Last Seen</th>
                    <th>Expires</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {apiKeys.length === 0 ? (
                    <tr>
                      <td colSpan={9} className="empty-cell">
                        <div className="empty-cta">
                          <p>No API keys yet. Create one to start reporting usage.</p>
                          <button className="btn btn-primary"
                            onClick={() => { setNewKeyLabel(''); setCreateKeyError(null); setShowCreateKeyModal(true) }}>
                            Create Your First Key
                          </button>
                        </div>
                      </td>
                    </tr>
                  ) : apiKeys.map(k => (
                    <tr key={k.key_id}>
                      <td><code className="key-id">{k.key_id.slice(0, 8)}…</code></td>
                      <td>{k.label}</td>
                      <td><span className="provider-badge">{k.provider}</span></td>
                      <td><span className="mode-badge">{k.auth_method}</span></td>
                      <td><span className="mode-badge">{k.billing_mode}</span></td>
                      <td className="text-muted">{new Date(k.created_at).toLocaleDateString()}</td>
                      <td className="text-muted">{k.last_seen_at ? new Date(k.last_seen_at).toLocaleString() : 'Never'}</td>
                      <td className="text-muted">
                        {k.expires_at ? new Date(k.expires_at).toLocaleDateString() : 'Never'}
                      </td>
                      <td>
                        <button className="btn btn-small btn-danger"
                          onClick={() => handleRevokeAPIKey(k.key_id)}>
                          Revoke
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>

          {/* ── Provider Keys ────────────────────────────────────────────── */}
          <section className="mgmt-section">
            <div className="section-hdr">
              <div>
                <h2>
                  Provider Keys{' '}
                  <span className="section-count">
                    {providerKeys.length}/{providerKeyLimit === null ? '∞' : providerKeyLimit}
                  </span>
                </h2>
                <p className="section-desc">
                  Upstream LLM provider API keys (Anthropic, OpenAI) stored and rotated centrally.
                  Set one as active and agents will use it automatically via the gateway proxy.
                </p>
              </div>
              <button className="btn btn-primary" onClick={() => {
                if (providerKeyLimit !== null && providerKeys.length >= providerKeyLimit) {
                  setLimitModal({ type: 'provider_keys' })
                  return
                }
                setAddKeyLabel(''); setAddKeyValue(''); setAddKeyProvider('anthropic')
                setShowAddKeyModal(true)
              }}>
                Add Key
              </button>
            </div>
            <div className="table-scroll">
              <table className="mgmt-table">
                <thead>
                  <tr>
                    <th>Provider</th>
                    <th>Label</th>
                    <th>Status</th>
                    <th>Created</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {providerKeys.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="empty-cell">
                        <div className="empty-cta">
                          <p>No provider keys yet. Add your Anthropic API key to enable the gateway proxy.</p>
                          <button className="btn btn-primary" onClick={() => {
                            setAddKeyLabel(''); setAddKeyValue(''); setAddKeyProvider('anthropic')
                            setShowAddKeyModal(true)
                          }}>
                            Add Your First Key
                          </button>
                        </div>
                      </td>
                    </tr>
                  ) : providerKeys.map(k => (
                    <tr key={k.id}>
                      <td><span className="provider-badge">{k.provider}</span></td>
                      <td>{k.label}</td>
                      <td>
                        <span className={`status-badge ${k.is_active ? 'status-active' : 'status-suspended'}`}>
                          {k.is_active ? 'Active' : 'Inactive'}
                        </span>
                      </td>
                      <td className="text-muted">{new Date(k.created_at).toLocaleDateString()}</td>
                      <td>
                        {!k.is_active && (
                          <button className="btn btn-small btn-secondary"
                            onClick={() => handleActivateKey(k.id)}>
                            Activate
                          </button>
                        )}
                        <button className="btn btn-small btn-danger"
                          onClick={() => handleRevokeProviderKey(k.id)}>
                          Revoke
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>

        </div>
      </div>

      {/* ── Create API Key Modal ─────────────────────────────────────────── */}
      {showCreateKeyModal && (
        <div className="modal-overlay" onClick={() => { setShowCreateKeyModal(false); setCreateKeyError(null) }}>
          <div className="modal-box modal-md" onClick={e => e.stopPropagation()}>
            <div className="modal-hdr">
              <h2>Create API Key</h2>
            </div>
            <div className="modal-body">
              <p className="modal-hint">
                Give this key a descriptive label so you can identify which agent or environment
                it belongs to (e.g. "laptop-dev", "ci-pipeline").
              </p>
              <div className="form-group">
                <label>Label</label>
                <input
                  type="text"
                  value={newKeyLabel}
                  onChange={e => setNewKeyLabel(e.target.value)}
                  placeholder="e.g. laptop-dev"
                  autoFocus
                  onKeyDown={e => e.key === 'Enter' && handleCreateAPIKey()}
                />
              </div>
              <div className="form-group">
                <label>Provider</label>
                <div className="role-select">
                  {['anthropic', 'openai'].map(p => (
                    <label key={p} className={`role-option ${newKeyProvider === p ? 'selected' : ''}`}>
                      <input
                        type="radio"
                        name="new-key-provider"
                        value={p}
                        checked={newKeyProvider === p}
                        onChange={() => setNewKeyProvider(p)}
                      />
                      <div>
                        <strong>{p.charAt(0).toUpperCase() + p.slice(1)}</strong>
                      </div>
                    </label>
                  ))}
                </div>
              </div>
              <div className="form-group">
                <label>Auth Method</label>
                <div className="role-select">
                  {AUTH_METHODS.map(m => (
                    <label key={m.value} className={`role-option ${newAuthMethod === m.value ? 'selected' : ''}`}>
                      <input
                        type="radio"
                        name="new-key-auth-method"
                        value={m.value}
                        checked={newAuthMethod === m.value}
                        onChange={() => {
                          setNewAuthMethod(m.value)
                          // BYOK implies API_USAGE billing — auto-select and lock it
                          if (m.value === 'BYOK') {
                            setNewBillingMode('API_USAGE')
                          }
                        }}
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
                <label>Billing Mode</label>
                <div className="role-select">
                  {BILLING_MODES.map(m => {
                    const disabled = newAuthMethod === 'BYOK' && m.value === 'MONTHLY_SUBSCRIPTION'
                    return (
                      <label key={m.value} className={`role-option ${newBillingMode === m.value ? 'selected' : ''} ${disabled ? 'disabled' : ''}`}>
                        <input
                          type="radio"
                          name="new-key-billing-mode"
                          value={m.value}
                          checked={newBillingMode === m.value}
                          onChange={() => setNewBillingMode(m.value)}
                          disabled={disabled}
                        />
                        <div>
                          <strong>{m.label}</strong>
                          <span className="role-desc">{m.description}{disabled ? ' (not available with BYOK)' : ''}</span>
                        </div>
                      </label>
                    )
                  })}
                </div>
              </div>
            </div>
            {createKeyError && (
              <div className="flash flash-error modal-flash">{createKeyError}</div>
            )}
            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => { setShowCreateKeyModal(false); setCreateKeyError(null) }}>
                Cancel
              </button>
              <button className="btn btn-primary" onClick={handleCreateAPIKey}
                disabled={!newKeyLabel.trim()}>
                Create
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── Add Provider Key Modal ───────────────────────────────────────── */}
      {showAddKeyModal && (
        <div className="modal-overlay" onClick={() => setShowAddKeyModal(false)}>
          <div className="modal-box modal-md" onClick={e => e.stopPropagation()}>
            <div className="modal-hdr">
              <h2>Add Provider Key</h2>
            </div>
            <div className="modal-body">
              <p className="modal-hint">
                Store your LLM provider API key securely. It will be encrypted at rest and never
                exposed to developers. Agents route through the gateway automatically.
              </p>
              <div className="form-group">
                <label>Provider</label>
                <div className="role-select">
                  {(['anthropic', 'openai'] as const).map(p => (
                    <label key={p} className={`role-option ${addKeyProvider === p ? 'selected' : ''}`}>
                      <input
                        type="radio"
                        name="provider"
                        value={p}
                        checked={addKeyProvider === p}
                        onChange={() => setAddKeyProvider(p)}
                      />
                      <div>
                        <strong>{p.charAt(0).toUpperCase() + p.slice(1)}</strong>
                      </div>
                    </label>
                  ))}
                </div>
              </div>
              <div className="form-group">
                <label>Label <span className="required">*</span></label>
                <input
                  type="text"
                  value={addKeyLabel}
                  onChange={e => setAddKeyLabel(e.target.value)}
                  placeholder="e.g. prod-anthropic-key"
                  autoFocus
                />
              </div>
              <div className="form-group">
                <label>API Key <span className="required">*</span></label>
                <input
                  type="password"
                  value={addKeyValue}
                  onChange={e => setAddKeyValue(e.target.value)}
                  placeholder="sk-ant-… or sk-…"
                  autoComplete="off"
                />
              </div>
            </div>
            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => setShowAddKeyModal(false)}
                disabled={addingKey}>
                Cancel
              </button>
              <button className="btn btn-primary" onClick={handleAddProviderKey}
                disabled={!addKeyLabel.trim() || !addKeyValue.trim() || addingKey}>
                {addingKey ? 'Adding…' : 'Add Key'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── Limit Reached Modal ──────────────────────────────────────────── */}
      {limitModal && (
        <div className="modal-overlay" onClick={() => setLimitModal(null)}>
          <div className="modal-box modal-md" onClick={e => e.stopPropagation()}>
            <div className="modal-hdr">
              <h2>
                {limitModal.type === 'keys' ? 'API Key Limit Reached'
                  : 'Provider Key Limit Reached'}
              </h2>
            </div>
            <div className="modal-body">
              <p>
                {limitModal.type === 'keys'
                  ? `You've reached the maximum of ${keyLimit} API key${keyLimit !== 1 ? 's' : ''} on your current plan. Upgrade your plan to create more API keys.`
                  : `You've reached the maximum of ${providerKeyLimit} provider key${providerKeyLimit !== 1 ? 's' : ''} on your current plan. Upgrade your plan to add more provider keys.`}
              </p>
            </div>
            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => setLimitModal(null)}>Cancel</button>
              <button className="btn btn-primary" onClick={() => navigate('/plan')}>Go to Plan</button>
            </div>
          </div>
        </div>
      )}

      {/* ── New Key Secret Modal ─────────────────────────────────────────── */}
      {showNewKeyModal && newKeySecret && (
        <div className="modal-overlay">
          <div className="modal-box modal-lg">
            <div className="modal-hdr">
              <h2>API Key Created</h2>
            </div>
            <div className="modal-body">
              <div className="warn-box">
                <span className="warn-icon">!</span>
                <p><strong>Save this now.</strong> The full secret is shown only once and cannot be retrieved again.</p>
              </div>
              <div className="key-reveal">
                <code>{newKeySecret}</code>
                <button className="btn btn-small btn-secondary"
                  onClick={() => copy(newKeySecret, 'secret')}>
                  {copiedID === 'secret' ? 'Copied!' : 'Copy'}
                </button>
              </div>
              <div className="install-box">
                <h3>Quick Setup</h3>
                {createdAuthMethod === 'BYOK' ? (
                  <>
                    <div className="install-step">
                      <div className="warn-box" style={{ marginTop: 0 }}>
                        <span className="warn-icon">!</span>
                        <p>Remember to add your <strong>{createdProvider === 'openai' ? 'OpenAI' : 'Anthropic'} provider key</strong> in the Provider Keys section before using this gateway key.</p>
                      </div>
                    </div>
                    {createdProvider === 'openai' && createdBillingMode === 'API_USAGE' && (
                      <>
                        <div className="install-step">
                          <h4>Option A: Codex CLI</h4>
                        </div>
                        <div className="install-step">
                          <h4>1. Clear <code>~/.codex/config.toml</code> and paste the following config</h4>
                          <div className="cmd-box">
                            <pre>{`model_provider = "tokengate"\n\n[model_providers.tokengate]\nname = "TokenGate Proxy"\nbase_url = "https://gateway.tokengate.to/v1"\nwire_api = "responses"\nhttp_headers = { \n  "X-Tokengate-Key" = "${newKeySecret}" \n}`}</pre>
                            <button className="btn btn-small btn-secondary"
                              onClick={() => copy(
                                `model_provider = "tokengate"\n\n[model_providers.tokengate]\nname = "TokenGate Proxy"\nbase_url = "https://gateway.tokengate.to/v1"\nwire_api = "responses"\nhttp_headers = { \n  "X-Tokengate-Key" = "${newKeySecret}" \n}`,
                                'codex-config'
                              )}>
                              {copiedID === 'codex-config' ? 'Copied!' : 'Copy'}
                            </button>
                          </div>
                        </div>
                        <div className="install-step">
                          <h4>2. Run <code>codex</code> in a code repo and select "3. Provide your own API key" if prompted, otherwise you are good to go</h4>
                        </div>
                        <div className="install-step" style={{ marginTop: '1.5rem', borderTop: '1px solid var(--border)', paddingTop: '1rem' }}>
                          <h4>Option B: Direct API Calls (curl / SDK)</h4>
                        </div>
                        <div className="install-step">
                          <h4>1. Example curl request</h4>
                          <div className="cmd-box">
                            <pre>{`curl https://gateway.tokengate.to/v1/openai/chat/completions \\
    -H "X-TokenGate-Key: ${newKeySecret}" \\
    -H "Content-Type: application/json" \\
    -d '{"model":"gpt-4.1","max_tokens":20,"messages":[{"role":"user","content":"Hello!"}]}'`}</pre>
                            <button className="btn btn-small btn-secondary"
                              onClick={() => copy(
                                `curl https://gateway.tokengate.to/v1/openai/chat/completions \\\n  -H "X-TokenGate-Key: ${newKeySecret}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"gpt-4.1","max_tokens":20,"messages":[{"role":"user","content":"Hello!"}]}'`,
                                'curl-openai'
                              )}>
                              {copiedID === 'curl-openai' ? 'Copied!' : 'Copy'}
                            </button>
                          </div>
                        </div>
                        <div className="install-step">
                          <h4>2. Or set environment variables for OpenAI SDK-compatible tools</h4>
                          <div className="cmd-box">
                            <pre>{`export OPENAI_BASE_URL=https://gateway.tokengate.to/v1/openai\nexport OPENAI_API_KEY="${newKeySecret}"`}</pre>
                            <button className="btn btn-small btn-secondary"
                              onClick={() => copy(
                                `export OPENAI_BASE_URL=https://gateway.tokengate.to/v1/openai\nexport OPENAI_API_KEY="${newKeySecret}"`,
                                'env'
                              )}>
                              {copiedID === 'env' ? 'Copied!' : 'Copy'}
                            </button>
                          </div>
                        </div>
                      </>
                    )}
                  </>
                ) : createdProvider === 'openai' && createdAuthMethod === 'BROWSER_OAUTH' && createdBillingMode === 'MONTHLY_SUBSCRIPTION' ? (
                  <>
                    <div className="install-step">
                      <h4>1. Paste the following config to the top of <code>~/.codex/config.toml</code></h4>
                      <div className="cmd-box">
                        <pre>{`model_provider = "tokengate"\n\n[model_providers.tokengate]\nname = "TokenGate Proxy"\nbase_url = "https://gateway.tokengate.to/v1"\nrequires_openai_auth = true\nwire_api = "responses"\nhttp_headers = { \n  "X-Tokengate-Key" = "${newKeySecret}" \n}`}</pre>
                        <button className="btn btn-small btn-secondary"
                          onClick={() => copy(
                            `model_provider = "tokengate"\n\n[model_providers.tokengate]\nname = "TokenGate Proxy"\nbase_url = "https://gateway.tokengate.to/v1"\nrequires_openai_auth = true\nwire_api = "responses"\nhttp_headers = { \n  "X-Tokengate-Key" = "${newKeySecret}" \n}`,
                            'env'
                          )}>
                          {copiedID === 'env' ? 'Copied!' : 'Copy'}
                        </button>
                      </div>
                    </div>
                    <div className="install-step">
                      <h4>2. Run <code>codex</code> command in a code repo</h4>
                    </div>
                  </>
                ) : (
                  <div className="install-step">
                    <h4>Set environment variables</h4>
                    <div className="cmd-box">
                      {createdProvider === 'openai' ? (
                        <pre>{`export OPENAI_BASE_URL=https://gateway.tokengate.to/v1/openai\nexport OPENAI_API_KEY="${newKeySecret}"${createdBillingMode === 'API_USAGE' ? '\n# No separate OpenAI key needed — the gateway uses your stored provider key' : '\n# Codex CLI will add its own auth automatically'}`}</pre>
                      ) : (
                        <pre>{`export ANTHROPIC_BASE_URL=https://gateway.tokengate.to\nexport ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:${newKeySecret}"${'\n# Claude Code will add its own auth automatically'}`}</pre>
                      )}
                      <button className="btn btn-small btn-secondary"
                        onClick={() => copy(
                          createdProvider === 'openai'
                            ? `export OPENAI_BASE_URL=https://gateway.tokengate.to/v1/openai\nexport OPENAI_API_KEY="${newKeySecret}"`
                            : `export ANTHROPIC_BASE_URL=https://gateway.tokengate.to\nexport ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:${newKeySecret}"`,
                          'env'
                        )}>
                        {copiedID === 'env' ? 'Copied!' : 'Copy'}
                      </button>
                    </div>
                  </div>
                )}
                {createdAuthMethod === 'BYOK' && !(createdProvider === 'openai' && createdBillingMode === 'API_USAGE') && (
                  <div className="install-step">
                    <h4>Test the gateway (example curl)</h4>
                    <div className="cmd-box">
                      {createdProvider === 'openai' ? (
                        <>
                          <pre>{`curl https://gateway.tokengate.to/v1/openai/chat/completions \\
    -H "X-TokenGate-Key: ${newKeySecret}" \\
    -H "Content-Type: application/json" \\
    -d '{"model":"gpt-4.1","max_tokens":20,"messages":[{"role":"user","content":"Hello!"}]}'`}</pre>
                          <button className="btn btn-small btn-secondary"
                            onClick={() => copy(
                              `curl https://gateway.tokengate.to/v1/openai/chat/completions \\\n  -H "X-TokenGate-Key: ${newKeySecret}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"gpt-4.1","max_tokens":20,"messages":[{"role":"user","content":"Hello!"}]}'`,
                              'curl'
                            )}>
                            {copiedID === 'curl' ? 'Copied!' : 'Copy'}
                          </button>
                        </>
                      ) : (
                        <>
                          <pre>{`curl https://gateway.tokengate.to/v1/messages \\
    -H "X-TokenGate-Key: ${newKeySecret}" \\
    -H "Content-Type: application/json" \\
    -d '{"model":"claude-sonnet-4-6","max_tokens":20,"messages":[{"role":"user","content":"Hello!"}]}'`}</pre>
                          <button className="btn btn-small btn-secondary"
                            onClick={() => copy(
                              `curl https://gateway.tokengate.to/v1/messages \\\n  -H "X-TokenGate-Key: ${newKeySecret}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"claude-sonnet-4-6","max_tokens":20,"messages":[{"role":"user","content":"Hello!"}]}'`,
                              'curl'
                            )}>
                            {copiedID === 'curl' ? 'Copied!' : 'Copy'}
                          </button>
                        </>
                      )}
                    </div>
                  </div>
                )}
              </div>
            </div>
            <div className="modal-ftr">
              <button className="btn btn-primary" onClick={() => {
                setShowNewKeyModal(false)
                setNewKeySecret(null)
              }}>
                I've saved my key
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
