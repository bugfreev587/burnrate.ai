import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../lib/api'

// ── Types ─────────────────────────────────────────────────────────────────────

export interface CatalogPriceEntry {
  price_per_unit: string
  unit_size: number
  pricing_id: number
}

export interface CatalogEntry {
  provider_id: number
  provider: string
  provider_display: string
  model_id: number
  model_name: string
  prices: Record<string, CatalogPriceEntry> // keyed by price_type
}

export interface ConfigRateView {
  id: number
  model_id: number
  model_name: string
  provider: string
  price_type: string
  price_per_unit: string
  unit_size: number
}

export interface AssignedKeyView {
  key_id: string
  label: string
}

export interface PricingConfigView {
  id: number
  name: string
  description: string
  rates: ConfigRateView[]
  assigned_key: AssignedKeyView | null
  created_at: string
}

export interface ActiveAPIKey {
  key_id: string
  label: string
  expires_at: string | null
  created_at: string
}

// ── Hook ──────────────────────────────────────────────────────────────────────

export function usePricingConfig() {
  const [catalog, setCatalog] = useState<CatalogEntry[]>([])
  const [configs, setConfigs] = useState<PricingConfigView[]>([])
  const [activeKeys, setActiveKeys] = useState<ActiveAPIKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchAll = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [catalogRes, configsRes, keysRes] = await Promise.all([
        apiFetch('/v1/admin/pricing/catalog'),
        apiFetch('/v1/admin/pricing-configs'),
        apiFetch('/v1/admin/api_keys'),
      ])
      if (!catalogRes.ok) throw new Error('Failed to fetch pricing catalog')
      if (!configsRes.ok) throw new Error('Failed to fetch pricing configs')
      if (!keysRes.ok) throw new Error('Failed to fetch API keys')

      const catalogData = await catalogRes.json()
      const configsData = await configsRes.json()
      const keysData = await keysRes.json()

      setCatalog(catalogData.catalog || [])
      setConfigs(configsData.configs || [])
      setActiveKeys(keysData.api_keys || [])
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchAll() }, [fetchAll])

  // ── CRUD ─────────────────────────────────────────────────────────────────

  async function createConfig(name: string, description: string): Promise<PricingConfigView> {
    const res = await apiFetch('/v1/admin/pricing-configs', {
      method: 'POST',
      body: JSON.stringify({ name, description }),
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to create config')
    }
    const created: PricingConfigView = await res.json()
    setConfigs(prev => [created, ...prev])
    return created
  }

  async function deleteConfig(configId: number): Promise<void> {
    const res = await apiFetch(`/v1/admin/pricing-configs/${configId}`, {
      method: 'DELETE',
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to delete config')
    }
    setConfigs(prev => prev.filter(c => c.id !== configId))
  }

  async function addRate(
    configId: number,
    modelId: number,
    priceType: string,
    pricePerUnit: string,
    unitSize = 1000000,
  ): Promise<ConfigRateView> {
    const res = await apiFetch(`/v1/admin/pricing-configs/${configId}/rates`, {
      method: 'POST',
      body: JSON.stringify({ model_id: modelId, price_type: priceType, price_per_unit: pricePerUnit, unit_size: unitSize }),
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to add rate')
    }
    const rate: ConfigRateView = await res.json()
    setConfigs(prev => prev.map(c => {
      if (c.id !== configId) return c
      const existing = c.rates.findIndex(r => r.model_id === modelId && r.price_type === priceType)
      if (existing >= 0) {
        const rates = [...c.rates]
        rates[existing] = rate
        return { ...c, rates }
      }
      return { ...c, rates: [...c.rates, rate] }
    }))
    return rate
  }

  async function deleteRate(configId: number, rateId: number): Promise<void> {
    const res = await apiFetch(`/v1/admin/pricing-configs/${configId}/rates/${rateId}`, {
      method: 'DELETE',
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to delete rate')
    }
    setConfigs(prev => prev.map(c =>
      c.id === configId ? { ...c, rates: c.rates.filter(r => r.id !== rateId) } : c
    ))
  }

  async function assignKey(configId: number, keyId: string, label: string): Promise<void> {
    const res = await apiFetch(`/v1/admin/pricing-configs/${configId}/assign`, {
      method: 'PUT',
      body: JSON.stringify({ key_id: keyId }),
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to assign key')
    }
    setConfigs(prev => prev.map(c =>
      c.id === configId ? { ...c, assigned_key: { key_id: keyId, label } } : c
    ))
  }

  async function unassignKey(configId: number): Promise<void> {
    const res = await apiFetch(`/v1/admin/pricing-configs/${configId}/assign`, {
      method: 'DELETE',
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to unassign key')
    }
    setConfigs(prev => prev.map(c =>
      c.id === configId ? { ...c, assigned_key: null } : c
    ))
  }

  return {
    catalog,
    configs,
    activeKeys,
    loading,
    error,
    refresh: fetchAll,
    createConfig,
    deleteConfig,
    addRate,
    deleteRate,
    assignKey,
    unassignKey,
  }
}
