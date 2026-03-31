import { useState, useEffect } from 'react'
import { api } from '../api'

interface Project {
  name: string
  status: string
  description?: string
  last_activity?: string
  context_md?: string
  logs?: LogEntry[]
}

interface LogEntry {
  timestamp: string
  level?: string
  message: string
  source?: string
}

export default function ProjectsPage() {
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selectedProject, setSelectedProject] = useState<Project | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [showNewForm, setShowNewForm] = useState(false)
  const [newName, setNewName] = useState('')
  const [creating, setCreating] = useState(false)

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
      setSelectedProject(data)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setDetailLoading(false)
    }
  }

  const handleSelectProject = (project: Project) => {
    if (selectedProject?.name === project.name) {
      setSelectedProject(null)
    } else {
      loadProjectDetail(project.name)
    }
  }

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newName.trim()) return

    setCreating(true)
    setError('')

    try {
      const result = await api.createConnection({
        name: newName.trim(),
        type: 'project',
      })
      setProjects((prev) => [...prev, result])
      setNewName('')
      setShowNewForm(false)
      loadProjects()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  const formatTime = (ts?: string) => {
    if (!ts) return '-'
    try {
      return new Date(ts).toLocaleString('zh-CN')
    } catch {
      return ts
    }
  }

  const getStatusClass = (status: string) => {
    switch (status?.toLowerCase()) {
      case 'active':
        return 'status-active'
      case 'archived':
        return 'status-archived'
      case 'paused':
        return 'status-paused'
      default:
        return ''
    }
  }

  const getStatusLabel = (status: string) => {
    switch (status?.toLowerCase()) {
      case 'active':
        return '进行中'
      case 'archived':
        return '已归档'
      case 'paused':
        return '已暂停'
      default:
        return status || '未知'
    }
  }

  if (loading) {
    return <div className="page-loading">加载中...</div>
  }

  return (
    <div className="page">
      <div className="page-header">
        <h2>项目</h2>
        <div className="page-actions">
          <button className="btn btn-primary" onClick={() => setShowNewForm(true)}>
            新建项目
          </button>
        </div>
      </div>

      {error && <div className="alert alert-error">{error}</div>}

      {showNewForm && (
        <div className="card form-card">
          <h3 className="card-title">新建项目</h3>
          <form onSubmit={handleCreate}>
            <div className="form-group">
              <label htmlFor="proj-name">项目名称</label>
              <input
                id="proj-name"
                type="text"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                placeholder="例如：blog-redesign"
                disabled={creating}
                autoFocus
              />
            </div>
            <div className="form-actions">
              <button type="submit" className="btn btn-primary" disabled={creating}>
                {creating ? '创建中...' : '创建'}
              </button>
              <button
                type="button"
                className="btn"
                onClick={() => setShowNewForm(false)}
                disabled={creating}
              >
                取消
              </button>
            </div>
          </form>
        </div>
      )}

      {projects.length === 0 ? (
        <div className="empty-state">
          <p>暂无项目</p>
          <p className="empty-hint">项目帮助 Agent 组织不同任务的上下文和进度</p>
        </div>
      ) : (
        <div className="project-list">
          {projects.map((project) => (
            <div key={project.name} className="project-item">
              <div
                className={`card project-card ${
                  selectedProject?.name === project.name ? 'selected' : ''
                }`}
                onClick={() => handleSelectProject(project)}
              >
                <div className="project-header">
                  <span className="project-name">{project.name}</span>
                  <span className={`project-status ${getStatusClass(project.status)}`}>
                    {getStatusLabel(project.status)}
                  </span>
                </div>
                {project.description && (
                  <p className="project-desc">{project.description}</p>
                )}
                <div className="project-meta">
                  <span>最后活动：{formatTime(project.last_activity)}</span>
                </div>
              </div>

              {selectedProject?.name === project.name && (
                <div className="project-detail">
                  {detailLoading ? (
                    <div className="page-loading">加载详情...</div>
                  ) : (
                    <>
                      {selectedProject.context_md && (
                        <div className="card">
                          <h4 className="card-title">context.md</h4>
                          <pre className="context-content">
                            {selectedProject.context_md}
                          </pre>
                        </div>
                      )}

                      {selectedProject.logs && selectedProject.logs.length > 0 && (
                        <div className="card">
                          <h4 className="card-title">最近日志</h4>
                          <div className="log-timeline">
                            {selectedProject.logs.map((log, i) => (
                              <div key={i} className="log-entry">
                                <div className="log-time">
                                  {formatTime(log.timestamp)}
                                </div>
                                <div className="log-dot" />
                                <div className="log-content">
                                  {log.source && (
                                    <span className="log-source">{log.source}</span>
                                  )}
                                  <span className="log-message">{log.message}</span>
                                </div>
                              </div>
                            ))}
                          </div>
                        </div>
                      )}

                      {!selectedProject.context_md &&
                        (!selectedProject.logs || selectedProject.logs.length === 0) && (
                          <div className="empty-state">
                            <p>暂无项目详情</p>
                          </div>
                        )}
                    </>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
