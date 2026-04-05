import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type FileNode } from '../api'

interface Stats {
  connections?: number
  skills?: number
  devices?: number
  projects?: number
  weekly_activity?: { platform: string; count: number }[]
  pending?: { type: string; count: number; message: string }[]
}

interface UserProfileData {
  user_id?: string
  display_name?: string
  preferences?: Record<string, string>
  updated_at?: string
}

interface ProjectSummary {
  name: string
  status: string
  description?: string
  last_activity?: string
  updated_at?: string
}

const PROFILE_SECTIONS = [
  { key: 'preferences', label: '个人偏好' },
  { key: 'relationships', label: '人际关系' },
  { key: 'principles', label: '行为准则' },
]

function formatDateTime(ts?: string) {
  if (!ts) return '-'
  try {
    return new Date(ts).toLocaleString('zh-CN')
  } catch {
    return ts
  }
}

function formatProfilePreview(content?: string) {
  if (!content) return '还没有内容'
  return content
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .slice(0, 2)
    .join(' / ')
}

function summarizeFileContent(node: FileNode) {
  if (!node.content) return '目录或空文件'
  return node.content
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 120) || '空内容'
}

export default function DashboardPage() {
  const [stats, setStats] = useState<Stats>({})
  const [profile, setProfile] = useState<UserProfileData | null>(null)
  const [projects, setProjects] = useState<ProjectSummary[]>([])
  const [rootTree, setRootTree] = useState<FileNode | null>(null)
  const [recentFiles, setRecentFiles] = useState<FileNode[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [exporting, setExporting] = useState(false)
  const [exportError, setExportError] = useState('')
  const [exportSuccess, setExportSuccess] = useState('')

  useEffect(() => {
    loadDashboard()
  }, [])

  const loadDashboard = async () => {
    try {
      const [statsData, profileData, projectData, rootTreeData, snapshotData] = await Promise.allSettled([
        api.getStats(),
        api.getProfile(),
        api.getProjects(),
        api.getTree('/'),
        api.getTreeSnapshot('/'),
      ])

      if (statsData.status === 'fulfilled') {
        setStats(statsData.value)
      } else {
        setError(statsData.reason?.message || '加载概览失败')
      }

      if (profileData.status === 'fulfilled') {
        setProfile(profileData.value || null)
      }

      if (projectData.status === 'fulfilled') {
        const sortedProjects = [...(projectData.value || [])].sort((a, b) => {
          const aTime = new Date(a.last_activity || a.updated_at || 0).getTime()
          const bTime = new Date(b.last_activity || b.updated_at || 0).getTime()
          return bTime - aTime
        })
        setProjects(sortedProjects.slice(0, 5))
      }

      if (rootTreeData.status === 'fulfilled') {
        setRootTree(rootTreeData.value || null)
      }

      if (snapshotData.status === 'fulfilled') {
        const files = (snapshotData.value?.entries || [])
          .filter((entry) => !entry.is_dir)
          .sort((a, b) => {
            const aTime = new Date(a.updated_at || a.created_at || 0).getTime()
            const bTime = new Date(b.updated_at || b.created_at || 0).getTime()
            return bTime - aTime
          })
          .slice(0, 6)
        setRecentFiles(files)
      }
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleExportZip = async () => {
    setExporting(true)
    setExportError('')
    setExportSuccess('')
    try {
      await api.exportZip()
      setExportSuccess('ZIP 文件已开始下载。')
    } catch (err: any) {
      setExportError(err.message || '导出失败')
    } finally {
      setExporting(false)
    }
  }

  const handleExportJSON = async () => {
    setExporting(true)
    setExportError('')
    setExportSuccess('')
    try {
      const data = await api.exportJSON()
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `agenthub-export-${new Date().toISOString().slice(0, 10)}.json`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
      setExportSuccess('JSON 文件已开始下载。')
    } catch (err: any) {
      setExportError(err.message || '导出失败')
    } finally {
      setExporting(false)
    }
  }

  if (loading) {
    return <div className="page-loading">加载中...</div>
  }

  const hasPending = stats.pending && stats.pending.length > 0
  const rootDirectories = (rootTree?.children || []).filter((entry) => entry.is_dir).slice(0, 8)
  const profileExtras = Object.entries(profile?.preferences || {}).filter(([key]) =>
    !PROFILE_SECTIONS.some((section) => section.key === key),
  )

  return (
    <div className="page">
      <div className="page-header">
        <h2>概览</h2>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}

      <div className="status-banner">
        <span className="status-icon status-ok">&#10003;</span>
        <span className="status-text">
          {hasPending ? '有待处理事项' : '一切正常'}
        </span>
      </div>

      <div className="stats-grid">
        <Link to="/connections" className="stat-card">
          <div className="stat-value">{stats.connections ?? '-'}</div>
          <div className="stat-label">已连接平台</div>
        </Link>
        <div className="stat-card">
          <div className="stat-value">{stats.skills ?? '-'}</div>
          <div className="stat-label">可用技能</div>
        </div>
        <div className="stat-card">
          <div className="stat-value">{stats.devices ?? '-'}</div>
          <div className="stat-label">设备</div>
        </div>
        <Link to="/projects" className="stat-card">
          <div className="stat-value">{stats.projects ?? '-'}</div>
          <div className="stat-label">活跃项目</div>
        </Link>
      </div>

      <div className="dashboard-content-grid">
        <div className="card dashboard-card">
          <div className="card-header">
            <h3 className="card-title">我的资料</h3>
            <Link to="/info" className="dashboard-card-link">编辑资料</Link>
          </div>

          <div className="dashboard-profile-head">
            <div>
              <div className="dashboard-profile-name">{profile?.display_name || '未设置显示名称'}</div>
              <div className="dashboard-profile-meta">
                Agent Hub 已记录你的偏好、关系和行为准则，Agent 会优先参考这些内容。
              </div>
            </div>
          </div>

          <div className="dashboard-profile-list">
            {PROFILE_SECTIONS.map((section) => (
              <div key={section.key} className="dashboard-profile-item">
                <div className="dashboard-profile-label">{section.label}</div>
                <div className="dashboard-profile-value">
                  {formatProfilePreview(profile?.preferences?.[section.key])}
                </div>
              </div>
            ))}
          </div>

          {profileExtras.length > 0 && (
            <div className="dashboard-inline-list">
              {profileExtras.map(([key, value]) => (
                <span key={key} className="dashboard-inline-chip" title={value}>
                  {key}
                </span>
              ))}
            </div>
          )}
        </div>

        <div className="card dashboard-card">
          <div className="card-header">
            <h3 className="card-title">Hub 文件</h3>
            <span className="dashboard-card-link dashboard-card-link-muted">显示最近内容</span>
          </div>

          <div className="dashboard-subtitle">顶层目录</div>
          {rootDirectories.length > 0 ? (
            <div className="dashboard-inline-list">
              {rootDirectories.map((entry) => (
                <span key={entry.path} className="dashboard-inline-chip">
                  {entry.path}
                </span>
              ))}
            </div>
          ) : (
            <p className="dashboard-empty-copy">还没有目录内容。</p>
          )}

          <div className="dashboard-subtitle" style={{ marginTop: 16 }}>最近文件</div>
          {recentFiles.length > 0 ? (
            <div className="dashboard-file-list">
              {recentFiles.map((entry) => (
                <div key={entry.path} className="dashboard-file-item">
                  <div className="dashboard-file-path">{entry.path}</div>
                  <div className="dashboard-file-preview">{summarizeFileContent(entry)}</div>
                  <div className="dashboard-file-meta">{formatDateTime(entry.updated_at || entry.created_at)}</div>
                </div>
              ))}
            </div>
          ) : (
            <p className="dashboard-empty-copy">还没有写入任何文件内容。</p>
          )}
        </div>
      </div>

      {projects.length > 0 && (
        <div className="card dashboard-card">
          <div className="card-header">
            <h3 className="card-title">最近项目</h3>
            <Link to="/projects" className="dashboard-card-link">查看全部</Link>
          </div>

          <div className="dashboard-project-list">
            {projects.map((project) => (
              <Link key={project.name} to="/projects" className="dashboard-project-item">
                <div className="dashboard-project-main">
                  <div className="dashboard-project-name">{project.name}</div>
                  {project.description && (
                    <div className="dashboard-project-desc">{project.description}</div>
                  )}
                </div>
                <div className="dashboard-project-meta">
                  <span className={`badge ${project.status === 'active' ? 'badge-active' : 'badge-archived'}`}>
                    {project.status === 'active' ? '进行中' : project.status}
                  </span>
                  <span>{formatDateTime(project.last_activity || project.updated_at)}</span>
                </div>
              </Link>
            ))}
          </div>
        </div>
      )}

      {hasPending && (
        <div className="card">
          <h3 className="card-title">待处理</h3>
          <div className="pending-list">
            {stats.pending!.map((item, i) => (
              <div key={i} className="pending-item">
                <span className="pending-badge">{item.count}</span>
                <span className="pending-message">{item.message}</span>
                <span className="pending-type">{item.type}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {stats.weekly_activity && stats.weekly_activity.length > 0 && (
        <div className="card">
          <h3 className="card-title">本周活动</h3>
          <div className="activity-list">
            {stats.weekly_activity.map((item, i) => (
              <div key={i} className="activity-row">
                <span className="activity-platform">{item.platform}</span>
                <div className="activity-bar-container">
                  <div
                    className="activity-bar"
                    style={{
                      width: `${Math.min(100, (item.count / Math.max(...stats.weekly_activity!.map((activity) => activity.count))) * 100)}%`,
                    }}
                  />
                </div>
                <span className="activity-count">{item.count}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="card">
        <h3 className="card-title">快捷入口</h3>
        <div className="quick-links">
          <Link to="/connections" className="quick-link">
            <span className="quick-link-icon">&#9670;</span>
            <span>管理连接</span>
          </Link>
          <Link to="/info" className="quick-link">
            <span className="quick-link-icon">&#9733;</span>
            <span>个人偏好</span>
          </Link>
          <Link to="/projects" className="quick-link">
            <span className="quick-link-icon">&#9654;</span>
            <span>查看项目</span>
          </Link>
          <Link to="/setup/web-apps" className="quick-link">
            <span className="quick-link-icon">&#9889;</span>
            <span>连接设置</span>
          </Link>
        </div>
      </div>

      <div className="card">
        <h3 className="card-title">数据管理</h3>
        <p style={{ marginBottom: '1rem', color: 'var(--color-text-secondary, #888)' }}>
          下载你所有的 Hub 数据，包括资料、文件树、技能、项目、设备和记忆。
        </p>
        <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap' }}>
          <button
            className="btn btn-primary"
            disabled={exporting}
            onClick={handleExportZip}
          >
            {exporting ? '导出中...' : '导出数据 (ZIP)'}
          </button>
          <button
            className="btn"
            disabled={exporting}
            onClick={handleExportJSON}
          >
            导出数据 (JSON)
          </button>
        </div>
        {exportError && <div className="alert alert-warn" style={{ marginTop: '0.75rem' }}>{exportError}</div>}
        {exportSuccess && <div className="alert alert-ok" style={{ marginTop: '0.75rem' }}>{exportSuccess}</div>}
      </div>
    </div>
  )
}
