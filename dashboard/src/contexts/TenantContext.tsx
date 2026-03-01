import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from 'react'

export interface TenantMembership {
  tenant_id: number
  tenant_name: string
  org_role: 'owner' | 'admin' | 'editor' | 'viewer'
}

interface TenantContextValue {
  activeTenantId: number | null
  memberships: TenantMembership[]
  orgRole: string | null
  switchTenant: (tenantId: number) => void
  setMemberships: (memberships: TenantMembership[]) => void
}

const TenantContext = createContext<TenantContextValue>({
  activeTenantId: null,
  memberships: [],
  orgRole: null,
  switchTenant: () => {},
  setMemberships: () => {},
})

export function useTenant() {
  return useContext(TenantContext)
}

export function TenantProvider({ children }: { children: ReactNode }) {
  const [memberships, setMembershipsState] = useState<TenantMembership[]>([])
  const [activeTenantId, setActiveTenantId] = useState<number | null>(() => {
    const stored = localStorage.getItem('active_tenant_id')
    return stored ? parseInt(stored, 10) : null
  })

  const activeMembership = memberships.find(m => m.tenant_id === activeTenantId)
  const orgRole = activeMembership?.org_role ?? null

  const switchTenant = useCallback((tenantId: number) => {
    setActiveTenantId(tenantId)
    localStorage.setItem('active_tenant_id', String(tenantId))
    // Also update legacy key for backward compat.
    localStorage.setItem('tenant_id', String(tenantId))
    const m = memberships.find(ms => ms.tenant_id === tenantId)
    if (m) {
      localStorage.setItem('user_role', m.org_role)
    }
  }, [memberships])

  const setMemberships = useCallback((ms: TenantMembership[]) => {
    setMembershipsState(ms)
    // Auto-select active tenant if not set or not in new list.
    const stored = localStorage.getItem('active_tenant_id')
    const storedId = stored ? parseInt(stored, 10) : null
    if (ms.length > 0) {
      const found = ms.find(m => m.tenant_id === storedId)
      if (found) {
        setActiveTenantId(found.tenant_id)
        localStorage.setItem('user_role', found.org_role)
      } else {
        // Default to first membership.
        const first = ms[0]
        setActiveTenantId(first.tenant_id)
        localStorage.setItem('active_tenant_id', String(first.tenant_id))
        localStorage.setItem('tenant_id', String(first.tenant_id))
        localStorage.setItem('user_role', first.org_role)
      }
    }
  }, [])

  // Keep localStorage in sync when activeTenantId changes.
  useEffect(() => {
    if (activeTenantId != null) {
      localStorage.setItem('active_tenant_id', String(activeTenantId))
      localStorage.setItem('tenant_id', String(activeTenantId))
    }
  }, [activeTenantId])

  return (
    <TenantContext.Provider value={{ activeTenantId, memberships, orgRole, switchTenant, setMemberships }}>
      {children}
    </TenantContext.Provider>
  )
}
