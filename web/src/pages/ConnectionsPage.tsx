import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type ConnectionResponse, type OAuthGrantResponse } from '../api'
import { useI18n } from '../i18n'
import { formatDateTime } from './data/DataShared'

type Platform = {
  key: string
  name: string
  method: string
  setupTime: string
  trust: string
}

const platforms: Platform[] = [
  { key: 'claude', name: 'Claude', method: 'Remote MCP', setupTime: '~2 min', trust: 'L4 Full Trust' },
  { key: 'chatgpt', name: 'ChatGPT Apps', method: 'Remote MCP via Apps', setupTime: '~3 min', trust: 'L3 Work Trust' },
  { key: 'cursor', name: 'Cursor', method: 'MCP server', setupTime: '~2 min', trust: 'L3 Work Trust' },
  { key: 'windsurf', name: 'Windsurf', method: 'MCP server', setupTime: '~2 min', trust: 'L3 Work Trust' },
  { key: 'claude-code', name: 'Claude Code', method: 'CLI MCP', setupTime: '~2 min', trust: 'L4 Full Trust' },
  { key: 'codex', name: 'Codex CLI', method: 'Remote MCP', setupTime: '~2 min', trust: 'L3 Work Trust' },
  { key: 'gemini', name: 'Gemini CLI', method: 'Remote MCP', setupTime: '~2 min', trust: 'L3 Work Trust' },
  { key: 'other', name: 'Other MCP Client', method: 'Remote MCP', setupTime: '~5 min', trust: 'Custom' },
]

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

function trustLabel(level?: number) {
  switch (level) {
    case 1: return 'L1 Guest'
    case 2: return 'L2 Shared'
    case 3: return 'L3 Work Trust'
    case 4: return 'L4 Full Trust'
    default: return 'Inherited'
  }
}

function platformTerms(platform: string) {
  if (platform === 'claude-code') return ['claude-code', 'claude code']
  if (platform === 'other') return ['mcp', 'custom', 'other']
  return [platform]
}

function isConnected(platform: string, rows: ConnectedRow[]) {
  const terms = platformTerms(platform)
  return rows.some((row) => {
    const haystack = `${row.app} ${row.rawName} ${row.authMethod}`.toLowerCase()
    return terms.some((term) => haystack.includes(term))
  })
}

export default function ConnectionsPage() {
  const { locale, tx } = useI18n()
  const [manual, setManual] = useState<ConnectionResponse[]>([])
  const [grants, setGrants] = useState<OAuthGrantResponse[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [testMessage, setTestMessage] = useState('')

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
      app: connection.platform || connection.name || 'Custom Agent',
      status: 'Connected',
      authMethod: connection.api_key_prefix ? 'Scoped token' : 'Manual',
      trustLevel: trustLabel(connection.trust_level),
      lastSync: connection.last_used_at || connection.created_at || '',
      rawName: `${connection.name} ${connection.platform}`,
    }))
    const oauthRows = grants.map((grant) => ({
      id: grant.id,
      kind: 'oauth' as const,
      app: grant.app?.name || 'OAuth App',
      status: 'Connected',
      authMethod: 'OAuth',
      trustLevel: grant.scopes?.includes('admin') ? 'L4 Full Trust' : 'L3 Work Trust',
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

  const testPlatform = (platform: Platform) => {
    const connected = isConnected(platform.key, rows)
    setTestMessage(connected
      ? tx(`${platform.name} is connected.`, `${platform.name} is connected.`)
      : tx(`${platform.name} 还没有连接。打开接入向导完成配置。`, `${platform.name} is not connected yet. Open the setup wizard to finish configuration.`))
  }

  if (loading) return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>

  return (
    <div className="page connections-page">
      <div className="page-header compact-header">
        <div>
          <h2>Connections</h2>
          <p className="page-subtitle">{tx('Connect neuDrive to the AI tools you use every day.', 'Connect neuDrive to the AI tools you use every day.')}</p>
        </div>
        <div className="page-actions">
          <Link to="/onboarding" className="btn btn-primary">{tx('连接应用', 'Connect app')}</Link>
        </div>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}
      {testMessage && <div className="alert alert-warn">{testMessage}</div>}

      {rows.length === 0 && (
        <section className="activation-banner">
          <div>
            <h3>{tx('No apps connected yet.', 'No apps connected yet.')}</h3>
            <p>{tx('Start with Claude, ChatGPT or Cursor.', 'Start with Claude, ChatGPT or Cursor.')}</p>
          </div>
          <div className="activation-actions">
            <Link className="btn btn-primary" to="/onboarding/claude">Connect Claude</Link>
            <Link className="btn btn-outline" to="/onboarding/chatgpt">Connect ChatGPT</Link>
            <Link className="btn btn-outline" to="/onboarding">More options</Link>
          </div>
        </section>
      )}

      <section className="platform-grid">
        {platforms.map((platform) => {
          const connected = isConnected(platform.key, rows)
          return (
            <article key={platform.key} className="platform-card platform-card-static">
              <div className="platform-card-head">
                <strong>{platform.name}</strong>
                <span className={connected ? 'status-pill connected' : 'status-pill'}>{connected ? 'Connected' : 'Not connected'}</span>
              </div>
              <span>{platform.method} · {platform.setupTime}</span>
              <small>{platform.trust}</small>
              <div className="platform-card-actions">
                <Link className="btn btn-primary" to={`/onboarding/${platform.key === 'claude-code' ? 'claude' : platform.key}`}>{connected ? 'Manage' : 'Connect'}</Link>
                <button className="btn btn-outline" onClick={() => testPlatform(platform)}>Test</button>
              </div>
            </article>
          )
        })}
      </section>

      <section className="card">
        <div className="card-header">
          <h3 className="card-title">{tx('Connected Apps', 'Connected Apps')}</h3>
        </div>
        {rows.length > 0 ? (
          <table className="data-table">
            <thead>
              <tr>
                <th>App</th>
                <th>Status</th>
                <th>Auth method</th>
                <th>Trust level</th>
                <th>Last sync</th>
                <th>Actions</th>
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
                      <button className="btn-text" onClick={() => setTestMessage(`${row.app}: Connected`)}>Test</button>
                      <button className="btn-text" onClick={() => { void revoke(row) }}>Revoke</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-action-state">
            <p>{tx('Connect your first AI app to start syncing memory.', 'Connect your first AI app to start syncing memory.')}</p>
            <Link className="btn btn-primary" to="/onboarding">Connect now</Link>
          </div>
        )}
      </section>
    </div>
  )
}
