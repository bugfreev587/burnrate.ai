import { useState, useRef, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { useUser, useAuth, SignOutButton } from '@clerk/clerk-react'
import { useUserSync, hasPermission } from '../hooks/useUserSync'
import './Navbar.css'

export default function Navbar() {
  const { user, isLoaded: userLoaded } = useUser()
  const { isLoaded: authLoaded, isSignedIn } = useAuth()
  const { role, isSynced } = useUserSync()
  const [showMenu, setShowMenu] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  const canAccessAdmin = isSynced && hasPermission(role, 'admin')

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setShowMenu(false)
      }
    }
    if (showMenu) document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [showMenu])

  const isLoaded = userLoaded && authLoaded

  return (
    <nav className="navbar">
      <div className="navbar-container">
        <Link to="/" className="navbar-logo">
          BurnRate AI
        </Link>

        <div className="navbar-right">
          {!isLoaded ? (
            <span className="navbar-text-muted">Loading...</span>
          ) : isSignedIn ? (
            <>
              <Link to="/dashboard" className="navbar-link">Dashboard</Link>
              <div className="user-menu" ref={menuRef}>
                <button
                  className="user-avatar-btn"
                  onClick={() => setShowMenu(v => !v)}
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
                      <Link to="/management" className="dropdown-item" onClick={() => setShowMenu(false)}>
                        Management
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
    </nav>
  )
}
