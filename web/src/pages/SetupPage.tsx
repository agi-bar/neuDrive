import { useCallback, useEffect, useState } from 'react'
import { api, CreateTokenRequest, ScopedTokenResponse } from '../api'

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const TRUST_LEVELS = [
  { value: 1, label: 'L1 访客', desc: '只能读取公开信息' },
  { value: 2, label: 'L2 协作', desc: '可读取大部分资源' },
  { value: 3, label: 'L3 工作', desc: '可读写常规资源' },
  { value: 4, label: 'L4 完全信任', desc: '完整访问权限' },
]

const EXPIRY_OPTIONS = [
  { value: 7, label: '7天' },
  { value: 30, label: '30天' },
  { value: 90, label: '90天' },
  { value: 365, label: '365天' },
  { value: 0, label: '永不过期' },
]

type Preset = 'agent' | 'readonly' | 'custom'
type ModeKey = 'local' | 'advanced'
type PlatformTab = 'claude' | 'codex'

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

const TOKEN_ENV_NAME = 'AGENTHUB_TOKEN'
const TOKEN_PLACEHOLDER = '<YOUR_AGENTHUB_TOKEN>'

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function SetupPage() {
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
  const [cloudPlatform, setCloudPlatform] = useState<PlatformTab>('claude')
  const [localPlatform, setLocalPlatform] = useState<PlatformTab>('claude')
  const [editingTokenId, setEditingTokenId] = useState<string | null>(null)
  const [editingTokenName, setEditingTokenName] = useState('')
  const [renamingTokenId, setRenamingTokenId] = useState<string | null>(null)

  // ---------------------------------------------------------------------------
  // Data loading
  // ---------------------------------------------------------------------------

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

  // ---------------------------------------------------------------------------
  // Helpers
  // ---------------------------------------------------------------------------

  const getSelectedScopes = (
    selectedPreset: Preset = preset,
    selectedCustomScopes: string[] = customScopes,
  ): string[] => {
    if (selectedPreset === 'agent') {
      return scopeInfo?.bundles?.agent ?? [
        'read:profile', 'read:memory', 'write:memory',
        'read:skills', 'read:vault.auth',
        'read:devices', 'call:devices',
        'read:inbox', 'write:inbox',
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
    return selectedCustomScopes
  }

  const toggleScope = (scope: string) => {
    setCustomScopes((prev) =>
      prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope],
    )
  }

  const copyToClipboard = (text: string, key: string) => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(key)
      setTimeout(() => setCopied(null), 2000)
    })
  }

  const formatExpiry = (token: ScopedTokenResponse): string => {
    if (token.is_revoked) return '已吊销'
    if (token.is_expired) return '已过期'
    const days = Math.ceil((new Date(token.expires_at).getTime() - Date.now()) / (1000 * 60 * 60 * 24))
    if (days > 3650) return '永不过期'
    return `${days}天后过期`
  }

  const trustLabel = (level: number): string => {
    const found = TRUST_LEVELS.find((item) => item.value === level)
    return found ? found.label : `L${level}`
  }

  const presetLabel = (token: ScopedTokenResponse): string => {
    const scopes = token.scopes
    if (scopes.includes('admin')) return 'Full'
    if (scopes.length >= 13) return 'Agent完整'
    if (scopes.length <= 6 && scopes.every((scope) => scope.startsWith('read:') || scope === 'search')) return '只读'
    return `${scopes.length}项权限`
  }

  const baseUrl = typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8080'
  const hostName = typeof window !== 'undefined' ? window.location.hostname : 'localhost'
  const isLocalOrigin = /^(localhost|127\.0\.0\.1|0\.0\.0\.0)$/.test(hostName) || hostName.endsWith('.local')
  const isSecureOrigin = baseUrl.startsWith('https://')
  const cloudModeNeedsPublicUrl = !isSecureOrigin || isLocalOrigin

  const claudeCloudCommand = `claude mcp add -s user --transport http agenthub \\
  ${baseUrl}/mcp`
  const codexCloudCommand = `codex mcp add agenthub --url ${baseUrl}/mcp`
  const codexLoginCommand = 'codex mcp login agenthub'
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

  const provisionModeToken = async (mode: ModeKey, force = false): Promise<ModeTokenState | null> => {
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
  }

  const toggleMode = (mode: ModeKey) => {
    const shouldOpen = !openModes[mode]
    setOpenModes((prev) => ({ ...prev, [mode]: shouldOpen }))
  }

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

  const gptTokenText = newToken || '在下方创建一个新的 Bearer Token 后填入这里'

  const scrollToTokenCreator = () => {
    document.getElementById('token-creator')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  // ---------------------------------------------------------------------------
  // Actions
  // ---------------------------------------------------------------------------

  const handleCreateToken = async () => {
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
  }

  const handleRevoke = async (id: string) => {
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
  }

  const startRenameToken = (token: ScopedTokenResponse) => {
    setEditingTokenId(token.id)
    setEditingTokenName(token.name)
    setError('')
  }

  const cancelRenameToken = () => {
    setEditingTokenId(null)
    setEditingTokenName('')
    setRenamingTokenId(null)
  }

  const handleRenameToken = async (token: ScopedTokenResponse) => {
    const trimmedName = editingTokenName.trim()
    if (!trimmedName) {
      setError('Token 名称不能为空')
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
  }

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  if (loading) {
    return <div className="page"><div className="page-loading">加载中...</div></div>
  }

  const activeTokens = tokens.filter((token) => !token.is_revoked && !token.is_expired)

  return (
    <div className="page">
      <div className="page-header">
        <h2>连接设置</h2>
      </div>

      {error && <div className="alert alert-error">{error}</div>}

      <div className="setup-section setup-section-highlight">
        <div className="setup-section-header">
          <span className="setup-section-icon">&#9729;</span>
          <div>
            <h3>云端模式（浏览器授权） <span className="badge badge-platform" style={{ marginLeft: 8, fontSize: 11 }}>推荐</span></h3>
            <p className="setup-section-desc">通过远程 HTTP MCP Server 连接 Agent Hub。默认添加到全局配置，设置一次，可在多个项目中复用。</p>
          </div>
        </div>

        {cloudModeNeedsPublicUrl && (
          <div className="alert alert-warn">
            当前地址是 <code>{baseUrl}</code>。云端模式需要可公开访问的 HTTPS Hub URL；如果你现在在本地开发，建议先用本地模式，或通过公网域名 / 隧道暴露这个 Hub。
          </div>
        )}

        <div className="setup-tabs" role="tablist" aria-label="云端模式平台">
          <button
            type="button"
            role="tab"
            className={`setup-tab ${cloudPlatform === 'claude' ? 'setup-tab-active' : ''}`}
            aria-selected={cloudPlatform === 'claude'}
            onClick={() => setCloudPlatform('claude')}
          >
            Claude
          </button>
          <button
            type="button"
            role="tab"
            className={`setup-tab ${cloudPlatform === 'codex' ? 'setup-tab-active' : ''}`}
            aria-selected={cloudPlatform === 'codex'}
            onClick={() => setCloudPlatform('codex')}
          >
            Codex
          </button>
        </div>

        <div className="setup-tab-panel">
          {cloudPlatform === 'claude' ? (
            <>
              <h4 className="setup-platform-title">Claude Code</h4>
              <p className="setup-note setup-note-first">
                把 Agent Hub 添加到 Claude Code 的全局 MCP 配置中，然后在 Claude Code 里发起浏览器授权。
              </p>

              <div className="code-block">
                <div className="code-block-label">步骤 1：添加远程 MCP Server（全局）</div>
                <pre>{claudeCloudCommand}</pre>
                <button
                  className="copy-btn"
                  onClick={() => copyToClipboard(claudeCloudCommand, 'cloud-claude-cmd')}
                >
                  {copied === 'cloud-claude-cmd' ? '已复制' : '复制'}
                </button>
              </div>

              <div className="code-block">
                <div className="code-block-label">步骤 2：在 Claude Code 中发起授权</div>
                <pre>/mcp</pre>
                <button
                  className="copy-btn"
                  onClick={() => copyToClipboard('/mcp', 'cloud-claude-auth')}
                >
                  {copied === 'cloud-claude-auth' ? '已复制' : '复制'}
                </button>
              </div>

              <ol className="setup-steps">
                <li>运行上面的命令后，Agent Hub 会作为全局 MCP Server 出现在 Claude Code 中。</li>
                <li>打开 Claude Code，执行 <code>/mcp</code>，选择 <code>agenthub</code>，然后开始认证。</li>
                <li>浏览器会打开授权页面；完成登录和批准后，Claude Code 会自动保存并刷新凭证。</li>
                <li>如果浏览器没有自动打开，就手动复制 Claude Code 提供的授权链接；如果网页授权完成后 CLI 仍提示等待，把浏览器地址栏里的完整 callback URL 粘回 Claude Code。</li>
              </ol>

              <p className="setup-note">
                授权完成后，你可以在 Claude Code 的 <code>/mcp</code> 菜单里重新认证或清除认证；Agent Hub 侧也会在“连接管理”中显示这条平台连接。
              </p>
            </>
          ) : (
            <>
              <h4 className="setup-platform-title">Codex CLI</h4>
              <p className="setup-note setup-note-first">
                把 Agent Hub 添加到 Codex 的全局 MCP 配置中，然后用 Codex CLI 发起浏览器授权。
              </p>

              <div className="code-block">
                <div className="code-block-label">步骤 1：添加远程 MCP Server（全局）</div>
                <pre>{codexCloudCommand}</pre>
                <button
                  className="copy-btn"
                  onClick={() => copyToClipboard(codexCloudCommand, 'cloud-codex-add')}
                >
                  {copied === 'cloud-codex-add' ? '已复制' : '复制'}
                </button>
              </div>

              <div className="code-block">
                <div className="code-block-label">步骤 2：发起授权</div>
                <pre>{codexLoginCommand}</pre>
                <button
                  className="copy-btn"
                  onClick={() => copyToClipboard(codexLoginCommand, 'cloud-codex-login')}
                >
                  {copied === 'cloud-codex-login' ? '已复制' : '复制'}
                </button>
              </div>

              <div className="code-block">
                <div className="code-block-label">步骤 3：确认连接状态</div>
                <pre>{codexStatusCommand}</pre>
                <button
                  className="copy-btn"
                  onClick={() => copyToClipboard(codexStatusCommand, 'cloud-codex-list')}
                >
                  {copied === 'cloud-codex-list' ? '已复制' : '复制'}
                </button>
              </div>

              <ol className="setup-steps">
                <li>运行 add 命令后，Agent Hub 会写入 Codex 的用户级 MCP 配置，可在多个工作区复用。</li>
                <li>运行 <code>codex mcp login agenthub</code> 后，浏览器会打开授权页面。</li>
                <li>完成登录和批准后，Codex 会保存 OAuth 凭证；再次运行 <code>codex mcp list</code> 可以查看连接状态。</li>
                <li>如果浏览器没有自动打开，就手动复制终端里提供的授权链接继续完成授权。</li>
              </ol>

              <p className="setup-note">
                授权完成后，Agent Hub 侧会在“连接管理”中显示这条平台连接；需要重新认证时，可再次运行 <code>codex mcp login agenthub</code>。
              </p>
            </>
          )}
        </div>

        <p className="setup-note">
          如果你本机已经有一个同名的本地 MCP 配置，例如旧的 <code>agenthub</code> stdio 配置，建议先删除或改名，避免在平台列表中和云端连接混淆。
        </p>
      </div>

      <div className="setup-section">
        <div className="setup-section-header">
          <span className="setup-section-icon">&#128187;</span>
          <div>
            <h3>本地模式（stdio + Token）</h3>
            <p className="setup-section-desc">通过本地 `agenthub-mcp` binary + scoped token 连接，适合本地开发或内网环境</p>
          </div>
        </div>

        <p className="setup-note">
          说明默认直接可看，不会自动创建 token。推荐把 token 放进环境变量 <code>{TOKEN_ENV_NAME}</code>，再让 Claude Code 或 Codex CLI 在启动本地 MCP binary 时读取它。
        </p>

        <div className="setup-tabs" role="tablist" aria-label="本地模式平台">
          <button
            type="button"
            role="tab"
            className={`setup-tab ${localPlatform === 'claude' ? 'setup-tab-active' : ''}`}
            aria-selected={localPlatform === 'claude'}
            onClick={() => setLocalPlatform('claude')}
          >
            Claude
          </button>
          <button
            type="button"
            role="tab"
            className={`setup-tab ${localPlatform === 'codex' ? 'setup-tab-active' : ''}`}
            aria-selected={localPlatform === 'codex'}
            onClick={() => setLocalPlatform('codex')}
          >
            Codex
          </button>
        </div>

        <div className="setup-mode-actions">
          <button
            className="btn btn-primary"
            onClick={() => toggleMode('local')}
          >
            {openModes.local ? '隐藏本地模式配置' : '查看本地模式配置'}
          </button>
          {openModes.local && (
            <button
              className="btn btn-outline"
              onClick={() => provisionModeToken('local', !!modeTokens.local)}
              disabled={provisioningMode === 'local'}
            >
              {provisioningMode === 'local'
                ? '生成中...'
                : modeTokens.local
                  ? '重新生成 Token'
                  : '创建本模式 Token'}
            </button>
          )}
        </div>

        {openModes.local && (
          <div className="setup-tab-panel">
            {modeTokens.local ? (
              <>
                <div className="alert alert-success">
                  已为本地模式创建一个新的 token。推荐下一步把它保存到环境变量 <code>{TOKEN_ENV_NAME}</code>；完整值只会在当前页面会话里显示一次，丢失后需要重新生成。
                </div>
                <div className="code-block">
                  <div className="code-block-label">刚创建的 Token（仅当前会话可见）</div>
                  <pre>{localSessionToken}</pre>
                  <button
                    className="copy-btn"
                    onClick={() => copyToClipboard(localSessionToken, 'local-token')}
                  >
                    {copied === 'local-token' ? '已复制' : '复制 Token'}
                  </button>
                </div>
              </>
            ) : (
              <div className="alert alert-warn">
                当前显示的是环境变量和配置模板，里面的 <code>{TOKEN_PLACEHOLDER}</code> 只是占位符。查看接法不需要新建 token；如果你要立即接入，再点上面的“创建本模式 Token”即可。
              </div>
            )}

            {localPlatform === 'claude' ? (
              <>
                <h4 className="setup-platform-title">Claude Code</h4>
                <p className="setup-note setup-note-first">
                  先在启动 Claude Code 的同一 shell、shell profile 或 launcher 里设置 <code>{TOKEN_ENV_NAME}</code>，再把 Agent Hub 注册为全局 stdio MCP server。
                </p>

                <div className="code-block">
                  <div className="code-block-label">步骤 1：设置环境变量</div>
                  <pre>{localEnvCommand}</pre>
                  <button
                    className="copy-btn"
                    onClick={() => copyToClipboard(localEnvCommand, 'local-env')}
                  >
                    {copied === 'local-env' ? '已复制' : '复制'}
                  </button>
                </div>

                <div className="code-block">
                  <div className="code-block-label">步骤 2：注册本地 MCP Server（全局）</div>
                  <pre>{localClaudeCommand}</pre>
                  <button
                    className="copy-btn"
                    onClick={() => copyToClipboard(localClaudeCommand, 'local-claude-cmd')}
                  >
                    {copied === 'local-claude-cmd' ? '已复制' : '复制'}
                  </button>
                </div>

                <p className="setup-or">或者手动写入 Claude Code 的 MCP 配置：</p>

                <div className="code-block">
                  <pre>{localConfig}</pre>
                  <button
                    className="copy-btn"
                    onClick={() => copyToClipboard(localConfig, 'local-claude-json')}
                  >
                    {copied === 'local-claude-json' ? '已复制' : '复制'}
                  </button>
                </div>
              </>
            ) : (
              <>
                <h4 className="setup-platform-title">Codex CLI</h4>
                <p className="setup-note setup-note-first">
                  先在启动 Codex CLI 的同一 shell、shell profile 或 launcher 里设置 <code>{TOKEN_ENV_NAME}</code>，再把 Agent Hub 添加到 Codex 的 stdio MCP 配置中。
                </p>

                <div className="code-block">
                  <div className="code-block-label">步骤 1：设置环境变量</div>
                  <pre>{localEnvCommand}</pre>
                  <button
                    className="copy-btn"
                    onClick={() => copyToClipboard(localEnvCommand, 'local-env-codex')}
                  >
                    {copied === 'local-env-codex' ? '已复制' : '复制'}
                  </button>
                </div>

                <div className="code-block">
                  <div className="code-block-label">步骤 2：注册本地 MCP Server</div>
                  <pre>{localCodexCommand}</pre>
                  <button
                    className="copy-btn"
                    onClick={() => copyToClipboard(localCodexCommand, 'local-codex-cmd')}
                  >
                    {copied === 'local-codex-cmd' ? '已复制' : '复制'}
                  </button>
                </div>

                <p className="setup-note">
                  Codex 推荐直接使用上面的 <code>codex mcp add ...</code> 命令完成配置，无需手动编辑配置文件。
                </p>
              </>
            )}
          </div>
        )}
      </div>

      <div className="setup-section">
        <div className="setup-section-header">
          <span className="setup-section-icon">&#128736;</span>
          <div>
            <h3>高级模式（HTTP + 手动 Bearer Token）</h3>
            <p className="setup-section-desc">面向支持 HTTP MCP 的通用客户端，使用静态 Bearer Token 直连 `/mcp`</p>
          </div>
        </div>

        <p className="setup-note">
          说明默认直接可看，不会自动创建 token。推荐优先把 Bearer Token 放进环境变量 <code>{TOKEN_ENV_NAME}</code>；只有客户端不支持 env 方式时，再退回静态 Bearer header。
        </p>

        <div className="setup-mode-actions">
          <button
            className="btn btn-primary"
            onClick={() => toggleMode('advanced')}
          >
            {openModes.advanced ? '隐藏高级模式配置' : '查看高级模式配置'}
          </button>
          {openModes.advanced && (
            <button
              className="btn btn-outline"
              onClick={() => provisionModeToken('advanced', !!modeTokens.advanced)}
              disabled={provisioningMode === 'advanced'}
            >
              {provisioningMode === 'advanced'
                ? '生成中...'
                : modeTokens.advanced
                  ? '重新生成 Token'
                  : '创建本模式 Token'}
            </button>
          )}
        </div>

        {openModes.advanced && (
          <>
            {modeTokens.advanced ? (
              <>
                <div className="alert alert-success">
                  已为高级模式创建一个新的 Bearer Token。推荐下一步把它保存到环境变量 <code>{TOKEN_ENV_NAME}</code>；完整值只会在当前页面会话里显示一次。
                </div>
                <div className="code-block">
                  <div className="code-block-label">刚创建的 Token（仅当前会话可见）</div>
                  <pre>{advancedSessionToken}</pre>
                  <button
                    className="copy-btn"
                    onClick={() => copyToClipboard(advancedSessionToken, 'advanced-token')}
                  >
                    {copied === 'advanced-token' ? '已复制' : '复制 Token'}
                  </button>
                </div>
              </>
            ) : (
              <div className="alert alert-warn">
                当前显示的是环境变量和配置模板，里面的 <code>{TOKEN_PLACEHOLDER}</code> 只是占位符。只有在你明确点击“创建本模式 Token”时，才会生成新的 Bearer Token。
              </div>
            )}

            <div className="code-block">
              <div className="code-block-label">步骤 1：设置环境变量</div>
              <pre>{advancedEnvCommand}</pre>
              <button
                className="copy-btn"
                onClick={() => copyToClipboard(advancedEnvCommand, 'advanced-env')}
              >
                {copied === 'advanced-env' ? '已复制' : '复制'}
              </button>
            </div>

            <div className="code-block">
              <div className="code-block-label">步骤 2：Codex CLI 直接接入（推荐）</div>
              <pre>{advancedCodexCommand}</pre>
              <button
                className="copy-btn"
                onClick={() => copyToClipboard(advancedCodexCommand, 'advanced-codex-cmd')}
              >
                {copied === 'advanced-codex-cmd' ? '已复制' : '复制'}
              </button>
            </div>

            <div className="code-block">
              <div className="code-block-label">步骤 3：通用 MCP HTTP 配置（静态 Bearer，兜底方案）</div>
              <pre>{advancedConfig}</pre>
              <button
                className="copy-btn"
                onClick={() => copyToClipboard(advancedConfig, 'advanced-json')}
              >
                {copied === 'advanced-json' ? '已复制' : '复制'}
              </button>
            </div>
          </>
        )}
      </div>

      <div className="setup-section">
        <div className="setup-section-header">
          <span className="setup-section-icon">&#129302;</span>
          <div>
            <h3>ChatGPT GPT Actions <span className="badge badge-platform" style={{ marginLeft: 8, fontSize: 11 }}>GPT</span></h3>
            <p className="setup-section-desc">在自定义 GPT 中通过 Actions 连接 Agent Hub</p>
          </div>
        </div>

        <div className="code-block">
          <div className="code-block-label">1. OpenAPI Schema URL（粘贴到 Actions 配置中）</div>
          <pre>{`${baseUrl}/gpt/openapi.json`}</pre>
          <button
            className="copy-btn"
            onClick={() => copyToClipboard(`${baseUrl}/gpt/openapi.json`, 'gpt-schema')}
          >
            {copied === 'gpt-schema' ? '已复制' : '复制'}
          </button>
        </div>

        <div className="code-block">
          <div className="code-block-label">2. Authentication 配置</div>
          <pre>{`Type: API Key\nAuth Type: Bearer\nToken: ${gptTokenText}`}</pre>
          {newToken ? (
            <button
              className="copy-btn"
              onClick={() => copyToClipboard(newToken, 'gpt-token')}
            >
              {copied === 'gpt-token' ? '已复制 Token' : '复制 Token'}
            </button>
          ) : (
            <button
              className="copy-btn"
              onClick={scrollToTokenCreator}
            >
              去创建
            </button>
          )}
        </div>

        <p className="setup-note">
          本页不会自动为 GPT Actions 生成 token。需要新的 Bearer Token 时，请在下方“创建新 Token”中手动创建，再把它填到 Actions 的认证配置里。
        </p>
      </div>

      <div className="setup-section" id="token-creator">
        <div className="setup-section-header">
          <span className="setup-section-icon">&#128273;</span>
          <div>
            <h3>创建新 Token</h3>
            <p className="setup-section-desc">为 GPT Actions、脚本或其他自定义用途创建独立的 Token</p>
          </div>
        </div>

        <div className="card">
          <div className="form-group" style={{ marginBottom: 12 }}>
            <label>名称</label>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如: Claude Desktop"
            />
          </div>

          <div className="form-group" style={{ marginBottom: 12 }}>
            <label>预设权限</label>
            <div className="preset-radio-group">
              <label className={`preset-radio ${preset === 'agent' ? 'preset-radio-active' : ''}`}>
                <input
                  type="radio"
                  name="preset"
                  checked={preset === 'agent'}
                  onChange={() => { setPreset('agent'); setTrustLevel(4); setExpiryDays(90) }}
                />
                <span className="preset-radio-dot" />
                <div>
                  <strong>Agent 完整权限</strong>
                  <span className="preset-radio-desc">读写 Memory、Skills、Inbox、Projects、Tree、Devices</span>
                </div>
              </label>
              <label className={`preset-radio ${preset === 'readonly' ? 'preset-radio-active' : ''}`}>
                <input
                  type="radio"
                  name="preset"
                  checked={preset === 'readonly'}
                  onChange={() => { setPreset('readonly'); setTrustLevel(3); setExpiryDays(30) }}
                />
                <span className="preset-radio-dot" />
                <div>
                  <strong>只读访问</strong>
                  <span className="preset-radio-desc">仅读取 Profile、Memory、Skills、Projects、Tree</span>
                </div>
              </label>
              <label className={`preset-radio ${preset === 'custom' ? 'preset-radio-active' : ''}`}>
                <input
                  type="radio"
                  name="preset"
                  checked={preset === 'custom'}
                  onChange={() => setPreset('custom')}
                />
                <span className="preset-radio-dot" />
                <div>
                  <strong>自定义</strong>
                  <span className="preset-radio-desc">手动选择权限范围</span>
                </div>
              </label>
            </div>
          </div>

          {preset === 'custom' && scopeInfo && (
            <div className="form-group" style={{ marginBottom: 12 }}>
              <label>权限范围</label>
              <div className="scope-grid">
                {Object.entries(scopeInfo.categories).map(([category, scopes]) => (
                  <div key={category} className="scope-grid-category">
                    <div className="scope-grid-category-name">{category}</div>
                    {scopes.map((scope) => (
                      <label key={scope} className="scope-grid-item">
                        <input
                          type="checkbox"
                          checked={customScopes.includes(scope)}
                          onChange={() => toggleScope(scope)}
                        />
                        <span>{scope}</span>
                      </label>
                    ))}
                  </div>
                ))}
              </div>
            </div>
          )}

          <div style={{ display: 'flex', gap: 12, marginBottom: 12 }}>
            <div className="form-group" style={{ flex: 1 }}>
              <label>信任等级</label>
              <select
                className={`trust-select trust-l${trustLevel}`}
                value={trustLevel}
                onChange={(e) => setTrustLevel(Number(e.target.value))}
              >
                {TRUST_LEVELS.map((item) => (
                  <option key={item.value} value={item.value}>{item.label} - {item.desc}</option>
                ))}
              </select>
            </div>

            <div className="form-group" style={{ flex: 1 }}>
              <label>有效期</label>
              <select
                className="expiry-select"
                value={expiryDays}
                onChange={(e) => setExpiryDays(Number(e.target.value))}
              >
                {EXPIRY_OPTIONS.map((item) => (
                  <option key={item.value} value={item.value}>{item.label}</option>
                ))}
              </select>
            </div>
          </div>

          <button
            className="btn btn-primary"
            onClick={handleCreateToken}
            disabled={manualCreating || (preset === 'custom' && customScopes.length === 0) || !name.trim()}
          >
            {manualCreating ? '生成中...' : '生成 Token'}
          </button>

          {newToken && (
            <div className="alert alert-success" style={{ marginTop: 16 }}>
              <strong>Token 已生成!</strong> 请立即保存，此 Token 仅显示一次。
              <div className="key-value" style={{ marginTop: 8 }}>
                <code>{newToken}</code>
                <button className="btn btn-sm" onClick={() => { copyToClipboard(newToken, 'new-token') }}>
                  {copied === 'new-token' ? '已复制' : '复制'}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="setup-section">
        <div className="setup-section-header">
          <span className="setup-section-icon">&#128218;</span>
          <div>
            <h3>已有 Token</h3>
            <p className="setup-section-desc">
              共 {tokens.length} 个，{activeTokens.length} 个有效
            </p>
          </div>
        </div>

        {tokens.length === 0 ? (
          <div className="empty-state">
            <p>暂无 Token</p>
            <p className="empty-hint">你可以先查看上方连接模板；需要真实 secret 时，再在上方模式里创建或在这里手动创建一个新的 Token</p>
          </div>
        ) : (
          <div className="token-list">
            {tokens.map((token) => (
              <div
                key={token.id}
                className={`token-list-item ${token.is_revoked || token.is_expired ? 'token-list-item-inactive' : ''}`}
              >
                <div className="token-list-main">
                  {editingTokenId === token.id ? (
                    <div className="token-inline-edit">
                      <input
                        className="token-inline-input"
                        value={editingTokenName}
                        onChange={(e) => setEditingTokenName(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') {
                            e.preventDefault()
                            void handleRenameToken(token)
                          }
                          if (e.key === 'Escape') {
                            e.preventDefault()
                            cancelRenameToken()
                          }
                        }}
                        autoFocus
                      />
                      <code className="token-list-prefix">{token.token_prefix}...</code>
                    </div>
                  ) : (
                    <>
                      <div className="token-list-name">{token.name}</div>
                      <code className="token-list-prefix">{token.token_prefix}...</code>
                    </>
                  )}
                </div>
                <div className="token-list-meta">
                  <span className={`trust-badge trust-l${token.max_trust_level}`}>
                    {trustLabel(token.max_trust_level)}
                  </span>
                  <span className="token-list-sep">&middot;</span>
                  <span>{presetLabel(token)}</span>
                  <span className="token-list-sep">&middot;</span>
                  <span>{formatExpiry(token)}</span>
                </div>
                <div className="token-list-actions">
                  {editingTokenId === token.id ? (
                    <>
                      <button
                        className="btn btn-sm btn-primary"
                        onClick={() => handleRenameToken(token)}
                        disabled={renamingTokenId === token.id || !editingTokenName.trim()}
                      >
                        {renamingTokenId === token.id ? '保存中...' : '保存'}
                      </button>
                      <button
                        className="btn btn-sm btn-outline"
                        onClick={cancelRenameToken}
                        disabled={renamingTokenId === token.id}
                      >
                        取消
                      </button>
                    </>
                  ) : (
                    <>
                      <button
                        className="btn btn-sm btn-outline"
                        onClick={() => startRenameToken(token)}
                      >
                        改名
                      </button>
                      {!token.is_revoked && !token.is_expired && (
                        <button
                          className="btn btn-sm btn-danger"
                          onClick={() => handleRevoke(token.id)}
                        >
                          吊销
                        </button>
                      )}
                    </>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
