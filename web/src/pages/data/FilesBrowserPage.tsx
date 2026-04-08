import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { api, type FileNode } from '../../api'
import { fileNamespaceLabel } from './DataShared'

type SortKey = 'name' | 'updated_at' | 'size'
type SortDir = 'asc' | 'desc'
type NamespaceFilter = 'all' | 'projects' | 'skills' | 'memory' | 'devices' | 'roles' | 'inbox' | 'notes' | 'root'

const NAMESPACE_CHIPS: Array<{ key: NamespaceFilter; label: string }> = [
  { key: 'all', label: '全部' },
  { key: 'projects', label: 'Projects' },
  { key: 'skills', label: 'Skills' },
  { key: 'memory', label: 'Memory' },
  { key: 'devices', label: 'Devices' },
  { key: 'roles', label: 'Roles' },
  { key: 'inbox', label: 'Inbox' },
  { key: 'notes', label: 'Notes' },
  { key: 'root', label: 'Root' },
]

function useQuery() {
  const { search } = useLocation()
  return useMemo(() => new URLSearchParams(search), [search])
}

function Breadcrumbs({ path, onNavigate }: { path: string; onNavigate: (p: string) => void }) {
  const parts = path.replace(/^\/+/, '').split('/').filter(Boolean)
  const segments = ['/', ...parts.map((_, i) => '/' + parts.slice(0, i + 1).join('/'))]
  const labels = ['根目录', ...parts]
  return (
    <nav aria-label="面包屑" className="breadcrumbs" style={{ marginBottom: 8 }}>
      {segments.map((seg, i) => (
        <span key={seg}>
          {i > 0 && <span className="breadcrumbs-sep">/</span>}
          <button className="btn-text" onClick={() => onNavigate(seg)}>{labels[i]}</button>
        </span>
      ))}
    </nav>
  )
}

function formatBytes(n?: number) {
  if (!n || n <= 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB']
  let v = n
  let u = 0
  while (v >= 1024 && u < units.length - 1) { v /= 1024; u++ }
  return `${v.toFixed(1)} ${units[u]}`
}

function topNamespace(path: string): NamespaceFilter {
  const seg = path.replace(/^\/+/, '').split('/')[0] || ''
  if (seg === 'projects' || seg === 'skills' || seg === 'memory' || seg === 'devices' || seg === 'roles' || seg === 'inbox' || seg === 'notes') {
    return seg
  }
  return 'root'
}

function sortNodes(nodes: FileNode[], key: SortKey, dir: SortDir) {
  const mul = dir === 'asc' ? 1 : -1
  return [...nodes].sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1
    if (key === 'name') return a.name.localeCompare(b.name) * mul
    if (key === 'size') return ((a.size || 0) - (b.size || 0)) * mul
    const ta = new Date(a.updated_at || a.created_at || 0).getTime()
    const tb = new Date(b.updated_at || b.created_at || 0).getTime()
    return (ta - tb) * mul
  })
}

export default function FilesBrowserPage() {
  const params = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const query = useQuery()
  const wildcard = params['*'] || ''
  const currentPath = useMemo(() => (wildcard ? '/' + decodeURIComponent(wildcard) : '/'), [wildcard])
  const [items, setItems] = useState<FileNode[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [creatingDir, setCreatingDir] = useState(false)
  const [newDirName, setNewDirName] = useState('新建文件夹')
  const [creatingFile, setCreatingFile] = useState(false)
  const [newFileName, setNewFileName] = useState('新建文档.md')
  const [searchInput, setSearchInput] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  const [namespaceFilter, setNamespaceFilter] = useState<NamespaceFilter>('all')
  const fileInputRef = useRef<HTMLInputElement>(null)

  const sortKey = (query.get('sort') as SortKey) || 'updated_at'
  const sortDir = (query.get('dir') as SortDir) || 'desc'
  const searchMode = Boolean(appliedSearch.trim())

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      if (searchMode) {
        const results = await api.search(appliedSearch.trim())
        const mapped: FileNode[] = results.map(r => ({
          path: r.path,
          name: r.name,
          is_dir: false,
          kind: r.kind,
          content: r.content,
          updated_at: r.updated_at,
        }))
        setItems(sortNodes(mapped, sortKey, sortDir))
      } else {
        const root = await api.getTree(currentPath)
        setItems(sortNodes(root.children || [], sortKey, sortDir))
      }
    } catch (err: any) {
      setError(err.message || '加载失败')
    } finally {
      setLoading(false)
    }
  }, [appliedSearch, currentPath, searchMode, sortKey, sortDir])

  useEffect(() => { refresh() }, [refresh])

  const toggleSort = (key: SortKey) => {
    const dir: SortDir = (sortKey === key ? (sortDir === 'asc' ? 'desc' : 'asc') : 'asc')
    const params = new URLSearchParams(location.search)
    params.set('sort', key)
    params.set('dir', dir)
    navigate({ search: params.toString() })
  }

  const onNavigatePath = (p: string) => {
    const normalized = p.replace(/^\/+/, '')
    navigate(normalized ? `/data/files/${encodeURIComponent(normalized)}` : '/data/files')
  }

  const handleOpen = (item: FileNode) => {
    if (item.is_dir) {
      onNavigatePath(item.path)
      return
    }
    navigate(`/data/files/edit/${encodeURIComponent(item.path.replace(/^\/+/, ''))}`)
  }

  const runSearch = () => {
    setAppliedSearch(searchInput.trim())
  }

  const clearSearch = () => {
    setSearchInput('')
    setAppliedSearch('')
  }

  const handleNewDir = async () => {
    if (!newDirName.trim()) return
    const target = (currentPath.endsWith('/') ? currentPath.slice(0, -1) : currentPath) + '/' + newDirName.trim()
    try {
      await api.writeTree(target, { content: '', isDir: true })
      setCreatingDir(false)
      setNewDirName('新建文件夹')
      refresh()
    } catch (e: any) {
      alert(e.message || '新建文件夹失败')
    }
  }

  const handleNewFile = async () => {
    if (!newFileName.trim()) return
    const target = (currentPath.endsWith('/') ? currentPath.slice(0, -1) : currentPath) + '/' + newFileName.trim()
    try {
      await api.writeTree(target, { content: '# 新文档\n', mimeType: target.toLowerCase().endsWith('.md') ? 'text/markdown' : 'text/plain' })
      setCreatingFile(false)
      setNewFileName('新建文档.md')
      refresh()
    } catch (e: any) {
      alert(e.message || '新建文件失败')
    }
  }

  const handleUpload = async (file: File) => {
    if (!file) return
    if (!/\.(md|txt)$/i.test(file.name)) { alert('仅支持 .md / .txt'); return }
    const text = await file.text()
    const target = (currentPath.endsWith('/') ? currentPath.slice(0, -1) : currentPath) + '/' + file.name
    try {
      await api.writeTree(target, { content: text, mimeType: /\.md$/i.test(file.name) ? 'text/markdown' : 'text/plain' })
      refresh()
    } catch (e: any) {
      alert(e.message || '上传失败')
    }
  }

  const handleDelete = async (paths: string[]) => {
    if (!confirm(`确定删除以下 ${paths.length} 个条目？\n` + paths.join('\n'))) return
    for (const p of paths) {
      try { await api.deleteTree(p) } catch (e: any) {
        alert(`删除失败：${p}\n${e.message || e}`)
      }
    }
    refresh()
  }

  const [renaming, setRenaming] = useState<string | null>(null)
  const [renameTo, setRenameTo] = useState('')
  const beginRename = (p: string) => { setRenaming(p); setRenameTo(p) }
  const commitRename = async () => {
    if (!renaming || !renameTo || renaming === renameTo) { setRenaming(null); return }
    try {
      const file = await api.getTree(renaming)
      if (file.is_dir) {
        // v1: 仅允许空目录（简化：先尝试删除/失败再提示）；这里直接尝试创建新目录后删除旧目录
        await api.writeTree(renameTo, { content: '', isDir: true })
        await api.deleteTree(renaming)
      } else {
        await api.writeTree(renameTo, { content: file.content || '', mimeType: file.mime_type || 'text/plain' })
        await api.deleteTree(renaming)
      }
      setRenaming(null)
      refresh()
    } catch (e: any) {
      alert(`重命名失败：${e.message || e}`)
    }
  }

  const filteredItems = useMemo(
    () => items.filter((item) => namespaceFilter === 'all' || topNamespace(item.path) === namespaceFilter),
    [items, namespaceFilter],
  )
  const listTitle = searchMode ? '搜索结果' : currentPath === '/' ? '根目录' : currentPath.split('/').pop() || currentPath

  return (
    <div className="page">
      <div className="page-header page-header-stack">
        <div>
          <h2>文件管理器</h2>
          <p className="page-subtitle">像 OneDrive 一样浏览、筛选、搜索和预览 Hub 文件。</p>
        </div>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}
      <div className="alert files-browser-note" style={{ background: '#fffbeb', border: '1px solid #fde68a', marginBottom: 12 }}>
        部分系统路径为只读（例如内置技能/设备配置）。如遇“path is read-only”，请改在 <code>/notes/</code>、<code>/projects/</code> 或你的 <code>/skills/</code> 子目录。
      </div>

      <div className="files-browser-toolbar">
        <Breadcrumbs path={currentPath} onNavigate={onNavigatePath} />
        <div className="files-browser-spacer" />
        <div className="files-browser-search">
          <input
            placeholder="搜索（关键词 / namespace / kind）"
            value={searchInput}
            onChange={e => setSearchInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') runSearch()
              if (e.key === 'Escape') clearSearch()
            }}
          />
        </div>
        <button className="btn" onClick={runSearch}>搜索</button>
        {searchMode && <button className="btn" onClick={clearSearch}>清除</button>}
        <button className="btn" onClick={() => setCreatingDir(v => !v)}>新建文件夹</button>
        <button className="btn" onClick={() => setCreatingFile(v => !v)}>新建文件</button>
        <input ref={fileInputRef} type="file" accept=".md,.txt" style={{ display: 'none' }} onChange={(e) => {
          const f = e.target.files?.[0]
          if (f) handleUpload(f)
          e.currentTarget.value = ''
        }} />
        <button className="btn" onClick={() => fileInputRef.current?.click()}>上传文本</button>
      </div>

      <div className="files-browser-chip-row" role="tablist" aria-label="命名空间筛选">
        {NAMESPACE_CHIPS.map((chip) => (
          <button
            key={chip.key}
            type="button"
            className={`files-browser-chip${namespaceFilter === chip.key ? ' is-active' : ''}`}
            onClick={() => setNamespaceFilter(chip.key)}
          >
            {chip.label}
          </button>
        ))}
      </div>

      {creatingDir && (
        <div className="card" style={{ marginBottom: 12 }}>
          <div className="form-row">
            <div className="form-group">
              <label>文件夹名称</label>
              <input value={newDirName} onChange={e => setNewDirName(e.target.value)} />
            </div>
            <div className="form-group">
              <label>&nbsp;</label>
              <div className="form-actions">
                <button className="btn" onClick={() => setCreatingDir(false)}>取消</button>
                <button className="btn btn-primary" onClick={handleNewDir}>创建</button>
              </div>
            </div>
          </div>
        </div>
      )}

      {creatingFile && (
        <div className="card" style={{ marginBottom: 12 }}>
          <div className="form-row">
            <div className="form-group">
              <label>文件名称</label>
              <input value={newFileName} onChange={e => setNewFileName(e.target.value)} placeholder="示例：readme.md" />
            </div>
            <div className="form-group">
              <label>&nbsp;</label>
              <div className="form-actions">
                <button className="btn" onClick={() => setCreatingFile(false)}>取消</button>
                <button className="btn btn-primary" onClick={handleNewFile}>创建</button>
              </div>
            </div>
          </div>
        </div>
      )}

      <div className="files-browser-panel">
        <div className="files-browser-panel-head">
          <div>
            <strong>{listTitle}</strong>
            <span className="files-browser-panel-path">{searchMode ? `关键词：${appliedSearch}` : currentPath}</span>
          </div>
        </div>
        {loading ? (
          <div className="page-loading">加载中...</div>
        ) : (
          <div className="files-table">
            <div className="files-thead">
              <div className="files-th files-col-name" onClick={() => toggleSort('name')}>名称</div>
              <div className="files-th files-col-size" onClick={() => toggleSort('size')}>大小</div>
              <div className="files-th files-col-kind">命名空间</div>
              <div className="files-th files-col-time" onClick={() => toggleSort('updated_at')}>最近修改</div>
            </div>
            <div className="files-tbody">
              {filteredItems.length === 0 ? (
                <div className="files-empty">{searchMode ? '无搜索结果' : '该目录暂无内容'}</div>
              ) : filteredItems.map((it) => (
                <div
                  key={it.path}
                  className="files-tr"
                  role="button"
                  tabIndex={0}
                  onClick={() => handleOpen(it)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      handleOpen(it)
                    }
                  }}
                >
                  <div className="files-td files-col-name">
                    <span className={"file-icon " + (it.is_dir ? 'fi-folder' : (/\.md$/i.test(it.name) ? 'fi-md' : 'fi-file'))} />
                    <span className="file-name">{it.name}</span>
                    <span className="files-browser-row-actions">
                      <button className="btn-text" onClick={(e) => { e.stopPropagation(); beginRename(it.path) }}>重命名</button>
                      <button className="btn-text" onClick={(e) => { e.stopPropagation(); handleDelete([it.path]) }}>删除</button>
                    </span>
                  </div>
                  <div className="files-td files-col-size">{it.is_dir ? '-' : formatBytes(it.size)}</div>
                  <div className="files-td files-col-kind">
                    <span className="files-browser-badge">{fileNamespaceLabel(it.path)}</span>
                  </div>
                  <div className="files-td files-col-time">{new Date(it.updated_at || it.created_at || 0).toLocaleString('zh-CN')}</div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      {renaming && (
        <div className="card" style={{ marginTop: 12 }}>
          <div className="form-row">
            <div className="form-group" style={{ gridColumn: '1 / span 2' }}>
              <label>新路径</label>
              <input value={renameTo} onChange={e => setRenameTo(e.target.value)} />
            </div>
            <div className="form-group">
              <label>&nbsp;</label>
              <div className="form-actions">
                <button className="btn" onClick={() => setRenaming(null)}>取消</button>
                <button className="btn btn-primary" onClick={commitRename}>保存</button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
