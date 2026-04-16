import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { api, type FileNode } from '../api'
import MaterialsSectionToolbar from '../components/MaterialsSectionToolbar'
import MaterialsTile from '../components/MaterialsTile'
import FileMaterialsTile from '../components/FileMaterialsTile'
import ResourceActionMenu from '../components/ResourceActionMenu'
import ResourceConfirmDialog from '../components/ResourceConfirmDialog'
import SourceFilterBar from '../components/SourceFilterBar'
import useResourceCardMenu from '../hooks/useResourceCardMenu'
import useTreeDeleteDialog from '../hooks/useTreeDeleteDialog'
import { useI18n } from '../i18n'
import {
  buildFileTileModel,
  buildSourceFilterOptions,
  bundleBrowsePath,
  bundleCapabilityLabel,
  bundleRelativeDirFromPath,
  dataFileEditorRoute,
  dataProjectBundleRoute,
  fileNodeSource,
  getMaterialsSortOptions,
  matchesSourceFilter,
  normalizeBundleRelativeDir,
  projectSource,
  sortMaterialsItems,
  sourceLabel,
  type MaterialsSortDir,
  type MaterialsSortKey,
} from './data/DataShared'

interface Project {
  name: string
  status: string
  description?: string
  primary_path?: string
  log_path?: string
  capabilities?: string[]
  last_activity?: string
  updated_at?: string
  context_md?: string
  metadata?: Record<string, any>
}

export default function ProjectsPage() {
  const { locale, tx } = useI18n()
  const navigate = useNavigate()
  const location = useLocation()
  const params = useParams()
  const projectName = (params.projectName || '').trim()
  const query = useMemo(() => new URLSearchParams(location.search), [location.search])
  const currentRelativeDir = normalizeBundleRelativeDir(query.get('dir'))
  const currentBundlePath = projectName ? `/projects/${projectName}` : ''
  const currentBrowsePath = currentBundlePath ? bundleBrowsePath(currentBundlePath, currentRelativeDir) : ''
  const isBundleView = Boolean(currentBundlePath)

  const [projects, setProjects] = useState<Project[]>([])
  const [bundleNode, setBundleNode] = useState<FileNode | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selectedProjectName, setSelectedProjectName] = useState<string | null>(null)
  const [selectedEntryPath, setSelectedEntryPath] = useState<string | null>(null)
  const [showNewForm, setShowNewForm] = useState(false)
  const [newName, setNewName] = useState('')
  const [creating, setCreating] = useState(false)
  const [sortKey, setSortKey] = useState<MaterialsSortKey>('updated_at')
  const [sortDir, setSortDir] = useState<MaterialsSortDir>('desc')
  const [sourceFilter, setSourceFilter] = useState('all')
  const [archiveTarget, setArchiveTarget] = useState<Project | null>(null)
  const [archiveSubmitting, setArchiveSubmitting] = useState(false)
  const { activeMenuId, closeMenu, isMenuOpen, toggleMenu } = useResourceCardMenu()

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      if (isBundleView) {
        const node = await api.getTree(currentBrowsePath)
        setBundleNode(node)
        setProjects([])
        closeMenu()
        setSelectedEntryPath(null)
        return
      }

      const data = await api.getProjects()
      setProjects(data || [])
      setBundleNode(null)
      closeMenu()
      setSelectedProjectName(null)
    } catch (err: any) {
      setError(err.message || tx('加载项目失败', 'Failed to load projects'))
    } finally {
      setLoading(false)
    }
  }, [closeMenu, currentBrowsePath, isBundleView, tx])

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

  const currentBundleContext = bundleNode?.bundle_context
  const bundleEntries = bundleNode?.children || []
  const selectedProject = selectedProjectName
    ? projects.find((project) => project.name === selectedProjectName) || null
    : null
  const selectedDeletePath = isBundleView ? selectedEntryPath : null
  const canDeleteSelection = Boolean(isBundleView && selectedDeletePath)

  const projectCapabilities = (project: Project) => {
    const raw = Array.isArray(project.capabilities)
      ? project.capabilities.map((value) => `${value || ''}`.trim().toLowerCase()).filter(Boolean)
      : []
    return Array.from(new Set(raw.length > 0 ? raw : ['context', 'logs']))
  }

  const projectDescription = (project: Project) =>
    project.description || tx('这个项目 bundle 还没有补充描述。', 'This project bundle does not have a description yet.')

  const handleCreate = async (event: FormEvent) => {
    event.preventDefault()
    if (!newName.trim()) return

    setCreating(true)
    setError('')
    try {
      await api.createProject(newName.trim())
      setNewName('')
      setShowNewForm(false)
      await load()
    } catch (err: any) {
      setError(err.message || tx('新建项目失败', 'Failed to create project'))
    } finally {
      setCreating(false)
    }
  }

  const requestArchive = (project: Project) => {
    closeMenu()
    setArchiveTarget(project)
  }

  const closeArchiveDialog = () => {
    if (archiveSubmitting) return
    setArchiveTarget(null)
  }

  const handleArchive = async (project: Project) => {
    setArchiveSubmitting(true)
    try {
      await api.archiveProject(project.name)
      setArchiveTarget(null)
      await load()
    } catch (err: any) {
      setError(err.message || tx('归档项目失败', 'Failed to archive project'))
    } finally {
      setArchiveSubmitting(false)
    }
  }

  const openProjectBundle = useCallback((name: string, relativeDir = '') => {
    closeMenu()
    navigate(dataProjectBundleRoute(name, relativeDir))
  }, [closeMenu, navigate])

  const openFileEditor = useCallback((path: string) => {
    closeMenu()
    navigate(dataFileEditorRoute(path))
  }, [closeMenu, navigate])

  const openBundleFolder = useCallback((path: string) => {
    if (!projectName) return
    openProjectBundle(projectName, bundleRelativeDirFromPath(currentBundlePath, path))
  }, [currentBundlePath, openProjectBundle, projectName])

  const handleDownloadZip = useCallback(async (path: string) => {
    closeMenu()
    try {
      await api.downloadTreeZip(path)
    } catch (err: any) {
      setError(err.message || tx('下载 ZIP 失败', 'Failed to download ZIP'))
    }
  }, [closeMenu, tx])

  const formatTime = (ts?: string) => {
    if (!ts) return '-'
    try {
      return new Date(ts).toLocaleString(locale === 'zh-CN' ? 'zh-CN' : 'en-US')
    } catch {
      return ts
    }
  }

  const sortOptions = getMaterialsSortOptions(locale)
  const getProjectLastActivity = (project: Project) => project.last_activity || project.updated_at
  const sortedProjects = useMemo(
    () =>
      sortMaterialsItems({
        items: projects,
        sortKey,
        sortDir,
        getName: (project) => project.name,
        getUpdatedAt: (project) => getProjectLastActivity(project),
      }),
    [projects, sortDir, sortKey],
  )
  const filteredProjects = useMemo(
    () => sortedProjects.filter((project) => matchesSourceFilter(projectSource(project), sourceFilter)),
    [sortedProjects, sourceFilter],
  )
  const sourceOptions = useMemo(
    () => buildSourceFilterOptions(projects, projectSource, locale),
    [locale, projects],
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
          const leftPriority = left.name === 'context.md' ? -2 : left.name === 'log.jsonl' ? -1 : 0
          const rightPriority = right.name === 'context.md' ? -2 : right.name === 'log.jsonl' ? -1 : 0
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
  const bundleSourceOptions = useMemo(
    () => buildSourceFilterOptions(bundleEntries, fileNodeSource, locale),
    [bundleEntries, locale],
  )

  const relativeSegments = currentRelativeDir.split('/').filter(Boolean)

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (archiveTarget || deleteDialog || activeMenuId) return
      if (event.key === 'Escape') {
        if (isBundleView) setSelectedEntryPath(null)
        else setSelectedProjectName(null)
        return
      }
      if (isBundleView && event.key === 'Delete' && selectedDeletePath) {
        event.preventDefault()
        void requestDelete([selectedDeletePath])
        return
      }
      if (!isBundleView && event.key === 'Delete' && selectedProject?.status === 'active') {
        event.preventDefault()
        requestArchive(selectedProject)
        return
      }
      if (event.key !== 'Enter') return
      if (isBundleView && selectedEntryPath) {
        const entry = bundleEntries.find((item) => item.path === selectedEntryPath)
        if (!entry) return
        if (entry.is_dir) {
          openBundleFolder(entry.path)
          return
        }
        const mimeType = entry.mime_type || ''
        if (/\.md$/i.test(entry.name) || mimeType.startsWith('text/')) {
          openFileEditor(entry.path)
        }
        return
      }
      if (!isBundleView && selectedProject) {
        openProjectBundle(selectedProject.name)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [
    activeMenuId,
    archiveTarget,
    bundleEntries,
    deleteDialog,
    isBundleView,
    openBundleFolder,
    openFileEditor,
    openProjectBundle,
    requestDelete,
    selectedDeletePath,
    selectedEntryPath,
    selectedProject,
  ])

  if (loading) {
    return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>
  }

  if (isBundleView) {
    return (
      <div className="page materials-page">
        <section className="materials-hero">
          <div className="materials-hero-copy">
            <nav aria-label={tx('面包屑', 'Breadcrumbs')} className="materials-breadcrumbs">
              <button className="btn-text" onClick={() => navigate('/data/projects')}>{tx('项目', 'Projects')}</button>
              {currentBundleContext ? (
                <>
                  <span className="breadcrumbs-sep">/</span>
                  <button className="btn-text" onClick={() => openProjectBundle(projectName)}>{currentBundleContext.name}</button>
                </>
              ) : null}
              {relativeSegments.map((segment, index) => {
                const relative = relativeSegments.slice(0, index + 1).join('/')
                return (
                  <span key={relative}>
                    <span className="breadcrumbs-sep">/</span>
                    <button className="btn-text" onClick={() => openProjectBundle(projectName, relative)}>{segment}</button>
                  </span>
                )
              })}
            </nav>
            <div className="materials-kicker">neuDrive Data</div>
            <h2 className="materials-title">{currentBundleContext?.name || projectName}</h2>
            <p className="materials-subtitle">
              {tx('项目详情现在按 project bundle 浏览模式展示，深入子目录时也会保留上下文。', 'Project details now use a project-bundle browsing view, with context preserved while you drill into subfolders.')}
            </p>
          </div>
        </section>

        {error && <div className="alert alert-error">{error}</div>}
        {!error && !currentBundleContext && (
          <div className="alert alert-warn">{tx('没有找到这个 project bundle。', 'This project bundle could not be found.')}</div>
        )}

        {currentBundleContext ? (
          <section className="materials-section">
            <div className="materials-section-head">
              <div>
                <h3 className="materials-section-title">{tx('Bundle 内容', 'Bundle contents')}</h3>
                <p className="materials-section-copy">{tx('项目内的文件和目录现在都在这个视图里继续浏览。', 'Continue browsing project files and directories from this bundle view.')}</p>
              </div>
              <MaterialsSectionToolbar
                count={filteredBundleEntries.length}
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

            {filteredBundleEntries.length === 0 ? (
              <div className="empty-state">
                <p>{tx('这个项目目录还没有内容', 'This project folder is empty')}</p>
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
                  const mimeType = entry.mime_type || ''
                  const editable = /\.md$/i.test(entry.name) || mimeType.startsWith('text/')
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
            : tx('这个操作会删除选中的项目文件或目录，且不可撤销。', 'This will delete the selected project file or directory and cannot be undone.')}
          cancelLabel={tx('取消', 'Cancel')}
          confirmLabel={deleteSubmitting ? tx('删除中...', 'Deleting...') : tx('确认删除', 'Delete')}
          tone="danger"
          submitting={deleteSubmitting}
          onCancel={closeDeleteDialog}
          onConfirm={() => void confirmDelete()}
        />

        <ResourceConfirmDialog
          open={Boolean(archiveTarget)}
          kicker={tx('危险操作', 'Danger zone')}
          title={archiveTarget ? tx(`确认归档项目 “${archiveTarget.name}”`, `Archive project "${archiveTarget.name}"`) : ''}
          description={tx('归档不会删除项目文件，但项目会退出活跃状态。你之后仍然可以继续在文件树里查看它。', 'Archiving does not delete project files, but the project leaves the active state. You can still view it in the file tree afterward.')}
          cancelLabel={tx('取消', 'Cancel')}
          confirmLabel={archiveSubmitting ? tx('归档中...', 'Archiving...') : tx('确认归档', 'Archive')}
          tone="danger"
          submitting={archiveSubmitting}
          onCancel={closeArchiveDialog}
          onConfirm={() => {
            if (archiveTarget) void handleArchive(archiveTarget)
          }}
        />
      </div>
    )
  }

  return (
    <div className="page materials-page">
      <section className="materials-hero">
        <div className="materials-hero-copy">
          <div className="materials-kicker">neuDrive Data</div>
          <h2 className="materials-title">{tx('项目', 'Projects')}</h2>
          <p className="materials-subtitle">{tx('把项目看成一组长期上下文卡片。点开后会进入 project bundle 视图。', 'Treat projects as long-lived context cards. Opening one jumps into the project bundle view.')}</p>
        </div>
      </section>

      {error && <div className="alert alert-error">{error}</div>}

      {showNewForm && (
        <div className="materials-panel form-card">
          <div className="materials-section-head">
            <div>
              <h3 className="materials-section-title">{tx('新建项目', 'New project')}</h3>
              <p className="materials-section-copy">{tx('创建一个新的项目空间，用来整理任务上下文、日志和相关资料。', 'Create a new project space to organize task context, logs, and supporting material.')}</p>
            </div>
          </div>
          <form onSubmit={handleCreate}>
            <div className="form-group">
              <label htmlFor="proj-name">{tx('项目名称', 'Project name')}</label>
              <input
                id="proj-name"
                type="text"
                value={newName}
                onChange={(event) => setNewName(event.target.value)}
                placeholder={tx('例如：blog-redesign', 'For example: blog-redesign')}
                disabled={creating}
                autoFocus
              />
            </div>
            <div className="form-actions">
              <button type="submit" className="btn btn-primary" disabled={creating}>
                {creating ? tx('创建中...', 'Creating...') : tx('创建', 'Create')}
              </button>
              <button type="button" className="btn" onClick={() => setShowNewForm(false)} disabled={creating}>
                {tx('取消', 'Cancel')}
              </button>
            </div>
          </form>
        </div>
      )}

      <section className="materials-section">
        <div className="materials-section-head">
          <div>
            <h3 className="materials-section-title">Project Library</h3>
            <p className="materials-section-copy">{tx('统一浏览项目卡片，打开后进入 project bundle。', 'Browse project cards here, then open one into its project bundle.')}</p>
          </div>
          <MaterialsSectionToolbar
            count={filteredProjects.length}
            sortKey={sortKey}
            sortOptions={sortOptions}
            sortDir={sortDir}
            onSortKeyChange={(value) => setSortKey(value as MaterialsSortKey)}
            onSortDirToggle={() => setSortDir((value) => (value === 'desc' ? 'asc' : 'desc'))}
          >
            <button className="btn btn-sm materials-toolbar-control" onClick={() => setShowNewForm((value) => !value)}>
              {showNewForm ? tx('取消新建', 'Close form') : tx('新建项目', 'New project')}
            </button>
            <button
              className="btn btn-sm materials-toolbar-control is-danger"
              disabled={selectedProject?.status !== 'active'}
              onClick={() => {
                if (selectedProject) requestArchive(selectedProject)
              }}
            >
              {tx('归档项目', 'Archive project')}
            </button>
          </MaterialsSectionToolbar>
        </div>

        {(sourceOptions.length > 1 || sourceFilter !== 'all') && (
          <SourceFilterBar options={sourceOptions} value={sourceFilter} onChange={setSourceFilter} />
        )}

        {filteredProjects.length === 0 ? (
          <div className="empty-state">
            <p>{tx('暂无项目', 'No projects yet')}</p>
            <p className="empty-hint">{tx('项目帮助 Agent 组织不同任务的上下文和进度。', 'Projects help agents organize context and progress across different tasks.')}</p>
          </div>
        ) : (
          <div className="materials-grid materials-grid-wide">
            {filteredProjects.map((project) => {
              const source = projectSource(project)
              return (
                <MaterialsTile
                  key={project.name}
                  iconClassName="icon-folder"
                  title={project.name}
                  titleActionAriaLabel={tx(`打开项目 ${project.name} 的 bundle 目录`, `Open the ${project.name} bundle folder`)}
                  subtitle={`${tx('项目 bundle', 'Project bundle')} · ${project.status}`}
                  description={projectDescription(project)}
                  path={`/projects/${project.name}/`}
                  pills={(
                    <>
                      {source ? (
                        <span className="materials-tile-pill materials-source-pill">
                          {sourceLabel(source, locale)}
                        </span>
                      ) : null}
                      {projectCapabilities(project).map((capability) => (
                        <span key={capability} className="materials-tile-pill">
                          {bundleCapabilityLabel(capability, locale)}
                        </span>
                      ))}
                    </>
                  )}
                  footerStart={tx('最后活动', 'Last activity')}
                  footerEnd={formatTime(getProjectLastActivity(project))}
                  selected={selectedProjectName === project.name}
                  menuOpen={isMenuOpen(project.name)}
                  menuButtonAriaLabel={tx(`打开项目 ${project.name} 的工具菜单`, `Open tools menu for project ${project.name}`)}
                  menuPanel={(
                    <ResourceActionMenu
                      items={[
                        {
                          key: 'bundle',
                          label: tx('打开 bundle', 'Open bundle'),
                          onSelect: () => {
                            closeMenu()
                            openProjectBundle(project.name)
                          },
                        },
                        {
                          key: 'open',
                          label: tx('打开 context', 'Open context'),
                          onSelect: () => {
                            closeMenu()
                            navigate(dataFileEditorRoute(project.primary_path || `/projects/${project.name}/context.md`))
                          },
                        },
                        {
                          key: 'log',
                          label: tx('打开日志', 'Open log'),
                          onSelect: () => {
                            closeMenu()
                            navigate(dataFileEditorRoute(project.log_path || `/projects/${project.name}/log.jsonl`))
                          },
                        },
                        {
                          key: 'select',
                          label: selectedProjectName === project.name ? tx('取消选中', 'Unselect') : tx('加入选择', 'Select'),
                          onSelect: () => {
                            closeMenu()
                            setSelectedProjectName((value) => (value === project.name ? null : project.name))
                          },
                        },
                        ...(project.status === 'active'
                          ? [{
                              key: 'archive',
                              label: tx('归档项目', 'Archive project'),
                              tone: 'danger' as const,
                              onSelect: () => requestArchive(project),
                            }]
                          : []),
                      ]}
                    />
                  )}
                  onMenuToggle={() => toggleMenu(project.name)}
                  onSelect={() => setSelectedProjectName((value) => (value === project.name ? null : project.name))}
                  onOpen={() => openProjectBundle(project.name)}
                >
                  <div className="project-bundle-paths">
                    <div className="project-bundle-path-row">
                      <span className="project-bundle-path-label">{tx('主文件', 'Primary')}</span>
                      <code className="project-bundle-path-value">{project.primary_path || `/projects/${project.name}/context.md`}</code>
                    </div>
                    <div className="project-bundle-path-row">
                      <span className="project-bundle-path-label">{tx('Log', 'Log')}</span>
                      <code className="project-bundle-path-value">{project.log_path || `/projects/${project.name}/log.jsonl`}</code>
                    </div>
                  </div>
                </MaterialsTile>
              )
            })}
          </div>
        )}
      </section>

      <ResourceConfirmDialog
        open={Boolean(archiveTarget)}
        kicker={tx('危险操作', 'Danger zone')}
        title={archiveTarget ? tx(`确认归档项目 “${archiveTarget.name}”`, `Archive project "${archiveTarget.name}"`) : ''}
        description={tx('归档不会删除项目文件，但项目会退出活跃状态。你之后仍然可以在文件树里查看它。', 'Archiving does not delete project files, but the project leaves the active state. You can still view it in the file tree later.')}
        cancelLabel={tx('取消', 'Cancel')}
        confirmLabel={archiveSubmitting ? tx('归档中...', 'Archiving...') : tx('确认归档', 'Archive')}
        tone="danger"
        submitting={archiveSubmitting}
        onCancel={closeArchiveDialog}
        onConfirm={() => {
          if (archiveTarget) void handleArchive(archiveTarget)
        }}
      />
    </div>
  )
}
