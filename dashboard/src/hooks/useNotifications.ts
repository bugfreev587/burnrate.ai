import { useState, useEffect, useCallback } from 'react'

const API_URL = import.meta.env.VITE_API_SERVER_URL || ''

function authHeaders(): Record<string, string> {
  const userId = localStorage.getItem('user_id') || ''
  return { 'Content-Type': 'application/json', 'X-User-ID': userId }
}

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

export function useNotifications() {
  const [channels, setChannels] = useState<NotificationChannel[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchChannels = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetch(`${API_URL}/v1/admin/notifications`, { headers: authHeaders() })
      if (!res.ok) throw new Error('Failed to fetch notification channels')
      const data = await res.json()
      setChannels(data.notification_channels || [])
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchChannels() }, [fetchChannels])

  async function createChannel(req: CreateNotificationChannelReq): Promise<NotificationChannel> {
    const res = await fetch(`${API_URL}/v1/admin/notifications`, {
      method: 'POST',
      headers: authHeaders(),
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
    const res = await fetch(`${API_URL}/v1/admin/notifications/${id}`, {
      method: 'PUT',
      headers: authHeaders(),
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
    const res = await fetch(`${API_URL}/v1/admin/notifications/${id}`, {
      method: 'DELETE',
      headers: authHeaders(),
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to delete notification channel')
    }
    setChannels(prev => prev.filter(c => c.id !== id))
  }

  async function testChannel(id: number): Promise<{ success: boolean; error?: string }> {
    const res = await fetch(`${API_URL}/v1/admin/notifications/${id}/test`, {
      method: 'POST',
      headers: authHeaders(),
    })
    if (!res.ok) {
      const d = await res.json()
      throw new Error(d.error || 'Failed to test notification channel')
    }
    return res.json()
  }

  return { channels, loading, error, refresh: fetchChannels, createChannel, updateChannel, deleteChannel, testChannel }
}
