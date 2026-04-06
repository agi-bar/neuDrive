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

## `merge` 与 `mirror`

- `merge`：只 upsert bundle 里出现的数据，不删除现有额外文件
- `mirror`：只会清理 bundle 中声明的 skill 里未出现的额外文件，不会全局删除其他 skill

推荐默认使用 `merge`。只有在你明确要把某个 skill 的 Hub 状态“对齐到 bundle”时，才使用 `mirror`，并且先做 `preview`。

## 标准流程

### 1. 本地导出

```bash
python3 tools/ahub-sync.py export --source /path/to/skills -o backup.ahub
python3 tools/ahub-sync.py export --source /path/to/skills --format archive -o backup.ahubz
```

### 2. 预览

```bash
python3 tools/ahub-sync.py preview --token aht_xxx --bundle backup.ahub
python3 tools/ahub-sync.py preview --token aht_xxx --bundle backup.ahubz --mode mirror
```

### 3. 导入

```bash
python3 tools/ahub-sync.py push --token aht_xxx --bundle backup.ahub --transport json
python3 tools/ahub-sync.py push --token aht_xxx --bundle backup.ahubz --transport auto
```

`auto` 的规则：

- JSON 编码后不超过 8 MiB：直接走 `/agent/import/bundle`
- 超过 8 MiB，或输入本身就是 `.ahubz`：走 session + parts + commit

### 4. 导出回本地

```bash
python3 tools/ahub-sync.py pull --token aht_xxx -o pulled.ahub
python3 tools/ahub-sync.py pull --token aht_xxx --format archive -o pulled.ahubz
```

### 5. 继续未完成上传

如果 archive 上传中断，CLI 会在 bundle 同目录写一个 sidecar：

- `backup.ahubz.session.json`

继续时：

```bash
python3 tools/ahub-sync.py resume --token aht_xxx --bundle backup.ahubz
```

前提是你重新选择原始 `.ahubz` 文件，而不是一个新的 archive。

### 6. 查看历史

```bash
python3 tools/ahub-sync.py history --token aht_xxx
```

### 7. 比对结果

```bash
python3 tools/ahub-sync.py diff --left backup.ahubz --right pulled.ahubz
python3 tools/ahub-sync.py diff --left backup.ahub --right pulled.ahubz --format json
```

退出码：

- `0`：完全一致
- `1`：存在差异
- `2`：参数或解析错误

## Selective Sync

可以按 domain 和 skill 过滤：

```bash
python3 tools/ahub-sync.py export \
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

archive 导入时，页面会自动：

- 读取 manifest
- 创建或续接 session
- 上传缺失 parts
- commit

`mirror` preview 里所有 delete 项都会单独高亮，并明确提示只影响 bundle 中声明的 skill。
