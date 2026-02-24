import { useEffect, useState, useCallback } from 'react'

const API_SERVER_URL = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

// ─── Date range ───────────────────────────────────────────────────────────────
export interface DateRange {
  preset?: string    // "1d" | "3d" | "7d" | "14d" | "30d" | "90d"
  startDate?: string // "YYYY-MM-DD", used when preset == "custom"
  endDate?: string
}

// Format a Date as YYYY-MM-DD in the browser's local timezone.
function localDateStr(d: Date): string {
  return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0')
}

// Resolve a DateRange to concrete start/end query strings.
function resolveDateRange(range: DateRange): { startDate: string; endDate: string } | null {
  const today = new Date()
  const todayStr = localDateStr(today)

  if (range.preset && range.preset !== 'custom') {
    const daysMap: Record<string, number> = {
      '1d': 1, '3d': 3, '7d': 7, '14d': 14, '30d': 30, '90d': 90,
    }
    const days = daysMap[range.preset]
    if (days !== undefined) {
      const start = new Date(today)
      start.setDate(start.getDate() - days)
      return { startDate: localDateStr(start), endDate: todayStr }
    }
  }

  if (range.preset === 'custom' && range.startDate && range.endDate) {
    return { startDate: range.startDate, endDate: range.endDate }
  }

  return null
}

// ─── Raw log (for recent-requests table) ─────────────────────────────────────
export interface UsageLog {
  id: number
  tenant_id: number
  user_id: string
  provider: string
  model: string
  prompt_tokens: number
  completion_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  reasoning_tokens: number
  cost: string   // decimal.Decimal serialises as a JSON string, e.g. "0.00123400"
  request_id: string | null
  api_key_fingerprint: string
  created_at: string
  api_usage_billed: boolean
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
  applied_range?: { start: string; end: string }
}

// ─── Budget ───────────────────────────────────────────────────────────────────
export interface BudgetStatus {
  id: number
  scope_type: string
  scope_id: string
  period_type: string
  provider: string
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
  appliedRange: { start: string; end: string } | null
  loading: boolean
  error: string | null
}

export function useUsageData(dateRange?: DateRange): DashboardState & { refresh: () => void } {
  const [state, setState] = useState<DashboardState>({
    logs: [],
    summary: null,
    budgets: [],
    appliedRange: null,
    loading: false,
    error: null,
  })

  const fetchData = useCallback(async () => {
    const userId = localStorage.getItem('user_id')
    if (!userId) return

    setState(prev => ({ ...prev, loading: true, error: null }))

    try {
      const headers = { 'X-User-ID': userId }

      // Build optional date query string, always including the browser timezone.
      const tz = Intl.DateTimeFormat().resolvedOptions().timeZone
      let dateQS = `?tz=${encodeURIComponent(tz)}`
      if (dateRange) {
        const resolved = resolveDateRange(dateRange)
        if (resolved) {
          dateQS += `&start_date=${resolved.startDate}&end_date=${resolved.endDate}`
        }
      }

      const [logsRes, summaryRes, budgetRes] = await Promise.all([
        fetch(`${API_SERVER_URL}/v1/usage${dateQS}`, { headers }),
        fetch(`${API_SERVER_URL}/v1/usage/summary${dateQS}`, { headers }),
        fetch(`${API_SERVER_URL}/v1/admin/budget?tz=${encodeURIComponent(tz)}`, { headers }),
      ])

      const logsData = logsRes.ok ? await logsRes.json() : { usage_logs: [] }
      const summaryData: UsageSummary | null = summaryRes.ok ? await summaryRes.json() : null
      const budgetData = budgetRes.ok ? await budgetRes.json() : { budget_limits: [] }

      setState({
        logs: logsData.usage_logs ?? [],
        summary: summaryData,
        budgets: budgetData.budget_limits ?? [],
        appliedRange: summaryData?.applied_range ?? null,
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
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dateRange?.preset, dateRange?.startDate, dateRange?.endDate])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  return { ...state, refresh: fetchData }
}
