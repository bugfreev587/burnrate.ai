import { useEffect, useState, useCallback } from 'react'

const API_SERVER_URL = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

// ─── Raw log (for recent-requests table) ─────────────────────────────────────
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

// ─── Summary ──────────────────────────────────────────────────────────────────
export interface CostPeriods {
  today: string
  yesterday: string
  this_month: string
  last_month: string
  cumulative: string
}
export interface RequestPeriods {
  today: number
  yesterday: number
  this_month: number
  last_month: number
  cumulative: number
}
export interface TokenSummary {
  input_total: number
  output_total: number
  total: number
  avg_per_request: number
}
export interface ModelBreakdown {
  model: string
  provider: string
  cost: string
  input_tokens: number
  output_tokens: number
  requests: number
}
export interface DailyTrend {
  date: string
  cost: string
  tokens: number
}
export interface UsageSummary {
  cost: CostPeriods
  requests: RequestPeriods
  tokens: TokenSummary
  by_model: ModelBreakdown[]
  daily_trend: DailyTrend[]
}

// ─── Budget ───────────────────────────────────────────────────────────────────
export interface BudgetStatus {
  id: number
  scope_type: string
  scope_id: string
  period_type: string
  limit_amount: string
  alert_threshold: string
  action: string
  current_spend: string
  pct_used: number
  period_start: string
}

// ─── Combined state ───────────────────────────────────────────────────────────
interface DashboardState {
  logs: UsageLog[]
  summary: UsageSummary | null
  budgets: BudgetStatus[]
  loading: boolean
  error: string | null
}

export function useUsageData(): DashboardState & { refresh: () => void } {
  const [state, setState] = useState<DashboardState>({
    logs: [],
    summary: null,
    budgets: [],
    loading: false,
    error: null,
  })

  const fetchData = useCallback(async () => {
    const userId = localStorage.getItem('user_id')
    if (!userId) return

    setState(prev => ({ ...prev, loading: true, error: null }))

    try {
      const headers = { 'X-User-ID': userId }

      const [logsRes, summaryRes, budgetRes] = await Promise.all([
        fetch(`${API_SERVER_URL}/v1/usage`, { headers }),
        fetch(`${API_SERVER_URL}/v1/usage/summary`, { headers }),
        fetch(`${API_SERVER_URL}/v1/admin/budget`, { headers }),
      ])

      const logsData = logsRes.ok ? await logsRes.json() : { usage_logs: [] }
      const summaryData = summaryRes.ok ? await summaryRes.json() : null
      const budgetData = budgetRes.ok ? await budgetRes.json() : { budget_limits: [] }

      setState({
        logs: logsData.usage_logs ?? [],
        summary: summaryData,
        budgets: budgetData.budget_limits ?? [],
        loading: false,
        error: null,
      })
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
