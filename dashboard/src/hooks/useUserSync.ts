import { useEffect, useState, useRef } from 'react'
import { useUser, useClerk } from '@clerk/clerk-react'
import { useTenant, type TenantMembership } from '../contexts/TenantContext'

const API_SERVER_URL = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

export type UserRole = 'owner' | 'admin' | 'editor' | 'viewer'
export type UserStatus = 'active' | 'suspended' | 'pending'

interface UserSyncState {
  isSyncing: boolean
  error: string | null
  isNewUser: boolean
  apiKey: string | null   // Only set for new users on first sync
}

export function hasPermission(userRole: UserRole | null, requiredRole: UserRole): boolean {
  if (!userRole) return false
  const levels: Record<UserRole, number> = { owner: 4, admin: 3, editor: 2, viewer: 1 }
  return levels[userRole] >= levels[requiredRole]
}

/**
 * useUserSync handles the one-time auth sync with the backend.
 * It should only be called ONCE (in UserSyncProvider). All other components
 * should read isSynced, userId, orgRole from useTenant() directly.
 */
export function useUserSync() {
  const { isSignedIn, isLoaded, user } = useUser()
  const { signOut } = useClerk()
  const { setMemberships, setUserId, isSynced, orgRole, userId, activeTenantId } = useTenant()
  const syncAttempted = useRef(false)

  const [state, setState] = useState<UserSyncState>({
    isSyncing: false,
    error: null,
    isNewUser: false,
    apiKey: null,
  })

  useEffect(() => {
    // Reset on sign-out
    if (isLoaded && !isSignedIn) {
      setState({
        isSyncing: false,
        error: null,
        isNewUser: false,
        apiKey: null,
      })
      localStorage.removeItem('user_id')
      localStorage.removeItem('tenant_id')
      localStorage.removeItem('active_tenant_id')
      localStorage.removeItem('user_role')
      localStorage.removeItem('user_status')
      syncAttempted.current = false
      return
    }

    if (!isLoaded || !isSignedIn || !user || syncAttempted.current) return

    const syncUser = async () => {
      syncAttempted.current = true
      setState(prev => ({ ...prev, isSyncing: true, error: null }))

      try {
        const email = user.primaryEmailAddress?.emailAddress
        if (!email) throw new Error('No email address found')

        const res = await fetch(`${API_SERVER_URL}/v1/auth/sync`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            clerk_user_id: user.id,
            email,
            first_name: user.firstName ?? '',
            last_name: user.lastName ?? '',
          }),
        })

        if (!res.ok) {
          const errData = await res.json().catch(() => ({}))
          if (errData.error === 'user_suspended') {
            localStorage.clear()
            await signOut()
            throw new Error('Your account has been suspended. Contact your administrator.')
          }
          throw new Error(errData.message || errData.error || `HTTP ${res.status}`)
        }

        const data = await res.json()

        // Store userId in TenantContext (shared state).
        setUserId(data.user_id)
        localStorage.setItem('user_status', data.status ?? '')

        // Store memberships in TenantContext — this also sets activeTenantId and orgRole.
        const memberships: TenantMembership[] = data.memberships ?? []
        setMemberships(memberships)

        setState({
          isSyncing: false,
          error: null,
          isNewUser: data.is_new_user ?? false,
          apiKey: data.api_key ?? null,
        })
      } catch (err) {
        console.error('User sync failed:', err)
        setState(prev => ({
          ...prev,
          isSyncing: false,
          error: err instanceof Error ? err.message : 'Failed to sync user',
        }))
      }
    }

    syncUser()
  }, [isLoaded, isSignedIn, user])

  return {
    ...state,
    // These come from TenantContext (single source of truth).
    isSynced,
    userId,
    tenantId: activeTenantId,
    role: (orgRole as UserRole) ?? null,
    status: (localStorage.getItem('user_status') as UserStatus) ?? null,
    memberships: [],  // Deprecated: use useTenant().memberships instead
  }
}
