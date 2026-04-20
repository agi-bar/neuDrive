import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type BillingStatus, type DashboardStats, type FileNode } from '../api'
import { useI18n } from '../i18n'
import { formatBillingStorage, resolvePlan } from './BillingShared'
import {
  formatDateTime,
  isProfileEntry,
  isProfilePreviewEntry,
  isVisibleFileEntry,
  profileLabelFromPath,
  sortNodesByRecent,
  sourceLabel,
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
  billingEnabled?: boolean
}

export default function DashboardPage({
  systemSettingsEnabled = false,
  localMode = false,
  billingEnabled = false,
}: DashboardPageProps) {
  const { locale, tx } = useI18n()
  const [stats, setStats] = useState<DashboardStats>({
    connections: 0,
    files: 0,
    projects: 0,
    conversations: 0,
    skills: 0,
    memory: 0,
    profile: 0,
    weekly_activity: [],
    pending: [],
  })
  const [profile, setProfile] = useState<UserProfileData | null>(null)
  const [recentProfileEntries, setRecentProfileEntries] = useState<FileNode[]>([])
  const [recentFiles, setRecentFiles] = useState<FileNode[]>([])
  const [billingStatus, setBillingStatus] = useState<BillingStatus | null>(null)
  const [billingError, setBillingError] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [exporting, setExporting] = useState(false)
  const [exportError, setExportError] = useState('')
  const [exportSuccess, setExportSuccess] = useState('')

  useEffect(() => {
    const loadDashboard = async () => {
      setLoading(true)
      try {
        const [statsData, profileData, profileSnapshotData, rootSnapshotData, billingData] = await Promise.allSettled([
          api.getStats(),
          api.getProfile(),
          api.getTreeSnapshot('/memory/profile'),
          api.getTreeSnapshot('/'),
          billingEnabled ? api.getBillingStatus() : Promise.resolve(null),
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

        if (billingEnabled) {
          if (billingData.status === 'fulfilled' && billingData.value) {
            setBillingStatus(billingData.value)
            setBillingError('')
          } else if (billingData.status === 'rejected') {
            setBillingStatus(null)
            setBillingError(billingData.reason?.message || tx('加载 Billing 状态失败', 'Failed to load billing status'))
          }
        } else {
          setBillingStatus(null)
          setBillingError('')
        }
      } catch (err: any) {
        setError(err?.message || tx('加载概览失败', 'Failed to load overview'))
      } finally {
        setLoading(false)
      }
    }

    void loadDashboard()
  }, [billingEnabled, tx])

  const handleExportZip = async () => {
    setExporting(true)
    setExportError('')
    setExportSuccess('')
    try {
      await api.exportZip()
      setExportSuccess(tx('ZIP 文件已开始下载。', 'The ZIP download has started.'))
    } catch (err: any) {
      setExportError(err?.message || tx('导出失败', 'Export failed'))
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
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `neudrive-export-${new Date().toISOString().slice(0, 10)}.json`
      document.body.appendChild(anchor)
      anchor.click()
      document.body.removeChild(anchor)
      URL.revokeObjectURL(url)
      setExportSuccess(tx('JSON 文件已开始下载。', 'The JSON download has started.'))
    } catch (err: any) {
      setExportError(err?.message || tx('导出失败', 'Export failed'))
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
    { key: 'conversations', label: tx('会话', 'Conversations'), to: '/data/conversations' },
    { key: 'skills', label: tx('技能', 'Skills'), to: '/data/skills' },
    { key: 'memory', label: 'Memory', to: '/data/memory' },
    { key: 'profile', label: tx('我的资料', 'My Profile'), to: '/data/profile' },
  ] as const

  const hasPending = stats.pending && stats.pending.length > 0
  const currentPlan = resolvePlan(billingStatus?.plans || [], billingStatus?.current_plan || 'free')
  const usagePercent = billingStatus && billingStatus.limit_bytes > 0
    ? Math.min(100, Math.round((billingStatus.used_bytes / billingStatus.limit_bytes) * 100))
    : 0

  return (
    <div className="page materials-page">
      <div className="page-header">
        <div>
          <h2>{tx('概览', 'Overview')}</h2>
          <p className="page-subtitle">
            {tx(
              '用和文件管理器同一套视觉语言，快速查看 Hub 的连接、文件、资料和同步状态。',
              'Use the same visual language as the file browser to quickly review connections, files, profile data, and sync status.',
            )}
          </p>
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
                {tx(
                  '首页只显示最近更新的 2 项资料，完整内容请到“我的资料”页面查看和编辑。',
                  'This page only shows the two most recently updated profile entries. Open My Profile to view and edit everything.',
                )}
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
            {tx(
              '首页只显示最近改过的 2 个文档，完整列表请到“文件管理器”查看。',
              'This page only shows the two most recently edited documents. Open the File Browser for the full list.',
            )}
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

        {billingEnabled && (
          <div className="card dashboard-card">
            <div className="card-header">
              <h3 className="card-title">{tx('Billing', 'Billing')}</h3>
              <Link to="/billing" className="dashboard-card-link">{tx('打开', 'Open')}</Link>
            </div>

            {billingError && <div className="alert alert-warn">{billingError}</div>}

            {!billingError && billingStatus && (
              <>
                <div className="billing-dashboard-plan">
                  <div className="billing-dashboard-name">{currentPlan?.name || billingStatus.current_plan}</div>
                  <div className="billing-dashboard-meta">
                    {tx('已用', 'Used')}: {formatBillingStorage(billingStatus.used_bytes, locale)} / {formatBillingStorage(billingStatus.limit_bytes, locale)}
                  </div>
                </div>
                <div className="billing-meter">
                  <div className="billing-meter-fill" style={{ width: `${usagePercent}%` }} />
                </div>
                <div className="dashboard-profile-meta">
                  {billingStatus.account_read_only
                    ? tx('当前账户因空间超额处于只读状态。', 'This account is currently read-only because it is over its storage limit.')
                    : billingStatus.entitlement_status === 'grace'
                      ? tx('当前处于宽限期，可在 Billing 页面续费或恢复订阅。', 'This account is currently in a grace period. Renew or manage the subscription from Billing.')
                      : tx('从这里快速查看套餐和空间使用情况。', 'Use Billing to review plan details and storage usage.')}
                </div>
              </>
            )}
          </div>
        )}
      </div>

      {hasPending && (
        <div className="card">
          <h3 className="card-title">{tx('待处理', 'Pending')}</h3>
          <div className="pending-list">
            {stats.pending.map((item, index) => (
              <div key={index} className="pending-item">
                <span className="pending-badge">{item.count}</span>
                <span className="pending-message">{item.message}</span>
                <span className="pending-type">{item.type}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {localMode && (
        <div className="card">
          <h3 className="card-title">{tx('本地平台迁移', 'Local Platform Migration')}</h3>
          <p style={{ marginBottom: '1rem', color: 'var(--color-text-secondary, #888)' }}>
            {tx(
              'Claude Code 和 Codex CLI 现在都支持本地扫描迁移；ChatGPT 当前主路径仍然是浏览器扩展导入对话。',
              'Claude Code and Codex CLI now support local scan-and-migrate flows. ChatGPT currently still imports conversations through the browser extension path.',
            )}
          </p>
          <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap' }}>
            <Link to="/imports/claude" className="btn btn-primary">
              {tx('Claude 迁移报告', 'Claude migration report')}
            </Link>
            <Link to="/imports/codex" className="btn">
              {tx('Codex 迁移报告', 'Codex migration report')}
            </Link>
            <Link to="/imports/claude-export" className="btn">
              {tx('导入 Claude 官方导出', 'Import Claude official export')}
            </Link>
            <Link to="/setup/web-apps" className="btn">
              {tx('ChatGPT Web 接入', 'ChatGPT web setup')}
            </Link>
            <Link to="/connections" className="btn">
              {tx('查看平台连接', 'View connections')}
            </Link>
          </div>
        </div>
      )}

      {stats.weekly_activity && stats.weekly_activity.length > 0 && (
        <div className="card">
          <h3 className="card-title">{tx('本周活动', 'This week')}</h3>
          <div className="activity-list">
            {stats.weekly_activity.map((item, index) => (
              <div key={index} className="activity-row">
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
          <button className="btn btn-primary" disabled={exporting} onClick={() => { void handleExportZip() }}>
            {exporting ? tx('导出中...', 'Exporting...') : tx('导出数据 (ZIP)', 'Export data (ZIP)')}
          </button>
          <button className="btn" disabled={exporting} onClick={() => { void handleExportJSON() }}>
            {tx('导出数据 (JSON)', 'Export data (JSON)')}
          </button>
        </div>
        {exportError && <div className="alert alert-warn" style={{ marginTop: '0.75rem' }}>{exportError}</div>}
        {exportSuccess && <div className="alert alert-ok" style={{ marginTop: '0.75rem' }}>{exportSuccess}</div>}
      </div>
    </div>
  )
}
