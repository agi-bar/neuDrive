import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, type ConnectionResponse, type OAuthGrantResponse } from '../api'
import { useI18n } from '../i18n'

type PlatformKey = 'claude' | 'chatgpt' | 'cursor' | 'windsurf' | 'other'

const platformCatalog: Record<PlatformKey, {
  name: string
  method: string
  copy: string
  settingsUrl?: string
}> = {
  claude: {
    name: 'Claude',
    method: 'Remote MCP connector',
    copy: 'Use neuDrive as a remote MCP connector.',
    settingsUrl: 'https://claude.ai/settings/connectors',
  },
  chatgpt: {
    name: 'ChatGPT',
    method: 'Remote MCP via Apps',
    copy: 'Create a ChatGPT App and point its MCP Server URL at neuDrive.',
  },
  cursor: {
    name: 'Cursor',
    method: 'MCP server',
    copy: 'Add neuDrive as an MCP server.',
  },
  windsurf: {
    name: 'Windsurf',
    method: 'MCP server',
    copy: 'Add neuDrive as an MCP server.',
  },
  other: {
    name: 'Other MCP Client',
    method: 'Remote MCP',
    copy: 'Use the remote MCP URL with any compatible client.',
  },
}

function normalizePlatform(raw?: string): PlatformKey | '' {
  const value = (raw || '').toLowerCase()
  if (value === 'claude' || value === 'chatgpt' || value === 'cursor' || value === 'windsurf') return value
  if (value === 'other' || value === 'mcp' || value === 'api') return 'other'
  return ''
}

function platformConnected(platform: PlatformKey, connections: ConnectionResponse[], grants: OAuthGrantResponse[]) {
  const terms = platform === 'other' ? ['mcp', 'api', 'custom'] : [platform]
  const manual = connections.some((connection) => {
    const haystack = `${connection.platform} ${connection.name}`.toLowerCase()
    return terms.some((term) => haystack.includes(term))
  })
  const oauth = grants.some((grant) => {
    const haystack = `${grant.app?.name || ''} ${grant.app?.client_id || ''} ${(grant.app?.redirect_uris || []).join(' ')}`.toLowerCase()
    return terms.some((term) => haystack.includes(term))
  })
  return manual || oauth
}

export default function OnboardingPage() {
  const { tx } = useI18n()
  const navigate = useNavigate()
  const params = useParams()
  const selectedPlatform = normalizePlatform(params.platform)
  const [step, setStep] = useState(selectedPlatform ? 1 : 0)
  const [testStatus, setTestStatus] = useState('')
  const [testing, setTesting] = useState(false)
  const [copied, setCopied] = useState('')
  const mcpURL = `${window.location.origin}/mcp`
  const platform = selectedPlatform ? platformCatalog[selectedPlatform] : null

  useEffect(() => {
    setStep(selectedPlatform ? 1 : 0)
    setTestStatus('')
  }, [selectedPlatform])

  const steps = useMemo(() => [
    tx('选择工具', 'Choose tool'),
    tx('配置连接', 'Configure'),
    tx('导入数据', 'Import data'),
    tx('测试', 'Test'),
    tx('完成', 'Done'),
  ], [tx])

  const choosePlatform = (key: PlatformKey) => {
    navigate(`/onboarding/${key}`)
  }

  const copyText = async (value: string, key: string) => {
    await navigator.clipboard?.writeText(value)
    setCopied(key)
    window.setTimeout(() => setCopied(''), 1600)
  }

  const testConnection = async () => {
    if (!selectedPlatform) return
    setTesting(true)
    setTestStatus('')
    try {
      const [connections, grants] = await Promise.all([
        api.getConnections().catch(() => []),
        api.getOAuthGrants().catch(() => []),
      ])
      const connected = platformConnected(selectedPlatform, connections, grants)
      setTestStatus(connected
        ? tx('Connected', 'Connected')
        : tx('No connection detected yet. Add the connector in your AI app, then test again.', 'No connection detected yet. Add the connector in your AI app, then test again.'))
      if (connected) {
        setStep(4)
      }
    } catch (err: any) {
      setTestStatus(err?.message || tx('测试失败', 'Test failed'))
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className="page onboarding-page">
      <div className="page-header compact-header">
        <div>
          <h2>{selectedPlatform ? tx(`连接 ${platform?.name}`, `Connect ${platform?.name}`) : tx('连接你的第一个 AI 工具', 'Connect your first AI tool')}</h2>
          <p className="page-subtitle">{tx('neuDrive 会在连接成功后成为你的 AI 记忆、文件和技能层。', 'After setup, neuDrive becomes the memory, files, and skills layer for your AI tools.')}</p>
        </div>
      </div>

      <div className="onboarding-steps">
        {steps.map((label, index) => (
          <span key={label} className={index <= step ? 'is-active' : ''}>{index + 1}. {label}</span>
        ))}
      </div>

      {!selectedPlatform && (
        <section className="platform-grid">
          {(Object.keys(platformCatalog) as PlatformKey[]).map((key) => (
            <button key={key} className="platform-card" onClick={() => choosePlatform(key)}>
              <strong>{platformCatalog[key].name}</strong>
              <span>{platformCatalog[key].copy}</span>
              <small>{platformCatalog[key].method}</small>
            </button>
          ))}
        </section>
      )}

      {selectedPlatform && platform && (
        <section className="setup-wizard">
          <div className="setup-main">
            <article className="wizard-card">
              <div className="wizard-step-label">Step 1 of 4</div>
              <h3>{tx('复制 MCP URL', 'Copy MCP URL')}</h3>
              <code className="copy-code">{mcpURL}</code>
              <button className="btn btn-primary" onClick={() => { void copyText(mcpURL, 'mcp') }}>
                {copied === 'mcp' ? tx('已复制', 'Copied') : tx('复制 URL', 'Copy URL')}
              </button>
            </article>

            <article className="wizard-card">
              <div className="wizard-step-label">Step 2 of 4</div>
              <h3>{tx('打开设置', 'Open settings')}</h3>
              <p>{selectedPlatform === 'claude' ? 'Open Claude → Settings → Connectors.' : tx('打开你的 AI 工具设置，找到 MCP 或 Actions 配置。', 'Open your AI tool settings and find MCP or Actions configuration.')}</p>
              {platform.settingsUrl ? (
                <a className="btn btn-outline" href={platform.settingsUrl} target="_blank" rel="noreferrer">{tx('打开设置', 'Open settings')}</a>
              ) : (
                <button className="btn btn-outline" onClick={() => setStep(2)}>{tx('我已打开设置', 'I opened settings')}</button>
              )}
            </article>

            <article className="wizard-card">
              <div className="wizard-step-label">Step 3 of 4</div>
              <h3>{tx('添加连接器', 'Add connector')}</h3>
              <div className="setup-fields">
                <div><span>Name</span><code>neuDrive</code></div>
                <div><span>URL</span><code>{mcpURL}</code></div>
              </div>
              <button className="btn btn-outline" onClick={() => setStep(3)}>{tx('我已添加', 'I added it')}</button>
            </article>

            <article className="wizard-card">
              <div className="wizard-step-label">Step 4 of 4</div>
              <h3>{tx('测试连接', 'Test connection')}</h3>
              <button className="btn btn-primary" disabled={testing} onClick={() => { void testConnection() }}>
                {testing ? tx('测试中...', 'Testing...') : tx('测试连接', 'Test connection')}
              </button>
              {testStatus && <div className={`alert ${testStatus === 'Connected' ? 'alert-success' : 'alert-warn'}`}>{testStatus}</div>}
            </article>
          </div>

          <aside className="setup-aside">
            <h3>{tx('导入第一份数据', 'Import your first data')}</h3>
            <Link to="/imports/claude-export" className="btn btn-outline btn-block">{tx('上传导出 ZIP', 'Upload export ZIP')}</Link>
            <Link to="/data/files" className="btn btn-outline btn-block">{tx('稍后导入', 'Skip for now')}</Link>
            <h3>{tx('测试 Prompt', 'Test prompt')}</h3>
            <p className="setup-prompt">Read my neuDrive profile and summarize what you know about my working preferences.</p>
            <button className="btn btn-outline btn-block" onClick={() => { void copyText('Read my neuDrive profile and summarize what you know about my working preferences.', 'prompt') }}>
              {copied === 'prompt' ? tx('已复制', 'Copied') : tx('复制 Prompt', 'Copy prompt')}
            </button>
            <Link to="/" className="btn btn-primary btn-block">{tx('打开 Dashboard', 'Open Dashboard')}</Link>
            <Link to="/connections" className="btn btn-outline btn-block">{tx('连接另一个应用', 'Connect another app')}</Link>
          </aside>
        </section>
      )}
    </div>
  )
}
