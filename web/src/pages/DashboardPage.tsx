import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type BillingStatus, type DashboardStats, type FileNode } from '../api'
import { useI18n } from '../i18n'
import { formatBillingStorage } from './BillingShared'
import { formatDateTime, sortNodesByRecent, sourceLabel } from './data/DataShared'

interface DashboardPageProps {
  systemSettingsEnabled?: boolean
  localMode?: boolean
  billingEnabled?: boolean
}

const emptyStats: DashboardStats = {
  connections: 0,
  files: 0,
  projects: 0,
  conversations: 0,
  skills: 0,
  memory: 0,
  profile: 0,
  weekly_activity: [],
  pending: [],
}

export default function DashboardPage({
  systemSettingsEnabled = false,
  localMode = false,
  billingEnabled = false,
}: DashboardPageProps) {
  const { locale, tx } = useI18n()
  const [stats, setStats] = useState<DashboardStats>(emptyStats)
  const [recent, setRecent] = useState<FileNode[]>([])
  const [billing, setBilling] = useState<BillingStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [exporting, setExporting] = useState('')
  const [exportMessage, setExportMessage] = useState('')

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      setLoading(true)
      setError('')
      const [statsResult, rootResult, billingResult] = await Promise.allSettled([
        api.getStats(),
        api.getTreeSnapshot('/'),
        billingEnabled ? api.getBillingStatus() : Promise.resolve(null),
      ])
      if (cancelled) return
      if (statsResult.status === 'fulfilled') setStats(statsResult.value)
      else setError(statsResult.reason?.message || tx('加载 Home 失败', 'Failed to load Home'))
      if (rootResult.status === 'fulfilled') {
        setRecent(sortNodesByRecent(rootResult.value.entries.filter((entry) => !entry.is_dir)).slice(0, 4))
      }
      if (billingResult.status === 'fulfilled') setBilling(billingResult.value)
      setLoading(false)
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [billingEnabled, tx])

  const checklist = useMemo(() => [
    { label: tx('Create account', 'Create account'), done: true, to: '/settings/profile' },
    { label: tx('Choose plan', 'Choose plan'), done: !billingEnabled || (billing?.current_plan && billing.current_plan !== 'free') || localStorage.getItem('neudrive.planGateSeen') === '1', to: billingEnabled ? '/plan' : '/onboarding' },
    { label: tx('Connect first AI app', 'Connect first AI app'), done: stats.connections > 0, to: '/connections' },
    { label: tx('Import first conversation', 'Import first conversation'), done: stats.conversations > 0, to: '/imports/claude-export' },
    { label: tx('Test neuDrive inside AI', 'Test neuDrive inside AI'), done: localStorage.getItem('neudrive.testPromptCopied') === '1', to: '/onboarding' },
  ], [billing?.current_plan, billingEnabled, stats.connections, stats.conversations, tx])

  const hasConnection = stats.connections > 0
  const isFree = billingEnabled && (!billing || billing.current_plan === 'free')
  const usagePercent = billing && billing.limit_bytes > 0
    ? Math.min(100, Math.round((billing.used_bytes / billing.limit_bytes) * 100))
    : 0

  const activity = [
    stats.conversations > 0 ? tx(`Imported ${stats.conversations} conversations`, `Imported ${stats.conversations} conversations`) : '',
    stats.skills > 0 ? tx(`${stats.skills} skills available`, `${stats.skills} skills available`) : '',
    stats.memory > 0 ? tx(`${stats.memory} memory entries`, `${stats.memory} memory entries`) : '',
    ...recent.slice(0, 2).map((entry) => tx(`Updated ${entry.name}`, `Updated ${entry.name}`)),
  ].filter(Boolean)

  const exportData = async (kind: 'zip' | 'json') => {
    setExporting(kind)
    setExportMessage('')
    try {
      if (kind === 'zip') {
        await api.exportZip()
      } else {
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
      }
      setExportMessage(tx('导出已开始。', 'Export started.'))
    } catch (err: any) {
      setExportMessage(err?.message || tx('导出失败', 'Export failed'))
    } finally {
      setExporting('')
    }
  }

  if (loading) return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>

  return (
    <div className="page home-page">
      <div className="page-header compact-header">
        <div>
          <h2>Home</h2>
          <p className="page-subtitle">
            {hasConnection
              ? tx('Your AI workspace is ready.', 'Your AI workspace is ready.')
              : tx('Connect your first AI tool to start using neuDrive.', 'Connect your first AI tool to start using neuDrive.')}
          </p>
        </div>
        <div className="page-actions">
          <Link to={hasConnection ? '/connections' : '/onboarding/claude'} className="btn btn-primary">
            {hasConnection ? tx('连接应用', 'Connect app') : tx('连接 Claude', 'Connect Claude')}
          </Link>
          <Link to="/imports/claude-export" className="btn btn-outline">{tx('导入数据', 'Import data')}</Link>
          {isFree && <Link to="/plan" className="btn btn-outline">{tx('升级', 'Upgrade')}</Link>}
        </div>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}

      {!hasConnection && (
        <section className="activation-banner">
          <div>
            <h3>{tx('Connect your first AI tool', 'Connect your first AI tool')}</h3>
            <p>{tx('neuDrive works after you connect Claude, ChatGPT, Cursor or another MCP client.', 'neuDrive works after you connect Claude, ChatGPT, Cursor or another MCP client.')}</p>
          </div>
          <div className="activation-actions">
            <Link className="btn btn-primary" to="/onboarding/claude">{tx('连接 Claude', 'Connect Claude')}</Link>
            <Link className="btn btn-outline" to="/onboarding">{tx('选择其他应用', 'Choose another app')}</Link>
          </div>
        </section>
      )}

      <section className="home-grid">
        <div className="card setup-checklist-card">
          <div className="card-header">
            <h3 className="card-title">{tx('Get started', 'Get started')}</h3>
          </div>
          <div className="setup-checklist">
            {checklist.map((item) => (
              <Link key={item.label} to={item.to} className="checklist-row">
                <span className={item.done ? 'check-dot done' : 'check-dot'}>{item.done ? '✓' : ''}</span>
                <span>{item.label}</span>
              </Link>
            ))}
          </div>
        </div>

        <div className="status-card-grid">
          {[
            { label: tx('Connections', 'Connections'), value: `${stats.connections} connected`, to: '/connections' },
            { label: tx('Storage', 'Storage'), value: billing ? `${formatBillingStorage(billing.used_bytes, locale)} / ${formatBillingStorage(billing.limit_bytes, locale)}` : tx('Core storage', 'Core storage'), to: '/data/files' },
            { label: 'Memory', value: `${stats.memory} entries`, to: '/memory' },
            { label: tx('Conversations', 'Conversations'), value: `${stats.conversations} imported`, to: '/data/conversations' },
            { label: 'Skills', value: `${stats.skills} installed`, to: '/skills' },
          ].map((item) => (
            <Link key={item.label} to={item.to} className="compact-status-card">
              <span>{item.label}</span>
              <strong>{item.value}</strong>
            </Link>
          ))}
        </div>
      </section>

      {isFree && billing && (
        <section className="free-upgrade-banner">
          <div>
            <h3>{tx('You are on Free', 'You are on Free')}</h3>
            <p>10 MiB storage · Manual sync</p>
            <div className="billing-meter">
              <div className="billing-meter-fill" style={{ width: `${usagePercent}%` }} />
            </div>
          </div>
          <div className="activation-actions">
            <Link to="/plan" className="btn btn-primary">{tx('年付升级', 'Upgrade yearly')}</Link>
            <Link to="/pricing" className="btn btn-outline">{tx('比较套餐', 'Compare plans')}</Link>
          </div>
        </section>
      )}

      <section className="home-grid lower">
        <div className="card">
          <div className="card-header">
            <h3 className="card-title">{tx('Quick Actions', 'Quick Actions')}</h3>
          </div>
          <div className="quick-action-grid">
            <Link to="/onboarding" className="quick-action">Connect AI app</Link>
            <Link to="/imports/claude-export" className="quick-action">Import data</Link>
            <Link to="/memory" className="quick-action">Create memory</Link>
            <Link to="/settings/developer-access" className="quick-action">Create token</Link>
            <button className="quick-action" disabled={exporting !== ''} onClick={() => { void exportData('zip') }}>Export backup</button>
          </div>
          {exportMessage && <div className="alert alert-warn">{exportMessage}</div>}
        </div>

        <div className="card">
          <div className="card-header">
            <h3 className="card-title">{tx('Recent Activity', 'Recent Activity')}</h3>
            <Link to="/data/files" className="dashboard-card-link">Data Explorer</Link>
          </div>
          {activity.length > 0 ? (
            <div className="activity-feed">
              {activity.slice(0, 5).map((item) => <div key={item}>{item}</div>)}
              {recent.slice(0, 2).map((entry) => (
                <div key={entry.path} className="activity-meta">
                  {sourceLabel(entry.source, locale)} · {formatDateTime(entry.updated_at || entry.created_at, locale)}
                </div>
              ))}
            </div>
          ) : (
            <div className="empty-action-state">
              <p>{tx('No activity yet. Connect an AI app to start syncing your memory.', 'No activity yet. Connect an AI app to start syncing your memory.')}</p>
              <Link to="/onboarding" className="btn btn-primary">{tx('立即连接', 'Connect now')}</Link>
            </div>
          )}
        </div>
      </section>

      {localMode && (
        <section className="card">
          <div className="card-header">
            <h3 className="card-title">{tx('Local imports', 'Local imports')}</h3>
          </div>
          <div className="page-actions">
            <Link to="/imports/claude" className="btn btn-outline">Claude Code</Link>
            <Link to="/imports/codex" className="btn btn-outline">Codex CLI</Link>
            <Link to="/imports/claude-export" className="btn btn-outline">Claude Export ZIP</Link>
            {systemSettingsEnabled && <Link to="/settings/security" className="btn btn-outline">{tx('安全设置', 'Security settings')}</Link>}
          </div>
        </section>
      )}
    </div>
  )
}
