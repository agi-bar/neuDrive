import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, ConnectionResponse, OAuthGrantResponse } from '../api'
import MaterialsSectionToolbar from '../components/MaterialsSectionToolbar'
import MaterialsTile from '../components/MaterialsTile'
import { useI18n } from '../i18n'

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

export default function ConnectionsPage() {
  const { locale, tx } = useI18n()
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
        errors.push(manualResult.reason?.message || tx('连接列表加载失败', 'Failed to load connections'))
      }

      if (grantResult.status === 'fulfilled') {
        setOAuthGrants(grantResult.value || [])
      } else {
        setOAuthGrants([])
        errors.push(grantResult.reason?.message || tx('OAuth 授权列表加载失败', 'Failed to load OAuth grants'))
      }
    } finally {
      if (errors.length > 0) {
        setError(errors.join(locale === 'zh-CN' ? '；' : '; '))
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
    if (!window.confirm(tx(`确认删除连接 "${name}"？此操作不可撤销。`, `Delete connection "${name}"? This action cannot be undone.`))) return

    try {
      await api.deleteConnection(id)
      setConnections((prev) => prev.filter((c) => c.id !== id))
    } catch (err: any) {
      setError(err.message)
    }
  }

  const handleRevokeGrant = async (id: string, name: string) => {
    if (!window.confirm(tx(`确认撤销 "${name}" 的授权？连接器将无法继续访问 Agent Hub。`, `Revoke access for "${name}"? The connector will no longer be able to access Agent Hub.`))) return

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
      return new Date(ts).toLocaleString(locale === 'zh-CN' ? 'zh-CN' : 'en-US')
    } catch {
      return ts
    }
  }

  const trustLevels = [
    { value: 1, label: tx('L1 访客', 'L1 Visitor'), className: 'trust-l1' },
    { value: 2, label: tx('L2 共享', 'L2 Shared'), className: 'trust-l2' },
    { value: 3, label: tx('L3 工作信任', 'L3 Work Trust'), className: 'trust-l3' },
    { value: 4, label: tx('L4 完全信任', 'L4 Full Trust'), className: 'trust-l4' },
  ]

  const platformLabels: Record<string, string> = {
    claude: 'Claude',
    gpt: 'GPT',
    feishu: tx('飞书', 'Feishu'),
    other: tx('其他', 'Other'),
  }

  const setupEntryCards = [
    {
      key: 'web-apps',
      title: 'Web / Desktop Apps',
      subtitle: tx('网页应用', 'Web apps'),
      description: tx('在 Claude、ChatGPT、Cursor、Windsurf 等图形界面里，把 Agent Hub 添加成远程 MCP Server。', 'Add Agent Hub as a remote MCP server in Claude, ChatGPT, Cursor, Windsurf, and other graphical apps.'),
      route: '/setup/web-apps',
      iconClassName: 'icon-device',
    },
    {
      key: 'cloud',
      title: 'CLI Apps',
      subtitle: tx('云端模式', 'Cloud mode'),
      description: tx('给 Claude Code、Codex CLI、Gemini CLI、Cursor Agent 配置远程 HTTP MCP 和浏览器授权。', 'Configure remote HTTP MCP and browser auth for Claude Code, Codex CLI, Gemini CLI, and Cursor Agent.'),
      route: '/setup/cloud',
      iconClassName: 'icon-sync',
    },
    {
      key: 'local',
      title: 'Local Mode',
      subtitle: tx('本地模式', 'Local mode'),
      description: tx('通过本地 stdio MCP binary 和 scoped token 接入，适合本机开发或内网环境。', 'Connect through a local stdio MCP binary and scoped token for local development or internal networks.'),
      route: '/setup/local',
      iconClassName: 'icon-file',
    },
    {
      key: 'advanced',
      title: 'Advanced',
      subtitle: tx('高级模式', 'Advanced'),
      description: tx('查看完整命令、环境变量和更底层的配置方式，适合需要自定义接法的场景。', 'Review full commands, environment variables, and lower-level configuration for custom integrations.'),
      route: '/setup/advanced',
      iconClassName: 'icon-stack',
    },
    {
      key: 'gpt-actions',
      title: 'ChatGPT GPT Actions',
      subtitle: 'GPT Actions',
      description: tx('在自定义 GPT 中通过 OpenAPI 和 Bearer Token 连接 Agent Hub。', 'Connect Agent Hub to a custom GPT with OpenAPI and a Bearer token.'),
      route: '/setup/gpt-actions',
      iconClassName: 'icon-mail',
    },
  ] as const

  const getTrustInfo = (level: number) => {
    return trustLevels.find((t) => t.value === level) || trustLevels[0]
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
    return platformLabels[platform] || platform || tx('未知', 'Unknown')
  }

  const summarizeScopes = (scopes: string[]) => {
    if (scopes.includes('admin')) return 'admin'
    if (scopes.length === 0) return tx('未声明 scope', 'No scopes declared')
    return tx(`${scopes.length} 项 scope`, `${scopes.length} scopes`)
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
        activityLabel: conn.last_used_at ? tx('最后使用', 'Last used') : tx('创建于', 'Created'),
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
        activityLabel: tx('授权于', 'Authorized'),
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
    return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>
  }

  return (
    <div className="page materials-page">
      <div className="page-header">
        <div>
          <h2>{tx('连接管理', 'Connections')}</h2>
          <p className="page-subtitle">{tx('上面负责新增连接入口，下面负责展示已经接入的 API Key 连接和 OAuth / MCP 平台连接。', 'Use the top section to add new connections, and the table below to review existing API key connections and OAuth / MCP integrations.')}</p>
        </div>
      </div>

      {error && <div className="alert alert-error">{error}</div>}

      <section className="materials-section">
        <div className="materials-section-head">
          <div>
            <h3 className="materials-section-title">{tx('添加连接', 'Add connection')}</h3>
            <p className="materials-section-copy">{tx('先选择一种接法，再进入对应说明页完成连接。独立 token 的生成和管理请放到下面的 Token 管理页。', 'Pick a connection method first, then follow its setup guide. Create and manage standalone tokens from the Token Manager page below.')}</p>
          </div>
          <MaterialsSectionToolbar count={showSetupCards ? setupEntryCards.length : undefined}>
            <button
              type="button"
              className="btn btn-sm materials-toolbar-control"
              onClick={() => setShowSetupCards((value) => !value)}
            >
              {showSetupCards ? tx('收起', 'Collapse') : tx('展开', 'Expand')}
            </button>
          </MaterialsSectionToolbar>
        </div>
        {showSetupCards ? (
          <div className="materials-grid materials-grid-wide">
            {setupEntryCards.map((entry) => (
              <MaterialsTile
                key={entry.key}
                iconClassName={entry.iconClassName}
                title={entry.title}
                titleActionAriaLabel={tx(`打开 ${entry.title}`, `Open ${entry.title}`)}
                subtitle={entry.subtitle}
                description={entry.description}
                path={entry.route}
                footerStart={tx('连接设置', 'Setup')}
                footerEnd={tx('打开说明', 'Open guide')}
                onOpen={() => navigate(entry.route)}
              />
            ))}
          </div>
        ) : null}
      </section>

      <section className="materials-section">
        <div className="materials-section-head">
          <div>
            <h3 className="materials-section-title">{tx('连接列表', 'Connection list')}</h3>
            <p className="materials-section-copy">{tx('这里统一显示已经创建的 API Key 连接，以及通过 OAuth / MCP 授权过的平台连接。', 'This section shows API key connections you created and platform connections authorized through OAuth / MCP.')}</p>
          </div>
          <MaterialsSectionToolbar count={rows.length} />
        </div>

        {rows.length === 0 ? (
          <div className="empty-state">
            <p>{tx('还没有连接', 'No connections yet')}</p>
            <p className="empty-hint">{tx('先从上面的六种接法里选一种完成接入，或者到 Token 管理里单独创建 Bearer Token。', 'Pick one of the setup methods above, or create a standalone Bearer token from Token Manager.')}</p>
          </div>
        ) : (
          <div className="table-container">
            <table className="table">
              <thead>
                <tr>
                  <th>{tx('名称', 'Name')}</th>
                  <th>{tx('平台', 'Platform')}</th>
                  <th>{tx('信任等级', 'Trust level')}</th>
                  <th>{tx('最后使用', 'Activity')}</th>
                  <th>{tx('操作', 'Actions')}</th>
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
                            {trustLevels.map((t) => (
                              <option key={t.value} value={t.value}>
                                {t.label}
                              </option>
                            ))}
                          </select>
                        ) : (
                          <span className="cell-key-prefix">{tx('OAuth 授权当前按完整访问处理', 'OAuth grants are currently treated as full access')}</span>
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
                            {tx('删除', 'Delete')}
                          </button>
                        ) : (
                          <button
                            className="btn btn-sm btn-danger"
                            onClick={() => handleRevokeGrant(row.id, row.name)}
                          >
                            {tx('撤销授权', 'Revoke')}
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
