import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import MaterialsSectionToolbar from '../components/MaterialsSectionToolbar'
import MaterialsTile from '../components/MaterialsTile'
import ResourceActionMenu from '../components/ResourceActionMenu'
import ResourceConfirmDialog from '../components/ResourceConfirmDialog'
import SourceFilterBar from '../components/SourceFilterBar'
import useResourceCardMenu from '../hooks/useResourceCardMenu'
import { useI18n } from '../i18n'
import {
  buildSourceFilterOptions,
  getMaterialsSortOptions,
  matchesSourceFilter,
  projectSource,
  sourceLabel,
  type MaterialsSortDir,
  type MaterialsSortKey,
  dataFileEditorRoute,
  sortMaterialsItems,
} from './data/DataShared'

interface Project {
  name: string
  status: string
  description?: string
  last_activity?: string
  updated_at?: string
  context_md?: string
  metadata?: Record<string, any>
  logs?: LogEntry[]
}

interface LogEntry {
  timestamp: string
  level?: string
  message: string
  source?: string
  action?: string
  summary?: string
  tags?: string[]
}

export default function ProjectsPage() {
  const { locale, tx } = useI18n()
  const navigate = useNavigate()
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selectedProject, setSelectedProject] = useState<Project | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [showNewForm, setShowNewForm] = useState(false)
  const [newName, setNewName] = useState('')
  const [creating, setCreating] = useState(false)
  const [sortKey, setSortKey] = useState<MaterialsSortKey>('updated_at')
  const [sortDir, setSortDir] = useState<MaterialsSortDir>('desc')
  const [sourceFilter, setSourceFilter] = useState('all')
  const [archiveTarget, setArchiveTarget] = useState<Project | null>(null)
  const [archiveSubmitting, setArchiveSubmitting] = useState(false)
  const { activeMenuId, closeMenu, isMenuOpen, toggleMenu } = useResourceCardMenu()

  useEffect(() => {
    loadProjects()
  }, [])

  const loadProjects = async () => {
    try {
      const data = await api.getProjects()
      setProjects(data || [])
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const loadProjectDetail = async (name: string) => {
    setDetailLoading(true)
    try {
      const data = await api.getProject(name)
      // API returns {project: {...}, logs: [...]} — flatten into a single Project object
      const proj = data.project || data
      setSelectedProject({
        ...proj,
        logs: data.logs || [],
      })
    } catch (err: any) {
      setError(err.message)
    } finally {
      setDetailLoading(false)
    }
  }

  const handleSelectProject = (project: Project) => {
    if (selectedProject?.name === project.name) return
    void loadProjectDetail(project.name)
  }

  const projectContextRoute = (name: string) => dataFileEditorRoute(projectContextPath(name))

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newName.trim()) return

    setCreating(true)
    setError('')

    try {
      await api.createProject(newName.trim())
      setNewName('')
      setShowNewForm(false)
      loadProjects()
    } catch (err: any) {
      setError(err.message)
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
      setProjects((prev) =>
        prev.map((p) => (p.name === project.name ? { ...p, status: 'archived' } : p))
      )
      if (selectedProject?.name === project.name) {
        setSelectedProject({ ...selectedProject, status: 'archived' })
      }
      setArchiveTarget(null)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setArchiveSubmitting(false)
    }
  }

  const formatTime = (ts?: string) => {
    if (!ts) return '-'
    try {
      return new Date(ts).toLocaleString(locale === 'zh-CN' ? 'zh-CN' : 'en-US')
    } catch {
      return ts
    }
  }

  const getStatusLabel = (status: string) => {
    switch (status?.toLowerCase()) {
      case 'active':
        return tx('进行中', 'Active')
      case 'archived':
        return tx('已归档', 'Archived')
      case 'paused':
        return tx('已暂停', 'Paused')
      default:
        return status || tx('未知', 'Unknown')
    }
  }

  const sortOptions = getMaterialsSortOptions(locale)

  const getProjectLastActivity = (project: Project) => project.last_activity || project.updated_at
  const projectContextPath = (name: string) => `/projects/${name}/context.md`
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

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (archiveTarget || activeMenuId) return
      if (event.key === 'Escape') {
        setSelectedProject(null)
        return
      }
      if (event.key === 'Delete' && selectedProject?.status === 'active') {
        event.preventDefault()
        requestArchive(selectedProject)
        return
      }
      if (event.key === 'Enter' && selectedProject) {
        navigate(projectContextRoute(selectedProject.name))
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [activeMenuId, archiveTarget, navigate, selectedProject])

  if (loading) {
    return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>
  }

  return (
    <div className="page materials-page">
      <section className="materials-hero">
        <div className="materials-hero-copy">
          <div className="materials-kicker">neuDrive Data</div>
          <h2 className="materials-title">{tx('项目', 'Projects')}</h2>
          <p className="materials-subtitle">{tx('把项目看成一组长期上下文卡片。点击任意项目卡片，可以继续查看 context 和最近日志。', 'Treat projects as long-lived context cards. Open any project card to inspect its context and recent logs.')}</p>
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
                onChange={(e) => setNewName(e.target.value)}
                placeholder={tx('例如：blog-redesign', 'For example: blog-redesign')}
                disabled={creating}
                autoFocus
              />
            </div>
            <div className="form-actions">
              <button type="submit" className="btn btn-primary" disabled={creating}>
                {creating ? tx('创建中...', 'Creating...') : tx('创建', 'Create')}
              </button>
              <button
                type="button"
                className="btn"
                onClick={() => setShowNewForm(false)}
                disabled={creating}
              >
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
            <p className="materials-section-copy">{tx('统一浏览项目卡片，选中后在下方查看 context 和日志。', 'Browse project cards here, then inspect context and logs below.')}</p>
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
            {filteredProjects.map((project) => (
                <MaterialsTile
                  key={project.name}
                  iconClassName="icon-folder"
                  title={project.name}
                  titleActionAriaLabel={tx(`打开项目 ${project.name} 的 context.md`, `Open ${project.name} context.md`)}
                  subtitle={`${getStatusLabel(project.status)} · ${sourceLabel(projectSource(project), locale)}`}
                  description={project.description || tx('这个项目还没有补充描述。', 'This project does not have a description yet.')}
                  path={projectContextPath(project.name)}
                  footerStart={tx('最后活动', 'Last activity')}
                  footerEnd={formatTime(getProjectLastActivity(project))}
                  selected={selectedProject?.name === project.name}
                  menuOpen={isMenuOpen(project.name)}
                  menuButtonAriaLabel={tx(`打开项目 ${project.name} 的工具菜单`, `Open tools menu for project ${project.name}`)}
                  menuPanel={(
                    <ResourceActionMenu
                      items={[
                        {
                          key: 'open',
                          label: tx('打开 context', 'Open context'),
                          onSelect: () => {
                            closeMenu()
                            navigate(projectContextRoute(project.name))
                          },
                        },
                        {
                          key: 'select',
                          label: selectedProject?.name === project.name ? tx('取消选中', 'Unselect') : tx('加入选择', 'Select'),
                          onSelect: () => {
                            closeMenu()
                            if (selectedProject?.name === project.name) {
                              setSelectedProject(null)
                              return
                            }
                            handleSelectProject(project)
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
                  onSelect={() => handleSelectProject(project)}
                  onOpen={() => navigate(projectContextRoute(project.name))}
                />
              ))}
          </div>
        )}
      </section>

      {selectedProject && (
        <section className="materials-section">
          <div className="materials-section-head">
            <div>
              <h3 className="materials-section-title">{selectedProject.name}</h3>
              <p className="materials-section-copy">{tx('项目详情会在这里显示，包括 context 和最近日志。', 'Project details appear here, including context and recent logs.')}</p>
            </div>
              <MaterialsSectionToolbar>
              {selectedProject.status === 'active' ? (
                <button className="btn btn-sm materials-toolbar-control" onClick={() => requestArchive(selectedProject)}>
                  {tx('归档项目', 'Archive project')}
                </button>
              ) : null}
            </MaterialsSectionToolbar>
          </div>
          <div className="project-detail">
            {detailLoading ? (
              <div className="page-loading">{tx('加载详情...', 'Loading details...')}</div>
            ) : (
              <>
                {selectedProject.context_md && (
                  <div className="materials-panel">
                    <h4 className="card-title">context.md</h4>
                    <pre className="context-content">
                      {selectedProject.context_md}
                    </pre>
                  </div>
                )}

                {selectedProject.logs && selectedProject.logs.length > 0 && (
                  <div className="materials-panel">
                    <h4 className="card-title">{tx('最近日志', 'Recent logs')}</h4>
                    <div className="log-timeline">
                      {selectedProject.logs.map((log, i) => (
                        <div key={i} className="timeline-item">
                          <div className="time">
                            {formatTime(log.timestamp)}
                            {log.source && (
                              <span className="source" style={{ marginLeft: 8 }}>{log.source}</span>
                            )}
                          </div>
                          {log.action && (
                            <div style={{ fontSize: 13, fontWeight: 500, marginTop: 2 }}>{log.action}</div>
                          )}
                          <div className="summary">
                            {log.summary || log.message}
                          </div>
                          {log.tags && log.tags.length > 0 && (
                            <div className="tags">
                              {log.tags.map((tag, j) => (
                                <span key={j} className="tag">{tag}</span>
                              ))}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {!selectedProject.context_md &&
                  (!selectedProject.logs || selectedProject.logs.length === 0) && (
                    <div className="empty-state">
                      <p>{tx('暂无项目详情', 'No project details yet')}</p>
                    </div>
                  )}
              </>
            )}
          </div>
        </section>
      )}

      <ResourceConfirmDialog
        open={Boolean(archiveTarget)}
        kicker={tx('危险操作', 'Danger zone')}
        title={archiveTarget ? tx(`确认归档项目 “${archiveTarget.name}”`, `Archive project "${archiveTarget.name}"`) : ''}
        description={tx('归档不会删除项目文件，但项目会退出活跃状态。你之后仍然可以在文件树里查看它的 context 和历史内容。', 'Archiving does not delete project files, but the project will leave the active state. You can still inspect its context and history from the file tree later.')}
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
