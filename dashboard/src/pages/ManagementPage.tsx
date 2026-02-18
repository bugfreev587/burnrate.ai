import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { useUserSync, hasPermission } from '../hooks/useUserSync'
import type { UserRole } from '../hooks/useUserSync'
import Navbar from '../components/Navbar'
import './ManagementPage.css'

const API_SERVER_URL = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

interface User {
  id: string
  email: string
  name: string
  role: UserRole
  status: string
  created_at: string
}

interface APIKey {
  key_id: string
  label: string
  scopes: string[] | null
  expires_at: string | null
  created_at: string
}

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

  const [users, setUsers] = useState<User[]>([])
  const [apiKeys, setApiKeys] = useState<APIKey[]>([])
  const [providerKeys, setProviderKeys] = useState<ProviderKey[]>([])
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
  const [newKeySecret, setNewKeySecret] = useState<string | null>(null)
  const [newKeyID, setNewKeyID] = useState('')
  const [copiedID, setCopiedID] = useState<string | null>(null)

  // Invite modal
  const [showInviteModal, setShowInviteModal] = useState(false)
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteName, setInviteName] = useState('')
  const [inviteRole, setInviteRole] = useState<'viewer' | 'editor'>('viewer')
  const [inviting, setInviting] = useState(false)

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

  const fetchProviderKeys = useCallback(async () => {
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/provider_keys`, { headers: headers() })
      if (res.ok) {
        const data = await res.json()
        setProviderKeys(data.provider_keys ?? [])
      }
    } catch (err) {
      console.error('fetch provider keys:', err)
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
      await Promise.all([fetchUsers(), fetchAPIKeys(), fetchProviderKeys()])
      setLoading(false)
    }
    load()
  }, [isSynced, isAdmin, navigate, fetchUsers, fetchAPIKeys, fetchProviderKeys])

  // ── Invite user ──────────────────────────────────────────────────────────

  const handleInviteUser = async () => {
    if (!inviteEmail.trim()) {
      showError('Email is required')
      return
    }
    setInviting(true)
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/users/invite`, {
        method: 'POST',
        headers: headers(),
        body: JSON.stringify({
          email: inviteEmail.trim(),
          name: inviteName.trim(),
          role: inviteRole,
        }),
      })
      const data = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(data.message ?? data.error ?? 'Failed to invite user')
      showSuccess(`Invite sent to ${inviteEmail}. They'll join when they sign up.`)
      setShowInviteModal(false)
      setInviteEmail('')
      setInviteName('')
      setInviteRole('viewer')
      fetchUsers()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to invite user')
    } finally {
      setInviting(false)
    }
  }

  // ── User role actions ────────────────────────────────────────────────────

  const handleUpdateRole = async (targetID: string, newRole: 'viewer' | 'editor') => {
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/admin/users/${targetID}/role`, {
        method: 'PATCH',
        headers: headers(),
        body: JSON.stringify({ role: newRole }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.message ?? d.error ?? 'Failed to update role')
      }
      showSuccess(`Role updated to ${newRole}`)
      fetchUsers()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to update role')
    }
  }

  const handlePromoteAdmin = async (targetID: string) => {
    if (!confirm('Promote this user to admin?')) return
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/owner/users/${targetID}/promote-admin`, {
        method: 'POST',
        headers: headers(),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.message ?? d.error ?? 'Failed to promote user')
      }
      showSuccess('User promoted to admin')
      fetchUsers()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to promote user')
    }
  }

  const handleDemoteAdmin = async (targetID: string) => {
    if (!confirm('Demote this admin to editor?')) return
    try {
      const res = await fetch(`${API_SERVER_URL}/v1/owner/users/${targetID}/demote-admin`, {
        method: 'DELETE',
        headers: headers(),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.message ?? d.error ?? 'Failed to demote admin')
      }
      showSuccess('Admin demoted to editor')
      fetchUsers()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to demote admin')
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
        throw new Error(d.message ?? d.error ?? 'Failed to suspend')
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
        throw new Error(d.message ?? d.error ?? 'Failed to unsuspend')
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
        throw new Error(d.message ?? d.error ?? 'Failed to remove user')
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

          {/* ── Team Members ─────────────────────────────────────────────── */}
          <section className="mgmt-section">
            <div className="section-hdr">
              <div>
                <h2>Team Members</h2>
                <p className="section-desc">Manage who has access to your workspace.</p>
              </div>
              <button className="btn btn-primary" onClick={() => {
                setInviteEmail(''); setInviteName(''); setInviteRole('viewer')
                setShowInviteModal(true)
              }}>
                Invite Member
              </button>
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
                    <tr key={u.id} className={u.id === userId ? 'row-self' : ''}>
                      <td>{u.name || u.email?.split('@')[0] || '—'}</td>
                      <td className="text-muted">{u.email}</td>
                      <td><span className={`role-badge role-${u.role}`}>{u.role}</span></td>
                      <td><span className={`status-badge status-${u.status}`}>{u.status}</span></td>
                      <td className="actions-cell">
                        {u.id === userId ? (
                          <span className="you-badge">You</span>
                        ) : (
                          <>
                            {/* viewer ↔ editor (admin+) */}
                            {u.role === 'viewer' && u.status !== 'pending' && (
                              <button className="btn btn-small btn-secondary"
                                onClick={() => handleUpdateRole(u.id, 'editor')}>
                                → Editor
                              </button>
                            )}
                            {u.role === 'editor' && (
                              <button className="btn btn-small btn-secondary"
                                onClick={() => handleUpdateRole(u.id, 'viewer')}>
                                → Viewer
                              </button>
                            )}
                            {/* promote/demote admin (owner only) */}
                            {isOwner && (u.role === 'viewer' || u.role === 'editor') && (
                              <button className="btn btn-small btn-secondary"
                                onClick={() => handlePromoteAdmin(u.id)}>
                                → Admin
                              </button>
                            )}
                            {isOwner && u.role === 'admin' && (
                              <button className="btn btn-small btn-secondary"
                                onClick={() => handleDemoteAdmin(u.id)}>
                                ↓ Editor
                              </button>
                            )}
                            {/* Suspend / Unsuspend */}
                            {u.role !== 'owner' && u.status !== 'pending' && (u.role !== 'admin' || isOwner) && (
                              u.status === 'active' ? (
                                <button className="btn btn-small btn-warning"
                                  onClick={() => handleSuspend(u.id)}>
                                  Suspend
                                </button>
                              ) : u.status === 'suspended' ? (
                                <button className="btn btn-small btn-secondary"
                                  onClick={() => handleUnsuspend(u.id)}>
                                  Unsuspend
                                </button>
                              ) : null
                            )}
                            {/* Remove */}
                            {u.role !== 'owner' && (u.role !== 'admin' || isOwner) && (
                              <button className="btn btn-small btn-danger"
                                onClick={() => handleRemoveUser(u.id)}>
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
                  Upstream LLM provider API keys (Anthropic, OpenAI) stored and rotated centrally.
                  Set one as active and agents will use it automatically via the gateway proxy.
                </p>
              </div>
              <button className="btn btn-primary" onClick={() => {
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

      {/* ── Invite Member Modal ───────────────────────────────────────────── */}
      {showInviteModal && (
        <div className="modal-overlay" onClick={() => setShowInviteModal(false)}>
          <div className="modal-box" onClick={e => e.stopPropagation()}>
            <div className="modal-hdr">
              <h2>Invite Team Member</h2>
            </div>
            <div className="modal-body">
              <p className="modal-hint">
                Enter the invitee's email address. They'll join your workspace automatically
                when they sign up with that email.
              </p>
              <div className="form-group">
                <label>Email <span className="required">*</span></label>
                <input
                  type="email"
                  value={inviteEmail}
                  onChange={e => setInviteEmail(e.target.value)}
                  placeholder="colleague@company.com"
                  autoFocus
                />
              </div>
              <div className="form-group">
                <label>Name <span className="optional">(optional)</span></label>
                <input
                  type="text"
                  value={inviteName}
                  onChange={e => setInviteName(e.target.value)}
                  placeholder="Full name"
                />
              </div>
              <div className="form-group">
                <label>Role</label>
                <div className="role-select">
                  {(['viewer', 'editor'] as const).map(r => (
                    <label key={r} className={`role-option ${inviteRole === r ? 'selected' : ''}`}>
                      <input
                        type="radio"
                        name="invite-role"
                        value={r}
                        checked={inviteRole === r}
                        onChange={() => setInviteRole(r)}
                      />
                      <div>
                        <strong>{r.charAt(0).toUpperCase() + r.slice(1)}</strong>
                        <span className="role-desc">
                          {r === 'viewer' ? 'Can view usage data' : 'Can view and manage API keys'}
                        </span>
                      </div>
                    </label>
                  ))}
                </div>
              </div>
            </div>
            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => setShowInviteModal(false)}
                disabled={inviting}>
                Cancel
              </button>
              <button className="btn btn-primary" onClick={handleInviteUser}
                disabled={!inviteEmail.trim() || inviting}>
                {inviting ? 'Inviting…' : 'Send Invite'}
              </button>
            </div>
          </div>
        </div>
      )}

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

      {/* ── Add Provider Key Modal ───────────────────────────────────────── */}
      {showAddKeyModal && (
        <div className="modal-overlay" onClick={() => setShowAddKeyModal(false)}>
          <div className="modal-box" onClick={e => e.stopPropagation()}>
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
  -d '{"provider":"anthropic","model":"claude-sonnet-4-6","prompt_tokens":100,"completion_tokens":50,"cost":0.001,"request_id":"req_abc123"}'`}</pre>
                    <button className="btn btn-small btn-secondary"
                      onClick={() => copy(
                        `curl -X POST ${API_SERVER_URL}/v1/agent/usage \\\n  -H "Authorization: ApiKey ${newKeySecret}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"provider":"anthropic","model":"claude-sonnet-4-6","prompt_tokens":100,"completion_tokens":50,"cost":0.001,"request_id":"req_abc123"}'`,
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
