[English](cli.md) | 简体中文

# neuDrive CLI 使用手册

这份文档是 README 里链接的详细 CLI 手册。逐平台接入方式请看 [接入说明](setup.zh-CN.md)。

## 安装

```bash
./tools/install-neudrive.sh
```

或者：

```bash
make install
```

## 快速开始

```bash
neudrive status
neudrive platform ls
neudrive connect claude
neudrive browse
```

## 内置帮助

```bash
neudrive help
neudrive help roots
neudrive help write
```

## 核心 Hub 命令

这些命令面向 neuDrive 的公开根目录，例如 `profile`、`memory`、`project`、`skill`、`secret`、`platform`。

| 命令 | 作用 | 示例 |
|------|------|------|
| `neudrive ls [path]` | 浏览公开根目录或某个子树 | `neudrive ls project/demo` |
| `neudrive read <path>` | 读取某个 Hub 路径的文本、摘要或 secret 值 | `neudrive read profile/preferences` |
| `neudrive write <path> <content-or-file>` | 用文本、stdin 或本地文件创建 / 更新 Hub 内容 | `neudrive write project/demo/docs/notes.md ./notes.md` |
| `neudrive search <query> [path]` | 全局搜索或在某个路径范围内搜索 | `neudrive search migration project/demo` |
| `neudrive create project <name>` | 创建项目 | `neudrive create project launch-plan` |
| `neudrive log <project-path> --action ... --summary ...` | 给项目追加结构化日志 | `neudrive log project/demo --action note --summary "Kickoff complete"` |
| `neudrive stats` | 查看当前 Hub 的内容概览 | `neudrive stats` |

## 本地运行时命令

| 命令 | 作用 | 示例 |
|------|------|------|
| `neudrive status` | 检查本地 daemon 和本地存储是否可用 | `neudrive status` |
| `neudrive browse [--print-url] [/route]` | 打开本地 dashboard，或打印带认证信息的 URL | `neudrive browse /data/files` |
| `neudrive doctor` | 做一次简洁的本地诊断 | `neudrive doctor` |
| `neudrive daemon status` | 查看 daemon 状态 | `neudrive daemon status` |
| `neudrive daemon logs [--tail N]` | 查看最近 daemon 日志 | `neudrive daemon logs --tail 50` |
| `neudrive daemon stop` | 停止本地 daemon | `neudrive daemon stop` |

## 平台命令

| 命令 | 作用 | 示例 |
|------|------|------|
| `neudrive platform ls` | 列出已发现的平台 adapter 和连接状态 | `neudrive platform ls` |
| `neudrive platform show <platform>` | 查看某个平台 adapter 的路径、入口和使用提示 | `neudrive platform show claude` |
| `neudrive connect <platform>` | 为某个平台安装或刷新 neuDrive 管理的本地入口 | `neudrive connect claude` |
| `neudrive disconnect <platform>` | 删除某个平台的本地入口和相关元数据 | `neudrive disconnect claude` |
| `neudrive export <platform> [--output DIR]` | 从当前本地 Hub 生成面向某个平台的导出材料 | `neudrive export claude --output ./claude-export` |

## 导入命令

| 命令 | 作用 | 示例 |
|------|------|------|
| `neudrive import platform <platform> [--mode ...] [--zip FILE]` | 导入 Codex、Claude 等平台数据 | `neudrive import platform codex` |
| `neudrive import skill <dir> [--name NAME]` | 导入一个本地 skill 目录 | `neudrive import skill ./demo-skill` |
| `neudrive import profile <file> [--category ...]` | 导入 profile 文档 | `neudrive import profile ./preferences.md --category preferences` |
| `neudrive import memory <file-or-dir>` | 导入 memory 内容 | `neudrive import memory ./notes` |
| `neudrive import project <file-or-dir> [--name NAME]` | 导入项目文件 | `neudrive import project ./demo-project --name demo` |

## Git Mirror 命令

| 命令 | 作用 | 示例 |
|------|------|------|
| `neudrive git init [--output DIR]` | 把本地 Hub 的非 secret 数据导出为 Git mirror 并注册 | `neudrive git init --output ./neudrive-export/git-mirror` |
| `neudrive git pull` | 从当前本地 Hub 刷新 active Git mirror | `neudrive git pull` |
| `neudrive git auth github-app --device` | 为 Git mirror 工作流连接 GitHub App 用户 | `neudrive git auth github-app --device` |

## Token 命令

| 命令 | 作用 | 示例 |
|------|------|------|
| `neudrive token create --kind sync ...` | 创建短期 sync token | `neudrive token create --kind sync --purpose backup --access both` |
| `neudrive token create --kind skills-upload ...` | 创建短期 skills-upload token | `neudrive token create --kind skills-upload --purpose skills --platform claude-web` |

## 官方云服务与 Remote Profile

当你希望在 bundle sync 之外管理命名远端 profile 时，用这一组命令。

| 命令 | 作用 | 示例 |
|------|------|------|
| `neudrive remote login <profile> [--url URL] [--token TOKEN]` | 登录一个命名 remote profile；`official` 默认指向 `https://neudrive.ai` | `neudrive remote login official` |
| `neudrive remote ls` | 列出已保存的 remote profile | `neudrive remote ls` |
| `neudrive remote use <profile>` | 切换当前 profile | `neudrive remote use official` |
| `neudrive remote whoami <profile>` | 查看某个 profile 的当前认证状态 | `neudrive remote whoami official` |
| `neudrive remote logout [profile]` | 清除某个 profile 保存的 token | `neudrive remote logout official` |

## Bundle Sync 命令

当你需要 archive 风格的导入 / 导出 / 迁移流程时，用 `sync` 这一组命令。

| 命令 | 作用 | 示例 |
|------|------|------|
| `neudrive sync login --profile NAME` | 通过浏览器登录并保存一个 sync profile | `neudrive sync login --profile official` |
| `neudrive sync profiles` | 列出已配置的 sync profile | `neudrive sync profiles` |
| `neudrive sync use [--profile NAME \| NAME]` | 切换当前 sync profile | `neudrive sync use official` |
| `neudrive sync whoami [--profile NAME]` | 查看当前 sync profile 的身份和 scopes | `neudrive sync whoami --profile official` |
| `neudrive sync logout --profile NAME` | 清除某个 sync profile 的保存 token | `neudrive sync logout --profile official` |
| `neudrive sync export --source DIR [--format json\|archive] [--output FILE]` | 从本地源目录构建导出 bundle | `neudrive sync export --source ./skills --output backup.ndrv` |
| `neudrive sync preview --source DIR \| --bundle FILE` | 预览一个即将导入的 bundle，但不真正写入 | `neudrive sync preview --bundle backup.ndrv` |
| `neudrive sync push --source DIR \| --bundle FILE` | 把本地源目录或现有 bundle 推到远端 Hub | `neudrive sync push --bundle backup.ndrv` |
| `neudrive sync pull [--format json\|archive] [--output FILE]` | 从远端 Hub 拉取内容到本地 bundle 文件 | `neudrive sync pull --format archive --output pulled.ndrvz` |
| `neudrive sync resume --bundle FILE [--session-file FILE]` | 继续一个中断的 archive 上传 session | `neudrive sync resume --bundle backup.ndrvz` |
| `neudrive sync history` | 查看最近的 sync session 历史 | `neudrive sync history` |
| `neudrive sync diff --left FILE --right FILE [--format text\|json]` | 比较两个 bundle，存在差异时返回非零退出码 | `neudrive sync diff --left before.ndrv --right after.ndrv` |

## 底层服务命令

| 命令 | 作用 | 示例 |
|------|------|------|
| `neudrive server [flags]` | 启动独立的 neuDrive HTTP 服务 | `neudrive server --listen 127.0.0.1:42690 --local-mode` |
| `neudrive mcp stdio [flags]` | 通过 stdio 启动 neuDrive MCP 服务 | `neudrive mcp stdio --token-env NEUDRIVE_TOKEN` |

## 帮助

如果你想看某个命令面的精确语法，直接用内置 help：

```bash
neudrive help
neudrive help roots
neudrive help write
```

如果你关心的是测试覆盖而不是日常用法，可以继续看 [CLI 测试矩阵](cli-test-matrix.md)。
