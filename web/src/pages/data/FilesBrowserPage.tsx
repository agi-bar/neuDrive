import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { api, type FileNode } from '../../api'
import MaterialsSectionToolbar from '../../components/MaterialsSectionToolbar'
import FileMaterialsTile from '../../components/FileMaterialsTile'
import ResourceActionMenu from '../../components/ResourceActionMenu'
import ResourceConfirmDialog from '../../components/ResourceConfirmDialog'
import SourceFilterBar from '../../components/SourceFilterBar'
import useResourceCardMenu from '../../hooks/useResourceCardMenu'
import useTreeDeleteDialog from '../../hooks/useTreeDeleteDialog'
import { useI18n } from '../../i18n'
import {
  buildFileTileModel,
  buildSourceFilterOptions,
  dataFileEditorRoute,
  fileNodeSource,
  isTextLikeFile,
  matchesSourceFilter,
  sourceLabel,
} from './DataShared'

type SortKey = 'name' | 'updated_at'
type SortDir = 'asc' | 'desc'

function useQuery() {
  const { search } = useLocation()
  return useMemo(() => new URLSearchParams(search), [search])
}

function Breadcrumbs({
  path,
  onNavigate,
  rootLabel,
  ariaLabel,
}: {
  path: string
  onNavigate: (p: string) => void
  rootLabel: string
  ariaLabel: string
}) {
  const parts = path.replace(/^\/+/, '').split('/').filter(Boolean)
  const segments = ['/', ...parts.map((_, i) => '/' + parts.slice(0, i + 1).join('/'))]
  const labels = [rootLabel, ...parts]
  return (
    <nav aria-label={ariaLabel} className="breadcrumbs" style={{ marginBottom: 8 }}>
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
  const { locale, tx } = useI18n()
  const params = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const query = useQuery()
  const wildcard = params['*'] || ''
  const currentPath = useMemo(() => (wildcard ? '/' + decodeURIComponent(wildcard) : '/'), [wildcard])
  const [currentNode, setCurrentNode] = useState<FileNode | null>(null)
  const [items, setItems] = useState<FileNode[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [creatingDir, setCreatingDir] = useState(false)
  const [newDirName, setNewDirName] = useState(tx('新建文件夹', 'New folder'))
  const [creatingFile, setCreatingFile] = useState(false)
  const [newFileName, setNewFileName] = useState(tx('新建文档.md', 'new-document.md'))
  const [searchInput, setSearchInput] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  const fileInputRef = useRef<HTMLInputElement>(null)
  const { activeMenuId, closeMenu, isMenuOpen, toggleMenu } = useResourceCardMenu()

  const sortKey = (query.get('sort') as SortKey) || 'updated_at'
  const sortDir = (query.get('dir') as SortDir) || 'desc'
  const sourceFilter = query.get('source') || 'all'
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
          source: result.source,
          is_dir: false,
          kind: result.kind,
          content: result.content,
          updated_at: result.updated_at,
        }))
        setCurrentNode(null)
        setItems(sortNodes(mapped, sortKey, sortDir))
      } else {
        const root = await api.getTree(currentPath)
        setCurrentNode(root)
        setItems(sortNodes(root.children || [], sortKey, sortDir))
      }
      closeMenu()
      setSelected(new Set())
    } catch (err: any) {
      setError(err.message || tx('加载失败', 'Failed to load files'))
    } finally {
      setLoading(false)
    }
  }, [appliedSearch, closeMenu, currentPath, searchMode, sortDir, sortKey, tx])

  const {
    closeDialog: closeDeleteDialog,
    confirmDelete,
    dialog: deleteDialog,
    requestDelete,
    submitting: deleteSubmitting,
  } = useTreeDeleteDialog({ tx, onDeleted: refresh })

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

  const changeSourceFilter = (value: string) => {
    const params = new URLSearchParams(location.search)
    if (!value || value === 'all') params.delete('source')
    else params.set('source', value)
    navigate({ search: params.toString() })
  }

  const onNavigatePath = (pathValue: string) => {
    const normalized = pathValue.replace(/^\/+/, '')
    navigate({
      pathname: normalized ? `/data/files/${encodeURIComponent(normalized)}` : '/data/files',
      search: location.search,
    })
  }

  const clearSearch = () => {
    setSearchInput('')
  }

  const isEditableNode = useCallback((item: FileNode) => {
    if (item.is_dir) return false
    return isTextLikeFile(item.name, item.mime_type)
  }, [])

  const openNode = useCallback((item: FileNode) => {
    closeMenu()
    if (item.is_dir) {
      onNavigatePath(item.path)
      return
    }
    if (!isEditableNode(item)) return
    navigate(dataFileEditorRoute(item.path))
  }, [closeMenu, isEditableNode, navigate])

  const handleDownloadZip = useCallback(async (pathValue: string) => {
    closeMenu()
    try {
      await api.downloadTreeZip(pathValue)
    } catch (err: any) {
      setError(err.message || tx('下载 ZIP 失败', 'Failed to download ZIP'))
    }
  }, [closeMenu, tx])

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
      await api.writeTree(target, { content: '', isDir: true, metadata: { source: 'manual' } })
      setCreatingDir(false)
      setNewDirName(tx('新建文件夹', 'New folder'))
      await refresh()
    } catch (err: any) {
      alert(err.message || tx('新建文件夹失败', 'Failed to create folder'))
    }
  }

  const handleNewFile = async () => {
    if (!newFileName.trim()) return
    const target = `${currentPath.endsWith('/') ? currentPath.slice(0, -1) : currentPath}/${newFileName.trim()}`
    try {
      await api.writeTree(target, {
        content: `# ${tx('新文档', 'New document')}\n`,
        mimeType: target.toLowerCase().endsWith('.md') ? 'text/markdown' : 'text/plain',
        metadata: { source: 'manual' },
      })
      setCreatingFile(false)
      setNewFileName(tx('新建文档.md', 'new-document.md'))
      await refresh()
    } catch (err: any) {
      alert(err.message || tx('新建文件失败', 'Failed to create file'))
    }
  }

  const handleUpload = async (file: File) => {
    if (!/\.(md|txt)$/i.test(file.name)) {
      alert(tx('仅支持 .md / .txt', 'Only .md and .txt files are supported'))
      return
    }
    const text = await file.text()
    const target = `${currentPath.endsWith('/') ? currentPath.slice(0, -1) : currentPath}/${file.name}`
    try {
      await api.writeTree(target, {
        content: text,
        mimeType: /\.md$/i.test(file.name) ? 'text/markdown' : 'text/plain',
        metadata: { source: 'upload' },
      })
      await refresh()
    } catch (err: any) {
      alert(err.message || tx('上传失败', 'Upload failed'))
    }
  }

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (deleteDialog || activeMenuId) return
      if (event.key === 'Escape') setSelected(new Set())
      if (event.key === 'Delete' && selected.size > 0) {
        event.preventDefault()
        void requestDelete(Array.from(selected))
      }
      if (event.key === 'Enter' && selected.size === 1) {
        const pathValue = Array.from(selected)[0]
        const item = items.find((entry) => entry.path === pathValue)
        if (item) openNode(item)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [activeMenuId, deleteDialog, items, openNode, requestDelete, selected])

  const isSelected = (pathValue: string) => selected.has(pathValue)
  const currentLabel = currentPath === '/' ? tx('根目录', 'Root') : currentPath.split('/').filter(Boolean).slice(-1)[0] || tx('根目录', 'Root')
  const currentBundleContext = !searchMode ? currentNode?.bundle_context : undefined
  const currentBundleMode = !searchMode && Boolean(currentBundleContext)
  const filteredItems = useMemo(
    () => items.filter((item) => matchesSourceFilter(fileNodeSource(item), sourceFilter)),
    [items, sourceFilter],
  )
  const sourceOptions = useMemo(
    () => buildSourceFilterOptions(items, fileNodeSource, locale),
    [items, locale],
  )

  if (loading) return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>

  return (
    <div className="page materials-page">
      <section className="materials-hero">
        <div className="materials-hero-copy">
          <Breadcrumbs
            path={currentPath}
            onNavigate={onNavigatePath}
            rootLabel={tx('根目录', 'Root')}
            ariaLabel={tx('面包屑', 'Breadcrumbs')}
          />
          <div className="materials-kicker">neuDrive Data</div>
          <h2 className="materials-title">{tx('文件管理器', 'File Browser')}</h2>
          <p className="materials-subtitle">{tx('参考你给的 Materials 卡片墙，把当前目录里的文件和文件夹统一显示成卡片。点文件名直接打开，双击目录继续下钻。', 'Using the Materials card wall style, this page renders files and folders as cards. Click a file name to open it, or double-click a folder to drill in.')}</p>
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
        {tx('部分系统路径为只读（例如内置技能或受保护目录）。如遇“path is read-only”，请改在 ', 'Some system paths are read-only (for example built-in skills or protected directories). If you see "path is read-only", work under ')}
        <code>/notes/</code>
        {tx('、', ', ')}
        <code>/projects/</code>
        {tx(' 或你的 ', ', or your ')}
        <code>/skills/</code>
        {tx(' 子目录。', ' subdirectory instead.')}
      </div>

      {creatingDir && (
        <div className="materials-panel" style={{ marginBottom: 12 }}>
          <div className="form-row">
            <div className="form-group">
              <label>{tx('文件夹名称', 'Folder name')}</label>
              <input value={newDirName} onChange={(event) => setNewDirName(event.target.value)} />
            </div>
            <div className="form-group">
              <label>&nbsp;</label>
              <div className="form-actions">
                <button className="btn" onClick={() => setCreatingDir(false)}>{tx('取消', 'Cancel')}</button>
                <button className="btn btn-primary" onClick={() => void handleNewDir()}>{tx('创建', 'Create')}</button>
              </div>
            </div>
          </div>
        </div>
      )}

      {creatingFile && (
        <div className="materials-panel" style={{ marginBottom: 12 }}>
          <div className="form-row">
            <div className="form-group">
              <label>{tx('文件名称', 'File name')}</label>
              <input value={newFileName} onChange={(event) => setNewFileName(event.target.value)} placeholder={tx('示例：readme.md', 'Example: readme.md')} />
            </div>
            <div className="form-group">
              <label>&nbsp;</label>
              <div className="form-actions">
                <button className="btn" onClick={() => setCreatingFile(false)}>{tx('取消', 'Cancel')}</button>
                <button className="btn btn-primary" onClick={() => void handleNewFile()}>{tx('创建', 'Create')}</button>
              </div>
            </div>
          </div>
        </div>
      )}

      <section className="materials-section">
        <div className="materials-section-head">
          <div>
            <h3 className="materials-section-title">{searchMode ? tx('搜索结果', 'Search results') : currentLabel}</h3>
            <p className="materials-section-copy">
              {searchMode ? tx('搜索命中的路径会显示在卡片里。', 'Matched paths are shown on the cards.') : tx('单击卡片选中，双击进入目录；点文件名会直接执行打开动作。', 'Click once to select a card, double-click to enter a folder, or click the file name to open it.')}
            </p>
          </div>
          <MaterialsSectionToolbar
            count={filteredItems.length}
            sortKey={sortKey}
            sortOptions={[
              { value: 'updated_at', label: tx('按时间', 'By time') },
              { value: 'name', label: tx('按名称', 'By name') },
            ]}
            sortDir={sortDir}
            onSortKeyChange={(value) => changeSortKey(value as SortKey)}
            onSortDirToggle={toggleSortDir}
          >
            <button className="btn btn-sm materials-toolbar-control" onClick={() => setCreatingDir((value) => !value)}>
              {creatingDir ? tx('取消文件夹', 'Close folder form') : tx('新建文件夹', 'New folder')}
            </button>
            <button className="btn btn-sm materials-toolbar-control" onClick={() => setCreatingFile((value) => !value)}>
              {creatingFile ? tx('取消文件', 'Close file form') : tx('新建文件', 'New file')}
            </button>
            <button className="btn btn-sm materials-toolbar-control" onClick={() => fileInputRef.current?.click()}>{tx('上传文本', 'Upload text')}</button>
            <button className="btn btn-sm materials-toolbar-control is-danger" disabled={selected.size === 0} onClick={() => void requestDelete(Array.from(selected))}>{tx('删除', 'Delete')}</button>
          </MaterialsSectionToolbar>
        </div>

        {(sourceOptions.length > 1 || sourceFilter !== 'all') && (
          <SourceFilterBar options={sourceOptions} value={sourceFilter} onChange={changeSourceFilter} />
        )}

        {filteredItems.length === 0 ? (
          <div className="materials-panel files-empty">{searchMode ? tx('无搜索结果', 'No search results') : tx('该目录暂无内容', 'This directory is empty')}</div>
        ) : (
          <div className="materials-grid">
            {filteredItems.map((item) => {
              const tile = searchMode
                ? buildFileTileModel({
                    node: item,
                    variant: 'search',
                    currentLabel,
                    locale,
                  })
                : currentBundleMode
                  ? buildFileTileModel({
                      node: item,
                      variant: 'bundle-entry',
                      bundleLabel: currentLabel,
                      locale,
                    })
                  : buildFileTileModel({
                      node: item,
                      variant: 'browser',
                      currentLabel,
                      locale,
                    })
              return (
                <FileMaterialsTile
                  key={item.path}
                  node={tile.node}
                  subtitle={tile.subtitle}
                  description={tile.description}
                  path={tile.path}
                  extraPills={tile.source ? <span className="materials-tile-pill materials-source-pill">{sourceLabel(tile.source, locale)}</span> : undefined}
                  footerStart={tile.footerStart}
                  footerEnd={tile.footerEnd}
                  selected={isSelected(item.path)}
                  menuOpen={isMenuOpen(item.path)}
                  menuButtonAriaLabel={tx(`打开 ${item.name} 的工具菜单`, `Open tools menu for ${item.name}`)}
                  menuPanel={(
                    <ResourceActionMenu
                      items={[
                        ...((item.is_dir || isEditableNode(item))
                          ? [{
                              key: 'open',
                              label: item.is_dir ? tx('进入目录', 'Open folder') : tx('打开文件', 'Open file'),
                              onSelect: () => {
                                closeMenu()
                                openNode(item)
                              },
                            }]
                          : []),
                        {
                          key: 'download',
                          label: tx('下载 ZIP', 'Download ZIP'),
                          onSelect: () => {
                            void handleDownloadZip(item.path)
                          },
                        },
                        {
                          key: 'select',
                          label: isSelected(item.path) ? tx('取消选中', 'Unselect') : tx('加入选择', 'Select'),
                          onSelect: () => {
                            closeMenu()
                            handleSelect(item.path, true)
                          },
                        },
                        {
                          key: 'delete',
                          label: tx('删除', 'Delete'),
                          tone: 'danger' as const,
                          onSelect: () => {
                            closeMenu()
                            void requestDelete([item.path])
                          },
                        },
                      ]}
                    />
                  )}
                  onMenuToggle={() => toggleMenu(item.path)}
                  onSelect={({ multi }) => handleSelect(item.path, multi)}
                  onOpen={() => openNode(item)}
                />
              )
            })}
          </div>
        )}
      </section>

      <ResourceConfirmDialog
        open={Boolean(deleteDialog)}
        kicker={tx('删除确认', 'Delete confirmation')}
        title={deleteDialog?.nonEmptyDirectories.length ? tx('这些目录不是空的', 'These folders are not empty') : tx('确认删除选中条目', 'Confirm deletion')}
        description={deleteDialog?.nonEmptyDirectories.length
          ? tx('确认后会递归删除其中所有可写文件和文件夹。只读内容不会被删除，可能会继续保留。', 'Continuing will recursively delete all writable files and folders inside. Read-only content will not be deleted and may remain in place.')
          : tx('这个操作会删除选中的文件或文件夹，且不可撤销。', 'This will delete the selected files or folders and cannot be undone.')}
        cancelLabel={tx('取消', 'Cancel')}
        confirmLabel={deleteSubmitting ? tx('删除中...', 'Deleting...') : tx('确认删除', 'Delete')}
        tone="danger"
        submitting={deleteSubmitting}
        onCancel={closeDeleteDialog}
        onConfirm={() => void confirmDelete()}
      />
    </div>
  )
}
