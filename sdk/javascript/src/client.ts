import type {
  NeuDriveConfig,
  Profile,
  Project,
  ProjectLog,
  VaultScope,
  InboxMessage,
  Device,
  ImportResult,
  FileTreeEntry,
  SearchResult,
  Skill,
  BundleFilters,
  BundlePreviewResult,
  DashboardStats,
  AgentAuthInfo,
  SyncJob,
  SyncSessionStatus,
  TreeSnapshot,
  TreeChanges,
  WriteFileOptions,
} from './types'

/**
 * NeuDriveError is thrown when the API returns a non-2xx response.
 */
export class NeuDriveError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: unknown,
  ) {
    const msg =
      typeof body === 'object' && body !== null && 'error' in body
        ? (body as { error: string }).error
        : `HTTP ${status}`
    super(msg)
    this.name = 'NeuDriveError'
  }
}

/**
 * Main client for neuDrive.
 *
 * Uses the `/agent/*` API surface authenticated via a scoped token
 * (ndt_xxxxx) sent as `Authorization: Bearer <token>`.
 *
 * @example
 * ```ts
 * const hub = new NeuDrive({ baseURL: 'https://hub.example.com', token: 'ndt_xxxxx' })
 * const profile = await hub.getProfile('preferences')
 * ```
 */
export class NeuDrive {
  private readonly baseURL: string
  private readonly token: string

  constructor(config: NeuDriveConfig) {
    if (!config.baseURL) throw new Error('NeuDrive: baseURL is required')
    if (!config.token) throw new Error('NeuDrive: token is required')
    this.baseURL = config.baseURL.replace(/\/+$/, '')
    this.token = config.token
  }

  // -------------------------------------------------------------------------
  // Internal helpers
  // -------------------------------------------------------------------------

  private headers(extra?: Record<string, string>): Record<string, string> {
    return {
      Authorization: `Bearer ${this.token}`,
      'Content-Type': 'application/json',
      ...extra,
    }
  }

  private async request<T = unknown>(
    method: string,
    path: string,
    body?: unknown,
    initOverride?: RequestInit,
  ): Promise<T> {
    const url = `${this.baseURL}${path}`
    const init: RequestInit = {
      method,
      headers: this.headers(),
      ...initOverride,
    }
    if (body !== undefined) {
      init.body = JSON.stringify(body)
    }
    const res = await fetch(url, init)
    if (!res.ok) {
      let errBody: unknown
      try {
        errBody = await res.json()
      } catch {
        errBody = await res.text()
      }
      throw new NeuDriveError(res.status, errBody)
    }
    // Some endpoints return 204 No Content
    if (res.status === 204) return undefined as T
    const data = (await res.json()) as T | { ok?: boolean; data?: T }
    if (data && typeof data === 'object' && 'ok' in data && 'data' in data) {
      return (data as { data: T }).data
    }
    return data as T
  }

  private get<T = unknown>(path: string): Promise<T> {
    return this.request<T>('GET', path)
  }

  private post<T = unknown>(path: string, body?: unknown): Promise<T> {
    return this.request<T>('POST', path, body)
  }

  private put<T = unknown>(path: string, body?: unknown): Promise<T> {
    return this.request<T>('PUT', path, body)
  }

  private async requestBytes(path: string): Promise<Uint8Array> {
    const url = `${this.baseURL}${path}`
    const res = await fetch(url, {
      method: 'GET',
      headers: {
        Authorization: `Bearer ${this.token}`,
      },
    })
    if (!res.ok) {
      let errBody: unknown
      try {
        errBody = await res.json()
      } catch {
        errBody = await res.text()
      }
      throw new NeuDriveError(res.status, errBody)
    }
    return new Uint8Array(await res.arrayBuffer())
  }

  // -------------------------------------------------------------------------
  // Profile
  // -------------------------------------------------------------------------

  /**
   * Get user profile entries, optionally filtered by category.
   */
  async getProfile(category?: string): Promise<Profile[]> {
    const qs = category ? `?category=${encodeURIComponent(category)}` : ''
    const res = await this.get<{ profiles?: Profile[] }>(`/agent/memory/profile${qs}`)
    return res.profiles ?? []
  }

  /**
   * Update (upsert) a profile category.
   */
  async updateProfile(category: string, content: string): Promise<void> {
    await this.put('/agent/memory/profile', { category, content })
  }

  // -------------------------------------------------------------------------
  // Memory / Search
  // -------------------------------------------------------------------------

  /**
   * Search memory, inbox, or both.
   */
  async searchMemory(
    query: string,
    scope: 'memory' | 'inbox' | 'all' = 'all',
  ): Promise<SearchResult[]> {
    const qs = `?q=${encodeURIComponent(query)}&scope=${encodeURIComponent(scope)}`
    const res = await this.get<{ results: SearchResult[] }>(
      `/agent/search${qs}`,
    )
    return res.results ?? []
  }

  // -------------------------------------------------------------------------
  // Projects
  // -------------------------------------------------------------------------

  /**
   * List all projects for the authenticated user.
   */
  async listProjects(): Promise<Project[]> {
    const res = await this.get<{ projects: Project[] }>('/agent/projects')
    return res.projects ?? []
  }

  /**
   * Get a single project with its logs.
   */
  async getProject(
    name: string,
  ): Promise<{ project: Project; logs: ProjectLog[] }> {
    return this.get<{ project: Project; logs: ProjectLog[] }>(`/agent/projects/${encodeURIComponent(name)}`)
  }

  /**
   * Append an action log entry to a project.
   */
  async logAction(
    project: string,
    action: string,
    summary: string,
    tags?: string[],
  ): Promise<void> {
    await this.post(`/agent/projects/${encodeURIComponent(project)}/log`, {
      action,
      summary,
      tags,
    })
  }

  // -------------------------------------------------------------------------
  // File Tree
  // -------------------------------------------------------------------------

  /**
   * List directory contents at the given path.
   */
  async listDirectory(path: string): Promise<FileTreeEntry[]> {
    const safePath = this.directoryPath(path)
    const res = await this.get<{ children: FileTreeEntry[] }>(`/agent/tree${safePath}`)
    return res.children ?? []
  }

  /**
   * Read a file's content from the file tree.
   */
  async readFile(path: string): Promise<string> {
    const safePath = this.filePath(path)
    const res = await this.get<{ content: string }>(`/agent/tree${safePath}`)
    return res.content ?? ''
  }

  /**
   * Write (create or overwrite) a file in the file tree.
   */
  async writeFile(path: string, content: string, options?: WriteFileOptions): Promise<void> {
    const safePath = this.filePath(path)
    await this.put(`/agent/tree${safePath}`, {
      content,
      content_type: options?.mime_type,
      metadata: options?.metadata,
      min_trust_level: options?.min_trust_level,
      expected_version: options?.expected_version,
      expected_checksum: options?.expected_checksum,
    })
  }

  /**
   * Snapshot a subtree for sync/bootstrap.
   */
  async snapshot(path: string = '/'): Promise<TreeSnapshot> {
    const qs = `?path=${encodeURIComponent(path)}`
    return this.get<TreeSnapshot>(`/agent/tree/snapshot${qs}`)
  }

  /**
   * Fetch incremental changes after a cursor.
   */
  async changes(cursor: number, path: string = '/'): Promise<TreeChanges> {
    const qs = `?cursor=${encodeURIComponent(String(cursor))}&path=${encodeURIComponent(path)}`
    return this.get<TreeChanges>(`/agent/tree/changes${qs}`)
  }

  // -------------------------------------------------------------------------
  // Vault
  // -------------------------------------------------------------------------

  /**
   * List all vault scopes visible to the current trust level.
   */
  async listSecrets(): Promise<VaultScope[]> {
    const res = await this.get<{ scopes: VaultScope[] }>('/agent/vault/scopes')
    return res.scopes ?? []
  }

  /**
   * Read a secret from the vault by scope name.
   */
  async readSecret(scope: string): Promise<string> {
    const res = await this.get<{ data: string }>(
      `/agent/vault/${encodeURIComponent(scope)}`,
    )
    return res.data ?? ''
  }

  // -------------------------------------------------------------------------
  // Skills
  // -------------------------------------------------------------------------

  /**
   * List available skills.
   */
  async listSkills(): Promise<Skill[]> {
    const res = await this.get<{ skills: Skill[] }>('/agent/skills')
    return res.skills ?? []
  }

  /**
   * Read a skill's content by name.
   */
  async readSkill(name: string): Promise<string> {
    const res = await this.get<{ content: string }>(
      `/agent/tree/skills/${encodeURIComponent(name)}/SKILL.md`,
    )
    return res.content ?? ''
  }

  // -------------------------------------------------------------------------
  // Devices
  // -------------------------------------------------------------------------

  /**
   * List registered devices.
   */
  async listDevices(): Promise<Device[]> {
    const res = await this.get<{ devices: Device[] }>('/agent/devices')
    return res.devices ?? []
  }

  /**
   * Call an action on a registered device.
   */
  async callDevice(
    device: string,
    action: string,
    params?: Record<string, unknown>,
  ): Promise<unknown> {
    return this.post(
      `/agent/devices/${encodeURIComponent(device)}/call`,
      { action, params },
    )
  }

  // -------------------------------------------------------------------------
  // Inbox
  // -------------------------------------------------------------------------

  /**
   * Send a message to another agent or role.
   */
  async sendMessage(
    to: string,
    subject: string,
    body: string,
    opts?: { domain?: string; tags?: string[] },
  ): Promise<void> {
    await this.post('/agent/inbox/send', {
      to,
      subject,
      body,
      domain: opts?.domain,
      tags: opts?.tags,
    })
  }

  /**
   * Read inbox messages, optionally filtered by role and/or status.
   */
  async readInbox(
    role: string = 'default',
    status?: string,
  ): Promise<InboxMessage[]> {
    const qs = status ? `?status=${encodeURIComponent(status)}` : ''
    const res = await this.get<{ messages: InboxMessage[] }>(
      `/agent/inbox/${encodeURIComponent(role)}${qs}`,
    )
    return res.messages ?? []
  }

  // -------------------------------------------------------------------------
  // Import
  // -------------------------------------------------------------------------

  /**
   * Import a skill (one or more files).
   */
  async importSkill(
    name: string,
    files: Record<string, string>,
  ): Promise<ImportResult> {
    const res = await this.post<{ ok: boolean; data: ImportResult }>(
      '/agent/import/skill',
      { name, files },
    )
    return res.data
  }

  /**
   * Import Claude-format memory entries.
   */
  async importClaudeMemory(
    memories: Array<{ content: string; type?: string; created_at?: string }>,
  ): Promise<ImportResult> {
    const res = await this.post<{ ok: boolean; data: ImportResult }>(
      '/agent/import/claude-memory',
      { memories },
    )
    return res.data
  }

  /**
   * Import profile fields (preferences, relationships, principles).
   */
  async importProfile(profile: {
    preferences?: string
    relationships?: string
    principles?: string
  }): Promise<ImportResult> {
    const res = await this.post<{ ok: boolean; data: ImportResult }>(
      '/agent/import/profile',
      profile,
    )
    return res.data
  }

  /**
   * Export all user data.
   */
  async exportAll(): Promise<unknown> {
    return this.get<unknown>('/agent/export/all')
  }

  // -------------------------------------------------------------------------
  // Dashboard
  // -------------------------------------------------------------------------

  /**
   * Get dashboard statistics.
   */
  async getStats(): Promise<DashboardStats> {
    return this.get<DashboardStats>('/agent/dashboard/stats')
  }

  // -------------------------------------------------------------------------
  // Bundle Sync
  // -------------------------------------------------------------------------

  async getAuthInfo(): Promise<AgentAuthInfo> {
    return this.get<AgentAuthInfo>('/agent/auth/whoami')
  }

  async previewBundle(payload: Record<string, unknown>): Promise<BundlePreviewResult> {
    return this.post<BundlePreviewResult>('/agent/import/preview', payload)
  }

  async importBundle(bundle: Record<string, unknown>): Promise<Record<string, unknown>> {
    return this.post<Record<string, unknown>>('/agent/import/bundle', bundle)
  }

  async exportBundle(
    format: 'json' | 'archive' = 'json',
    filters?: BundleFilters,
  ): Promise<Record<string, unknown> | Uint8Array> {
    const qs = this.bundleFilterQuery(filters)
    const prefix = qs ? `&${qs}` : ''
    if (format === 'archive') {
      return this.requestBytes(`/agent/export/bundle?format=archive${prefix}`)
    }
    return this.get<Record<string, unknown>>(`/agent/export/bundle?format=json${prefix}`)
  }

  async startSyncSession(request: Record<string, unknown>): Promise<SyncSessionStatus> {
    return this.post<SyncSessionStatus>('/agent/import/session', request)
  }

  async uploadPart(sessionId: string, index: number, data: Uint8Array): Promise<SyncSessionStatus> {
    return this.request<SyncSessionStatus>(
      'PUT',
      `/agent/import/session/${encodeURIComponent(sessionId)}/parts/${index}`,
      undefined,
      {
        headers: {
          Authorization: `Bearer ${this.token}`,
          'Content-Type': 'application/octet-stream',
        },
        body: data as BodyInit,
      },
    )
  }

  async getSyncSession(sessionId: string): Promise<SyncSessionStatus> {
    return this.get<SyncSessionStatus>(`/agent/import/session/${encodeURIComponent(sessionId)}`)
  }

  async commitSession(sessionId: string, previewFingerprint?: string): Promise<Record<string, unknown>> {
    return this.post<Record<string, unknown>>(
      `/agent/import/session/${encodeURIComponent(sessionId)}/commit`,
      previewFingerprint ? { preview_fingerprint: previewFingerprint } : {},
    )
  }

  async abortSession(sessionId: string): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('DELETE', `/agent/import/session/${encodeURIComponent(sessionId)}`)
  }

  async resumeSession(sessionId: string, archive: Uint8Array): Promise<SyncSessionStatus> {
    let state = await this.getSyncSession(sessionId)
    const chunkSize = Math.max(state.chunk_size_bytes || 1, 1)
    for (const index of state.missing_parts ?? []) {
      const start = index * chunkSize
      const end = Math.min(archive.length, start + chunkSize)
      state = await this.uploadPart(sessionId, index, archive.slice(start, end))
    }
    return state
  }

  async listSyncJobs(): Promise<SyncJob[]> {
    const res = await this.get<{ jobs: SyncJob[] }>('/agent/sync/jobs')
    return res.jobs ?? []
  }

  async getSyncJob(jobId: string): Promise<SyncJob> {
    return this.get<SyncJob>(`/agent/sync/jobs/${encodeURIComponent(jobId)}`)
  }

  private filePath(path: string): string {
    return path.startsWith('/') ? path : `/${path}`
  }

  private bundleFilterQuery(filters?: BundleFilters): string {
    const params = new URLSearchParams()
    for (const domain of filters?.include_domains ?? []) params.append('include_domain', domain)
    for (const skill of filters?.include_skills ?? []) params.append('include_skill', skill)
    for (const skill of filters?.exclude_skills ?? []) params.append('exclude_skill', skill)
    return params.toString()
  }

  private directoryPath(path: string): string {
    const safePath = this.filePath(path)
    if (safePath === '/') return safePath
    return safePath.endsWith('/') ? safePath : `${safePath}/`
  }
}
