import { useState, useRef, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { useUser, useAuth, SignOutButton } from '@clerk/clerk-react'
import { hasPermission, type UserRole } from '../hooks/useUserSync'
import { useTenant } from '../contexts/TenantContext'
import { useUserNotifications, type UserNotification } from '../hooks/useUserNotifications'
import { apiFetch } from '../lib/api'
import logoDark from '../assets/logo-dark.svg'
import './Navbar.css'

interface OnboardingHints {
  dismissed_integration_hint: boolean
  dismissed_avatar_hint: boolean
}

export default function Navbar() {
  const { user, isLoaded: userLoaded } = useUser()
  const { isLoaded: authLoaded, isSignedIn } = useAuth()
  const { memberships, activeTenantId, switchTenant, orgRole } = useTenant()
  const [showMenu, setShowMenu] = useState(false)
  const [showNotifMenu, setShowNotifMenu] = useState(false)
  const [selectedNotif, setSelectedNotif] = useState<UserNotification | null>(null)
  const [notifError, setNotifError] = useState<string | null>(null)
  const [hints, setHints] = useState<OnboardingHints | null>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const notifRef = useRef<HTMLDivElement>(null)

  // Derive sync status from TenantContext: if memberships are loaded, sync is done.
  // This avoids running a duplicate useUserSync() instance with its own isSynced state.
  const isSynced = memberships.length > 0
  const role = (orgRole as UserRole) ?? null
  const canAccessEditor = isSynced && hasPermission(role, 'editor')
  const canAccessAdmin = isSynced && hasPermission(role, 'admin')
  const isLoaded = userLoaded && authLoaded
  const notificationsEnabled = isLoaded && isSignedIn && isSynced
  const {
    notifications,
    unreadCount,
    markRead,
    markAllRead,
    acceptInvitation,
    denyInvitation,
  } = useUserNotifications(notificationsEnabled)

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setShowMenu(false)
      }
      if (notifRef.current && !notifRef.current.contains(e.target as Node)) {
        setShowNotifMenu(false)
      }
    }
    if (showMenu || showNotifMenu) document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [showMenu, showNotifMenu])

  useEffect(() => {
    if (!isLoaded || !isSignedIn) {
      setHints(null)
      return
    }
    if (!isSynced) return
    let cancelled = false
    const loadHints = async () => {
      try {
        const res = await apiFetch('/v1/user/onboarding-hints')
        if (!res.ok) return
        const data = await res.json()
        if (!cancelled) {
          setHints({
            dismissed_integration_hint: !!data.dismissed_integration_hint,
            dismissed_avatar_hint: !!data.dismissed_avatar_hint,
          })
        }
      } catch {
        // Non-critical onboarding UI; fail silently.
      }
    }
    loadHints()
    return () => { cancelled = true }
  }, [isLoaded, isSignedIn, isSynced])

  const dismissHint = async (key: 'dismissed_integration_hint' | 'dismissed_avatar_hint') => {
    if (!hints || hints[key]) return
    const optimistic = { ...hints, [key]: true }
    setHints(optimistic)
    try {
      const res = await apiFetch('/v1/user/onboarding-hints', {
        method: 'PATCH',
        body: JSON.stringify({ [key]: true }),
      })
      if (!res.ok) throw new Error('failed to persist hint preference')
      const data = await res.json().catch(() => null)
      if (data) {
        setHints({
          dismissed_integration_hint: !!data.dismissed_integration_hint,
          dismissed_avatar_hint: !!data.dismissed_avatar_hint,
        })
      }
    } catch {
      setHints(prev => prev ? { ...prev, [key]: false } : prev)
    }
  }

  const showIntegrationHint = isLoaded && isSignedIn && isSynced && !!hints && !hints.dismissed_integration_hint
  const showAvatarHint = isLoaded && isSignedIn && isSynced && !!hints && !hints.dismissed_avatar_hint
  const visibleNotifications = notifications.slice(0, 8)

  function parseInvitePayload(n: UserNotification): { tenant_id?: number; tenant_name?: string; invitation_status?: string } {
    try {
      const p = JSON.parse(n.payload || '{}')
      return { tenant_id: p.tenant_id, tenant_name: p.tenant_name, invitation_status: p.invitation_status }
    } catch {
      return {}
    }
  }

  const handleSelectNotification = async (n: UserNotification) => {
    setSelectedNotif(n)
    setNotifError(null)
    if (n.status === 'unread') {
      await markRead(n.id)
    }
  }

  const handleAccept = async () => {
    if (!selectedNotif) return
    const payload = parseInvitePayload(selectedNotif)
    if (!payload.tenant_id) {
      setNotifError('Missing tenant_id in invitation payload')
      return
    }
    try {
      await acceptInvitation(payload.tenant_id)
      setSelectedNotif(null)
      setNotifError(null)
    } catch (err) {
      setNotifError(err instanceof Error ? err.message : 'Failed to accept invitation')
    }
  }

  const handleDeny = async () => {
    if (!selectedNotif) return
    const payload = parseInvitePayload(selectedNotif)
    if (!payload.tenant_id) {
      setNotifError('Missing tenant_id in invitation payload')
      return
    }
    try {
      await denyInvitation(payload.tenant_id)
      setSelectedNotif(null)
      setNotifError(null)
    } catch (err) {
      setNotifError(err instanceof Error ? err.message : 'Failed to deny invitation')
    }
  }

  const selectedInviteStatus = selectedNotif ? (parseInvitePayload(selectedNotif).invitation_status || 'pending') : 'pending'

  return (
    <nav className="navbar">
      <div className="navbar-container">
        <Link to="/" className="navbar-logo">
          <img src={logoDark} alt="" className="navbar-logo-icon" aria-hidden="true" />
          TokenGate
        </Link>

        {isLoaded && isSignedIn && memberships.length > 1 && (
          <select
            className="tenant-switcher"
            value={activeTenantId ?? ''}
            onChange={e => {
              const id = parseInt(e.target.value, 10)
              if (!isNaN(id)) {
                switchTenant(id)
                window.location.reload()
              }
            }}
          >
            {memberships.map(m => (
              <option key={m.tenant_id} value={m.tenant_id}>
                {m.tenant_name} ({m.org_role})
              </option>
            ))}
          </select>
        )}

        {isLoaded && isSignedIn && (
          <div className="navbar-center">
            <Link to="/dashboard" className="navbar-link">Dashboard</Link>
            {canAccessEditor && <Link to="/management" className="navbar-link">Management</Link>}
            {canAccessEditor && <Link to="/limits" className="navbar-link">Limits</Link>}
            <Link to="/notifications" className="navbar-link">Notifications</Link>
            {canAccessEditor && <Link to="/pricing" className="navbar-link">Pricing Config</Link>}
            <div className="hint-anchor">
              <Link to="/integration" className={`navbar-link${showIntegrationHint ? ' hint-pulse-target' : ''}`}>Integration</Link>
              {showIntegrationHint && (
                <div className="onboarding-hint onboarding-hint-integration" role="status" aria-live="polite">
                  <p className="onboarding-hint-title">Get started here</p>
                  <p className="onboarding-hint-body">
                    Integrate your AI coding tool with TokenGate to enable auditing and governance.
                  </p>
                  <button className="onboarding-hint-dismiss" onClick={() => dismissHint('dismissed_integration_hint')}>
                    Don&apos;t show again
                  </button>
                </div>
              )}
            </div>
            <Link to="/audit" className="navbar-link">Audit</Link>
          </div>
        )}

        <div className="navbar-right">
          {!isLoaded ? (
            <span className="navbar-text-muted">Loading...</span>
          ) : isSignedIn ? (
            <>
              <div className="notif-menu" ref={notifRef}>
                <button
                  className={`notif-bell-btn${unreadCount > 0 ? ' unread' : ''}`}
                  onClick={() => { setShowNotifMenu(v => !v); setShowMenu(false) }}
                  aria-label="Notifications"
                >
                  <span aria-hidden="true">&#128276;</span>
                  {unreadCount > 0 && <span className="notif-count">{unreadCount > 99 ? '99+' : unreadCount}</span>}
                </button>
                {showNotifMenu && (
                  <div className="notif-dropdown">
                    <div className="notif-header">
                      <strong>Notifications</strong>
                      {unreadCount > 0 && (
                        <button className="notif-mark-all" onClick={() => markAllRead()}>Mark all read</button>
                      )}
                    </div>
                    <div className="notif-list">
                      {visibleNotifications.length === 0 ? (
                        <div className="notif-empty">No notifications yet.</div>
                      ) : visibleNotifications.map(n => (
                        <button
                          key={n.id}
                          className={`notif-item${n.status === 'unread' ? ' unread' : ''}`}
                          onClick={() => handleSelectNotification(n)}
                        >
                          <div className="notif-item-title">{n.title}</div>
                          <div className="notif-item-body">{n.body || n.type}</div>
                          <div className="notif-item-time">{new Date(n.created_at).toLocaleString()}</div>
                        </button>
                      ))}
                    </div>
                    <div className="notif-footer">
                      <Link to="/notifications" className="notif-link" onClick={() => setShowNotifMenu(false)}>Notification settings</Link>
                    </div>
                  </div>
                )}
              </div>
              <div className="user-menu" ref={menuRef}>
                <button
                  className={`user-avatar-btn${showAvatarHint ? ' hint-pulse-target' : ''}`}
                  onClick={() => { setShowMenu(v => !v); setShowNotifMenu(false) }}
                  aria-label="User menu"
                >
                  {user?.imageUrl ? (
                    <img src={user.imageUrl} alt="avatar" className="avatar-img" />
                  ) : (
                    <div className="avatar-placeholder">
                      {user?.firstName?.[0] ?? user?.emailAddresses[0]?.emailAddress?.[0] ?? 'U'}
                    </div>
                  )}
                </button>
                {showAvatarHint && (
                  <div className="onboarding-hint onboarding-hint-avatar" role="status" aria-live="polite">
                    <p className="onboarding-hint-title">More options here</p>
                    <p className="onboarding-hint-body">Access billing, settings, and account management.</p>
                    <button className="onboarding-hint-dismiss" onClick={() => dismissHint('dismissed_avatar_hint')}>
                      Don&apos;t show again
                    </button>
                  </div>
                )}
                {showMenu && (
                  <div className="dropdown">
                    <div className="dropdown-header">
                      <p className="dropdown-name">
                        {user?.firstName && user?.lastName
                          ? `${user.firstName} ${user.lastName}`
                          : user?.firstName ?? user?.emailAddresses[0]?.emailAddress ?? 'User'}
                      </p>
                      <p className="dropdown-email">{user?.emailAddresses[0]?.emailAddress}</p>
                    </div>
                    <div className="dropdown-divider" />
                    <Link to="/profile" className="dropdown-item" onClick={() => setShowMenu(false)}>
                      Profile
                    </Link>
                    {canAccessAdmin && (
                      <Link to="/plan" className="dropdown-item" onClick={() => setShowMenu(false)}>
                        Plans
                      </Link>
                    )}
                    {canAccessAdmin && (
                      <Link to="/billing" className="dropdown-item" onClick={() => setShowMenu(false)}>
                        Billing
                      </Link>
                    )}
                    {canAccessAdmin && (
                      <Link to="/settings" className="dropdown-item" onClick={() => setShowMenu(false)}>
                        Settings
                      </Link>
                    )}
                    <div className="dropdown-divider" />
                    <SignOutButton>
                      <button className="dropdown-item dropdown-signout">Sign Out</button>
                    </SignOutButton>
                  </div>
                )}
              </div>
            </>
          ) : (
            <>
              <Link to="/sign-in" className="navbar-link">Sign In</Link>
              <Link to="/sign-up" className="btn btn-primary">Sign Up</Link>
            </>
          )}
        </div>
      </div>
      {selectedNotif && selectedNotif.type === 'team_invitation' && (
        <div className="invite-action-bar">
          <div className="invite-action-content">
            <div>
              <div className="invite-action-title">{selectedNotif.title}</div>
              <div className="invite-action-body">
                {selectedInviteStatus === 'accepted'
                  ? 'This invitation has been accepted.'
                  : selectedInviteStatus === 'denied'
                  ? 'This invitation has been denied.'
                  : (selectedNotif.body || 'Choose Accept, Deny, or Decide later.')}
              </div>
              {notifError && <div className="invite-action-error">{notifError}</div>}
            </div>
            <div className="invite-action-buttons">
              {selectedInviteStatus === 'pending' ? (
                <>
                  <button className="btn btn-primary" onClick={handleAccept}>Accept</button>
                  <button className="btn btn-danger" onClick={handleDeny}>Deny</button>
                  <button className="btn btn-secondary" onClick={() => setSelectedNotif(null)}>Decide later</button>
                </>
              ) : (
                <button className="btn btn-secondary" onClick={() => setSelectedNotif(null)}>Close</button>
              )}
            </div>
          </div>
        </div>
      )}
    </nav>
  )
}
