import { useCallback, useEffect, useMemo, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { api, type FileNode } from '../../api'
import MaterialsSectionToolbar from '../../components/MaterialsSectionToolbar'
import FileMaterialsTile from '../../components/FileMaterialsTile'
import ResourceActionMenu from '../../components/ResourceActionMenu'
import ResourceConfirmDialog from '../../components/ResourceConfirmDialog'
import SourceFilterBar from '../../components/SourceFilterBar'
import useResourceCardMenu from '../../hooks/useResourceCardMenu'
import useTreeDeleteDialog from '../../hooks/useTreeDeleteDialog'
import { useI18n, type AppLocale } from '../../i18n'
import {
  buildFileTileModel,
  buildSourceFilterOptions,
  bundleBrowsePath,
  bundleInfoFromNode,
  bundleRelativeDirFromPath,
  conversationBundleKeyFromPath,
  dataConversationBundleRoute,
  dataFileEditorRoute,
  fileNodeSource,
  formatDateTime,
  getMaterialsSortOptions,
  isTextLikeFile,
  matchesSourceFilter,
  normalizeBundleRelativeDir,
  sortMaterialsItems,
  sourceLabel,
  type MaterialsSortDir,
  type MaterialsSortKey,
} from './DataShared'

function exportFilePath(bundlePath: string, target: 'claude' | 'chatgpt') {
  return `${bundlePath.replace(/\/+$/, '')}/resume-${target}.md`
}

function transcriptFilePath(bundlePath: string) {
  return `${bundlePath.replace(/\/+$/, '')}/conversation.md`
}

function triggerTextDownload(content: string, filename: string) {
  const blob = new Blob([content], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  URL.revokeObjectURL(url)
}

type ConversationBundleMeta = {
  title: string
  source: string
  sourceConversationId: string
  messageCount: number
  startedAt: string
  endedAt: string
  importedAt: string
  model: string
  projectName: string
  sourceUrl: string
  description: string
}

type ConversationMetaEntry = {
  label: string
  value: string
  code?: boolean
}

function metadataString(metadata: Record<string, any> | undefined, key: string) {
  const value = metadata?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

function metadataNumber(metadata: Record<string, any> | undefined, key: string) {
  const value = metadata?.[key]
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return 0
}

function conversationBundleMeta(node: FileNode): ConversationBundleMeta {
  const bundle = bundleInfoFromNode(node)
  const metadata = node.metadata
  return {
    title: bundle?.name || metadataString(metadata, 'conversation_title') || node.name,
    source: bundle?.source || fileNodeSource(node),
    sourceConversationId: metadataString(metadata, 'source_conversation_id'),
    messageCount:
      metadataNumber(metadata, 'conversation_message_count')
      || metadataNumber(metadata, 'message_count')
      || metadataNumber(metadata, 'turn_count'),
    startedAt: metadataString(metadata, 'conversation_started_at') || node.created_at || '',
    endedAt: metadataString(metadata, 'conversation_ended_at') || node.updated_at || node.created_at || '',
    importedAt: metadataString(metadata, 'imported_at') || node.updated_at || node.created_at || '',
    model: metadataString(metadata, 'conversation_model'),
    projectName: metadataString(metadata, 'conversation_project_name'),
    sourceUrl: metadataString(metadata, 'conversation_source_url'),
    description: metadataString(metadata, 'description') || bundle?.description || '',
  }
}

function conversationMessageCountLabel(count: number, locale: AppLocale) {
  if (count <= 0) return locale === 'zh-CN' ? '消息数未知' : 'Message count unknown'
  return locale === 'zh-CN' ? `${count} 条消息` : `${count} messages`
}

function conversationTimeSpanLabel(meta: ConversationBundleMeta, locale: AppLocale) {
  const startedAt = meta.startedAt ? formatDateTime(meta.startedAt, locale) : ''
  const endedAt = meta.endedAt ? formatDateTime(meta.endedAt, locale) : ''
  if (startedAt && endedAt) {
    if (startedAt === endedAt) return startedAt
    return locale === 'zh-CN' ? `${startedAt} 至 ${endedAt}` : `${startedAt} to ${endedAt}`
  }
  return startedAt || endedAt
}

function conversationCardDescription(meta: ConversationBundleMeta, fallbackDescription: string | undefined, locale: AppLocale) {
  const details = [
    meta.model,
    meta.projectName ? (locale === 'zh-CN' ? `项目 ${meta.projectName}` : `Project ${meta.projectName}`) : '',
    meta.sourceConversationId ? (locale === 'zh-CN' ? `来源 ID ${meta.sourceConversationId}` : `Source ID ${meta.sourceConversationId}`) : '',
  ].filter(Boolean)
  if (details.length > 0) return details.join(' · ')
  return fallbackDescription || meta.description
}

function buildConversationMetaEntries(meta: ConversationBundleMeta, locale: AppLocale, detailed = false): ConversationMetaEntry[] {
  const entries: ConversationMetaEntry[] = []
  const timeSpan = conversationTimeSpanLabel(meta, locale)
  if (detailed && meta.source) {
    entries.push({
      label: locale === 'zh-CN' ? '来源平台' : 'Source',
      value: sourceLabel(meta.source, locale),
    })
  }
  if (detailed && timeSpan) {
    entries.push({
      label: locale === 'zh-CN' ? '时间跨度' : 'Time span',
      value: timeSpan,
    })
  }
  if (meta.messageCount > 0 || detailed) {
    entries.push({
      label: locale === 'zh-CN' ? '消息数' : 'Messages',
      value: conversationMessageCountLabel(meta.messageCount, locale),
    })
  }
  if (meta.importedAt) {
    entries.push({
      label: locale === 'zh-CN' ? '导入时间' : 'Imported',
      value: formatDateTime(meta.importedAt, locale),
    })
  }
  if (detailed && meta.model) {
    entries.push({
      label: locale === 'zh-CN' ? '模型' : 'Model',
      value: meta.model,
    })
  }
  if (detailed && meta.projectName) {
    entries.push({
      label: locale === 'zh-CN' ? '项目' : 'Project',
      value: meta.projectName,
    })
  }
  if (detailed && meta.sourceConversationId) {
    entries.push({
      label: locale === 'zh-CN' ? '来源 ID' : 'Source ID',
      value: meta.sourceConversationId,
      code: true,
    })
  }
  return entries
}

function ConversationMetaPanel({ entries, compact = false }: { entries: ConversationMetaEntry[]; compact?: boolean }) {
  if (entries.length === 0) return null
  return (
    <div className={`conversation-meta-panel${compact ? ' is-compact' : ''}`}>
      {entries.map((entry) => (
        <div key={`${entry.label}:${entry.value}`} className="conversation-meta-item">
          <span className="conversation-meta-label">{entry.label}</span>
          {entry.code ? (
            <code className="conversation-meta-value is-code">{entry.value}</code>
          ) : (
            <span className="conversation-meta-value">{entry.value}</span>
          )}
        </div>
      ))}
    </div>
  )
}

export default function DataConversationsPage() {
  const { locale, tx } = useI18n()
  const navigate = useNavigate()
  const location = useLocation()
  const params = useParams()
  const bundleWildcard = (params['*'] || '').trim()
  const query = useMemo(() => new URLSearchParams(location.search), [location.search])
  const currentRelativeDir = normalizeBundleRelativeDir(query.get('dir'))
  const currentBundlePath = bundleWildcard ? `/conversations/${decodeURIComponent(bundleWildcard).replace(/^\/+/, '')}` : ''
  const currentBrowsePath = currentBundlePath ? bundleBrowsePath(currentBundlePath, currentRelativeDir) : ''
  const isBundleView = Boolean(currentBundlePath)

  const [bundles, setBundles] = useState<FileNode[]>([])
  const [bundleNode, setBundleNode] = useState<FileNode | null>(null)
  const [bundleRootNode, setBundleRootNode] = useState<FileNode | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selectedBundlePath, setSelectedBundlePath] = useState<string | null>(null)
  const [selectedEntryPath, setSelectedEntryPath] = useState<string | null>(null)
  const [sortKey, setSortKey] = useState<MaterialsSortKey>('updated_at')
  const [sortDir, setSortDir] = useState<MaterialsSortDir>('desc')
  const [sourceFilter, setSourceFilter] = useState('all')
  const [exportingTarget, setExportingTarget] = useState<string | null>(null)
  const { closeMenu, isMenuOpen, toggleMenu } = useResourceCardMenu()

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      if (isBundleView) {
        const [node, rootNode] = await Promise.all([
          api.getTree(currentBrowsePath),
          currentBrowsePath === currentBundlePath ? Promise.resolve<FileNode | null>(null) : api.getTree(currentBundlePath),
        ])
        setBundleNode(node)
        setBundleRootNode(rootNode || node)
        setBundles([])
        setSelectedEntryPath(null)
        closeMenu()
        return
      }

      const snapshot = await api.getTreeSnapshot('/conversations')
      const nextBundles = snapshot.entries.filter((entry) => {
        if (!entry.is_dir) return false
        return bundleInfoFromNode(entry)?.kind === 'conversation'
      })
      setBundles(nextBundles)
      setBundleNode(null)
      setBundleRootNode(null)
      setSelectedBundlePath(null)
      closeMenu()
    } catch (err: any) {
      setError(err.message || tx('加载会话失败', 'Failed to load conversations'))
    } finally {
      setLoading(false)
    }
  }, [closeMenu, currentBrowsePath, currentBundlePath, isBundleView, tx])

  const {
    closeDialog: closeDeleteDialog,
    confirmDelete,
    dialog: deleteDialog,
    requestDelete,
    submitting: deleteSubmitting,
  } = useTreeDeleteDialog({ tx, onDeleted: load })

  useEffect(() => {
    void load()
  }, [load])

  const currentBundleContext = bundleNode?.bundle_context || bundleRootNode?.bundle_context
  const currentConversationMeta = bundleRootNode
    ? conversationBundleMeta(bundleRootNode)
    : (bundleNode ? conversationBundleMeta(bundleNode) : null)
  const bundleEntries = bundleNode?.children || []
  const selectedDeletePath = isBundleView ? selectedEntryPath : selectedBundlePath
  const canDeleteSelection = Boolean(selectedDeletePath)

  const sortOptions = getMaterialsSortOptions(locale)
  const sortedBundles = useMemo(
    () =>
      sortMaterialsItems({
        items: bundles,
        sortKey,
        sortDir,
        getName: (bundle) => bundleInfoFromNode(bundle)?.name || bundle.name,
        getUpdatedAt: (bundle) => bundle.updated_at || bundle.created_at,
      }),
    [bundles, sortDir, sortKey],
  )
  const filteredBundles = useMemo(
    () => sortedBundles.filter((bundle) => matchesSourceFilter(fileNodeSource(bundle), sourceFilter)),
    [sortedBundles, sourceFilter],
  )
  const bundleSourceOptions = useMemo(
    () => buildSourceFilterOptions(bundles, fileNodeSource, locale),
    [bundles, locale],
  )

  const sortedBundleEntries = useMemo(
    () =>
      sortMaterialsItems({
        items: bundleEntries,
        sortKey,
        sortDir,
        getName: (entry) => entry.name,
        getUpdatedAt: (entry) => entry.updated_at || entry.created_at,
        groupComparator: (left, right) => {
          const leftPriority = left.name === 'conversation.md' ? -3 : left.name === 'resume-claude.md' ? -2 : left.name === 'resume-chatgpt.md' ? -1 : 0
          const rightPriority = right.name === 'conversation.md' ? -3 : right.name === 'resume-claude.md' ? -2 : right.name === 'resume-chatgpt.md' ? -1 : 0
          if (leftPriority !== rightPriority) return leftPriority - rightPriority
          if (left.is_dir !== right.is_dir) return left.is_dir ? -1 : 1
          return 0
        },
      }),
    [bundleEntries, sortDir, sortKey],
  )
  const filteredBundleEntries = useMemo(
    () => sortedBundleEntries.filter((entry) => matchesSourceFilter(fileNodeSource(entry), sourceFilter)),
    [sortedBundleEntries, sourceFilter],
  )
  const entrySourceOptions = useMemo(
    () => buildSourceFilterOptions(bundleEntries, fileNodeSource, locale),
    [bundleEntries, locale],
  )

  const openConversationBundle = useCallback((bundlePath: string, relativeDir = '') => {
    closeMenu()
    navigate(dataConversationBundleRoute(bundlePath, relativeDir))
  }, [closeMenu, navigate])

  const openFileEditor = useCallback((path: string) => {
    closeMenu()
    navigate(dataFileEditorRoute(path))
  }, [closeMenu, navigate])

  const openBundleFolder = useCallback((path: string) => {
    if (!currentBundlePath) return
    openConversationBundle(currentBundlePath, bundleRelativeDirFromPath(currentBundlePath, path))
  }, [currentBundlePath, openConversationBundle])

  const handleDownloadZip = useCallback(async (path: string) => {
    closeMenu()
    try {
      await api.downloadTreeZip(path)
    } catch (err: any) {
      setError(err.message || tx('下载 ZIP 失败', 'Failed to download ZIP'))
    }
  }, [closeMenu, tx])

  const handleExport = useCallback(async (bundlePath: string, target: 'claude' | 'chatgpt') => {
    const exportPath = exportFilePath(bundlePath, target)
    setExportingTarget(`${bundlePath}:${target}`)
    setError('')
    try {
      const node = await api.getTree(exportPath)
      triggerTextDownload(node.content || '', `${conversationBundleKeyFromPath(bundlePath).replace(/\//g, '-')}-resume-${target}.md`)
    } catch (err: any) {
      setError(err.message || tx('导出文件不存在', 'Export file is missing'))
    } finally {
      setExportingTarget(null)
    }
  }, [tx])

  if (loading) {
    return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>
  }

  if (isBundleView) {
    const relativeSegments = currentRelativeDir.split('/').filter(Boolean)
    const transcriptPath = transcriptFilePath(currentBundlePath)

    return (
      <div className="page materials-page">
        <section className="materials-hero">
          <div className="materials-hero-copy">
            <nav aria-label={tx('面包屑', 'Breadcrumbs')} className="materials-breadcrumbs">
              <button className="btn-text" onClick={() => navigate('/data/conversations')}>{tx('会话', 'Conversations')}</button>
              {currentBundleContext ? (
                <>
                  <span className="breadcrumbs-sep">/</span>
                  <button className="btn-text" onClick={() => openConversationBundle(currentBundlePath)}>{currentBundleContext.name}</button>
                </>
              ) : null}
              {relativeSegments.map((segment, index) => {
                const relative = relativeSegments.slice(0, index + 1).join('/')
                return (
                  <span key={relative}>
                    <span className="breadcrumbs-sep">/</span>
                    <button className="btn-text" onClick={() => openConversationBundle(currentBundlePath, relative)}>{segment}</button>
                  </span>
                )
              })}
            </nav>
            <div className="materials-kicker">neuDrive Data</div>
            <h2 className="materials-title">{currentBundleContext?.name || tx('会话 Bundle', 'Conversation Bundle')}</h2>
            <p className="materials-subtitle">{tx('会话现在作为一等 bundle 管理：可读转录、规范化 sidecar，以及 Claude / ChatGPT 续聊导出都放在同一个目录。', 'Conversations now live as first-class bundles: readable transcript, normalized sidecar, and Claude / ChatGPT resume exports stay in one directory.')}</p>
            {currentConversationMeta ? (
              <ConversationMetaPanel entries={buildConversationMetaEntries(currentConversationMeta, locale, true)} />
            ) : null}
          </div>
        </section>

        {error && <div className="alert alert-error">{error}</div>}
        {!error && !currentBundleContext && (
          <div className="alert alert-warn">{tx('没有找到这个 conversation bundle。', 'This conversation bundle could not be found.')}</div>
        )}

        {currentBundleContext ? (
          <section className="materials-section">
            <div className="materials-section-head">
              <div>
                <h3 className="materials-section-title">{tx('Bundle 内容', 'Bundle contents')}</h3>
                <p className="materials-section-copy">{tx('直接浏览会话归档里的 transcript、sidecar 和导出稿。', 'Browse the transcript, sidecar, and resume exports directly inside the conversation archive.')}</p>
              </div>
              <MaterialsSectionToolbar
                count={filteredBundleEntries.length}
                sortKey={sortKey}
                sortOptions={sortOptions}
                sortDir={sortDir}
                onSortKeyChange={(value) => setSortKey(value as MaterialsSortKey)}
                onSortDirToggle={() => setSortDir((value) => (value === 'desc' ? 'asc' : 'desc'))}
              >
                <button className="btn btn-sm materials-toolbar-control" onClick={() => navigate(dataFileEditorRoute(transcriptPath))}>
                  {tx('打开转录', 'Open transcript')}
                </button>
                <button
                  className="btn btn-sm materials-toolbar-control"
                  onClick={() => void handleExport(currentBundlePath, 'claude')}
                  disabled={exportingTarget === `${currentBundlePath}:claude`}
                >
                  {exportingTarget === `${currentBundlePath}:claude` ? tx('导出中...', 'Exporting...') : tx('导出到 Claude', 'Export to Claude')}
                </button>
                <button
                  className="btn btn-sm materials-toolbar-control"
                  onClick={() => void handleExport(currentBundlePath, 'chatgpt')}
                  disabled={exportingTarget === `${currentBundlePath}:chatgpt`}
                >
                  {exportingTarget === `${currentBundlePath}:chatgpt` ? tx('导出中...', 'Exporting...') : tx('导出到 ChatGPT', 'Export to ChatGPT')}
                </button>
                <button
                  className="btn btn-sm materials-toolbar-control is-danger"
                  disabled={!canDeleteSelection}
                  onClick={() => {
                    if (selectedDeletePath) void requestDelete([selectedDeletePath])
                  }}
                >
                  {tx('删除', 'Delete')}
                </button>
              </MaterialsSectionToolbar>
            </div>

            {(entrySourceOptions.length > 1 || sourceFilter !== 'all') && (
              <SourceFilterBar options={entrySourceOptions} value={sourceFilter} onChange={setSourceFilter} />
            )}

            {filteredBundleEntries.length === 0 ? (
              <div className="empty-state">
                <p>{tx('这个会话目录还没有内容', 'This conversation folder is empty')}</p>
              </div>
            ) : (
              <div className="materials-grid">
                {filteredBundleEntries.map((entry) => {
                  const tile = buildFileTileModel({
                    node: entry,
                    variant: 'bundle-entry',
                    bundleLabel: currentBundleContext.name,
                    locale,
                  })
                  const editable = isTextLikeFile(entry.name, entry.mime_type)
                  return (
                    <FileMaterialsTile
                      key={entry.path}
                      node={tile.node}
                      subtitle={tile.subtitle}
                      description={tile.description}
                      extraPills={tile.source ? <span className="materials-tile-pill materials-source-pill">{sourceLabel(tile.source, locale)}</span> : undefined}
                      path={tile.path}
                      footerStart={tile.footerStart}
                      footerEnd={tile.footerEnd}
                      selected={selectedEntryPath === entry.path}
                      menuOpen={isMenuOpen(entry.path)}
                      menuButtonAriaLabel={tx(`打开 ${entry.name} 的工具菜单`, `Open tools menu for ${entry.name}`)}
                      menuPanel={(
                        <ResourceActionMenu
                          items={[
                            ...((entry.is_dir || editable)
                              ? [{
                                  key: 'open',
                                  label: entry.is_dir ? tx('进入目录', 'Open folder') : tx('打开文件', 'Open file'),
                                  onSelect: () => {
                                    closeMenu()
                                    if (entry.is_dir) {
                                      openBundleFolder(entry.path)
                                    } else {
                                      openFileEditor(entry.path)
                                    }
                                  },
                                }]
                              : []),
                            {
                              key: 'download',
                              label: tx('下载 ZIP', 'Download ZIP'),
                              onSelect: () => {
                                void handleDownloadZip(entry.path)
                              },
                            },
                            {
                              key: 'select',
                              label: selectedEntryPath === entry.path ? tx('取消选中', 'Unselect') : tx('加入选择', 'Select'),
                              onSelect: () => {
                                closeMenu()
                                setSelectedEntryPath((value) => (value === entry.path ? null : entry.path))
                              },
                            },
                            {
                              key: 'delete',
                              label: tx('删除', 'Delete'),
                              tone: 'danger' as const,
                              onSelect: () => {
                                closeMenu()
                                void requestDelete([entry.path])
                              },
                            },
                          ]}
                        />
                      )}
                      onMenuToggle={() => toggleMenu(entry.path)}
                      onSelect={() => setSelectedEntryPath(entry.path)}
                      onOpen={entry.is_dir ? () => openBundleFolder(entry.path) : (editable ? () => openFileEditor(entry.path) : undefined)}
                    />
                  )
                })}
              </div>
            )}
          </section>
        ) : null}

        <ResourceConfirmDialog
          open={Boolean(deleteDialog)}
          kicker={tx('删除确认', 'Delete confirmation')}
          title={deleteDialog?.nonEmptyDirectories.length ? tx('这些目录不是空的', 'These folders are not empty') : tx('确认删除选中条目', 'Confirm deletion')}
          description={deleteDialog?.nonEmptyDirectories.length
            ? tx('确认后会递归删除其中所有可写文件和文件夹。', 'Continuing will recursively delete all writable files and folders inside.')
            : tx('这个操作会删除选中的会话文件或目录，且不可撤销。', 'This will delete the selected conversation file or directory and cannot be undone.')}
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

  return (
    <div className="page materials-page">
      <section className="materials-hero">
        <div className="materials-hero-copy">
          <div className="materials-kicker">neuDrive Data</div>
          <h2 className="materials-title">{tx('会话', 'Conversations')}</h2>
          <p className="materials-subtitle">{tx('Conversation 现在和 Projects、Skills、Memory 并列，是一等 neuDrive 域。每张卡片都是一个 bundle，里面包含 transcript、normalized sidecar 和平台续聊导出。', 'Conversations now sit alongside Projects, Skills, and Memory as a first-class neuDrive domain. Each card is a bundle that contains the transcript, normalized sidecar, and platform resume exports.')}</p>
        </div>
      </section>

      {error && <div className="alert alert-error">{error}</div>}

      <section className="materials-section">
        <div className="materials-section-head">
          <div>
            <h3 className="materials-section-title">{tx('Conversation Library', 'Conversation Library')}</h3>
            <p className="materials-section-copy">{tx('统一浏览 conversation bundles，并直接导出到 Claude 或 ChatGPT。', 'Browse conversation bundles and export directly to Claude or ChatGPT.')}</p>
          </div>
          <MaterialsSectionToolbar
            count={filteredBundles.length}
            sortKey={sortKey}
            sortOptions={sortOptions}
            sortDir={sortDir}
            onSortKeyChange={(value) => setSortKey(value as MaterialsSortKey)}
            onSortDirToggle={() => setSortDir((value) => (value === 'desc' ? 'asc' : 'desc'))}
          >
            <button
              className="btn btn-sm materials-toolbar-control is-danger"
              disabled={!canDeleteSelection}
              onClick={() => {
                if (selectedDeletePath) void requestDelete([selectedDeletePath])
              }}
            >
              {tx('删除', 'Delete')}
            </button>
          </MaterialsSectionToolbar>
        </div>

        {(bundleSourceOptions.length > 1 || sourceFilter !== 'all') && (
          <SourceFilterBar options={bundleSourceOptions} value={sourceFilter} onChange={setSourceFilter} />
        )}

        {filteredBundles.length === 0 ? (
          <div className="empty-state">
            <p>{tx('还没有 Conversation 内容', 'No conversations yet')}</p>
            <p className="empty-hint">{tx('从 Claude Web、ChatGPT Web 或 Claude Code 导入后，会在这里生成 conversation bundle。', 'Conversation bundles will appear here after imports from Claude Web, ChatGPT Web, or Claude Code.')}</p>
          </div>
        ) : (
          <div className="materials-grid materials-grid-wide">
            {filteredBundles.map((bundle) => {
              const tile = buildFileTileModel({
                node: bundle,
                variant: 'browser',
                currentLabel: tx('会话', 'Conversations'),
                locale,
              })
              const bundlePath = bundle.path
              const meta = conversationBundleMeta(bundle)
              const metaEntries = buildConversationMetaEntries(meta, locale)
              const subtitle = conversationTimeSpanLabel(meta, locale) || tile.subtitle
              const description = conversationCardDescription(meta, typeof tile.description === 'string' ? tile.description : meta.description, locale) || undefined
              return (
                <FileMaterialsTile
                  key={bundle.path}
                  node={tile.node}
                  subtitle={subtitle}
                  description={description}
                  path={tile.path}
                  extraPills={tile.source ? <span className="materials-tile-pill materials-source-pill">{sourceLabel(tile.source, locale)}</span> : undefined}
                  footerStart={tile.footerStart}
                  footerEnd={tile.footerEnd}
                  selected={selectedBundlePath === bundle.path}
                  actions={(
                    <>
                      <button
                        type="button"
                        className="btn btn-sm materials-toolbar-control"
                        onClick={() => void handleExport(bundlePath, 'claude')}
                        disabled={exportingTarget === `${bundlePath}:claude`}
                      >
                        {exportingTarget === `${bundlePath}:claude` ? tx('导出中...', 'Exporting...') : tx('导出到 Claude', 'Export to Claude')}
                      </button>
                      <button
                        type="button"
                        className="btn btn-sm materials-toolbar-control"
                        onClick={() => void handleExport(bundlePath, 'chatgpt')}
                        disabled={exportingTarget === `${bundlePath}:chatgpt`}
                      >
                        {exportingTarget === `${bundlePath}:chatgpt` ? tx('导出中...', 'Exporting...') : tx('导出到 ChatGPT', 'Export to ChatGPT')}
                      </button>
                    </>
                  )}
                  children={<ConversationMetaPanel entries={metaEntries} compact />}
                  menuOpen={isMenuOpen(bundle.path)}
                  menuButtonAriaLabel={tx(`打开 ${tile.node.name} 的工具菜单`, `Open tools menu for ${tile.node.name}`)}
                  menuPanel={(
                    <ResourceActionMenu
                      items={[
                        {
                          key: 'open',
                          label: tx('打开 Bundle', 'Open bundle'),
                          onSelect: () => openConversationBundle(bundle.path),
                        },
                        {
                          key: 'download',
                          label: tx('下载 ZIP', 'Download ZIP'),
                          onSelect: () => {
                            void handleDownloadZip(bundle.path)
                          },
                        },
                        {
                          key: 'delete',
                          label: tx('删除', 'Delete'),
                          tone: 'danger' as const,
                          onSelect: () => {
                            closeMenu()
                            void requestDelete([bundle.path])
                          },
                        },
                      ]}
                    />
                  )}
                  onMenuToggle={() => toggleMenu(bundle.path)}
                  onSelect={() => setSelectedBundlePath((value) => (value === bundle.path ? null : bundle.path))}
                  onOpen={() => openConversationBundle(bundle.path)}
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
          ? tx('确认后会递归删除其中所有可写文件和文件夹。', 'Continuing will recursively delete all writable files and folders inside.')
          : tx('这个操作会删除选中的 conversation bundle，且不可撤销。', 'This will delete the selected conversation bundle and cannot be undone.')}
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
