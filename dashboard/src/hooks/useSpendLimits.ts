import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../lib/api'

export interface SpendLimit {
  id: number
  scope_type: string      // "account" | "api_key"
  scope_id: string
  key_label?: string      // present when scope_type="api_key"
  period_type: string     // "monthly" | "weekly" | "daily"
  provider: string        // "" = all, "anthropic", "openai"
  limit_amount: string    // decimal string (USD)
  alert_threshold: string // percentage as string
  action: string          // "alert" | "block"
  enabled: boolean
  current_spend: string   // decimal string
  pct_used: number        // 0-100
  period_start: string    // ISO date
  created_at: string
}

export interface UpsertSpendLimitReq {
  scope_type: string
  scope_id: string
  period_type: string
  provider: string
  limit_amount: string
  alert_threshold: string
  action: string
  enabled?: boolean
}

export function useSpendLimits() {
  const [limits, setLimits] = useState<SpendLimit[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchLimits = useCallback(async () => {
    if (!localStorage.getItem('user_id')) return
    setLoading(true)
    setError(null)
    try {
      const res = await apiFetch('/v1/budget')
      if (!res.ok) throw new Error('Failed to fetch spend limits')
      const data = await res.json()
      setLimits(data.budget_limits || [])
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchLimits() }, [fetchLimits])

  async function upsertLimit(req: UpsertSpendLimitReq): Promise<SpendLimit> {
    const res = await apiFetch('/v1/admin/budget', {
      method: 'PUT',
      body: JSON.stringify(req),
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || d.message || 'Failed to save spend limit')
    }
    const saved: SpendLimit = await res.json()
    await fetchLimits()
    return saved
  }

  async function deleteLimit(id: number): Promise<void> {
    const res = await apiFetch(`/v1/admin/budget/${id}`, {
      method: 'DELETE',
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to delete spend limit')
    }
    setLimits(prev => prev.filter(l => l.id !== id))
  }

  return { limits, loading, error, refresh: fetchLimits, upsertLimit, deleteLimit }
}
