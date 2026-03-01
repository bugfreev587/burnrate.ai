import { useEffect, useState } from 'react'
import { apiFetch } from '../lib/api'
import { useTenant } from '../contexts/TenantContext'

export interface PresetOption {
  key: string   // "1d" | "3d" | "7d" | "14d" | "30d" | "90d" | "custom"
  days?: number
  enabled: boolean
}

export interface DashboardPlanLimits {
  max_api_keys: number
  max_provider_keys: number
  max_members: number
  allowed_periods: string[]
  allow_block_action: boolean
  allow_per_key_budget: boolean
  data_retention_days: number
  allow_rate_limits: boolean
  allow_per_key_rate_limit: boolean
  allow_export: boolean
  max_budget_limits: number          // -1 = unlimited
  max_rate_limits: number            // -1 = unlimited
  max_notification_channels: number  // -1 = unlimited
}

export interface DashboardConfig {
  plan: string
  retention: { type: string; max_days: number }
  availability: { min_start_date: string }
  effective: { min_start_date: string }
  preset_options: PresetOption[]
  plan_limits: DashboardPlanLimits
}

export function useDashboardConfig(): { config: DashboardConfig | null; loading: boolean } {
  const { isSynced } = useTenant()
  const [config, setConfig] = useState<DashboardConfig | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!isSynced) return

    setLoading(true)
    apiFetch('/v1/dashboard/config')
      .then(res => (res.ok ? res.json() : null))
      .then((data: DashboardConfig | null) => {
        setConfig(data)
        setLoading(false)
      })
      .catch(() => setLoading(false))
  }, [isSynced])

  return { config, loading }
}
