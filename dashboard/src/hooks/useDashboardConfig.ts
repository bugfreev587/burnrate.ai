import { useEffect, useState } from 'react'

const API_SERVER_URL = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

export interface PresetOption {
  key: string   // "1d" | "3d" | "7d" | "14d" | "30d" | "90d" | "custom"
  days?: number
  enabled: boolean
}

export interface DashboardConfig {
  plan: string
  retention: { type: string; max_days: number }
  availability: { min_start_date: string }
  effective: { min_start_date: string }
  preset_options: PresetOption[]
}

export function useDashboardConfig(): { config: DashboardConfig | null; loading: boolean } {
  const [config, setConfig] = useState<DashboardConfig | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    const userId = localStorage.getItem('user_id')
    if (!userId) return

    setLoading(true)
    fetch(`${API_SERVER_URL}/v1/dashboard/config`, {
      headers: { 'X-User-ID': userId },
    })
      .then(res => (res.ok ? res.json() : null))
      .then((data: DashboardConfig | null) => {
        setConfig(data)
        setLoading(false)
      })
      .catch(() => setLoading(false))
  }, [])

  return { config, loading }
}
