import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type ConnectionResponse, type FileNode, type MemoryConflict, type OAuthGrantResponse } from '../api'
import { useI18n } from '../i18n'
import { sourceLabel } from './data/DataShared'

interface InfoPageProps {
  title?: string
}

const profileFields = [
  { key: 'preferences', label: 'Work preferences' },
  { key: 'writing_style', label: 'Writing style' },
  { key: 'communication', label: 'Communication preferences' },
  { key: 'principles', label: 'Decision style' },
]

function dataType(node: FileNode) {
  const path = node.path.toLowerCase()
  if (path.startsWith('/conversations/')) return 'Conversations'
  if (path.startsWith('/memory/')) return 'Memory'
  if (path.startsWith('/skills/')) return 'Skills'
  if (path.startsWith('/projects/')) return 'Projects'
  if (path.startsWith('/vault/')) return 'Vault items'
  return 'Files'
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

export default function InfoPage({ title }: InfoPageProps) {
  const { locale, tx } = useI18n()
  const [profile, setProfile] = useState<Record<string, any>>({})
  const [values, setValues] = useState<Record<string, string>>({})
  const [entries, setEntries] = useState<FileNode[]>([])
  const [connections, setConnections] = useState<ConnectionResponse[]>([])
  const [grants, setGrants] = useState<OAuthGrantResponse[]>([])
  const [conflicts, setConflicts] = useState<MemoryConflict[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  const load = async () => {
    setLoading(true)
    setError('')
    const [profileResult, snapshotResult, connectionsResult, grantsResult, conflictsResult] = await Promise.allSettled([
      api.getProfile(),
      api.getTreeSnapshot('/'),
      api.getConnections(),
      api.getOAuthGrants(),
      api.getConflicts(),
    ])
    if (profileResult.status === 'fulfilled') {
      const next = profileResult.value || {}
      setProfile(next)
      const prefs = next.preferences || {}
      setValues({
        preferences: String(prefs.preferences || ''),
        writing_style: String(prefs.writing_style || ''),
        communication: String(prefs.communication || ''),
        principles: String(prefs.principles || ''),
      })
    }
    if (snapshotResult.status === 'fulfilled') setEntries(snapshotResult.value.entries)
    if (connectionsResult.status === 'fulfilled') setConnections(connectionsResult.value || [])
    if (grantsResult.status === 'fulfilled') setGrants(grantsResult.value || [])
    if (conflictsResult.status === 'fulfilled') setConflicts(conflictsResult.value || [])
    setLoading(false)
  }

  useEffect(() => {
    void load()
  }, [])

  const counts = useMemo(() => {
    const map = new Map<string, number>()
    for (const entry of entries) {
      map.set(dataType(entry), (map.get(dataType(entry)) || 0) + 1)
    }
    return ['Conversations', 'Memory', 'Skills', 'Files', 'Vault items', 'Projects'].map((label) => ({
      label,
      count: map.get(label) || 0,
    }))
  }, [entries])

  const sources = useMemo(() => {
    const map = new Map<string, number>()
    for (const entry of entries) {
      const source = String(entry.source || entry.metadata?.source || entry.bundle_context?.source || 'system')
      map.set(source, (map.get(source) || 0) + 1)
    }
    return Array.from(map.entries()).sort((a, b) => b[1] - a[1]).slice(0, 8)
  }, [entries])

  const saveProfile = async () => {
    setSaving('profile')
    setError('')
    try {
      await api.upsertProfile({ preferences: values })
      setMessage(tx('Profile 已保存。', 'Profile saved.'))
      await load()
    } catch (err: any) {
      setError(err?.message || tx('保存失败', 'Save failed'))
    } finally {
      setSaving('')
    }
  }

  const resolveConflict = async (id: string, resolution: string) => {
    try {
      await api.resolveConflict(id, resolution)
      setConflicts((current) => current.filter((item) => item.id !== id))
      setMessage(tx('冲突已解决。', 'Conflict resolved.'))
    } catch (err: any) {
      setError(err?.message || tx('解决冲突失败', 'Failed to resolve conflict'))
    }
  }

  const exportAll = async () => {
    await api.exportZip()
    setMessage(tx('导出已开始。', 'Export started.'))
  }

  const deleteImportedConversations = async () => {
    if (!window.confirm(tx('删除所有导入会话？此操作不可撤销。', 'Delete all imported conversations? This cannot be undone.'))) return
    const conversations = entries.filter((entry) => entry.path.startsWith('/conversations/') && (entry.is_dir || entry.name.endsWith('.md')))
    for (const entry of conversations) {
      await api.deleteTree(entry.path)
    }
    setMessage(tx('导入会话已删除。', 'Imported conversations deleted.'))
    await load()
  }

  const clearMemory = async () => {
    if (!window.confirm(tx('清空 Memory？Profile 之外的记忆会被删除。', 'Clear memory? Memory outside Profile will be deleted.'))) return
    const memory = entries.filter((entry) => entry.path.startsWith('/memory/') && !entry.path.startsWith('/memory/profile/'))
    for (const entry of memory) {
      await api.deleteTree(entry.path)
    }
    setMessage(tx('Memory 已清空。', 'Memory cleared.'))
    await load()
  }

  const revokeAllTokens = async () => {
    if (!window.confirm(tx('撤销所有 token？所有外部 Agent 将失去访问权限。', 'Revoke all tokens? External agents will lose access.'))) return
    const tokens = await api.getTokens()
    for (const token of tokens) {
      if (!token.is_revoked) await api.revokeToken(token.id)
    }
    setMessage(tx('Token 已全部撤销。', 'All tokens revoked.'))
  }

  if (loading) return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>

  return (
    <div className="page profile-page">
      <div className="page-header compact-header">
        <div>
          <h2>{title || tx('My Profile', 'My Profile')}</h2>
          <p className="page-subtitle">{tx('See what neuDrive knows about you, where it came from, and which agents can use it.', 'See what neuDrive knows about you, where it came from, and which agents can use it.')}</p>
        </div>
        <div className="page-actions">
          <button className="btn btn-primary" disabled={saving !== ''} onClick={() => { void saveProfile() }}>{saving ? tx('保存中...', 'Saving...') : tx('保存 Profile', 'Save Profile')}</button>
        </div>
      </div>

      {message && <div className="alert alert-success">{message}</div>}
      {error && <div className="alert alert-warn">{error}</div>}

      <section className="profile-layout">
        <div className="card profile-main-card">
          <div className="card-header">
            <h3 className="card-title">{tx('What neuDrive knows about you', 'What neuDrive knows about you')}</h3>
          </div>
          <div className="profile-field-grid">
            <div className="profile-readonly-field">
              <span>Name</span>
              <strong>{profile.display_name || '-'}</strong>
            </div>
            <div className="profile-readonly-field">
              <span>Language preference</span>
              <strong>{profile.language || locale}</strong>
            </div>
            {profileFields.map((field) => (
              <label key={field.key} className="profile-edit-field">
                <span>{field.label}</span>
                <textarea value={values[field.key] || ''} onChange={(event) => setValues({ ...values, [field.key]: event.target.value })} />
              </label>
            ))}
          </div>
        </div>

        <aside className="card">
          <div className="card-header">
            <h3 className="card-title">Memory Map</h3>
          </div>
          <div className="memory-map-grid">
            {counts.map((item) => (
              <Link key={item.label} to={`/data/files?type=${encodeURIComponent(item.label.replace(/ items$/, ''))}`} className="memory-map-item">
                <span>{item.label}</span>
                <strong>{item.count}</strong>
              </Link>
            ))}
          </div>
        </aside>
      </section>

      <section className="profile-layout">
        <div className="card">
          <div className="card-header">
            <h3 className="card-title">{tx('Data sources', 'Data sources')}</h3>
          </div>
          <div className="source-list">
            {sources.map(([source, count]) => (
              <div key={source} className="source-row">
                <span>{sourceLabel(source, locale)}</span>
                <strong>{count}</strong>
              </div>
            ))}
          </div>
        </div>

        <div className="card">
          <div className="card-header">
            <h3 className="card-title">{tx('Who can access your data?', 'Who can access your data?')}</h3>
          </div>
          <div className="source-list">
            {connections.map((connection) => (
              <div key={connection.id} className="source-row">
                <span>{connection.name || connection.platform}</span>
                <strong>{trustLabel(connection.trust_level)}</strong>
              </div>
            ))}
            {grants.map((grant) => (
              <div key={grant.id} className="source-row">
                <span>{grant.app?.name || 'OAuth App'}</span>
                <strong>{grant.scopes?.includes('admin') ? 'L4 Full Trust' : 'L3 Work Trust'}</strong>
              </div>
            ))}
            {connections.length === 0 && grants.length === 0 && <p className="dashboard-empty-copy">{tx('No agents connected yet.', 'No agents connected yet.')}</p>}
          </div>
        </div>
      </section>

      {conflicts.length > 0 && (
        <section className="card">
          <div className="card-header">
            <h3 className="card-title">{tx('Memory conflicts', 'Memory conflicts')}</h3>
          </div>
          <div className="conflict-list">
            {conflicts.map((conflict) => (
              <div key={conflict.id} className="conflict-card">
                <strong>{conflict.category}</strong>
                <div className="conflict-options">
                  <div><span>{conflict.source_a}</span><p>{conflict.content_a}</p><button className="btn btn-outline" onClick={() => { void resolveConflict(conflict.id, 'keep_a') }}>Keep</button></div>
                  <div><span>{conflict.source_b}</span><p>{conflict.content_b}</p><button className="btn btn-outline" onClick={() => { void resolveConflict(conflict.id, 'keep_b') }}>Keep</button></div>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      <section className="card privacy-actions">
        <div className="card-header">
          <h3 className="card-title">{tx('Privacy Actions', 'Privacy Actions')}</h3>
        </div>
        <div className="page-actions">
          <button className="btn btn-outline" onClick={() => { void exportAll() }}>Export all data</button>
          <button className="btn btn-outline" onClick={() => { void deleteImportedConversations() }}>Delete imported conversations</button>
          <button className="btn btn-outline" onClick={() => { void clearMemory() }}>Clear memory</button>
          <button className="btn btn-danger" onClick={() => { void revokeAllTokens() }}>Revoke all tokens</button>
        </div>
      </section>
    </div>
  )
}
