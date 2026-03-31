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
}
