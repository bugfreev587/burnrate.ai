import { useState, useEffect, useCallback } from 'react'

const API_URL = import.meta.env.VITE_API_SERVER_URL || ''

function authHeaders(): Record<string, string> {
  const userId = localStorage.getItem('user_id') || ''
  return { 'Content-Type': 'application/json', 'X-User-ID': userId }
}

export interface SpendLimit {
  id: number
  scope_type: string      // "account" | "api_key"
  scope_id: string
  period_type: string     // "monthly" | "weekly" | "daily"
  limit_amount: string    // decimal string (USD)
  alert_threshold: string // percentage as string
  action: string          // "alert" | "block"
  current_spend: string   // decimal string
  pct_used: number        // 0-100
  period_start: string    // ISO date
  created_at: string
}

export interface UpsertSpendLimitReq {
  scope_type: string
  scope_id: string
  period_type: string
  limit_amount: string
  alert_threshold: string
  action: string
}

export function useSpendLimits() {
  const [limits, setLimits] = useState<SpendLimit[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchLimits = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetch(`${API_URL}/v1/admin/budget`, { headers: authHeaders() })
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
    const res = await fetch(`${API_URL}/v1/admin/budget`, {
      method: 'PUT',
      headers: authHeaders(),
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
    const res = await fetch(`${API_URL}/v1/admin/budget/${id}`, {
      method: 'DELETE',
      headers: authHeaders(),
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to delete spend limit')
    }
    setLimits(prev => prev.filter(l => l.id !== id))
  }

  return { limits, loading, error, refresh: fetchLimits, upsertLimit, deleteLimit }
}
