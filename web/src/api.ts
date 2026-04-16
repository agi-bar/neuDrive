const API_BASE = '/api'

function parseDownloadFilename(contentDisposition: string | null, fallback: string) {
  if (!contentDisposition) return fallback
  const utf8Match = contentDisposition.match(/filename\*=UTF-8''([^;]+)/i)
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1])
    } catch {
      return utf8Match[1]
    }
  }
  const quotedMatch = contentDisposition.match(/filename="([^"]+)"/i)
  if (quotedMatch?.[1]) return quotedMatch[1]
  const plainMatch = contentDisposition.match(/filename=([^;]+)/i)
  if (plainMatch?.[1]) return plainMatch[1].trim()
  return fallback
}

function triggerBrowserDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  URL.revokeObjectURL(url)
}

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

export interface ConnectionResponse {
  id: string
  name: string
  platform: string
  trust_level: number
  api_key_prefix?: string
  last_used_at?: string
  created_at?: string
}

export interface OAuthAppResponse {
  id: string
  name: string
  client_id: string
  redirect_uris: string[]
  scopes: string[]
  description: string
  logo_url?: string
  is_active: boolean
  created_at: string
}

export interface OAuthGrantResponse {
  id: string
  app: OAuthAppResponse
  scopes: string[]
  created_at: string
}

export interface FileNode {
  path: string
  name: string
  is_dir: boolean
  source?: string
  kind?: string
  content?: string
  mime_type?: string
  size?: number
  version?: number
  checksum?: string
  metadata?: Record<string, any>
  children?: FileNode[]
  created_at?: string
  updated_at?: string
  deleted_at?: string
}

export interface TreeSnapshotResponse {
  path: string
  cursor: number
  root_checksum: string
  entries: FileNode[]
}

export interface DashboardActivity {
  platform: string
  count: number
}

export interface DashboardPending {
  type: string
  count: number
  message: string
}

export interface PublicConfig {
  github_client_id?: string
  github_enabled?: boolean
  github_app_enabled?: boolean
  github_app_slug?: string
  storage?: string
  local_mode?: boolean
  system_settings_enabled?: boolean
  git_mirror_execution_mode?: 'local' | 'hosted'
}

export interface DashboardStats {
  connections: number
  files: number
  memory: number
  profile: number
  skills: number
  projects: number
  weekly_activity: DashboardActivity[]
  pending: DashboardPending[]
}

export interface SkillSummary {
  name: string
  path: string
  bundle_path?: string
  primary_path?: string
  source: string
  read_only?: boolean
  description?: string
  when_to_use?: string
  capabilities?: string[]
  tags?: string[]
  min_trust_level?: number
}

export interface LocalGitSyncInfo {
  enabled: boolean
  path?: string
  execution_mode?: 'local' | 'hosted'
  sync_state?: 'idle' | 'queued' | 'running' | 'error'
  sync_requested_at?: string
  sync_started_at?: string
  sync_next_attempt_at?: string
  sync_attempt_count?: number
  synced: boolean
  last_synced_at?: string
  message?: string
  last_error?: string
  auto_commit_enabled?: boolean
  auto_push_enabled?: boolean
  auth_mode?: string
  remote_name?: string
  remote_branch?: string
  last_commit_at?: string
  last_commit_hash?: string
  last_push_at?: string
  last_push_error?: string
  commit_created?: boolean
  push_attempted?: boolean
  push_succeeded?: boolean
}

export interface RequestEnvelope<T> {
  data: T
  localGitSync?: LocalGitSyncInfo
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

async function requestWithMetadata<T>(path: string, options?: RequestInit): Promise<RequestEnvelope<T>> {
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
        throw new Error(err.message || err.error || retryRes.statusText)
      }
      const retryJson = await retryRes.json()
      if (retryJson && retryJson.ok === true && retryJson.data !== undefined) {
        return {
          data: retryJson.data,
          localGitSync: retryJson.local_git_sync,
        }
      }
      return { data: retryJson }
    }
    throw new Error('session expired')
  }

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.message || err.error || res.statusText)
  }
  const json = await res.json()
  // Unwrap APISuccess envelope: {ok: true, data: {...}} → return data
  if (json && json.ok === true && json.data !== undefined) {
    return {
      data: json.data,
      localGitSync: json.local_git_sync,
    }
  }
  return { data: json }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const result = await requestWithMetadata<T>(path, options)
  return result.data
}

async function requestEnvelope<T>(path: string, options?: RequestInit): Promise<RequestEnvelope<T>> {
  return requestWithMetadata<T>(path, options)
}

async function agentRequest<T>(path: string, token: string, options?: RequestInit): Promise<T> {
  const hasExplicitContentType =
    !!options?.headers &&
    typeof options.headers === 'object' &&
    !Array.isArray(options.headers) &&
    'Content-Type' in options.headers
  const res = await fetch(path, {
    ...options,
    headers: {
      Authorization: `Bearer ${token}`,
      ...(!hasExplicitContentType && !(options?.body instanceof FormData) ? { 'Content-Type': 'application/json' } : {}),
      ...options?.headers,
    },
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.message || err.error || res.statusText)
  }
  if (res.status === 204) {
    return undefined as T
  }
  const json = await res.json()
  if (json && json.ok === true && json.data !== undefined) {
    return json.data
  }
  return json
}

async function agentRequestBytes(path: string, token: string): Promise<Blob> {
  const res = await fetch(path, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.message || err.error || res.statusText)
  }
  return res.blob()
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

  getPublicConfig: (): Promise<PublicConfig> =>
    request<PublicConfig>('/config'),

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
  getStats: () => request<DashboardStats>('/dashboard/stats'),

  // Connections
  getConnections: () => request<{ connections: ConnectionResponse[] }>('/connections').then(r => r.connections || []),
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

  getOAuthGrants: () => request<{ grants: OAuthGrantResponse[] }>('/oauth/grants').then(r => r.grants || []),
  revokeOAuthGrant: (id: string) =>
    request<void>(`/oauth/grants/${id}`, { method: 'DELETE' }),

  // Memory
  getProfile: () => request<any>('/memory/profile'),
  upsertProfile: (data: any) =>
    request<any>('/memory/profile', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  // Projects
  getProjects: () => request<{ projects: any[] }>('/projects').then(r => r.projects),
  getProject: (name: string) => request<any>(`/projects/${name}`),
  createProject: (name: string) =>
    request<any>('/projects', {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),
  archiveProject: (name: string) =>
    request<any>(`/projects/${name}/archive`, { method: 'PUT' }),
  appendProjectLog: (name: string, data: { source: string; action: string; summary: string; tags?: string[] }) =>
    request<any>(`/projects/${name}/log`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Skills
  getSkills: () => request<{ skills: SkillSummary[] }>('/skills').then(r => r.skills || []),

  // File tree
  getTree: (path = '/'): Promise<FileNode> => {
    const normalized = path.startsWith('/') ? path : `/${path}`
    return request<FileNode>(`/tree${normalized}`)
  },
  downloadTreeZip: async (path: string) => {
    const normalized = path.startsWith('/') ? path : `/${path}`
    const token = localStorage.getItem('token')
    const fallbackName = `${normalized.split('/').filter(Boolean).slice(-1)[0] || 'root'}.zip`
    const res = await fetch(`${API_BASE}/tree/archive?path=${encodeURIComponent(normalized)}`, {
      headers: {
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }))
      throw new Error(err.message || err.error || res.statusText)
    }
    const blob = await res.blob()
    const filename = parseDownloadFilename(res.headers.get('Content-Disposition'), fallbackName)
    triggerBrowserDownload(blob, filename)
  },
  getTreeSnapshot: (path = '/'): Promise<TreeSnapshotResponse> =>
    request<TreeSnapshotResponse>(`/tree/snapshot?path=${encodeURIComponent(path)}`),

  search: (q: string): Promise<{ path: string; name: string; source?: string; kind?: string; content?: string; updated_at?: string }[]> =>
    request<{ query: string; results: any[] }>(`/search?q=${encodeURIComponent(q)}`)
      .then(r => (r.results || []).map((x: any) => ({
        path: x.path,
        name: x.name || x.path.split('/').pop() || x.path,
        source: x.source,
        kind: x.kind,
        content: x.content,
        updated_at: x.updated_at,
      }))),

  writeTree: (
    path: string,
    params: {
      content: string
      mimeType?: string
      isDir?: boolean
      metadata?: Record<string, any>
      expectedVersion?: number
      expectedChecksum?: string
      minTrustLevel?: number
    },
  ): Promise<FileNode> => {
    const normalized = path.startsWith('/') ? path : `/${path}`
    const body = {
      content: params.content,
      mime_type: params.mimeType || 'text/plain',
      is_dir: params.isDir || false,
      metadata: params.metadata,
      expected_version: params.expectedVersion,
      expected_checksum: params.expectedChecksum,
      min_trust_level: params.minTrustLevel,
    }
    return request<FileNode>(`/tree${normalized}` as string, {
      method: 'PUT',
      body: JSON.stringify(body),
    })
  },
  deleteTree: (path: string): Promise<{ status: string; path: string }> => {
    const normalized = path.startsWith('/') ? path : `/${path}`
    return request<{ status: string; path: string }>(`/tree${normalized}`, { method: 'DELETE' })
  },

  // Memory conflicts
  getConflicts: () => request<{ conflicts: MemoryConflict[] }>('/memory/conflicts').then(r => r.conflicts),
  resolveConflict: (id: string, resolution: string) =>
    request<{ status: string; resolution: string }>(`/memory/conflicts/${id}/resolve`, {
      method: 'POST',
      body: JSON.stringify({ resolution }),
    }),

  // Vault
  getVaultScopes: () => request<{ scopes: any[] }>('/vault/scopes').then(r => r.scopes || []),

  requestEnvelope,

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
  importFull: (data: FullHubExport) =>
    request<ImportResult>('/import/full', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  exportFull: () => request<FullHubExport>('/export/full'),
  exportZip: async () => {
    const token = localStorage.getItem('token')
    const res = await fetch(`${API_BASE}/export/zip`, {
      headers: {
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }))
      throw new Error(err.error || res.statusText)
    }
    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `neudrive-export-${new Date().toISOString().slice(0, 10)}.zip`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  },
  exportJSON: () => request<any>('/export/json'),

  // Token management
  getTokens: (): Promise<ScopedTokenResponse[]> =>
    request<{ tokens: ScopedTokenResponse[] }>('/tokens').then(r => r.tokens),

  createToken: (req: CreateTokenRequest): Promise<CreateTokenResponse> =>
    request<CreateTokenResponse>('/tokens', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  createSyncToken: (req: SyncTokenRequest): Promise<SyncTokenResponse> =>
    request<SyncTokenResponse>('/tokens/sync', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  updateToken: (id: string, req: UpdateTokenRequest): Promise<ScopedTokenResponse> =>
    request<ScopedTokenResponse>(`/tokens/${id}`, {
      method: 'PUT',
      body: JSON.stringify(req),
    }),

  revokeToken: (id: string): Promise<void> =>
    request<void>(`/tokens/${id}`, { method: 'DELETE' }),

  getTokenScopes: (): Promise<{ scopes: string[], categories: Record<string, string[]>, bundles: Record<string, string[]> }> =>
    request('/tokens/scopes'),
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

  previewBundle: (token: string, payload: any): Promise<BundlePreviewResult> =>
    agentRequest('/agent/import/preview', token, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  importBundle: (token: string, bundle: any): Promise<any> =>
    agentRequest('/agent/import/bundle', token, {
      method: 'POST',
      body: JSON.stringify(bundle),
    }),

  startSyncSession: (token: string, payload: any): Promise<SyncSessionStatus> =>
    agentRequest('/agent/import/session', token, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  uploadSyncPart: (token: string, sessionId: string, index: number, data: Uint8Array): Promise<SyncSessionStatus> =>
    agentRequest(`/agent/import/session/${sessionId}/parts/${index}`, token, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/octet-stream',
      },
      body: data as BodyInit,
    }),

  getSyncSession: (token: string, sessionId: string): Promise<SyncSessionStatus> =>
    agentRequest(`/agent/import/session/${sessionId}`, token),

  commitSyncSession: (token: string, sessionId: string, previewFingerprint?: string): Promise<any> =>
    agentRequest(`/agent/import/session/${sessionId}/commit`, token, {
      method: 'POST',
      body: JSON.stringify(previewFingerprint ? { preview_fingerprint: previewFingerprint } : {}),
    }),

  abortSyncSession: (token: string, sessionId: string): Promise<any> =>
    agentRequest(`/agent/import/session/${sessionId}`, token, { method: 'DELETE' }),

  listSyncJobs: (token: string): Promise<SyncJob[]> =>
    agentRequest<{ jobs: SyncJob[] }>('/agent/sync/jobs', token).then((data) => data.jobs || []),

  getSyncJob: (token: string, jobId: string): Promise<SyncJob> =>
    agentRequest(`/agent/sync/jobs/${jobId}`, token),

  exportBundle: (token: string, format: 'json' | 'archive', filters?: BundleFilters): Promise<any> => {
    const params = new URLSearchParams()
    params.set('format', format)
    for (const domain of filters?.include_domains || []) params.append('include_domain', domain)
    for (const skill of filters?.include_skills || []) params.append('include_skill', skill)
    for (const skill of filters?.exclude_skills || []) params.append('exclude_skill', skill)
    const path = `/agent/export/bundle?${params.toString()}`
    if (format === 'archive') return agentRequestBytes(path, token)
    return agentRequest(path, token)
  },

  getLocalConfig: (): Promise<LocalConfigFile> =>
    request<LocalConfigFile>('/local/config'),

  updateLocalConfig: (req: UpdateLocalConfigRequest): Promise<LocalConfigFile> =>
    request<LocalConfigFile>('/local/config', {
      method: 'PUT',
      body: JSON.stringify(req),
    }),

  getLocalGitMirror: (): Promise<GitMirrorSettings> =>
    request<GitMirrorSettings>('/local/git-mirror'),

  updateLocalGitMirror: (req: UpdateGitMirrorRequest): Promise<GitMirrorSettings> =>
    request<GitMirrorSettings>('/local/git-mirror', {
      method: 'PUT',
      body: JSON.stringify(req),
    }),

  testGitMirrorGitHubToken: (req: GitMirrorGitHubTestRequest): Promise<GitMirrorGitHubTestResult> =>
    request<GitMirrorGitHubTestResult>('/local/git-mirror/github/test', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  getGitMirror: (): Promise<GitMirrorSettings> =>
    request<GitMirrorSettings>('/git-mirror'),

  updateGitMirror: (req: UpdateGitMirrorRequest): Promise<GitMirrorSettings> =>
    request<GitMirrorSettings>('/git-mirror', {
      method: 'PUT',
      body: JSON.stringify(req),
    }),

  syncGitMirror: (): Promise<LocalGitSyncInfo> =>
    request<LocalGitSyncInfo>('/git-mirror/sync', {
      method: 'POST',
      body: JSON.stringify({}),
    }),

  testGitMirrorGitHubTokenGeneric: (req: GitMirrorGitHubTestRequest): Promise<GitMirrorGitHubTestResult> =>
    request<GitMirrorGitHubTestResult>('/git-mirror/github/test', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  startGitMirrorGitHubAppBrowser: (returnTo: string): Promise<GitMirrorGitHubAppBrowserStartResult> =>
    request<GitMirrorGitHubAppBrowserStartResult>('/git-mirror/github-app/browser/start', {
      method: 'POST',
      body: JSON.stringify({ return_to: returnTo }),
    }),

  disconnectGitMirrorGitHubAppUser: (): Promise<{ status: string }> =>
    request<{ status: string }>('/git-mirror/github-app/disconnect', {
      method: 'POST',
      body: JSON.stringify({}),
    }),

  listGitMirrorGitHubAppRepos: (): Promise<GitMirrorRepo[]> =>
    request<GitMirrorRepo[]>('/git-mirror/github-app/repos'),

  createGitMirrorGitHubAppRepo: (req: CreateGitMirrorRepoRequest): Promise<GitMirrorRepo> =>
    request<GitMirrorRepo>('/git-mirror/github-app/repos', {
      method: 'POST',
      body: JSON.stringify(req),
    }),
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
  projects: ProjectExport[]
  vault_scopes: string[]
}

export interface ImportResult {
  imported: number
  skipped: number
  errors?: string[]
  skills?: string[]
}

// ---------------------------------------------------------------------------
// Token types
// ---------------------------------------------------------------------------

export interface ScopedTokenResponse {
  id: string
  user_id: string
  name: string
  token_prefix: string
  scopes: string[]
  max_trust_level: number
  expires_at: string
  rate_limit: number
  request_count: number
  last_used_at?: string
  last_used_ip?: string
  created_at: string
  revoked_at?: string
  is_expired: boolean
  is_revoked: boolean
}

export interface CreateTokenRequest {
  name: string
  scopes: string[]
  max_trust_level: number
  expires_in_days: number
}

export interface UpdateTokenRequest {
  name: string
}

export interface CreateTokenResponse {
  token: string
  token_prefix: string
  scoped_token: ScopedTokenResponse
}

export interface SyncTokenRequest {
  access: 'push' | 'pull' | 'both'
  ttl_minutes: number
}

export interface SyncTokenResponse {
  token: string
  expires_at: string
  api_base: string
  scopes: string[]
  usage: string
}

export interface BundleFilters {
  include_domains?: string[]
  include_skills?: string[]
  exclude_skills?: string[]
}

export interface BundlePreviewResult {
  version: string
  mode: string
  fingerprint?: string
  summary: Record<string, number>
  profile?: Array<{ path: string; action: string; kind?: string }>
  memory?: Array<{ path: string; action: string; kind?: string }>
  skills?: Record<string, { summary: Record<string, number>; files?: Array<{ path: string; action: string; kind?: string }> }>
}

export interface SyncSessionStatus {
  session_id: string
  job_id: string
  status: string
  chunk_size_bytes: number
  total_parts: number
  expires_at: string
  mode?: string
  summary: Record<string, any>
  received_parts?: number[]
  missing_parts?: number[]
}

export interface SyncJob {
  id: string
  user_id: string
  session_id?: string
  direction: string
  transport: string
  status: string
  source?: string
  mode?: string
  filters?: BundleFilters
  summary?: Record<string, any>
  error?: string
  created_at: string
  updated_at: string
  completed_at?: string
}

export interface GitMirrorSettings {
  enabled: boolean
  path?: string
  execution_mode?: 'local' | 'hosted'
  sync_state?: 'idle' | 'queued' | 'running' | 'error'
  sync_requested_at?: string
  sync_started_at?: string
  sync_next_attempt_at?: string
  sync_attempt_count?: number
  auto_commit_enabled: boolean
  auto_push_enabled: boolean
  auth_mode: 'local_credentials' | 'github_token' | 'github_app_user'
  remote_name: string
  remote_url?: string
  remote_branch: string
  last_synced_at?: string
  last_error?: string
  last_commit_at?: string
  last_commit_hash?: string
  last_push_at?: string
  last_push_error?: string
  github_token_configured: boolean
  github_token_verified_at?: string
  github_token_login?: string
  github_repo_permission?: string
  github_app_user_connected: boolean
  github_app_user_login?: string
  github_app_user_authorized_at?: string
  github_app_user_refresh_expires_at?: string
  message?: string
}

export interface LocalConfigFile {
  path: string
  raw: string
}

export interface UpdateLocalConfigRequest {
  raw: string
}

export interface UpdateGitMirrorRequest {
  auto_commit_enabled: boolean
  auto_push_enabled: boolean
  auth_mode: 'local_credentials' | 'github_token' | 'github_app_user'
  remote_name?: string
  remote_url?: string
  remote_branch?: string
  github_token?: string
  clear_github_token?: boolean
}

export interface GitMirrorGitHubAppBrowserStartResult {
  authorization_url: string
}

export interface GitMirrorRepo {
  owner_login: string
  owner_type: string
  repo_name: string
  full_name: string
  default_branch?: string
  clone_url: string
  viewer_permission?: string
}

export interface CreateGitMirrorRepoRequest {
  owner_login: string
  repo_name: string
  description?: string
  private: boolean
  remote_name?: string
  remote_branch?: string
}

export interface GitMirrorGitHubTestRequest {
  remote_url: string
  github_token: string
}

export interface GitMirrorGitHubTestResult {
  ok: boolean
  login?: string
  repo?: string
  normalized_remote_url?: string
  permission?: string
  message?: string
}

// ---------------------------------------------------------------------------
// Memory conflict types
// ---------------------------------------------------------------------------

export interface MemoryConflict {
  id: string
  user_id: string
  category: string
  source_a: string
  content_a: string
  source_b: string
  content_b: string
  status: string
  resolved_at?: string
  created_at: string
}
