import { useState, useEffect, useCallback, useRef } from 'react'
import { apiFetch } from '../lib/api'

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

export function useRateLimits(pollInterval?: number) {
  const [limits, setLimits] = useState<RateLimit[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const fetchLimits = useCallback(async (silent = false) => {
    if (!silent) { setLoading(true); setError(null) }
    try {
      const res = await apiFetch('/v1/admin/rate-limits')
      if (!res.ok) throw new Error('Failed to fetch rate limits')
      const data = await res.json()
      setLimits(data.rate_limits || [])
      if (silent) setError(null)
    } catch (e: unknown) {
      if (!silent) setError(e instanceof Error ? e.message : 'Unknown error')
    } finally {
      if (!silent) setLoading(false)
    }
  }, [])

  useEffect(() => { fetchLimits() }, [fetchLimits])

  useEffect(() => {
    if (pollInterval && pollInterval > 0) {
      pollRef.current = setInterval(() => fetchLimits(true), pollInterval)
    }
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
      pollRef.current = null
    }
  }, [pollInterval, fetchLimits])

  async function upsertLimit(req: UpsertRateLimitReq): Promise<RateLimit> {
    const res = await apiFetch('/v1/admin/rate-limits', {
      method: 'PUT',
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
    const res = await apiFetch(`/v1/admin/rate-limits/${id}`, {
      method: 'DELETE',
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to delete rate limit')
    }
    setLimits(prev => prev.filter(l => l.ID !== id))
  }

  return { limits, loading, error, refresh: () => fetchLimits(), upsertLimit, deleteLimit }
}
