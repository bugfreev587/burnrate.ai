import { useState, useEffect, useCallback } from 'react'

const API_URL = import.meta.env.VITE_API_SERVER_URL || ''

function authHeaders(): Record<string, string> {
  const userId = localStorage.getItem('user_id') || ''
  return { 'Content-Type': 'application/json', 'X-User-ID': userId }
}

export interface AuditReport {
  id: number
  tenant_id: number
  created_by_user_id: string
  created_by_email: string
  period_start: string
  period_end: string
  filters: string
  format: string
  status: string
  error_message: string
  artifact_size_bytes: number
  row_count: number
  created_at: string
}

export interface CreateReportRequest {
  period_start: string   // YYYY-MM-DD
  period_end: string     // YYYY-MM-DD
  format: string         // "PDF" | "CSV"
  provider?: string
  api_key_ids?: string[]
  api_usage_billed?: boolean
}

export function useAuditReports() {
  const [reports, setReports] = useState<AuditReport[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchReports = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetch(`${API_URL}/v1/audit/reports`, { headers: authHeaders() })
      if (!res.ok) throw new Error('Failed to fetch reports')
      const data = await res.json()
      setReports(data.reports || [])
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchReports() }, [fetchReports])

  async function generate(req: CreateReportRequest): Promise<AuditReport | null> {
    try {
      const res = await fetch(`${API_URL}/v1/audit/reports`, {
        method: 'POST',
        headers: authHeaders(),
        body: JSON.stringify(req),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error || 'Failed to generate report')
      }
      const created: AuditReport = await res.json()
      await fetchReports()
      return created
    } catch (e: unknown) {
      throw e instanceof Error ? e : new Error('Unknown error')
    }
  }

  async function deleteReport(id: number): Promise<boolean> {
    try {
      const res = await fetch(`${API_URL}/v1/audit/reports/${id}`, {
        method: 'DELETE',
        headers: authHeaders(),
      })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error || 'Failed to delete report')
      }
      setReports(prev => prev.filter(r => r.id !== id))
      return true
    } catch {
      return false
    }
  }

  async function downloadReport(id: number, format: string): Promise<void> {
    const userId = localStorage.getItem('user_id') || ''
    const res = await fetch(`${API_URL}/v1/audit/reports/${id}/download`, {
      headers: { 'X-User-ID': userId },
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || 'Download failed')
    }
    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `audit-report-${id}.${format === 'PDF' ? 'pdf' : 'csv'}`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  return { reports, loading, error, refresh: fetchReports, generate, deleteReport, downloadReport }
}
