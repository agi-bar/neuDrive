[English](setup.md) | 简体中文

# neuDrive 接入指南

这份文档对应管理后台“连接设置”页面里的各类入口，适合在你已经部署好 neuDrive 之后，用来选择具体接法。

下面的示例统一使用：

- Hub 地址：`https://www.neudrive.ai`
- MCP 地址：`https://www.neudrive.ai/mcp`
- scoped token 环境变量：`NEUDRIVE_TOKEN`

如果你当前跑在本地开发地址（例如 `http://localhost:8080`），那么只有“本地模式”适合直接使用；Web / Desktop Apps 和 CLI Apps 通常需要一个可公开访问的 HTTPS 地址。

## Web and Desktop Apps

这一类适合通过图形界面完成连接的场景，包括浏览器里的 Apps / Connectors，以及像 Cursor、Windsurf 这样的桌面应用。

如果你更想在现有聊天网页里直接挂一个 sidecar，而不是走原生 MCP / Apps 接入，先看[浏览器插件指南](./browser-extension.zh-CN.md)。

### 浏览器插件（Chrome / Edge）

如果你想直接在 Claude、ChatGPT、Gemini、Kimi 页面里使用 neuDrive，推荐先用浏览器插件。

当前能力包括：

- 使用 neuDrive 官方登录，或手动配置 `Hub URL + token`
- 把 neuDrive 上下文注入当前聊天输入框
- 在 Claude Web 和 ChatGPT Web 导入当前对话
- 在 Claude Web 批量导入多条对话

完整安装与使用说明见：[浏览器插件指南](./browser-extension.zh-CN.md)

### Claude Connectors

1. 登录 Claude 网页应用，进入 `Settings -> Connectors -> Go to Customize`。
2. 点击 `Add custom connector`。
3. 把 `Remote MCP Server URL` 填成 `https://www.neudrive.ai/mcp`。
4. 保存后点击 `Connect`。
5. 浏览器会跳转到 neuDrive 的登录与授权页；完成后回到 Claude。

### ChatGPT Apps

1. 登录 ChatGPT，进入 `Settings -> Apps`。
2. 在 `Advanced settings` 里点击 `Create app`。
3. 把 `MCP Server URL` 填成 `https://www.neudrive.ai/mcp`。
4. 按提示完成 neuDrive 登录和授权。

如果你的账号里暂时看不到 `Apps` 入口，通常意味着当前计划或灰度范围还没有开放这一功能。

### Cursor Desktop

你可以直接在图形界面里添加 Remote MCP，也可以写配置文件。

```json
{
  "mcpServers": {
    "neudrive": {
      "url": "https://www.neudrive.ai/mcp"
    }
  }
}
```

推荐步骤：

1. 打开 `Settings -> Tools & MCPs -> Add Custom MCP`。
2. 把 `Remote MCP Server URL` 设为 `https://www.neudrive.ai/mcp`。
3. 点击 `Connect` 或 `Authenticate`。
4. 浏览器会打开 neuDrive 登录与授权页；完成后回到 Cursor。

### Windsurf Desktop

Windsurf 当前主要通过配置文件接入远程 MCP：

```json
{
  "mcpServers": {
    "neudrive": {
      "serverUrl": "https://www.neudrive.ai/mcp"
    }
  }
}
```

推荐步骤：

1. 打开 `Windsurf Settings -> Cascade`。
2. 在 `MCP Servers` 区域点击 `Open MCP Marketplace`。
3. 点击 config 图标，打开 `~/.codeium/windsurf/mcp_config.json`。
4. 写入上面的 `neudrive` 配置并保存。
5. 点击 `Open`，完成 neuDrive 登录和授权。

### 连接完成后下一步做什么

当 MCP 配置和浏览器授权都完成后，建议你先在 Claude、ChatGPT、Cursor 或 Windsurf 里打开一个**新的对话**，再发起第一条导入指令。很多客户端里，新加的工具在新对话中最稳定、最容易被正确调用。

推荐直接这样说：

- `请将我的 skills、projects 和 profile 导入到 neuDrive。`
- `请读取我在 neuDrive 中已有的 profile、skills 和最近的项目上下文，并告诉我里面已经有什么内容。`

如果客户端当前已经打开了某个工作区、仓库或对话，它通常可以直接利用这些本地上下文，把内容写进 neuDrive。遇到大文件集合或二进制资源时，建议完成首次接入后改用 [Bundle Sync](./sync.md)。

如果你需要迁移大量 Claude 历史对话，dashboard 里的 Claude 官方导出 ZIP 导入器通常会比浏览器侧批量导入更完整、更稳定。

## CLI Apps

这一类适合日常在终端里工作的用户。它们通过远程 HTTP MCP + OAuth 接入 neuDrive。

### Claude Code

```bash
claude mcp add -s user --transport http neudrive https://www.neudrive.ai/mcp
```

然后在 Claude Code 中执行：

```text
/mcp
```

再按提示完成浏览器授权。

### Codex CLI

```bash
codex mcp add neudrive --url https://www.neudrive.ai/mcp
codex mcp login neudrive
codex mcp list
```

### Gemini CLI

```bash
gemini mcp add --transport http neudrive https://www.neudrive.ai/mcp
```

然后在 Gemini 中执行：

```text
/mcp auth neudrive
```

注意：`gemini mcp add` 必须带 `--transport http`，否则 Gemini 可能会把 URL 当成本地 command，而不是远程 MCP server。

### Cursor Agent

先在 `.cursor/mcp.json` 或 `~/.cursor/mcp.json` 中写入：

```json
{
  "mcpServers": {
    "neudrive": {
      "url": "https://www.neudrive.ai/mcp"
    }
  }
}
```

然后执行：

```bash
cursor-agent mcp login neudrive
cursor-agent mcp list
```

### CLI 接好后下一步做什么

当 MCP 入口已经加好、登录 / 授权也完成后，尽量在这个 CLI 客户端里开启一个**新的会话**，再发第一条导入指令。终端类客户端通常在会话开始前就已经挂好工具时最稳定。

推荐直接这样说：

- `请把这个工作区里有用的 skills、项目上下文和 profile/preferences 导入到 neuDrive。`
- `请把当前仓库作为一个项目写入 neuDrive，然后告诉我实际保存了哪些内容。`
- `请扫描这个工作区，把可复用的 skills 和 profile 提示保存到 neuDrive。`

如果 CLI 客户端当前就是在某个 repo 或工作区里启动的，它通常可以直接利用这些本地上下文。如果你发现工具还没有生效，先重启客户端或新开一个会话再试。

## Local Mode

本地模式适合本地开发、内网环境，或者当前还没有公网 HTTPS 地址的情况。它通过本地 `neudrive-mcp` binary 和 scoped token 接入。

先准备 token：

```bash
export NEUDRIVE_TOKEN=ndt_xxxxx
```

### Claude Code

```bash
claude mcp add -s user neudrive -- neudrive-mcp --token-env NEUDRIVE_TOKEN
```

### Codex CLI

```bash
codex mcp add neudrive -- neudrive-mcp --token-env NEUDRIVE_TOKEN
```

如果你只是想查看接法而不想立刻生成 token，建议直接打开管理后台“连接设置 -> 本地模式”，在那里可以创建和复制当前模式专用 token。

### 本地模式接好后下一步做什么

当本地 MCP binary 和 token 都配置好后，建议在连接好的客户端里打开一个**新的会话**，然后让它直接从当前机器上下文开始导入。

推荐直接这样说：

- `请把当前本地工作区导入到 neuDrive，包括项目上下文、有用的 skills 和 profile/preferences。`
- `请把当前仓库保存成 neuDrive 里的一个项目，并告诉我导入了哪些文件或笔记。`

本地模式尤其适合 Agent 已经能直接读取本地文件的场景。遇到大批量或二进制资源时，建议先完成首次连通性验证，再改用 [Bundle Sync](./sync.md)。

## Advanced Mode

高级模式面向支持 HTTP MCP 的通用客户端。推荐优先使用环境变量，只有客户端不支持 env 方式时，再回退到静态 Bearer header。

```bash
export NEUDRIVE_TOKEN=ndt_xxxxx
```

Codex CLI 可直接引用环境变量，不把 secret 写进配置：

```bash
codex mcp add neudrive --url https://www.neudrive.ai/mcp --bearer-token-env-var NEUDRIVE_TOKEN
```

对于其他客户端，如果暂不支持 env 方式，再使用静态 Bearer 配置。

### 高级模式接好后下一步做什么

当认证接通后，建议先验证这个客户端是否真的能调用 neuDrive，再让它执行更大的导入任务。

推荐直接这样说：

- `请先确认你可以访问 neuDrive，然后写入一条测试记录，并告诉我保存到了哪里。`
- `请先读取我的 neuDrive profile；如果你能访问当前工作区，再把当前项目上下文导入到 neuDrive。`

如果这个客户端只是一个通用聊天入口，看不到本地文件或当前 repo，就直接把要保存的内容贴进去，并明确要求它写入 `profile`、`project` 或 `memory`。

## ChatGPT GPT Actions

如果你想在自定义 GPT 中接入 neuDrive，可以用 GPT Actions：

1. 打开 ChatGPT，创建一个 GPT。
2. 进入 `Configure -> Actions`。
3. OpenAPI Schema URL 填写 `https://www.neudrive.ai/gpt/openapi.json`。
4. Authentication 选择 `Bearer Token`。
5. 使用一个 scoped token 作为 Bearer Token。

推荐先在管理后台“连接设置 -> Token 管理”中创建一个专用 token。

### GPT Actions 接好后下一步做什么

当 GPT 配置完成后，建议你和这个 GPT 开一个**新的对话**，并明确告诉它要把什么内容写入 neuDrive。和桌面端 / CLI 这类工作区感知型客户端不同，GPT Actions 通常看不到你的本地仓库或编辑器上下文。

推荐直接这样说：

- `请把下面这些偏好写入我的 neuDrive profile/preferences：...`
- `请在 neuDrive 中创建一个名为 launch-plan 的项目，并把下面这些笔记保存进去：...`
- `请把这段可复用的提示词或 skill 草稿存到 neuDrive，并告诉我保存到了哪里。`

如果你需要“自动读取当前工作区并整体导入”，更适合使用上面的 [Web and Desktop Apps](#web-and-desktop-apps) 或 [CLI Apps](#cli-apps)。

## Adapters

Adapters 适合飞书、钉钉、Slack 这类工作区平台。目前 README 里的可用示例主要是 Feishu Bot Adapter。

### Feishu Bot Adapter

回调地址格式：

```text
https://www.neudrive.ai/api/adapters/feishu/<your-slug>/events
```

服务端环境变量：

```bash
FEISHU_APP_ID=replace-with-your-app-id
FEISHU_APP_SECRET=replace-with-your-app-secret
FEISHU_VERIFICATION_TOKEN=replace-with-your-verification-token
FEISHU_ENCRYPT_KEY=replace-with-your-encrypt-key
```

推荐步骤：

1. 在飞书开放平台创建自建应用，并开启机器人能力。
2. 订阅 `消息与群组 -> 接收消息 v2.0`。
3. 选择 `将事件发送至开发者服务器`。
4. 请求网址填写上面的 callback URL。
5. 在服务端配置 `FEISHU_APP_ID`、`FEISHU_APP_SECRET`、`FEISHU_VERIFICATION_TOKEN`。
6. 推荐同时配置 `FEISHU_ENCRYPT_KEY`，启用签名校验与事件解密。

### Adapter 接好后下一步做什么

当 Adapter 已经可用后，先给它发一条测试消息，并让它把内容写进 neuDrive，这样最容易验证整条链路已经打通。

推荐直接这样说：

- `请把这条消息保存到 neuDrive memory：...`
- `请在 neuDrive 里创建或更新一个名为 launch-plan 的项目，并写入这段摘要：...`
- `请把这条偏好写入我的 neuDrive profile：...`

Adapters 更适合消息驱动的轻量记录和更新；如果你想做仓库级、工作区级的大批量导入，还是更推荐上面的 MCP 接法。

## Token Management

Token 管理不是一种独立接入方式，但几乎所有非 OAuth 路径都依赖它。

你可以在管理后台里：

- 创建 scoped token
- 选择 read-only / agent full / sync 等预设
- 手动勾选 scope
- 改名、吊销、轮换 token

## Bundle Sync

对于大 skill、长文档、PNG / PDF 等二进制资源，推荐走 Bundle Sync，而不是让 AI 逐文件通过 MCP tool 写入。

- [Bundle Sync 指南](./sync.md)
- [Prod-like 验收 Runbook](./sync-prodlike-acceptance.md)
- [安全与资源审计](./sync-audit.md)
