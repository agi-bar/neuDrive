import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, ConnectionResponse, OAuthGrantResponse } from '../api'
import MaterialsSectionToolbar from '../components/MaterialsSectionToolbar'
import MaterialsTile from '../components/MaterialsTile'

const TRUST_LEVELS = [
  { value: 1, label: 'L1 访客', className: 'trust-l1' },
  { value: 2, label: 'L2 共享', className: 'trust-l2' },
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

const SETUP_ENTRY_CARDS = [
  {
    key: 'web-apps',
    title: 'Web / Desktop Apps',
    subtitle: '网页应用',
    description: '在 Claude、ChatGPT、Cursor、Windsurf 等图形界面里，把 Agent Hub 添加成远程 MCP Server。',
    route: '/setup/web-apps',
    iconClassName: 'icon-device',
  },
  {
    key: 'cloud',
    title: 'CLI Apps',
    subtitle: '云端模式',
    description: '给 Claude Code、Codex CLI、Gemini CLI、Cursor Agent 配置远程 HTTP MCP 和浏览器授权。',
    route: '/setup/cloud',
    iconClassName: 'icon-sync',
  },
  {
    key: 'local',
    title: 'Local Mode',
    subtitle: '本地模式',
    description: '通过本地 stdio MCP binary 和 scoped token 接入，适合本机开发或内网环境。',
    route: '/setup/local',
    iconClassName: 'icon-file',
  },
  {
    key: 'advanced',
    title: 'Advanced',
    subtitle: '高级模式',
    description: '查看完整命令、环境变量和更底层的配置方式，适合需要自定义接法的场景。',
    route: '/setup/advanced',
    iconClassName: 'icon-stack',
  },
  {
    key: 'gpt-actions',
    title: 'ChatGPT GPT Actions',
    subtitle: 'GPT Actions',
    description: '在自定义 GPT 中通过 OpenAPI 和 Bearer Token 连接 Agent Hub。',
    route: '/setup/gpt-actions',
    iconClassName: 'icon-mail',
  },
] as const

export default function ConnectionsPage() {
  const navigate = useNavigate()
  const [connections, setConnections] = useState<ConnectionResponse[]>([])
  const [oauthGrants, setOAuthGrants] = useState<OAuthGrantResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showSetupCards, setShowSetupCards] = useState(true)

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

  const getGrantScopes = (grant: OAuthGrantResponse) => {
    const directScopes = Array.isArray(grant.scopes) ? grant.scopes.filter(Boolean) : []
    if (directScopes.length > 0) {
      return directScopes
    }
    return Array.isArray(grant.app?.scopes) ? grant.app.scopes.filter(Boolean) : []
  }

  const inferOAuthPlatform = (grant: OAuthGrantResponse) => {
    const clientID = typeof grant.app?.client_id === 'string' ? grant.app.client_id : ''
    const redirectURIs = Array.isArray(grant.app?.redirect_uris) ? grant.app.redirect_uris : []
    const appName = typeof grant.app?.name === 'string' ? grant.app.name : ''
    const clientHost = parseHost(clientID)
    const redirectHosts = redirectURIs.map(parseHost).filter(Boolean)
    const knownHosts = [clientHost, ...redirectHosts]
    const primaryHost = knownHosts[0]

    if (knownHosts.some((host) => host === 'claude.ai' || host === 'claude.com')) {
      return {
        platform: 'Claude',
        name: 'Claude Connector',
        detail: primaryHost || clientID,
      }
    }

    if (knownHosts.some((host) => host.includes('openai.com') || host.includes('chatgpt.com'))) {
      return {
        platform: 'GPT',
        name: appName || 'ChatGPT Connector',
        detail: primaryHost || clientID,
      }
    }

    return {
      platform: primaryHost || 'OAuth',
      name: appName || primaryHost || 'OAuth App',
      detail: primaryHost || clientID,
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
      const scopes = getGrantScopes(grant)
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
        secondaryDetail: summarizeScopes(scopes),
        scopes,
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
    <div className="page materials-page">
      <div className="page-header">
        <div>
          <h2>连接管理</h2>
          <p className="page-subtitle">上面负责新增连接入口，下面负责展示已经接入的 API Key 连接和 OAuth / MCP 平台连接。</p>
        </div>
      </div>

      {error && <div className="alert alert-error">{error}</div>}

      <section className="materials-section">
        <div className="materials-section-head">
          <div>
            <h3 className="materials-section-title">添加连接</h3>
            <p className="materials-section-copy">先选择一种接法，再进入对应说明页完成连接。独立 token 的生成和管理请放到下面的 Token 管理页。</p>
          </div>
          <MaterialsSectionToolbar count={showSetupCards ? SETUP_ENTRY_CARDS.length : undefined}>
            <button
              type="button"
              className="btn btn-sm materials-toolbar-control"
              onClick={() => setShowSetupCards((value) => !value)}
            >
              {showSetupCards ? '收起' : '展开'}
            </button>
          </MaterialsSectionToolbar>
        </div>
        {showSetupCards ? (
          <div className="materials-grid materials-grid-wide">
            {SETUP_ENTRY_CARDS.map((entry) => (
              <MaterialsTile
                key={entry.key}
                iconClassName={entry.iconClassName}
                title={entry.title}
                titleActionAriaLabel={`打开 ${entry.title}`}
                subtitle={entry.subtitle}
                description={entry.description}
                path={entry.route}
                footerStart="连接设置"
                footerEnd="打开说明"
                onOpen={() => navigate(entry.route)}
              />
            ))}
          </div>
        ) : null}
      </section>

      <section className="materials-section">
        <div className="materials-section-head">
          <div>
            <h3 className="materials-section-title">连接列表</h3>
            <p className="materials-section-copy">这里统一显示已经创建的 API Key 连接，以及通过 OAuth / MCP 授权过的平台连接。</p>
          </div>
          <MaterialsSectionToolbar count={rows.length} />
        </div>

        {rows.length === 0 ? (
          <div className="empty-state">
            <p>还没有连接</p>
            <p className="empty-hint">先从上面的六种接法里选一种完成接入，或者到 Token 管理里单独创建 Bearer Token。</p>
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
      </section>
    </div>
  )
}
