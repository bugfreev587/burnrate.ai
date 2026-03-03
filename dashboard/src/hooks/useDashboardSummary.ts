import { useEffect, useState, useCallback, useRef, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { apiFetch } from '../lib/api'

// ─── Filter types ───────────────────────────────────────────────────────────

export type BillingModeFilter = 'all' | 'api_usage_billed' | 'monthly_subscription'
export type DatePreset = '1d' | '7d' | '30d' | 'this_month' | 'last_month' | 'custom'

export interface DashboardFilters {
  from: string          // YYYY-MM-DD
  to: string            // YYYY-MM-DD
  preset: DatePreset
  billing_mode: BillingModeFilter
  project_id: string    // "" = all
  api_key_id: string    // "" = all
}

const LS_KEY = 'tg.dashboard.filters.v2'

function localDateStr(d: Date): string {
  return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0')
}

function presetToDates(preset: DatePreset): { from: string; to: string } {
  const now = new Date()
  const today = localDateStr(now)
  switch (preset) {
    case '1d': {
      const d = new Date(now); d.setDate(d.getDate() - 1)
      return { from: localDateStr(d), to: today }
    }
    case '7d': {
      const d = new Date(now); d.setDate(d.getDate() - 7)
      return { from: localDateStr(d), to: today }
    }
    case '30d': {
      const d = new Date(now); d.setDate(d.getDate() - 30)
      return { from: localDateStr(d), to: today }
    }
    case 'this_month': {
      const monthStart = new Date(now.getFullYear(), now.getMonth(), 1)
      return { from: localDateStr(monthStart), to: today }
    }
    case 'last_month': {
      const lmStart = new Date(now.getFullYear(), now.getMonth() - 1, 1)
      const lmEnd = new Date(now.getFullYear(), now.getMonth(), 0)
      return { from: localDateStr(lmStart), to: localDateStr(lmEnd) }
    }
    default:
      return { from: today, to: today }
  }
}

function defaultFilters(): DashboardFilters {
  const { from, to } = presetToDates('7d')
  return { from, to, preset: '7d', billing_mode: 'all', project_id: '', api_key_id: '' }
}

function loadFilters(searchParams: URLSearchParams): DashboardFilters {
  const def = defaultFilters()

  // Priority 1: URL params
  const urlFrom = searchParams.get('from')
  const urlTo = searchParams.get('to')
  if (urlFrom && urlTo) {
    return {
      from: urlFrom,
      to: urlTo,
      preset: (searchParams.get('preset') as DatePreset) || 'custom',
      billing_mode: (searchParams.get('billing_mode') as BillingModeFilter) || def.billing_mode,
      project_id: searchParams.get('project_id') || '',
      api_key_id: searchParams.get('api_key_id') || '',
    }
  }

  // Priority 2: localStorage
  try {
    const stored = localStorage.getItem(LS_KEY)
    if (stored) {
      const parsed = JSON.parse(stored) as Partial<DashboardFilters>
      if (parsed.from && parsed.to) {
        return { ...def, ...parsed }
      }
    }
  } catch { /* ignore */ }

  return def
}

// ─── Response types ─────────────────────────────────────────────────────────

export interface KpiValue {
  value: number | string
  delta_pct: number
}

export interface BudgetHealth {
  status: 'ok' | 'warning' | 'blocking_soon'
  message: string
}

export interface TimeseriesPoint {
  ts: string
  cost?: string
  requests?: number
  tokens?: number
  ms?: number
  success?: number
  blocked?: number
  error?: number
}

export interface BreakdownItem {
  name: string
  key_id?: string
  provider?: string
  project_id?: number
  cost: string
  input_tokens: number
  output_tokens: number
  requests: number
  pct_of_total: number
}

export interface SpendLimitEntry {
  id: number
  scope_type: string
  scope_id: string
  key_label?: string
  period_type: string
  provider: string
  limit_amount: string
  alert_threshold: string
  action: string
  current_spend: string
  pct_used: number
  status: string
}

export interface RateLimitEntry {
  id: number
  scope_type: string
  scope_id: string
  provider: string
  model: string
  metric: string
  limit_value: number
  window_seconds: number
}

export interface CostDriverInsight {
  type: string
  text: string
}

export interface GovernanceData {
  active_api_keys: number
  active_projects: number
  audit_events_7d: number
  revoked_keys_period: number
}

export interface DashboardSummary {
  plan: string
  cost_note: string
  range: { from: string; to: string; prev_from: string; prev_to: string }
  filters: { billing_mode: string; project_id: number; api_key_id: string }
  kpis: {
    spend_total: KpiValue
    projected_month_end: KpiValue
    budget_health: BudgetHealth
    success_rate: KpiValue
    requests_total: KpiValue
    avg_cost_per_request: KpiValue
  }
  forecast: {
    total_so_far: string
    daily_average: string
    forecast: string
    days_elapsed: number
    days_remaining: number
  }
  timeseries: {
    daily_cost: TimeseriesPoint[]
    daily_latency_p95: TimeseriesPoint[]
    outcomes: TimeseriesPoint[]
  }
  breakdowns: {
    by_provider: BreakdownItem[]
    by_model: BreakdownItem[]
    by_api_key: BreakdownItem[]
    by_project: BreakdownItem[]
  }
  limits: {
    budget_utilization_pct: number
    blocked_requests: number
    rate_limited_requests: number
    active_spend_limits: SpendLimitEntry[]
    active_rate_limits: RateLimitEntry[]
  }
  governance: GovernanceData
  insights: {
    cost_drivers: CostDriverInsight[]
    concentration: { top_api_key_pct: number; top_project_pct: number; top_model_pct: number }
  }
  latency: {
    p50: number
    p95: number
    p99: number
    avg: number
    sample_count: number
  }
  recent_requests: { default_limit: number; has_more: boolean }
}

export interface RecentRequestRow {
  id: number
  provider: string
  model: string
  key_id: string
  key_label: string
  prompt_tokens: number
  completion_tokens: number
  cost: string
  latency_ms: number
  api_usage_billed: boolean
  result: string
  created_at: string
}

// ─── Hook ───────────────────────────────────────────────────────────────────

interface DashboardState {
  summary: DashboardSummary | null
  recentRequests: RecentRequestRow[]
  loading: boolean
  error: string | null
}

export function useDashboardSummary(pollIntervalMs = 30_000) {
  const [searchParams, setSearchParams] = useSearchParams()
  const [filters, setFiltersState] = useState<DashboardFilters>(() => loadFilters(searchParams))
  const [state, setState] = useState<DashboardState>({
    summary: null,
    recentRequests: [],
    loading: false,
    error: null,
  })
  const initialLoadDone = useRef(false)

  // Sync filters → URL + localStorage
  const setFilters = useCallback((f: DashboardFilters | ((prev: DashboardFilters) => DashboardFilters)) => {
    setFiltersState(prev => {
      const next = typeof f === 'function' ? f(prev) : f
      // Persist to localStorage
      try { localStorage.setItem(LS_KEY, JSON.stringify(next)) } catch { /* ignore */ }
      // Persist to URL
      const params: Record<string, string> = { from: next.from, to: next.to }
      if (next.preset !== '7d') params.preset = next.preset
      if (next.billing_mode !== 'all') params.billing_mode = next.billing_mode
      if (next.project_id) params.project_id = next.project_id
      if (next.api_key_id) params.api_key_id = next.api_key_id
      setSearchParams(params, { replace: true })
      return next
    })
  }, [setSearchParams])

  // Apply preset helper
  const applyPreset = useCallback((preset: DatePreset) => {
    if (preset === 'custom') return
    const { from, to } = presetToDates(preset)
    setFilters(prev => ({ ...prev, preset, from, to }))
  }, [setFilters])

  const tz = useMemo(() => Intl.DateTimeFormat().resolvedOptions().timeZone, [])

  const fetchSummary = useCallback(async () => {
    const userId = localStorage.getItem('user_id')
    if (!userId) return

    if (!initialLoadDone.current) {
      setState(prev => ({ ...prev, loading: true, error: null }))
    }

    try {
      const qs = new URLSearchParams({
        from: filters.from,
        to: filters.to,
        billing_mode: filters.billing_mode,
        tz,
      })
      if (filters.project_id) qs.set('project_id', filters.project_id)
      if (filters.api_key_id) qs.set('api_key_id', filters.api_key_id)

      const res = await apiFetch(`/v1/dashboard/summary?${qs}`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data: DashboardSummary = await res.json()

      initialLoadDone.current = true
      setState(prev => ({ ...prev, summary: data, loading: false, error: null }))
    } catch (err) {
      setState(prev => ({
        ...prev,
        loading: false,
        error: err instanceof Error ? err.message : 'Failed to load dashboard',
      }))
    }
  }, [filters.from, filters.to, filters.billing_mode, filters.project_id, filters.api_key_id, tz])

  const fetchRecentRequests = useCallback(async (limit = 100) => {
    try {
      const qs = new URLSearchParams({
        from: filters.from,
        to: filters.to,
        billing_mode: filters.billing_mode,
        limit: String(limit),
        tz,
      })
      if (filters.project_id) qs.set('project_id', filters.project_id)
      if (filters.api_key_id) qs.set('api_key_id', filters.api_key_id)

      const res = await apiFetch(`/v1/dashboard/recent-requests?${qs}`)
      if (!res.ok) return
      const data = await res.json()
      setState(prev => ({ ...prev, recentRequests: data.requests ?? [] }))
    } catch { /* ignore */ }
  }, [filters.from, filters.to, filters.billing_mode, filters.project_id, filters.api_key_id, tz])

  // Fetch on mount / filter change
  useEffect(() => {
    initialLoadDone.current = false
    fetchSummary()
  }, [fetchSummary])

  // Polling
  useEffect(() => {
    const id = setInterval(fetchSummary, pollIntervalMs)
    return () => clearInterval(id)
  }, [fetchSummary, pollIntervalMs])

  return {
    ...state,
    filters,
    setFilters,
    applyPreset,
    refresh: fetchSummary,
    fetchRecentRequests,
  }
}
