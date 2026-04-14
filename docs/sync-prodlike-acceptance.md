# Bundle Sync Prod-like 验收 Runbook

这份 Runbook 用于单机 prod-like 环境的最终验收。目标不是压测，而是验证“真实数据 round-trip + resume + mirror 边界”在接近生产的配置下稳定可用。

## 环境前提

- 最新 `main` 已部署
- migrations 已执行完成
- 独立数据库、独立用户，或可清空的专用 schema
- 可访问的 Web UI 和 `/agent/*` API
- 已配置：
  - `DATABASE_URL`
  - `PUBLIC_BASE_URL`
  - `CORS_ORIGINS`
  - 如需 Web 登录，再配置 GitHub OAuth

建议额外确认：

- scheduler 正在运行
- 服务日志可看
- 可以访问 `/api/health`

## 验收数据

分两组：

1. 仓库匿名真实样本
2. 一套真实 bundle / skill 包

真实数据要求：

- 优先 `.ndrvz`
- 或者可导出成 `.ndrv` / 原始 skill 目录
- 使用专用验收用户，不混用正式业务用户

## 部署前检查

```bash
curl -fsS "$BASE_URL/api/health"
neudrive sync --help
neudrive sync diff --help
```

打开 Web 管理后台的数据同步页面，确认：

- 能生成 Sync Token
- 能看到导入、导出、历史三个区域

## 验收步骤

### A. 匿名样本 round-trip

1. 生成 `both` Sync Token
2. 登录 CLI profile
3. 从仓库 fixture 或本地样本导出 archive
4. 运行 preview
5. 运行 push
6. 运行 pull
7. 运行 diff

命令模板：

```bash
neudrive sync login --token "$SYNC_TOKEN" --api-base "$BASE_URL"
neudrive sync export --source /path/to/fixture --format archive -o fixture.ndrvz
neudrive sync preview --bundle fixture.ndrvz
neudrive sync push --bundle fixture.ndrvz --transport auto
neudrive sync pull --format archive -o fixture-pulled.ndrvz
neudrive sync diff --left fixture.ndrvz --right fixture-pulled.ndrvz
```

通过标准：

- `diff` 退出码为 `0`
- 二进制 hash 一致
- history 中出现 import/export 记录

### B. 真实 bundle round-trip

按 A 的流程，对真实 bundle 再做一遍。

额外记录：

- bundle 总字节数
- skills 数量
- 文件数量
- 二进制数量
- 总耗时

### C. Resume 验收

目标是验证 archive session 中断后可以继续。

建议做法：

1. 先创建 archive session
2. 上传一部分或不上传任何 part
3. 中断
4. 通过 Web UI 或 CLI `resume` 继续
5. commit 完成

CLI 方式：

```bash
neudrive sync resume --bundle real.ndrvz
```

通过标准：

- session 最终变为 `committed`
- history 中对应 job 为 `succeeded`
- sidecar session 文件被清理

### D. Mirror 删除边界

准备一个只包含目标 skill 部分文件的 mirror bundle，先 preview，再 push。

重点验证：

- delete 只作用于 bundle 中声明的 skill
- 其他 skill 不受影响
- profile category 不会因缺失而被删除

### E. History 与清理

导入后检查：

- `preview` 不写 history
- import/export 写 history
- failed job 有错误摘要
- 成功 job 不回显原始 bundle 内容

然后等待或手动触发一次过期 session 清理验证：

- 过期 session 被标为 `expired`
- `sync_session_parts` 被清理
- `sync_jobs` 元数据仍保留

## 结果记录模板

```text
环境：
- Base URL:
- Deploy commit:
- Database:
- Scheduler running: yes/no

匿名样本：
- Bundle bytes:
- Skills:
- Files:
- Binary files:
- Push time:
- Pull time:
- Diff result:

真实样本：
- Bundle bytes:
- Skills:
- Files:
- Binary files:
- Push time:
- Pull time:
- Diff result:

Resume：
- Session ID:
- Interrupted at:
- Resume succeeded: yes/no

Mirror：
- Target skill:
- Preview delete count:
- Unexpected deletions: yes/no

History / Cleanup：
- Preview history clean: yes/no
- Failed job sanitized: yes/no
- Expired session parts cleaned: yes/no

未解决问题：
- ...
```

## 验收后清理

- 删除专用验收用户或清空专用数据库
- 删除本地真实 bundle 副本
- 归档日志、history 摘要和 diff 输出
