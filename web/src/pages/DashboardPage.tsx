import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'

interface Stats {
  connections?: number
  skills?: number
  devices?: number
  projects?: number
  weekly_activity?: { platform: string; count: number }[]
  pending?: { type: string; count: number; message: string }[]
}

export default function DashboardPage() {
  const [stats, setStats] = useState<Stats>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [exporting, setExporting] = useState(false)
  const [exportError, setExportError] = useState('')
  const [exportSuccess, setExportSuccess] = useState('')

  useEffect(() => {
    loadStats()
  }, [])

  const loadStats = async () => {
    try {
      const data = await api.getStats()
      setStats(data)
    } catch (err: any) {
      setError(err.message)
      // Fall back to loading individual counts
      try {
        const [connections, projects, devices] = await Promise.allSettled([
          api.getConnections(),
          api.getProjects(),
          api.getDevices(),
        ])
        setStats({
          connections: connections.status === 'fulfilled' ? connections.value.length : 0,
          projects: projects.status === 'fulfilled' ? projects.value.length : 0,
          devices: devices.status === 'fulfilled' ? devices.value.length : 0,
        })
        setError('')
      } catch {
        // Keep the original error
      }
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
                      width: `${Math.min(100, (item.count / Math.max(...stats.weekly_activity!.map(a => a.count))) * 100)}%`,
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
        </div>
      </div>

      <div className="card">
        <h3 className="card-title">数据管理</h3>
        <p style={{ marginBottom: '1rem', color: 'var(--color-text-secondary, #888)' }}>
          下载你所有的 Hub 数据（技能、记忆、项目、设备、角色等）。
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
