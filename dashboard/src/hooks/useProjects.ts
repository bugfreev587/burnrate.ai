import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../lib/api'

export interface Project {
  id: number
  tenant_id: number
  name: string
  description: string
  status: string
  is_default: boolean
  created_at: string
  updated_at: string
}

export interface ProjectMember {
  user_id: string
  email: string
  name: string
  project_role: string
  created_at: string
}

interface ProjectsState {
  projects: Project[]
  loading: boolean
  error: string | null
  limit: number | null
  slotsLeft: number | null
  plan: string
}

export function useProjects() {
  const [state, setState] = useState<ProjectsState>({
    projects: [],
    loading: true,
    error: null,
    limit: null,
    slotsLeft: null,
    plan: '',
  })

  const fetchProjects = useCallback(async () => {
    try {
      const res = await apiFetch('/v1/projects')
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error || d.message || `HTTP ${res.status}`)
      }
      const data = await res.json()
      setState({
        projects: data.projects ?? [],
        loading: false,
        error: null,
        limit: data.limit ?? null,
        slotsLeft: data.slots_left ?? null,
        plan: data.plan ?? '',
      })
    } catch (err) {
      setState(prev => ({
        ...prev,
        loading: false,
        error: err instanceof Error ? err.message : 'Failed to fetch projects',
      }))
    }
  }, [])

  useEffect(() => { fetchProjects() }, [fetchProjects])

  const createProject = useCallback(async (name: string, description?: string) => {
    const res = await apiFetch('/v1/projects', {
      method: 'POST',
      body: JSON.stringify({ name, description: description ?? '' }),
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || d.message || 'Failed to create project')
    }
    const project = await res.json()
    await fetchProjects()
    return project as Project
  }, [fetchProjects])

  const updateProject = useCallback(async (id: number, updates: { name?: string; description?: string }) => {
    const res = await apiFetch(`/v1/projects/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(updates),
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || d.message || 'Failed to update project')
    }
    await fetchProjects()
  }, [fetchProjects])

  const deleteProject = useCallback(async (id: number) => {
    const res = await apiFetch(`/v1/projects/${id}`, { method: 'DELETE' })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || d.message || 'Failed to delete project')
    }
    await fetchProjects()
  }, [fetchProjects])

  const listMembers = useCallback(async (projectId: number): Promise<ProjectMember[]> => {
    const res = await apiFetch(`/v1/projects/${projectId}/members`)
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || d.message || 'Failed to list members')
    }
    const data = await res.json()
    return data.members ?? []
  }, [])

  const addMember = useCallback(async (projectId: number, userId: string, projectRole?: string) => {
    const res = await apiFetch(`/v1/projects/${projectId}/members`, {
      method: 'POST',
      body: JSON.stringify({ user_id: userId, project_role: projectRole }),
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || d.message || 'Failed to add member')
    }
  }, [])

  const updateMemberRole = useCallback(async (projectId: number, userId: string, projectRole: string) => {
    const res = await apiFetch(`/v1/projects/${projectId}/members/${userId}`, {
      method: 'PATCH',
      body: JSON.stringify({ project_role: projectRole }),
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || d.message || 'Failed to update member role')
    }
  }, [])

  const removeMember = useCallback(async (projectId: number, userId: string) => {
    const res = await apiFetch(`/v1/projects/${projectId}/members/${userId}`, { method: 'DELETE' })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || d.message || 'Failed to remove member')
    }
  }, [])

  return {
    ...state,
    refetch: fetchProjects,
    createProject,
    updateProject,
    deleteProject,
    listMembers,
    addMember,
    updateMemberRole,
    removeMember,
  }
}
