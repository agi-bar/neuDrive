import { useEffect, useState, type ReactNode } from 'react'
import { Link, Navigate, useLocation, useParams } from 'react-router-dom'
import { api, type AuthProvider } from '../api'
import GitHubRepoLink from '../components/GitHubRepoLink'
import LanguageToggle from '../components/LanguageToggle'
import { useI18n } from '../i18n'
import { usePublicFeedbackEnabled } from '../publicFeedback'

const HUB_URL = 'https://www.neudrive.ai'
const MCP_URL = `${HUB_URL}/mcp`
const DEFAULT_SEO_DESCRIPTION = 'neuDrive lets Claude, ChatGPT, Cursor, and other AI tools share one memory, file, skill, and vault layer.'

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
  detailSummary?: LocalizedText
  detailHighlights?: LocalizedText[]
  detailLimits?: LocalizedText[]
  detailFaq?: Array<{ question: LocalizedText; answer: LocalizedText }>
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
    detailSummary: {
      zh: 'Claude 适合成为第一个接入的 AI 工具：连接后可以直接读取 neuDrive 中的个人记忆、项目资料和可用技能。',
      en: 'Claude is a good first connection: after setup, it can read Profile Memory, project material, and available Skills from neuDrive.',
    },
    detailHighlights: [
      { zh: '通过 Claude Connectors 添加 neuDrive。', en: 'Add neuDrive through Claude Connectors.' },
      { zh: '浏览器授权后，新会话即可使用。', en: 'After browser authorization, use it from a fresh chat.' },
      { zh: '适合读取偏好、项目上下文和可复用技能。', en: 'Good for reading preferences, project context, and reusable skills.' },
    ],
    detailLimits: [
      { zh: '如果账号看不到自定义 Connector，可能需要确认 Claude 账号或团队策略。', en: 'If custom Connectors are unavailable, check your Claude account or team policy.' },
      { zh: '新增工具通常在新开的 Claude 对话里最稳定。', en: 'Newly added tools are usually most reliable in a fresh Claude chat.' },
    ],
    detailFaq: [
      {
        question: { zh: 'Claude 会读取全部 neuDrive 数据吗？', en: 'Can Claude read all neuDrive data?' },
        answer: { zh: '不会。Claude 只能读取当前连接被授权访问的资料。', en: 'No. Claude can only read material authorized for that connection.' },
      },
      {
        question: { zh: '需要每次复制上下文吗？', en: 'Do I need to paste context every time?' },
        answer: { zh: '不需要。连接后可以让 Claude 从 neuDrive 读取已有记忆和项目资料。', en: 'No. After setup, Claude can read existing memory and project material from neuDrive.' },
      },
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
      { zh: '把需要复用的资料整理成 Memory 或 Projects。', en: 'Turn reusable material into Memory or Projects.' },
    ],
    detailSummary: {
      zh: 'ChatGPT Apps 接入适合希望在 ChatGPT 里调用 neuDrive 记忆、文件和技能的用户。',
      en: 'ChatGPT Apps setup is for using neuDrive memory, files, and skills from inside ChatGPT.',
    },
    detailHighlights: [
      { zh: '创建 neuDrive App 并粘贴 MCP Server URL。', en: 'Create a neuDrive App and paste the MCP Server URL.' },
      { zh: '授权后在新会话里测试读取。', en: 'After authorization, test from a fresh chat.' },
      { zh: '可与 Claude、编辑器和 CLI 接入共享同一份资料。', en: 'Shares the same neuDrive data with Claude, editors, and CLI tools.' },
    ],
    detailLimits: [
      { zh: 'ChatGPT Apps 入口取决于账号计划和开放范围。', en: 'The ChatGPT Apps entry depends on account plan and rollout availability.' },
      { zh: '如果暂时没有 Create app，可以先使用 Claude、编辑器或 CLI 接入。', en: 'If Create app is unavailable, start with Claude, editors, or CLI setup.' },
    ],
    detailFaq: [
      {
        question: { zh: '这是 GPT Actions 吗？', en: 'Is this GPT Actions?' },
        answer: { zh: '不是。这里面向 ChatGPT Apps 的 MCP 接入。', en: 'No. This page is for MCP setup through ChatGPT Apps.' },
      },
      {
        question: { zh: '连接后能做什么？', en: 'What can I do after setup?' },
        answer: { zh: '可以让 ChatGPT 读取 neuDrive 中的个人资料、项目上下文、文件和技能。', en: 'You can ask ChatGPT to read your neuDrive profile, project context, files, and skills.' },
      },
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
      { zh: '导入可复用技能。', en: 'Import reusable skills.' },
      { zh: '在文件管理器里查看写入结果。', en: 'Review the result in Data Explorer.' },
    ],
    detailSummary: {
      zh: 'Cursor / Windsurf 适合把当前 repo 的项目背景、命令、约定和长期上下文写入 neuDrive。',
      en: 'Cursor / Windsurf are best for saving repo background, commands, conventions, and durable project context into neuDrive.',
    },
    detailHighlights: [
      { zh: '在编辑器 MCP 设置里添加 neuDrive。', en: 'Add neuDrive in the editor MCP settings.' },
      { zh: '让编辑器 AI 读取当前 repo 并写入项目上下文。', en: 'Ask the editor AI to read the current repo and write project context.' },
      { zh: '后续可在 Claude 或 ChatGPT 复用这些项目资料。', en: 'Reuse that project material later from Claude or ChatGPT.' },
    ],
    detailLimits: [
      { zh: '不同编辑器的 MCP 配置字段可能不同，请使用对应配置。', en: 'MCP config fields differ by editor; use the matching config.' },
      { zh: '首次保存后建议到文件管理器检查写入路径。', en: 'After first save, check the written path in Data Explorer.' },
    ],
    detailFaq: [
      {
        question: { zh: 'Cursor 和 Windsurf 为什么放一起？', en: 'Why are Cursor and Windsurf grouped together?' },
        answer: { zh: '它们都属于代码编辑器接入，目标都是让 repo 上下文进入 neuDrive。', en: 'They are both coding editor setups, focused on bringing repo context into neuDrive.' },
      },
      {
        question: { zh: '会自动上传整个仓库吗？', en: 'Will it upload the entire repo automatically?' },
        answer: { zh: '不会。你需要让 AI 工具选择并写入有长期价值的上下文、文件或摘要。', en: 'No. You ask the agent to select and write durable context, files, or summaries.' },
      },
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
      zh: '先登录 neuDrive 官网，再复制对应 CLI 命令并验证连接。',
      en: 'First sign in to the neuDrive website, then copy the matching CLI commands and verify the connection.',
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
        title: { zh: '先登录官方 neuDrive', en: 'Sign in to official neuDrive' },
        copy: {
          zh: '运行这一条命令即可登录 neuDrive 官网账号。',
          en: 'Run this one command to sign in to your neuDrive website account.',
        },
        codes: [{
          label: { zh: 'neu hosted profile', en: 'neu hosted profile' },
          language: 'bash',
          value: 'neu login',
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
    detailSummary: {
      zh: 'CLI 接入适合日常在终端工作的用户，把 Codex、Claude Code 等命令行 AI 工具接到同一个 neuDrive 资料中枢。',
      en: 'CLI setup is for terminal-based work, connecting Codex, Claude Code, and similar agents to the same neuDrive data layer.',
    },
    detailHighlights: [
      { zh: '用 `neu login` 登录 neuDrive 官网账号。', en: 'Use `neu login` to sign in to your neuDrive website account.' },
      { zh: '为 Claude Code 或 Codex CLI 添加 remote MCP。', en: 'Add remote MCP for Claude Code or Codex CLI.' },
      { zh: '用 neu 命令导入、浏览和验证资料。', en: 'Use neu commands to import, browse, and verify material.' },
    ],
    detailLimits: [
      { zh: 'CLI 接入需要一个可访问的 HTTPS neuDrive 地址。', en: 'CLI setup needs a reachable HTTPS neuDrive URL.' },
      { zh: '不同 CLI 的登录命令不同，请按对应平台执行。', en: 'Login commands differ by CLI; use the commands for your platform.' },
    ],
    detailFaq: [
      {
        question: { zh: 'Codex CLI 和 Claude Code 都支持吗？', en: 'Are Codex CLI and Claude Code both supported?' },
        answer: { zh: '是。详情页和指南分别给出对应命令。', en: 'Yes. The detail and guide pages show separate commands for each.' },
      },
      {
        question: { zh: 'CLI 适合做什么？', en: 'What is CLI setup best for?' },
        answer: { zh: '适合导入项目资料、保存开发约定、同步备份，并让终端 AI 工具读取 neuDrive。', en: 'It is best for importing project material, saving conventions, syncing backups, and letting terminal agents read neuDrive.' },
      },
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
      zh: '适合自研 AI 工具、内部系统和自动化脚本。',
      en: 'For custom agents, internal systems, and automation scripts.',
    },
    demo: {
      zh: '为自定义 AI 工具创建访问凭证，复制服务地址，然后接入 MCP 或 REST API。',
      en: 'Create an access credential for a custom agent, copy the service URL, then connect to MCP or REST API.',
    },
    workflowSummary: {
      zh: '这条路线面向开发者。按用途创建连接即可。',
      en: 'This path is for developers. Create a connection for the intended use.',
    },
    guideTitle: {
      zh: '用 MCP / REST API 构建自定义 AI 工具',
      en: 'Build custom agents with MCP / REST API',
    },
    steps: [
      {
        title: { zh: '选择用途模板', en: 'Choose a purpose template' },
        copy: {
          zh: '进入 Settings -> Developer Access，选择 Custom Agent（自定义工具）。',
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
          zh: '复制下面的服务地址，填到你的 AI 工具或内部系统里。',
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
      zh: '让自定义 AI 工具读取 neuDrive 个人资料摘要，并返回读取结果。',
      en: 'Ask the custom agent to read a neuDrive profile summary and return the result.',
    },
    afterConnection: [
      { zh: '为每个 AI 工具单独创建凭证。', en: 'Create a separate credential for each agent.' },
      { zh: '在 Recent Activity 查看写入记录。', en: 'Review writes in Recent Activity.' },
      { zh: '不再使用时撤销连接。', en: 'Revoke the connection when it is no longer used.' },
    ],
    detailSummary: {
      zh: 'MCP / REST API 面向自研 AI 工具、内部系统和自动化脚本，用独立访问凭证连接 neuDrive。',
      en: 'MCP / REST API is for custom agents, internal systems, and automation scripts using separate access credentials.',
    },
    detailHighlights: [
      { zh: '按用途创建自定义工具凭证。', en: 'Create Custom Agent credentials by purpose.' },
      { zh: '接入 MCP 或 REST 服务地址。', en: 'Connect through the MCP or REST service URL.' },
      { zh: '可审计、轮换和撤销访问。', en: 'Access can be audited, rotated, and revoked.' },
    ],
    detailLimits: [
      { zh: 'API 接入需要你自己处理凭证保存和调用逻辑。', en: 'API setup requires you to handle credential storage and calls.' },
      { zh: '生产系统建议为每个 AI 工具单独创建凭证。', en: 'For production systems, create a separate credential for each agent.' },
    ],
    detailFaq: [
      {
        question: { zh: '什么时候用 API 而不是平台接入？', en: 'When should I use API instead of a platform setup?' },
        answer: { zh: '当你在构建自研 AI 工具、内部自动化或自定义系统时，API 更合适。', en: 'Use API when building custom agents, internal automation, or custom systems.' },
      },
      {
        question: { zh: 'Token 可以撤销吗？', en: 'Can tokens be revoked?' },
        answer: { zh: '可以。Developer Access 中可以查看、轮换和撤销凭证。', en: 'Yes. Developer Access lets you review, rotate, and revoke credentials.' },
      },
    ],
  },
]

function getIntegration(key?: string) {
  const normalized = (key || '').toLowerCase()
  if (normalized === 'browser') return undefined
  return integrations.find((item) => item.key === normalized || item.aliases?.includes(normalized))
}

function tr(tx: (zh: string, en: string) => string, text: LocalizedText) {
  return tx(text.zh, text.en)
}

function setMeta(selector: string, attr: string, value: string) {
  const element = document.querySelector(selector)
  if (element) element.setAttribute(attr, value)
}

function useDocumentTitle(title: string, description = DEFAULT_SEO_DESCRIPTION, robots = 'index, follow') {
  const location = useLocation()
  useEffect(() => {
    document.title = title
    const url = `${HUB_URL}${location.pathname === '/' ? '/' : location.pathname}`
    setMeta('meta[name="description"]', 'content', description)
    setMeta('meta[name="robots"]', 'content', robots)
    setMeta('link[rel="canonical"]', 'href', url)
    setMeta('meta[property="og:title"]', 'content', title)
    setMeta('meta[property="og:description"]', 'content', description)
    setMeta('meta[property="og:url"]', 'content', url)
    setMeta('meta[name="twitter:title"]', 'content', title)
    setMeta('meta[name="twitter:description"]', 'content', description)
  }, [description, location.pathname, robots, title])
}

function CopySnippet({ label, value, language = 'text' }: { label: string; value: string; language?: string }) {
  const { tx } = useI18n()
  const [copied, setCopied] = useState(false)
  const copyWithFallback = async () => {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
      return
    }
    const textarea = document.createElement('textarea')
    textarea.value = value
    textarea.setAttribute('readonly', '')
    textarea.style.position = 'fixed'
    textarea.style.left = '-9999px'
    textarea.style.top = '0'
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    document.body.removeChild(textarea)
  }
  const copy = async () => {
    try {
      await copyWithFallback()
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      setCopied(false)
    }
  }
  return (
    <div className="public-code-block">
      <div className="public-code-head">
        <span>{label}</span>
        <button type="button" onClick={copy} aria-live="polite">{copied ? tx('已复制 ✓', 'Copied ✓') : tx('复制', 'Copy')}</button>
      </div>
      <pre><code className={`language-${language}`}>{value}</code></pre>
    </div>
  )
}

export function PublicShell({ children, feedbackEnabled }: { children: ReactNode; feedbackEnabled?: boolean }) {
  const { tx } = useI18n()
  const contextFeedbackEnabled = usePublicFeedbackEnabled()
  const showFeedback = feedbackEnabled ?? contextFeedbackEnabled
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const closeMobileMenu = () => setMobileMenuOpen(false)
  const feedbackLoginHref = `/login?redirect=${encodeURIComponent('/?open_feedback=1')}`
  return (
    <div className={`public-site ${mobileMenuOpen ? 'public-menu-open' : ''}`}>
      <header className="public-nav">
        <Link to="/" className="public-brand" onClick={closeMobileMenu}>neuDrive</Link>
        <nav className="public-nav-links" aria-label={tx('主导航', 'Primary navigation')}>
          <a href="/#product">{tx('产品', 'Product')}</a>
          <Link to="/integrations">{tx('集成', 'Integrations')}</Link>
          <Link to="/pricing">{tx('价格', 'Pricing')}</Link>
          <Link to="/docs">{tx('文档', 'Docs')}</Link>
        </nav>
        <div className="public-nav-actions">
          {showFeedback && (
            <Link to={feedbackLoginHref} className="public-feedback-link" onClick={closeMobileMenu}>
              {tx('反馈', 'Feedback')}
            </Link>
          )}
          <GitHubRepoLink className="public-github-link" />
          <div className="public-nav-language"><LanguageToggle compact /></div>
          <Link to="/login" className="btn btn-outline public-login-link">{tx('登录', 'Log in')}</Link>
          <button
            type="button"
            className="public-menu-button"
            aria-label={mobileMenuOpen ? tx('关闭菜单', 'Close menu') : tx('打开菜单', 'Open menu')}
            aria-expanded={mobileMenuOpen}
            aria-controls="public-mobile-menu"
            onClick={() => setMobileMenuOpen((open) => !open)}
          >
            <span />
            <span />
            <span />
          </button>
        </div>
      </header>
      {mobileMenuOpen && (
        <div
          id="public-mobile-menu"
          className="public-mobile-menu"
        >
          <a href="/#product" onClick={closeMobileMenu}>{tx('产品', 'Product')}</a>
          <Link to="/integrations" onClick={closeMobileMenu}>{tx('集成', 'Integrations')}</Link>
          <Link to="/pricing" onClick={closeMobileMenu}>{tx('价格', 'Pricing')}</Link>
          <Link to="/docs" onClick={closeMobileMenu}>{tx('文档', 'Docs')}</Link>
          <Link to="/login" onClick={closeMobileMenu}>{tx('登录', 'Log in')}</Link>
          <div className="public-mobile-language">
            <LanguageToggle compact />
          </div>
        </div>
      )}
      {children}
      <PublicFooter />
    </div>
  )
}

function PublicFooter() {
  const { tx } = useI18n()
  const year = new Date().getFullYear()
  return (
    <footer className="public-footer">
      <div className="public-footer-main">
        <div className="public-footer-brand">
          <Link to="/" className="public-brand">neuDrive</Link>
          <p>{tx('让 Claude、ChatGPT、Cursor 等 AI 工具共用同一份记忆、文件和技能。', 'One shared memory, file, and skill layer for Claude, ChatGPT, Cursor, and other AI tools.')}</p>
          <a className="public-footer-support" href="mailto:support@neudrive.ai">support@neudrive.ai</a>
        </div>
        <nav className="public-footer-columns" aria-label={tx('页脚导航', 'Footer navigation')}>
          <div>
            <h2>{tx('产品', 'Product')}</h2>
            <Link to="/integrations">{tx('集成', 'Integrations')}</Link>
            <Link to="/pricing">{tx('价格', 'Pricing')}</Link>
            <a href="/#how-it-works">{tx('接入方式', 'How it works')}</a>
            <a href="/#product">{tx('能力', 'Capabilities')}</a>
          </div>
          <div>
            <h2>{tx('资源', 'Resources')}</h2>
            <Link to="/docs">{tx('文档', 'Docs')}</Link>
            <Link to="/guides/claude">Claude</Link>
            <Link to="/guides/chatgpt">ChatGPT</Link>
            <Link to="/guides/editors">Cursor / Windsurf</Link>
          </div>
          <div>
            <h2>{tx('公司', 'Company')}</h2>
            <a href="mailto:support@neudrive.ai">{tx('联系支持', 'Contact support')}</a>
            <a href="mailto:support@neudrive.ai?subject=neuDrive%20status">{tx('状态咨询', 'Status')}</a>
            <a href="https://github.com/agi-bar/neuDrive" target="_blank" rel="noreferrer">GitHub</a>
          </div>
          <div>
            <h2>{tx('法律', 'Legal')}</h2>
            <Link to="/privacy">{tx('隐私政策', 'Privacy')}</Link>
            <Link to="/terms">{tx('服务条款', 'Terms')}</Link>
            <a href="mailto:support@neudrive.ai?subject=neuDrive%20security">{tx('安全联系', 'Security contact')}</a>
          </div>
        </nav>
      </div>
      <div className="public-footer-bottom">
        <span>© {year} neuDrive</span>
        <span>{tx('AI 数据由你控制。', 'Your AI data stays under your control.')}</span>
      </div>
    </footer>
  )
}

export function MarketingHomePage() {
  const { tx, isZh } = useI18n()
  useDocumentTitle(
    tx('别让 AI 工具每次重新认识你 — neuDrive', 'Your AI tools should not have to meet you again every time — neuDrive'),
    tx('neuDrive 让 Claude、ChatGPT、Cursor 等 AI 工具共用同一份记忆、文件、技能和私密资料中枢。', DEFAULT_SEO_DESCRIPTION),
  )
  const [activeKey, setActiveKey] = useState('claude')
  const [activeHeroPanel, setActiveHeroPanel] = useState<'memory' | 'files' | 'skills'>('memory')
  const activeIntegration = getIntegration(activeKey) || integrations[0]
  const firstCode = activeIntegration.steps.flatMap((step) => step.codes || [])[0]
  const heroPanels = {
    memory: {
      label: tx('记忆', 'Memory'),
      source: 'Claude',
      prompt: tx('读取我的工作偏好和项目上下文。', 'Read my working preferences and project context.'),
      response: tx('返回已授权的个人记忆、项目资料和可用技能。', 'Returns approved profile memory, project files, and available skills.'),
      meta: [
        tx('长期偏好', 'Stable preferences'),
        tx('项目上下文', 'Project context'),
        tx('短期工作记忆', 'Scratch memory'),
      ],
    },
    files: {
      label: tx('文件', 'Files'),
      source: 'Cursor',
      prompt: tx('帮我找项目里关于接入配置的资料。', 'Find the project material related to setup configuration.'),
      response: tx('返回匹配的文件、来源和访问范围。', 'Returns matching files, sources, and access ranges from Data Explorer.'),
      meta: [
        tx('会话资料', 'Conversations'),
        tx('项目文件', 'Project files'),
        tx('可导出', 'Exportable'),
      ],
    },
    skills: {
      label: tx('技能', 'Skills'),
      source: 'ChatGPT',
      prompt: tx('看看我有哪些可复用的工作技能。', 'Show the reusable work skills I can use.'),
      response: tx('列出已启用的技能，并说明哪些 AI 工具可以使用。', 'Lists enabled Skills and which agents can use them.'),
      meta: [
        tx('接入一次', 'Register once'),
        tx('多工具复用', 'Reuse across agents'),
        tx('可验证', 'Testable'),
      ],
    },
  }
  const activeHero = heroPanels[activeHeroPanel]
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
    { name: tx('记忆', 'Memory'), label: tx('个人偏好 / 项目 / 临时记忆', 'Profile / Projects / Scratch'), copy: tx('保存个人偏好、项目上下文和短期工作记忆。', 'Store profile, project, and scratch working memory.') },
    { name: tx('文件', 'Files'), label: tx('数据浏览器', 'Data Explorer'), copy: tx('统一管理 AI 可读取的文件、会话和资料。', 'Manage files, conversations, and references your AI can read.') },
    { name: tx('技能', 'Skills'), label: tx('技能复用', '.skill routing'), copy: tx('接入一次，多个 AI 工具都能复用。', 'Register once and reuse across multiple agents.') },
    { name: tx('私密数据', 'Private data'), label: tx('访问控制', 'Access control'), copy: tx('决定每个 AI 工具能看到哪些资料。', 'Decide which material each AI tool can see.') },
  ]
  return (
    <PublicShell>
      <main>
        <section className="public-hero">
          <div className="public-hero-copy">
            <p className="public-kicker">{tx('AI 资料中枢', 'AI data layer')}</p>
            <h1 className="public-hero-title">
              {isZh ? (
                <>
                  <span>别让 AI 工具</span>
                  <span>每次重新认识你</span>
                </>
              ) : (
                'Your AI tools should not have to meet you again every time.'
              )}
            </h1>
            <p>
              {tx(
                'neuDrive 统一保存你的会话、项目资料、技能、文件和私密数据，再安全提供给 Claude、ChatGPT、Cursor、Windsurf 等 AI 工具。',
                'neuDrive stores your conversations, project context, skills, files, and private data in one layer for Claude, ChatGPT, Cursor, Windsurf, and more.',
              )}
            </p>
            <div className="public-hero-actions">
              <Link to="/signup" className="btn btn-primary">{tx('3 分钟连接第一个 AI 工具', 'Connect your first agent in 3 min')}</Link>
              <a href="#how-it-works" className="btn btn-outline">{tx('查看接入方式', 'See how it works')}</a>
            </div>
            <div className="public-hero-proof" aria-label={tx('产品能力', 'Product capabilities')}>
              <span>{tx('个人记忆', 'Profile memory')}</span>
              <span>{tx('访问权限控制', 'Private data control')}</span>
              <span>{tx('技能复用', 'Skill routing')}</span>
            </div>
            <a href="#product" className="public-scroll-cue">
              {tx('看看 neuDrive 怎么工作', 'Explore how the layer works')}
              <span />
            </a>
          </div>
          <div className="public-product-visual public-product-visual-rich" aria-label={tx('neuDrive 产品演示', 'neuDrive product demonstration')}>
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
              <div className="window-tabs" role="tablist" aria-label={tx('产品示例', 'Product examples')}>
                {(Object.keys(heroPanels) as Array<keyof typeof heroPanels>).map((key) => (
                  <button
                    key={key}
                    type="button"
                    role="tab"
                    aria-selected={activeHeroPanel === key}
                    className={activeHeroPanel === key ? 'active' : ''}
                    onClick={() => setActiveHeroPanel(key)}
                  >
                    {heroPanels[key].label}
                  </button>
                ))}
              </div>
              <div className="memory-thread" role="tabpanel">
                <div>
                  <span className="thread-label">{activeHero.source}</span>
                  <p>{activeHero.prompt}</p>
                </div>
                <div className="thread-response">
                  <span className="thread-label">neuDrive</span>
                  <p>{activeHero.response}</p>
                </div>
              </div>
              <div className="visual-access-row">
                {activeHero.meta.map((item) => <span key={item}>{item}</span>)}
              </div>
            </div>
          </div>
        </section>

        <section className="public-continuity-strip" aria-label={tx('neuDrive 产品流程', 'neuDrive product flow')}>
          <div>
            <span>01</span>
            <strong>{tx('连接一次', 'Connect once')}</strong>
            <p>{tx('把 Claude、ChatGPT、Cursor 等工具连接到同一个资料库。', 'Point Claude, ChatGPT, Cursor, and other tools at one data layer.')}</p>
          </div>
          <div>
            <span>02</span>
            <strong>{tx('控制访问', 'Control access')}</strong>
            <p>{tx('为每个 AI 工具决定能读取哪些资料，之后可以随时调整或撤销。', 'Decide which material each AI tool can read, then adjust or revoke it anytime.')}</p>
          </div>
          <div>
            <span>03</span>
            <strong>{tx('跨工具复用', 'Reuse everywhere')}</strong>
            <p>{tx('记忆、文件和技能跟着你走，而不是锁在某一个 AI 工具里。', 'Memory, files, and skills follow your workflow instead of staying locked inside one AI tool.')}</p>
          </div>
        </section>

        <section className="public-band pain-band">
          <div className="public-section-head stacked">
            <p className="public-kicker">{tx('为什么需要 neuDrive', 'Why neuDrive')}</p>
            <h2>{tx('上下文散落，是 AI 工作流最隐形的成本。', 'Scattered context is the hidden cost of AI workflows.')}</h2>
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
            <h2>{tx('一个地方管理给 AI 使用的资料。', 'One place for your AI working memory.')}</h2>
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
              <h2>{tx('3 分钟连接第一个 AI 工具。', '3 minutes to connect your first AI agent.')}</h2>
              <p>{tx('先选择你要连接的平台。每个平台都有步骤、可复制内容和连接后的测试问题。', 'Choose the tool you want to connect. The guide gives you the URL to copy, the settings path, and a test question for after connection.')}</p>
            </div>
            <Link to={`/guides/${activeIntegration.key}`} className="btn btn-outline">{tx('查看接入指南', 'Open demo guide')}</Link>
          </div>
          <div className="workflow-shell">
            <div className="workflow-picker" role="tablist" aria-label={tx('集成接入演示', 'Integration setup demos')}>
              {integrations.map((item) => (
                <button
                  key={item.key}
                  className={item.key === activeKey ? 'active' : ''}
                  type="button"
                  role="tab"
                  aria-label={tx(`显示 ${item.name} 接入演示`, `Show ${item.name} setup demo`)}
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
                <Link to={`/signup?platform=${activeIntegration.key}`} className="btn btn-primary">{tx('创建账号并接入', 'Create account to connect')}</Link>
              </div>
            </div>
          </div>
          <div className="workflow-mobile-accordion" aria-label={tx('移动端接入指南', 'Mobile setup guides')}>
            {integrations.map((item) => {
              const isActive = item.key === activeKey
              const itemCode = item.steps.flatMap((step) => step.codes || [])[0]
              return (
                <article key={item.key} className={`workflow-accordion-card ${isActive ? 'active' : ''}`}>
                  <button
                    type="button"
                    className="workflow-accordion-trigger"
                    aria-expanded={isActive}
                    aria-controls={`workflow-panel-${item.key}`}
                    onClick={() => setActiveKey(item.key)}
                  >
                    <span className="workflow-accordion-icon" aria-hidden="true">{item.accent}</span>
                    <span className="workflow-accordion-copy">
                      <strong>{item.shortName}</strong>
                      <small>{tr(tx, item.method)} · {tr(tx, item.setup)}</small>
                    </span>
                    <span className="workflow-accordion-caret" aria-hidden="true">⌄</span>
                  </button>
                  {isActive && (
                    <div id={`workflow-panel-${item.key}`} className="workflow-accordion-panel">
                      <p>{tr(tx, item.demo)}</p>
                      <p className="workflow-summary">{tr(tx, item.workflowSummary)}</p>
                      {itemCode && <CopySnippet label={tr(tx, itemCode.label)} language={itemCode.language} value={itemCode.value} />}
                      <div className="workflow-timeline compact">
                        {item.steps.map((step, index) => (
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
                        <Link to={`/guides/${item.key}`} className="btn btn-outline">{tx('查看完整指南', 'Open full guide')}</Link>
                        <Link to={`/signup?platform=${item.key}`} className="btn btn-primary">{tx('创建账号并接入', 'Create account to connect')}</Link>
                      </div>
                    </div>
                  )}
                </article>
              )
            })}
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
    <section className="public-band" id="integrations">
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
              <Link className="integration-guide-link" to={`/integrations/${item.key}`}>{tx('查看详情', 'View details')}</Link>
              <Link className="integration-connect-link" to={`/guides/${item.key}`}>{tx('接入指南', 'Setup guide')}</Link>
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
          <h3>{tx('Free 免费版', 'Free')}</h3>
          <div className="pricing-price">$0</div>
          <p>{tx('10 MiB 存储', '10 MiB storage')}</p>
        </article>
        <article className="pricing-public-card featured">
          <span className="recommended-chip">{tx('推荐', 'Recommended')}</span>
          <h3>{tx('Pro 年付', 'Pro Yearly')}</h3>
          <div className="pricing-price">{tx('$60 / 年', '$60 / year')}</div>
          <p>{tx('1 GiB 存储 · 自动同步 · Git 备份 · 优先导入', '1 GiB storage · Auto sync · Git backup · Priority import')}</p>
          <Link to="/signup?plan=pro_yearly" className="btn btn-primary">{tx('年付 Pro', 'Start Pro yearly')}</Link>
        </article>
        <article className="pricing-public-card">
          <h3>{tx('Pro 月付', 'Pro Monthly')}</h3>
          <div className="pricing-price">{tx('$10 / 月', '$10 / month')}</div>
          <p>{tx('按月灵活使用 Pro 能力。', 'Use Pro month to month.')}</p>
          <Link to="/signup?plan=pro_monthly" className="btn btn-outline">{tx('月付 Pro', 'Start Pro monthly')}</Link>
        </article>
      </div>
    </section>
  )
}

export function PricingPage() {
  const { tx } = useI18n()
  useDocumentTitle(
    tx('价格 — neuDrive', 'Pricing — neuDrive'),
    tx('比较 neuDrive Free 和 Pro 方案，了解存储空间、同步、备份和 AI 工具连接能力。', 'Compare neuDrive Free and Pro plans for storage, sync, backup, and AI tool connections.'),
  )
  const comparisonRows = [
    [tx('存储空间', 'Storage'), '10 MiB', '1 GiB', '1 GiB'],
    [tx('AI 工具连接', 'AI connections'), tx('不限连接', 'Unlimited'), tx('不限连接', 'Unlimited'), tx('不限连接', 'Unlimited')],
    [tx('同步方式', 'Sync'), tx('手动同步', 'Manual sync'), tx('自动同步', 'Auto sync'), tx('自动同步', 'Auto sync')],
    [tx('备份', 'Backup'), tx('ZIP 导出', 'ZIP export'), tx('ZIP + Git backup', 'ZIP + Git backup'), tx('ZIP + Git backup', 'ZIP + Git backup')],
    [tx('导入', 'Import'), tx('基础导入', 'Basic import'), tx('优先导入大批量会话', 'Priority large conversation import'), tx('优先导入大批量会话', 'Priority large conversation import')],
    [tx('私密数据控制', 'Private data controls'), tx('基础控制', 'Basic controls'), tx('完整控制', 'Full controls'), tx('完整控制', 'Full controls')],
  ]
  const faqItems = [
    {
      question: tx('我的数据属于谁？', 'Who owns my data?'),
      answer: tx(
        '你的记忆、文件、会话和技能属于你。neuDrive 负责存放和连接，你可以随时导出或删除。',
        'Your memory, files, conversations, and skills belong to you. neuDrive stores and connects them, and you can export or delete them anytime.',
      ),
    },
    {
      question: tx('存储空间满了会怎样？', 'What happens when storage is full?'),
      answer: tx(
        '导入和同步会停止写入新数据，已有数据仍可读取。你可以清理数据、导出备份，或升级到 Pro 获得 1 GiB 存储。',
        'Imports and sync stop writing new data, while existing data remains readable. You can clean up data, export a backup, or upgrade to Pro for 1 GiB storage.',
      ),
    },
    {
      question: tx('是否支持团队？', 'Do you support teams?'),
      answer: tx(
        '当前公开价格面向个人工作流。团队、企业部署、更高存储、审计和统一管理可以联系我们单独配置。',
        'The public plans are designed for individual workflows. Teams, enterprise deployment, higher storage, audit, and centralized administration can be arranged with us.',
      ),
    },
    {
      question: tx('可以自托管吗？', 'Can I self-host?'),
      answer: tx(
        '可以。neuDrive Core 是开源核心能力，自托管适合需要完全控制基础设施的团队；Hosted Pro 适合希望直接使用托管同步、备份和升级路径的用户。',
        'Yes. neuDrive Core is open-source and fits teams that want full infrastructure control; Hosted Pro is for users who want managed sync, backup, and an upgrade path.',
      ),
    },
    {
      question: tx('有试用或退款政策吗？', 'Is there a trial or refund policy?'),
      answer: tx(
        '你可以先用 Free 验证接入路径。若出现重复扣费、误购或账单异常，请联系 support@neudrive.ai，我们会根据实际情况处理。',
        'You can validate the setup path on Free first. For duplicate charges, accidental purchases, or billing issues, contact support@neudrive.ai and we will review the case.',
      ),
    },
  ]
  return (
    <PublicShell>
      <main className="public-simple">
        <PricingSection />
        <section className="public-band pricing-roi-section">
          <div className="pricing-roi-panel">
            <div>
              <p className="public-kicker">{tx('年付价值', 'Yearly value')}</p>
              <h2>{tx('年付 Pro 相当于每月 $5。', 'Pro Yearly is effectively $5 per month.')}</h2>
              <p>{tx('按月一年是 $120，年付是 $60。省下来的 $60 可以覆盖一整年的托管同步、备份和更大的 AI 工作记忆空间。', 'Monthly for a full year is $120; yearly is $60. The $60 saved covers a year of managed sync, backup, and a larger AI working-memory space.')}</p>
            </div>
            <div className="pricing-roi-metrics" aria-label={tx('年付节省', 'Yearly savings')}>
              <span>
                <strong>$60</strong>
                {tx('每年节省', 'saved yearly')}
              </span>
              <span>
                <strong>1 GiB</strong>
                {tx('托管存储', 'managed storage')}
              </span>
              <span>
                <strong>{tx('自动', 'Auto')}</strong>
                {tx('同步与备份', 'sync and backup')}
              </span>
            </div>
          </div>
        </section>
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
                  <th>{tx('Free 免费版', 'Free')}</th>
                  <th>{tx('Pro 年付', 'Pro Yearly')}</th>
                  <th>{tx('Pro 月付', 'Pro Monthly')}</th>
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
        <section className="public-band pricing-team-section">
          <article className="pricing-team-card">
            <div>
              <p className="public-kicker">{tx('团队与企业', 'Team and Enterprise')}</p>
              <h2>{tx('需要共享工作区、审计或更高存储？', 'Need shared workspaces, audit, or higher storage?')}</h2>
              <p>{tx('我们可以为团队配置更高存储、统一管理、部署支持和安全审查材料。', 'We can configure higher storage, centralized administration, deployment support, and security review materials for teams.')}</p>
            </div>
            <a className="btn btn-primary" href="mailto:support@neudrive.ai?subject=neuDrive%20Team%20or%20Enterprise">{tx('联系团队方案', 'Contact for teams')}</a>
          </article>
        </section>
        <section className="public-band self-host-section">
          <div className="public-section-head">
            <div>
              <p className="public-kicker">{tx('自托管', 'Self-hosting')}</p>
              <h2>{tx('开源核心可以自托管。', 'The open-source core can be self-hosted.')}</h2>
              <p>{tx('如果你只需要核心资料中枢、MCP 接入和本地控制，可以部署 neuDrive Core。Hosted Pro 则适合想减少运维、直接使用托管同步和备份的个人用户。', 'If you only need the core data layer, MCP access, and local control, you can deploy neuDrive Core. Hosted Pro is for individual users who want less operations work and managed sync and backup.')}</p>
            </div>
            <Link to="/docs" className="btn btn-outline">{tx('查看部署文档', 'View deployment docs')}</Link>
          </div>
        </section>
        <section className="public-band pricing-faq-section">
          <div className="public-section-head">
            <div>
              <p className="public-kicker">FAQ</p>
              <h2>{tx('购买前常见问题', 'Questions before you buy')}</h2>
            </div>
          </div>
          <div className="public-faq-grid">
            {faqItems.map((item) => (
              <article key={item.question} className="public-faq-item">
                <h3>{item.question}</h3>
                <p>{item.answer}</p>
              </article>
            ))}
          </div>
        </section>
      </main>
    </PublicShell>
  )
}

export function IntegrationsPage() {
  const { tx } = useI18n()
  useDocumentTitle(
    tx('集成 — neuDrive', 'Integrations — neuDrive'),
    tx('连接 Claude、ChatGPT、Cursor、Windsurf、命令行 AI 工具、MCP 和 REST API。', 'Connect neuDrive to Claude, ChatGPT, Cursor, Windsurf, CLI agents, MCP, and REST API.'),
  )
  return (
    <PublicShell>
      <main className="public-simple">
        <section className="public-band integrations-hero-section">
          <p className="public-kicker">{tx('集成目录', 'Integration catalog')}</p>
          <h1>{tx('选择你要把 neuDrive 接到哪里。', 'Choose where neuDrive should connect.')}</h1>
          <p>{tx('每个平台详情页都会说明适用场景、接入步骤、限制和常见问题。', 'Each platform detail page explains use cases, setup steps, limitations, and common questions.')}</p>
        </section>
        <section className="public-band integration-directory-section">
          <div className="integration-directory-grid">
            {integrations.map((item) => (
              <article key={item.key} className="integration-directory-card">
                <div className="integration-directory-top">
                  <div className="integration-icon" aria-hidden="true">{item.accent}</div>
                  <span>{tr(tx, item.method)} · {tr(tx, item.setup)}</span>
                </div>
                <h2>{item.name}</h2>
                <p>{tr(tx, item.detailSummary || item.demo)}</p>
                <ul>
                  {(item.detailHighlights || item.afterConnection).slice(0, 3).map((highlight) => (
                    <li key={tr(tx, highlight)}>{tr(tx, highlight)}</li>
                  ))}
                </ul>
                <div className="integration-directory-actions">
                  <Link className="btn btn-primary" to={`/integrations/${item.key}`}>{tx('查看详情', 'View details')}</Link>
                  <Link className="btn btn-outline" to={`/guides/${item.key}`}>{tx('接入指南', 'Setup guide')}</Link>
                </div>
              </article>
            ))}
          </div>
        </section>
      </main>
    </PublicShell>
  )
}

export function IntegrationDetailPage() {
  const { platform } = useParams()
  const { tx } = useI18n()
  const item = getIntegration(platform)
  useDocumentTitle(
    item
      ? tx(`${item.shortName} 集成 — neuDrive`, `${item.shortName} Integration — neuDrive`)
      : tx('集成 — neuDrive', 'Integrations — neuDrive'),
    item
      ? tx(`了解如何把 ${item.shortName} 连接到 neuDrive，共用记忆、文件和技能。`, `Learn how to connect ${item.shortName} to neuDrive so it can use shared memory, files, and skills.`)
      : tx('选择 Claude、ChatGPT、Cursor、Windsurf、CLI 或 API 接入 neuDrive。', 'Choose Claude, ChatGPT, Cursor, Windsurf, CLI, or API setup for neuDrive.'),
  )
  if (!item) return <Navigate to="/integrations" replace />
  const codeSnippets = item.steps.flatMap((step) => step.codes || [])
  const previewCode = codeSnippets[0]
  return (
    <PublicShell>
      <main className="public-simple integration-detail-page">
        <section className="public-band integration-detail-hero">
          <div>
            <p className="public-kicker">{tx('集成详情', 'Integration detail')}</p>
            <h1>{item.name}</h1>
            <p>{tr(tx, item.detailSummary || item.demo)}</p>
            <div className="public-hero-actions">
              <Link to={`/guides/${item.key}`} className="btn btn-primary">{tx('打开接入指南', 'Open setup guide')}</Link>
              <Link to={`/signup?platform=${item.key}`} className="btn btn-outline">{tx('创建账号并接入', 'Create account to connect')}</Link>
            </div>
          </div>
          <div className="integration-preview-panel">
            <div className="window-bar"><span /><span /><span /></div>
            <div className="integration-preview-head">
              <strong>{tr(tx, item.method)}</strong>
              <span>{tr(tx, item.setup)}</span>
            </div>
            <div className="integration-preview-flow">
              {item.steps.slice(0, 4).map((step, index) => (
                <div key={tr(tx, step.title)}>
                  <span>{index + 1}</span>
                  <p>{tr(tx, step.title)}</p>
                </div>
              ))}
            </div>
            {previewCode && (
              <div className="integration-preview-code">
                <span>{tr(tx, previewCode.label)}</span>
                <code>{previewCode.value}</code>
              </div>
            )}
          </div>
        </section>

        <section className="public-band integration-detail-grid-section">
          <article className="integration-detail-card">
            <h2>{tx('适合什么场景', 'Best for')}</h2>
            <ul>
              {(item.detailHighlights || item.afterConnection).map((highlight) => (
                <li key={tr(tx, highlight)}>{tr(tx, highlight)}</li>
              ))}
            </ul>
          </article>
          <article className="integration-detail-card">
            <h2>{tx('限制和注意事项', 'Limits and notes')}</h2>
            <ul>
              {(item.detailLimits || item.limits || []).map((limit) => (
                <li key={tr(tx, limit)}>{tr(tx, limit)}</li>
              ))}
            </ul>
          </article>
        </section>

        <section className="public-band integration-detail-faq-section">
          <div className="public-section-head">
            <div>
              <p className="public-kicker">Q&A</p>
              <h2>{tx('接入前常见问题', 'Questions before setup')}</h2>
            </div>
          </div>
          <div className="public-faq-grid">
            {(item.detailFaq || []).map((faq) => (
              <article key={tr(tx, faq.question)} className="public-faq-item">
                <h3>{tr(tx, faq.question)}</h3>
                <p>{tr(tx, faq.answer)}</p>
              </article>
            ))}
          </div>
        </section>
      </main>
    </PublicShell>
  )
}

export function GuidePage() {
  const { platform } = useParams()
  const { tx } = useI18n()
  const guide = getIntegration(platform)
  useDocumentTitle(
    guide
      ? tx(`${guide.shortName} 接入指南 — neuDrive`, `${guide.shortName} Setup Guide — neuDrive`)
      : tx('接入指南 — neuDrive', 'Setup Guide — neuDrive'),
    guide
      ? tx(`${guide.shortName} 接入 neuDrive 的设置步骤、可复制内容、授权流程和测试问题。`, `Follow the neuDrive setup guide for ${guide.shortName}, including copyable URLs, authorization steps, and a test prompt.`)
      : tx('选择 Claude、ChatGPT、Cursor、Windsurf、CLI 或 API 接入 neuDrive。', 'Choose Claude, ChatGPT, Cursor, Windsurf, CLI, or API setup for neuDrive.'),
  )
  if (!guide) return <Navigate to="/integrations" replace />
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
              <Link to={`/signup?platform=${guide.key}`} className="btn btn-primary">{tx('创建账号开始接入', 'Create account for this setup')}</Link>
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
  useDocumentTitle(
    tx('文档 — neuDrive', 'Docs — neuDrive'),
    tx('neuDrive 文档包含 Claude、ChatGPT、代码编辑器、CLI 和自定义 MCP 客户端接入指南。', 'Setup guides for connecting neuDrive to Claude, ChatGPT, coding editors, CLI agents, and custom MCP clients.'),
  )
  const docsGroups = [
    {
      title: tx('快速开始', 'Quick start'),
      copy: tx('先确认你要接入的是网页平台、代码编辑器、CLI 还是开发者 API。', 'First decide whether you are connecting a web platform, coding editor, CLI, or developer API.'),
      links: [
        { label: tx('Claude Connectors 接入', 'Claude Connectors'), href: '/guides/claude' },
        { label: tx('ChatGPT Apps 接入', 'ChatGPT Apps'), href: '/guides/chatgpt' },
      ],
    },
    {
      title: tx('代码工作流', 'Coding workflows'),
      copy: tx('Cursor、Windsurf 和命令行 AI 工具主要用于读取当前 repo，并把项目上下文写回 neuDrive。', 'Cursor, Windsurf, and CLI agents are best for reading the current repo and writing project context back to neuDrive.'),
      links: [
        { label: tx('Cursor / Windsurf 编辑器', 'Cursor / Windsurf'), href: '/guides/editors' },
        { label: tx('Codex / Claude CLI 命令行', 'Codex / Claude CLI'), href: '/guides/cli' },
      ],
    },
    {
      title: tx('开发者接入', 'Developer access'),
      copy: tx('API 适合自定义 AI 工具、内部系统和自动化任务。', 'API setup is for custom agents, internal systems, and automation.'),
      links: [
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

const legalContent = {
  privacy: {
    kicker: { zh: '隐私', en: 'Privacy' },
    title: { zh: '隐私政策', en: 'Privacy Policy' },
    intro: {
      zh: 'neuDrive 用来保存 AI 工具需要读取的记忆、文件、会话、技能和私密资料。我们的产品原则是：你能看到保存了什么，能导出，能撤销访问，也能删除数据。',
      en: 'neuDrive stores memory, files, conversations, skills, and private material that your AI tools may need to read. Our product principle is simple: you can see what is stored, export it, revoke access, and delete data.',
    },
    sections: [
      {
        title: { zh: '我们保存什么', en: 'What we store' },
        body: {
          zh: '你主动导入或创建的资料，例如 profile memory、项目上下文、conversation、skill、文件、Vault 条目和连接授权记录。',
          en: 'Material you import or create, such as profile memory, project context, conversations, skills, files, Vault entries, and connection authorization records.',
        },
      },
      {
        title: { zh: '你如何控制访问', en: 'How you control access' },
        body: {
          zh: '每个连接的 AI 工具只应读取被授权的资料。你可以在产品内调整访问范围、撤销连接、删除 token 或导出数据。',
          en: 'Each connected AI tool should only read material that has been authorized. You can adjust access, revoke connections, delete tokens, or export data from the product.',
        },
      },
      {
        title: { zh: '联系我们', en: 'Contact us' },
        body: {
          zh: '隐私、安全或数据删除请求可以发送到 support@neudrive.ai。',
          en: 'Privacy, security, or data deletion requests can be sent to support@neudrive.ai.',
        },
      },
    ],
  },
  terms: {
    kicker: { zh: '条款', en: 'Terms' },
    title: { zh: '服务条款', en: 'Terms of Service' },
    intro: {
      zh: '使用 neuDrive 时，你需要确保导入的数据、连接的 AI 工具和创建的 token 符合你的组织规则和适用法律。',
      en: 'When using neuDrive, you are responsible for ensuring that imported data, connected AI tools, and created tokens comply with your organization’s rules and applicable law.',
    },
    sections: [
      {
        title: { zh: '账户和访问', en: 'Accounts and access' },
        body: {
          zh: '你负责保护账号、token 和连接授权。发现异常访问时，请尽快撤销相关连接并联系我们。',
          en: 'You are responsible for protecting your account, tokens, and connection authorizations. If you notice unexpected access, revoke the related connection and contact us.',
        },
      },
      {
        title: { zh: '数据和导出', en: 'Data and export' },
        body: {
          zh: 'neuDrive 旨在帮助你集中管理 AI 工作数据。你可以导出数据；删除或撤销操作可能影响已连接 AI 工具的可用上下文。',
          en: 'neuDrive is designed to help you manage AI working data in one place. You can export data; deletion or revocation may affect context available to connected AI tools.',
        },
      },
      {
        title: { zh: '付费和支持', en: 'Billing and support' },
        body: {
          zh: '托管版的支付、订阅和存储限制由 neuDrive 的支付网关处理。账单或支持问题可以发送到 support@neudrive.ai。',
          en: 'Hosted billing, subscriptions, and storage limits are handled by the neuDrive payment gateway. Billing or support questions can be sent to support@neudrive.ai.',
        },
      },
    ],
  },
}

export function LegalPage({ kind }: { kind: 'privacy' | 'terms' }) {
  const { tx } = useI18n()
  const page = legalContent[kind]
  useDocumentTitle(
    kind === 'privacy' ? tx('隐私 — neuDrive', 'Privacy — neuDrive') : tx('条款 — neuDrive', 'Terms — neuDrive'),
    tr(tx, page.intro),
  )
  return (
    <PublicShell>
      <main className="public-simple legal-page">
        <section className="public-band legal-hero">
          <p className="public-kicker">{tr(tx, page.kicker)}</p>
          <h1>{tr(tx, page.title)}</h1>
          <p>{tr(tx, page.intro)}</p>
        </section>
        <section className="public-band legal-section-list">
          {page.sections.map((section) => (
            <article key={tr(tx, section.title)} className="legal-section-card">
              <h2>{tr(tx, section.title)}</h2>
              <p>{tr(tx, section.body)}</p>
            </article>
          ))}
        </section>
      </main>
    </PublicShell>
  )
}

export function SignupPage() {
  const { tx } = useI18n()
  useDocumentTitle(tx('注册 — neuDrive', 'Sign up — neuDrive'), DEFAULT_SEO_DESCRIPTION, 'noindex, nofollow')
  const [providers, setProviders] = useState<AuthProvider[]>([])
  const [error, setError] = useState('')
  const [loadingAction, setLoadingAction] = useState('')

  useEffect(() => {
    api.getAuthProviders().then(setProviders).catch(() => setProviders([]))
  }, [])

  const githubProvider = providers.find((provider) => provider.id === 'github')
  const pocketProvider = providers.find((provider) => provider.kind === 'oidc')
  const githubEnabled = !!githubProvider?.enabled
  const pocketEnabled = !!pocketProvider?.enabled
  const busy = loadingAction !== ''

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

  return (
    <PublicShell>
      <main className="auth-split">
        <section className="auth-copy">
          <p className="public-kicker">{tx('创建账号', 'Create account')}</p>
          <h1>{tx('3 分钟连接第一个 AI 工具。', 'Connect your first AI tool in 3 minutes.')}</h1>
        </section>
        <section className="auth-card">
          {error && <div className="alert alert-warn">{error}</div>}
          <button className="btn btn-primary btn-block" disabled={busy || !githubEnabled} onClick={() => { void beginProviderSignup(githubProvider, 'login', 'github') }}>
            {loadingAction === 'github' ? tx('跳转中...', 'Redirecting...') : tx('使用 GitHub 继续', 'Continue with GitHub')}
          </button>
          <button className="btn btn-outline btn-block" disabled={busy || !pocketEnabled} onClick={() => { void beginProviderSignup(pocketProvider, 'signup', 'pocket') }}>
            {loadingAction === 'pocket' ? tx('跳转中...', 'Redirecting...') : tx('邮箱登录 / 注册', 'Continue with email')}
          </button>
          <p className="login-note">
            {tx('已有账户？', 'Already have an account?')} <Link to="/login">{tx('登录', 'Log in')}</Link>
          </p>
        </section>
      </main>
    </PublicShell>
  )
}
