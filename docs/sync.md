# Bundle Sync 指南

Agent Hub 现在有两条并行通道：

- MCP：适合小而智能的在线操作
- Bundle Sync：适合迁移、备份、恢复、大体积 skill 和二进制资源

Bundle Sync 支持两种文件格式：

| 格式 | 何时使用 | 特点 |
| --- | --- | --- |
| `.ahub` | 小体积、调试、想直接看 JSON 内容 | 结构直观，便于 review 和脚本处理 |
| `.ahubz` | 大 bundle、二进制、需要 session/resume | zip 容器，支持 archive session 上传 |

## Token 与权限

推荐在 Web 管理后台的“数据同步”页面生成短命 Sync Token。

- 默认 TTL：30 分钟
- 可选 TTL：1 小时、2 小时
- `read:bundle`：允许导出 bundle、读取 sync history
- `write:bundle`：允许 preview、JSON import、archive session upload/commit
- `both`：同时拥有上面两组能力

建议：

- 只做导出时用 `pull`
- 只做导入时用 `push`
- 做 round-trip 验收时用 `both`

## CLI 配置与登录

`agenthub sync` 现在支持本地 profile 配置。登录一次后，后续 `preview / push / pull / resume / history / whoami` 默认都会读取当前 profile，不需要每次重复传 `--token` 和 `--api-base`。

默认配置文件位置：

- macOS：`~/Library/Application Support/AgentHub/config.json`
- Linux：`$XDG_CONFIG_HOME/agenthub/config.json`
- Linux（无 XDG 时）：`~/.config/agenthub/config.json`

配置里会保存：

- `current_profile`
- `profiles.<name>.api_base`
- `profiles.<name>.token`
- `profiles.<name>.expires_at`
- `profiles.<name>.scopes`

参数优先级：

1. CLI 显式参数
2. 环境变量
3. 当前 profile 配置
4. 内建默认值

相关环境变量：

- `AGENTHUB_SYNC_CONFIG`
- `AGENTHUB_SYNC_PROFILE`
- `AGENTHUB_SYNC_API_BASE` 或 `AGENTHUB_API_BASE`
- `AGENTHUB_SYNC_TOKEN` 或 `AGENTHUB_TOKEN`

首次登录推荐直接走浏览器：

```bash
agenthub sync login --api-base https://agenthub.agi.bar
agenthub sync profiles
agenthub sync whoami
```

也支持手工粘贴 token：

```bash
agenthub sync login \
  --profile prod \
  --api-base https://agenthub.agi.bar \
  --token aht_xxx
```

多 profile 切换：

```bash
agenthub sync use prod
agenthub sync logout --profile staging
```

## `merge` 与 `mirror`

- `merge`：只 upsert bundle 里出现的数据，不删除现有额外文件
- `mirror`：只会清理 bundle 中声明的 skill 里未出现的额外文件，不会全局删除其他 skill

推荐默认使用 `merge`。只有在你明确要把某个 skill 的 Hub 状态“对齐到 bundle”时，才使用 `mirror`，并且先做 `preview`。

## 标准流程

### 1. 本地导出

```bash
agenthub sync export --source /path/to/skills -o backup.ahub
agenthub sync export --source /path/to/skills --format archive -o backup.ahubz
```

### 2. 预览

```bash
agenthub sync preview --bundle backup.ahub
agenthub sync preview --bundle backup.ahubz --mode mirror
```

### 3. 导入

```bash
agenthub sync push --bundle backup.ahub --transport json
agenthub sync push --bundle backup.ahubz --transport auto
```

`auto` 的规则：

- JSON 编码后不超过 8 MiB：直接走 `/agent/import/bundle`
- 超过 8 MiB，或输入本身就是 `.ahubz`：走 session + parts + commit

### 4. 导出回本地

```bash
agenthub sync pull -o pulled.ahub
agenthub sync pull --format archive -o pulled.ahubz
```

### 5. 继续未完成上传

如果 archive 上传中断，CLI 会在 bundle 同目录写一个 sidecar：

- `backup.ahubz.session.json`

继续时：

```bash
agenthub sync resume --bundle backup.ahubz
```

前提是你重新选择原始 `.ahubz` 文件，而不是一个新的 archive。

### 6. 查看历史

```bash
agenthub sync history
```

### 7. 比对结果

```bash
agenthub sync diff --left backup.ahubz --right pulled.ahubz
agenthub sync diff --left backup.ahub --right pulled.ahubz --format json
```

退出码：

- `0`：完全一致
- `1`：存在差异
- `2`：参数或解析错误

## Selective Sync

可以按 domain 和 skill 过滤：

```bash
agenthub sync export \
  --source /path/to/skills \
  --format archive \
  --include-domain skills \
  --include-skill atlas-brief \
  --exclude-skill atlas-layout \
  -o partial.ahubz
```

支持的 domain：

- `profile`
- `memory`
- `skills`

## Web UI

管理后台“数据同步”页面提供四块能力：

- 临时 Sync Token
- 导入上传
- 导出下载
- 最近同步历史

如果页面是由 CLI 打开的，还会直接把生成的 token 回填到本地 profile。

archive 导入时，页面会自动：

- 读取 manifest
- 创建或续接 session
- 上传缺失 parts
- commit

`mirror` preview 里所有 delete 项都会单独高亮，并明确提示只影响 bundle 中声明的 skill。
