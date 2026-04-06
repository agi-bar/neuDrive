import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type DashboardStats, type FileNode } from '../api'
import {
  formatDateTime,
  isProfileEntry,
  isVisibleFileEntry,
  isProfilePreviewEntry,
  profileLabelFromPath,
  sortNodesByRecent,
  summarizeNodeContent,
} from './data/DataShared'

interface UserProfileData {
  user_id?: string
  display_name?: string
  preferences?: Record<string, string>
  updated_at?: string
}

const DASHBOARD_STATS = [
  { key: 'connections', label: '已连接平台', to: '/connections' },
  { key: 'files', label: '所有文件', to: '/data/files' },
  { key: 'projects', label: '项目', to: '/data/projects' },
  { key: 'skills', label: '技能', to: '/data/skills' },
  { key: 'memory', label: 'Memory', to: '/data/memory' },
  { key: 'profile', label: '我的资料', to: '/data/profile' },
  { key: 'devices', label: '设备', to: '/data/devices' },
  { key: 'inbox', label: 'Inbox', to: '/data/inbox' },
] as const

export default function DashboardPage() {
  const [stats, setStats] = useState<DashboardStats>({
    connections: 0,
    files: 0,
    projects: 0,
    skills: 0,
    memory: 0,
    profile: 0,
    devices: 0,
    inbox: 0,
    weekly_activity: [],
    pending: [],
  })
  const [profile, setProfile] = useState<UserProfileData | null>(null)
  const [recentProfileEntries, setRecentProfileEntries] = useState<FileNode[]>([])
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
      const [statsData, profileData, profileSnapshotData, rootSnapshotData] = await Promise.allSettled([
        api.getStats(),
        api.getProfile(),
        api.getTreeSnapshot('/memory/profile'),
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

      if (profileSnapshotData.status === 'fulfilled') {
        const entries = sortNodesByRecent(profileSnapshotData.value.entries.filter(isProfilePreviewEntry)).slice(0, 2)
        setRecentProfileEntries(entries)
      }

      if (rootSnapshotData.status === 'fulfilled') {
        const files = sortNodesByRecent(
          rootSnapshotData.value.entries.filter((entry) => isVisibleFileEntry(entry) && !isProfileEntry(entry)),
        ).slice(0, 2)
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
        {DASHBOARD_STATS.map((item) => (
          <Link key={item.key} to={item.to} className="stat-card">
            <div className="stat-value">{stats[item.key] ?? '-'}</div>
            <div className="stat-label">{item.label}</div>
          </Link>
        ))}
      </div>

      <div className="dashboard-content-grid">
        <div className="card dashboard-card">
          <div className="card-header">
            <h3 className="card-title">我的资料</h3>
            <Link to="/data/profile" className="dashboard-card-link">更多</Link>
          </div>

          <div className="dashboard-profile-head">
            <div>
              <div className="dashboard-profile-name">{profile?.display_name || '未设置显示名称'}</div>
              <div className="dashboard-profile-meta">
                首页只显示最近更新的 2 项资料，完整内容请到“我的资料”页面查看和编辑。
              </div>
            </div>
          </div>

          {recentProfileEntries.length > 0 ? (
            <div className="dashboard-profile-list">
              {recentProfileEntries.map((entry) => (
                <div key={entry.path} className="dashboard-profile-item">
                  <div className="dashboard-profile-label">{profileLabelFromPath(entry.path)}</div>
                  <div className="dashboard-profile-value">{summarizeNodeContent(entry, 120)}</div>
                  <div className="dashboard-profile-item-meta">{formatDateTime(entry.updated_at || entry.created_at)}</div>
                </div>
              ))}
            </div>
          ) : (
            <p className="dashboard-empty-copy">还没有资料内容。</p>
          )}
        </div>

        <div className="card dashboard-card">
          <div className="card-header">
            <h3 className="card-title">Hub 文件</h3>
            <Link to="/data/files" className="dashboard-card-link">更多</Link>
          </div>

          <div className="dashboard-profile-meta dashboard-preview-meta">
            首页只显示最近更新的 2 个 Hub 文件，完整列表请到“所有文件”页面查看。
          </div>

          {recentFiles.length > 0 ? (
            <div className="dashboard-file-list">
              {recentFiles.map((entry) => (
                <div key={entry.path} className="dashboard-file-item">
                  <div className="dashboard-file-path">{entry.path}</div>
                  <div className="dashboard-file-preview">{summarizeNodeContent(entry, 140)}</div>
                  <div className="dashboard-file-meta">{formatDateTime(entry.updated_at || entry.created_at)}</div>
                </div>
              ))}
            </div>
          ) : (
            <p className="dashboard-empty-copy">还没有文件内容。</p>
          )}
        </div>
      </div>

      {hasPending && (
        <div className="card">
          <h3 className="card-title">待处理</h3>
          <div className="pending-list">
            {stats.pending.map((item, i) => (
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
                      width: `${Math.min(100, (item.count / Math.max(...stats.weekly_activity.map((activity) => activity.count))) * 100)}%`,
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
        <h3 className="card-title">数据管理</h3>
        <p style={{ marginBottom: '1rem', color: 'var(--color-text-secondary, #888)' }}>
          下载你所有的 Hub 数据，或进入新的 Bundle Sync 页面执行 `.ahub` / `.ahubz` 的导入、导出和历史查看。
        </p>
        <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap' }}>
          <Link to="/data/sync" className="btn">
            打开 Bundle Sync
          </Link>
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
