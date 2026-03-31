const API_BASE = '/api'

// ---------------------------------------------------------------------------
// Auth types
// ---------------------------------------------------------------------------

export interface RegisterRequest {
  email: string
  password: string
  display_name: string
  slug: string
}

export interface LoginRequest {
  email: string
  password: string
}

export interface AuthResponse {
  access_token: string
  refresh_token: string
  expires_in: number
  user: any
}

export interface Session {
  id: string
  user_id: string
  user_agent: string
  ip_address: string
  expires_at: string
  created_at: string
}

// ---------------------------------------------------------------------------
// Token refresh logic
// ---------------------------------------------------------------------------

let isRefreshing = false
let refreshPromise: Promise<AuthResponse | null> | null = null

async function doRefreshToken(): Promise<AuthResponse | null> {
  const refreshToken = localStorage.getItem('refresh_token')
  if (!refreshToken) return null

  try {
    const res = await fetch(`${API_BASE}/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    })
    if (!res.ok) {
      localStorage.removeItem('token')
      localStorage.removeItem('refresh_token')
      return null
    }
    const data: AuthResponse = await res.json()
    localStorage.setItem('token', data.access_token)
    localStorage.setItem('refresh_token', data.refresh_token)
    return data
  } catch {
    localStorage.removeItem('token')
    localStorage.removeItem('refresh_token')
    return null
  }
}

// ---------------------------------------------------------------------------
// Core request function with automatic 401 refresh
// ---------------------------------------------------------------------------

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

  // If 401, try to refresh the token once
  if (res.status === 401 && localStorage.getItem('refresh_token')) {
    if (!isRefreshing) {
      isRefreshing = true
      refreshPromise = doRefreshToken().finally(() => {
        isRefreshing = false
        refreshPromise = null
      })
    }

    const refreshResult = await (refreshPromise || doRefreshToken())
    if (refreshResult) {
      // Retry the original request with the new token
      const retryRes = await fetch(`${API_BASE}${path}`, {
        ...options,
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${refreshResult.access_token}`,
          ...options?.headers,
        },
      })
      if (!retryRes.ok) {
        const err = await retryRes.json().catch(() => ({ error: retryRes.statusText }))
        throw new Error(err.error || retryRes.statusText)
      }
      return retryRes.json()
    }
    throw new Error('session expired')
  }

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  return res.json()
}

export const api = {
  // Auth
  register: (req: RegisterRequest): Promise<AuthResponse> =>
    request<AuthResponse>('/auth/register', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  login: (req: LoginRequest): Promise<AuthResponse> =>
    request<AuthResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  refreshToken: (refreshToken: string): Promise<AuthResponse> =>
    request<AuthResponse>('/auth/refresh', {
      method: 'POST',
      body: JSON.stringify({ refresh_token: refreshToken }),
    }),

  logout: async (): Promise<void> => {
    const refreshToken = localStorage.getItem('refresh_token')
    if (refreshToken) {
      try {
        await request<any>('/auth/logout', {
          method: 'POST',
          body: JSON.stringify({ refresh_token: refreshToken }),
        })
      } catch {
        // Ignore errors on logout
      }
    }
    localStorage.removeItem('token')
    localStorage.removeItem('refresh_token')
  },

  githubLogin: (code: string): Promise<AuthResponse> =>
    request<AuthResponse>('/auth/github/callback', {
      method: 'POST',
      body: JSON.stringify({ code }),
    }),

  getSessions: (): Promise<Session[]> =>
    request<Session[]>('/auth/sessions'),

  revokeSession: (id: string): Promise<void> =>
    request<void>(`/auth/sessions/${id}`, { method: 'DELETE' }),

  devLogin: (slug: string) =>
    request<{ token: string; user: any }>('/auth/token/dev', {
      method: 'POST',
      body: JSON.stringify({ slug }),
    }),

  getMe: () => request<any>('/auth/me'),

  updateMe: (data: { display_name: string; bio: string; timezone: string; language: string }) =>
    request<any>('/auth/me', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  changePassword: (oldPassword: string, newPassword: string) =>
    request<{ status: string }>('/auth/change-password', {
      method: 'POST',
      body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
    }),

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
