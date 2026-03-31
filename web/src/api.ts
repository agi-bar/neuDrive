const API_BASE = '/api'

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const token = localStorage.getItem('token')
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options?.headers,
    },
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  return res.json()
}

export const api = {
  // Auth
  devLogin: (slug: string) =>
    request<{ token: string; user: any }>('/auth/token/dev', {
      method: 'POST',
      body: JSON.stringify({ slug }),
    }),
  getMe: () => request<any>('/auth/me'),

  // Dashboard
  getStats: () => request<any>('/dashboard/stats'),

  // Connections
  getConnections: () => request<any[]>('/connections'),
  createConnection: (data: any) =>
    request<any>('/connections', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateConnection: (id: string, data: any) =>
    request<any>(`/connections/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteConnection: (id: string) =>
    request<void>(`/connections/${id}`, { method: 'DELETE' }),

  // Memory
  getProfile: () => request<any[]>('/memory/profile'),
  upsertProfile: (data: any) =>
    request<any>('/memory/profile', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  // Projects
  getProjects: () => request<any[]>('/projects'),
  getProject: (name: string) => request<any>(`/projects/${name}`),

  // Vault
  getVaultScopes: () => request<any[]>('/vault/scopes'),

  // Roles
  getRoles: () => request<any[]>('/roles'),

  // Devices
  getDevices: () => request<any[]>('/devices'),

  // Inbox
  getInbox: (role: string) => request<any[]>(`/inbox/${role}`),

  // Import / Export
  importSkills: (skills: SkillFile[]) =>
    request<ImportResult>('/import/skills', {
      method: 'POST',
      body: JSON.stringify({ skills }),
    }),
  importClaudeMemory: (memories: ClaudeMemoryItem[]) =>
    request<ImportResult>('/import/claude-memory', {
      method: 'POST',
      body: JSON.stringify({ memories }),
    }),
  importProfile: (profile: ImportProfileRequest) =>
    request<ImportResult>('/import/profile', {
      method: 'POST',
      body: JSON.stringify(profile),
    }),
  importVault: (secrets: VaultSecretImport[]) =>
    request<ImportResult>('/import/vault', {
      method: 'POST',
      body: JSON.stringify({ secrets }),
    }),
  importDevices: (devices: DeviceImport[]) =>
    request<ImportResult>('/import/devices', {
      method: 'POST',
      body: JSON.stringify({ devices }),
    }),
  importFull: (data: FullHubExport) =>
    request<ImportResult>('/import/full', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  exportFull: () => request<FullHubExport>('/export/full'),
  uploadSkillsZip: (file: File) => {
    const formData = new FormData()
    formData.append('file', file)
    const token = localStorage.getItem('token')
    return fetch(`${API_BASE}/import/skills`, {
      method: 'POST',
      headers: {
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: formData,
    }).then(async (res) => {
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText }))
        throw new Error(err.error || res.statusText)
      }
      return res.json() as Promise<ImportResult>
    })
  },
}

// ---------------------------------------------------------------------------
// Import / Export types
// ---------------------------------------------------------------------------

export interface SkillFile {
  path: string
  content: string
  content_type?: string
}

export interface ClaudeMemoryItem {
  content: string
  source: string
  created_at?: string
}

export interface ImportProfileRequest {
  preferences?: string
  relationships?: string
  principles?: string
}

export interface VaultSecretImport {
  scope: string
  value: string
  description: string
  min_trust_level?: number
}

export interface DeviceImport {
  name: string
  device_type: string
  brand?: string
  protocol: string
  endpoint: string
  skill_md?: string
  config?: Record<string, any>
}

export interface ProjectExport {
  name: string
  status: string
  context_md: string
}

export interface FullHubExport {
  version: string
  exported_at: string
  user: any
  profile: Record<string, string>
  skills: SkillFile[]
  devices: DeviceImport[]
  projects: ProjectExport[]
  vault_scopes: string[]
}

export interface ImportResult {
  imported: number
  skipped: number
  errors?: string[]
}
