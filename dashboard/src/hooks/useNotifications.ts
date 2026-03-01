import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../lib/api'

export interface NotificationChannel {
  id: number
  tenant_id: number
  channel_type: string   // "email" | "slack" | "webhook"
  name: string
  config: string         // JSON string
  event_types: string[]  // ["budget_blocked","budget_warning","rate_limit_exceeded"]
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface CreateNotificationChannelReq {
  channel_type: string
  name: string
  config: Record<string, string>
  event_types: string[]
  enabled: boolean
}

export interface UpdateNotificationChannelReq {
  channel_type?: string
  name?: string
  config?: Record<string, string>
  event_types?: string[]
  enabled?: boolean
}

export function useNotifications(enabled = true) {
  const [channels, setChannels] = useState<NotificationChannel[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchChannels = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await apiFetch('/v1/admin/notifications')
      if (!res.ok) throw new Error('Failed to fetch notification channels')
      const data = await res.json()
      setChannels(data.notification_channels || [])
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!enabled) {
      setChannels([])
      setLoading(false)
      setError(null)
      return
    }
    fetchChannels()
  }, [enabled, fetchChannels])

  async function createChannel(req: CreateNotificationChannelReq): Promise<NotificationChannel> {
    if (!enabled) throw new Error('Notification channel management is not enabled')
    const res = await apiFetch('/v1/admin/notifications', {
      method: 'POST',
      body: JSON.stringify(req),
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to create notification channel')
    }
    const saved: NotificationChannel = await res.json()
    await fetchChannels()
    return saved
  }

  async function updateChannel(id: number, req: UpdateNotificationChannelReq): Promise<NotificationChannel> {
    if (!enabled) throw new Error('Notification channel management is not enabled')
    const res = await apiFetch(`/v1/admin/notifications/${id}`, {
      method: 'PUT',
      body: JSON.stringify(req),
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to update notification channel')
    }
    const saved: NotificationChannel = await res.json()
    await fetchChannels()
    return saved
  }

  async function deleteChannel(id: number): Promise<void> {
    if (!enabled) throw new Error('Notification channel management is not enabled')
    const res = await apiFetch(`/v1/admin/notifications/${id}`, {
      method: 'DELETE',
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to delete notification channel')
    }
    setChannels(prev => prev.filter(c => c.id !== id))
  }

  async function testChannel(id: number): Promise<{ success: boolean; error?: string }> {
    if (!enabled) throw new Error('Notification channel management is not enabled')
    const res = await apiFetch(`/v1/admin/notifications/${id}/test`, {
      method: 'POST',
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to test notification channel')
    }
    return res.json()
  }

  return { channels, loading, error, refresh: fetchChannels, createChannel, updateChannel, deleteChannel, testChannel }
}
