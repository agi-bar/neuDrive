# Agent Hub Setup Guide

这份文档对应管理后台“连接设置”页面里的各类入口，适合在你已经部署好 Agent Hub 之后，用来选择具体接法。

下面的示例统一使用：

- Hub 地址：`https://hub.example.com`
- MCP 地址：`https://hub.example.com/mcp`
- scoped token 环境变量：`AGENTHUB_TOKEN`

如果你当前跑在本地开发地址（例如 `http://localhost:8080`），那么只有“本地模式”适合直接使用；Web / Desktop Apps 和 CLI Apps 通常需要一个可公开访问的 HTTPS 地址。

## Web and Desktop Apps

这一类适合通过图形界面完成连接的场景，包括浏览器里的 Apps / Connectors，以及像 Cursor、Windsurf 这样的桌面应用。

### Claude Connectors

1. 登录 Claude 网页应用，进入 `Settings -> Connectors -> Go to Customize`。
2. 点击 `Add custom connector`。
3. 把 `Remote MCP Server URL` 填成 `https://hub.example.com/mcp`。
4. 保存后点击 `Connect`。
5. 浏览器会跳转到 Agent Hub 的登录与授权页；完成后回到 Claude。

### ChatGPT Apps

1. 登录 ChatGPT，进入 `Settings -> Apps`。
2. 在 `Advanced settings` 里点击 `Create app`。
3. 把 `MCP Server URL` 填成 `https://hub.example.com/mcp`。
4. 按提示完成 Agent Hub 登录和授权。

如果你的账号里暂时看不到 `Apps` 入口，通常意味着当前计划或灰度范围还没有开放这一功能。

### Cursor Desktop

你可以直接在图形界面里添加 Remote MCP，也可以写配置文件。

```json
{
  "mcpServers": {
    "agenthub": {
      "url": "https://hub.example.com/mcp"
    }
  }
}
```

推荐步骤：

1. 打开 `Settings -> Tools & MCPs -> Add Custom MCP`。
2. 把 `Remote MCP Server URL` 设为 `https://hub.example.com/mcp`。
3. 点击 `Connect` 或 `Authenticate`。
4. 浏览器会打开 Agent Hub 登录与授权页；完成后回到 Cursor。

### Windsurf Desktop

Windsurf 当前主要通过配置文件接入远程 MCP：

```json
{
  "mcpServers": {
    "agenthub": {
      "serverUrl": "https://hub.example.com/mcp"
    }
  }
}
```

推荐步骤：

1. 打开 `Windsurf Settings -> Cascade`。
2. 在 `MCP Servers` 区域点击 `Open MCP Marketplace`。
3. 点击 config 图标，打开 `~/.codeium/windsurf/mcp_config.json`。
4. 写入上面的 `agenthub` 配置并保存。
5. 点击 `Open`，完成 Agent Hub 登录和授权。

## CLI Apps

这一类适合日常在终端里工作的用户。它们通过远程 HTTP MCP + OAuth 接入 Agent Hub。

### Claude Code

```bash
claude mcp add -s user --transport http agenthub https://hub.example.com/mcp
```

然后在 Claude Code 中执行：

```text
/mcp
```

再按提示完成浏览器授权。

### Codex CLI

```bash
codex mcp add agenthub --url https://hub.example.com/mcp
codex mcp login agenthub
codex mcp list
```

### Gemini CLI

```bash
gemini mcp add --transport http agenthub https://hub.example.com/mcp
```

然后在 Gemini 中执行：

```text
/mcp auth agenthub
```

注意：`gemini mcp add` 必须带 `--transport http`，否则 Gemini 可能会把 URL 当成本地 command，而不是远程 MCP server。

### Cursor Agent

先在 `.cursor/mcp.json` 或 `~/.cursor/mcp.json` 中写入：

```json
{
  "mcpServers": {
    "agenthub": {
      "url": "https://hub.example.com/mcp"
    }
  }
}
```

然后执行：

```bash
cursor-agent mcp login agenthub
cursor-agent mcp list
```

## Local Mode

本地模式适合本地开发、内网环境，或者当前还没有公网 HTTPS 地址的情况。它通过本地 `agenthub-mcp` binary 和 scoped token 接入。

先准备 token：

```bash
export AGENTHUB_TOKEN=aht_xxxxx
```

### Claude Code

```bash
claude mcp add -s user agenthub -- agenthub-mcp --token-env AGENTHUB_TOKEN
```

### Codex CLI

```bash
codex mcp add agenthub -- agenthub-mcp --token-env AGENTHUB_TOKEN
```

如果你只是想查看接法而不想立刻生成 token，建议直接打开管理后台“连接设置 -> 本地模式”，在那里可以创建和复制当前模式专用 token。

## Advanced Mode

高级模式面向支持 HTTP MCP 的通用客户端。推荐优先使用环境变量，只有客户端不支持 env 方式时，再回退到静态 Bearer header。

```bash
export AGENTHUB_TOKEN=aht_xxxxx
```

Codex CLI 可直接引用环境变量，不把 secret 写进配置：

```bash
codex mcp add agenthub --url https://hub.example.com/mcp --bearer-token-env-var AGENTHUB_TOKEN
```

对于其他客户端，如果暂不支持 env 方式，再使用静态 Bearer 配置。

## ChatGPT GPT Actions

如果你想在自定义 GPT 中接入 Agent Hub，可以用 GPT Actions：

1. 打开 ChatGPT，创建一个 GPT。
2. 进入 `Configure -> Actions`。
3. OpenAPI Schema URL 填写 `https://hub.example.com/gpt/openapi.json`。
4. Authentication 选择 `Bearer Token`。
5. 使用一个 scoped token 作为 Bearer Token。

推荐先在管理后台“连接设置 -> Token 管理”中创建一个专用 token。

## Adapters

Adapters 适合飞书、钉钉、Slack 这类工作区平台。目前 README 里的可用示例主要是 Feishu Bot Adapter。

### Feishu Bot Adapter

回调地址格式：

```text
https://hub.example.com/api/adapters/feishu/<your-slug>/events
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
