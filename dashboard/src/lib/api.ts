const API_URL = import.meta.env.VITE_API_SERVER_URL || ''

/**
 * Centralized API client that includes X-User-ID and X-Tenant-Id headers
 * on every request. All hooks should use this instead of raw fetch.
 */
export async function apiFetch(path: string, options?: RequestInit): Promise<Response> {
  const userId = localStorage.getItem('user_id') || ''
  const tenantId = localStorage.getItem('active_tenant_id') || localStorage.getItem('tenant_id') || ''

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-User-ID': userId,
    ...(tenantId ? { 'X-Tenant-Id': tenantId } : {}),
  }

  // Merge caller-provided headers (allowing override).
  if (options?.headers) {
    const extra =
      options.headers instanceof Headers
        ? Object.fromEntries(options.headers.entries())
        : (options.headers as Record<string, string>)
    Object.assign(headers, extra)
  }

  return fetch(`${API_URL}${path}`, {
    ...options,
    headers,
  })
}

/**
 * Legacy helper for components that haven't migrated yet.
 * Returns the standard auth headers object.
 */
export function authHeaders(): Record<string, string> {
  const userId = localStorage.getItem('user_id') || ''
  const tenantId = localStorage.getItem('active_tenant_id') || localStorage.getItem('tenant_id') || ''
  return {
    'Content-Type': 'application/json',
    'X-User-ID': userId,
    ...(tenantId ? { 'X-Tenant-Id': tenantId } : {}),
  }
}
