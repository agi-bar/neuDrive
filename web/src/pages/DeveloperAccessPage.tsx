import { useEffect, useMemo, useState } from 'react'
import { api, type ScopedTokenResponse } from '../api'
import { useI18n } from '../i18n'
import { formatDateTime } from './data/DataShared'

type TokenPurpose = 'browser' | 'cli' | 'gpt' | 'custom'

const purposePresets: Record<TokenPurpose, {
  label: string
  copy: string
  name: string
  trust: number
  days: number
  scopes: string[]
}> = {
  browser: {
    label: 'Browser Extension',
    copy: 'Read memory, read files, and write conversations from the browser.',
    name: 'Browser Extension',
    trust: 3,
    days: 90,
    scopes: ['read:profile', 'read:memory', 'read:tree', 'write:tree', 'search'],
  },
  cli: {
    label: 'CLI / Local app',
    copy: 'Full local agent access for CLI workflows.',
    name: 'CLI Local App',
    trust: 4,
    days: 90,
    scopes: ['read:profile', 'write:profile', 'read:memory', 'write:memory', 'read:tree', 'write:tree', 'read:skills', 'read:projects', 'write:projects', 'search'],
  },
  gpt: {
    label: 'ChatGPT App / MCP client',
    copy: 'Scoped token for ChatGPT Apps fallback, MCP clients, and OpenAPI usage.',
    name: 'ChatGPT App Token',
    trust: 3,
    days: 90,
    scopes: ['read:profile', 'read:memory', 'write:memory', 'read:tree', 'write:tree', 'search'],
  },
  custom: {
    label: 'Custom Agent',
    copy: 'Choose scopes manually for a custom integration.',
    name: 'Custom Agent',
    trust: 3,
    days: 30,
    scopes: ['read:profile', 'read:memory', 'read:tree', 'search'],
  },
}

function trustLabel(level: number) {
  switch (level) {
    case 1: return 'L1 Guest'
    case 2: return 'L2 Shared'
    case 3: return 'L3 Work Trust'
    case 4: return 'L4 Full Trust'
    default: return `L${level}`
  }
}

export default function DeveloperAccessPage() {
  const { locale, tx } = useI18n()
  const [tokens, setTokens] = useState<ScopedTokenResponse[]>([])
  const [purpose, setPurpose] = useState<TokenPurpose>('browser')
  const [name, setName] = useState(purposePresets.browser.name)
  const [trust, setTrust] = useState(purposePresets.browser.trust)
  const [days, setDays] = useState(purposePresets.browser.days)
  const [scopes, setScopes] = useState<string[]>(purposePresets.browser.scopes)
  const [availableScopes, setAvailableScopes] = useState<string[]>([])
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [newToken, setNewToken] = useState('')
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')

  const load = async () => {
    setLoading(true)
    const [tokenResult, scopesResult] = await Promise.allSettled([
      api.getTokens(),
      api.getTokenScopes(),
    ])
    if (tokenResult.status === 'fulfilled') setTokens(tokenResult.value || [])
    if (scopesResult.status === 'fulfilled') setAvailableScopes(scopesResult.value.scopes || [])
    setLoading(false)
  }

  useEffect(() => {
    void load()
  }, [])

  const activeTokens = useMemo(() => tokens.filter((token) => !token.is_expired && !token.is_revoked), [tokens])

  const selectPurpose = (next: TokenPurpose) => {
    const preset = purposePresets[next]
    setPurpose(next)
    setName(preset.name)
    setTrust(preset.trust)
    setDays(preset.days)
    setScopes(preset.scopes)
    setShowAdvanced(next === 'custom')
    setNewToken('')
  }

  const createToken = async () => {
    setCreating(true)
    setError('')
    setNewToken('')
    try {
      const response = await api.createToken({
        name,
        scopes,
        max_trust_level: trust,
        expires_in_days: days,
      })
      setNewToken(response.token)
      await load()
    } catch (err: any) {
      setError(err?.message || tx('创建 token 失败', 'Failed to create token'))
    } finally {
      setCreating(false)
    }
  }

  const revoke = async (token: ScopedTokenResponse) => {
    if (!window.confirm(tx(`吊销 ${token.name}？`, `Revoke ${token.name}?`))) return
    await api.revokeToken(token.id)
    await load()
  }

  const rename = async (token: ScopedTokenResponse) => {
    const nextName = window.prompt(tx('新的 token 名称', 'New token name'), token.name)
    if (!nextName) return
    await api.updateToken(token.id, { name: nextName })
    await load()
  }

  if (loading) return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>

  return (
    <div className="page developer-page">
      <div className="page-header compact-header">
        <div>
          <h2>{tx('Developer Access', 'Developer Access')}</h2>
          <p className="page-subtitle">{tx('Create scoped tokens for CLI, ChatGPT Apps, browser extensions and custom agents.', 'Create scoped tokens for CLI, ChatGPT Apps, browser extensions and custom agents.')}</p>
        </div>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}

      <section className="developer-layout">
        <div className="card">
          <div className="card-header"><h3 className="card-title">{tx('Create token for', 'Create token for')}</h3></div>
          <div className="purpose-grid">
            {(Object.keys(purposePresets) as TokenPurpose[]).map((key) => (
              <button key={key} className={purpose === key ? 'purpose-card active' : 'purpose-card'} onClick={() => selectPurpose(key)}>
                <strong>{purposePresets[key].label}</strong>
                <span>{purposePresets[key].copy}</span>
              </button>
            ))}
          </div>

          <div className="developer-form">
            <label>Name<input className="input" value={name} onChange={(event) => setName(event.target.value)} /></label>
            <label>Trust level
              <select value={trust} onChange={(event) => setTrust(Number(event.target.value))}>
                {[1, 2, 3, 4].map((level) => <option key={level} value={level}>{trustLabel(level)}</option>)}
              </select>
            </label>
            <label>Expires
              <select value={days} onChange={(event) => setDays(Number(event.target.value))}>
                <option value={7}>7 days</option>
                <option value={30}>30 days</option>
                <option value={90}>90 days</option>
                <option value={365}>365 days</option>
                <option value={0}>Never</option>
              </select>
            </label>
          </div>

          <button className="btn-text" onClick={() => setShowAdvanced((value) => !value)}>Customize scopes</button>
          {showAdvanced && (
            <div className="scope-chip-grid">
              {availableScopes.map((scope) => (
                <label key={scope} className="scope-chip">
                  <input type="checkbox" checked={scopes.includes(scope)} onChange={() => setScopes((current) => current.includes(scope) ? current.filter((item) => item !== scope) : [...current, scope])} />
                  {scope}
                </label>
              ))}
            </div>
          )}

          <button className="btn btn-primary" disabled={creating || !name.trim() || scopes.length === 0} onClick={() => { void createToken() }}>
            {creating ? tx('生成中...', 'Creating...') : tx('Create token', 'Create token')}
          </button>

          {newToken && (
            <div className="token-once-card">
              <strong>{tx('Copy this token now. You will not be able to see it again.', 'Copy this token now. You will not be able to see it again.')}</strong>
              <code>{newToken}</code>
              <code>export NEUDRIVE_TOKEN={newToken}</code>
              <button className="btn btn-outline" onClick={() => { void navigator.clipboard?.writeText(newToken) }}>Copy token</button>
            </div>
          )}
        </div>

        <div className="card">
          <div className="card-header"><h3 className="card-title">{tx('Existing Tokens', 'Existing Tokens')} · {activeTokens.length}</h3></div>
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Purpose</th>
                <th>Trust level</th>
                <th>Expires</th>
                <th>Last used</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {tokens.map((token) => (
                <tr key={token.id} className={token.is_expired || token.is_revoked ? 'is-muted' : ''}>
                  <td><strong>{token.name}</strong><small>{token.token_prefix}...</small></td>
                  <td>{purposeFromScopes(token.scopes)}</td>
                  <td>{trustLabel(token.max_trust_level)}</td>
                  <td>{token.expires_at ? formatDateTime(token.expires_at, locale) : 'Never'}</td>
                  <td>{formatDateTime(token.last_used_at, locale)}</td>
                  <td>
                    <div className="table-actions">
                      <button className="btn-text" onClick={() => { void rename(token) }}>Rename</button>
                      {!token.is_revoked && <button className="btn-text" onClick={() => { void revoke(token) }}>Revoke</button>}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {tokens.length === 0 && <div className="empty-action-state"><p>No tokens yet.</p></div>}
        </div>
      </section>
    </div>
  )
}

function purposeFromScopes(scopes: string[]) {
  if (scopes.includes('write:projects')) return 'CLI / Local app'
  if (scopes.includes('write:tree')) return 'Browser / ChatGPT App'
  if (scopes.length === 0) return 'Unknown'
  return 'Custom Agent'
}
