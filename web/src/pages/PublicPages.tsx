import { FormEvent, useEffect, useMemo, useState, type ReactNode } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, type AuthProvider } from '../api'
import LanguageToggle from '../components/LanguageToggle'
import { useI18n } from '../i18n'

const HUB_URL = 'https://www.neudrive.ai'
const MCP_URL = `${HUB_URL}/mcp`

type LocalizedText = {
  zh: string
  en: string
}

type GuideCode = {
  label: LocalizedText
  language: string
  value: string
}

type GuideStep = {
  title: LocalizedText
  copy: LocalizedText
  detail?: LocalizedText
  codes?: GuideCode[]
  note?: LocalizedText
}

type IntegrationGuide = {
  key: string
  aliases?: string[]
  name: string
  shortName: string
  method: LocalizedText
  setup: LocalizedText
  accent: string
  audience: LocalizedText
  demo: LocalizedText
  workflowSummary: LocalizedText
  guideTitle: LocalizedText
  steps: GuideStep[]
  testPrompt: LocalizedText
  afterConnection: LocalizedText[]
  limits?: LocalizedText[]
}

const integrations: IntegrationGuide[] = [
  {
    key: 'claude',
    aliases: ['cloud'],
    name: 'Claude',
    shortName: 'Claude',
    method: { zh: 'Claude Connector', en: 'Claude Connector' },
    setup: { zh: '约 2 分钟', en: '~2 min' },
    accent: 'C',
    audience: {
      zh: '适合正在使用 Claude Web 的用户。Claude 会通过 Connectors 连接 neuDrive。',
      en: 'For Claude Web users. Claude connects to neuDrive through Connectors.',
    },
    demo: {
      zh: '复制 neuDrive MCP 地址，在 Claude 添加 Connector，完成授权后验证读取。',
      en: 'Copy the neuDrive MCP URL, add a Connector in Claude, authorize, then verify the read.',
    },
    workflowSummary: {
      zh: '准备好 Claude 登录状态和 neuDrive 账号，按下面 4 步完成连接。',
      en: 'Make sure you are signed in to Claude and neuDrive, then follow the 4 steps below.',
    },
    guideTitle: {
      zh: '把 Claude 连接到 neuDrive',
      en: 'Connect Claude to neuDrive',
    },
    steps: [
      {
        title: { zh: '复制 neuDrive MCP 地址', en: 'Copy the neuDrive MCP URL' },
        copy: {
          zh: '点击复制。下一步把它粘贴到 Claude 的 Remote MCP Server URL 输入框。',
          en: 'Click copy. In the next step, paste it into Claude’s Remote MCP Server URL field.',
        },
        codes: [{ label: { zh: 'neuDrive MCP 地址', en: 'neuDrive MCP URL' }, language: 'text', value: MCP_URL }],
      },
      {
        title: { zh: '在 Claude 添加自定义 Connector', en: 'Add a custom Connector in Claude' },
        copy: {
          zh: '打开 Claude：Settings -> Connectors -> Go to Customize -> Add custom connector。',
          en: 'Open Claude: Settings -> Connectors -> Go to Customize -> Add custom connector.',
        },
        detail: {
          zh: 'Name 填 `neuDrive`，Remote MCP Server URL 粘贴上面的地址，然后保存。',
          en: 'Use `neuDrive` as the name, paste the URL above into Remote MCP Server URL, then save.',
        },
      },
      {
        title: { zh: '完成 neuDrive 授权', en: 'Finish neuDrive authorization' },
        copy: {
          zh: '点击 Connect。浏览器会打开 neuDrive 授权页；登录并确认连接。',
          en: 'Click Connect. The browser opens neuDrive authorization; sign in and approve the connection.',
        },
        note: {
          zh: '完成后回到 Claude，新开一个对话。',
          en: 'After approval, return to Claude and start a new chat to test.',
        },
      },
      {
        title: { zh: '验证 Claude 能读取记忆', en: 'Verify Claude can read memory' },
        copy: {
          zh: '复制下方测试问题，让 Claude 读取 neuDrive。',
          en: 'Copy the test question below and ask Claude to read neuDrive.',
        },
      },
    ],
    testPrompt: {
      zh: '请读取我的 neuDrive 资料，并总结你现在能看到的工作偏好、项目资料和技能。',
      en: 'Please read my neuDrive data and summarize the work preferences, project material, and skills you can see.',
    },
    afterConnection: [
      { zh: '导入 Claude 官方导出文件。', en: 'Import the official Claude export file.' },
      { zh: '把当前项目资料保存到 neuDrive。', en: 'Save current project material to neuDrive.' },
      { zh: '在 Connections 里查看 Claude 连接状态。', en: 'Check Claude connection status in Connections.' },
    ],
  },
  {
    key: 'chatgpt',
    aliases: ['openai', 'apps'],
    name: 'ChatGPT Apps',
    shortName: 'ChatGPT',
    method: { zh: 'ChatGPT App', en: 'ChatGPT App' },
    setup: { zh: '约 3 分钟', en: '~3 min' },
    accent: 'G',
    audience: {
      zh: '适合已经在 ChatGPT 设置里看到 Apps 入口的用户。ChatGPT 会通过 App 连接 neuDrive。',
      en: 'For ChatGPT users who already see Apps in settings. ChatGPT connects to neuDrive through an App.',
    },
    demo: {
      zh: '在 ChatGPT Apps 里创建 neuDrive App，粘贴 MCP 地址，完成授权后验证读取。',
      en: 'Create a neuDrive App in ChatGPT Apps, paste the MCP URL, authorize, then verify the read.',
    },
    workflowSummary: {
      zh: '准备好 ChatGPT 登录状态和 neuDrive 账号，按下面 4 步完成连接。',
      en: 'Make sure you are signed in to ChatGPT and neuDrive, then follow the 4 steps below.',
    },
    guideTitle: {
      zh: '把 ChatGPT Apps 连接到 neuDrive',
      en: 'Connect ChatGPT Apps to neuDrive',
    },
    steps: [
      {
        title: { zh: '复制 neuDrive MCP 地址', en: 'Copy the neuDrive MCP URL' },
        copy: {
          zh: '点击复制。创建 ChatGPT App 时，把它粘贴到 MCP Server URL 输入框。',
          en: 'Click copy. When creating the ChatGPT App, paste it into the MCP Server URL field.',
        },
        codes: [{ label: { zh: 'neuDrive MCP 地址', en: 'neuDrive MCP URL' }, language: 'text', value: MCP_URL }],
      },
      {
        title: { zh: '创建 neuDrive App', en: 'Create the neuDrive App' },
        copy: {
          zh: '打开 ChatGPT：Settings -> Apps -> Advanced settings -> Create app。',
          en: 'Open ChatGPT: Settings -> Apps -> Advanced settings -> Create app.',
        },
        detail: {
          zh: 'App 名称填 `neuDrive`，MCP Server URL 粘贴上面的地址，然后保存。',
          en: 'Use `neuDrive` as the App name, paste the URL above into MCP Server URL, then save.',
        },
      },
      {
        title: { zh: '完成 neuDrive 授权', en: 'Finish neuDrive authorization' },
        copy: {
          zh: '保存 App。浏览器会打开 neuDrive 授权页；登录并确认连接。',
          en: 'Save the App. The browser opens neuDrive authorization; sign in and approve the connection.',
        },
      },
      {
        title: { zh: '新开会话测试', en: 'Test in a fresh chat' },
        copy: {
          zh: '新开一个 ChatGPT 对话，复制下方测试问题。',
          en: 'Start a new ChatGPT chat and copy the test question below.',
        },
      },
    ],
    testPrompt: {
      zh: '请读取我的 neuDrive 资料，并总结我的工作偏好和最近项目上下文。',
      en: 'Please read my neuDrive data and summarize my work preferences and recent project context.',
    },
    afterConnection: [
      { zh: '把重要对话保存到 Conversations。', en: 'Save important chats to Conversations.' },
      { zh: '在 Connections 里查看 ChatGPT 连接状态。', en: 'Check ChatGPT connection status in Connections.' },
      { zh: '需要网页导入时，继续安装浏览器插件。', en: 'Install the browser extension when you need web-chat import.' },
    ],
  },
  {
    key: 'editors',
    aliases: ['cursor', 'windsurf', 'coding-editors'],
    name: 'Coding Editors',
    shortName: 'Cursor / Windsurf',
    method: { zh: '编辑器接入', en: 'Editor connection' },
    setup: { zh: '约 3 分钟', en: '~3 min' },
    accent: 'Ed',
    audience: {
      zh: '适合 Cursor、Windsurf 这类代码编辑器。编辑器里的 AI 可以读取 neuDrive 项目资料。',
      en: 'For coding editors such as Cursor and Windsurf. The editor AI can read project material from neuDrive.',
    },
    demo: {
      zh: '选择你的编辑器，复制对应配置，完成授权后让 AI 读取项目上下文。',
      en: 'Choose your editor, copy the matching config, authorize, then ask the AI to read project context.',
    },
    workflowSummary: {
      zh: '选择你的编辑器，复制对应配置，保存后完成浏览器授权。',
      en: 'Choose your editor, copy the matching config, save it, then finish browser authorization.',
    },
    guideTitle: {
      zh: '把 Cursor / Windsurf 连接到 neuDrive',
      en: 'Connect Cursor / Windsurf to neuDrive',
    },
    steps: [
      {
        title: { zh: '打开编辑器 MCP 设置', en: 'Open editor MCP settings' },
        copy: {
          zh: 'Cursor 打开 Settings -> Tools & MCPs -> Add Custom MCP；Windsurf 打开 Settings -> Cascade -> MCP Servers。',
          en: 'In Cursor, open Settings -> Tools & MCPs -> Add Custom MCP. In Windsurf, open Settings -> Cascade -> MCP Servers.',
        },
      },
      {
        title: { zh: '复制对应 MCP 配置', en: 'Copy the matching MCP config' },
        copy: {
          zh: '复制 Cursor 或 Windsurf 对应配置，粘贴到编辑器的 MCP 设置里。',
          en: 'Copy the Cursor or Windsurf config and paste it into that editor’s MCP settings.',
        },
        codes: [
          {
            label: { zh: 'Cursor MCP config', en: 'Cursor MCP config' },
            language: 'json',
            value: JSON.stringify({ mcpServers: { neudrive: { url: MCP_URL } } }, null, 2),
          },
          {
            label: { zh: 'Windsurf MCP config', en: 'Windsurf MCP config' },
            language: 'json',
            value: JSON.stringify({ mcpServers: { neudrive: { serverUrl: MCP_URL } } }, null, 2),
          },
        ],
      },
      {
        title: { zh: '保存并完成授权', en: 'Save and authorize' },
        copy: {
          zh: '保存配置后点击 Connect、Authenticate 或 Open。浏览器会打开 neuDrive 授权页；登录并确认连接。',
          en: 'After saving, click Connect, Authenticate, or Open. The browser opens neuDrive authorization; sign in and approve the connection.',
        },
      },
      {
        title: { zh: '测试项目上下文', en: 'Test project context' },
        copy: {
          zh: '复制下方测试问题，让编辑器里的 AI 读取当前项目并写入 neuDrive。',
          en: 'Copy the test question below and ask the editor AI to read the current project and write it to neuDrive.',
        },
      },
    ],
    testPrompt: {
      zh: '请读取当前项目，并把项目背景、开发约定和常用命令保存到 neuDrive。',
      en: 'Please read the current project and save its background, development conventions, and common commands to neuDrive.',
    },
    afterConnection: [
      { zh: '保存项目 README 和开发约定。', en: 'Save the project README and development conventions.' },
      { zh: '导入可复用 skill。', en: 'Import reusable skills.' },
      { zh: '在 Data Explorer 里查看写入结果。', en: 'Review the result in Data Explorer.' },
    ],
  },
  {
    key: 'cli',
    aliases: ['codex', 'claude-code', 'gemini', 'cursor-agent', 'terminal'],
    name: 'CLI Agents',
    shortName: 'Codex / Claude CLI',
    method: { zh: '终端接入', en: 'Terminal setup' },
    setup: { zh: '约 3 分钟', en: '~3 min' },
    accent: 'CLI',
    audience: {
      zh: '适合在终端里使用 Codex、Claude Code、Gemini CLI 或 Cursor Agent 的用户。',
      en: 'For terminal users running Codex, Claude Code, Gemini CLI, or Cursor Agent.',
    },
    demo: {
      zh: '登录 neuDrive，复制对应 CLI 命令，然后验证连接。',
      en: 'Sign in to neuDrive, copy the matching CLI commands, then verify the connection.',
    },
    workflowSummary: {
      zh: '先登录 neuDrive，再选择你使用的 CLI 并复制对应命令。',
      en: 'Sign in to neuDrive first, then choose your CLI and copy the matching commands.',
    },
    guideTitle: {
      zh: '把 Codex / Claude CLI 连接到 neuDrive',
      en: 'Connect Codex / Claude CLI to neuDrive',
    },
    steps: [
      {
        title: { zh: '先登录 neuDrive', en: 'Sign in to neuDrive' },
        copy: {
          zh: '运行下面命令。登录完成后，`neu` 会保存当前 neuDrive 账号。',
          en: 'Run the commands below. After sign-in, `neu` saves your current neuDrive account.',
        },
        codes: [{
          label: { zh: 'neu hosted profile', en: 'neu hosted profile' },
          language: 'bash',
          value: `neu login --profile official --api-base ${HUB_URL}\nneu status\nneu browse`,
        }],
      },
      {
        title: { zh: '连接 Claude Code', en: 'Connect Claude Code' },
        copy: {
          zh: '使用 Claude Code 时，先添加 neuDrive，再在 Claude Code 里运行 `/mcp` 完成授权。',
          en: 'If you use Claude Code, add neuDrive first, then run `/mcp` inside Claude Code to authorize.',
        },
        codes: [
          {
            label: { zh: 'Claude Code command', en: 'Claude Code command' },
            language: 'bash',
            value: `claude mcp add -s user --transport http neudrive ${MCP_URL}`,
          },
          {
            label: { zh: 'Claude Code auth menu', en: 'Claude Code auth menu' },
            language: 'text',
            value: '/mcp',
          },
        ],
      },
      {
        title: { zh: '连接 Codex CLI', en: 'Connect Codex CLI' },
        copy: {
          zh: '使用 Codex CLI 时，运行下面三行命令：添加、登录、查看状态。',
          en: 'If you use Codex CLI, run the three commands below: add, log in, and list status.',
        },
        codes: [{
          label: { zh: 'Codex CLI commands', en: 'Codex CLI commands' },
          language: 'bash',
          value: `codex mcp add neudrive --url ${MCP_URL}\ncodex mcp login neudrive\ncodex mcp list`,
        }],
      },
      {
        title: { zh: '用 neu 导入、浏览和验证数据', en: 'Use neu to import, browse, and verify data' },
        copy: {
          zh: '连接后，用下面命令导入资料或打开 Data 页面检查结果。',
          en: 'After connection, use the commands below to import material or open Data to inspect results.',
        },
        codes: [{
          label: { zh: 'Import and browser commands', en: 'Import and browser commands' },
          language: 'bash',
          value: `neu platform ls\nneu import claude --dry-run\nneu import memory ./notes\nneu import skill ./demo-skill\nneu browse /data`,
        }],
      },
    ],
    testPrompt: {
      zh: '请读取当前工作区，把有长期价值的项目背景、命令和偏好写入 neuDrive，并列出写入路径。',
      en: 'Please read the current workspace, save durable project context, commands, and preferences into neuDrive, then list the written paths.',
    },
    afterConnection: [
      { zh: '用 `neu browse /data` 查看导入结果。', en: 'Use `neu browse /data` to review imported data.' },
      { zh: '用 `neu import project` 导入项目资料。', en: 'Use `neu import project` to import project material.' },
      { zh: '用 `neu sync pull` 导出备份。', en: 'Use `neu sync pull` to export a backup.' },
    ],
  },
  {
    key: 'browser',
    name: 'Browser Extension',
    shortName: 'Browser Import',
    method: { zh: 'Chrome / Edge 插件', en: 'Chrome / Edge sidecar' },
    setup: { zh: '约 2 分钟', en: '~2 min' },
    accent: 'B',
    audience: {
      zh: '适合想在 Claude、ChatGPT、Gemini、Kimi 网页里导入当前对话的用户。',
      en: 'For users who want to import the current chat from Claude, ChatGPT, Gemini, or Kimi web pages.',
    },
    demo: {
      zh: '安装扩展、登录 neuDrive、打开聊天页面，然后点击导入。',
      en: 'Install the extension, sign in to neuDrive, open a chat page, then click import.',
    },
    workflowSummary: {
      zh: '安装扩展后，聊天页面里会出现 neuDrive 面板。',
      en: 'After installing the extension, a neuDrive panel appears inside supported chat pages.',
    },
    guideTitle: {
      zh: '用 Browser Extension 导入网页对话',
      en: 'Use the Browser Extension to import web chats',
    },
    steps: [
      {
        title: { zh: '安装 Chrome / Edge 扩展', en: 'Install the Chrome / Edge extension' },
        copy: {
          zh: '打开 `chrome://extensions`，开启 Developer mode，点击 Load unpacked，选择 `extension/`。',
          en: 'Open `chrome://extensions`, enable Developer mode, click Load unpacked, and choose `extension/`.',
        },
      },
      {
        title: { zh: '连接 neuDrive', en: 'Connect to neuDrive' },
        copy: {
          zh: '点击扩展图标，选择“登录官方 neuDrive”，按浏览器提示完成登录。',
          en: 'Click the extension icon, choose “Sign in to official neuDrive”, and finish sign-in in the browser.',
        },
        codes: [{ label: { zh: 'Hosted Hub URL', en: 'Hosted Hub URL' }, language: 'text', value: HUB_URL }],
      },
      {
        title: { zh: '导入当前对话', en: 'Import the current conversation' },
        copy: {
          zh: '打开 Claude 或 ChatGPT 对话页，点击页面里的 neuDrive 面板，再点击导入当前对话。',
          en: 'Open a Claude or ChatGPT chat page, open the neuDrive panel, then click import current chat.',
        },
      },
      {
        title: { zh: '注入偏好、项目和 Skills', en: 'Inject preferences, project, and skills' },
        copy: {
          zh: '需要时，在面板里选择偏好、项目资料或技能，把内容放进当前输入框。',
          en: 'When needed, choose preferences, project material, or skills in the panel and place them into the current input box.',
        },
      },
    ],
    testPrompt: {
      zh: '打开扩展面板，导入当前对话，然后在 neuDrive Data Explorer 里查看结果。',
      en: 'Open the extension panel, import the current chat, then review the result in neuDrive Data Explorer.',
    },
    afterConnection: [
      { zh: '导入当前 Claude 或 ChatGPT 对话。', en: 'Import the current Claude or ChatGPT chat.' },
      { zh: '在 Data Explorer 里查看导入结果。', en: 'Review the imported result in Data Explorer.' },
      { zh: '把重要对话转成 Memory。', en: 'Convert important chats into Memory.' },
    ],
  },
  {
    key: 'api',
    aliases: ['rest', 'developer'],
    name: 'MCP / REST API',
    shortName: 'API',
    method: { zh: '开发者 API', en: 'Developer API' },
    setup: { zh: '约 5 分钟', en: '~5 min' },
    accent: 'API',
    audience: {
      zh: '适合自研 Agent、内部系统、自动化脚本和平台适配器。',
      en: 'For custom agents, internal systems, automation scripts, and platform adapters.',
    },
    demo: {
      zh: '为自定义 Agent 创建访问凭证，复制服务地址，然后接入 MCP 或 REST API。',
      en: 'Create an access credential for a custom agent, copy the service URL, then connect to MCP or REST API.',
    },
    workflowSummary: {
      zh: '这条路线面向开发者。按用途创建连接即可。',
      en: 'This path is for developers. Create a connection for the intended use.',
    },
    guideTitle: {
      zh: '用 MCP / REST API 构建自定义 Agent',
      en: 'Build custom agents with MCP / REST API',
    },
    steps: [
      {
        title: { zh: '选择用途模板', en: 'Choose a purpose template' },
        copy: {
          zh: '进入 Settings -> Developer Access，选择 Custom Agent。',
          en: 'Open Settings -> Developer Access and choose Custom Agent.',
        },
      },
      {
        title: { zh: '创建访问凭证', en: 'Create an access credential' },
        copy: {
          zh: '填写名称，保存后复制生成的凭证。',
          en: 'Enter a name, save, then copy the generated credential.',
        },
      },
      {
        title: { zh: '接入 MCP 或 REST', en: 'Connect through MCP or REST' },
        copy: {
          zh: '复制下面的服务地址，填到你的 Agent 或内部系统里。',
          en: 'Copy the service URL below and add it to your agent or internal system.',
        },
        codes: [{ label: { zh: 'neuDrive 服务地址', en: 'neuDrive service URL' }, language: 'text', value: MCP_URL }],
      },
      {
        title: { zh: '审计和撤销', en: 'Audit and revoke' },
        copy: {
          zh: '在 Developer Access 里查看使用记录；不再需要时点击 Revoke。',
          en: 'Review usage in Developer Access. Click Revoke when it is no longer needed.',
        },
      },
    ],
    testPrompt: {
      zh: '让自定义 Agent 读取 neuDrive profile 摘要，并返回读取结果。',
      en: 'Ask the custom agent to read a neuDrive profile summary and return the result.',
    },
    afterConnection: [
      { zh: '为每个 Agent 单独创建凭证。', en: 'Create a separate credential for each agent.' },
      { zh: '在 Recent Activity 查看写入记录。', en: 'Review writes in Recent Activity.' },
      { zh: '不再使用时撤销连接。', en: 'Revoke the connection when it is no longer used.' },
    ],
  },
]

function getIntegration(key?: string) {
  const normalized = (key || '').toLowerCase()
  return integrations.find((item) => item.key === normalized || item.aliases?.includes(normalized)) || integrations[0]
}

function tr(tx: (zh: string, en: string) => string, text: LocalizedText) {
  return tx(text.zh, text.en)
}

function CopySnippet({ label, value, language = 'text' }: { label: string; value: string; language?: string }) {
  const { tx } = useI18n()
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1400)
    } catch {
      setCopied(false)
    }
  }
  return (
    <div className="public-code-block">
      <div className="public-code-head">
        <span>{label}</span>
        <button type="button" onClick={copy}>{copied ? tx('已复制', 'Copied') : tx('复制', 'Copy')}</button>
      </div>
      <pre><code className={`language-${language}`}>{value}</code></pre>
    </div>
  )
}

function PublicShell({ children }: { children: ReactNode }) {
  const { tx } = useI18n()
  return (
    <div className="public-site">
      <header className="public-nav">
        <Link to="/" className="public-brand">neuDrive</Link>
        <nav className="public-nav-links">
          <a href="/#product">{tx('产品', 'Product')}</a>
          <Link to="/integrations">{tx('集成', 'Integrations')}</Link>
          <Link to="/pricing">{tx('价格', 'Pricing')}</Link>
          <Link to="/docs">{tx('文档', 'Docs')}</Link>
        </nav>
        <div className="public-nav-actions">
          <LanguageToggle compact />
          <Link to="/login" className="btn btn-outline">{tx('登录', 'Log in')}</Link>
          <Link to="/signup" className="btn btn-primary">{tx('立即开始', 'Get started')}</Link>
        </div>
      </header>
      {children}
    </div>
  )
}

export function MarketingHomePage() {
  const { tx } = useI18n()
  const [activeKey, setActiveKey] = useState('claude')
  const activeIntegration = getIntegration(activeKey)
  const firstCode = activeIntegration.steps.flatMap((step) => step.codes || [])[0]
  const pains = [
    {
      title: tx('上下文散落', 'Context is scattered'),
      copy: tx('Claude、ChatGPT、Cursor 各有各的上下文。', 'Claude, ChatGPT, and Cursor each keep separate context.'),
    },
    {
      title: tx('记忆不能迁移', 'Memory is not portable'),
      copy: tx('会话、偏好、项目资料很难迁移和复用。', 'Conversations, preferences, and project notes are hard to reuse.'),
    },
    {
      title: tx('私密信息不该到处粘贴', 'Secrets are unsafe to paste'),
      copy: tx('API Key、账号、私密信息不应该到处复制。', 'API keys, accounts, and private data should not be pasted everywhere.'),
    },
  ]
  const modules = [
    { name: tx('记忆 Memory', 'Memory'), label: tx('Profile / Projects / Scratch', 'Profile / Projects / Scratch'), copy: tx('保存个人偏好、项目上下文和短期工作记忆。', 'Store profile, project, and scratch working memory.') },
    { name: tx('文件 Files', 'Files'), label: tx('Data Explorer', 'Data Explorer'), copy: tx('统一管理 AI 可读取的文件、会话和资料。', 'Manage files, conversations, and references your AI can read.') },
    { name: tx('技能 Skills', 'Skills'), label: tx('.skill 路由', '.skill routing'), copy: tx('一次注册，多个 Agent 复用。', 'Register once and reuse across multiple agents.') },
    { name: tx('私密数据', 'Private data'), label: tx('访问控制', 'Access control'), copy: tx('决定每个 AI 工具能看到哪些资料。', 'Decide which material each AI tool can see.') },
  ]
  return (
    <PublicShell>
      <main>
        <section className="public-hero">
          <div className="public-hero-copy">
            <p className="public-kicker">{tx('AI 数据层', 'AI data layer')}</p>
            <h1>{tx('让所有 AI 工具共用同一份记忆。', 'One memory layer for all your AI agents.')}</h1>
            <p>
              {tx(
                'neuDrive 把你的会话、项目上下文、技能、文件和私密数据统一存放，并连接 Claude、ChatGPT、Cursor、Windsurf 等 AI 工具。',
                'neuDrive stores your conversations, project context, skills, files, and private data in one layer for Claude, ChatGPT, Cursor, Windsurf, and more.',
              )}
            </p>
            <div className="public-hero-actions">
              <Link to="/signup" className="btn btn-primary">{tx('立即开始', 'Get started')}</Link>
              <a href="#how-it-works" className="btn btn-outline">{tx('查看如何接入', 'See how it works')}</a>
            </div>
            <div className="public-hero-proof" aria-label="Product capabilities">
              <span>{tx('Profile 记忆', 'Profile memory')}</span>
              <span>{tx('私密数据控制', 'Private data control')}</span>
              <span>{tx('Skill 路由', 'Skill routing')}</span>
            </div>
            <a href="#product" className="public-scroll-cue">
              {tx('继续看 neuDrive 如何工作', 'Explore how the layer works')}
              <span />
            </a>
          </div>
          <div className="public-product-visual public-product-visual-rich" aria-label="neuDrive product demonstration">
            <div className="visual-grid" aria-hidden="true">
              {Array.from({ length: 48 }).map((_, index) => <span key={index} />)}
            </div>
            <div className="signal-routes" aria-hidden="true">
              <span className="signal-packet signal-packet-1" />
              <span className="signal-packet signal-packet-2" />
              <span className="signal-packet signal-packet-3" />
            </div>
            <div className="agent-orbit">
              {integrations.slice(0, 4).map((item, index) => (
                <span key={item.key} className={`orbit-chip orbit-chip-${index + 1}`}>{item.shortName}</span>
              ))}
              <div className="orbit-core">
                <strong>neuDrive</strong>
                <span>{tx('记忆 / 文件 / 技能 / 私密数据', 'Memory / Files / Skills / Private data')}</span>
              </div>
            </div>
            <div className="hero-window">
              <div className="window-bar"><span /><span /><span /></div>
              <div className="window-tabs">
                <span className="active">{tx('记忆', 'Memory')}</span>
                <span>{tx('文件', 'Files')}</span>
                <span>{tx('技能', 'Skills')}</span>
              </div>
              <div className="memory-thread">
                <div>
                  <span className="thread-label">Claude</span>
                  <p>{tx('读取我的工作偏好和项目上下文。', 'Read my working preferences and project context.')}</p>
                </div>
                <div className="thread-response">
                  <span className="thread-label">neuDrive</span>
                  <p>{tx('返回已授权的 Profile Memory、项目资料和可用 Skills。', 'Returns approved profile memory, project files, and available skills.')}</p>
                </div>
              </div>
              <div className="visual-access-row">
                <span>{tx('登录授权', 'Signed-in access')}</span>
                <span>{tx('可查看范围', 'Visible range')}</span>
                <span>{tx('可随时撤销', 'Revocable')}</span>
              </div>
            </div>
          </div>
        </section>

        <section className="public-continuity-strip" aria-label="neuDrive product flow">
          <div>
            <span>01</span>
            <strong>{tx('连接一次', 'Connect once')}</strong>
            <p>{tx('把 Claude、ChatGPT、Cursor 等工具指向同一个数据层。', 'Point Claude, ChatGPT, Cursor, and other tools at one data layer.')}</p>
          </div>
          <div>
            <span>02</span>
            <strong>{tx('控制访问', 'Control access')}</strong>
            <p>{tx('为每个 AI 工具决定能读取哪些资料，之后可以随时调整或撤销。', 'Decide which material each AI tool can read, then adjust or revoke it anytime.')}</p>
          </div>
          <div>
            <span>03</span>
            <strong>{tx('跨工具复用', 'Reuse everywhere')}</strong>
            <p>{tx('记忆、文件和技能跟着你的工作流走，而不是锁在某一个 AI 工具里。', 'Memory, files, and skills follow your workflow instead of staying locked inside one AI tool.')}</p>
          </div>
        </section>

        <section className="public-band pain-band">
          <div className="public-section-head stacked">
            <p className="public-kicker">{tx('为什么需要 neuDrive', 'Why neuDrive')}</p>
            <h2>{tx('你的 AI 工具不该每次都重新认识你。', 'Your AI tools should not have to meet you again every time.')}</h2>
          </div>
          <div className="public-card-grid three">
            {pains.map((item) => (
              <article key={item.title} className="public-card pain-card">
                <span className="card-mark" />
                <h3>{item.title}</h3>
                <p>{item.copy}</p>
              </article>
            ))}
          </div>
        </section>

        <section id="product" className="public-band product-split-band">
          <div className="product-story">
            <p className="public-kicker">{tx('产品能力', 'Product')}</p>
            <h2>{tx('一个地方管理 AI 工作记忆。', 'One place for your AI working memory.')}</h2>
            <p>{tx('把偏好、项目资料、会话、文件和技能放在 neuDrive。连接新的 AI 工具时，它可以从同一份资料开始工作。', 'Keep preferences, project material, conversations, files, and skills in neuDrive. When you connect a new AI tool, it can start from the same shared context.')}</p>
          </div>
          <div className="module-board">
            {modules.map((item, index) => (
              <article key={item.name} className={`module-card module-card-${index + 1}`}>
                <small>{item.label}</small>
                <h3>{item.name}</h3>
                <p>{item.copy}</p>
              </article>
            ))}
          </div>
        </section>

        <section id="how-it-works" className="public-band workflow-band">
          <div className="public-section-head">
            <div>
              <p className="public-kicker">{tx('接入方式', 'How it works')}</p>
              <h2>{tx('3 分钟接入第一个 AI 工具。', '3 minutes to connect your first AI agent.')}</h2>
              <p>{tx('先选择你要连接的工具。指南会给出需要复制的地址、设置路径和连接后的测试问题。', 'Choose the tool you want to connect. The guide gives you the URL to copy, the settings path, and a test question for after connection.')}</p>
            </div>
            <Link to={`/guides/${activeIntegration.key}`} className="btn btn-outline">{tx('查看演示指南', 'Open demo guide')}</Link>
          </div>
          <div className="workflow-shell">
            <div className="workflow-picker" role="tablist" aria-label="Integration setup demos">
              {integrations.map((item) => (
                <button
                  key={item.key}
                  className={item.key === activeKey ? 'active' : ''}
                  type="button"
                  role="tab"
                  aria-label={`Show ${item.name} setup demo`}
                  aria-selected={item.key === activeKey}
                  onClick={() => setActiveKey(item.key)}
                >
                  <span aria-hidden="true">{item.accent}</span>
                  {item.shortName}
                </button>
              ))}
            </div>
            <div className="workflow-demo-card">
              <div className="workflow-demo-head">
                <span>{tr(tx, activeIntegration.method)}</span>
                <strong>{tr(tx, activeIntegration.setup)}</strong>
              </div>
              <h3>{activeIntegration.name}</h3>
              <p>{tr(tx, activeIntegration.demo)}</p>
              <p className="workflow-summary">{tr(tx, activeIntegration.workflowSummary)}</p>
              {firstCode && <CopySnippet label={tr(tx, firstCode.label)} language={firstCode.language} value={firstCode.value} />}
              <div className="workflow-timeline compact">
                {activeIntegration.steps.map((step, index) => (
                  <div key={tr(tx, step.title)} className="workflow-step">
                    <span>{index + 1}</span>
                    <div>
                      <strong>{tr(tx, step.title)}</strong>
                      <p>{tr(tx, step.copy)}</p>
                      {step.detail && <small>{tr(tx, step.detail)}</small>}
                    </div>
                  </div>
                ))}
              </div>
              <div className="workflow-actions">
                <Link to={`/guides/${activeIntegration.key}`} className="btn btn-outline">{tx('查看完整指南', 'Open full guide')}</Link>
                <Link to={`/signup?platform=${activeIntegration.key}`} className="btn btn-primary">{tx('开始连接', 'Connect now')}</Link>
              </div>
            </div>
          </div>
        </section>

        <IntegrationsSection />
        <PricingSection />

        <section className="public-band security-band">
          <div>
            <p className="public-kicker">{tx('安全控制', 'Security')}</p>
            <h2>{tx('你的 AI 数据仍由你控制。', 'Your AI data stays under your control.')}</h2>
            <p>{tx('neuDrive 保存的是会话、文件、偏好和私密资料，所以用户必须一开始就知道：数据能导出、访问能控制、连接能撤销。', 'neuDrive stores conversations, files, preferences, and private material, so users should know from the first visit: data can be exported, access can be controlled, and connections can be revoked.')}</p>
          </div>
          <div className="security-panel">
            {[
              tx('随时导出', 'Export anytime'),
              tx('按应用授权', 'App-by-app access'),
              tx('可调整访问范围', 'Adjustable access range'),
              tx('私密数据保护', 'Private data protection'),
              tx('随时撤销访问', 'Revoke access anytime'),
            ].map((item, index) => (
              <div key={item} className="security-control">
                <span>0{index + 1}</span>
                <strong>{item}</strong>
              </div>
            ))}
          </div>
        </section>
      </main>
    </PublicShell>
  )
}

function IntegrationsSection() {
  const { tx } = useI18n()
  return (
    <section className="public-band">
      <div className="public-section-head">
        <div>
          <p className="public-kicker">{tx('集成', 'Integrations')}</p>
          <h2>{tx('连接你每天使用的 AI 工具。', 'Connect the AI tools you use every day.')}</h2>
          <p>{tx('先查看具体步骤。准备好后，再登录 neuDrive 完成连接。', 'Review the setup steps first. When you are ready, sign in to neuDrive to complete the connection.')}</p>
        </div>
        <Link to="/integrations" className="btn btn-outline">{tx('查看全部', 'View all')}</Link>
      </div>
      <div className="public-card-grid integrations">
        {integrations.map((item) => (
          <article key={item.key} className="public-card integration-card">
            <div className="integration-icon" aria-hidden="true">{item.accent}</div>
            <h3>{item.name}</h3>
            <p>{tr(tx, item.method)} · {tr(tx, item.setup)}</p>
            <small>{tr(tx, item.demo)}</small>
            <div className="integration-actions">
              <Link className="integration-guide-link" to={`/guides/${item.key}`}>{tx('演示指南', 'Demo guide')}</Link>
              <Link className="integration-connect-link" to={`/signup?platform=${item.key}`}>{tx('立即连接', 'Connect now')}</Link>
            </div>
          </article>
        ))}
      </div>
    </section>
  )
}

function PricingSection() {
  const { tx } = useI18n()
  return (
    <section className="public-band">
      <div className="public-section-head">
        <div>
          <p className="public-kicker">{tx('价格', 'Pricing')}</p>
          <h2>{tx('默认年付，省下 50%。', 'Yearly by default. Save 50%.')}</h2>
          <p>{tx('Free 适合试用和自托管评估。Pro 提供更多存储、自动同步和备份。', 'Free is for trying neuDrive or evaluating self-hosting. Pro adds more storage, auto sync, and backup.')}</p>
        </div>
        <Link to="/pricing" className="btn btn-outline">{tx('比较套餐', 'Compare plans')}</Link>
      </div>
      <div className="pricing-public-grid">
        <article className="pricing-public-card">
          <h3>Free</h3>
          <div className="pricing-price">$0</div>
          <p>{tx('10 MiB 存储', '10 MiB storage')}</p>
          <Link to="/signup" className="btn btn-outline">{tx('免费开始', 'Start free')}</Link>
        </article>
        <article className="pricing-public-card featured">
          <span className="recommended-chip">{tx('推荐', 'Recommended')}</span>
          <h3>Pro Yearly</h3>
          <div className="pricing-price">$60 / year</div>
          <p>{tx('1 GiB 存储 · 自动同步 · Git 备份 · 优先导入', '1 GiB storage · Auto sync · Git backup · Priority import')}</p>
          <Link to="/signup?plan=pro_yearly" className="btn btn-primary">{tx('年付 Pro', 'Start Pro yearly')}</Link>
        </article>
        <article className="pricing-public-card">
          <h3>Pro Monthly</h3>
          <div className="pricing-price">$10 / month</div>
          <p>{tx('按月灵活使用 Pro 能力。', 'Use Pro month to month.')}</p>
          <Link to="/signup?plan=pro_monthly" className="btn btn-outline">{tx('月付 Pro', 'Start monthly')}</Link>
        </article>
      </div>
    </section>
  )
}

export function PricingPage() {
  const { tx } = useI18n()
  const comparisonRows = [
    [tx('存储空间', 'Storage'), '10 MiB', '1 GiB', '1 GiB'],
    [tx('AI 工具连接', 'AI connections'), tx('不限连接', 'Unlimited'), tx('不限连接', 'Unlimited'), tx('不限连接', 'Unlimited')],
    [tx('同步方式', 'Sync'), tx('手动同步', 'Manual sync'), tx('自动同步', 'Auto sync'), tx('自动同步', 'Auto sync')],
    [tx('备份', 'Backup'), tx('ZIP 导出', 'ZIP export'), tx('ZIP + Git backup', 'ZIP + Git backup'), tx('ZIP + Git backup', 'ZIP + Git backup')],
    [tx('导入', 'Import'), tx('基础导入', 'Basic import'), tx('优先导入大批量会话', 'Priority large conversation import'), tx('优先导入大批量会话', 'Priority large conversation import')],
    [tx('私密数据控制', 'Private data controls'), tx('基础控制', 'Basic controls'), tx('完整控制', 'Full controls'), tx('完整控制', 'Full controls')],
  ]
  return (
    <PublicShell>
      <main className="public-simple">
        <PricingSection />
        <section className="public-band pricing-compare-section">
          <div className="public-section-head">
            <div>
              <p className="public-kicker">{tx('套餐比较', 'Plan comparison')}</p>
              <h2>{tx('选择适合你的使用方式。', 'Choose the plan that fits how you work.')}</h2>
            </div>
          </div>
          <div className="pricing-compare-table-wrap">
            <table className="pricing-compare-table">
              <thead>
                <tr>
                  <th>{tx('能力', 'Capability')}</th>
                  <th>Free</th>
                  <th>Pro Yearly</th>
                  <th>Pro Monthly</th>
                </tr>
              </thead>
              <tbody>
                {comparisonRows.map(([feature, free, yearly, monthly]) => (
                  <tr key={feature}>
                    <th>{feature}</th>
                    <td>{free}</td>
                    <td>{yearly}</td>
                    <td>{monthly}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      </main>
    </PublicShell>
  )
}

export function IntegrationsPage() {
  return (
    <PublicShell>
      <main className="public-simple">
        <IntegrationsSection />
      </main>
    </PublicShell>
  )
}

export function GuidePage() {
  const { platform } = useParams()
  const { tx } = useI18n()
  const guide = getIntegration(platform)
  return (
    <PublicShell>
      <main className="public-simple guide-page">
        <section className="public-band guide-hero">
          <div>
            <p className="public-kicker">{tx('演示指南', 'Setup guide')}</p>
            <h1>{tr(tx, guide.guideTitle)}</h1>
            <p>{tr(tx, guide.audience)}</p>
            <p>{tr(tx, guide.demo)}</p>
            <div className="public-hero-actions">
              <Link to={`/signup?platform=${guide.key}`} className="btn btn-primary">{tx('开始这个接入', 'Start this setup')}</Link>
              <Link to="/integrations" className="btn btn-outline">{tx('查看全部集成', 'View integrations')}</Link>
            </div>
          </div>
          <div className="guide-preview-card">
            <div className="window-bar"><span /><span /><span /></div>
            <h3>{tr(tx, guide.method)}</h3>
            <p>{tr(tx, guide.workflowSummary)}</p>
            <div className="guide-meta-grid">
              <span>{tx('预计时间', 'Setup time')}<strong>{tr(tx, guide.setup)}</strong></span>
              <span>{tx('接入入口', 'Entry point')}<strong>{tr(tx, guide.method)}</strong></span>
            </div>
          </div>
        </section>
        <section className="public-band guide-detail-section">
          <div className="guide-step-list">
            {guide.steps.map((step, index) => (
              <article key={tr(tx, step.title)} className="guide-step-card">
                <div className="guide-step-index">{String(index + 1).padStart(2, '0')}</div>
                <div className="guide-step-body">
                  <h2>{tr(tx, step.title)}</h2>
                  <p>{tr(tx, step.copy)}</p>
                  {step.detail && <p>{tr(tx, step.detail)}</p>}
                  {step.codes?.map((code) => (
                    <CopySnippet key={`${tr(tx, code.label)}-${code.value}`} label={tr(tx, code.label)} language={code.language} value={code.value} />
                  ))}
                  {step.note && <div className="guide-note">{tr(tx, step.note)}</div>}
                </div>
              </article>
            ))}
          </div>
        </section>
        <section className="public-band guide-test-section">
          <div className="guide-test-card">
            <p className="public-kicker">{tx('验证连接', 'Verify connection')}</p>
            <h2>{tx('复制测试问题，让平台读取 neuDrive。', 'Copy the test question and let the platform read neuDrive.')}</h2>
            <CopySnippet label={tx('复制测试问题', 'Copy test question')} language="text" value={tr(tx, guide.testPrompt)} />
          </div>
          <div className="guide-after-card">
            <p className="public-kicker">{tx('接下来', 'Next')}</p>
            <h2>{tx('连接成功后', 'After connection')}</h2>
            <ul>
              {guide.afterConnection.map((item) => <li key={tr(tx, item)}>{tr(tx, item)}</li>)}
            </ul>
            {guide.limits && (
              <div className="guide-limit-box">
                <strong>{tx('限制说明', 'Limitations')}</strong>
                {guide.limits.map((item) => <p key={tr(tx, item)}>{tr(tx, item)}</p>)}
              </div>
            )}
          </div>
        </section>
      </main>
    </PublicShell>
  )
}

export function DocsLandingPage() {
  const { tx } = useI18n()
  const docsGroups = [
    {
      title: tx('快速开始', 'Quick start'),
      copy: tx('先确认你要接入的是网页平台、代码编辑器、CLI 还是浏览器导入。', 'First decide whether you are connecting a web platform, coding editor, CLI, or browser import flow.'),
      links: [
        { label: tx('Claude Connectors 接入', 'Claude Connectors'), href: '/guides/claude' },
        { label: tx('ChatGPT Apps 接入', 'ChatGPT Apps'), href: '/guides/chatgpt' },
      ],
    },
    {
      title: tx('代码工作流', 'Coding workflows'),
      copy: tx('Cursor、Windsurf 和 CLI Agent 主要用于读取当前 repo，并把项目上下文写回 neuDrive。', 'Cursor, Windsurf, and CLI agents are best for reading the current repo and writing project context back to neuDrive.'),
      links: [
        { label: tx('Cursor / Windsurf 编辑器', 'Cursor / Windsurf'), href: '/guides/editors' },
        { label: tx('Codex / Claude CLI 命令行', 'Codex / Claude CLI'), href: '/guides/cli' },
      ],
    },
    {
      title: tx('导入与开发者接入', 'Import and developer access'),
      copy: tx('浏览器插件适合网页对话导入；API 适合自定义 Agent、内部系统和自动化任务。', 'The browser extension is for web-chat import; API is for custom agents, internal systems, and automation.'),
      links: [
        { label: tx('浏览器插件', 'Browser Extension'), href: '/guides/browser' },
        { label: tx('MCP / REST API 开发者接入', 'MCP / REST API'), href: '/guides/api' },
      ],
    },
  ]
  return (
    <PublicShell>
      <main className="public-simple">
        <section className="public-band docs-hero-section">
          <p className="public-kicker">{tx('文档', 'Docs')}</p>
          <h1>{tx('选择你要连接的工具。', 'Choose the tool you want to connect.')}</h1>
          <p>{tx('每个指南都给出设置路径、可复制内容、授权步骤和连接后的测试问题。', 'Each guide gives the settings path, copyable content, authorization steps, and a test question for after connection.')}</p>
        </section>
        <section className="public-band docs-grid-section">
          <div className="docs-grid">
            {docsGroups.map((group) => (
              <article key={group.title} className="docs-card">
                <h2>{group.title}</h2>
                <p>{group.copy}</p>
                <div className="docs-link-list">
                  {group.links.map((link) => (
                    <Link key={link.href} to={link.href}>{link.label}<span>-&gt;</span></Link>
                  ))}
                </div>
              </article>
            ))}
          </div>
        </section>
      </main>
    </PublicShell>
  )
}

export function SignupPage() {
  const { tx } = useI18n()
  const navigate = useNavigate()
  const [providers, setProviders] = useState<AuthProvider[]>([])
  const [error, setError] = useState('')
  const [loadingAction, setLoadingAction] = useState('')
  const [form, setForm] = useState({
    email: '',
    password: '',
    display_name: '',
    slug: '',
  })

  useEffect(() => {
    api.getAuthProviders().then(setProviders).catch(() => setProviders([]))
  }, [])

  const githubProvider = providers.find((provider) => provider.id === 'github')
  const pocketProvider = providers.find((provider) => provider.kind === 'oidc')
  const githubEnabled = !!githubProvider?.enabled
  const pocketEnabled = !!pocketProvider?.enabled
  const busy = loadingAction !== ''
  const suggestedSlug = useMemo(() => {
    const base = form.slug || form.email.split('@')[0] || form.display_name
    return base.toLowerCase().replace(/[^a-z0-9-]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40)
  }, [form.display_name, form.email, form.slug])

  const beginProviderSignup = async (provider: AuthProvider | undefined, action: 'login' | 'signup', key: string) => {
    if (!provider?.enabled) return
    setLoadingAction(key)
    setError('')
    localStorage.setItem('neudrive.postSignupIntent', '1')
    try {
      const resp = await api.startAuthProvider(provider.id, `${window.location.origin}/plan`, action)
      window.location.assign(resp.authorization_url)
    } catch (err: any) {
      setError(err?.message || tx('启动注册失败', 'Failed to start signup'))
      setLoadingAction('')
    }
  }

  const submitLocalSignup = async (event: FormEvent) => {
    event.preventDefault()
    if (busy) return
    setLoadingAction('email')
    setError('')
    try {
      const response = await api.register({
        ...form,
        slug: suggestedSlug,
        display_name: form.display_name || form.email,
      })
      localStorage.setItem('token', response.access_token)
      localStorage.setItem('refresh_token', response.refresh_token)
      localStorage.setItem('neudrive.postSignupIntent', '1')
      navigate('/plan', { replace: true })
    } catch (err: any) {
      setError(err?.message || tx('注册失败', 'Signup failed'))
      setLoadingAction('')
    }
  }

  return (
    <PublicShell>
      <main className="auth-split">
        <section className="auth-copy">
          <p className="public-kicker">Get started</p>
          <h1>{tx('3 分钟接入第一个 AI 工具。', 'Connect your first AI tool in 3 minutes.')}</h1>
          <p>{tx('注册后先选择套餐，再进入接入向导。Free 可以继续使用，Pro Yearly 会默认推荐。', 'After signup you will choose a plan, then enter the setup wizard. Free remains available and Pro Yearly is recommended by default.')}</p>
        </section>
        <section className="auth-card">
          {error && <div className="alert alert-warn">{error}</div>}
          <button className="btn btn-primary btn-block" disabled={busy || !githubEnabled} onClick={() => { void beginProviderSignup(githubProvider, 'login', 'github') }}>
            {loadingAction === 'github' ? tx('跳转中...', 'Redirecting...') : tx('使用 GitHub 继续', 'Continue with GitHub')}
          </button>
          <button className="btn btn-outline btn-block" disabled={busy || !pocketEnabled} onClick={() => { void beginProviderSignup(pocketProvider, 'signup', 'pocket') }}>
            {loadingAction === 'pocket' ? tx('跳转中...', 'Redirecting...') : tx('使用 Pocket ID 注册', 'Continue with Pocket ID')}
          </button>
          <div className="auth-divider"><span>{tx('或使用邮箱', 'or use email')}</span></div>
          <form className="login-form" onSubmit={submitLocalSignup}>
            <input className="input" type="email" required placeholder="you@example.com" value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} />
            <input className="input" type="text" placeholder={tx('显示名称', 'Display name')} value={form.display_name} onChange={(event) => setForm({ ...form, display_name: event.target.value })} />
            <input className="input" type="password" required minLength={8} placeholder={tx('密码', 'Password')} value={form.password} onChange={(event) => setForm({ ...form, password: event.target.value })} />
            <input className="input" type="text" placeholder={tx('用户名 slug', 'Username slug')} value={form.slug} onChange={(event) => setForm({ ...form, slug: event.target.value })} />
            <button className="btn btn-primary btn-block" disabled={busy}>
              {loadingAction === 'email' ? tx('创建中...', 'Creating...') : tx('创建账户', 'Create account')}
            </button>
          </form>
          <p className="login-note">
            {tx('已有账户？', 'Already have an account?')} <Link to="/login">{tx('登录', 'Log in')}</Link>
          </p>
        </section>
      </main>
    </PublicShell>
  )
}
