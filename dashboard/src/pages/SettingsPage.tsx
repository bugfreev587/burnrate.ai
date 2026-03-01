import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { useClerk } from '@clerk/clerk-react'
import { useUserSync, hasPermission } from '../hooks/useUserSync'
import type { UserRole } from '../hooks/useUserSync'
import { useProjects, type Project, type ProjectMember } from '../hooks/useProjects'
import { apiFetch } from '../lib/api'
import Navbar from '../components/Navbar'
import './SettingsPage.css'
import './ManagementPage.css'

interface User {
  id: string
  email: string
  name: string
  role: UserRole
  status: string
  created_at: string
}

export default function SettingsPage() {
  const navigate = useNavigate()
  const { signOut } = useClerk()
  const { role, userId, isSynced } = useUserSync()

  const isAdmin = hasPermission(role, 'admin')
  const isOwner = role === 'owner'
  const canManageTeam = isAdmin

  const { projects, loading: projectsLoading, limit: projectLimit, slotsLeft: projectSlotsLeft, refetch: refetchProjects, createProject, updateProject, deleteProject, listMembers, addMember, updateMemberRole, removeMember } = useProjects()

  // ── Workspace name ──────────────────────────────────────────────────────
  const [workspaceName, setWorkspaceName] = useState('')
  const [originalName, setOriginalName] = useState('')
  const [savingName, setSavingName] = useState(false)

  // ── Team members ────────────────────────────────────────────────────────
  const [users, setUsers] = useState<User[]>([])
  const [memberLimit, setMemberLimit] = useState<number | null>(null)

  // Invite modal
  const [showInviteModal, setShowInviteModal] = useState(false)
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteName, setInviteName] = useState('')
  const [inviteRole, setInviteRole] = useState<'viewer' | 'editor'>('viewer')
  const [inviting, setInviting] = useState(false)
  const [inviteError, setInviteError] = useState<string | null>(null)

  // Limit modal
  const [showLimitModal, setShowLimitModal] = useState(false)

  // ── Projects ──────────────────────────────────────────────────────────
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

  // ── Danger zone ─────────────────────────────────────────────────────────
  const [confirmName, setConfirmName] = useState('')
  const [deleting, setDeleting] = useState(false)

  // ── Flash messages ──────────────────────────────────────────────────────
  const [successMsg, setSuccessMsg] = useState<string | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const showSuccess = (msg: string) => {
    setSuccessMsg(msg)
    setTimeout(() => setSuccessMsg(null), 3000)
  }
  const showError = (msg: string) => {
    setErrorMsg(msg)
    setTimeout(() => setErrorMsg(null), 5000)
  }

  // ── Fetch workspace name ───────────────────────────────────────────────
  const fetchSettings = useCallback(async () => {
    try {
      const res = await apiFetch('/v1/owner/settings')
      if (res.ok) {
        const data = await res.json()
        setWorkspaceName(data.name ?? '')
        setOriginalName(data.name ?? '')
      }
    } catch (err) {
      console.error('fetch settings:', err)
    }
  }, [])

  // ── Fetch team members ─────────────────────────────────────────────────
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
    const load = async () => {
      setLoading(true)
      const fetches: Promise<void>[] = []
      if (isOwner) fetches.push(fetchSettings())
      if (canManageTeam) fetches.push(fetchUsers())
      await Promise.all(fetches)
      setLoading(false)
    }
    load()
  }, [isSynced, isOwner, canManageTeam, fetchSettings, fetchUsers])

  // ── Save workspace name ────────────────────────────────────────────────
  const handleSaveName = async () => {
    if (!workspaceName.trim() || workspaceName === originalName) return
    setSavingName(true)
    try {
      const res = await apiFetch('/v1/owner/settings', {
        method: 'PATCH',
        body: JSON.stringify({ name: workspaceName.trim() }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.message ?? d.error ?? 'Failed to save name')
      }
      const data = await res.json()
      setOriginalName(data.name)
      setWorkspaceName(data.name)
      showSuccess('Workspace name updated')
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to save name')
    } finally {
      setSavingName(false)
    }
  }

  // ── Invite user ────────────────────────────────────────────────────────
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

  // ── User role actions ──────────────────────────────────────────────────
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

  // ── Delete account ─────────────────────────────────────────────────────
  const handleDeleteAccount = async () => {
    if (confirmName !== originalName) return
    if (!confirm('This action is irreversible. Are you absolutely sure?')) return
    setDeleting(true)
    try {
      const res = await apiFetch('/v1/owner/account', {
        method: 'DELETE',
        body: JSON.stringify({ confirm_name: confirmName }),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.message ?? d.error ?? 'Failed to delete account')
      }
      await signOut()
      navigate('/')
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to delete account')
    } finally {
      setDeleting(false)
    }
  }

  // ── Project handlers ──────────────────────────────────────────────────
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

  const handleRemoveProjectMember = async (projectId: number, userId: string) => {
    if (!confirm('Remove this member from the project?')) return
    try {
      await removeMember(projectId, userId)
      const members = await listMembers(projectId)
      setProjectMembers(members)
      showSuccess('Member removed from project')
    } catch (err) {
      showError(err instanceof Error ? err.message : 'Failed to remove member')
    }
  }

  // ── Redirect non-admin users ──────────────────────────────────────────
  if (isSynced && !isAdmin) {
    navigate('/dashboard', { replace: true })
    return null
  }

  // ── Loading ────────────────────────────────────────────────────────────
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
        <div className="settings-container">

          {/* Header */}
          <div className="mgmt-header">
            <h1>Settings</h1>
          </div>

          {/* Flash messages */}
          {successMsg && <div className="flash flash-success">{successMsg}</div>}
          {errorMsg   && <div className="flash flash-error">{errorMsg}</div>}

          {/* ── Workspace Name (owner only) ────────────────────────────── */}
          {isOwner && (
            <section className="settings-section">
              <div className="section-hdr">
                <div>
                  <h2>Workspace</h2>
                  <p className="section-desc">Your workspace name is visible to all team members.</p>
                </div>
              </div>
              <div className="settings-name-form">
                <input
                  type="text"
                  value={workspaceName}
                  onChange={e => setWorkspaceName(e.target.value)}
                  placeholder="Workspace name"
                  maxLength={100}
                />
                <button
                  className="btn btn-primary"
                  onClick={handleSaveName}
                  disabled={savingName || !workspaceName.trim() || workspaceName === originalName}
                >
                  {savingName ? 'Saving...' : 'Save'}
                </button>
              </div>
            </section>
          )}

          {/* ── Team Members (editor+ only) ────────────────────────────── */}
          {canManageTeam && (
            <section className="settings-section">
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
                    setShowLimitModal(true)
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
                              {isOwner && (u.role === 'viewer' || u.role === 'editor') && (
                                <button className="btn btn-small btn-secondary"
                                  onClick={() => handlePromoteAdmin(u.id)}>
                                  &rarr; Admin
                                </button>
                              )}
                              {isOwner && u.role === 'admin' && (
                                <button className="btn btn-small btn-secondary"
                                  onClick={() => handleDemoteAdmin(u.id)}>
                                  &darr; Editor
                                </button>
                              )}
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
          )}

          {/* ── Projects (admin only) ──────────────────────────────────── */}
          {isAdmin && (
            <section className="settings-section">
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
                    showError('Project limit reached. Upgrade your plan to create more projects.')
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

          {/* ── Danger Zone (owner only) ───────────────────────────────── */}
          {isOwner && (
            <section className="settings-section settings-section--danger">
              <div className="section-hdr">
                <div>
                  <h2>Danger Zone</h2>
                  <p className="section-desc">Irreversible actions that permanently affect your account.</p>
                </div>
              </div>
              <p className="danger-zone-warning">
                Deleting your workspace will immediately cancel your subscription,
                revoke all API keys and provider keys, remove all team members,
                and permanently disable this account. This action cannot be undone.
              </p>
              <p className="danger-confirm-text">
                Type <strong>{originalName}</strong> to confirm:
              </p>
              <input
                type="text"
                className="danger-confirm-input"
                value={confirmName}
                onChange={e => setConfirmName(e.target.value)}
                placeholder="Workspace name"
              />
              <button
                className="btn btn-danger"
                onClick={handleDeleteAccount}
                disabled={confirmName !== originalName || deleting}
              >
                {deleting ? 'Deleting...' : 'Delete this workspace'}
              </button>
            </section>
          )}

        </div>
      </div>

      {/* ── Invite Member Modal ──────────────────────────────────────────── */}
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

      {/* ── Member Limit Reached Modal ───────────────────────────────────── */}
      {showLimitModal && (
        <div className="modal-overlay" onClick={() => setShowLimitModal(false)}>
          <div className="modal-box modal-md" onClick={e => e.stopPropagation()}>
            <div className="modal-hdr">
              <h2>Member Limit Reached</h2>
            </div>
            <div className="modal-body">
              <p>
                You've reached the maximum of {memberLimit} member{memberLimit !== 1 ? 's' : ''} on your current plan.
                Upgrade your plan to invite more members.
              </p>
            </div>
            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => setShowLimitModal(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={() => navigate('/plan')}>Go to Plan</button>
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
    </div>
  )
}
