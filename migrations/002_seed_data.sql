-- Seed demo user
INSERT INTO users (id, slug, display_name, timezone, language) VALUES
    ('a0000000-0000-0000-0000-000000000001', 'demo', 'Demo User', 'Asia/Shanghai', 'zh-CN')
ON CONFLICT (slug) DO NOTHING;

-- GitHub auth binding
INSERT INTO auth_bindings (user_id, provider, provider_id, provider_data) VALUES
    ('a0000000-0000-0000-0000-000000000001', 'github', '12345678', '{"login": "demo-user", "name": "Demo User"}')
ON CONFLICT (provider, provider_id) DO NOTHING;

-- Connections with different trust levels
INSERT INTO connections (id, user_id, name, platform, trust_level, api_key_hash, api_key_prefix) VALUES
    ('c0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'My Claude', 'claude', 4, 'seed_claude_key_hash', 'ahk_clau'),
    ('c0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000001', 'My GPT', 'gpt', 3, 'seed_gpt_key_hash', 'ahk_gpt_'),
    ('c0000000-0000-0000-0000-000000000003', 'a0000000-0000-0000-0000-000000000001', '飞书 Agent', 'feishu', 3, 'seed_feishu_key_hash', 'ahk_feis'),
    ('c0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000001', '朋友 A', 'external', 2, 'seed_friend_key_hash', 'ahk_frie')
ON CONFLICT DO NOTHING;

-- File tree entries (skills, identity, etc.)
INSERT INTO file_tree (user_id, path, is_directory, content, content_type, min_trust_level) VALUES
    -- Root directories
    ('a0000000-0000-0000-0000-000000000001', '/identity', true, NULL, NULL, 1),
    ('a0000000-0000-0000-0000-000000000001', '/vault', true, NULL, NULL, 4),
    ('a0000000-0000-0000-0000-000000000001', '/skills', true, NULL, NULL, 3),
    ('a0000000-0000-0000-0000-000000000001', '/devices', true, NULL, NULL, 3),
    ('a0000000-0000-0000-0000-000000000001', '/memory', true, NULL, NULL, 2),
    ('a0000000-0000-0000-0000-000000000001', '/roles', true, NULL, NULL, 4),
    ('a0000000-0000-0000-0000-000000000001', '/inbox', true, NULL, NULL, 3),

    -- Identity
    ('a0000000-0000-0000-0000-000000000001', '/identity/profile.json', false,
     '{"name": "Demo User", "timezone": "Asia/Shanghai", "language": "zh-CN", "bio": "AI 时代的探索者，AGI Bar 核心成员"}',
     'application/json', 1),
    ('a0000000-0000-0000-0000-000000000001', '/identity/SKILL.md', false,
     '# 身份系统

这个目录包含用户的身份信息和各平台绑定。

## 使用方式
- 读取 `profile.json` 获取基本信息
- 读取 `bindings/` 下的文件查看已绑定的平台

## 权限
- L1 访客：只能看到名字和时区
- L3 工作信任：可以看到完整 profile
- L4 完全信任：可以看到所有绑定详情',
     'text/markdown', 1),

    -- Skills
    ('a0000000-0000-0000-0000-000000000001', '/skills/cyberzen-write', true, NULL, NULL, 3),
    ('a0000000-0000-0000-0000-000000000001', '/skills/cyberzen-write/SKILL.md', false,
     '# CyberZen 写作风格

## 核心原则
- 段落结尾不用句号
- 不用比喻和类比
- 标题信息密度高，不超过 20 字
- 每段不超过 3 句话
- 用短句，避免从句嵌套

## 格式要求
- 公众号文章：标题 + 引言 + 正文 + 结语
- 标题用问句或判断句，不用"论..."的格式
- 引言一句话说清楚核心观点

## 语气
直接、坦诚、不说废话。像跟朋友聊天，但有信息密度',
     'text/markdown', 3),

    ('a0000000-0000-0000-0000-000000000001', '/skills/code-review', true, NULL, NULL, 3),
    ('a0000000-0000-0000-0000-000000000001', '/skills/code-review/SKILL.md', false,
     '# Code Review Skill

## 审查重点
1. 安全性：注入风险、权限检查、数据验证
2. 性能：N+1 查询、不必要的内存分配
3. 可维护性：命名清晰、职责单一
4. 测试：关键路径有无测试覆盖

## 输出格式
- 严重问题：🔴 标记，必须修改
- 建议改进：🟡 标记，建议修改
- 风格问题：🟢 标记，可选

## 原则
不做无意义的风格纠正。关注逻辑正确性和安全性',
     'text/markdown', 3),

    -- Devices
    ('a0000000-0000-0000-0000-000000000001', '/devices/living-room-light', true, NULL, NULL, 3),
    ('a0000000-0000-0000-0000-000000000001', '/devices/living-room-light/SKILL.md', false,
     '# 客厅灯 - Yeelight LED

## 设备信息
- 品牌：Yeelight
- 型号：YLDP06YL
- 协议：HTTP (LAN API)
- IP：192.168.1.100

## 支持的操作
- `turn_on`: 开灯
- `turn_off`: 关灯
- `set_brightness`: 设置亮度 (1-100)
- `set_color_temp`: 设置色温 (1700-6500K)

## 调用示例
```json
{"action": "set_brightness", "params": {"value": 50}}
```',
     'text/markdown', 3),

    ('a0000000-0000-0000-0000-000000000001', '/devices/bedroom-ac', true, NULL, NULL, 3),
    ('a0000000-0000-0000-0000-000000000001', '/devices/bedroom-ac/SKILL.md', false,
     '# 卧室空调 - 美的

## 设备信息
- 品牌：Midea
- 型号：KFR-35GW
- 协议：HTTP (美云智数 API)
- 设备 ID：midea_ac_001

## 支持的操作
- `turn_on`: 开启空调
- `turn_off`: 关闭空调
- `set_temperature`: 设置温度 (16-30°C)
- `set_mode`: 设置模式 (cool/heat/auto/fan)

## 调用示例
```json
{"action": "set_temperature", "params": {"value": 26, "mode": "cool"}}
```',
     'text/markdown', 3),

    -- Memory
    ('a0000000-0000-0000-0000-000000000001', '/memory/SKILL.md', false,
     '# 记忆系统

Agent Hub 的记忆分三层：

## Profile 层（稳定画像）
路径：`/memory/profile/`
- `preferences.md`: 写作风格、UI 偏好、沟通习惯
- `relationships.md`: 常联系的人、合作关系
- `principles.md`: 做事原则、决策倾向

更新频率极低，跨平台通用。

## Projects 层（项目上下文）
路径：`/memory/projects/{name}/`
每个项目一个文件夹，包含 context.md 和 log.jsonl。

## Scratch 层（短期记忆）
自动生成的每日摘要，30 天后自动归档。

## 使用方式
- 搜索记忆：调用 search API，scope="memory"
- 写入记忆：写入对应路径的文件
- 项目日志：调用 project log API，append-only',
     'text/markdown', 2),

    ('a0000000-0000-0000-0000-000000000001', '/memory/profile', true, NULL, NULL, 2),
    ('a0000000-0000-0000-0000-000000000001', '/memory/profile/preferences.md', false,
     '# 个人偏好

## 写作
- 段落结尾不用句号
- 不用比喻
- 标题不超过 20 字
- 正式文档可以用句号（与日常写作区分）

## 沟通
- 喜欢直接，不喜欢绕弯子
- 回复消息倾向简短
- 重要事情用文字而非语音

## 工作习惯
- 早上 9-12 点是深度工作时间
- 下午适合开会和沟通
- 晚上偶尔写文章

## 技术偏好
- 后端偏好 Go 和 Node.js
- 前端用 React
- 数据库首选 PostgreSQL
- 部署用 Docker',
     'text/markdown', 2)
ON CONFLICT (user_id, path) DO NOTHING;

-- Vault entries (these will need to be encrypted in production, seeded as placeholders)
-- In production, these are encrypted. For seed data we use dummy encrypted bytes.
INSERT INTO vault_entries (user_id, scope, encrypted_data, nonce, description, min_trust_level) VALUES
    ('a0000000-0000-0000-0000-000000000001', 'auth.github', E'\\x00', E'\\x00', 'GitHub Personal Access Token', 3),
    ('a0000000-0000-0000-0000-000000000001', 'auth.openai', E'\\x00', E'\\x00', 'OpenAI API Key', 3),
    ('a0000000-0000-0000-0000-000000000001', 'auth.feishu', E'\\x00', E'\\x00', '飞书应用凭证', 3),
    ('a0000000-0000-0000-0000-000000000001', 'identity.personal', E'\\x00', E'\\x00', '身份证信息（加密存储）', 4),
    ('a0000000-0000-0000-0000-000000000001', 'finance.bank', E'\\x00', E'\\x00', '银行卡信息（加密存储）', 4)
ON CONFLICT (user_id, scope) DO NOTHING;

-- Roles
INSERT INTO roles (user_id, name, role_type, allowed_paths, allowed_vault_scopes, lifecycle) VALUES
    ('a0000000-0000-0000-0000-000000000001', 'assistant', 'assistant', ARRAY['/**'], ARRAY['**'], 'permanent'),
    ('a0000000-0000-0000-0000-000000000001', 'worker-agenthub', 'worker', ARRAY['/memory/projects/agenthub/**', '/skills/**'], ARRAY['auth.*'], 'project'),
    ('a0000000-0000-0000-0000-000000000001', 'worker-cyberzen', 'worker', ARRAY['/memory/projects/cyberzen/**', '/skills/cyberzen-*/**'], ARRAY['auth.feishu'], 'project'),
    ('a0000000-0000-0000-0000-000000000001', 'delegate-friend-a', 'delegate', ARRAY['/memory/projects/shared-research/**'], ARRAY[]::TEXT[], 'permanent')
ON CONFLICT (user_id, name) DO NOTHING;

-- Projects
INSERT INTO projects (id, user_id, name, status, context_md) VALUES
    ('b0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'agenthub', 'active',
     '# Agent Hub 项目

## 概述
AI 时代的身份与信任基础设施。让用户的 AI Agent 跨平台共享身份、记忆、能力和通信。

## 当前状态
Phase 1 开发中 - 核心系统构建

## 关键决策
- 后端使用 Go
- 数据库使用 PostgreSQL
- MCP 作为首选连接协议
- AES-256-GCM 用于 vault 加密'),
    ('b0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000001', 'cyberzen', 'active',
     '# CyberZen 公众号

## 概述
AI 与科技领域的深度分析公众号

## 写作风格
见 /skills/cyberzen-write/SKILL.md

## 最近文章
- 算力券到底补贴了谁
- AI Agent 的信任问题')
ON CONFLICT (user_id, name) DO NOTHING;

-- Project logs
INSERT INTO project_logs (project_id, source, role, action, summary, tags) VALUES
    ('b0000000-0000-0000-0000-000000000001', 'claude', 'assistant', 'created_project', '创建了 Agent Hub 项目，完成了设计文档 v0.2', ARRAY['init', 'design']),
    ('b0000000-0000-0000-0000-000000000001', 'claude', 'assistant', 'started_development', '开始 Phase 1 核心系统开发：Go 后端 + PostgreSQL + React 前端', ARRAY['development', 'phase1']),
    ('b0000000-0000-0000-0000-000000000002', 'claude', 'assistant', 'wrote_article', '写了一篇关于海淀算力券的分析文章', ARRAY['writing', 'policy'])
ON CONFLICT DO NOTHING;

-- Memory profile
INSERT INTO memory_profile (user_id, category, content, source) VALUES
    ('a0000000-0000-0000-0000-000000000001', 'preferences', '段落结尾不用句号。不用比喻。标题不超过20字。喜欢直接沟通。后端偏好Go和Node.js。前端用React。', 'claude'),
    ('a0000000-0000-0000-0000-000000000001', 'relationships', '常联系：AGI Bar 核心团队成员。合作伙伴：多位 AI 领域 VC。', 'manual'),
    ('a0000000-0000-0000-0000-000000000001', 'principles', '用户数据可导出。身份层永远免费。简单优先。先验证再扩展。', 'manual')
ON CONFLICT (user_id, category) DO NOTHING;

-- Devices
INSERT INTO devices (user_id, name, device_type, brand, protocol, endpoint, skill_md, config) VALUES
    ('a0000000-0000-0000-0000-000000000001', 'living-room-light', 'light', 'Yeelight', 'http', 'http://192.168.1.100:55443', 'Yeelight LED 客厅灯', '{"model": "YLDP06YL"}'),
    ('a0000000-0000-0000-0000-000000000001', 'bedroom-ac', 'ac', 'Midea', 'http', 'https://api.midea.com/device/midea_ac_001', '美的卧室空调', '{"model": "KFR-35GW"}'),
    ('a0000000-0000-0000-0000-000000000001', 'nas-synology', 'nas', 'Synology', 'http', 'http://192.168.1.200:5000', 'Synology NAS DS920+', '{"model": "DS920+"}')
ON CONFLICT (user_id, name) DO NOTHING;

-- Sample inbox messages
INSERT INTO inbox_messages (user_id, from_address, to_address, thread_id, priority, action_required, domain, action_type, tags, subject, body, structured_payload, status) VALUES
    ('a0000000-0000-0000-0000-000000000001', 'worker:cyberzen@demo.hub', 'assistant@demo.hub', 'cyberzen-article-01', 'normal', false, 'kb', 'result', ARRAY['writing', 'completed'],
     '算力券文章写作完成',
     '已完成关于海淀算力券的公众号文章。文章标题：《算力券到底补贴了谁》。要点：1. 算力券本质是政府对AI算力的间接补贴 2. 主要受益方是大模型训练企业 3. 对中小开发者的实际帮助有限。文章已按 CyberZen 风格完成，等待您审阅。',
     '{"article_title": "算力券到底补贴了谁", "word_count": 2800, "status": "draft"}',
     'incoming'),
    ('a0000000-0000-0000-0000-000000000001', 'assistant@demo.hub', 'assistant@demo.hub', 'preference-update-01', 'normal', false, 'kb', 'memory_sync', ARRAY['preference-change'],
     '用户偏好更新：公众号标题长度',
     '用户在今天的交互中明确表示标题不超过 20 字。之前的偏好是 25 字以内。已更新 profile/preferences.md。',
     '{"field": "title_max_length", "old_value": 25, "new_value": 20}',
     'archived')
ON CONFLICT DO NOTHING;

-- Activity log
INSERT INTO activity_log (user_id, connection_id, action, path) VALUES
    ('a0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000001', 'read', '/memory/profile/preferences.md'),
    ('a0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000001', 'read', '/skills/cyberzen-write/SKILL.md'),
    ('a0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000001', 'write', '/memory/projects/agenthub/context.md'),
    ('a0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000002', 'read', '/identity/profile.json'),
    ('a0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000003', 'read', '/skills/cyberzen-write/SKILL.md')
ON CONFLICT DO NOTHING;
