[English](README.md) | 简体中文

# neuDrive

**AI 时代的身份与信任基础设施**

> 让所有 AI Agent 认识你、记住你、代表你行动

---

## 这是什么

neuDrive 是一个独立的中间层服务。每个人拥有一个 Hub，所有 AI Agent（Claude、ChatGPT、Codex、Cursor、Copilot、飞书、Kimi、智谱……）通过这个 Hub 共享身份、记忆、能力和通信。

**你的身份、偏好、秘密、技能跟着人走，不跟平台走。**

底层统一为一棵 canonical virtual tree，对外同时提供 typed API、文件树读写和 `snapshot/changes` 同步接口。

- 上午在 Claude 写的文章风格偏好，下午切到 GPT 自动生效
- 存在保险柜里的 API Key，被授权的 Agent 可以安全调用
- 你的 Agent 之间可以发邮件、协作、交接任务——你不需要当传话筒
- 一个 Hub ID，通行所有 AI 平台

类比：Google Login 之于 Web、Apple ID 之于移动设备、Stripe 之于支付——neuDrive 是 AI Agent 世界的信任层。

详见 [设计文档](docs/design.md)

---

## 使用模式

可以按下面这个顺序选入口，直接对应不同的使用场景：

| 顺序 | 模式 | 适合场景 | 文档 |
|------|------|----------|------|
| 1 | **Web / Desktop Apps** | 想最快接入 Claude、ChatGPT、Cursor、Windsurf 等图形界面，并使用官方云服务 + 浏览器授权 | [查看文档](docs/setup.zh-CN.md#web-and-desktop-apps) |
| 2 | **CLI Apps** | 使用 Claude Code、Codex CLI、Gemini CLI、Cursor Agent，通过 Remote HTTP MCP + OAuth 接入 | [查看文档](docs/setup.zh-CN.md#cli-apps) |
| 3 | **本地模式** | 仓库内本地开发、局域网环境，或者当前还没有公网 HTTPS 地址 | [查看文档](docs/setup.zh-CN.md#local-mode) |
| 4 | **高级模式 / GPT Actions / Adapters** | 通用 MCP 客户端、自定义 GPT、Feishu / webhook 等更进阶的接法 | [查看文档](docs/setup.zh-CN.md#advanced-mode) |

更多入口：

- [Token 管理](docs/setup.zh-CN.md#token-management)
- [Bundle Sync](docs/sync.md)
- [SDK / HTTP API](docs/reference.zh-CN.md#sdk)

## 本地 CLI 快速开始

```bash
git clone https://github.com/agi-bar/neudrive.git
cd neudrive
./tools/install-neudrive.sh

neudrive status
neudrive platform ls
neudrive connect claude
neudrive browse
```

详细 CLI 使用见：[CLI 使用手册](docs/cli.zh-CN.md)

## 登录官方云服务

如果你希望使用 `https://neudrive.ai` 这个官方云 Hub，并把 Claude 等 Web 应用接到同一个云端账号上，可以先登录官方 profile：

```bash
neudrive remote login official
```

这个命令会拉起浏览器登录流程，并把 `official` profile 保存到本地。登录后，再按 [Web / Desktop Apps 接入说明](docs/setup.zh-CN.md#web-and-desktop-apps) 去连接 Claude、ChatGPT、Cursor 或 Windsurf。

## 文档索引

- [接入说明：Web / Desktop Apps、CLI Apps、本地模式、高级模式、GPT Actions、Adapters](docs/setup.zh-CN.md)
- [CLI 使用手册](docs/cli.zh-CN.md)
- [详细参考](docs/reference.zh-CN.md)
- [CLI Guide](docs/cli.md)
- [Reference](docs/reference.md)
- [产品设计文档](docs/design.md)
- [Bundle Sync 指南](docs/sync.md)
- [Prod-like 验收 Runbook](docs/sync-prodlike-acceptance.md)
- [安全与资源审计](docs/sync-audit.md)
- [CLI 测试矩阵](docs/cli-test-matrix.md)
