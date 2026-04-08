import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { api, type FileNode } from '../../api'

type SortKey = 'name' | 'updated_at'
type SortDir = 'asc' | 'desc'

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

function sortNodes(nodes: FileNode[], key: SortKey, dir: SortDir) {
  const mul = dir === 'asc' ? 1 : -1
  return [...nodes].sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1
    if (key === 'name') return a.name.localeCompare(b.name) * mul
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
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [creatingDir, setCreatingDir] = useState(false)
  const [newDirName, setNewDirName] = useState('新建文件夹')
  const [creatingFile, setCreatingFile] = useState(false)
  const [newFileName, setNewFileName] = useState('新建文档.md')
  const [searchInput, setSearchInput] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
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
        const mapped: FileNode[] = results.map((result) => ({
          path: result.path,
          name: result.name,
          is_dir: false,
          kind: result.kind,
          content: result.content,
          updated_at: result.updated_at,
        }))
        setItems(sortNodes(mapped, sortKey, sortDir))
      } else {
        const root = await api.getTree(currentPath)
        setItems(sortNodes(root.children || [], sortKey, sortDir))
      }
      setSelected(new Set())
    } catch (err: any) {
      setError(err.message || '加载失败')
    } finally {
      setLoading(false)
    }
  }, [appliedSearch, currentPath, searchMode, sortDir, sortKey])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const toggleSort = (key: SortKey) => {
    const dir: SortDir = sortKey === key ? (sortDir === 'asc' ? 'desc' : 'asc') : 'asc'
    const params = new URLSearchParams(location.search)
    params.set('sort', key)
    params.set('dir', dir)
    navigate({ search: params.toString() })
  }

  const onNavigatePath = (pathValue: string) => {
    const normalized = pathValue.replace(/^\/+/, '')
    navigate(normalized ? `/data/files/${encodeURIComponent(normalized)}` : '/data/files')
  }

  const runSearch = () => {
    setAppliedSearch(searchInput.trim())
  }

  const clearSearch = () => {
    setSearchInput('')
    setAppliedSearch('')
  }

  const isEditableNode = useCallback((item: FileNode) => {
    if (item.is_dir) return false
    const mimeType = item.mime_type || ''
    return /\.md$/i.test(item.name) || mimeType.startsWith('text/')
  }, [])

  const openNode = useCallback((item: FileNode) => {
    if (item.is_dir) {
      onNavigatePath(item.path)
      return
    }
    if (!isEditableNode(item)) return
    navigate(`/data/files/edit/${encodeURIComponent(item.path.replace(/^\/+/, ''))}`)
  }, [isEditableNode, navigate])

  const handleSelect = (pathValue: string, multi = false) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (!multi) next.clear()
      if (next.has(pathValue)) next.delete(pathValue)
      else next.add(pathValue)
      return next
    })
  }

  const handleNewDir = async () => {
    if (!newDirName.trim()) return
    const target = `${currentPath.endsWith('/') ? currentPath.slice(0, -1) : currentPath}/${newDirName.trim()}`
    try {
      await api.writeTree(target, { content: '', isDir: true })
      setCreatingDir(false)
      setNewDirName('新建文件夹')
      await refresh()
    } catch (err: any) {
      alert(err.message || '新建文件夹失败')
    }
  }

  const handleNewFile = async () => {
    if (!newFileName.trim()) return
    const target = `${currentPath.endsWith('/') ? currentPath.slice(0, -1) : currentPath}/${newFileName.trim()}`
    try {
      await api.writeTree(target, {
        content: '# 新文档\n',
        mimeType: target.toLowerCase().endsWith('.md') ? 'text/markdown' : 'text/plain',
      })
      setCreatingFile(false)
      setNewFileName('新建文档.md')
      await refresh()
    } catch (err: any) {
      alert(err.message || '新建文件失败')
    }
  }

  const handleUpload = async (file: File) => {
    if (!/\.(md|txt)$/i.test(file.name)) {
      alert('仅支持 .md / .txt')
      return
    }
    const text = await file.text()
    const target = `${currentPath.endsWith('/') ? currentPath.slice(0, -1) : currentPath}/${file.name}`
    try {
      await api.writeTree(target, {
        content: text,
        mimeType: /\.md$/i.test(file.name) ? 'text/markdown' : 'text/plain',
      })
      await refresh()
    } catch (err: any) {
      alert(err.message || '上传失败')
    }
  }

  const handleDelete = async (paths: string[]) => {
    if (!confirm(`确定删除以下 ${paths.length} 个条目？\n${paths.join('\n')}`)) return
    for (const pathValue of paths) {
      try {
        await api.deleteTree(pathValue)
      } catch (err: any) {
        alert(`删除失败：${pathValue}\n${err.message || err}`)
      }
    }
    await refresh()
  }

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setSelected(new Set())
      if (event.key === 'Delete' && selected.size > 0) {
        event.preventDefault()
        void handleDelete(Array.from(selected))
      }
      if (event.key === 'Enter' && selected.size === 1) {
        const pathValue = Array.from(selected)[0]
        const item = items.find((entry) => entry.path === pathValue)
        if (item) openNode(item)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [items, openNode, selected])

  const isSelected = (pathValue: string) => selected.has(pathValue)

  if (loading) return <div className="page-loading">加载中...</div>

  return (
    <div className="page">
      <div className="page-header page-header-stack">
        <div>
          <h2>文件管理器</h2>
          <Breadcrumbs path={currentPath} onNavigate={onNavigatePath} />
        </div>
        <div className="page-actions" style={{ gap: 6 }}>
          <input
            placeholder="搜索文件（回车执行）"
            value={searchInput}
            onChange={(event) => setSearchInput(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') runSearch()
              if (event.key === 'Escape') clearSearch()
            }}
          />
          <button className="btn" onClick={runSearch}>搜索</button>
          {searchMode && <button className="btn" onClick={clearSearch}>清除</button>}
          <button className="btn" onClick={() => setCreatingDir((value) => !value)}>新建文件夹</button>
          <button className="btn" onClick={() => setCreatingFile((value) => !value)}>新建文件</button>
          <input
            ref={fileInputRef}
            type="file"
            accept=".md,.txt"
            style={{ display: 'none' }}
            onChange={(event) => {
              const file = event.target.files?.[0]
              if (file) void handleUpload(file)
              event.currentTarget.value = ''
            }}
          />
          <button className="btn" onClick={() => fileInputRef.current?.click()}>上传文本</button>
          <button className="btn btn-danger" disabled={selected.size === 0} onClick={() => void handleDelete(Array.from(selected))}>删除</button>
        </div>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}
      <div className="alert" style={{ background: '#fffbeb', border: '1px solid #fde68a', marginBottom: 12 }}>
        部分系统路径为只读（例如内置技能/设备配置）。如遇“path is read-only”，请改在 <code>/notes/</code>、<code>/projects/</code> 或你的 <code>/skills/</code> 子目录。
      </div>

      {creatingDir && (
        <div className="card" style={{ marginBottom: 12 }}>
          <div className="form-row">
            <div className="form-group">
              <label>文件夹名称</label>
              <input value={newDirName} onChange={(event) => setNewDirName(event.target.value)} />
            </div>
            <div className="form-group">
              <label>&nbsp;</label>
              <div className="form-actions">
                <button className="btn" onClick={() => setCreatingDir(false)}>取消</button>
                <button className="btn btn-primary" onClick={() => void handleNewDir()}>创建</button>
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
              <input value={newFileName} onChange={(event) => setNewFileName(event.target.value)} placeholder="示例：readme.md" />
            </div>
            <div className="form-group">
              <label>&nbsp;</label>
              <div className="form-actions">
                <button className="btn" onClick={() => setCreatingFile(false)}>取消</button>
                <button className="btn btn-primary" onClick={() => void handleNewFile()}>创建</button>
              </div>
            </div>
          </div>
        </div>
      )}

      <div className="card">
        <div className="files-table">
          <div className="files-thead">
            <div className="files-th files-col-name" onClick={() => toggleSort('name')}>名称</div>
            <div className="files-th files-col-time" onClick={() => toggleSort('updated_at')}>更新时间</div>
          </div>
          <div className="files-tbody">
            {items.length === 0 ? (
              <div className="files-empty">{searchMode ? '无搜索结果' : '该目录暂无内容'}</div>
            ) : items.map((item) => (
              <div
                key={item.path}
                className={`files-tr${isSelected(item.path) ? ' is-selected' : ''}`}
                onClick={(event) => handleSelect(item.path, event.metaKey || event.ctrlKey || event.shiftKey)}
                onDoubleClick={() => openNode(item)}
              >
                <div className="files-td files-col-name">
                  <span className={`file-icon ${item.is_dir ? 'fi-folder' : /\.md$/i.test(item.name) ? 'fi-md' : 'fi-file'}`} />
                  <div className="file-name-stack">
                    <button
                      type="button"
                      className="btn-text file-name-button"
                      onClick={(event) => {
                        event.stopPropagation()
                        openNode(item)
                      }}
                    >
                      {item.name}
                    </button>
                    {searchMode && <div className="file-row-secondary">{item.path}</div>}
                  </div>
                </div>
                <div className="files-td files-col-time">{new Date(item.updated_at || item.created_at || 0).toLocaleString('zh-CN')}</div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
