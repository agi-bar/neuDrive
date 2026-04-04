# Agent Hub

**AI 时代的身份与信任基础设施**

> 让所有 AI Agent 认识你、记住你、代表你行动

---

## 这是什么

Agent Hub 是一个独立的中间层服务。每个人拥有一个 Hub，所有 AI Agent（Claude、ChatGPT、飞书、Kimi、智谱……）通过这个 Hub 共享身份、记忆、能力和通信。

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
cp .env.example .env
# 编辑 .env，填入你的 GitHub OAuth 和密钥配置
docker compose up
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

### 连接 Claude Code

```bash
# 1. 在管理后台创建 Scoped Token（或访问"连接设置"页面自动生成）
# 2. 一行命令接入
claude mcp add agenthub -- agenthub-mcp --token aht_xxxxx
```

接入后 Claude Code 自动发现 21 个工具：读取偏好、搜索记忆、控制设备、发送 Agent 间消息等。

### 连接 ChatGPT

1. 打开 ChatGPT → 创建 GPT → Configure → Actions
2. 粘贴 Agent Hub 的 OpenAPI schema（在管理后台"连接设置"页面获取）
3. 配置 Bearer Token 认证
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

## 平台兼容

| 平台 | 接入方式 | 状态 |
|------|---------|------|
| **Claude Code** | MCP 协议 (stdio) | ✅ 可用 |
| **Claude Desktop** | MCP 协议 (stdio) | ✅ 可用 |
| **ChatGPT** | GPT Actions + OpenAPI | ✅ 可用 |
| **任意平台** | HTTP REST API | ✅ 可用 |
| **JavaScript 应用** | `@agenthub/sdk` | ✅ 可用 |
| **Python 应用** | `agenthub-sdk` | ✅ 可用 |
| **浏览器** | Chrome/Edge 扩展 | ✅ 可用 (Claude/GPT/Gemini/Kimi) |
| **飞书/钉钉** | Adapter | 🔜 计划中 |

---

## Scoped Token

类似 GitHub Personal Access Token，支持细粒度权限控制：

```
aht_  前缀 + 40 位随机 hex
```

19 种权限 scope：

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
Claude ─── MCP (stdio) ──┐
                         │
GPT ─── GPT Actions ──┐ │
                       │ │
飞书 ── HTTP API ──────┤ ├──→  Hub Server (Go)
                       │ │     ├── Auth (JWT + OAuth 2.0 + Scoped Token)
Kimi ──────────────────┘ │     ├── Router (信任等级裁剪文件树)
                         │     ├── Storage (PostgreSQL + AES-256-GCM)
浏览器扩展 ──────────────┘     ├── Scheduler (后台任务)
                               ├── MCP Server (21 工具)
                               └── Webhook (事件通知)
```

### 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go 1.23, Chi router, pgx/v5 |
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
| **连接设置** | Claude Code 一键接入、GPT Actions 配置、Token 管理 |
| **连接管理** | 已连接平台列表、信任等级调整 |
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
- [ ] 邮件通知 (注册验证/密码重置)
- [ ] 国际化 (中/英)

### 未来 (P2-P3)

- [ ] Redis 缓存层
- [ ] 飞书/钉钉 Adapter
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
