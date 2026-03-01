import { useEffect, useState, useRef } from 'react'
import { useUser, useClerk } from '@clerk/clerk-react'
import { useTenant, type TenantMembership } from '../contexts/TenantContext'

const API_SERVER_URL = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

export type UserRole = 'owner' | 'admin' | 'editor' | 'viewer'
export type UserStatus = 'active' | 'suspended' | 'pending'

interface UserSyncState {
  isSynced: boolean
  isSyncing: boolean
  error: string | null
  userId: string | null   // Clerk user ID
  tenantId: number | null
  role: UserRole | null
  status: UserStatus | null
  isNewUser: boolean
  apiKey: string | null   // Only set for new users on first sync
  memberships: TenantMembership[]
}

export function hasPermission(userRole: UserRole | null, requiredRole: UserRole): boolean {
  if (!userRole) return false
  const levels: Record<UserRole, number> = { owner: 4, admin: 3, editor: 2, viewer: 1 }
  return levels[userRole] >= levels[requiredRole]
}

export function useUserSync(): UserSyncState {
  const { isSignedIn, isLoaded, user } = useUser()
  const { signOut } = useClerk()
  const { activeTenantId, orgRole, setMemberships } = useTenant()
  const syncAttempted = useRef(false)

  const [state, setState] = useState<UserSyncState>({
    isSynced: false,
    isSyncing: false,
    error: null,
    userId: null,
    tenantId: null,
    role: null,
    status: null,
    isNewUser: false,
    apiKey: null,
    memberships: [],
  })

  useEffect(() => {
    // Reset on sign-out
    if (isLoaded && !isSignedIn) {
      setState({
        isSynced: false,
        isSyncing: false,
        error: null,
        userId: null,
        tenantId: null,
        role: null,
        status: null,
        isNewUser: false,
        apiKey: null,
        memberships: [],
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

        // Handle new memberships[] response from multi-tenant auth sync.
        const memberships: TenantMembership[] = data.memberships ?? []
        setMemberships(memberships)

        // Determine active tenant + role from TenantContext (it auto-selects).
        const firstMembership = memberships[0]
        const tenantId = firstMembership?.tenant_id ?? null
        const role = firstMembership?.org_role as UserRole ?? null

        setState({
          isSynced: true,
          isSyncing: false,
          error: null,
          userId: data.user_id,
          tenantId,
          role,
          status: data.status as UserStatus,
          isNewUser: data.is_new_user ?? false,
          apiKey: data.api_key ?? null,
          memberships,
        })

        localStorage.setItem('user_id', String(data.user_id))
        localStorage.setItem('user_status', data.status ?? '')
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

  // Keep role in sync with active tenant from TenantContext.
  const effectiveRole = (orgRole as UserRole) ?? state.role

  return {
    ...state,
    tenantId: activeTenantId,
    role: effectiveRole,
  }
}
