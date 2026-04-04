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

  const cloudCommand = `claude mcp add --transport http agenthub \\
  ${baseUrl}/mcp`

  const buildModeTokenRequest = (mode: ModeKey): CreateTokenRequest => {
    const defaults = MODE_DEFAULTS[mode]
    return {
      name: defaults.name,
      scopes: getSelectedScopes(defaults.preset),
      max_trust_level: defaults.trustLevel,
      expires_in_days: defaults.expiryDays,
    }
  }

  const ensureModeToken = async (mode: ModeKey): Promise<ModeTokenState | null> => {
    const existing = modeTokens[mode]
    if (existing) return existing

    setProvisioningMode(mode)
    setError('')

    try {
      const resp = await api.createToken(buildModeTokenRequest(mode))
      const createdToken = {
        id: resp.scoped_token.id,
        token: resp.token,
      }
      setModeTokens((prev) => ({ ...prev, [mode]: createdToken }))
      await loadTokens()
      return createdToken
    } catch (e: any) {
      setError(e.message)
      return null
    } finally {
      setProvisioningMode((current) => (current === mode ? null : current))
    }
  }

  const toggleMode = async (mode: ModeKey) => {
    const shouldOpen = !openModes[mode]
    setOpenModes((prev) => ({ ...prev, [mode]: shouldOpen }))
    if (shouldOpen) {
      await ensureModeToken(mode)
    }
  }

  const localCommand = modeTokens.local
    ? `claude mcp add agenthub \\
  --transport stdio \\
  -- agenthub-mcp \\
  --token ${modeTokens.local.token}`
    : ''

  const localConfig = modeTokens.local
    ? JSON.stringify({
        mcpServers: {
          agenthub: {
            command: 'agenthub-mcp',
            args: ['--token', modeTokens.local.token],
          },
        },
      }, null, 2)
    : ''

  const advancedConfig = modeTokens.advanced
    ? JSON.stringify({
        mcpServers: {
          agenthub: {
            type: 'http',
            url: `${baseUrl}/mcp`,
            headers: {
              Authorization: `Bearer ${modeTokens.advanced.token}`,
            },
          },
        },
      }, null, 2)
    : ''

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
            <h3>Claude Code 云端模式 <span className="badge badge-platform" style={{ marginLeft: 8, fontSize: 11 }}>推荐</span></h3>
            <p className="setup-section-desc">直接连接远程 HTTP MCP server，在 Claude Code 中用浏览器完成 OAuth 授权</p>
          </div>
        </div>

        {cloudModeNeedsPublicUrl && (
          <div className="alert alert-warn">
            当前地址是 <code>{baseUrl}</code>。云端模式更适合可从 Claude Code 访问的 HTTPS Hub URL；如果你现在在本地开发，建议先用本地模式，或通过公网域名 / 隧道暴露这个 Hub。
          </div>
        )}

        <div className="code-block">
          <div className="code-block-label">步骤 1：添加远程 MCP server</div>
          <pre>{cloudCommand}</pre>
          <button
            className="copy-btn"
            onClick={() => copyToClipboard(cloudCommand, 'cloud-cmd')}
          >
            {copied === 'cloud-cmd' ? '已复制' : '复制'}
          </button>
        </div>

        <div className="code-block">
          <div className="code-block-label">步骤 2：在 Claude Code 中发起浏览器授权</div>
          <pre>/mcp</pre>
          <button
            className="copy-btn"
            onClick={() => copyToClipboard('/mcp', 'cloud-auth')}
          >
            {copied === 'cloud-auth' ? '已复制' : '复制'}
          </button>
        </div>

        <ol className="setup-steps">
          <li>运行上面的 `claude mcp add --transport http ...` 命令，把 Agent Hub 注册成远程 MCP server。</li>
          <li>打开 Claude Code，执行 `/mcp`，按提示选择 `agenthub` 并开始认证。</li>
          <li>浏览器会打开授权页；完成登录和批准后，Claude Code 会保存并刷新 OAuth 凭证。</li>
          <li>如果浏览器没有自动打开，就手动复制 Claude Code 提供的 URL；如果回调完成后 CLI 里仍提示等待，把浏览器地址栏中的完整 callback URL 粘回 Claude Code。</li>
        </ol>

        <p className="setup-note">
          授权完成后，可在 Claude Code 的 `/mcp` 菜单里重新认证或清除认证；Agent Hub 侧的 OAuth 连接会出现在“连接管理”中。
        </p>
      </div>

      <div className="setup-section">
        <div className="setup-section-header">
          <span className="setup-section-icon">&#128187;</span>
          <div>
            <h3>Claude Code 本地模式</h3>
            <p className="setup-section-desc">使用本地 `agenthub-mcp` binary + scoped token，适合本地开发或内网环境</p>
          </div>
        </div>

        <p className="setup-note">
          首次展开会自动创建一个名为 <code>Claude Code</code> 的 Agent 权限 token，并把它加入下方的 Token 列表。
        </p>

        <div className="setup-mode-actions">
          <button
            className="btn btn-primary"
            onClick={() => toggleMode('local')}
            disabled={provisioningMode === 'local'}
          >
            {provisioningMode === 'local'
              ? '生成中...'
              : openModes.local
                ? '隐藏本地模式配置'
                : '生成并显示本地模式配置'}
          </button>
        </div>

        {openModes.local && modeTokens.local && (
          <>
            <div className="code-block">
              <div className="code-block-label">一键配置</div>
              <pre>{localCommand}</pre>
              <button
                className="copy-btn"
                onClick={() => copyToClipboard(localCommand, 'local-cmd')}
              >
                {copied === 'local-cmd' ? '已复制' : '复制'}
              </button>
            </div>

            <p className="setup-or">或者手动写入 Claude Code 的 MCP 配置：</p>

            <div className="code-block">
              <pre>{localConfig}</pre>
              <button
                className="copy-btn"
                onClick={() => copyToClipboard(localConfig, 'local-json')}
              >
                {copied === 'local-json' ? '已复制' : '复制'}
              </button>
            </div>
          </>
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
          首次展开会自动创建一个名为 <code>MCP HTTP</code> 的 Agent 权限 token。这个模式不会触发浏览器 OAuth，而是由客户端直接携带 Bearer Token 发起请求。
        </p>

        <div className="setup-mode-actions">
          <button
            className="btn btn-primary"
            onClick={() => toggleMode('advanced')}
            disabled={provisioningMode === 'advanced'}
          >
            {provisioningMode === 'advanced'
              ? '生成中...'
              : openModes.advanced
                ? '隐藏高级模式配置'
                : '生成并显示高级模式配置'}
          </button>
        </div>

        {openModes.advanced && modeTokens.advanced && (
          <div className="code-block">
            <div className="code-block-label">通用 MCP HTTP 配置（Bearer）</div>
            <pre>{advancedConfig}</pre>
            <button
              className="copy-btn"
              onClick={() => copyToClipboard(advancedConfig, 'advanced-json')}
            >
              {copied === 'advanced-json' ? '已复制' : '复制'}
            </button>
          </div>
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
            <p className="empty-hint">展开上方本地模式 / 高级模式会自动创建，或在这里手动创建一个新的 Token</p>
          </div>
        ) : (
          <div className="token-list">
            {tokens.map((token) => (
              <div
                key={token.id}
                className={`token-list-item ${token.is_revoked || token.is_expired ? 'token-list-item-inactive' : ''}`}
              >
                <div className="token-list-main">
                  <div className="token-list-name">{token.name}</div>
                  <code className="token-list-prefix">{token.token_prefix}...</code>
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
                  {!token.is_revoked && !token.is_expired && (
                    <button
                      className="btn btn-sm btn-danger"
                      onClick={() => handleRevoke(token.id)}
                    >
                      吊销
                    </button>
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
