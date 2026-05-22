import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type ConnectionResponse, type DashboardStats, type FileNode, type OAuthGrantResponse } from '../api'
import DashboardFileBrowser from '../components/DashboardFileBrowser'
import { useI18n } from '../i18n'

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

function normalizePlatform(value?: string) {
  return `${value || ''}`.trim().toLowerCase().replace(/[\s_]+/g, '-')
}

function hasConnectedPlatform(connections: ConnectionResponse[], grants: OAuthGrantResponse[], aliases: string[]) {
  const normalizedAliases = aliases.map(normalizePlatform)
  const manual = connections.some((connection) => {
    const platform = normalizePlatform(connection.platform || connection.name)
    const name = normalizePlatform(connection.name)
    return normalizedAliases.includes(platform) || normalizedAliases.includes(name)
  })
  const oauth = grants.some((grant) => {
    const values = [
      grant.app?.name,
      grant.app?.client_id,
      ...(grant.app?.redirect_uris || []),
    ].map(normalizePlatform)
    return values.some((value) => normalizedAliases.includes(value))
  })
  return manual || oauth
}

const brandPaths = {
  claude: 'm4.7144 15.9555 4.7174-2.6471.079-.2307-.079-.1275h-.2307l-.7893-.0486-2.6956-.0729-2.3375-.0971-2.2646-.1214-.5707-.1215-.5343-.7042.0546-.3522.4797-.3218.686.0608 1.5179.1032 2.2767.1578 1.6514.0972 2.4468.255h.3886l.0546-.1579-.1336-.0971-.1032-.0972L6.973 9.8356l-2.55-1.6879-1.3356-.9714-.7225-.4918-.3643-.4614-.1578-1.0078.6557-.7225.8803.0607.2246.0607.8925.686 1.9064 1.4754 2.4893 1.8336.3643.3035.1457-.1032.0182-.0728-.164-.2733-1.3539-2.4467-1.445-2.4893-.6435-1.032-.17-.6194c-.0607-.255-.1032-.4674-.1032-.7285L6.287.1335 6.6997 0l.9957.1336.419.3642.6192 1.4147 1.0018 2.2282 1.5543 3.0296.4553.8985.2429.8318.091.255h.1579v-.1457l.1275-1.706.2368-2.0947.2307-2.6957.0789-.7589.3764-.9107.7468-.4918.5828.2793.4797.686-.0668.4433-.2853 1.8517-.5586 2.9021-.3643 1.9429h.2125l.2429-.2429.9835-1.3053 1.6514-2.0643.7286-.8196.85-.9046.5464-.4311h1.0321l.759 1.1293-.34 1.1657-1.0625 1.3478-.8804 1.1414-1.2628 1.7-.7893 1.36.0729.1093.1882-.0183 2.8535-.607 1.5421-.2794 1.8396-.3157.8318.3886.091.3946-.3278.8075-1.967.4857-2.3072.4614-3.4364.8136-.0425.0304.0486.0607 1.5482.1457.6618.0364h1.621l3.0175.2247.7892.522.4736.6376-.079.4857-1.2142.6193-1.6393-.3886-3.825-.9107-1.3113-.3279h-.1822v.1093l1.0929 1.0686 2.0035 1.8092 2.5075 2.3314.1275.5768-.3218.4554-.34-.0486-2.2039-1.6575-.85-.7468-1.9246-1.621h-.1275v.17l.4432.6496 2.3436 3.5214.1214 1.0807-.17.3521-.6071.2125-.6679-.1214-1.3721-1.9246L14.38 17.959l-1.1414-1.9428-.1397.079-.674 7.2552-.3156.3703-.7286.2793-.6071-.4614-.3218-.7468.3218-1.4753.3886-1.9246.3157-1.53.2853-1.9004.17-.6314-.0121-.0425-.1397.0182-1.4328 1.9672-2.1796 2.9446-1.7243 1.8456-.4128.164-.7164-.3704.0667-.6618.4008-.5889 2.386-3.0357 1.4389-1.882.929-1.0868-.0062-.1579h-.0546l-6.3385 4.1164-1.1293.1457-.4857-.4554.0608-.7467.2307-.2429 1.9064-1.3114Z',
  openai: 'M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.142.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.872zm16.5963 3.8558L13.1038 8.364 15.1192 7.2a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.6765 8.1042v-5.6772a.79.79 0 0 0-.407-.667zm2.0107-3.0231l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.2297V6.8974a.0662.0662 0 0 1 .0284-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.02-1.1638a.0804.0804 0 0 1-.038-.0567V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805L8.704 5.459a.7948.7948 0 0 0-.3927.6813zm1.0976-2.3654l2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.4997-2.6067-1.4997Z',
  cursor: 'M11.503.131 1.891 5.678a.84.84 0 0 0-.42.726v11.188c0 .3.162.575.42.724l9.609 5.55a1 1 0 0 0 .998 0l9.61-5.55a.84.84 0 0 0 .42-.724V6.404a.84.84 0 0 0-.42-.726L12.497.131a1.01 1.01 0 0 0-.996 0M2.657 6.338h18.55c.263 0 .43.287.297.515L12.23 22.918c-.062.107-.229.064-.229-.06V12.335a.59.59 0 0 0-.295-.51l-9.11-5.257c-.109-.063-.064-.23.061-.23',
  windsurf: 'M23.55 5.067c-1.2038-.002-2.1806.973-2.1806 2.1765v4.8676c0 .972-.8035 1.7594-1.7597 1.7594-.568 0-1.1352-.286-1.4718-.7659l-4.9713-7.1003c-.4125-.5896-1.0837-.941-1.8103-.941-1.1334 0-2.1533.9635-2.1533 2.153v4.8957c0 .972-.7969 1.7594-1.7596 1.7594-.57 0-1.1363-.286-1.4728-.7658L.4076 5.1598C.2822 4.9798 0 5.0688 0 5.2882v4.2452c0 .2147.0656.4228.1884.599l5.4748 7.8183c.3234.462.8006.8052 1.3509.9298 1.3771.313 2.6446-.747 2.6446-2.0977v-4.893c0-.972.7875-1.7593 1.7596-1.7593h.003a1.798 1.798 0 0 1 1.4718.7658l4.9723 7.0994c.4135.5905 1.05.941 1.8093.941 1.1587 0 2.1515-.9645 2.1515-2.153v-4.8948c0-.972.7875-1.7594 1.7596-1.7594h.194a.22.22 0 0 0 .2204-.2202v-4.622a.22.22 0 0 0-.2203-.2203Z',
  gemini: 'M11.04 19.32Q12 21.51 12 24q0-2.49.93-4.68.96-2.19 2.58-3.81t3.81-2.55Q21.51 12 24 12q-2.49 0-4.68-.93a12.3 12.3 0 0 1-3.81-2.58 12.3 12.3 0 0 1-2.58-3.81Q12 2.49 12 0q0 2.49-.96 4.68-.93 2.19-2.55 3.81a12.3 12.3 0 0 1-3.81 2.58Q2.49 12 0 12q2.49 0 4.68.96 2.19.93 3.81 2.55t2.55 3.81',
}

function PlatformIcon({ id }: { id: string }) {
  if (id === 'browser') {
    return (
      <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <path d="M7.2 3.5h3.2c.5 0 .9.4.9.9v1.1c0 .7.6 1.3 1.3 1.3s1.3-.6 1.3-1.3V4.4c0-.5.4-.9.9-.9h2c1 0 1.8.8 1.8 1.8v4.1h-1.1c-1 0-1.9.8-1.9 1.9s.8 1.9 1.9 1.9h1.1v5.5c0 1-.8 1.8-1.8 1.8H5.3c-1 0-1.8-.8-1.8-1.8v-5.5h1.2c1 0 1.9-.8 1.9-1.9s-.8-1.9-1.9-1.9H3.5V5.3c0-1 .8-1.8 1.8-1.8h1.9Z" />
      </svg>
    )
  }
  if (id === 'mcp') {
    return (
      <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <path fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" d="M7 7h10M7 17h10M8 8l8 8M16 8l-8 8" />
        <circle cx="6" cy="6" r="3" />
        <circle cx="18" cy="6" r="3" />
        <circle cx="6" cy="18" r="3" />
        <circle cx="18" cy="18" r="3" />
      </svg>
    )
  }
  if (id === 'claude-code' || id === 'codex') {
    return (
      <svg viewBox="0 0 56 24" aria-hidden="true" focusable="false">
        <rect x="1.5" y="5" width="15" height="14" rx="3" fill="none" stroke="currentColor" strokeWidth="2" />
        <path d="M5.2 10.2 8 12l-2.8 1.8M9.6 15h3.2" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
        <path className={id === 'claude-code' ? 'icon-claude' : 'icon-openai'} d={id === 'claude-code' ? brandPaths.claude : brandPaths.openai} transform="translate(28 3.5) scale(.72)" />
      </svg>
    )
  }
  if (id === 'api') {
    return (
      <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <path fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" d="M8 7 3 12l5 5M16 7l5 5-5 5" />
        <path fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" d="m14 4-4 16" />
      </svg>
    )
  }
  const path = id === 'chatgpt'
    ? brandPaths.openai
    : id === 'gemini'
      ? brandPaths.gemini
      : brandPaths[id as keyof typeof brandPaths]
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path d={path} />
    </svg>
  )
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  if (value < 1024) return `${value} B`
  const kib = value / 1024
  if (kib < 1024) return `${kib.toFixed(kib >= 10 ? 0 : 1)} KiB`
  const mib = kib / 1024
  if (mib < 1024) return `${mib.toFixed(mib >= 10 ? 0 : 1)} MiB`
  const gib = mib / 1024
  return `${gib.toFixed(gib >= 10 ? 0 : 1)} GiB`
}

function entrySize(entry: FileNode) {
  if (entry.is_dir) return 0
  return Math.max(0, Number(entry.size || entry.content?.length || 0))
}

function normalizeEntryPath(entry: FileNode) {
  const path = `/${String(entry.path || '').replace(/^\/+|\/+$/g, '')}`
  return path === '/.' ? '/' : path
}

function skillTreeStats(entries: FileNode[]) {
  let files = 0
  let folders = 0
  for (const entry of entries) {
    const path = normalizeEntryPath(entry).toLowerCase()
    if (!path.startsWith('/skills/')) continue
    if (entry.is_dir) folders += 1
    else files += 1
  }
  return { files, folders }
}

function latestTimestamp(entries: FileNode[]) {
  let latest = 0
  for (const entry of entries) {
    const value = new Date(entry.updated_at || entry.created_at || 0).getTime()
    if (Number.isFinite(value) && value > latest) latest = value
  }
  return latest
}

export default function DashboardPage({}: DashboardPageProps) {
  const { locale, tx } = useI18n()
  const [stats, setStats] = useState<DashboardStats>(emptyStats)
  const [connections, setConnections] = useState<ConnectionResponse[]>([])
  const [grants, setGrants] = useState<OAuthGrantResponse[]>([])
  const [treeEntries, setTreeEntries] = useState<FileNode[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const loadOverview = async () => {
    setError('')
    const [statsResult, connectionsResult, grantsResult, treeResult] = await Promise.allSettled([
      api.getStats(),
      api.getConnections(),
      api.getOAuthGrants(),
      api.getTreeSnapshot('/'),
    ])
    if (statsResult.status === 'fulfilled') setStats(statsResult.value || emptyStats)
    else setError(statsResult.reason?.message || tx('加载概览失败', 'Failed to load overview'))
    if (connectionsResult.status === 'fulfilled') setConnections(connectionsResult.value || [])
    if (grantsResult.status === 'fulfilled') setGrants(grantsResult.value || [])
    if (treeResult.status === 'fulfilled') setTreeEntries(Array.isArray(treeResult.value.entries) ? treeResult.value.entries : [])
  }

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      setLoading(true)
      await loadOverview()
      if (!cancelled) setLoading(false)
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [])

  const connectionMethods = useMemo(() => [
    {
      id: 'claude',
      name: 'Claude',
      description: tx('连接 Claude 到 neuDrive', 'Connect Claude to neuDrive'),
      to: '/setup/mcp',
      state: { platform: 'claude' },
      aliases: ['claude', 'claude.ai', 'claude.com', 'claude-web', 'claude-desktop', 'claude-connector'],
    },
    {
      id: 'chatgpt',
      name: 'ChatGPT',
      description: tx('连接 ChatGPT App', 'Connect ChatGPT app'),
      to: '/setup/mcp',
      state: { platform: 'chatgpt' },
      aliases: ['chatgpt', 'chatgpt-apps', 'openai'],
    },
    {
      id: 'cursor',
      name: 'Cursor',
      description: tx('连接 Cursor 编辑器', 'Connect Cursor editor'),
      to: '/setup/mcp',
      state: { platform: 'cursor' },
      aliases: ['cursor'],
    },
    {
      id: 'windsurf',
      name: 'Windsurf',
      description: tx('连接 Windsurf 编辑器', 'Connect Windsurf editor'),
      to: '/setup/mcp',
      state: { platform: 'windsurf' },
      aliases: ['windsurf'],
    },
    {
      id: 'claude-code',
      name: 'Claude Code',
      description: tx('连接 Claude Code CLI', 'Connect Claude Code CLI'),
      to: '/setup/cli',
      state: { cloudPlatform: 'claude' },
      aliases: ['claude-code', 'claude code'],
    },
    {
      id: 'codex',
      name: 'Codex',
      description: tx('连接 Codex CLI', 'Connect Codex CLI'),
      to: '/setup/cli',
      state: { cloudPlatform: 'codex' },
      aliases: ['codex', 'codex-cli'],
    },
    {
      id: 'gemini',
      name: 'Gemini CLI',
      description: tx('连接 Gemini CLI', 'Connect Gemini CLI'),
      to: '/setup/cli',
      state: { cloudPlatform: 'gemini' },
      aliases: ['gemini'],
    },
    {
      id: 'mcp',
      name: tx('其他 MCP 客户端', 'Other MCP Client'),
      description: tx('连接其他 AI 工具', 'Connect another AI tool'),
      to: '/setup/mcp',
      state: { platform: 'claude' },
      aliases: ['mcp', 'mcp-client', 'other-mcp-client', 'custom', 'other'],
    },
    {
      id: 'api',
      name: 'REST API / SDK',
      description: tx('连接自定义 Agent', 'Connect custom agents'),
      to: '/settings/developer-access',
      aliases: ['api', 'sdk', 'rest', 'rest-api'],
    },
  ], [tx])

  const storageBytes = useMemo(() => treeEntries.reduce((total, entry) => total + entrySize(entry), 0), [treeEntries])
  const folderCount = useMemo(() => treeEntries.filter((entry) => entry.is_dir).length, [treeEntries])
  const lastSyncTime = latestTimestamp(treeEntries)
  const lastSyncLabel = lastSyncTime
    ? new Intl.DateTimeFormat(locale === 'zh-CN' ? 'zh-CN' : 'en-US', {
        month: 'numeric',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
      }).format(new Date(lastSyncTime))
    : '-'
  const pending = Array.isArray(stats.pending) ? stats.pending : []
  const pendingCount = pending.reduce((total, item) => total + Math.max(0, item.count || 0), 0)
  const skillEntries = skillTreeStats(treeEntries)
  const summaryItems = [
    { label: tx('连接', 'Connections'), value: stats.connections },
    { label: tx('文件', 'Files'), value: stats.files },
    { label: tx('存储', 'Storage'), value: formatBytes(storageBytes) },
    { label: tx('上次同步', 'Last sync'), value: lastSyncLabel, compact: true },
    { label: tx('Skill 包', 'Skill bundles'), value: stats.skills },
    { label: tx('Skill 文件', 'Skill files'), value: skillEntries.files },
    { label: tx('Skill 文件夹', 'Skill folders'), value: skillEntries.folders },
    { label: tx('文件夹', 'Folders'), value: folderCount },
    { label: tx('待处理', 'Pending'), value: pendingCount },
  ]

  if (loading) return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>

  return (
    <div className="page home-page dashboard-redesign-page">
      {error && <div className="alert alert-warn">{error}</div>}

      <section className="dashboard-platform-panel" aria-label={tx('连接方式', 'Connection methods')}>
        <div className="dashboard-section-head">
          <div>
            <h3>{tx('连接方式', 'Connection methods')}</h3>
            <p>{tx('绿色表示当前账户已经有这种类型的连接。', 'Green means this account already has that connection type.')}</p>
          </div>
          <Link to="/connections" className="dashboard-card-link">{tx('管理连接', 'Manage connections')}</Link>
        </div>
        <div className="dashboard-platform-grid">
          {connectionMethods.map((method) => {
            const connected = hasConnectedPlatform(connections, grants, method.aliases)
            return (
              <Link key={method.id} to={method.to} state={method.state} className={connected ? 'dashboard-platform-card is-connected' : 'dashboard-platform-card'}>
                <span className={`dashboard-platform-icon platform-${method.id}`} aria-hidden="true">
                  <PlatformIcon id={method.id} />
                </span>
                <span className="dashboard-platform-copy">
                  <strong>{method.name}</strong>
                  <small>{method.description}</small>
                  <em>{connected ? tx('已连接', 'Connected') : tx('未连接', 'Not connected')}</em>
                </span>
              </Link>
            )
          })}
        </div>
      </section>

      <section className="dashboard-main-grid">
        <DashboardFileBrowser stats={stats} initialSnapshotEntries={treeEntries} onDataChange={() => { void loadOverview() }} />

        <aside className="dashboard-summary-panel">
          <div className="dashboard-section-head compact">
            <h3>{tx('摘要', 'Summary')}</h3>
          </div>
          <div className="dashboard-summary-list">
            {summaryItems.map((item) => (
              <div key={item.label} className={item.compact ? 'dashboard-summary-item is-compact' : 'dashboard-summary-item'}>
                <span>{item.label}</span>
                <strong>{item.value}</strong>
              </div>
            ))}
          </div>
        </aside>
      </section>
    </div>
  )
}
