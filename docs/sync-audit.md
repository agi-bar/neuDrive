# Bundle Sync 安全与资源审计

这份文档记录当前 V2 Sync 能力的上线前审计结论，按三类输出：

- 已满足
- 需修复 / 上线前待完成
- 可接受风险

## 已满足

### 1. Token scope 边界

- `read:bundle` 可以导出 bundle、读取 sync history
- `write:bundle` 可以 preview、import、session upload/commit
- `read:bundle` 不能 import
- `write:bundle` 不能 export 或读取 history

自动化覆盖：

- `sdk/python/tests/test_sync_integration.py`

### 2. TTL 边界

- `/api/tokens/sync` 默认 30 分钟
- 超过 120 分钟会被钳制
- 响应中返回实际 `expires_at`

自动化覆盖：

- `sdk/python/tests/test_sync_integration.py`

### 3. Preview 不写 history

- `preview` 只返回 diff，不生成 sync job
- import/export/session 才会写 history

自动化覆盖：

- `internal/services/sync_service_integration_test.go`
- `sdk/python/tests/test_sync_integration.py`

### 4. Session 冲突和失败路径

- part hash 冲突会返回 409
- archive/hash 失败不会部分导入 skill
- failed job 会记录错误摘要

自动化覆盖：

- `internal/services/sync_service_integration_test.go`
- `sdk/python/tests/test_sync_integration.py`

### 5. 过期 session 回收

- 后台 scheduler 每 1 小时执行一次 `CleanExpiredSyncSessions`
- 过期且未完成的 session 会被标记为 `expired`
- 对应 `sync_session_parts` 字节数据会被删除
- `sync_jobs` 元数据保留，用于审计和排障

自动化覆盖：

- `internal/services/sync_service_integration_test.go`

### 6. History 内容最小化

- history 保存方向、transport、mode、filters、summary、error
- 不保存原始 bundle 内容
- 成功路径不会回显二进制字节

自动化覆盖：

- `internal/services/sync_service_integration_test.go`

## 需修复 / 上线前待完成

### 1. 目标环境 prod-like 实测

代码层自动化已经覆盖主要链路，但仍需在目标部署环境完成：

- 真实 bundle round-trip
- archive resume
- mirror 删除边界
- 日志与数据库空间观察

执行文档：

- `docs/sync-prodlike-acceptance.md`

## 可接受风险

### 1. 过期 part 的清理不是实时的

过期 session part 由后台任务回收，默认 1 小时间隔。这意味着：

- 过期后到清理任务运行前，临时字节数据可能短暂保留
- 这不会恢复 session 可用性，只影响临时存储窗口

### 2. Error message 仍依赖底层库的一部分安全表述

当前自动化确认错误信息不会泄露 bundle 正文或二进制内容，但底层 zip / JSON / DB 错误消息仍建议在 prod-like 验收时继续观察。

### 3. Real-data 兼容性需要实际样本闭环

匿名真实样本已经覆盖多 skill、多文本、大体积和二进制，但真实用户样本仍可能引入：

- 异常文件命名
- 极端长路径
- 特殊 mime / 二进制类型
- 非预期编码文本

因此需要用一套真实 bundle 完成最终 round-trip。
