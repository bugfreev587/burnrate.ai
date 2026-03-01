import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { useClerk } from '@clerk/clerk-react'
import { hasPermission, type UserRole } from '../hooks/useUserSync'
import { useTenant } from '../contexts/TenantContext'
import { apiFetch } from '../lib/api'
import Navbar from '../components/Navbar'
import './SettingsPage.css'

export default function SettingsPage() {
  const navigate = useNavigate()
  const { signOut } = useClerk()
  const { orgRole, isSynced } = useTenant()
  const role = (orgRole as UserRole) ?? null

  const isAdmin = hasPermission(role, 'admin')
  const isOwner = role === 'owner'
  // ── Workspace name ──────────────────────────────────────────────────────
  const [workspaceName, setWorkspaceName] = useState('')
  const [originalName, setOriginalName] = useState('')
  const [savingName, setSavingName] = useState(false)

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

  useEffect(() => {
    if (!isSynced) return
    const load = async () => {
      setLoading(true)
      const fetches: Promise<void>[] = []
      if (isOwner) fetches.push(fetchSettings())
      await Promise.all(fetches)
      setLoading(false)
    }
    load()
  }, [isSynced, isOwner, fetchSettings])

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

    </div>
  )
}
