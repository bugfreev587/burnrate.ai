import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { hasPermission, type UserRole } from '../hooks/useUserSync'
import { useTenant } from '../contexts/TenantContext'
import { useProjects, type Project, type ProjectMember } from '../hooks/useProjects'
import { usePricingConfig } from '../hooks/usePricingConfig'
import { apiFetch } from '../lib/api'
import Navbar from '../components/Navbar'
import './ManagementPage.css'

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
  project_id: number | null
  model_allowlist: string | null
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

interface User {
  id: string
  email: string
  name: string
  role: UserRole
  status: string
  created_at: string
}

export default function ManagementPage() {
  const navigate = useNavigate()
  const { orgRole, userId, isSynced } = useTenant()
  const role = (orgRole as UserRole) ?? null
  const { projects, limit: projectLimit, slotsLeft: projectSlotsLeft, createProject, updateProject, deleteProject, listMembers, removeMember } = useProjects()
  const { catalog } = usePricingConfig()

  const [apiKeys, setApiKeys] = useState<APIKey[]>([])
  const [providerKeys, setProviderKeys] = useState<ProviderKey[]>([])
  const [keyLimit, setKeyLimit] = useState<number | null>(null)
  const [providerKeyLimit, setProviderKeyLimit] = useState<number | null>(null)
  const [revokeTarget, setRevokeTarget] = useState<ProviderKey | null>(null)
  const [loading, setLoading] = useState(true)
  const [successMsg, setSuccessMsg] = useState<string | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  // Team members
  const [users, setUsers] = useState<User[]>([])
  const [memberLimit, setMemberLimit] = useState<number | null>(null)
  const [showInviteModal, setShowInviteModal] = useState(false)
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteName, setInviteName] = useState('')
  const [inviteRole, setInviteRole] = useState<'viewer' | 'editor'>('viewer')
  const [inviting, setInviting] = useState(false)
  const [inviteError, setInviteError] = useState<string | null>(null)

  // Project filter
  const [filterProjectId, setFilterProjectId] = useState<number | ''>('')

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
  const [newKeyProjectId, setNewKeyProjectId] = useState<number | ''>('')
  const [newKeyModelAllowlist, setNewKeyModelAllowlist] = useState<string[]>([])

  // Deduplicated model names from the pricing catalog, filtered by selected provider.
  const availableModels = useMemo(() => {
    const names = new Set<string>()
    for (const entry of catalog) {
      if (entry.provider === newKeyProvider) names.add(entry.model_name)
    }
    return Array.from(names).sort()
  }, [catalog, newKeyProvider])

  const [createdAuthMethod, setCreatedAuthMethod] = useState<string>('')
  const [createdBillingMode, setCreatedBillingMode] = useState<string>('')
  const [createdProvider, setCreatedProvider] = useState<string>('')
  const [newKeySecret, setNewKeySecret] = useState<string | null>(null)
  const [copiedID, setCopiedID] = useState<string | null>(null)
  const [createKeyError, setCreateKeyError] = useState<string | null>(null)

  // Limit-reached modal
  const [limitModal, setLimitModal] = useState<{ type: 'keys' | 'provider_keys' | 'projects' | 'members' } | null>(null)

  // Project management
  const [showCreateProjectModal, setShowCreateProjectModal] = useState(false)
  const [newProjectName, setNewProjectName] = useState('')
  const [newProjectDesc, setNewProjectDesc] = useState('')
  const [creatingProject, setCreatingProject] = useState(false)
  const [createProjectError, setCreateProjectError] = useState<string | null>(null)
  const [editingProject, setEditingProject] = useState<Project | null>(null)
  const [editProjectName, setEditProjectName] = useState('')
  const [editProjectDesc, setEditProjectDesc] = useState('')
  const [projectMembers, setProjectMembers] = useState<ProjectMember[]>([])
  const [showProjectMembers, setShowProjectMembers] = useState<number | null>(null)
  const [loadingMembers, setLoadingMembers] = useState(false)

  // Rich dropdown open state
  const [authMethodOpen, setAuthMethodOpen] = useState(false)
  const [billingModeOpen, setBillingModeOpen] = useState(false)
  const [projectOpen, setProjectOpen] = useState(false)
  const authMethodRef = useRef<HTMLDivElement>(null)
  const billingModeRef = useRef<HTMLDivElement>(null)
  const projectRef = useRef<HTMLDivElement>(null)

  // Close dropdowns on outside click
  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (authMethodRef.current && !authMethodRef.current.contains(e.target as Node)) setAuthMethodOpen(false)
      if (billingModeRef.current && !billingModeRef.current.contains(e.target as Node)) setBillingModeOpen(false)
      if (projectRef.current && !projectRef.current.contains(e.target as Node)) setProjectOpen(false)
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  const canAccess = hasPermission(role, 'editor')
  const canManageTeam = hasPermission(role, 'admin')
  const canManageProviderKeys = hasPermission(role, 'admin')

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
      const res = await apiFetch('/v1/admin/api_keys')
      if (res.ok) {
        const data = await res.json()
        setApiKeys(data.api_keys ?? [])
        setKeyLimit(data.limit ?? null)
      }
    } catch (err) {
      console.error('fetch api keys:', err)
    }
  }, [])

  const fetchProviderKeys = useCallback(async () => {
    try {
      const res = await apiFetch('/v1/admin/provider_keys')
      if (res.ok) {
        const data = await res.json()
        setProviderKeys(data.provider_keys ?? [])
        setProviderKeyLimit(data.limit ?? null)
      }
    } catch (err) {
      console.error('fetch provider keys:', err)
    }
  }, [])

  const fetchUsers = useCallback(async () => {
    try {
      const res = await apiFetch('/v1/admin/users')
      if (res.ok) {
        const data = await res.json()
        setUsers(data.users ?? [])
        setMemberLimit(data.member_limit ?? null)
      }
    } catch (err) {
      console.error('fetch users:', err)
    }
  }, [])

  useEffect(() => {
    if (!isSynced) return
    if (!canAccess) {
      navigate('/dashboard')
      return
    }
    const load = async () => {
      setLoading(true)
      const fetches: Promise<void>[] = [fetchAPIKeys(), fetchProviderKeys()]
      if (canManageTeam) fetches.push(fetchUsers())
      await Promise.all(fetches)
      setLoading(false)
    }
    load()
  }, [isSynced, canAccess, canManageTeam, navigate, fetchAPIKeys, fetchProviderKeys, fetchUsers])

  // ── API Key actions ──────────────────────────────────────────────────────

  const handleCreateAPIKey = async () => {
    if (!newKeyLabel.trim()) {
      setCreateKeyError('Please enter a label')
      return
    }
    if (!newKeyProjectId) {
      setCreateKeyError('Please select a project')
      return
    }
    setCreateKeyError(null)
    try {
      const body: Record<string, unknown> = {
        label: newKeyLabel.trim(),
        provider: newKeyProvider,
        auth_method: newAuthMethod,
        billing_mode: newBillingMode,
        scopes: ['*'],
        project_id: newKeyProjectId,
      }
      if (newKeyModelAllowlist.length > 0) {
        body.model_allowlist = newKeyModelAllowlist
      }
      const res = await apiFetch('/v1/admin/api_keys', {
        method: 'POST',
        body: JSON.stringify(body),
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
      setNewKeyProjectId('')
      setNewKeyModelAllowlist([])
      fetchAPIKeys()
    } catch (err) {
      setCreateKeyError(err instanceof Error ? err.message : 'Failed to create API key')
    }
  }

  const handleRevokeAPIKey = async (keyID: string) => {
    if (!confirm('Revoke this API key? Any agents using it will stop working.')) return
    try {
      const res = await apiFetch(`/v1/admin/api_keys/${keyID}`, {
        method: 'DELETE',
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
      const res = await apiFetch('/v1/admin/provider_keys', {
        method: 'POST',
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
      const res = await apiFetch(`/v1/admin/provider_keys/${id}/activate`, {
        method: 'PUT',
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

  const confirmRevokeProviderKey = async () => {
    if (!revokeTarget) return
    try {
      const res = await apiFetch(`/v1/admin/provider_keys/${revokeTarget.id}`, {
        method: 'DELETE',
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error ?? 'Failed to revoke key')
      }
      showSuccess('Provider key revoked')
      setRevokeTarget(null)
      fetchProviderKeys()
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to revoke provider key')
      setRevokeTarget(null)
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

  // ── Project actions ──────────────────────────────────────────────────────

  const handleCreateProject = async () => {
    if (!newProjectName.trim()) { setCreateProjectError('Name is required'); return }
    setCreatingProject(true)
    setCreateProjectError(null)
    try {
      await createProject(newProjectName.trim(), newProjectDesc.trim())
      showSuccess('Project created')
      setShowCreateProjectModal(false)
      setNewProjectName('')
      setNewProjectDesc('')
    } catch (err) {
      setCreateProjectError(err instanceof Error ? err.message : 'Failed to create project')
    } finally {
      setCreatingProject(false)
    }
  }

  const handleUpdateProject = async () => {
    if (!editingProject) return
    try {
      await updateProject(editingProject.id, { name: editProjectName.trim(), description: editProjectDesc.trim() })
      showSuccess('Project updated')
      setEditingProject(null)
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to update project')
    }
  }

  const handleDeleteProject = async (id: number) => {
    if (!confirm('Delete this project? API keys and limits associated with it will need to be reassigned.')) return
    try {
      await deleteProject(id)
      showSuccess('Project deleted')
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to delete project')
    }
  }

  const handleViewProjectMembers = async (projectId: number) => {
    if (showProjectMembers === projectId) { setShowProjectMembers(null); return }
    setLoadingMembers(true)
    try {
      const members = await listMembers(projectId)
      setProjectMembers(members)
      setShowProjectMembers(projectId)
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to load members')
    } finally {
      setLoadingMembers(false)
    }
  }

  const handleRemoveProjectMember = async (projectId: number, memberId: string) => {
    if (!confirm('Remove this member from the project?')) return
    try {
      await removeMember(projectId, memberId)
      const members = await listMembers(projectId)
      setProjectMembers(members)
      showSuccess('Member removed from project')
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to remove member')
    }
  }

  // ── Team member actions ──────────────────────────────────────────────────

  const handleInviteUser = async () => {
    if (!inviteEmail.trim()) {
      setInviteError('Email is required')
      return
    }
    setInviting(true)
    setInviteError(null)
    try {
      const res = await apiFetch('/v1/admin/users/invite', {
        method: 'POST',
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
      setInviteError(null)
      fetchUsers()
    } catch (err) {
      setInviteError(err instanceof Error ? err.message : 'Failed to invite user')
    } finally {
      setInviting(false)
    }
  }

  const handleUpdateRole = async (targetID: string, newRole: 'viewer' | 'editor') => {
    try {
      const res = await apiFetch(`/v1/admin/users/${targetID}/role`, {
        method: 'PATCH',
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
      const res = await apiFetch(`/v1/owner/users/${targetID}/promote-admin`, {
        method: 'POST',
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
      const res = await apiFetch(`/v1/owner/users/${targetID}/demote-admin`, {
        method: 'DELETE',
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
      const res = await apiFetch(`/v1/admin/users/${targetID}/suspend`, {
        method: 'PATCH',
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
      const res = await apiFetch(`/v1/admin/users/${targetID}/unsuspend`, {
        method: 'PATCH',
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
      const res = await apiFetch(`/v1/admin/users/${targetID}`, {
        method: 'DELETE',
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
                    {apiKeys.length}/{keyLimit === null ? '\u221e' : keyLimit}
                  </span>
                </h2>
                <p className="section-desc">
                  Keys used by the AI code agent to report usage through the TokenGate gateway.
                </p>
              </div>
              <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
                {projects.length > 1 && (
                  <select
                    className="gateway-project-filter"
                    value={filterProjectId}
                    onChange={e => setFilterProjectId(e.target.value ? parseInt(e.target.value, 10) : '')}
                  >
                    <option value="">All Projects</option>
                    {projects.map(p => (
                      <option key={p.id} value={p.id}>{p.name}</option>
                    ))}
                  </select>
                )}
              <button className="btn btn-primary" onClick={() => {
                if (keyLimit !== null && apiKeys.length >= keyLimit) {
                  setLimitModal({ type: 'keys' })
                  return
                }
                setNewKeyLabel(''); setCreateKeyError(null); setNewKeyProjectId(projects[0]?.id ?? ''); setNewKeyModelAllowlist([]); setAuthMethodOpen(false); setBillingModeOpen(false); setShowCreateKeyModal(true)
              }}>
                Create Key
              </button>
              </div>
            </div>
            <div className="table-scroll">
              <table className="mgmt-table">
                <thead>
                  <tr>
                    <th>Key ID</th>
                    <th>Label</th>
                    <th>Project</th>
                    <th>Provider</th>
                    <th>Auth</th>
                    <th>Billing</th>
                    <th>Created</th>
                    <th>Last Seen</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {(() => {
                    const filtered = filterProjectId ? apiKeys.filter(k => k.project_id === filterProjectId) : apiKeys
                    return filtered.length === 0 ? (
                    <tr>
                      <td colSpan={9} className="empty-cell">
                        <div className="empty-cta">
                          <p>{filterProjectId ? 'No API keys in this project.' : 'No API keys yet. Create one to start reporting usage.'}</p>
                          {!filterProjectId && (
                          <button className="btn btn-primary"
                            onClick={() => { setNewKeyLabel(''); setCreateKeyError(null); setNewKeyProjectId(projects[0]?.id ?? ''); setNewKeyModelAllowlist([]); setAuthMethodOpen(false); setBillingModeOpen(false); setShowCreateKeyModal(true) }}>
                            Create Your First Key
                          </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ) : filtered.map(k => (
                    <tr key={k.key_id}>
                      <td><code className="key-id">{k.key_id.slice(0, 8)}\u2026</code></td>
                      <td>{k.label}</td>
                      <td className="text-muted">{projects.find(p => p.id === k.project_id)?.name ?? '\u2014'}</td>
                      <td><span className="provider-badge">{k.provider}</span></td>
                      <td><span className="mode-badge">{k.auth_method}</span></td>
                      <td><span className="mode-badge">{k.billing_mode}</span></td>
                      <td className="text-muted">{new Date(k.created_at).toLocaleDateString()}</td>
                      <td className="text-muted">{k.last_seen_at ? new Date(k.last_seen_at).toLocaleString() : 'Never'}</td>
                      <td>
                        <button className="btn btn-small btn-danger"
                          onClick={() => handleRevokeAPIKey(k.key_id)}>
                          Revoke
                        </button>
                      </td>
                    </tr>
                  ))
                  })()}
                </tbody>
              </table>
            </div>
          </section>

          {/* ── Provider Keys (Owner/Admin only) ─────────────────────────── */}
          {canManageProviderKeys && (
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
                          onClick={() => setRevokeTarget(k)}>
                          Revoke
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
          )}

          {/* ── Projects (admin only) ─────────────────────────────────────── */}
          {canManageProviderKeys && (
            <section className="mgmt-section">
              <div className="section-hdr">
                <div>
                  <h2>
                    Projects{' '}
                    <span className="section-count">
                      {projects.length}/{projectLimit === null ? '\u221e' : projectLimit}
                    </span>
                  </h2>
                  <p className="section-desc">Organize API keys and limits by project.</p>
                </div>
                <button className="btn btn-primary" onClick={() => {
                  if (projectSlotsLeft !== null && projectSlotsLeft <= 0) {
                    setLimitModal({ type: 'projects' })
                    return
                  }
                  setNewProjectName(''); setNewProjectDesc(''); setCreateProjectError(null)
                  setShowCreateProjectModal(true)
                }}>
                  Create Project
                </button>
              </div>
              <div className="table-scroll">
                <table className="mgmt-table">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Description</th>
                      <th>Status</th>
                      <th>Created</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {projects.length === 0 ? (
                      <tr><td colSpan={5} className="empty-cell">No projects yet.</td></tr>
                    ) : projects.map(p => (
                      <>
                        <tr key={p.id}>
                          <td>
                            {p.name}
                            {p.is_default && <span className="role-badge role-viewer" style={{ marginLeft: '0.5rem', fontSize: '0.7rem' }}>Default</span>}
                          </td>
                          <td className="text-muted">{p.description || '\u2014'}</td>
                          <td><span className={`status-badge status-${p.status}`}>{p.status}</span></td>
                          <td className="text-muted">{new Date(p.created_at).toLocaleDateString()}</td>
                          <td className="actions-cell">
                            <button className="btn btn-small btn-secondary" onClick={() => handleViewProjectMembers(p.id)}>
                              {showProjectMembers === p.id ? 'Hide Members' : 'Members'}
                            </button>
                            <button className="btn btn-small btn-secondary" onClick={() => {
                              setEditingProject(p)
                              setEditProjectName(p.name)
                              setEditProjectDesc(p.description)
                            }}>
                              Edit
                            </button>
                            {!p.is_default && (
                              <button className="btn btn-small btn-danger" onClick={() => handleDeleteProject(p.id)}>
                                Delete
                              </button>
                            )}
                          </td>
                        </tr>
                        {showProjectMembers === p.id && (
                          <tr key={`members-${p.id}`}>
                            <td colSpan={5} style={{ padding: '0.5rem 1rem', background: 'var(--bg-secondary, #1a1a2e)' }}>
                              {loadingMembers ? (
                                <div className="spinner" style={{ margin: '0.5rem auto' }} />
                              ) : projectMembers.length === 0 ? (
                                <p className="text-muted" style={{ margin: '0.5rem 0' }}>No members assigned to this project.</p>
                              ) : (
                                <table className="mgmt-table" style={{ marginBottom: 0 }}>
                                  <thead>
                                    <tr>
                                      <th>Name</th>
                                      <th>Email</th>
                                      <th>Project Role</th>
                                      <th>Actions</th>
                                    </tr>
                                  </thead>
                                  <tbody>
                                    {projectMembers.map(m => (
                                      <tr key={m.user_id}>
                                        <td>{m.name || '\u2014'}</td>
                                        <td className="text-muted">{m.email}</td>
                                        <td><span className={`role-badge`}>{m.project_role}</span></td>
                                        <td>
                                          <button className="btn btn-small btn-danger" onClick={() => handleRemoveProjectMember(p.id, m.user_id)}>
                                            Remove
                                          </button>
                                        </td>
                                      </tr>
                                    ))}
                                  </tbody>
                                </table>
                              )}
                            </td>
                          </tr>
                        )}
                      </>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          )}

          {/* ── Team Members (admin+ only) ─────────────────────────────── */}
          {canManageTeam && (
            <section className="mgmt-section">
              <div className="section-hdr">
                <div>
                  <h2>
                    Team Members{' '}
                    <span className="section-count">
                      {users.length}/{memberLimit === null ? '\u221e' : memberLimit}
                    </span>
                  </h2>
                  <p className="section-desc">Manage who has access to your workspace.</p>
                </div>
                <button className="btn btn-primary" onClick={() => {
                  if (memberLimit !== null && users.length >= memberLimit) {
                    setLimitModal({ type: 'members' })
                    return
                  }
                  setInviteEmail(''); setInviteName(''); setInviteRole('viewer')
                  setInviteError(null); setShowInviteModal(true)
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
                        <td>{u.name || u.email?.split('@')[0] || '\u2014'}</td>
                        <td className="text-muted">{u.email}</td>
                        <td><span className={`role-badge role-${u.role}`}>{u.role}</span></td>
                        <td><span className={`status-badge status-${u.status}`}>{u.status}</span></td>
                        <td className="actions-cell">
                          {u.id === userId ? (
                            <span className="you-badge">You</span>
                          ) : (
                            <>
                              {u.role === 'viewer' && u.status !== 'pending' && (
                                <button className="btn btn-small btn-secondary"
                                  onClick={() => handleUpdateRole(u.id, 'editor')}>
                                  &rarr; Editor
                                </button>
                              )}
                              {u.role === 'editor' && (
                                <button className="btn btn-small btn-secondary"
                                  onClick={() => handleUpdateRole(u.id, 'viewer')}>
                                  &rarr; Viewer
                                </button>
                              )}
                              {role === 'owner' && (u.role === 'viewer' || u.role === 'editor') && (
                                <button className="btn btn-small btn-secondary"
                                  onClick={() => handlePromoteAdmin(u.id)}>
                                  &rarr; Admin
                                </button>
                              )}
                              {role === 'owner' && u.role === 'admin' && (
                                <button className="btn btn-small btn-secondary"
                                  onClick={() => handleDemoteAdmin(u.id)}>
                                  &darr; Editor
                                </button>
                              )}
                              {u.role !== 'owner' && u.status !== 'pending' && (u.role !== 'admin' || role === 'owner') && (
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
                              {u.role !== 'owner' && (u.role !== 'admin' || role === 'owner') && (
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
          )}

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
                <label>Project <span className="required">*</span></label>
                <div className={`rich-dropdown${projectOpen ? ' open' : ''}`} ref={projectRef}>
                  <button
                    type="button"
                    className="rich-dropdown-trigger"
                    onClick={() => { setProjectOpen(v => !v); setAuthMethodOpen(false); setBillingModeOpen(false) }}
                    onKeyDown={e => {
                      if (e.key === 'Escape') { setProjectOpen(false); return }
                      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
                        e.preventDefault()
                        const idx = projects.findIndex(p => p.id === newKeyProjectId)
                        const next = e.key === 'ArrowDown'
                          ? (idx + 1) % projects.length
                          : (idx - 1 + projects.length) % projects.length
                        setNewKeyProjectId(projects[next].id)
                      }
                    }}
                    aria-haspopup="listbox"
                    aria-expanded={projectOpen}
                  >
                    <span className="trigger-label">
                      {(() => {
                        const sel = projects.find(p => p.id === newKeyProjectId)
                        if (!sel) return 'Select a project...'
                        return sel.name + (sel.is_default ? ' (Default)' : '')
                      })()}
                    </span>
                    <span className="trigger-chevron">&#9660;</span>
                  </button>
                  {projectOpen && (
                    <div className="rich-dropdown-panel" role="listbox">
                      {projects.map(p => (
                        <div
                          key={p.id}
                          className={`rich-dropdown-option${newKeyProjectId === p.id ? ' selected' : ''}`}
                          role="option"
                          aria-selected={newKeyProjectId === p.id}
                          onClick={() => {
                            setNewKeyProjectId(p.id)
                            setProjectOpen(false)
                          }}
                        >
                          <span className="option-title">{p.name}{p.is_default ? ' (Default)' : ''}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </div>
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
                <select
                  value={newKeyProvider}
                  onChange={e => { setNewKeyProvider(e.target.value); setNewKeyModelAllowlist([]) }}
                >
                  <option value="anthropic">Anthropic</option>
                  <option value="openai">OpenAI</option>
                </select>
              </div>
              <div className="form-group">
                <label>Auth Method</label>
                <div className={`rich-dropdown${authMethodOpen ? ' open' : ''}`} ref={authMethodRef}>
                  <button
                    type="button"
                    className="rich-dropdown-trigger"
                    onClick={() => { setAuthMethodOpen(v => !v); setBillingModeOpen(false); setProjectOpen(false) }}
                    onKeyDown={e => {
                      if (e.key === 'Escape') { setAuthMethodOpen(false); return }
                      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
                        e.preventDefault()
                        const idx = AUTH_METHODS.findIndex(m => m.value === newAuthMethod)
                        const next = e.key === 'ArrowDown'
                          ? (idx + 1) % AUTH_METHODS.length
                          : (idx - 1 + AUTH_METHODS.length) % AUTH_METHODS.length
                        const v = AUTH_METHODS[next].value
                        setNewAuthMethod(v)
                        if (v === 'BYOK') setNewBillingMode('API_USAGE')
                      }
                    }}
                    aria-haspopup="listbox"
                    aria-expanded={authMethodOpen}
                  >
                    <span className="trigger-label">
                      {AUTH_METHODS.find(m => m.value === newAuthMethod)?.label ?? 'Select...'}
                    </span>
                    <span className="trigger-chevron">&#9660;</span>
                  </button>
                  {authMethodOpen && (
                    <div className="rich-dropdown-panel" role="listbox">
                      {AUTH_METHODS.map(m => (
                        <div
                          key={m.value}
                          className={`rich-dropdown-option${newAuthMethod === m.value ? ' selected' : ''}`}
                          role="option"
                          aria-selected={newAuthMethod === m.value}
                          onClick={() => {
                            setNewAuthMethod(m.value)
                            if (m.value === 'BYOK') setNewBillingMode('API_USAGE')
                            setAuthMethodOpen(false)
                          }}
                        >
                          <span className="option-title">{m.label}</span>
                          <span className="option-desc">{m.description}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </div>
              <div className="form-group">
                <label>Billing Mode</label>
                <div className={`rich-dropdown${billingModeOpen ? ' open' : ''}`} ref={billingModeRef}>
                  <button
                    type="button"
                    className="rich-dropdown-trigger"
                    onClick={() => { setBillingModeOpen(v => !v); setAuthMethodOpen(false); setProjectOpen(false) }}
                    onKeyDown={e => {
                      if (e.key === 'Escape') { setBillingModeOpen(false); return }
                      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
                        e.preventDefault()
                        const enabledModes = BILLING_MODES.filter(m => !(newAuthMethod === 'BYOK' && m.value === 'MONTHLY_SUBSCRIPTION'))
                        const idx = enabledModes.findIndex(m => m.value === newBillingMode)
                        const next = e.key === 'ArrowDown'
                          ? (idx + 1) % enabledModes.length
                          : (idx - 1 + enabledModes.length) % enabledModes.length
                        setNewBillingMode(enabledModes[next].value)
                      }
                    }}
                    aria-haspopup="listbox"
                    aria-expanded={billingModeOpen}
                  >
                    <span className="trigger-label">
                      {BILLING_MODES.find(m => m.value === newBillingMode)?.label ?? 'Select...'}
                    </span>
                    <span className="trigger-chevron">&#9660;</span>
                  </button>
                  {billingModeOpen && (
                    <div className="rich-dropdown-panel" role="listbox">
                      {BILLING_MODES.map(m => {
                        const disabled = newAuthMethod === 'BYOK' && m.value === 'MONTHLY_SUBSCRIPTION'
                        return (
                          <div
                            key={m.value}
                            className={`rich-dropdown-option${newBillingMode === m.value ? ' selected' : ''}${disabled ? ' disabled' : ''}`}
                            role="option"
                            aria-selected={newBillingMode === m.value}
                            aria-disabled={disabled}
                            onClick={() => {
                              if (disabled) return
                              setNewBillingMode(m.value)
                              setBillingModeOpen(false)
                            }}
                          >
                            <span className="option-title">
                              {m.label}
                              {disabled && <span style={{ fontWeight: 400, fontSize: '0.75rem', color: 'var(--color-text-muted)' }}> (not available with BYOK)</span>}
                            </span>
                            <span className="option-desc">{m.description}</span>
                          </div>
                        )
                      })}
                    </div>
                  )}
                </div>
              </div>
              <div className="form-group">
                <label>Model Allowlist <span className="optional">(optional)</span></label>
                <select
                  multiple
                  value={newKeyModelAllowlist}
                  onChange={e => {
                    const selected = Array.from(e.target.selectedOptions, o => o.value)
                    setNewKeyModelAllowlist(selected)
                  }}
                  style={{ minHeight: '120px' }}
                >
                  {availableModels.map(m => (
                    <option key={m} value={m}>{m}</option>
                  ))}
                </select>
                <p className="form-helper">
                  Hold Ctrl/Cmd to select multiple models. Leave empty to allow all models.
                </p>
                {newKeyModelAllowlist.length > 0 && (
                  <div style={{ marginTop: '0.5rem', display: 'flex', flexWrap: 'wrap', gap: '0.25rem' }}>
                    {newKeyModelAllowlist.map(m => (
                      <span key={m} className="mode-badge" style={{ display: 'inline-flex', alignItems: 'center', gap: '0.25rem' }}>
                        {m}
                        <button
                          type="button"
                          onClick={() => setNewKeyModelAllowlist(prev => prev.filter(x => x !== m))}
                          style={{ background: 'none', border: 'none', color: 'var(--color-text-muted)', cursor: 'pointer', padding: '0 2px', fontSize: '0.875rem' }}
                        >x</button>
                      </span>
                    ))}
                    <button
                      type="button"
                      className="btn btn-small btn-secondary"
                      onClick={() => setNewKeyModelAllowlist([])}
                      style={{ fontSize: '0.7rem', padding: '0.15rem 0.4rem' }}
                    >Clear all</button>
                  </div>
                )}
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
                disabled={!newKeyLabel.trim() || !newKeyProjectId}>
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

      {/* ── Invite Member Modal ─────────────────────────────────────────── */}
      {showInviteModal && (
        <div className="modal-overlay" onClick={() => { setShowInviteModal(false); setInviteError(null) }}>
          <div className="modal-box modal-md" onClick={e => e.stopPropagation()}>
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
            {inviteError && (
              <div className="flash flash-error modal-flash">{inviteError}</div>
            )}
            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => { setShowInviteModal(false); setInviteError(null) }}
                disabled={inviting}>
                Cancel
              </button>
              <button className="btn btn-primary" onClick={handleInviteUser}
                disabled={!inviteEmail.trim() || inviting}>
                {inviting ? 'Inviting\u2026' : 'Send Invite'}
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
                  : limitModal.type === 'provider_keys' ? 'Provider Key Limit Reached'
                  : limitModal.type === 'projects' ? 'Project Limit Reached'
                  : 'Member Limit Reached'}
              </h2>
            </div>
            <div className="modal-body">
              <p>
                {limitModal.type === 'keys'
                  ? `You've reached the maximum of ${keyLimit} API key${keyLimit !== 1 ? 's' : ''} on your current plan. Upgrade your plan to create more API keys.`
                  : limitModal.type === 'provider_keys'
                  ? `You've reached the maximum of ${providerKeyLimit} provider key${providerKeyLimit !== 1 ? 's' : ''} on your current plan. Upgrade your plan to add more provider keys.`
                  : limitModal.type === 'projects'
                  ? `You've reached the maximum of ${projectLimit} project${projectLimit !== 1 ? 's' : ''} on your current plan. Upgrade your plan to create more projects.`
                  : `You've reached the maximum of ${memberLimit} member${memberLimit !== 1 ? 's' : ''} on your current plan. Upgrade your plan to invite more members.`}
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

      {/* ── Create Project Modal ─────────────────────────────────────────── */}
      {showCreateProjectModal && (
        <div className="modal-overlay" onClick={() => setShowCreateProjectModal(false)}>
          <div className="modal-box modal-md" onClick={e => e.stopPropagation()}>
            <div className="modal-hdr"><h2>Create Project</h2></div>
            <div className="modal-body">
              <div className="form-group">
                <label>Name <span className="required">*</span></label>
                <input type="text" value={newProjectName} onChange={e => setNewProjectName(e.target.value)} placeholder="e.g. Backend API" autoFocus />
              </div>
              <div className="form-group">
                <label>Description <span className="optional">(optional)</span></label>
                <input type="text" value={newProjectDesc} onChange={e => setNewProjectDesc(e.target.value)} placeholder="Brief description" />
              </div>
            </div>
            {createProjectError && <div className="flash flash-error modal-flash">{createProjectError}</div>}
            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => setShowCreateProjectModal(false)} disabled={creatingProject}>Cancel</button>
              <button className="btn btn-primary" onClick={handleCreateProject} disabled={!newProjectName.trim() || creatingProject}>
                {creatingProject ? 'Creating\u2026' : 'Create'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── Edit Project Modal ───────────────────────────────────────────── */}
      {editingProject && (
        <div className="modal-overlay" onClick={() => setEditingProject(null)}>
          <div className="modal-box modal-md" onClick={e => e.stopPropagation()}>
            <div className="modal-hdr"><h2>Edit Project</h2></div>
            <div className="modal-body">
              <div className="form-group">
                <label>Name</label>
                <input type="text" value={editProjectName} onChange={e => setEditProjectName(e.target.value)} autoFocus />
              </div>
              <div className="form-group">
                <label>Description</label>
                <input type="text" value={editProjectDesc} onChange={e => setEditProjectDesc(e.target.value)} />
              </div>
            </div>
            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => setEditingProject(null)}>Cancel</button>
              <button className="btn btn-primary" onClick={handleUpdateProject} disabled={!editProjectName.trim()}>Save</button>
            </div>
          </div>
        </div>
      )}

      {/* ── Revoke Provider Key Confirm Modal ──────────────────────────────── */}
      {revokeTarget && (() => {
        const affectedCount = apiKeys.filter(k => k.auth_method === 'BYOK' && k.provider === revokeTarget.provider).length
        const providerLabel = revokeTarget.provider === 'openai' ? 'OpenAI' : 'Anthropic'
        return (
          <div className="modal-overlay" onClick={() => setRevokeTarget(null)}>
            <div className="modal-box modal-md" onClick={e => e.stopPropagation()}>
              <div className="modal-hdr">
                <h2>Revoke Provider Key</h2>
              </div>
              <div className="modal-body">
                <p>
                  {revokeTarget.is_active && affectedCount > 0
                    ? `This is the active key for ${providerLabel}. Revoking it will break ${affectedCount} BYOK API key${affectedCount !== 1 ? 's' : ''} — proxied requests will fail until a new key is activated.`
                    : revokeTarget.is_active
                    ? `This is the active key for ${providerLabel}. Revoking it will deactivate ${providerLabel} proxying.`
                    : `Revoke provider key "${revokeTarget.label}"? This cannot be undone.`}
                </p>
                <p style={{ marginTop: '1rem' }}>Confirm to revoke?</p>
              </div>
              <div className="modal-ftr">
                <button className="btn btn-secondary" onClick={() => setRevokeTarget(null)}>Cancel</button>
                <button className="btn btn-danger" onClick={confirmRevokeProviderKey}>Revoke</button>
              </div>
            </div>
          </div>
        )
      })()}
    </div>
  )
}
