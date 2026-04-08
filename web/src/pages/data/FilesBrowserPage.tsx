import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { api, type FileNode, type SkillSummary } from '../../api'
import MaterialsSectionToolbar from '../../components/MaterialsSectionToolbar'
import FileMaterialsTile from '../../components/FileMaterialsTile'
import { buildFileTileModel, buildSkillBundleTileModel, buildSkillSummaryLookup, dataFileEditorRoute, skillSummaryForPath } from './DataShared'

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
  const [skillLookup, setSkillLookup] = useState<Record<string, SkillSummary>>({})
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
        setSkillLookup({})
      } else {
        const [root, skills] = await Promise.all([
          api.getTree(currentPath),
          currentPath === '/skills' ? api.getSkills() : Promise.resolve<SkillSummary[]>([]),
        ])
        if (skills.length > 0) {
          setSkillLookup(buildSkillSummaryLookup(skills))
        } else {
          setSkillLookup({})
        }
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

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setAppliedSearch(searchInput.trim())
    }, 180)
    return () => window.clearTimeout(timer)
  }, [searchInput])

  const updateSort = (key: SortKey, dir: SortDir) => {
    const params = new URLSearchParams(location.search)
    params.set('sort', key)
    params.set('dir', dir)
    navigate({ search: params.toString() })
  }

  const toggleSortDir = () => {
    updateSort(sortKey, sortDir === 'asc' ? 'desc' : 'asc')
  }

  const changeSortKey = (key: SortKey) => {
    updateSort(key, sortDir)
  }

  const onNavigatePath = (pathValue: string) => {
    const normalized = pathValue.replace(/^\/+/, '')
    navigate(normalized ? `/data/files/${encodeURIComponent(normalized)}` : '/data/files')
  }

  const clearSearch = () => {
    setSearchInput('')
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
    navigate(dataFileEditorRoute(item.path))
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
  const currentLabel = currentPath === '/' ? '根目录' : currentPath.split('/').filter(Boolean).slice(-1)[0] || '根目录'

  if (loading) return <div className="page-loading">加载中...</div>

  return (
    <div className="page materials-page">
      <section className="materials-hero">
        <div className="materials-hero-copy">
          <Breadcrumbs path={currentPath} onNavigate={onNavigatePath} />
          <div className="materials-kicker">Agent Hub Data</div>
          <h2 className="materials-title">文件管理器</h2>
          <p className="materials-subtitle">参考你给的 Materials 卡片墙，把当前目录里的文件和文件夹统一显示成卡片。点文件名直接打开，双击目录继续下钻。</p>
        </div>
        <div className="materials-actions">
          <input
            className="files-browser-hero-search"
            placeholder="Search"
            value={searchInput}
            onChange={(event) => setSearchInput(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Escape') clearSearch()
            }}
          />
        </div>
      </section>

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

      {error && <div className="alert alert-warn">{error}</div>}
      <div className="materials-note">
        部分系统路径为只读（例如内置技能或受保护目录）。如遇“path is read-only”，请改在 <code>/notes/</code>、<code>/projects/</code> 或你的 <code>/skills/</code> 子目录。
      </div>

      {creatingDir && (
        <div className="materials-panel" style={{ marginBottom: 12 }}>
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
        <div className="materials-panel" style={{ marginBottom: 12 }}>
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

      <section className="materials-section">
        <div className="materials-section-head">
          <div>
            <h3 className="materials-section-title">{searchMode ? '搜索结果' : currentLabel}</h3>
            <p className="materials-section-copy">
              {searchMode ? '搜索命中的路径会显示在卡片里。' : '单击卡片选中，双击进入目录；点文件名会直接执行打开动作。'}
            </p>
          </div>
          <MaterialsSectionToolbar
            count={items.length}
            sortKey={sortKey}
            sortOptions={[
              { value: 'updated_at', label: '按时间' },
              { value: 'name', label: '按名称' },
            ]}
            sortDir={sortDir}
            onSortKeyChange={(value) => changeSortKey(value as SortKey)}
            onSortDirToggle={toggleSortDir}
          >
            <button className="btn btn-sm materials-toolbar-control" onClick={() => setCreatingDir((value) => !value)}>
              {creatingDir ? '取消文件夹' : '新建文件夹'}
            </button>
            <button className="btn btn-sm materials-toolbar-control" onClick={() => setCreatingFile((value) => !value)}>
              {creatingFile ? '取消文件' : '新建文件'}
            </button>
            <button className="btn btn-sm materials-toolbar-control" onClick={() => fileInputRef.current?.click()}>上传文本</button>
            <button className="btn btn-sm materials-toolbar-control is-danger" disabled={selected.size === 0} onClick={() => void handleDelete(Array.from(selected))}>删除</button>
          </MaterialsSectionToolbar>
        </div>

        {items.length === 0 ? (
          <div className="materials-panel files-empty">{searchMode ? '无搜索结果' : '该目录暂无内容'}</div>
        ) : (
          <div className="materials-grid">
            {items.map((item) => {
              const skillSummary = skillSummaryForPath(item.path, skillLookup)
              const tile = searchMode
                ? buildFileTileModel({
                    node: item,
                    variant: 'search',
                    currentLabel,
                    skillLookup,
                  })
                : currentPath === '/skills' && item.is_dir && skillSummary
                  ? buildSkillBundleTileModel(skillSummary)
                  : currentPath.startsWith('/skills/') && currentPath !== '/skills'
                    ? buildFileTileModel({
                        node: item,
                        variant: 'skill-bundle-entry',
                        bundleLabel: currentLabel,
                      })
                    : buildFileTileModel({
                        node: item,
                        variant: 'browser',
                        currentLabel,
                        skillLookup,
                      })
              return (
                <FileMaterialsTile
                  key={item.path}
                  node={tile.node}
                  subtitle={tile.subtitle}
                  description={tile.description}
                  path={tile.path}
                  footerStart={tile.footerStart}
                  footerEnd={tile.footerEnd}
                  selected={isSelected(item.path)}
                  onSelect={({ multi }) => handleSelect(item.path, multi)}
                  onOpen={() => openNode(item)}
                />
              )
            })}
          </div>
        )}
      </section>
    </div>
  )
}
