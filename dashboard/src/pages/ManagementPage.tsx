import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { useUserSync, hasPermission } from '../hooks/useUserSync'
import type { UserRole } from '../hooks/useUserSync'
import Navbar from '../components/Navbar'
import './ManagementPage.css'

const API_SERVER_URL = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

interface User {
  ID: string
  Email: string
  Name: string
  Role: UserRole
  Status: string
  CreatedAt: string
}

interface APIKey {
  key_id: string
  label: string
  scopes: string[] | null
  expires_at: string | null
  created_at: string
}

export default function ManagementPage() {
  const navigate = useNavigate()
  const { role, userId, isSynced } = useUserSync()

  const [users, setUsers] = useState<User[]>([])
  const [apiKeys, setApiKeys] = useState<APIKey[]>([])
  const [loading, setLoading] = useState(true)
  const [successMsg, setSuccessMsg] = useState<string | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  // Modal states
  const [showCreateKeyModal, setShowCreateKeyModal] = useState(false)
  const [showNewKeyModal, setShowNewKeyModal] = useState(false)
  const [newKeyLabel, setNewKeyLabel] = useState('')
  const [newKeySecret, setNewKeySecret] = useState<string | null>(null)
  const [newKeyID, setNewKeyID] = useState('')
  const [copiedID, setCopiedID] = useState<string | null>(null)

  const isAdmin = hasPermission(role, 'admin')
  const isOwner = role === 'owner'

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

  const fetchUsers = useCallback(async () => {
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/users`, { headers: headers() })
      if (res.ok) {
        const data = await res.json()
        setUsers(data.users ?? [])
      }
    } catch (err) {
      console.error('fetch users:', err)
    }
  }, [headers])

  const fetchAPIKeys = useCallback(async () => {
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/api_keys`, { headers: headers() })
      if (res.ok) {
        const data = await res.json()
        setApiKeys(data.api_keys ?? [])
      }
    } catch (err) {
      console.error('fetch api keys:', err)
    }
  }, [headers])

  useEffect(() => {
    if (!isSynced) return
    if (!isAdmin) {
      navigate('/dashboard')
      return
    }
    const load = async () => {
      setLoading(true)
      await Promise.all([fetchUsers(), fetchAPIKeys()])
      setLoading(false)
    }
    load()
  }, [isSynced, isAdmin, navigate, fetchUsers, fetchAPIKeys])

  // ── User actions ─────────────────────────────────────────────────────────

  const handleUpdateRole = async (targetID: string, newRole: 'viewer' | 'editor') => {
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/users/${targetID}/role`, {
        method: 'PATCH',
        headers: headers(),
        body: JSON.stringify({ role: newRole }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error ?? 'Failed to update role')
      }
      showSuccess(`Role updated to ${newRole}`)
      fetchUsers()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to update role')
    }
  }

  const handleSuspend = async (targetID: string) => {
    if (!confirm('Suspend this user?')) return
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/users/${targetID}/suspend`, {
        method: 'PATCH',
        headers: headers(),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error ?? 'Failed to suspend')
      }
      showSuccess('User suspended')
      fetchUsers()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to suspend user')
    }
  }

  const handleUnsuspend = async (targetID: string) => {
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/users/${targetID}/unsuspend`, {
        method: 'PATCH',
        headers: headers(),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error ?? 'Failed to unsuspend')
      }
      showSuccess('User unsuspended')
      fetchUsers()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to unsuspend user')
    }
  }

  const handleRemoveUser = async (targetID: string) => {
    if (!confirm('Remove this user? This cannot be undone.')) return
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/users/${targetID}`, {
        method: 'DELETE',
        headers: headers(),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error ?? 'Failed to remove user')
      }
      showSuccess('User removed')
      fetchUsers()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to remove user')
    }
  }

  // ── API Key actions ──────────────────────────────────────────────────────

  const handleCreateAPIKey = async () => {
    if (!newKeyLabel.trim()) {
      showError('Please enter a label')
      return
    }
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/api_keys`, {
        method: 'POST',
        headers: headers(),
        body: JSON.stringify({ label: newKeyLabel.trim(), scopes: ['*'] }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error ?? 'Failed to create API key')
      }
      const data = await res.json()
      setNewKeyID(data.key_id)
      setNewKeySecret(`${data.key_id}:${data.secret}`)
      setShowCreateKeyModal(false)
      setShowNewKeyModal(true)
      setNewKeyLabel('')
      fetchAPIKeys()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to create API key')
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

          {/* ── Team Members ─────────────────────────────────────────────── */}
          <section className="mgmt-section">
            <div className="section-hdr">
              <h2>Team Members</h2>
            </div>
            <div className="table-scroll">
              <table className="mgmt-table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Email</th>
                    <th>Role</th>
                    <th>Status</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {users.length === 0 ? (
                    <tr><td colSpan={5} className="empty-cell">No users found.</td></tr>
                  ) : users.map(u => (
                    <tr key={u.ID} className={u.ID === userId ? 'row-self' : ''}>
                      <td>{u.Name || u.Email?.split('@')[0] || '—'}</td>
                      <td className="text-muted">{u.Email}</td>
                      <td><span className={`role-badge role-${u.Role}`}>{u.Role}</span></td>
                      <td><span className={`status-badge status-${u.Status}`}>{u.Status}</span></td>
                      <td className="actions-cell">
                        {u.ID === userId ? (
                          <span className="you-badge">You</span>
                        ) : (
                          <>
                            {/* Promote viewer → editor */}
                            {u.Role === 'viewer' && (
                              <button className="btn btn-small btn-secondary"
                                onClick={() => handleUpdateRole(u.ID, 'editor')}>
                                Promote
                              </button>
                            )}
                            {/* Demote editor → viewer */}
                            {u.Role === 'editor' && (
                              <button className="btn btn-small btn-secondary"
                                onClick={() => handleUpdateRole(u.ID, 'viewer')}>
                                Demote
                              </button>
                            )}
                            {/* Suspend / Unsuspend */}
                            {u.Role !== 'owner' && (u.Role !== 'admin' || isOwner) && (
                              u.Status === 'active' ? (
                                <button className="btn btn-small btn-warning"
                                  onClick={() => handleSuspend(u.ID)}>
                                  Suspend
                                </button>
                              ) : u.Status === 'suspended' ? (
                                <button className="btn btn-small btn-secondary"
                                  onClick={() => handleUnsuspend(u.ID)}>
                                  Unsuspend
                                </button>
                              ) : null
                            )}
                            {/* Remove */}
                            {u.Role !== 'owner' && (u.Role !== 'admin' || isOwner) && (
                              <button className="btn btn-small btn-danger"
                                onClick={() => handleRemoveUser(u.ID)}>
                                Remove
                              </button>
                            )}
                          </>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>

          {/* ── Gateway API Keys ─────────────────────────────────────────── */}
          <section className="mgmt-section">
            <div className="section-hdr">
              <div>
                <h2>Gateway API Keys</h2>
                <p className="section-desc">
                  Keys used by the claude-code agent to report usage through the burnrate gateway.
                </p>
              </div>
              <button className="btn btn-primary" onClick={() => { setNewKeyLabel(''); setShowCreateKeyModal(true) }}>
                Create Key
              </button>
            </div>
            <div className="table-scroll">
              <table className="mgmt-table">
                <thead>
                  <tr>
                    <th>Key ID</th>
                    <th>Label</th>
                    <th>Created</th>
                    <th>Expires</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {apiKeys.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="empty-cell">
                        <div className="empty-cta">
                          <p>No API keys yet. Create one to start reporting usage.</p>
                          <button className="btn btn-primary"
                            onClick={() => { setNewKeyLabel(''); setShowCreateKeyModal(true) }}>
                            Create Your First Key
                          </button>
                        </div>
                      </td>
                    </tr>
                  ) : apiKeys.map(k => (
                    <tr key={k.key_id}>
                      <td><code className="key-id">{k.key_id.slice(0, 8)}…</code></td>
                      <td>{k.label}</td>
                      <td className="text-muted">{new Date(k.created_at).toLocaleDateString()}</td>
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
                <h2>Provider Keys</h2>
                <p className="section-desc">
                  Upstream LLM provider API keys (Anthropic, OpenAI) managed centrally by the gateway.
                </p>
              </div>
              <button className="btn btn-secondary" disabled title="Coming soon">
                Add Key
              </button>
            </div>
            <div className="coming-soon-box">
              <p>Provider key management is coming soon. You'll be able to store and rotate your
                Anthropic API keys here so they never need to be distributed to developers.</p>
            </div>
          </section>

        </div>
      </div>

      {/* ── Create API Key Modal ─────────────────────────────────────────── */}
      {showCreateKeyModal && (
        <div className="modal-overlay" onClick={() => setShowCreateKeyModal(false)}>
          <div className="modal-box" onClick={e => e.stopPropagation()}>
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
            </div>
            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => setShowCreateKeyModal(false)}>
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

                <div className="install-step">
                  <h4>Set environment variable</h4>
                  <div className="cmd-box">
                    <pre>{`export BURNRATE_API_KEY="${newKeySecret}"`}</pre>
                    <button className="btn btn-small btn-secondary"
                      onClick={() => copy(`export BURNRATE_API_KEY="${newKeySecret}"`, 'env')}>
                      {copiedID === 'env' ? 'Copied!' : 'Copy'}
                    </button>
                  </div>
                </div>

                <div className="install-step">
                  <h4>Report usage (example curl)</h4>
                  <div className="cmd-box">
                    <pre>{`curl -X POST ${API_SERVER_URL}/v1/agent/usage \\
  -H "Authorization: ApiKey ${newKeyID}:<secret>" \\
  -H "Content-Type: application/json" \\
  -d '{"provider":"anthropic","model":"claude-sonnet-4-6","input_tokens":100,"output_tokens":50,"cost_usd":0.001}'`}</pre>
                    <button className="btn btn-small btn-secondary"
                      onClick={() => copy(
                        `curl -X POST ${API_SERVER_URL}/v1/agent/usage \\\n  -H "Authorization: ApiKey ${newKeyID}:<secret>" \\\n  -H "Content-Type: application/json" \\\n  -d '{"provider":"anthropic","model":"claude-sonnet-4-6","input_tokens":100,"output_tokens":50,"cost_usd":0.001}'`,
                        'curl'
                      )}>
                      {copiedID === 'curl' ? 'Copied!' : 'Copy'}
                    </button>
                  </div>
                </div>
              </div>
            </div>
            <div className="modal-ftr">
              <button className="btn btn-primary" onClick={() => {
                setShowNewKeyModal(false)
                setNewKeySecret(null)
                setNewKeyID('')
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
