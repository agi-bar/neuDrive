import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type DashboardStats, type FileNode } from '../api'
import { useI18n } from '../i18n'
import {
  formatDateTime,
  isProfileEntry,
  isVisibleFileEntry,
  isProfilePreviewEntry,
  profileLabelFromPath,
  sourceLabel,
  sortNodesByRecent,
  summarizeNodeContent,
} from './data/DataShared'

interface UserProfileData {
  user_id?: string
  display_name?: string
  preferences?: Record<string, string>
  updated_at?: string
}

interface DashboardPageProps {
  systemSettingsEnabled?: boolean
  localMode?: boolean
}

export default function DashboardPage({ systemSettingsEnabled = false, localMode = false }: DashboardPageProps) {
  const { locale, tx } = useI18n()
  const [stats, setStats] = useState<DashboardStats>({
    connections: 0,
    files: 0,
    projects: 0,
    skills: 0,
    memory: 0,
    profile: 0,
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
        setError(statsData.reason?.message || tx('加载概览失败', 'Failed to load overview'))
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
      setExportSuccess(tx('ZIP 文件已开始下载。', 'The ZIP download has started.'))
    } catch (err: any) {
      setExportError(err.message || tx('导出失败', 'Export failed'))
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
      a.download = `neudrive-export-${new Date().toISOString().slice(0, 10)}.json`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
      setExportSuccess(tx('JSON 文件已开始下载。', 'The JSON download has started.'))
    } catch (err: any) {
      setExportError(err.message || tx('导出失败', 'Export failed'))
    } finally {
      setExporting(false)
    }
  }

  if (loading) {
    return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>
  }

  const dashboardStats = [
    { key: 'connections', label: tx('已连接平台', 'Connected apps'), to: '/connections' },
    { key: 'files', label: tx('所有文件', 'All files'), to: '/data/files' },
    { key: 'projects', label: tx('项目', 'Projects'), to: '/data/projects' },
    { key: 'skills', label: tx('技能', 'Skills'), to: '/data/skills' },
    { key: 'memory', label: 'Memory', to: '/data/memory' },
    { key: 'profile', label: tx('我的资料', 'My Profile'), to: '/data/profile' },
  ] as const

  const hasPending = stats.pending && stats.pending.length > 0

  return (
    <div className="page materials-page">
      <div className="page-header">
        <div>
          <h2>{tx('概览', 'Overview')}</h2>
          <p className="page-subtitle">{tx('用和文件管理器同一套视觉语言，快速查看 Hub 的连接、文件、资料和同步状态。', 'Use the same visual language as the file browser to quickly review connections, files, profile data, and sync status.')}</p>
        </div>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}

      <div className="status-banner">
        <span className="status-icon status-ok">&#10003;</span>
        <span className="status-text">
          {hasPending ? tx('有待处理事项', 'Pending items') : tx('一切正常', 'Everything looks good')}
        </span>
      </div>

      <div className="stats-grid">
        {dashboardStats.map((item) => (
          <Link key={item.key} to={item.to} className="stat-card">
            <div className="stat-value">{stats[item.key] ?? '-'}</div>
            <div className="stat-label">
              {item.key === 'connections' && (stats.connections ?? 0) === 0 ? tx('添加平台', 'Add app') : item.label}
            </div>
          </Link>
        ))}
      </div>

      <div className="dashboard-content-grid">
        <div className="card dashboard-card">
          <div className="card-header">
            <h3 className="card-title">{tx('我的资料', 'My Profile')}</h3>
            <Link to="/data/profile" className="dashboard-card-link">{tx('更多', 'More')}</Link>
          </div>

          <div className="dashboard-profile-head">
            <div>
              <div className="dashboard-profile-name">{profile?.display_name || tx('未设置显示名称', 'No display name set')}</div>
              <div className="dashboard-profile-meta">
                {tx('首页只显示最近更新的 2 项资料，完整内容请到“我的资料”页面查看和编辑。', 'This page only shows the two most recently updated profile entries. Open My Profile to view and edit everything.')}
              </div>
            </div>
          </div>

          {recentProfileEntries.length > 0 ? (
            <div className="dashboard-profile-list">
              {recentProfileEntries.map((entry) => (
                <div key={entry.path} className="dashboard-profile-item">
                  <div className="dashboard-profile-label">{profileLabelFromPath(entry.path, locale)}</div>
                  <div className="dashboard-profile-value">{summarizeNodeContent(entry, 120, locale)}</div>
                  <div className="dashboard-profile-item-meta">
                    <span className="dashboard-inline-chip">{sourceLabel(entry.source, locale)}</span>
                    <span style={{ marginLeft: 8 }}>{formatDateTime(entry.updated_at || entry.created_at, locale)}</span>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="dashboard-empty-copy">{tx('还没有资料内容。', 'No profile data yet.')}</p>
          )}
        </div>

        <div className="card dashboard-card">
          <div className="card-header">
            <h3 className="card-title">{tx('最近更新', 'Recently updated')}</h3>
            <Link to="/data/files" className="dashboard-card-link">{tx('文件管理器', 'File Browser')}</Link>
          </div>

          <div className="dashboard-profile-meta dashboard-preview-meta">
            {tx('首页只显示最近改过的 2 个文档，完整列表请到“文件管理器”查看。', 'This page only shows the two most recently edited documents. Open the File Browser for the full list.')}
          </div>

          {recentFiles.length > 0 ? (
            <div className="dashboard-file-list">
              {recentFiles.map((entry) => (
                <div key={entry.path} className="dashboard-file-item">
                  <div className="dashboard-file-path">{entry.path}</div>
                  <div className="dashboard-file-meta">
                    <span className="dashboard-inline-chip">{sourceLabel(entry.source, locale)}</span>
                    <span style={{ marginLeft: 8 }}>{formatDateTime(entry.updated_at || entry.created_at, locale)}</span>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="dashboard-empty-copy">{tx('还没有文件内容。', 'No files yet.')}</p>
          )}
        </div>
      </div>

      {hasPending && (
        <div className="card">
          <h3 className="card-title">{tx('待处理', 'Pending')}</h3>
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
          <h3 className="card-title">{tx('本周活动', 'This week')}</h3>
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

      {localMode && (
        <div className="card">
          <h3 className="card-title">{tx('Claude Code 迁移', 'Claude Code Migration')}</h3>
          <p style={{ marginBottom: '1rem', color: 'var(--color-text-secondary, #888)' }}>
            {tx(
              '先扫描本机 Claude Code 数据，再把 projects、memory、skills、会话和结构化归档迁移到 neuDrive。',
              'Scan local Claude Code data first, then migrate projects, memory, skills, conversations, and structured archives into neuDrive.',
            )}
          </p>
          <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap' }}>
            <Link to="/migrations/claude" className="btn btn-primary">
              {tx('打开迁移报告', 'Open migration report')}
            </Link>
            <Link to="/connections" className="btn">
              {tx('查看平台连接', 'View connections')}
            </Link>
          </div>
        </div>
      )}

      <div className="card">
        <h3 className="card-title">{tx('数据管理', 'Data')}</h3>
        <p style={{ marginBottom: '1rem', color: 'var(--color-text-secondary, #888)' }}>
          {systemSettingsEnabled
            ? tx('下载你所有的 Hub 数据，或分别打开 Git Mirror 与系统设置页面。', 'Download all Hub data, or open the Git Mirror and System Settings pages separately.')
            : tx('下载你所有的 Hub 数据，或打开 Git Mirror 配置同步。', 'Download all Hub data, or open Git Mirror to configure sync.')}
        </p>
        <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap' }}>
          <Link to="/git-mirror" className="btn">
            {tx('打开 Git Mirror', 'Open Git Mirror')}
          </Link>
          {systemSettingsEnabled && (
            <Link to="/settings" className="btn">
              {tx('打开系统设置', 'Open System Settings')}
            </Link>
          )}
          <button
            className="btn btn-primary"
            disabled={exporting}
            onClick={handleExportZip}
          >
            {exporting ? tx('导出中...', 'Exporting...') : tx('导出数据 (ZIP)', 'Export data (ZIP)')}
          </button>
          <button
            className="btn"
            disabled={exporting}
            onClick={handleExportJSON}
          >
            {tx('导出数据 (JSON)', 'Export data (JSON)')}
          </button>
        </div>
        {exportError && <div className="alert alert-warn" style={{ marginTop: '0.75rem' }}>{exportError}</div>}
        {exportSuccess && <div className="alert alert-ok" style={{ marginTop: '0.75rem' }}>{exportSuccess}</div>}
      </div>
    </div>
  )
}
