import {
  useCallback,
  useEffect,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react'
import { Outlet, useLocation, useOutletContext } from 'react-router-dom'
import { api, CreateTokenRequest, ScopedTokenResponse } from '../api'
import { useI18n } from '../i18n'

export const TRUST_LEVELS = [
  { value: 1, label: 'L1 访客', desc: '只能读取公开信息' },
  { value: 2, label: 'L2 共享', desc: '可读取有限共享资源' },
  { value: 3, label: 'L3 工作', desc: '可读写常规资源' },
  { value: 4, label: 'L4 完全信任', desc: '完整访问权限' },
]

export const EXPIRY_OPTIONS = [
  { value: 7, label: '7天' },
  { value: 30, label: '30天' },
  { value: 90, label: '90天' },
  { value: 365, label: '365天' },
  { value: 0, label: '永不过期' },
]

export type Preset = 'agent' | 'readonly' | 'sync' | 'custom'
export type ModeKey = 'local' | 'advanced'
export type CloudPlatformTab = 'claude' | 'codex' | 'gemini' | 'cursor'
export type LocalPlatformTab = 'claude' | 'codex'

interface ScopeInfo {
  scopes: string[]
  categories: Record<string, string[]>
  bundles: Record<string, string[]>
}

interface ModeTokenState {
  id: string
  token: string
}

const MODE_DEFAULTS: Record<ModeKey, { name: string; preset: Preset; trustLevel: number; expiryDays: number }> = {
  local: {
    name: 'Claude Code',
    preset: 'agent',
    trustLevel: 4,
    expiryDays: 90,
  },
  advanced: {
    name: 'MCP HTTP',
    preset: 'agent',
    trustLevel: 4,
    expiryDays: 90,
  },
}

const EMPTY_MODE_STATE: Record<ModeKey, boolean> = {
  local: false,
  advanced: false,
}

export const TOKEN_ENV_NAME = 'AGENTHUB_TOKEN'
export const TOKEN_PLACEHOLDER = '<YOUR_AGENTHUB_TOKEN>'

export interface SetupOutletContext {
  tokens: ScopedTokenResponse[]
  activeTokens: ScopedTokenResponse[]
  scopeInfo: ScopeInfo | null
  copied: string | null
  cloudPlatform: CloudPlatformTab
  setCloudPlatform: Dispatch<SetStateAction<CloudPlatformTab>>
  localPlatform: LocalPlatformTab
  setLocalPlatform: Dispatch<SetStateAction<LocalPlatformTab>>
  openModes: Record<ModeKey, boolean>
  modeTokens: Partial<Record<ModeKey, ModeTokenState>>
  provisioningMode: ModeKey | null
  name: string
  setName: Dispatch<SetStateAction<string>>
  preset: Preset
  setPreset: Dispatch<SetStateAction<Preset>>
  trustLevel: number
  setTrustLevel: Dispatch<SetStateAction<number>>
  expiryDays: number
  setExpiryDays: Dispatch<SetStateAction<number>>
  customScopes: string[]
  manualCreating: boolean
  newToken: string | null
  editingTokenId: string | null
  editingTokenName: string
  setEditingTokenName: Dispatch<SetStateAction<string>>
  renamingTokenId: string | null
  baseUrl: string
  cloudModeNeedsPublicUrl: boolean
  claudeCloudCommand: string
  codexCloudCommand: string
  geminiCloudCommand: string
  cursorAgentStatusCommand: string
  codexLoginCommand: string
  cursorAgentLoginCommand: string
  geminiAuthCommand: string
  codexStatusCommand: string
  localSessionToken: string
  advancedSessionToken: string
  localEnvCommand: string
  advancedEnvCommand: string
  localClaudeCommand: string
  localCodexCommand: string
  localConfig: string
  advancedCodexCommand: string
  advancedConfig: string
  gptTokenText: string
  getSelectedScopes: (selectedPreset?: Preset, selectedCustomScopes?: string[]) => string[]
  toggleScope: (scope: string) => void
  copyToClipboard: (text: string, key: string) => void
  formatExpiry: (token: ScopedTokenResponse) => string
  trustLabel: (level: number) => string
  presetLabel: (token: ScopedTokenResponse) => string
  provisionModeToken: (mode: ModeKey, force?: boolean) => Promise<ModeTokenState | null>
  toggleMode: (mode: ModeKey) => void
  handleCreateToken: () => Promise<void>
  handleRevoke: (id: string) => Promise<void>
  startRenameToken: (token: ScopedTokenResponse) => void
  cancelRenameToken: () => void
  handleRenameToken: (token: ScopedTokenResponse) => Promise<void>
}

export function useSetup() {
  return useOutletContext<SetupOutletContext>()
}

export default function SetupPage() {
  const { tx } = useI18n()
  const location = useLocation()
  const [tokens, setTokens] = useState<ScopedTokenResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [scopeInfo, setScopeInfo] = useState<ScopeInfo | null>(null)

  const [name, setName] = useState('Claude Code')
  const [preset, setPreset] = useState<Preset>('agent')
  const [trustLevel, setTrustLevel] = useState(4)
  const [expiryDays, setExpiryDays] = useState(90)
  const [customScopes, setCustomScopes] = useState<string[]>([])
  const [manualCreating, setManualCreating] = useState(false)
  const [newToken, setNewToken] = useState<string | null>(null)

  const [copied, setCopied] = useState<string | null>(null)
  const [openModes, setOpenModes] = useState<Record<ModeKey, boolean>>(EMPTY_MODE_STATE)
  const [modeTokens, setModeTokens] = useState<Partial<Record<ModeKey, ModeTokenState>>>({})
  const [provisioningMode, setProvisioningMode] = useState<ModeKey | null>(null)
  const [cloudPlatform, setCloudPlatform] = useState<CloudPlatformTab>('claude')
  const [localPlatform, setLocalPlatform] = useState<LocalPlatformTab>('claude')
  const [editingTokenId, setEditingTokenId] = useState<string | null>(null)
  const [editingTokenName, setEditingTokenName] = useState('')
  const [renamingTokenId, setRenamingTokenId] = useState<string | null>(null)

  const loadTokens = useCallback(async () => {
    try {
      const data = await api.getTokens()
      setTokens(data)
    } catch (e: any) {
      setError(e.message)
    }
  }, [])

  const loadScopes = useCallback(async () => {
    try {
      const data = await api.getTokenScopes()
      setScopeInfo(data)
    } catch {
      // non-critical
    }
  }, [])

  useEffect(() => {
    Promise.all([loadTokens(), loadScopes()]).finally(() => setLoading(false))
  }, [loadTokens, loadScopes])

  const getSelectedScopes = useCallback((
    selectedPreset: Preset = preset,
    selectedCustomScopes: string[] = customScopes,
  ): string[] => {
    if (selectedPreset === 'agent') {
      return scopeInfo?.bundles?.agent ?? [
        'read:profile', 'read:memory', 'write:memory',
        'read:skills', 'read:vault.auth',
        'read:projects', 'write:projects',
        'read:tree', 'write:tree',
        'search',
      ]
    }
    if (selectedPreset === 'readonly') {
      return scopeInfo?.bundles?.read_only ?? [
        'read:profile', 'read:memory', 'read:skills',
        'read:projects', 'read:tree', 'search',
      ]
    }
    if (selectedPreset === 'sync') {
      return ['read:bundle', 'write:bundle']
    }
    return selectedCustomScopes
  }, [customScopes, preset, scopeInfo])

  const toggleScope = useCallback((scope: string) => {
    setCustomScopes((prev) =>
      prev.includes(scope) ? prev.filter((item) => item !== scope) : [...prev, scope],
    )
  }, [])

  const copyToClipboard = useCallback((text: string, key: string) => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(key)
      setTimeout(() => setCopied(null), 2000)
    })
  }, [])

  const formatExpiry = useCallback((token: ScopedTokenResponse): string => {
    if (token.is_revoked) return tx('已吊销', 'Revoked')
    if (token.is_expired) return tx('已过期', 'Expired')
    const days = Math.ceil((new Date(token.expires_at).getTime() - Date.now()) / (1000 * 60 * 60 * 24))
    if (days > 3650) return tx('永不过期', 'Never expires')
    return tx(`${days}天后过期`, `Expires in ${days} days`)
  }, [tx])

  const trustLabel = useCallback((level: number): string => {
    switch (level) {
      case 1:
        return tx('L1 访客', 'L1 Visitor')
      case 2:
        return tx('L2 共享', 'L2 Shared')
      case 3:
        return tx('L3 工作', 'L3 Work')
      case 4:
        return tx('L4 完全信任', 'L4 Full Trust')
      default:
        return `L${level}`
    }
  }, [tx])

  const presetLabel = useCallback((token: ScopedTokenResponse): string => {
    const scopes = token.scopes
    if (scopes.includes('admin')) return 'Full'
    if (scopes.length === 2 && scopes.includes('read:bundle') && scopes.includes('write:bundle')) return 'Sync'
    if (scopes.length >= 13) return tx('Agent完整', 'Agent full')
    if (scopes.length <= 6 && scopes.every((scope) => scope.startsWith('read:') || scope === 'search')) return tx('只读', 'Read-only')
    return tx(`${scopes.length}项权限`, `${scopes.length} permissions`)
  }, [tx])

  const baseUrl = typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8080'
  const hostName = typeof window !== 'undefined' ? window.location.hostname : 'localhost'
  const isLocalOrigin = /^(localhost|127\.0\.0\.1|0\.0\.0\.0)$/.test(hostName) || hostName.endsWith('.local')
  const isSecureOrigin = baseUrl.startsWith('https://')
  const cloudModeNeedsPublicUrl = !isSecureOrigin || isLocalOrigin

  const claudeCloudCommand = `claude mcp add -s user --transport http agenthub \\
  ${baseUrl}/mcp`
  const codexCloudCommand = `codex mcp add agenthub --url ${baseUrl}/mcp`
  const geminiCloudCommand = `gemini mcp add --transport http agenthub ${baseUrl}/mcp`
  const cursorAgentLoginCommand = 'cursor-agent mcp login agenthub'
  const cursorAgentStatusCommand = 'cursor-agent mcp list'
  const codexLoginCommand = 'codex mcp login agenthub'
  const geminiAuthCommand = '/mcp auth agenthub'
  const codexStatusCommand = 'codex mcp list'
  const localSessionToken = modeTokens.local?.token ?? ''
  const advancedSessionToken = modeTokens.advanced?.token ?? ''
  const localTokenText = modeTokens.local?.token ?? TOKEN_PLACEHOLDER
  const advancedTokenText = modeTokens.advanced?.token ?? TOKEN_PLACEHOLDER
  const localEnvCommand = `export ${TOKEN_ENV_NAME}=${localTokenText}`
  const advancedEnvCommand = `export ${TOKEN_ENV_NAME}=${advancedTokenText}`
  const localClaudeCommand = `claude mcp add -s user agenthub -- agenthub-mcp --token-env ${TOKEN_ENV_NAME}`
  const localCodexCommand = `codex mcp add agenthub -- agenthub-mcp --token-env ${TOKEN_ENV_NAME}`
  const advancedCodexCommand = `codex mcp add agenthub --url ${baseUrl}/mcp --bearer-token-env-var ${TOKEN_ENV_NAME}`

  const buildModeTokenRequest = (mode: ModeKey): CreateTokenRequest => {
    const defaults = MODE_DEFAULTS[mode]
    const resolvedName = mode === 'local'
      ? localPlatform === 'codex'
        ? 'Codex CLI'
        : 'Claude Code'
      : defaults.name
    return {
      name: resolvedName,
      scopes: getSelectedScopes(defaults.preset),
      max_trust_level: defaults.trustLevel,
      expires_in_days: defaults.expiryDays,
    }
  }

  const provisionModeToken = useCallback(async (mode: ModeKey, force = false): Promise<ModeTokenState | null> => {
    const existing = modeTokens[mode]
    if (existing && !force) {
      setOpenModes((prev) => ({ ...prev, [mode]: true }))
      return existing
    }

    setProvisioningMode(mode)
    setError('')

    try {
      const resp = await api.createToken(buildModeTokenRequest(mode))
      const createdToken = {
        id: resp.scoped_token.id,
        token: resp.token,
      }
      setModeTokens((prev) => ({ ...prev, [mode]: createdToken }))
      setOpenModes((prev) => ({ ...prev, [mode]: true }))
      await loadTokens()
      return createdToken
    } catch (e: any) {
      setError(e.message)
      return null
    } finally {
      setProvisioningMode((current) => (current === mode ? null : current))
    }
  }, [getSelectedScopes, loadTokens, localPlatform, modeTokens])

  const toggleMode = useCallback((mode: ModeKey) => {
    const shouldOpen = !openModes[mode]
    setOpenModes((prev) => ({ ...prev, [mode]: shouldOpen }))
  }, [openModes])

  const localConfig = JSON.stringify({
    mcpServers: {
      agenthub: {
        command: 'agenthub-mcp',
        args: ['--token-env', TOKEN_ENV_NAME],
      },
    },
  }, null, 2)

  const advancedConfig = JSON.stringify({
    mcpServers: {
      agenthub: {
        type: 'http',
        url: `${baseUrl}/mcp`,
        headers: {
          Authorization: `Bearer ${advancedTokenText}`,
        },
      },
    },
  }, null, 2)

  const gptTokenText = newToken || tx('在 Token 管理页创建一个新的 Bearer Token 后填入这里', 'Create a new Bearer token in Token Manager and paste it here')

  const handleCreateToken = useCallback(async () => {
    setManualCreating(true)
    setError('')

    const req: CreateTokenRequest = {
      name,
      scopes: getSelectedScopes(),
      max_trust_level: trustLevel,
      expires_in_days: expiryDays === 0 ? 36500 : expiryDays,
    }

    try {
      const resp = await api.createToken(req)
      setNewToken(resp.token)
      await loadTokens()
    } catch (e: any) {
      setError(e.message)
    } finally {
      setManualCreating(false)
    }
  }, [expiryDays, getSelectedScopes, loadTokens, name, trustLevel])

  const handleRevoke = useCallback(async (id: string) => {
    try {
      await api.revokeToken(id)
      if (editingTokenId === id) {
        setEditingTokenId(null)
        setEditingTokenName('')
      }
      setModeTokens((prev) => {
        const next = { ...prev }
        if (next.local?.id === id) delete next.local
        if (next.advanced?.id === id) delete next.advanced
        return next
      })
      await loadTokens()
      setError('')
    } catch (e: any) {
      setError(e.message)
    }
  }, [editingTokenId, loadTokens])

  const startRenameToken = useCallback((token: ScopedTokenResponse) => {
    setEditingTokenId(token.id)
    setEditingTokenName(token.name)
    setError('')
  }, [])

  const cancelRenameToken = useCallback(() => {
    setEditingTokenId(null)
    setEditingTokenName('')
    setRenamingTokenId(null)
  }, [])

  const handleRenameToken = useCallback(async (token: ScopedTokenResponse) => {
    const trimmedName = editingTokenName.trim()
    if (!trimmedName) {
      setError(tx('Token 名称不能为空', 'Token name cannot be empty'))
      return
    }
    if (trimmedName === token.name) {
      cancelRenameToken()
      return
    }

    setRenamingTokenId(token.id)
    setError('')
    try {
      await api.updateToken(token.id, { name: trimmedName })
      await loadTokens()
      cancelRenameToken()
    } catch (e: any) {
      setError(e.message)
    } finally {
      setRenamingTokenId((current) => (current === token.id ? null : current))
    }
  }, [cancelRenameToken, editingTokenName, loadTokens])

  if (loading) {
    return <div className="page"><div className="page-loading">{tx('加载中...', 'Loading...')}</div></div>
  }

  const activeTokens = tokens.filter((token) => !token.is_revoked && !token.is_expired)
  const isTokenManagementRoute = location.pathname === '/setup/tokens'
  const pageTitle = isTokenManagementRoute ? tx('Token 管理', 'Token Manager') : tx('连接设置', 'Connection Setup')
  const pageSubtitle = isTokenManagementRoute
    ? tx('创建、改名、吊销和查看用于 API、脚本与高级 MCP 客户端的 Bearer Token。', 'Create, rename, revoke, and review Bearer tokens for APIs, scripts, and advanced MCP clients.')
    : tx('把 Agent Hub 配置到网页应用、CLI 和其他 MCP 客户端。', 'Configure Agent Hub for web apps, CLIs, and other MCP clients.')

  return (
    <div className="page materials-page">
      <div className="page-header page-header-stack">
        <div>
          <h2>{pageTitle}</h2>
          <p className="page-subtitle">{pageSubtitle}</p>
        </div>
      </div>

      {error && <div className="alert alert-error">{error}</div>}

      <Outlet
        context={{
          tokens,
          activeTokens,
          scopeInfo,
          copied,
          cloudPlatform,
          setCloudPlatform,
          localPlatform,
          setLocalPlatform,
          openModes,
          modeTokens,
          provisioningMode,
          name,
          setName,
          preset,
          setPreset,
          trustLevel,
          setTrustLevel,
          expiryDays,
          setExpiryDays,
          customScopes,
          manualCreating,
          newToken,
          editingTokenId,
          editingTokenName,
          setEditingTokenName,
          renamingTokenId,
          baseUrl,
          cloudModeNeedsPublicUrl,
          claudeCloudCommand,
          codexCloudCommand,
          geminiCloudCommand,
          cursorAgentStatusCommand,
          codexLoginCommand,
          cursorAgentLoginCommand,
          geminiAuthCommand,
          codexStatusCommand,
          localSessionToken,
          advancedSessionToken,
          localEnvCommand,
          advancedEnvCommand,
          localClaudeCommand,
          localCodexCommand,
          localConfig,
          advancedCodexCommand,
          advancedConfig,
          gptTokenText,
          getSelectedScopes,
          toggleScope,
          copyToClipboard,
          formatExpiry,
          trustLabel,
          presetLabel,
          provisionModeToken,
          toggleMode,
          handleCreateToken,
          handleRevoke,
          startRenameToken,
          cancelRenameToken,
          handleRenameToken,
        }}
      />
    </div>
  )
}
