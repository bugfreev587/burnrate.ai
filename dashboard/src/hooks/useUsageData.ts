import { useEffect, useState, useCallback } from 'react'

const API_SERVER_URL = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

export interface UsageLog {
  id: number
  user_id: string
  provider: string
  model: string
  prompt_tokens: number
  completion_tokens: number
  cost: number
  request_id: string | null
  created_at: string
}

interface UsageDataState {
  logs: UsageLog[]
  loading: boolean
  error: string | null
}

export function useUsageData(): UsageDataState & { refresh: () => void } {
  const [state, setState] = useState<UsageDataState>({
    logs: [],
    loading: false,
    error: null,
  })

  const fetchData = useCallback(async () => {
    const userId = localStorage.getItem('user_id')
    if (!userId) return

    setState(prev => ({ ...prev, loading: true, error: null }))

    try {
      const res = await fetch(`${API_SERVER_URL}/v1/usage`, {
        headers: { 'X-User-ID': userId },
      })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      setState({ logs: data.usage_logs ?? [], loading: false, error: null })
    } catch (err) {
      setState(prev => ({
        ...prev,
        loading: false,
        error: err instanceof Error ? err.message : 'Failed to load usage data',
      }))
    }
  }, [])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  return { ...state, refresh: fetchData }
}
