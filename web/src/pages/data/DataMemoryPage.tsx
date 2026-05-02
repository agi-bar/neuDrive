import { FormEvent, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, type FileNode, type MemoryConflict } from '../../api'
import { useI18n } from '../../i18n'
import { dataFileEditorRoute, formatDateTime, sourceLabel } from './DataShared'

type MemoryTab = 'profile' | 'project' | 'scratch' | 'conflicts'

function titleFromPath(path: string) {
  return path.split('/').pop()?.replace(/\.md$/i, '').replace(/[-_]+/g, ' ') || path
}

export default function DataMemoryPage() {
  const { locale, tx } = useI18n()
  const navigate = useNavigate()
  const [tab, setTab] = useState<MemoryTab>('profile')
  const [profileEntries, setProfileEntries] = useState<FileNode[]>([])
  const [projectEntries, setProjectEntries] = useState<FileNode[]>([])
  const [scratch, setScratch] = useState('')
  const [conflicts, setConflicts] = useState<MemoryConflict[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [newMemoryName, setNewMemoryName] = useState('working-note.md')

  const load = async () => {
    setLoading(true)
    setError('')
    const [profileResult, projectsResult, scratchResult, conflictsResult] = await Promise.allSettled([
      api.getTreeSnapshot('/memory/profile'),
      api.getTreeSnapshot('/projects'),
      api.getScratchMemory(),
      api.getConflicts(),
    ])
    if (profileResult.status === 'fulfilled') setProfileEntries(profileResult.value.entries.filter((entry) => !entry.is_dir))
    if (projectsResult.status === 'fulfilled') setProjectEntries(projectsResult.value.entries)
    if (scratchResult.status === 'fulfilled') {
      const value = scratchResult.value?.content || scratchResult.value?.scratch || scratchResult.value?.text || ''
      setScratch(typeof value === 'string' ? value : JSON.stringify(value, null, 2))
    }
    if (conflictsResult.status === 'fulfilled') setConflicts(conflictsResult.value || [])
    setLoading(false)
  }

  useEffect(() => {
    void load()
  }, [])

  const tabs = useMemo(() => [
    { key: 'profile' as const, label: 'Profile Memory', count: profileEntries.length },
    { key: 'project' as const, label: 'Project Memory', count: projectEntries.length },
    { key: 'scratch' as const, label: 'Scratch Memory', count: scratch.trim() ? 1 : 0 },
    { key: 'conflicts' as const, label: 'Conflicts', count: conflicts.length },
  ], [conflicts.length, profileEntries.length, projectEntries.length, scratch])

  const saveScratch = async () => {
    setSaving(true)
    setError('')
    try {
      await api.writeScratchMemory({ content: scratch })
      setMessage(tx('Scratch Memory 已保存。', 'Scratch Memory saved.'))
    } catch (err: any) {
      setError(err?.message || tx('保存 Scratch Memory 失败', 'Failed to save Scratch Memory'))
    } finally {
      setSaving(false)
    }
  }

  const createMemory = async (event: FormEvent) => {
    event.preventDefault()
    const fileName = newMemoryName.trim().replace(/^\/+/, '')
    if (!fileName) return
    const normalized = fileName.endsWith('.md') ? fileName : `${fileName}.md`
    try {
      const path = `/memory/${normalized}`
      await api.writeTree(path, {
        content: `# ${titleFromPath(normalized)}\n\n`,
        mimeType: 'text/markdown',
        metadata: { source: 'manual' },
        minTrustLevel: 3,
      })
      navigate(dataFileEditorRoute(path))
    } catch (err: any) {
      setError(err?.message || tx('创建 Memory 失败', 'Failed to create memory'))
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

  if (loading) return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>

  return (
    <div className="page memory-page">
      <div className="page-header compact-header">
        <div>
          <h2>Memory</h2>
          <p className="page-subtitle">{tx('Manage long-term preferences, project context, temporary scratch memory, and conflicts.', 'Manage long-term preferences, project context, temporary scratch memory, and conflicts.')}</p>
        </div>
        <form className="inline-create-form" onSubmit={createMemory}>
          <input className="input" value={newMemoryName} onChange={(event) => setNewMemoryName(event.target.value)} />
          <button className="btn btn-primary">Create memory</button>
        </form>
      </div>

      {message && <div className="alert alert-success">{message}</div>}
      {error && <div className="alert alert-warn">{error}</div>}

      <div className="tab-strip">
        {tabs.map((item) => (
          <button key={item.key} className={tab === item.key ? 'active' : ''} onClick={() => setTab(item.key)}>
            {item.label}<span>{item.count}</span>
          </button>
        ))}
      </div>

      {tab === 'profile' && (
        <section className="memory-list">
          {profileEntries.map((entry) => (
            <Link key={entry.path} to={dataFileEditorRoute(entry.path)} className="memory-row">
              <strong>{titleFromPath(entry.path)}</strong>
              <span>{sourceLabel(entry.source, locale)} · {formatDateTime(entry.updated_at || entry.created_at, locale)}</span>
            </Link>
          ))}
          {profileEntries.length === 0 && <div className="empty-action-state"><p>No profile memory yet.</p><Link to="/settings/profile" className="btn btn-primary">Open Profile</Link></div>}
        </section>
      )}

      {tab === 'project' && (
        <section className="memory-list">
          {projectEntries.map((entry) => (
            <Link key={entry.path} to={entry.is_dir ? `/data/projects/${encodeURIComponent(entry.name)}` : dataFileEditorRoute(entry.path)} className="memory-row">
              <strong>{titleFromPath(entry.path)}</strong>
              <span>{entry.is_dir ? 'Project' : sourceLabel(entry.source, locale)} · {formatDateTime(entry.updated_at || entry.created_at, locale)}</span>
            </Link>
          ))}
          {projectEntries.length === 0 && <div className="empty-action-state"><p>No project memory yet.</p><Link to="/data/projects" className="btn btn-primary">Create project</Link></div>}
        </section>
      )}

      {tab === 'scratch' && (
        <section className="card scratch-card">
          <textarea value={scratch} onChange={(event) => setScratch(event.target.value)} placeholder={tx('Temporary notes that can expire or be replaced quickly.', 'Temporary notes that can expire or be replaced quickly.')} />
          <div className="page-actions">
            <button className="btn btn-primary" disabled={saving} onClick={() => { void saveScratch() }}>{saving ? tx('保存中...', 'Saving...') : tx('保存 Scratch', 'Save scratch')}</button>
          </div>
        </section>
      )}

      {tab === 'conflicts' && (
        <section className="conflict-list">
          {conflicts.map((conflict) => (
            <div key={conflict.id} className="conflict-card">
              <strong>{conflict.category}</strong>
              <div className="conflict-options">
                <div><span>{conflict.source_a}</span><p>{conflict.content_a}</p><button className="btn btn-outline" onClick={() => { void resolveConflict(conflict.id, 'keep_a') }}>Keep this</button></div>
                <div><span>{conflict.source_b}</span><p>{conflict.content_b}</p><button className="btn btn-outline" onClick={() => { void resolveConflict(conflict.id, 'keep_b') }}>Keep this</button></div>
              </div>
            </div>
          ))}
          {conflicts.length === 0 && <div className="empty-action-state"><p>No memory conflicts.</p></div>}
        </section>
      )}
    </div>
  )
}
