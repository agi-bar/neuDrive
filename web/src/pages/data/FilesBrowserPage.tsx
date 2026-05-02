import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { api, type FileNode } from '../../api'
import { useI18n } from '../../i18n'
import { dataFileEditorRoute, formatDateTime, sourceLabel } from './DataShared'

function nodeType(node: FileNode) {
  const path = node.path.toLowerCase()
  if (node.bundle_context?.kind === 'conversation' || path.startsWith('/conversations/')) return 'Conversation'
  if (node.bundle_context?.kind === 'skill' || path.startsWith('/skills/')) return 'Skill'
  if (node.bundle_context?.kind === 'project' || path.startsWith('/projects/')) return 'Project'
  if (path.startsWith('/memory/')) return 'Memory'
  if (path.startsWith('/vault/')) return 'Vault'
  if (node.is_dir) return 'Folder'
  return 'File'
}

function accessLabel(level?: number) {
  switch (level) {
    case 1: return 'L1 Guest'
    case 2: return 'L2 Shared'
    case 3: return 'L3 Work'
    case 4: return 'L4 Full'
    default: return 'Inherited'
  }
}

function summarizeContent(node: FileNode) {
  const explicit = node.metadata?.summary || node.metadata?.description || node.bundle_context?.description
  if (typeof explicit === 'string' && explicit.trim()) return explicit.trim()
  const content = (node.content || '').replace(/\s+/g, ' ').trim()
  if (content) return content.slice(0, 180)
  return node.is_dir ? 'Directory or bundle container.' : 'No summary available yet.'
}

export default function FilesBrowserPage() {
  const { locale, tx } = useI18n()
  const navigate = useNavigate()
  const location = useLocation()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [entries, setEntries] = useState<FileNode[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [typeFilter, setTypeFilter] = useState(new URLSearchParams(location.search).get('type') || 'all')
  const [sourceFilter, setSourceFilter] = useState(new URLSearchParams(location.search).get('source') || 'all')
  const [accessFilter, setAccessFilter] = useState(new URLSearchParams(location.search).get('access') || 'all')
  const [selected, setSelected] = useState<FileNode | null>(null)
  const [uploading, setUploading] = useState(false)
  const [copied, setCopied] = useState('')

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const snapshot = await api.getTreeSnapshot('/')
      setEntries(snapshot.entries)
    } catch (err: any) {
      setError(err?.message || tx('加载 Data Explorer 失败', 'Failed to load Data Explorer'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const summary = useMemo(() => {
    const byType = new Map<string, number>()
    const bySource = new Map<string, number>()
    for (const entry of entries) {
      byType.set(nodeType(entry), (byType.get(nodeType(entry)) || 0) + 1)
      const source = entry.source || entry.metadata?.source || entry.bundle_context?.source || 'system'
      bySource.set(String(source), (bySource.get(String(source)) || 0) + 1)
    }
    return { byType, bySource }
  }, [entries])

  const typeOptions = ['all', ...Array.from(summary.byType.keys()).sort()]
  const sourceOptions = ['all', ...Array.from(summary.bySource.keys()).sort()]

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return entries
      .filter((entry) => typeFilter === 'all' || nodeType(entry) === typeFilter)
      .filter((entry) => {
        const source = String(entry.source || entry.metadata?.source || entry.bundle_context?.source || 'system')
        return sourceFilter === 'all' || source === sourceFilter
      })
      .filter((entry) => accessFilter === 'all' || String(entry.min_trust_level || '') === accessFilter)
      .filter((entry) => {
        if (!q) return true
        const haystack = `${entry.name} ${entry.path} ${entry.source || ''} ${nodeType(entry)}`.toLowerCase()
        return haystack.includes(q)
      })
      .sort((a, b) => {
        const at = new Date(a.updated_at || a.created_at || 0).getTime()
        const bt = new Date(b.updated_at || b.created_at || 0).getTime()
        return bt - at
      })
  }, [accessFilter, entries, query, sourceFilter, typeFilter])

  const syncFiltersToURL = (patch: Record<string, string>) => {
    const params = new URLSearchParams(location.search)
    for (const [key, value] of Object.entries(patch)) {
      if (!value || value === 'all') params.delete(key)
      else params.set(key, value)
    }
    navigate({ search: params.toString() }, { replace: true })
  }

  const handleUpload = async (file: File) => {
    setUploading(true)
    setError('')
    try {
      const text = await file.text()
      await api.writeTree(`/uploads/${file.name}`, {
        content: text,
        mimeType: file.type || (file.name.toLowerCase().endsWith('.md') ? 'text/markdown' : 'text/plain'),
        metadata: { source: 'manual-upload' },
      })
      await load()
    } catch (err: any) {
      setError(err?.message || tx('上传失败', 'Upload failed'))
    } finally {
      setUploading(false)
    }
  }

  const deleteEntry = async (entry: FileNode) => {
    if (!window.confirm(tx(`删除 ${entry.name}？`, `Delete ${entry.name}?`))) return
    try {
      await api.deleteTree(entry.path)
      setSelected(null)
      await load()
    } catch (err: any) {
      setError(err?.message || tx('删除失败', 'Delete failed'))
    }
  }

  const copyPrompt = async (entry: FileNode) => {
    const prompt = `Use this neuDrive item as context: ${entry.path}. Summarize it and explain how it should influence the current task.`
    await navigator.clipboard?.writeText(prompt)
    localStorage.setItem('neudrive.testPromptCopied', '1')
    setCopied(entry.path)
    window.setTimeout(() => setCopied(''), 1600)
  }

  if (loading) return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>

  return (
    <div className="page data-explorer-page">
      <div className="page-header compact-header">
        <div>
          <h2>Data Explorer</h2>
          <p className="page-subtitle">{tx('This is the data your AI agents can access.', 'This is the data your AI agents can access.')}</p>
        </div>
        <div className="page-actions">
          <Link to="/imports/claude-export" className="btn btn-primary">{tx('导入数据', 'Import data')}</Link>
          <button className="btn btn-outline" disabled={uploading} onClick={() => fileInputRef.current?.click()}>{uploading ? tx('上传中...', 'Uploading...') : tx('上传文件', 'Upload file')}</button>
          <button className="btn btn-outline" onClick={() => { void api.exportZip() }}>{tx('导出', 'Export')}</button>
          <input ref={fileInputRef} className="hidden-file-input" type="file" accept=".md,.txt,.json,.csv" onChange={(event) => {
            const file = event.target.files?.[0]
            if (file) void handleUpload(file)
            event.currentTarget.value = ''
          }} />
        </div>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}

      <section className="data-summary-grid">
        <div className="data-summary-card">
          <span>{tx('Stored data', 'Stored data')}</span>
          <strong>{entries.length}</strong>
          <small>{tx('items', 'items')}</small>
        </div>
        {Array.from(summary.byType.entries()).slice(0, 5).map(([type, count]) => (
          <button key={type} className="data-summary-card" onClick={() => {
            setTypeFilter(type)
            syncFiltersToURL({ type })
          }}>
            <span>{type}</span>
            <strong>{count}</strong>
            <small>{tx('by type', 'by type')}</small>
          </button>
        ))}
      </section>

      <section className="data-filter-bar">
        <input className="input" placeholder={tx('搜索数据', 'Search data')} value={query} onChange={(event) => setQuery(event.target.value)} />
        <select value={typeFilter} onChange={(event) => {
          setTypeFilter(event.target.value)
          syncFiltersToURL({ type: event.target.value })
        }}>
          {typeOptions.map((option) => <option key={option} value={option}>{option === 'all' ? 'Type: All' : option}</option>)}
        </select>
        <select value={sourceFilter} onChange={(event) => {
          setSourceFilter(event.target.value)
          syncFiltersToURL({ source: event.target.value })
        }}>
          {sourceOptions.map((option) => <option key={option} value={option}>{option === 'all' ? 'Source: All' : sourceLabel(option, locale)}</option>)}
        </select>
        <select value={accessFilter} onChange={(event) => {
          setAccessFilter(event.target.value)
          syncFiltersToURL({ access: event.target.value })
        }}>
          <option value="all">Access: All</option>
          <option value="1">L1 Guest</option>
          <option value="2">L2 Shared</option>
          <option value="3">L3 Work</option>
          <option value="4">L4 Full</option>
        </select>
      </section>

      <section className="data-table-wrap">
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Source</th>
              <th>Access</th>
              <th>Size</th>
              <th>Last updated</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((entry) => (
              <tr key={entry.path} className={selected?.path === entry.path ? 'is-selected' : ''} onClick={() => setSelected(entry)}>
                <td>
                  <strong>{entry.name}</strong>
                  <small>{entry.path}</small>
                </td>
                <td>{nodeType(entry)}</td>
                <td>{sourceLabel(String(entry.source || entry.metadata?.source || entry.bundle_context?.source || 'system'), locale)}</td>
                <td>{accessLabel(entry.min_trust_level || entry.bundle_context?.min_trust_level)}</td>
                <td>{entry.is_dir ? '-' : `${Math.max(0, entry.size || entry.content?.length || 0)} B`}</td>
                <td>{formatDateTime(entry.updated_at || entry.created_at, locale)}</td>
                <td>
                  <div className="table-actions">
                    <button className="btn-text" onClick={(event) => {
                      event.stopPropagation()
                      setSelected(entry)
                    }}>Preview</button>
                    {!entry.is_dir && <Link onClick={(event) => event.stopPropagation()} to={dataFileEditorRoute(entry.path)}>Open</Link>}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      {filtered.length === 0 && (
        <div className="empty-action-state">
          <p>{tx('No data matches these filters.', 'No data matches these filters.')}</p>
          <button className="btn btn-primary" onClick={() => {
            setQuery('')
            setTypeFilter('all')
            setSourceFilter('all')
            setAccessFilter('all')
            navigate({ search: '' }, { replace: true })
          }}>{tx('清除筛选', 'Clear filters')}</button>
        </div>
      )}

      {selected && (
        <aside className="preview-drawer">
          <button className="drawer-close" onClick={() => setSelected(null)}>×</button>
          <h3>{selected.name}</h3>
          <code>{selected.path}</code>
          <dl className="preview-meta">
            <div><dt>Type</dt><dd>{nodeType(selected)}</dd></div>
            <div><dt>Source</dt><dd>{sourceLabel(String(selected.source || selected.metadata?.source || selected.bundle_context?.source || 'system'), locale)}</dd></div>
            <div><dt>Access</dt><dd>{accessLabel(selected.min_trust_level || selected.bundle_context?.min_trust_level)}</dd></div>
            <div><dt>Last updated</dt><dd>{formatDateTime(selected.updated_at || selected.created_at, locale)}</dd></div>
          </dl>
          <h4>Summary</h4>
          <p>{summarizeContent(selected)}</p>
          <h4>{tx('谁可以访问', 'Who can access')}</h4>
          <p>{accessLabel(selected.min_trust_level || selected.bundle_context?.min_trust_level)} {tx('及以上信任等级的 Agent。', 'and higher trust agents.')}</p>
          <div className="drawer-actions">
            {!selected.is_dir && <Link className="btn btn-primary" to={dataFileEditorRoute(selected.path)}>Open</Link>}
            <button className="btn btn-outline" onClick={() => { void copyPrompt(selected) }}>
              {copied === selected.path ? tx('已复制', 'Copied') : tx('Ask AI about this', 'Ask AI about this')}
            </button>
            <button className="btn btn-outline" onClick={() => { void api.downloadTreeZip(selected.path) }}>Export</button>
            <button className="btn btn-danger" onClick={() => { void deleteEntry(selected) }}>Delete</button>
          </div>
        </aside>
      )}
    </div>
  )
}
