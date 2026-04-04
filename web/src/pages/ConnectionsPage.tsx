import { useEffect, useState } from 'react'
import { api, ConnectionResponse, OAuthGrantResponse } from '../api'

const TRUST_LEVELS = [
  { value: 1, label: 'L1 访客', className: 'trust-l1' },
  { value: 2, label: 'L2 协作', className: 'trust-l2' },
  { value: 3, label: 'L3 工作信任', className: 'trust-l3' },
  { value: 4, label: 'L4 完全信任', className: 'trust-l4' },
]

type ConnectionRow =
  | {
      id: string
      kind: 'manual'
      name: string
      platform: string
      platformDetail?: string
      trustLevel: number
      activityAt?: string
      activityLabel: string
      badgeDetail: string
      secondaryDetail?: string
      apiKeyPrefix?: string
    }
  | {
      id: string
      kind: 'oauth'
      name: string
      platform: string
      platformDetail?: string
      trustLevel: number
      activityAt?: string
      activityLabel: string
      badgeDetail: string
      secondaryDetail?: string
      scopes: string[]
    }

const PLATFORM_LABELS: Record<string, string> = {
  claude: 'Claude',
  gpt: 'GPT',
  feishu: '飞书',
  other: '其他',
}

export default function ConnectionsPage() {
  const [connections, setConnections] = useState<ConnectionResponse[]>([])
  const [oauthGrants, setOAuthGrants] = useState<OAuthGrantResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [newConn, setNewConn] = useState({ name: '', platform: '', trust_level: 2 })
  const [creating, setCreating] = useState(false)
  const [createdKey, setCreatedKey] = useState('')
  const [keyCopied, setKeyCopied] = useState(false)

  useEffect(() => {
    loadConnections()
  }, [])

  const loadConnections = async () => {
    setError('')

    const errors: string[] = []

    try {
      const [manualResult, grantResult] = await Promise.allSettled([
        api.getConnections(),
        api.getOAuthGrants(),
      ])

      if (manualResult.status === 'fulfilled') {
        setConnections(manualResult.value || [])
      } else {
        setConnections([])
        errors.push(manualResult.reason?.message || '连接列表加载失败')
      }

      if (grantResult.status === 'fulfilled') {
        setOAuthGrants(grantResult.value || [])
      } else {
        setOAuthGrants([])
        errors.push(grantResult.reason?.message || 'OAuth 授权列表加载失败')
      }
    } finally {
      if (errors.length > 0) {
        setError(errors.join('；'))
      }
      setLoading(false)
    }
  }

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newConn.name.trim() || !newConn.platform.trim()) return

    setCreating(true)
    setError('')

    try {
      const result = await api.createConnection({
        name: newConn.name,
        type: newConn.platform,
        trust_level: newConn.trust_level,
      })
      if (result.api_key) {
        setCreatedKey(result.api_key)
      }
      // API returns {connection: {...}, api_key: "..."} — extract connection object
      const conn = result.connection || result
      setConnections((prev) => [...prev, conn])
      setNewConn({ name: '', platform: '', trust_level: 2 })
      if (!result.api_key) {
        setShowForm(false)
      }
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  const handleTrustChange = async (id: string, trust_level: number) => {
    try {
      await api.updateConnection(id, { trust_level })
      setConnections((prev) =>
        prev.map((c) => (c.id === id ? { ...c, trust_level } : c))
      )
    } catch (err: any) {
      setError(err.message)
    }
  }

  const handleDelete = async (id: string, name: string) => {
    if (!window.confirm(`确认删除连接 "${name}"？此操作不可撤销。`)) return

    try {
      await api.deleteConnection(id)
      setConnections((prev) => prev.filter((c) => c.id !== id))
    } catch (err: any) {
      setError(err.message)
    }
  }

  const handleRevokeGrant = async (id: string, name: string) => {
    if (!window.confirm(`确认撤销 "${name}" 的授权？连接器将无法继续访问 Agent Hub。`)) return

    try {
      await api.revokeOAuthGrant(id)
      setOAuthGrants((prev) => prev.filter((grant) => grant.id !== id))
    } catch (err: any) {
      setError(err.message)
    }
  }

  const copyKey = async () => {
    try {
      await navigator.clipboard.writeText(createdKey)
      setKeyCopied(true)
      setTimeout(() => setKeyCopied(false), 2000)
    } catch {
      // Fallback
      const textarea = document.createElement('textarea')
      textarea.value = createdKey
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
      setKeyCopied(true)
      setTimeout(() => setKeyCopied(false), 2000)
    }
  }

  const dismissKey = () => {
    setCreatedKey('')
    setKeyCopied(false)
    setShowForm(false)
  }

  const formatTime = (ts?: string) => {
    if (!ts) return '-'
    try {
      return new Date(ts).toLocaleString('zh-CN')
    } catch {
      return ts
    }
  }

  const getTrustInfo = (level: number) => {
    return TRUST_LEVELS.find((t) => t.value === level) || TRUST_LEVELS[0]
  }

  const parseHost = (value?: string) => {
    if (!value) return ''
    try {
      return new URL(value).host.replace(/^www\./, '')
    } catch {
      return ''
    }
  }

  const getPlatformLabel = (platform: string) => {
    return PLATFORM_LABELS[platform] || platform || '未知'
  }

  const summarizeScopes = (scopes: string[]) => {
    if (scopes.includes('admin')) return 'admin'
    if (scopes.length === 0) return '未声明 scope'
    return `${scopes.length} 项 scope`
  }

  const inferOAuthPlatform = (grant: OAuthGrantResponse) => {
    const clientHost = parseHost(grant.app.client_id)
    const redirectHosts = grant.app.redirect_uris.map(parseHost).filter(Boolean)
    const knownHosts = [clientHost, ...redirectHosts]
    const primaryHost = knownHosts[0]

    if (knownHosts.some((host) => host === 'claude.ai' || host === 'claude.com')) {
      return {
        platform: 'Claude',
        name: 'Claude Connector',
        detail: primaryHost || grant.app.client_id,
      }
    }

    if (knownHosts.some((host) => host.includes('openai.com') || host.includes('chatgpt.com'))) {
      return {
        platform: 'GPT',
        name: grant.app.name || 'ChatGPT Connector',
        detail: primaryHost || grant.app.client_id,
      }
    }

    return {
      platform: primaryHost || 'OAuth',
      name: grant.app.name || primaryHost || 'OAuth App',
      detail: primaryHost || grant.app.client_id,
    }
  }

  const rows: ConnectionRow[] = [
    ...connections.map((conn) => ({
      id: conn.id,
      kind: 'manual' as const,
      name: conn.name,
      platform: getPlatformLabel(conn.platform),
      trustLevel: conn.trust_level,
      activityAt: conn.last_used_at || conn.created_at,
      activityLabel: conn.last_used_at ? '最后使用' : '创建于',
      badgeDetail: 'API Key',
      apiKeyPrefix: conn.api_key_prefix,
      secondaryDetail: conn.api_key_prefix ? `${conn.api_key_prefix}...` : undefined,
    })),
    ...oauthGrants.map((grant) => {
      const inferred = inferOAuthPlatform(grant)
      return {
        id: grant.id,
        kind: 'oauth' as const,
        name: inferred.name,
        platform: inferred.platform,
        platformDetail: inferred.detail,
        trustLevel: 4,
        activityAt: grant.created_at,
        activityLabel: '授权于',
        badgeDetail: 'OAuth / MCP',
        secondaryDetail: summarizeScopes(grant.scopes),
        scopes: grant.scopes,
      }
    }),
  ].sort((a, b) => {
    const aTime = a.activityAt ? new Date(a.activityAt).getTime() : 0
    const bTime = b.activityAt ? new Date(b.activityAt).getTime() : 0
    return bTime - aTime
  })

  if (loading) {
    return <div className="page-loading">加载中...</div>
  }

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h2>连接管理</h2>
          <p className="empty-hint">这里会显示手动创建的 API Key 连接，以及 OAuth / MCP 授权过的平台连接</p>
        </div>
        <button
          className="btn btn-primary"
          onClick={() => {
            setShowForm(true)
            setCreatedKey('')
          }}
        >
          添加连接
        </button>
      </div>

      {error && <div className="alert alert-error">{error}</div>}

      {createdKey && (
        <div className="alert alert-success">
          <div className="key-display">
            <p className="api-key-warning">
              此密钥仅显示一次
            </p>
            <div className="api-key-box">
              <code>{createdKey}</code>
              <button className="btn btn-sm" onClick={copyKey} style={{ marginLeft: 12, color: '#68d391' }}>
                {keyCopied ? '已复制' : '复制'}
              </button>
            </div>
            <button className="btn btn-text" onClick={dismissKey}>
              我已保存，关闭
            </button>
          </div>
        </div>
      )}

      {showForm && !createdKey && (
        <div className="card form-card">
          <h3 className="card-title">新建连接</h3>
          <form onSubmit={handleCreate}>
            <div className="form-row">
              <div className="form-group">
                <label htmlFor="conn-name">名称</label>
                <input
                  id="conn-name"
                  type="text"
                  value={newConn.name}
                  onChange={(e) => setNewConn({ ...newConn, name: e.target.value })}
                  placeholder="例如：我的 Telegram Bot"
                  disabled={creating}
                />
              </div>
              <div className="form-group">
                <label htmlFor="conn-platform">平台</label>
                <select
                  id="conn-platform"
                  value={newConn.platform}
                  onChange={(e) => setNewConn({ ...newConn, platform: e.target.value })}
                  disabled={creating}
                >
                  <option value="">请选择平台</option>
                  <option value="claude">Claude</option>
                  <option value="gpt">GPT</option>
                  <option value="feishu">飞书</option>
                  <option value="other">其他</option>
                </select>
              </div>
              <div className="form-group">
                <label htmlFor="conn-trust">信任等级</label>
                <select
                  id="conn-trust"
                  value={newConn.trust_level}
                  onChange={(e) =>
                    setNewConn({ ...newConn, trust_level: Number(e.target.value) })
                  }
                  disabled={creating}
                >
                  {TRUST_LEVELS.map((t) => (
                    <option key={t.value} value={t.value}>
                      {t.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>
            <div className="form-actions">
              <button type="submit" className="btn btn-primary" disabled={creating}>
                {creating ? '创建中...' : '创建'}
              </button>
              <button
                type="button"
                className="btn"
                onClick={() => setShowForm(false)}
                disabled={creating}
              >
                取消
              </button>
            </div>
          </form>
        </div>
      )}

      {rows.length === 0 ? (
        <div className="empty-state">
          <p>还没有连接</p>
          <p className="empty-hint">添加 API Key 连接，或者先在 Claude Connector 等平台完成 OAuth 授权</p>
        </div>
      ) : (
        <div className="table-container">
          <table className="table">
            <thead>
              <tr>
                <th>名称</th>
                <th>平台</th>
                <th>信任等级</th>
                <th>最后使用</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                const trust = getTrustInfo(row.trustLevel)
                return (
                  <tr key={`${row.kind}-${row.id}`}>
                    <td className="cell-name">
                      <div>{row.name}</div>
                      {row.secondaryDetail && (
                        <div className="cell-key-prefix">{row.badgeDetail} · {row.secondaryDetail}</div>
                      )}
                    </td>
                    <td>
                      <span className="badge badge-platform">{row.platform}</span>
                      {row.platformDetail && (
                        <div className="cell-key-prefix">{row.platformDetail}</div>
                      )}
                    </td>
                    <td>
                      <span className={`badge badge-l${row.trustLevel}`} style={{ marginRight: 8 }}>
                        {trust.label}
                      </span>
                      {row.kind === 'manual' ? (
                        <select
                          className={`trust-select ${trust.className}`}
                          value={row.trustLevel}
                          onChange={(e) =>
                            handleTrustChange(row.id, Number(e.target.value))
                          }
                        >
                          {TRUST_LEVELS.map((t) => (
                            <option key={t.value} value={t.value}>
                              {t.label}
                            </option>
                          ))}
                        </select>
                      ) : (
                        <span className="cell-key-prefix">OAuth 授权当前按完整访问处理</span>
                      )}
                    </td>
                    <td className="cell-time">
                      <div>{formatTime(row.activityAt)}</div>
                      <div className="cell-key-prefix">{row.activityLabel}</div>
                    </td>
                    <td>
                      {row.kind === 'manual' ? (
                        <button
                          className="btn btn-sm btn-danger"
                          onClick={() => handleDelete(row.id, row.name)}
                        >
                          删除
                        </button>
                      ) : (
                        <button
                          className="btn btn-sm btn-danger"
                          onClick={() => handleRevokeGrant(row.id, row.name)}
                        >
                          撤销授权
                        </button>
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
