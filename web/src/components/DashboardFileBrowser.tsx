import { useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, type DashboardStats, type FileNode } from '../api'
import { useI18n, type AppLocale } from '../i18n'
import { dataFileEditorRoute, formatDateTime, sourceLabel } from '../pages/data/DataShared'

interface DashboardFileBrowserProps {
  stats: DashboardStats
  initialSnapshotEntries?: FileNode[]
  onDataChange?: () => void
}

const rootPath = '/'
const typeOrder = ['Conversation', 'Memory', 'Project', 'Skill', 'Vault', 'Folder', 'File']

function normalizePath(path?: string | null) {
  if (!path || path === rootPath) return rootPath
  return `/${path.replace(/^\/+|\/+$/g, '')}`
}

function pathParts(path: string) {
  return normalizePath(path).split('/').filter(Boolean)
}

function isDirectSkillBundlePath(path: string) {
  const parts = pathParts(path)
  return parts.length === 2 && parts[0]?.toLowerCase() === 'skills'
}

function joinPath(parts: string[]) {
  return parts.length === 0 ? rootPath : `/${parts.join('/')}`
}

function parentPath(path: string) {
  return joinPath(pathParts(path).slice(0, -1))
}

function nodeType(node: FileNode) {
  const path = normalizePath(node.path).toLowerCase()
  if (node.bundle_context?.kind === 'conversation' || path.startsWith('/conversations/')) return 'Conversation'
  if (node.bundle_context?.kind === 'skill' || isDirectSkillBundlePath(path)) return 'Skill'
  if (node.bundle_context?.kind === 'project' || path.startsWith('/projects/')) return 'Project'
  if (path.startsWith('/memory/')) return 'Memory'
  if (path.startsWith('/vault/')) return 'Vault'
  if (node.is_dir) return 'Folder'
  return 'File'
}

function typeLabel(type: string, locale: AppLocale) {
  if (locale !== 'zh-CN') return type
  switch (type) {
    case 'Conversation': return '会话'
    case 'Memory': return '记忆'
    case 'Project': return '项目'
    case 'Skill': return '技能'
    case 'Vault': return 'Vault'
    case 'Folder': return '文件夹'
    case 'File': return '文件'
    default: return type
  }
}

function fileTypeLabel(node: FileNode, locale: AppLocale) {
  if (node.is_dir && !node.path.toLowerCase().startsWith('/memory/')) {
    return locale === 'zh-CN' ? '文件夹' : 'Folder'
  }
  return typeLabel(nodeType(node), locale)
}

function formatBytes(value?: number) {
  const bytes = Number(value || 0)
  if (!Number.isFinite(bytes) || bytes <= 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  const kib = bytes / 1024
  if (kib < 1024) return `${kib.toFixed(kib >= 10 ? 0 : 1)} KiB`
  const mib = kib / 1024
  return `${mib.toFixed(mib >= 10 ? 0 : 1)} MiB`
}

function entryPath(entry: FileNode, currentPath: string) {
  return normalizePath(entry.path || `${currentPath.replace(/\/+$/, '')}/${entry.name}`)
}

function sourceFor(entry: FileNode) {
  return String(entry.source || entry.metadata?.source || entry.bundle_context?.source || 'system')
}

function sortTreeEntries(entries: FileNode[]) {
  return [...entries].sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1
    return a.name.localeCompare(b.name)
  })
}

function sortFilteredEntries(entries: FileNode[]) {
  return [...entries].sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1
    const at = new Date(a.updated_at || a.created_at || 0).getTime()
    const bt = new Date(b.updated_at || b.created_at || 0).getTime()
    if (at !== bt) return bt - at
    return a.path.localeCompare(b.path)
  })
}

async function writeClipboardText(text: string) {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return
    }
  } catch {
    // Fall through to the textarea fallback.
  }
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', 'true')
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  textarea.style.top = '0'
  document.body.appendChild(textarea)
  textarea.select()
  document.execCommand('copy')
  document.body.removeChild(textarea)
}

export default function DashboardFileBrowser({ stats, initialSnapshotEntries = [], onDataChange }: DashboardFileBrowserProps) {
  const { locale, tx } = useI18n()
  const location = useLocation()
  const navigate = useNavigate()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [treeNode, setTreeNode] = useState<FileNode | null>(null)
  const [snapshotEntries, setSnapshotEntries] = useState<FileNode[]>(initialSnapshotEntries)
  const [treeLoading, setTreeLoading] = useState(true)
  const [snapshotLoading, setSnapshotLoading] = useState(initialSnapshotEntries.length === 0)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState('')
  const [openMenuPath, setOpenMenuPath] = useState('')
  const [copiedPath, setCopiedPath] = useState('')

  const params = useMemo(() => new URLSearchParams(location.search), [location.search])
  const currentPath = normalizePath(params.get('path') || rootPath)
  const query = params.get('q') || ''
  const typeFilter = params.get('type') || 'all'
  const sourceFilter = params.get('source') || 'all'
  const hasFilters = !!query.trim() || typeFilter !== 'all' || sourceFilter !== 'all'

  const writeParams = (patch: Record<string, string | null>, clearPath = false) => {
    const next = new URLSearchParams(location.search)
    next.delete('access')
    if (clearPath) next.delete('path')
    for (const [key, value] of Object.entries(patch)) {
      if (!value || value === 'all' || (key === 'path' && normalizePath(value) === rootPath)) {
        next.delete(key)
      } else {
        next.set(key, key === 'path' ? normalizePath(value) : value)
      }
    }
    const search = next.toString()
    navigate({ pathname: '/', search: search ? `?${search}` : '' }, { replace: true })
  }

  const browsePath = (nextPath: string) => {
    setOpenMenuPath('')
    const next = new URLSearchParams()
    const normalized = normalizePath(nextPath)
    if (normalized !== rootPath) next.set('path', normalized)
    navigate({ pathname: '/', search: next.toString() ? `?${next.toString()}` : '' }, { replace: true })
  }

  const loadSnapshot = async () => {
    setSnapshotLoading(true)
    try {
      const snapshot = await api.getTreeSnapshot('/')
      setSnapshotEntries(snapshot.entries || [])
    } catch (err: any) {
      setError(err?.message || tx('加载文件失败', 'Failed to load files'))
    } finally {
      setSnapshotLoading(false)
    }
  }

  const loadTree = async () => {
    setTreeLoading(true)
    try {
      setTreeNode(await api.getTree(currentPath))
    } catch (err: any) {
      setError(err?.message || tx('加载文件失败', 'Failed to load files'))
    } finally {
      setTreeLoading(false)
    }
  }

  const reloadData = async () => {
    setError('')
    await Promise.all([loadSnapshot(), loadTree()])
    onDataChange?.()
  }

  useEffect(() => {
    if (!params.has('access')) return
    params.delete('access')
    navigate({ pathname: '/', search: params.toString() ? `?${params.toString()}` : '' }, { replace: true })
  }, [location.search])

  useEffect(() => {
    setSnapshotEntries(initialSnapshotEntries)
    setSnapshotLoading(initialSnapshotEntries.length === 0)
  }, [initialSnapshotEntries])

  useEffect(() => {
    void loadSnapshot()
  }, [])

  useEffect(() => {
    void loadTree()
  }, [currentPath])

  useEffect(() => {
    setOpenMenuPath('')
  }, [currentPath, query, sourceFilter, typeFilter])

  const summary = useMemo(() => {
    const byType = new Map<string, number>()
    const bySource = new Map<string, number>()
    for (const entry of snapshotEntries) {
      const type = nodeType(entry)
      byType.set(type, (byType.get(type) || 0) + 1)
      const source = sourceFor(entry)
      bySource.set(source, (bySource.get(source) || 0) + 1)
    }
    return { byType, bySource }
  }, [snapshotEntries])

  const sourceOptions = useMemo(() => ['all', ...Array.from(summary.bySource.keys()).sort()], [summary.bySource])
  const typeOptions = useMemo(() => {
    const discovered = Array.from(summary.byType.keys())
    return ['all', ...typeOrder.filter((type) => discovered.includes(type)), ...discovered.filter((type) => !typeOrder.includes(type)).sort()]
  }, [summary.byType])

  const visibleEntries = useMemo(() => {
    if (!hasFilters) return sortTreeEntries(treeNode?.children || [])
    const q = query.trim().toLowerCase()
    return sortFilteredEntries(snapshotEntries
      .filter((entry) => typeFilter === 'all' || nodeType(entry) === typeFilter)
      .filter((entry) => sourceFilter === 'all' || sourceFor(entry) === sourceFilter)
      .filter((entry) => {
        if (!q) return true
        return `${entry.name} ${entry.path} ${sourceFor(entry)} ${nodeType(entry)}`.toLowerCase().includes(q)
      }))
  }, [hasFilters, query, snapshotEntries, sourceFilter, treeNode?.children, typeFilter])

  const categories = [
    { label: tx('全部', 'All'), path: rootPath, type: 'all', value: stats.files },
    { label: tx('技能', 'Skills'), path: '/skills', type: 'Skill', value: stats.skills },
    { label: tx('记忆', 'Memory'), path: '/memory', type: 'Memory', value: stats.memory },
    { label: tx('项目', 'Projects'), path: '/projects', type: 'Project', value: stats.projects },
    { label: tx('会话', 'Conversations'), path: '/conversations', type: 'Conversation', value: stats.conversations },
  ]

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
      browsePath('/uploads')
      await reloadData()
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
      setOpenMenuPath('')
      await reloadData()
    } catch (err: any) {
      setError(err?.message || tx('删除失败', 'Delete failed'))
    }
  }

  const copyPrompt = async (entry: FileNode) => {
    const prompt = `Use this neuDrive item as context: ${entry.path}`
    await writeClipboardText(prompt)
    setCopiedPath(entry.path)
    window.setTimeout(() => setCopiedPath(''), 1500)
  }

  const openEntry = (entry: FileNode) => {
    const path = entryPath(entry, currentPath)
    if (entry.is_dir) {
      browsePath(path)
      return
    }
    navigate(dataFileEditorRoute(path))
  }

  const loading = hasFilters ? snapshotLoading : treeLoading
  const currentRelativeParts = pathParts(currentPath)

  return (
    <div className="dashboard-file-stack">
      <div className="dashboard-data-category-grid" aria-label={tx('数据分类', 'Data categories')}>
        {categories.map((item) => {
          const active = item.type === 'all'
            ? !hasFilters && currentPath === rootPath
            : typeFilter === item.type || (!hasFilters && currentPath.startsWith(item.path))
          return (
            <button key={item.path} className={active ? 'dashboard-data-category is-active' : 'dashboard-data-category'} onClick={() => browsePath(item.path)}>
              <span>{item.label}</span>
              <strong>{item.value}</strong>
            </button>
          )
        })}
      </div>

      <section className="dashboard-file-panel dashboard-file-browser">
        <div className="dashboard-section-head">
          <div>
            <h3>{tx('文件', 'Files')}</h3>
            <div className="dashboard-breadcrumb" aria-label={tx('当前位置', 'Current path')}>
              <button onClick={() => browsePath(rootPath)}>{tx('根目录', 'Root')}</button>
              {!hasFilters && currentRelativeParts.map((part, index) => {
                const to = joinPath(currentRelativeParts.slice(0, index + 1))
                return (
                  <span key={to}>
                    <span>/</span>
                    <button onClick={() => browsePath(to)}>{part}</button>
                  </span>
                )
              })}
              {hasFilters && <span>{tx('/ 筛选结果', '/ filtered results')}</span>}
            </div>
          </div>
          <div className="dashboard-file-actions-bar">
            <button className="btn btn-outline" disabled={uploading} onClick={() => fileInputRef.current?.click()}>
              {uploading ? tx('上传中...', 'Uploading...') : tx('上传文件', 'Upload file')}
            </button>
            <button className="btn btn-outline" onClick={() => { void api.exportZip().catch((err: any) => setError(err?.message || tx('导出失败', 'Export failed'))) }}>
              {tx('导出全部', 'Export all')}
            </button>
            <input ref={fileInputRef} className="hidden-file-input" type="file" accept=".md,.txt,.json,.csv" onChange={(event) => {
              const file = event.target.files?.[0]
              if (file) void handleUpload(file)
              event.currentTarget.value = ''
            }} />
          </div>
        </div>

        <div className="dashboard-file-toolbar">
          <input
            className="input"
            placeholder={tx('搜索文件', 'Search files')}
            value={query}
            onChange={(event) => writeParams({ q: event.target.value }, true)}
          />
          <select value={typeFilter} onChange={(event) => writeParams({ type: event.target.value }, true)}>
            {typeOptions.map((option) => (
              <option key={option} value={option}>{option === 'all' ? tx('类型：全部', 'Type: All') : typeLabel(option, locale)}</option>
            ))}
          </select>
          <select value={sourceFilter} onChange={(event) => writeParams({ source: event.target.value }, true)}>
            {sourceOptions.map((option) => (
              <option key={option} value={option}>{option === 'all' ? tx('来源：全部', 'Source: All') : sourceLabel(option, locale)}</option>
            ))}
          </select>
        </div>

        {error && <div className="alert alert-warn">{error}</div>}

        <div className="dashboard-github-file-list" aria-busy={loading}>
          {!hasFilters && currentPath !== rootPath && (
            <div className="dashboard-file-row" role="button" tabIndex={0} onClick={() => browsePath(parentPath(currentPath))}>
              <span className="dashboard-file-icon">↩</span>
              <span className="dashboard-file-name">..</span>
              <span className="dashboard-file-kind">{tx('上一级', 'Parent')}</span>
              <span className="dashboard-file-source">-</span>
              <span className="dashboard-file-updated">-</span>
              <span className="dashboard-file-menu-cell" />
            </div>
          )}

          {visibleEntries.map((entry) => {
            const path = entryPath(entry, currentPath)
            const source = sourceFor(entry)
            return (
              <div
                key={path}
                className="dashboard-file-row"
                role="button"
                tabIndex={0}
                onClick={() => openEntry(entry)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' || event.key === ' ') openEntry(entry)
                }}
              >
                <span className="dashboard-file-icon">{entry.is_dir ? '▸' : '•'}</span>
                <span className={entry.is_dir ? 'dashboard-file-name is-folder' : 'dashboard-file-name'}>{entry.name}</span>
                <span className="dashboard-file-kind">{fileTypeLabel(entry, locale)}{entry.is_dir ? '' : ` · ${formatBytes(entry.size || entry.content?.length)}`}</span>
                <span className="dashboard-file-source">{sourceLabel(source, locale)}</span>
                <span className="dashboard-file-updated">{formatDateTime(entry.updated_at || entry.created_at, locale)}</span>
                <span className="dashboard-file-menu-cell" onClick={(event) => event.stopPropagation()}>
                  <button
                    className="dashboard-file-menu-button"
                    type="button"
                    aria-label={tx('更多操作', 'More actions')}
                    onClick={(event) => {
                      event.stopPropagation()
                      setOpenMenuPath(openMenuPath === path ? '' : path)
                    }}
                  >
                    ⋯
                  </button>
                  {openMenuPath === path && (
                    <span className="dashboard-file-menu" role="menu">
                      <button type="button" role="menuitem" onClick={() => { void copyPrompt(entry) }}>
                        {copiedPath === entry.path ? tx('已复制', 'Copied') : tx('复制提示词', 'Copy Prompt')}
                      </button>
                      <button type="button" role="menuitem" onClick={() => { void api.downloadTreeZip(entry.path).catch((err: any) => setError(err?.message || tx('导出失败', 'Export failed'))) }}>
                        {tx('导出', 'Export')}
                      </button>
                      <button type="button" role="menuitem" className="danger" onClick={() => { void deleteEntry(entry) }}>
                        {tx('删除', 'Delete')}
                      </button>
                    </span>
                  )}
                </span>
              </div>
            )
          })}

          {!loading && visibleEntries.length === 0 && (
            <div className="dashboard-file-empty">
              {hasFilters ? tx('没有匹配的文件。', 'No files match these filters.') : tx('这个目录暂时没有文件。', 'This folder is empty.')}
            </div>
          )}
          {loading && <div className="dashboard-file-empty">{tx('加载中...', 'Loading...')}</div>}
        </div>
      </section>
    </div>
  )
}
