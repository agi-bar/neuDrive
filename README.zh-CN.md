[English](README.md) | 简体中文

# neuDrive

<p align="center">
  <img src="docs/assets/neudrive-logo.png" alt="neuDrive logo" width="320" />
</p>

**AI Agent 的身份、记忆与信任中枢**

> 一个地方，让 Claude、ChatGPT、Codex、Cursor 等 Agent 共享你是谁、你偏好什么、以及它们被允许做什么。

---

## 这是什么

neuDrive 给每个人一个 Hub。Claude、ChatGPT、Codex、Cursor、Copilot、飞书、Kimi、智谱等 Agent 可以通过这个 Hub 共享身份、记忆、能力、秘密和通信，而不是在每个平台里重复建立上下文。

你可以自己部署 neuDrive，运行属于自己的 Hub。我也提供了一个已经部署好的版本，可以直接在 [https://www.neudrive.ai](https://www.neudrive.ai) 体验。使用兑换码 `VIVO50` 可以获得三个月免费服务；三个月到期后，你可以选择继续续费、继续使用可用的托管方案，或者切换到自己部署的版本。

**你的身份、偏好、秘密、技能跟着人走，不跟平台走。**

## 功能特性

- **一个人的 AI Hub**：集中保存 profile、偏好、memory、projects、skills 和 Agent 通信记录，让上下文跟着你跨工具流动。
- **跨平台 AI 接入**：通过 hosted OAuth、Remote MCP 或本地 adapter 连接 Claude、ChatGPT、Cursor、Windsurf、Codex CLI、Gemini CLI、飞书和自定义 MCP 客户端。
- **记忆和技能迁移**：从 Agent 工具导入 skills、项目上下文、profile/preferences 和 notes，也可以通过 CLI、API、Bundle Sync 导出或恢复。
- **秘密和信任控制**：secrets 放在统一 vault 中，通过 scoped token 和 trust level 控制每个 Agent 能访问什么。
- **Agent 协作**：Agent 可以互发消息、写入项目日志、交接任务，不需要你在工具之间手动复制上下文。
- **面向开发者的数据接口**：提供 canonical virtual tree、typed HTTP API、MCP tools、文件树读写和 `snapshot/changes` 同步接口。
- **GitHub Backup**：把 neuDrive 可见文件树同步到私有 GitHub 仓库，保留可恢复版本历史。[查看文档](docs/github-backup.zh-CN.md)
- **Hosted 或自托管**：可以直接使用 [neudrive.ai](https://www.neudrive.ai)，也可以部署自己的 Hub，使用本地或远端存储。

本仓库里的 hosted service 示例统一使用：

- Hub 地址：`https://www.neudrive.ai`
- MCP 地址：`https://www.neudrive.ai/mcp`

## 从这里开始

按你的接入方式选第一个合适的入口：

1. **Web / Desktop Apps**：最快接入 Claude、ChatGPT、Cursor、Windsurf 等图形界面，并使用官方云服务 + 浏览器授权。[查看文档](docs/setup.zh-CN.md#web-and-desktop-apps)
2. **CLI Apps**：使用 Claude Code、Codex CLI、Gemini CLI、Cursor Agent，通过 Remote HTTP MCP + OAuth 接入。[查看文档](docs/setup.zh-CN.md#cli-apps)
3. **本地模式**：仓库内本地开发、局域网环境，或者当前还没有公网 HTTPS 地址。[查看文档](docs/setup.zh-CN.md#local-mode)
4. **高级模式 / GPT Actions / Adapters**：通用 MCP 客户端、自定义 GPT、Feishu / webhook 等更进阶的接法。[查看文档](docs/setup.zh-CN.md#advanced-mode)

## Web / Desktop Apps

如果你是从 Claude 网页版、ChatGPT、Cursor 或 Windsurf 这类图形界面发起连接，先看这一节。

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

完成授权后，建议马上打开一个**新的对话**，然后直接发出导入指令，例如：

- `请将我的 skills、projects 和 profile 导入到 neuDrive。`
- `请读取我在 neuDrive 中已有的 profile、skills 和最近的项目上下文，并告诉我里面已经有什么内容。`

Cursor、Windsurf 和更完整的接入变体见：[Web / Desktop Apps](docs/setup.zh-CN.md#web-and-desktop-apps)

## 本地 CLI 快速开始

先在本地安装 CLI：

```bash
git clone https://github.com/agi-bar/neuDrive.git
cd neuDrive
./tools/install-neudrive.sh
```

安装完成后，默认使用 `neu`；兼容别名 `neudrive` 也仍然可用。

```bash
neu status         # 检查 daemon、本地存储和当前 target 是否就绪
neu platform ls    # 查看已发现的平台 adapter 和连接状态
neu connect claude # 安装 / 配置 Claude 集成
neu browse         # 在浏览器里打开本地 Hub
```

接着在已连接好的客户端里打开一个**新的对话**，直接说一句：`请把这个工作区里有用的 skills、项目上下文和 profile/preferences 导入到 neuDrive。`

详细 CLI 使用见：[CLI 使用手册](docs/cli.zh-CN.md)

## 登录官方云服务

如果你希望登录官方云 Hub，用于 CLI、Claude 等接入流程，可以直接运行：

```bash
neu login
```

这个命令会拉起浏览器登录流程，把官方云 profile 保存到本地，并自动把当前 target 切到这个 profile。

## 文档索引

先看这些：

- [接入说明](docs/setup.zh-CN.md)
- [GitHub Backup 指南](docs/github-backup.zh-CN.md)
- [CLI 使用手册](docs/cli.zh-CN.md)
- [详细参考](docs/reference.zh-CN.md)

英文文档：

- [README](README.md)
- [Setup Guide](docs/setup.md)
- [GitHub Backup Guide](docs/github-backup.md)
- [CLI Guide](docs/cli.md)
- [Reference](docs/reference.md)

更多资料：

- [Token 管理](docs/setup.zh-CN.md#token-management)
- [Bundle Sync 指南](docs/sync.md)
- [SDK / HTTP API](docs/reference.zh-CN.md#sdk)
- [产品设计文档](docs/design.md)
- [Prod-like 验收 Runbook](docs/sync-prodlike-acceptance.md)
- [安全与资源审计](docs/sync-audit.md)
- [CLI 测试矩阵](docs/cli-test-matrix.md)
