import { useState, useEffect, useCallback } from 'react'
import { api, ScopedTokenResponse, CreateTokenRequest } from '../api'

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

interface ScopeInfo {
  scopes: string[]
  categories: Record<string, string[]>
  bundles: Record<string, string[]>
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function SetupPage() {
  // Token list
  const [tokens, setTokens] = useState<ScopedTokenResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // Scope metadata from server
  const [scopeInfo, setScopeInfo] = useState<ScopeInfo | null>(null)

  // Currently active token (for display in config snippets)
  const [activeToken, setActiveToken] = useState<string | null>(null)

  // Token creation form
  const [name, setName] = useState('Claude Code')
  const [preset, setPreset] = useState<Preset>('agent')
  const [trustLevel, setTrustLevel] = useState(4)
  const [expiryDays, setExpiryDays] = useState(90)
  const [customScopes, setCustomScopes] = useState<string[]>([])
  const [creating, setCreating] = useState(false)
  const [newToken, setNewToken] = useState<string | null>(null)

  // Copy feedback
  const [copied, setCopied] = useState<string | null>(null)

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

  // Auto-generate a token on first visit if none exist
  useEffect(() => {
    if (!loading && tokens.length === 0 && !activeToken && !creating) {
      handleCreateToken(true)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loading, tokens.length])

  // ---------------------------------------------------------------------------
  // Actions
  // ---------------------------------------------------------------------------

  const handleCreateToken = async (auto = false) => {
    setCreating(true)
    setError('')

    const scopes = getSelectedScopes()
    const req: CreateTokenRequest = {
      name: auto ? 'Claude Code (auto)' : name,
      scopes,
      max_trust_level: auto ? 4 : trustLevel,
      expires_in_days: auto ? 90 : (expiryDays === 0 ? 36500 : expiryDays),
    }

    try {
      const resp = await api.createToken(req)
      setNewToken(resp.token)
      setActiveToken(resp.token)
      await loadTokens()
    } catch (e: any) {
      setError(e.message)
    }
    setCreating(false)
  }

  const handleRevoke = async (id: string) => {
    try {
      await api.revokeToken(id)
      await loadTokens()
      setError('')
    } catch (e: any) {
      setError(e.message)
    }
  }

  // ---------------------------------------------------------------------------
  // Helpers
  // ---------------------------------------------------------------------------

  const getSelectedScopes = (): string[] => {
    if (preset === 'agent') return scopeInfo?.bundles?.agent ?? ['read:profile', 'read:memory', 'write:memory', 'read:skills', 'read:vault.auth', 'read:devices', 'call:devices', 'read:inbox', 'write:inbox', 'read:projects', 'write:projects', 'read:tree', 'write:tree', 'search']
    if (preset === 'readonly') return scopeInfo?.bundles?.read_only ?? ['read:profile', 'read:memory', 'read:skills', 'read:projects', 'read:tree', 'search']
    return customScopes
  }

  const toggleScope = (scope: string) => {
    setCustomScopes(prev =>
      prev.includes(scope) ? prev.filter(s => s !== scope) : [...prev, scope]
    )
  }

  const displayToken = activeToken || newToken || 'aht_xxxxx'

  const baseUrl = typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8080'

  const claudeCommand = `claude mcp add agenthub \\
  --transport stdio \\
  -- agenthub-mcp \\
  --token ${displayToken}`

  const mcpConfig = JSON.stringify({
    mcpServers: {
      agenthub: {
        command: 'agenthub-mcp',
        args: ['--token', displayToken],
      },
    },
  }, null, 2)

  const httpConfig = `Base URL: ${baseUrl}/agent
Authorization: Bearer ${displayToken}`

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
    const found = TRUST_LEVELS.find(t => t.value === level)
    return found ? found.label : `L${level}`
  }

  const presetLabel = (token: ScopedTokenResponse): string => {
    const s = token.scopes
    if (s.includes('admin')) return 'Full'
    if (s.length >= 13) return 'Agent完整'
    if (s.length <= 6 && s.every(sc => sc.startsWith('read:') || sc === 'search')) return '只读'
    return `${s.length}项权限`
  }

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  if (loading) {
    return <div className="page"><div className="page-loading">加载中...</div></div>
  }

  const activeTokens = tokens.filter(t => !t.is_revoked && !t.is_expired)

  return (
    <div className="page">
      <div className="page-header">
        <h2>连接设置</h2>
      </div>

      {error && <div className="alert alert-error">{error}</div>}

      {newToken && (
        <div className="alert alert-success" style={{ marginBottom: 20 }}>
          <strong>Token 已生成!</strong> 请立即保存，此 Token 仅显示一次。
          <div className="key-value" style={{ marginTop: 8 }}>
            <code>{newToken}</code>
            <button className="btn btn-sm" onClick={() => { copyToClipboard(newToken, 'new-token'); }}>
              {copied === 'new-token' ? '已复制' : '复制'}
            </button>
          </div>
        </div>
      )}

      {/* ── Claude Code section ── */}
      <div className="setup-section">
        <div className="setup-section-header">
          <span className="setup-section-icon">&#128279;</span>
          <div>
            <h3>Claude Code <span className="badge badge-platform" style={{ marginLeft: 8, fontSize: 11 }}>推荐</span></h3>
            <p className="setup-section-desc">通过 MCP 协议连接，一键配置即可使用</p>
          </div>
        </div>

        <div className="code-block">
          <div className="code-block-label">一键配置:</div>
          <pre>{claudeCommand}</pre>
          <button
            className="copy-btn"
            onClick={() => copyToClipboard(claudeCommand, 'claude-cmd')}
          >
            {copied === 'claude-cmd' ? '已复制' : '复制'}
          </button>
        </div>

        <p className="setup-or">或者手动配置 claude_code_config:</p>

        <div className="code-block">
          <pre>{mcpConfig}</pre>
          <button
            className="copy-btn"
            onClick={() => copyToClipboard(mcpConfig, 'mcp-json')}
          >
            {copied === 'mcp-json' ? '已复制' : '复制'}
          </button>
        </div>
      </div>

      {/* ── HTTP API section ── */}
      <div className="setup-section">
        <div className="setup-section-header">
          <span className="setup-section-icon">&#128225;</span>
          <div>
            <h3>HTTP API <span className="badge badge-platform" style={{ marginLeft: 8, fontSize: 11 }}>通用</span></h3>
            <p className="setup-section-desc">适用于 GPT、Gemini 等其他平台</p>
          </div>
        </div>

        <div className="code-block">
          <pre>{httpConfig}</pre>
          <button
            className="copy-btn"
            onClick={() => copyToClipboard(httpConfig, 'http-cfg')}
          >
            {copied === 'http-cfg' ? '已复制' : '复制'}
          </button>
        </div>
      </div>

      {/* ── Create Token ── */}
      <div className="setup-section">
        <div className="setup-section-header">
          <span className="setup-section-icon">&#128273;</span>
          <div>
            <h3>创建新 Token</h3>
            <p className="setup-section-desc">为不同平台或用途创建独立的 Token</p>
          </div>
        </div>

        <div className="card">
          <div className="form-group" style={{ marginBottom: 12 }}>
            <label>名称</label>
            <input
              value={name}
              onChange={e => setName(e.target.value)}
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
                  onChange={() => { setPreset('agent'); setTrustLevel(4); setExpiryDays(90); }}
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
                  onChange={() => { setPreset('readonly'); setTrustLevel(3); setExpiryDays(30); }}
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
                    {scopes.map(scope => (
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
                onChange={e => setTrustLevel(Number(e.target.value))}
              >
                {TRUST_LEVELS.map(t => (
                  <option key={t.value} value={t.value}>{t.label} - {t.desc}</option>
                ))}
              </select>
            </div>

            <div className="form-group" style={{ flex: 1 }}>
              <label>有效期</label>
              <select
                className="expiry-select"
                value={expiryDays}
                onChange={e => setExpiryDays(Number(e.target.value))}
              >
                {EXPIRY_OPTIONS.map(o => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </select>
            </div>
          </div>

          <button
            className="btn btn-primary"
            onClick={() => handleCreateToken(false)}
            disabled={creating || (preset === 'custom' && customScopes.length === 0) || !name.trim()}
          >
            {creating ? '生成中...' : '生成 Token'}
          </button>
        </div>
      </div>

      {/* ── Existing Tokens ── */}
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
            <p className="empty-hint">使用上方表单创建你的第一个 Token</p>
          </div>
        ) : (
          <div className="token-list">
            {tokens.map(token => (
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
