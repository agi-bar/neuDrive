import type {
  AgentHubConfig,
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
  DashboardStats,
} from './types'

/**
 * AgentHubError is thrown when the API returns a non-2xx response.
 */
export class AgentHubError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: unknown,
  ) {
    const msg =
      typeof body === 'object' && body !== null && 'error' in body
        ? (body as { error: string }).error
        : `HTTP ${status}`
    super(msg)
    this.name = 'AgentHubError'
  }
}

/**
 * Main client for Agent Hub.
 *
 * Uses the `/agent/*` API surface authenticated via a scoped token
 * (aht_xxxxx) sent as `Authorization: Bearer <token>`.
 *
 * @example
 * ```ts
 * const hub = new AgentHub({ baseURL: 'https://hub.example.com', token: 'aht_xxxxx' })
 * const profile = await hub.getProfile('preferences')
 * ```
 */
export class AgentHub {
  private readonly baseURL: string
  private readonly token: string

  constructor(config: AgentHubConfig) {
    if (!config.baseURL) throw new Error('AgentHub: baseURL is required')
    if (!config.token) throw new Error('AgentHub: token is required')
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
  ): Promise<T> {
    const url = `${this.baseURL}${path}`
    const init: RequestInit = {
      method,
      headers: this.headers(),
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
      throw new AgentHubError(res.status, errBody)
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
  async writeFile(path: string, content: string): Promise<void> {
    const safePath = this.filePath(path)
    await this.put(`/agent/tree${safePath}`, { content })
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

  private filePath(path: string): string {
    return path.startsWith('/') ? path : `/${path}`
  }

  private directoryPath(path: string): string {
    const safePath = this.filePath(path)
    if (safePath === '/') return safePath
    return safePath.endsWith('/') ? safePath : `${safePath}/`
  }
}
