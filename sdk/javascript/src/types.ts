// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

export interface AgentHubConfig {
  /** Base URL of the Agent Hub instance (e.g. "https://hub.example.com") */
  baseURL: string
  /** Scoped token (aht_xxxxx) for agent/MCP authentication */
  token?: string
  /** OAuth client ID for third-party app flow */
  clientId?: string
  /** OAuth client secret for third-party app flow */
  clientSecret?: string
}

// ---------------------------------------------------------------------------
// Core domain types
// ---------------------------------------------------------------------------

export interface User {
  id: string
  slug: string
  display_name: string
  email?: string
  avatar_url?: string
  bio?: string
  timezone: string
  language: string
  created_at: string
  updated_at: string
}

export interface Profile {
  id: string
  user_id: string
  category: string
  content: string
  source: string
  created_at: string
  updated_at: string
}

export interface Project {
  id: string
  user_id: string
  name: string
  status: string
  context_md: string
  metadata?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface ProjectLog {
  id: string
  project_id: string
  source: string
  role: string
  action: string
  summary: string
  artifacts: string[]
  tags: string[]
  created_at: string
}

export interface VaultScope {
  id: string
  scope: string
  description: string
  min_trust_level: number
  created_at: string
}

export interface InboxMessage {
  id: string
  from_address: string
  to_address: string
  thread_id?: string
  priority: string
  action_required: boolean
  ttl?: string
  expires_at?: string
  domain: string
  action_type?: string
  tags?: string[]
  context_hash?: string
  subject: string
  body: string
  structured_payload?: Record<string, unknown>
  attachments?: string[]
  status: string
  created_at: string
  archived_at?: string
}

export interface Device {
  id: string
  user_id: string
  name: string
  device_type: string
  brand: string
  protocol: string
  endpoint: string
  skill_md: string
  config?: Record<string, unknown>
  status: string
  created_at: string
  updated_at: string
}

export interface ImportResult {
  imported_count: number
  paths?: string[]
  errors?: string[]
}

export interface FileTreeEntry {
  name: string
  path: string
  is_dir: boolean
  kind?: string
  content?: string
  mime_type?: string
  version?: number
  checksum?: string
  metadata?: Record<string, unknown>
  children?: FileTreeEntry[]
  size?: number
  updated_at?: string
  deleted_at?: string
}

export interface SearchResult {
  path: string
  type?: string
  snippet: string
  score?: number
}

export interface Skill {
  name: string
  path?: string
  source?: string
  description?: string
  when_to_use?: string
  allowed_tools?: string[]
  tags?: string[]
}

export interface WriteFileOptions {
  mime_type?: string
  metadata?: Record<string, unknown>
  min_trust_level?: number
  expected_version?: number
  expected_checksum?: string
}

export interface TreeSnapshot {
  path: string
  cursor: number
  root_checksum: string
  entries: FileTreeEntry[]
}

export interface TreeChange {
  cursor: number
  change_type: string
  entry: FileTreeEntry
}

export interface TreeChanges {
  path: string
  from_cursor: number
  next_cursor: number
  changes: TreeChange[]
}

export interface DashboardStats {
  connections: number
  skills: number
  devices: number
  projects: number
  weekly_activity: { platform: string; count: number }[]
  pending: { type: string; count: number; message: string }[]
}

// ---------------------------------------------------------------------------
// Auth types
// ---------------------------------------------------------------------------

export interface AuthTokenResponse {
  access_token: string
  refresh_token?: string
  expires_in?: number
  user: User
}

// ---------------------------------------------------------------------------
// API response envelope
// ---------------------------------------------------------------------------

export interface APIResponse<T = unknown> {
  ok: boolean
  data?: T
  error?: string
}
