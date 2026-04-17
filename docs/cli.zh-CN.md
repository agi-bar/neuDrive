[English](cli.md) | 简体中文

# neuDrive CLI 使用手册

这份文档是 README 里链接的详细 CLI 手册。逐平台接入方式请看 [接入说明](setup.zh-CN.md)。

下面的示例统一使用 `neu`；兼容别名 `neudrive` 仍然支持。

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
neu status
neu platform ls
neu connect claude
neu browse
```

- `neu status` 用来检查本地 daemon、本地存储和当前 target 是否就绪。
- `neu platform ls` 用来查看已安装的平台 adapter 和连接状态。
- `neu connect claude` 用来为当前环境安装 / 配置 Claude 集成。
- `neu browse` 用来在浏览器中打开本地 Hub。

## 内置帮助

```bash
neu help
neu help roots
neu help write
```

## 核心 Hub 命令

这些命令面向 neuDrive 的公开根目录，例如 `profile`、`memory`、`project`、`skill`、`secret`、`platform`。

| 命令 | 作用 | 示例 |
|------|------|------|
| `neu ls [path]` | 浏览公开根目录或某个子树 | `neu ls project/demo` |
| `neu read <path>` | 读取某个 Hub 路径的文本、摘要或 secret 值 | `neu read profile/preferences` |
| `neu write <path> <content-or-file>` | 用文本、stdin 或本地文件创建 / 更新 Hub 内容 | `neu write project/demo/docs/notes.md ./notes.md` |
| `neu search <query> [path]` | 全局搜索或在某个路径范围内搜索 | `neu search migration project/demo` |
| `neu create project <name>` | 创建项目 | `neu create project launch-plan` |
| `neu log <project-path> --action ... --summary ...` | 给项目追加结构化日志 | `neu log project/demo --action note --summary "Kickoff complete"` |
| `neu stats` | 查看当前 Hub 的内容概览 | `neu stats` |

## 本地运行时命令

| 命令 | 作用 | 示例 |
|------|------|------|
| `neu status` | 检查本地 daemon 和本地存储是否可用 | `neu status` |
| `neu browse [--print-url] [/route]` | 打开本地 dashboard，或打印带认证信息的 URL | `neu browse /data/files` |
| `neu doctor` | 做一次简洁的本地诊断 | `neu doctor` |
| `neu daemon status` | 查看 daemon 状态 | `neu daemon status` |
| `neu daemon logs [--tail N]` | 查看最近 daemon 日志 | `neu daemon logs --tail 50` |
| `neu daemon stop` | 停止本地 daemon | `neu daemon stop` |

## 平台命令

| 命令 | 作用 | 示例 |
|------|------|------|
| `neu platform ls` | 列出已发现的平台 adapter 和连接状态 | `neu platform ls` |
| `neu platform show <platform>` | 查看某个平台 adapter 的路径、入口和使用提示 | `neu platform show claude` |
| `neu connect <platform>` | 为某个平台安装或刷新 neuDrive 管理的本地入口 | `neu connect claude` |
| `neu disconnect <platform>` | 删除某个平台的本地入口和相关元数据 | `neu disconnect claude` |
| `neu export <platform> [--output DIR]` | 从当前本地 Hub 生成面向某个平台的导出材料 | `neu export claude --output ./claude-export` |

## 导入命令

| 命令 | 作用 | 示例 |
|------|------|------|
| `neu import platform <platform> [--mode ...] [--dry-run] [--zip FILE]` | 导入 Codex、Claude 等平台数据 | `neu import platform claude --dry-run --mode agent` |
| `neu import skill <dir> [--name NAME]` | 导入一个本地 skill 目录 | `neu import skill ./demo-skill` |
| `neu import profile <file> [--category ...]` | 导入 profile 文档 | `neu import profile ./preferences.md --category preferences` |
| `neu import memory <file-or-dir>` | 导入 memory 内容 | `neu import memory ./notes` |
| `neu import project <file-or-dir> [--name NAME]` | 导入项目文件 | `neu import project ./demo-project --name demo` |

## Git Mirror 命令

| 命令 | 作用 | 示例 |
|------|------|------|
| `neu git init [--output DIR]` | 把本地 Hub 的非 secret 数据导出为 Git mirror 并注册 | `neu git init --output ./neudrive-export/git-mirror` |
| `neu git pull` | 从当前本地 Hub 刷新 active Git mirror | `neu git pull` |
| `neu git auth github-app --device` | 为 Git mirror 工作流连接 GitHub App 用户 | `neu git auth github-app --device` |

## Token 命令

| 命令 | 作用 | 示例 |
|------|------|------|
| `neu token create --kind sync ...` | 创建短期 sync token | `neu token create --kind sync --purpose backup --access both` |
| `neu token create --kind skills-upload ...` | 创建短期 skills-upload token | `neu token create --kind skills-upload --purpose skills --platform claude-web` |

## 官方云服务与 Hosted Profile

当你希望登录 hosted neuDrive，并在多个已保存 profile 之间切换时，用这一组命令。

| 命令 | 作用 | 示例 |
|------|------|------|
| `neu login [--profile NAME] [--api-base URL] [--token TOKEN]` | 通过浏览器登录一个 hosted profile；`official` 是默认官方云 profile | `neu login` |
| `neu profiles` | 列出已保存的 hosted profile，并显示当前 active target | `neu profiles` |
| `neu use <local\|profile>` | 切换当前默认 target | `neu use official` |
| `neu whoami [--local \| --profile NAME \| --api-base URL --token TOKEN]` | 查看解析后 target 对应的当前身份 | `neu whoami` |
| `neu logout [--profile NAME]` | 清除某个 hosted profile 的保存会话 | `neu logout --profile official` |

## Bundle Sync 命令

当你需要 archive 风格的导入 / 导出 / 迁移流程时，用 `sync` 这一组命令。如果目标是官方云，先用 `neu login` 登录并选中目标 profile。

| 命令 | 作用 | 示例 |
|------|------|------|
| `neu sync export --source DIR [--format json\|archive] [--output FILE]` | 从本地源目录构建导出 bundle | `neu sync export --source ./skills --output backup.ndrv` |
| `neu sync preview --source DIR \| --bundle FILE` | 预览一个即将导入的 bundle，但不真正写入 | `neu sync preview --bundle backup.ndrv` |
| `neu sync push --source DIR \| --bundle FILE` | 把本地源目录或现有 bundle 推到远端 Hub | `neu sync push --bundle backup.ndrv` |
| `neu sync pull [--format json\|archive] [--output FILE]` | 从远端 Hub 拉取内容到本地 bundle 文件 | `neu sync pull --format archive --output pulled.ndrvz` |
| `neu sync resume --bundle FILE [--session-file FILE]` | 继续一个中断的 archive 上传 session | `neu sync resume --bundle backup.ndrvz` |
| `neu sync history` | 查看最近的 sync session 历史 | `neu sync history` |
| `neu sync diff --left FILE --right FILE [--format text\|json]` | 比较两个 bundle，存在差异时返回非零退出码 | `neu sync diff --left before.ndrv --right after.ndrv` |

## 底层服务命令

| 命令 | 作用 | 示例 |
|------|------|------|
| `neu server [flags]` | 启动独立的 neuDrive HTTP 服务 | `neu server --listen 127.0.0.1:42690 --local-mode` |
| `neu mcp stdio [flags]` | 通过 stdio 启动 neuDrive MCP 服务 | `neu mcp stdio --token-env NEUDRIVE_TOKEN` |

## 帮助

如果你想看某个命令面的精确语法，直接用内置 help：

```bash
neu help
neu help roots
neu help write
```

如果你关心的是测试覆盖而不是日常用法，可以继续看 [CLI 测试矩阵](cli-test-matrix.md)。
