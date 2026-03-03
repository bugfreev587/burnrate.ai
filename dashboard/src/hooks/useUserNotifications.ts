import { useCallback, useEffect, useState } from 'react'
import { apiFetch } from '../lib/api'

export interface UserNotification {
  id: number
  user_id: string
  tenant_id?: number
  type: string
  title: string
  body: string
  payload: string
  status: 'unread' | 'read'
  read_at?: string
  created_at: string
}

export interface UserNotificationChannel {
  id: number
  user_id: string
  channel_type: 'email' | 'slack' | 'webhook'
  name: string
  config: string
  event_types: string[]
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface UserNotificationChannelReq {
  channel_type: 'email' | 'slack' | 'webhook'
  name: string
  config: Record<string, string>
  event_types: string[]
  enabled: boolean
}

export function useUserNotifications(enabled = true) {
  const [notifications, setNotifications] = useState<UserNotification[]>([])
  const [unreadCount, setUnreadCount] = useState(0)
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    if (!enabled) {
      setNotifications([])
      setUnreadCount(0)
      setLoading(false)
      return
    }
    setLoading(true)
    try {
      const res = await apiFetch('/v1/user/notifications?limit=50')
      if (!res.ok) throw new Error('Failed to fetch notifications')
      const data = await res.json()
      setNotifications(data.notifications ?? [])
      setUnreadCount(data.unread_count ?? 0)
    } catch {
      setNotifications([])
      setUnreadCount(0)
    } finally {
      setLoading(false)
    }
  }, [enabled])

  useEffect(() => { refresh() }, [refresh])

  const markRead = useCallback(async (id: number) => {
    if (!enabled) return
    const res = await apiFetch(`/v1/user/notifications/${id}/read`, { method: 'PATCH' })
    if (!res.ok) return
    setNotifications(prev => prev.map(n => n.id === id ? { ...n, status: 'read' } : n))
    setUnreadCount(prev => Math.max(0, prev - 1))
  }, [])

  const deleteNotification = useCallback(async (id: number) => {
    if (!enabled) return
    const res = await apiFetch(`/v1/user/notifications/${id}`, { method: 'DELETE' })
    if (!res.ok) return
    setNotifications(prev => {
      const target = prev.find(n => n.id === id)
      if (target && target.status === 'unread') {
        setUnreadCount(c => Math.max(0, c - 1))
      }
      return prev.filter(n => n.id !== id)
    })
  }, [enabled])

  const markAllRead = useCallback(async () => {
    if (!enabled) return
    const res = await apiFetch('/v1/user/notifications/read-all', { method: 'PATCH' })
    if (!res.ok) return
    setNotifications(prev => prev.map(n => ({ ...n, status: 'read' })))
    setUnreadCount(0)
  }, [enabled])

  const acceptInvitation = useCallback(async (tenantId: number) => {
    if (!enabled) throw new Error('Notifications are not enabled')
    const res = await apiFetch(`/v1/user/invitations/${tenantId}/accept`, { method: 'POST' })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error ?? 'Failed to accept invitation')
    }
    await refresh()
  }, [enabled, refresh])

  const denyInvitation = useCallback(async (tenantId: number) => {
    if (!enabled) throw new Error('Notifications are not enabled')
    const res = await apiFetch(`/v1/user/invitations/${tenantId}/deny`, { method: 'POST' })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error ?? 'Failed to deny invitation')
    }
    await refresh()
  }, [enabled, refresh])

  return { notifications, unreadCount, loading, refresh, markRead, markAllRead, deleteNotification, acceptInvitation, denyInvitation }
}

export function useUserNotificationChannels(enabled = true) {
  const [channels, setChannels] = useState<UserNotificationChannel[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    if (!enabled) {
      setChannels([])
      setLoading(false)
      setError(null)
      return
    }
    setLoading(true)
    setError(null)
    try {
      const res = await apiFetch('/v1/user/notification-channels')
      if (!res.ok) throw new Error('Failed to fetch personal notification channels')
      const data = await res.json()
      setChannels(data.notification_channels || [])
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }, [enabled])

  useEffect(() => { refresh() }, [refresh])

  async function createChannel(req: UserNotificationChannelReq): Promise<UserNotificationChannel> {
    if (!enabled) throw new Error('Personal notification channels are not enabled')
    const res = await apiFetch('/v1/user/notification-channels', {
      method: 'POST',
      body: JSON.stringify(req),
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || 'Failed to create notification channel')
    }
    const saved: UserNotificationChannel = await res.json()
    await refresh()
    return saved
  }

  async function updateChannel(id: number, req: Partial<UserNotificationChannelReq>): Promise<UserNotificationChannel> {
    if (!enabled) throw new Error('Personal notification channels are not enabled')
    const res = await apiFetch(`/v1/user/notification-channels/${id}`, {
      method: 'PUT',
      body: JSON.stringify(req),
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || 'Failed to update notification channel')
    }
    const saved: UserNotificationChannel = await res.json()
    await refresh()
    return saved
  }

  async function deleteChannel(id: number): Promise<void> {
    if (!enabled) throw new Error('Personal notification channels are not enabled')
    const res = await apiFetch(`/v1/user/notification-channels/${id}`, {
      method: 'DELETE',
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || 'Failed to delete notification channel')
    }
    setChannels(prev => prev.filter(c => c.id !== id))
  }

  async function testChannel(id: number): Promise<{ success: boolean; error?: string }> {
    if (!enabled) throw new Error('Personal notification channels are not enabled')
    const res = await apiFetch(`/v1/user/notification-channels/${id}/test`, {
      method: 'POST',
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || 'Failed to test notification channel')
    }
    return res.json()
  }

  return { channels, loading, error, refresh, createChannel, updateChannel, deleteChannel, testChannel }
}
