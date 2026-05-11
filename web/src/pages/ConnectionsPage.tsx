import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type ConnectionResponse, type OAuthGrantResponse } from '../api'
import { useI18n } from '../i18n'
import { formatDateTime } from './data/DataShared'

type ConnectedRow = {
  id: string
  kind: 'manual' | 'oauth'
  app: string
  status: string
  authMethod: string
  trustLevel: string
  lastSync: string
  rawName: string
}

function trustLabel(level: number | undefined, tx: (zh: string, en: string) => string) {
  switch (level) {
    case 1: return tx('L1 访客', 'L1 Guest')
    case 2: return tx('L2 共享', 'L2 Shared')
    case 3: return tx('L3 工作信任', 'L3 Work Trust')
    case 4: return tx('L4 完全信任', 'L4 Full Trust')
    default: return tx('继承', 'Inherited')
  }
}

export default function ConnectionsPage() {
  const { locale, tx } = useI18n()
  const [manual, setManual] = useState<ConnectionResponse[]>([])
  const [grants, setGrants] = useState<OAuthGrantResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = async () => {
    setLoading(true)
    setError('')
    const [manualResult, grantResult] = await Promise.allSettled([
      api.getConnections(),
      api.getOAuthGrants(),
    ])
    if (manualResult.status === 'fulfilled') setManual(manualResult.value || [])
    else setError(manualResult.reason?.message || tx('连接列表加载失败', 'Failed to load connections'))
    if (grantResult.status === 'fulfilled') setGrants(grantResult.value || [])
    setLoading(false)
  }

  useEffect(() => {
    void load()
  }, [])

  const rows = useMemo<ConnectedRow[]>(() => {
    const manualRows = manual.map((connection) => ({
      id: connection.id,
      kind: 'manual' as const,
      app: connection.platform || connection.name || tx('自定义 Agent', 'Custom Agent'),
      status: tx('已连接', 'Connected'),
      authMethod: connection.api_key_prefix ? tx('Scoped token', 'Scoped token') : tx('手动', 'Manual'),
      trustLevel: trustLabel(connection.trust_level, tx),
      lastSync: connection.last_used_at || connection.created_at || '',
      rawName: `${connection.name} ${connection.platform}`,
    }))
    const oauthRows = grants.map((grant) => ({
      id: grant.id,
      kind: 'oauth' as const,
      app: grant.app?.name || 'OAuth App',
      status: tx('已连接', 'Connected'),
      authMethod: 'OAuth',
      trustLevel: grant.scopes?.includes('admin') ? tx('L4 完全信任', 'L4 Full Trust') : tx('L3 工作信任', 'L3 Work Trust'),
      lastSync: grant.created_at,
      rawName: `${grant.app?.name || ''} ${grant.app?.client_id || ''} ${(grant.app?.redirect_uris || []).join(' ')}`,
    }))
    return [...manualRows, ...oauthRows]
  }, [grants, manual])

  const revoke = async (row: ConnectedRow) => {
    if (!window.confirm(tx(`撤销 ${row.app}？`, `Revoke ${row.app}?`))) return
    try {
      if (row.kind === 'manual') await api.deleteConnection(row.id)
      else await api.revokeOAuthGrant(row.id)
      await load()
    } catch (err: any) {
      setError(err?.message || tx('撤销失败', 'Failed to revoke connection'))
    }
  }

  if (loading) return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>

  return (
    <div className="page connections-page">
      {error && <div className="alert alert-warn">{error}</div>}

      <section className="card">
        <div className="card-header">
          <h3 className="card-title">{tx('已连接应用', 'Connected Apps')}</h3>
        </div>
        {rows.length > 0 ? (
          <table className="data-table">
            <thead>
              <tr>
                <th>{tx('应用', 'App')}</th>
                <th>{tx('状态', 'Status')}</th>
                <th>{tx('认证方式', 'Auth method')}</th>
                <th>{tx('信任等级', 'Trust level')}</th>
                <th>{tx('上次同步', 'Last sync')}</th>
                <th>{tx('操作', 'Actions')}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={`${row.kind}:${row.id}`}>
                  <td><strong>{row.app}</strong><small>{row.rawName}</small></td>
                  <td>{row.status}</td>
                  <td>{row.authMethod}</td>
                  <td>{row.trustLevel}</td>
                  <td>{formatDateTime(row.lastSync, locale)}</td>
                  <td>
                    <div className="table-actions">
                      <button className="btn-text" onClick={() => { void revoke(row) }}>{tx('撤销', 'Revoke')}</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-action-state">
            <p>{tx('连接第一个 AI 应用后，neuDrive 就可以开始同步记忆。', 'Connect your first AI app to start syncing memory with neuDrive.')}</p>
            <Link className="btn btn-primary" to="/onboarding">{tx('立即连接', 'Connect now')}</Link>
          </div>
        )}
      </section>
    </div>
  )
}
