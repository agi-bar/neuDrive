# Agent Hub

**AI 时代的身份与信任基础设施**

> 让所有 AI Agent 认识你、记住你、代表你行动

---

## 这是什么

Agent Hub 是一个独立的中间层服务。每个人拥有一个 Hub，所有 AI Agent（Claude、ChatGPT、Codex、Cursor、Copilot、飞书、Kimi、智谱……）通过这个 Hub 共享身份、记忆、能力和通信。

**你的身份、偏好、秘密、技能跟着人走，不跟平台走。**

底层统一为一棵 canonical virtual tree，对外同时提供 typed API、文件树读写和 `snapshot/changes` 同步接口。

- 上午在 Claude 写的文章风格偏好，下午切到 GPT 自动生效
- 存在保险柜里的 API Key，被授权的 Agent 可以安全调用
- 你的 Agent 之间可以发邮件、协作、交接任务——你不需要当传话筒
- 一个 Hub ID，通行所有 AI 平台

类比：Google Login 之于 Web、Apple ID 之于移动设备、Stripe 之于支付——Agent Hub 是 AI Agent 世界的信任层。

详见 [设计文档](docs/design.md)

---

## 快速开始

### Docker 一键启动

```bash
cp agenthub.env.example agenthub.env
# 编辑 agenthub.env，填入你的 GitHub OAuth 和密钥配置
docker compose --env-file agenthub.env up
```

服务启动在 `http://localhost:8080`，管理后台直接访问即可。

### 本地开发

```bash
# 后端
go run ./cmd/server/

# 前端（另一个终端）
cd web && npm install && npm run dev
```

或者用 Makefile：

```bash
make dev    # 同时启动后端和前端开发服务器
make build  # 构建生产版本（前端嵌入 Go 二进制）
make test   # 运行所有测试
```

### 连接常见平台

推荐先打开管理后台的“连接设置”页面。页面里现在分成七个独立入口：

- **Web / Desktop Apps**：在 Claude Connectors、ChatGPT Apps、Cursor Desktop 和 Windsurf Desktop 中直接添加远程 MCP Server
- **CLI Apps（推荐）**：Claude Code、Codex CLI、Gemini CLI、Cursor Agent 通过远程 HTTP MCP + 浏览器 OAuth 授权接入
- **Adapters**：通过 webhook / bot / 事件回调接入飞书、钉钉、Slack 等工作区平台
- **本地模式**：通过本地 `agenthub-mcp` binary + scoped token 接入，推荐用环境变量保存 token
- **高级模式**：给任意 MCP 客户端提供 HTTP + Bearer Token 配置，推荐优先使用环境变量
- **ChatGPT Actions**：在自定义 GPT 中通过 OpenAPI + Bearer Token 接入
- **Token 管理**：集中创建、改名、吊销各类 token

Web / Desktop Apps：

```text
Claude 网页应用
1. Settings -> Connectors -> Go to Customize
2. Add custom connector
3. Remote MCP Server URL 填写 https://hub.example.com/mcp
4. 点击 Connect，完成 Agent Hub 登录和授权

ChatGPT 网页应用
1. Settings -> Apps
2. Advanced settings -> Create app
3. MCP Server URL 填写 https://hub.example.com/mcp
4. 按提示完成登录和授权

Cursor Desktop
1. Settings -> Tools & MCPs -> Add Custom MCP
2. Remote MCP Server URL 填写 https://hub.example.com/mcp
3. 点击 Connect / Authenticate
4. 完成 Agent Hub 登录和授权

Windsurf Desktop
1. Windsurf Settings -> Cascade
2. 在 MCP Servers 下点击 Open MCP Marketplace
3. 点击 config 图标，打开 ~/.codeium/windsurf/mcp_config.json
4. 写入 { "mcpServers": { "agenthub": { "serverUrl": "https://hub.example.com/mcp" } } }
5. 保存后点击 Open，完成 Agent Hub 登录和授权
```

CLI Apps（推荐）：

```bash
# Claude Code
claude mcp add -s user --transport http agenthub https://hub.example.com/mcp
# 然后在 Claude Code 中执行 /mcp 完成浏览器授权

# Codex CLI
codex mcp add agenthub --url https://hub.example.com/mcp
codex mcp login agenthub

# Gemini CLI
gemini mcp add --transport http agenthub https://hub.example.com/mcp
# 然后在 Gemini 中执行 /mcp auth agenthub

# Cursor Agent
# 先在 .cursor/mcp.json 或 ~/.cursor/mcp.json 中写入:
# { "mcpServers": { "agenthub": { "url": "https://hub.example.com/mcp" } } }
cursor-agent mcp login agenthub
cursor-agent mcp list
```

Adapters：

```text
Feishu Bot Adapter
1. 在飞书开放平台创建自建应用，并开启机器人能力
2. 订阅「消息与群组 -> 接收消息 v2.0」
3. 选择「将事件发送至开发者服务器」
4. 请求网址填写 https://hub.example.com/api/adapters/feishu/<your-slug>/events
5. 服务端配置 FEISHU_APP_ID、FEISHU_APP_SECRET、FEISHU_VERIFICATION_TOKEN
6. 推荐同时配置 FEISHU_ENCRYPT_KEY，启用签名校验与事件解密
7. 验证通过后，飞书消息会进入 Agent Hub 的 Inbox；如果同时配置了 App ID / Secret，还会自动回一条确认消息
```

本地模式（stdio + scoped token）：

```bash
# 先在“连接设置”页面生成一个 scoped token
export AGENTHUB_TOKEN=aht_xxxxx

# Claude Code
claude mcp add -s user agenthub -- agenthub-mcp --token-env AGENTHUB_TOKEN

# Codex CLI
codex mcp add agenthub -- agenthub-mcp --token-env AGENTHUB_TOKEN
```

高级模式（HTTP + 静态 Bearer / 环境变量）：

```bash
# 推荐：先把 Bearer Token 存进环境变量
export AGENTHUB_TOKEN=aht_xxxxx

# Codex CLI 可直接引用环境变量，不把 secret 写进配置
codex mcp add agenthub --url https://hub.example.com/mcp --bearer-token-env-var AGENTHUB_TOKEN

# 其他客户端如果暂不支持 env 方式，再退回静态 Bearer header
```

接入后，支持远程 OAuth 的客户端会自动发现 Agent Hub 的 MCP 工具；授权完成后，对应平台连接也会出现在“连接管理”中。

### Bundle Sync（迁移 / 备份 / 恢复）

对于大 skill、长文档、PNG/PDF 等二进制资源，推荐走 Bundle Sync，而不是让 AI 逐文件通过 MCP tool 写入。

- UI 入口：管理后台的数据同步页面
- CLI 入口：`python3 tools/ahub-sync.py`
- 文档：
  - [Bundle Sync 指南](docs/sync.md)
  - [Prod-like 验收 Runbook](docs/sync-prodlike-acceptance.md)
  - [安全与资源审计](docs/sync-audit.md)

最短链路：

```bash
# 1. 登录一次，保存默认 profile（浏览器会打开到 /data/sync）
python3 tools/ahub-sync.py login --api-base https://agenthub.agi.bar

# 2. 本地导出 bundle
python3 tools/ahub-sync.py export --source /path/to/skills --format archive -o backup.ahubz

# 3. 预览并导入
python3 tools/ahub-sync.py preview --bundle backup.ahubz
python3 tools/ahub-sync.py push --bundle backup.ahubz --transport auto

# 4. 从 Hub 拉回并比对
python3 tools/ahub-sync.py pull --format archive -o pulled.ahubz
python3 tools/ahub-sync.py diff --left backup.ahubz --right pulled.ahubz
```

### 连接 ChatGPT Actions

1. 打开 ChatGPT → 创建 GPT → Configure → Actions
2. 粘贴 Agent Hub 的 OpenAPI schema（在管理后台“连接设置”页面获取）
3. 配置 Bearer Token 认证（可在同一页面创建或复用 token）
4. 完成

---

## 六大核心能力

### 1. 统一身份

一个 ID 通行所有 Agent 平台。支持邮箱密码注册、GitHub OAuth 登录、OAuth 2.0 Provider（第三方应用可以使用"Sign in with Agent Hub"）。

### 2. 上下文漫游

三层记忆系统：
- **Profile 层**：稳定偏好（写作风格、沟通习惯、做事原则），极少变动
- **Projects 层**：按项目组织的上下文和结构化日志，自动生成摘要
- **Scratch 层**：按条目归档的短期工作记忆，默认 7 天自动衰减

不同平台捕获矛盾偏好时，系统自动检测冲突并在管理后台提示用户决策。

### 3. 秘密管理

AES-256-GCM 加密保险柜。API Key、身份证号、银行卡信息安全存储。四级信任等级控制访问：

| 等级 | 名称 | 典型场景 |
|------|------|---------|
| L4 | 完全信任 | 你的主力 AI 助手（Claude） |
| L3 | 工作信任 | 日常使用的其他 AI 平台 |
| L2 | 协作 | 帮朋友干活、跨组织合作 |
| L1 | 访客 | 第三方 Agent、陌生人 |

低等级的 Agent 看到的文件树是裁剪过的——不是"没有权限"，是"根本不存在"。

### 4. 能力路由

`.skill` 文件统一注册。Agent 进来后读目录发现有什么可用，读 `SKILL.md` 知道怎么调用；服务端会索引 frontmatter 中的 `description`、`when_to_use`、`allowed_tools`、`tags`、`arguments`、`activation` 等字段。支持批量导入 Claude 的 `.skill` 目录和记忆导出。

### 5. Agent 通信

Agent 之间可以发邮件。消息三层结构：信封（路由）→ 元数据（不读正文就能决策）→ 内容（自包含，收件方无需前置信息）。

通信记录自动成为可搜索的记忆存档——用户问"Q2 预算当时怎么调的"，Agent 能在邮件存档里找到答案。

### 6. 设备控制

智能设备统一注册为 skill。每个设备的 SKILL.md 描述支持哪些操作，Hub 负责翻译成具体协议调用。

---

## 平台支持

### 接入方式支持

| 接入对象 | 接入方式 | 状态 |
|----------|---------|------|
| **Claude 网页应用** | Connectors + Remote MCP | ✅ 可用 |
| **Claude Code** | MCP (HTTP + OAuth / stdio) | ✅ 可用 |
| **Claude Desktop** | MCP (stdio) | ✅ 可用 |
| **Codex CLI** | MCP (HTTP + OAuth / stdio) | ✅ 可用 |
| **Gemini CLI** | MCP (HTTP + OAuth) | ✅ 可用 |
| **Cursor Agent** | MCP (HTTP + OAuth) | ✅ 可用 |
| **Cursor Desktop** | MCP (Remote HTTP + OAuth / stdio) | ✅ 可用 |
| **Windsurf Desktop** | MCP (Remote HTTP + OAuth / stdio) | ✅ 可用 |
| **Feishu Bot Adapter** | Webhook + Event Subscription | 🟡 Beta |
| **ChatGPT Apps** | Remote MCP | ✅ 可用 |
| **ChatGPT** | GPT Actions + OpenAPI | ✅ 可用 |
| **任意 MCP 客户端** | HTTP MCP + Bearer Token | ✅ 可用 |
| **任意 HTTP/REST 客户端** | HTTP REST API | ✅ 可用 |
| **JavaScript 应用** | `@agenthub/sdk` | ✅ 可用 |
| **Python 应用** | `agenthub-sdk` | ✅ 可用 |
| **浏览器扩展** | Chrome/Edge 扩展 | ✅ 可用 (Claude/ChatGPT/Gemini/Kimi) |

这张表只回答“接不接得上 AgentHub”，不展开平台数据的迁移、备份和恢复能力。

补充说明：

- `Cursor Desktop` 现已验证 Remote MCP + OAuth 直连，可通过 Cursor 的 `Tools & MCPs -> Add Custom MCP` 或 `~/.cursor/mcp.json` 接入
- `Windsurf Desktop` 现已验证 Remote MCP + OAuth 直连，可通过 `Windsurf Settings -> Cascade -> Open MCP Marketplace` 后编辑 `~/.codeium/windsurf/mcp_config.json` 接入
- `任意 MCP 客户端` 这一路径仍覆盖能手工配置 MCP 的产品，例如 `GitHub Copilot` 等
- 除已验证的 Cursor / Windsurf 外，其余通用 MCP 客户端当前仍缺少 AgentHub 专用向导，但可以作为优先目标平台纳入后续 portability 和 setup 文档

### 平台能力进度矩阵

状态图例：

- `✅ 已可用`
- `🟡 部分可用`
- `📝 手动/文档方式`
- `🔜 计划中`
- `❌ 未开始`

| 平台 | Hub 连接 | 浏览器扩展 | 导入到 AgentHub | 从 AgentHub 导出/恢复 | Portability 手册 | 当前阶段 |
|------|----------|------------|-----------------|------------------------|------------------|----------|
| **Claude** | ✅ | ✅ | 🟡 | 📝 | 🔜 | 连接成熟，导入领先 |
| **ChatGPT** | ✅ | ✅ | 🔜 | 📝 | 🔜 | 连接成熟，portability 待补 |
| **Codex** | ✅ | ❌ | 🔜 | 📝 | 🔜 | 连接成熟，portability 待补 |
| **Gemini** | ✅ | ✅ | 🔜 | 🔜 | 🔜 | CLI + 浏览器侧支持 |
| **Cursor** | ✅ | ❌ | 🔜 | 🔜 | 🔜 | Desktop / CLI Remote MCP 已验证 |
| **GitHub Copilot** | 📝 | ❌ | 🔜 | 🔜 | 🔜 | 通用 MCP 可接入，等待专用手册 |
| **Windsurf** | ✅ | ❌ | 🔜 | 🔜 | 🔜 | Desktop Remote MCP 已验证 |
| **Perplexity** | ❌ | ❌ | 🔜 | 🔜 | 🔜 | 目标平台 |
| **Kimi** | ❌ | ✅ | 🔜 | 🔜 | 🔜 | 浏览器侧支持 |
| **DeepSeek** | ❌ | ❌ | 🔜 | 🔜 | 🔜 | 浏览器 / HTTP 目标平台 |
| **Qwen / 通义** | ❌ | ❌ | 🔜 | 🔜 | 🔜 | 浏览器 / HTTP 目标平台 |
| **智谱 GLM** | ❌ | ❌ | 🔜 | 🔜 | 🔜 | 浏览器 / HTTP 目标平台 |
| **MiniMax** | ❌ | ❌ | 🔜 | 🔜 | 🔜 | 浏览器 / HTTP 目标平台 |
| **飞书** | 🟡 | ❌ | 🔜 | 🔜 | 🔜 | Bot Adapter MVP |
| **钉钉** | 🔜 | ❌ | 🔜 | 🔜 | 🔜 | Adapter 计划中 |
| **Slack** | 🔜 | ❌ | 🔜 | 🔜 | 🔜 | Workspace Adapter 目标 |
| **Discord** | 🔜 | ❌ | 🔜 | 🔜 | 🔜 | Workspace Adapter 目标 |
| **Microsoft Teams** | 🔜 | ❌ | 🔜 | 🔜 | 🔜 | Workspace Adapter 目标 |

说明：

- `Hub 连接`：平台能否直接把 AgentHub 当外部能力来用
- `导入到 AgentHub`：平台自身数据能否被迁入 AgentHub
- `从 AgentHub 导出/恢复`：AgentHub 数据能否被重建为该平台可消费的形态
- `Portability 手册`：指系统级只读 skill 形式的平台迁移说明，计划路径为：
  - `/skills/portability/claude/SKILL.md`
  - `/skills/portability/chatgpt/SKILL.md`
  - `/skills/portability/codex/SKILL.md`

当前判断依据：

- `Claude` 已支持 `Claude memory` 和 `Claude exported data zip` 导入，因此“导入到 AgentHub”标记为 `🟡`
- `ChatGPT` 与 `Codex` 当前已具备稳定连接方式，但恢复仍以通用导出 + 手工映射为主，因此“从 AgentHub 导出/恢复”标记为 `📝`
- `Cursor Desktop` 与 `Cursor Agent` 都已完成 Remote MCP + OAuth 连接验证，因此 `Hub 连接` 标记为 `✅`；后续仍需要补 portability 手册
- `Windsurf Desktop` 已完成 Remote MCP + OAuth 连接验证，因此 `Windsurf` 的 `Hub 连接` 标记为 `✅`
- `飞书` 当前先提供 `Webhook + Event Subscription` 的 Bot Adapter MVP：支持请求网址校验、签名校验、加密事件解密、消息写入 Agent Hub Inbox，以及可选的自动确认回复，因此 `Hub 连接` 标记为 `🟡`
- `GitHub Copilot` 已进入“通用 MCP 可接入”范围，但目前仍缺少 AgentHub 专用 setup/portability 文档，因此 `Hub 连接` 标记为 `📝`
- `Gemini` 已完成 CLI + OAuth 直连验证；`Kimi` 当前仍主要通过浏览器扩展获得平台侧支持
- `DeepSeek`、`Qwen / 通义`、`智谱 GLM`、`MiniMax`、`Perplexity` 等主流平台已纳入目标矩阵，用来表达扩展方向，不代表已经实现平台专用接入
- `飞书`、`钉钉`、`Slack`、`Discord`、`Microsoft Teams` 处于 Adapter 或 workspace integration 目标阶段，README 不提前承诺尚未实现的接入或迁移能力

---

## Scoped Token

类似 GitHub Personal Access Token，支持细粒度权限控制：

管理后台的 Token 列表支持按用途命名，并且可以随时改名或吊销，便于区分本地 MCP、GPT Actions、脚本集成等不同接入。

```
aht_  前缀 + 40 位随机 hex
```

21 种权限 scope：

| Scope | 说明 |
|-------|------|
| `read:profile` / `write:profile` | 身份与偏好 |
| `read:memory` / `write:memory` | 记忆系统 |
| `read:vault` / `write:vault` | 加密保险柜 |
| `read:skills` / `write:skills` | 技能库 |
| `read:devices` / `call:devices` | 设备控制 |
| `read:inbox` / `write:inbox` | 收件箱 |
| `read:projects` / `write:projects` | 项目管理 |
| `read:tree` / `write:tree` | 文件树 |
| `read:bundle` / `write:bundle` | Bundle Sync 导入导出 |
| `search` | 全文搜索 |
| `admin` | 全部权限 |

支持层级匹配：`read:vault` 自动覆盖 `read:vault.auth`。

预设 bundle：
- **Agent 完整权限**：适合主力 AI 助手
- **只读访问**：适合轻度集成
- **自定义**：逐项勾选

---

## SDK

### JavaScript / TypeScript

```typescript
import { AgentHub } from '@agenthub/sdk'

const hub = new AgentHub({
  baseURL: 'https://hub.example.com',
  token: 'aht_xxxxx'
})

const profile = await hub.getProfile('preferences')
const results = await hub.searchMemory('海淀算力券')
await hub.callDevice('living-room-light', 'off')
await hub.sendMessage('worker:research@hub', '请调研 Q2 政策', '...')
```

### Python

```python
from agenthub import AgentHub

with AgentHub("https://hub.example.com", token="aht_xxxxx") as hub:
    profile = hub.get_profile("preferences")
    results = hub.search_memory("海淀算力券")
    hub.call_device("living-room-light", "off")
    hub.send_message("worker:research@hub", "请调研 Q2 政策", "...")
```

### OAuth（第三方应用接入）

```typescript
import { AgentHubAuth } from '@agenthub/sdk'

const auth = new AgentHubAuth({
  baseURL: 'https://hub.example.com',
  clientId: 'your-client-id',
  clientSecret: 'your-client-secret'
})

// 用户授权
const url = auth.getAuthorizationURL(redirectURI, ['read:profile', 'read:memory'])
// 回调后换 token
const { access_token, user } = await auth.exchangeCode(code, redirectURI)
```

---

## 技术架构

```
Claude / Codex ─ MCP (HTTP OAuth / stdio) ───────────────┐
Claude Desktop ─ MCP (stdio) ────────────────────────────┤
ChatGPT ─────── GPT Actions ─────────────────────────────┤
任意 MCP 客户端（Cursor / Windsurf / Copilot 等） ──────┤──→  Hub Server (Go)
飞书 / Kimi / DeepSeek / Qwen 等 ─ HTTP API ────────────┤     ├── Auth (JWT + OAuth 2.0 + Scoped Token)
浏览器扩展 ───── HTTP API / Extension ──────────────────┘     ├── Router (信任等级裁剪文件树)
                                                               ├── Storage (PostgreSQL + AES-256-GCM)
                                                               ├── Scheduler (后台任务)
                                                               ├── MCP Server (21 工具)
                                                               └── Webhook (事件通知)
```

### 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go 1.25, Chi router, pgx/v5 |
| 数据库 | PostgreSQL 16 (结构化 + JSONB + 全文搜索) |
| 加密 | AES-256-GCM |
| 认证 | JWT, bcrypt, OAuth 2.0, HMAC-SHA256 |
| 前端 | React 18, TypeScript, Vite |
| 协议 | MCP (JSON-RPC 2.0), REST, OAuth 2.0 |
| 部署 | Docker 单容器 (前端嵌入 Go 二进制) |
| CI/CD | GitHub Actions |

### 项目结构

```
agenthub/
├── cmd/
│   ├── server/main.go        # HTTP 服务入口
│   └── mcp/main.go           # MCP stdio 二进制
├── internal/
│   ├── api/                   # HTTP handlers
│   ├── auth/                  # 认证 + OAuth Provider
│   ├── config/                # 环境变量配置
│   ├── database/              # PostgreSQL 连接 + 迁移
│   ├── hubpath/               # canonical path 规则
│   ├── jobs/                  # 后台任务调度器
│   ├── logger/                # 结构化日志 (slog)
│   ├── mcp/                   # MCP 协议服务器
│   ├── models/                # 数据模型
│   ├── services/              # 业务逻辑
│   ├── vault/                 # AES-256-GCM 加密
│   └── web/                   # 前端嵌入
├── migrations/                # SQL 迁移
├── web/                       # React 前端
├── sdk/
│   ├── javascript/            # JS/TS SDK
│   └── python/                # Python SDK
├── extension/                 # Chrome 浏览器扩展
├── docs/                      # 设计文档 + API schema
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

---

## 管理后台

配置一次，偶尔回来看看。不是用户的日常工具。

| 页面 | 功能 |
|------|------|
| **总览** | 连接数、技能数、设备数、项目数、周活动、数据导出 |
| **连接设置** | Web / Desktop Apps、CLI Apps、本地模式、高级 MCP、GPT Actions、Token 管理（创建 / 改名 / 吊销） |
| **连接管理** | 已连接平台列表、OAuth / MCP 授权、信任等级调整 |
| **我的信息** | 偏好编辑、Vault 查看、记忆冲突检测与解决 |
| **项目** | 项目列表、上下文查看、日志时间线、自动摘要 |
| **协作** | 跨用户共享管理、共享路径配置 |

---

## 安全

- **传输**：HTTPS (生产环境)
- **存储**：Vault 内容 AES-256-GCM 加密
- **认证**：bcrypt (cost 12) 密码哈希、JWT 短期 token、Refresh Token 轮换
- **授权**：四级信任等级、19 种 scope 细粒度权限、路径级别访问控制
- **防护**：速率限制、安全头 (CSP/X-Frame-Options/etc)、Request Body 大小限制、Panic 恢复
- **审计**：结构化日志 + Request ID 追踪
- **Webhook**：HMAC-SHA256 签名验证

---

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `DATABASE_URL` | PostgreSQL 连接字符串 | - |
| `PORT` | 服务端口 | `8080` |
| `JWT_SECRET` | JWT 签名密钥 | - |
| `VAULT_MASTER_KEY` | Vault 主密钥 (64 位 hex) | - |
| `GITHUB_CLIENT_ID` | GitHub OAuth | - |
| `GITHUB_CLIENT_SECRET` | GitHub OAuth | - |
| `CORS_ORIGINS` | 允许的前端域名 | `http://localhost:3000` |
| `RATE_LIMIT` | 每分钟请求数 | `100` |
| `MAX_BODY_SIZE` | 请求体大小限制 | `10485760` (10MB) |
| `LOG_LEVEL` | 日志级别 | `info` |
| `LOG_FORMAT` | 日志格式 (`text`/`json`) | `text` |

---

## Roadmap

### 已完成

**核心功能**

- [x] 统一身份 (邮箱密码 + GitHub OAuth + JWT + Scoped Token + OAuth Provider)
- [x] 上下文漫游 (三层记忆 + 冲突检测 + 自动摘要)
- [x] 秘密管理 (AES-256-GCM + 四级信任等级)
- [x] 能力路由 (.skill 注册 + 批量导入)
- [x] Agent 通信 (邮件系统 + 全文搜索 + TTL 自动归档)
- [x] 设备控制 (统一注册/发现接口，调用层为 mock，真实协议对接见 P1)
- [x] MCP 协议 (21 个工具，Claude Code/Desktop 兼容)
- [x] GPT Actions (ChatGPT 兼容)
- [x] JS/Python SDK (同步 + 异步)
- [x] 浏览器扩展 (Claude/GPT/Gemini/Kimi 四平台)
- [x] 跨用户协作 (路径级共享 + 过期时间)
- [x] Webhook 通知 (HMAC-SHA256 签名)
- [x] 管理后台
- [x] 数据导出 (ZIP + JSON)
- [x] CI/CD + Docker

**代码成熟化 + 测试**

- [x] API Handler 全部接通 Service 层 (消除 26 个 TODO stub)
- [x] Agent API 端点全部接通真实数据 (tree/vault/inbox/device 7 个端点)
- [x] 消除 crypto 操作中的 panic，改为 error 返回
- [x] 输入验证 (slug 格式、内容长度限制)
- [x] 错误处理完善 (fire-and-forget 日志、transaction rollback)
- [x] OAuthService 初始化修复 (之前 nil pointer crash)
- [x] 前端 API envelope 自动 unwrap + 数据格式对齐
- [x] InfoPage 保存格式修复 + 持久化验证
- [x] ProjectsPage 详情展开修复
- [x] FileTree COALESCE nullable 列修复
- [x] MCP ContentBlock.Text omitempty 修复
- [x] 自动化测试覆盖 Playwright 浏览器交互、功能/API 集成、GPT Actions、MCP 协议、单元测试和 E2E 页面测试

### 已知缺失 (需要开发)

- [ ] **设备调用返回 mock** — `DeviceService.Call()` 不对接真实协议 (P1)
- [ ] **Webhook 事件覆盖仍不完整** — 核心路径已接入，但事件面还没有完全统一到所有写入路径 (P1)
- [ ] **共享协作体验仍偏底层** — 共享树可读，但缺少更友好的跨用户发现、审计和冲突处理能力 (P2)

### 下一步 (P1)

- [ ] 设备 Adapter 真实对接 (HTTP/MQTT/米家/HomeAssistant)
- [ ] Webhook 事件面补齐并统一命名
- [ ] 向量搜索 (pgvector 语义检索)
- [ ] Claude Memory 自动导入
- [ ] 平台 portability 手册 (Claude/ChatGPT/Codex)
- [ ] 邮件通知 (注册验证/密码重置)
- [ ] 国际化 (中/英)

### 未来 (P2-P3)

- [ ] Redis 缓存层
- [ ] 飞书/钉钉 Adapter
- [ ] 更多平台的 portability 矩阵与手册补齐
- [ ] Agent 市场 (.skill 共享)
- [ ] 联邦协议 (Hub-to-Hub 去中心化)
- [ ] 端到端加密
- [ ] 支付鉴权
- [ ] SMTP/IMAP 桥接
- [ ] 浏览器扩展完善 + 测试
- [ ] JS/Python SDK 测试

---

## 贡献

本仓库仅对 AGI Bar Core 核心组成员开放。

## License

Proprietary - AGI Bar
