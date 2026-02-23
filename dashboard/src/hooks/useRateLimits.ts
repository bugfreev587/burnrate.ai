import { useState, useEffect, useCallback } from 'react'

const API_URL = import.meta.env.VITE_API_SERVER_URL || ''

function authHeaders(): Record<string, string> {
  const userId = localStorage.getItem('user_id') || ''
  return { 'Content-Type': 'application/json', 'X-User-ID': userId }
}

export interface RateLimit {
  ID: number
  TenantID: number
  Provider: string
  Model: string
  ScopeType: string
  ScopeID: string
  Metric: string
  LimitValue: number
  WindowSeconds: number
  Enabled: boolean
  CreatedAt: string
  UpdatedAt: string
  current_usage: number
}

export interface UpsertRateLimitReq {
  provider: string
  model: string
  scope_type: string
  scope_id: string
  metric: string
  limit_value: number
  window_seconds: number
  enabled: boolean
}

export function useRateLimits() {
  const [limits, setLimits] = useState<RateLimit[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchLimits = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetch(`${API_URL}/v1/admin/rate-limits`, { headers: authHeaders() })
      if (!res.ok) throw new Error('Failed to fetch rate limits')
      const data = await res.json()
      setLimits(data.rate_limits || [])
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchLimits() }, [fetchLimits])

  async function upsertLimit(req: UpsertRateLimitReq): Promise<RateLimit> {
    const res = await fetch(`${API_URL}/v1/admin/rate-limits`, {
      method: 'PUT',
      headers: authHeaders(),
      body: JSON.stringify(req),
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to save rate limit')
    }
    const saved: RateLimit = await res.json()
    await fetchLimits()
    return saved
  }

  async function deleteLimit(id: number): Promise<void> {
    const res = await fetch(`${API_URL}/v1/admin/rate-limits/${id}`, {
      method: 'DELETE',
      headers: authHeaders(),
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to delete rate limit')
    }
    setLimits(prev => prev.filter(l => l.ID !== id))
  }

  return { limits, loading, error, refresh: fetchLimits, upsertLimit, deleteLimit }
}
