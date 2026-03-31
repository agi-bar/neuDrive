-- Realistic seed data for De (AGI Bar founder)
-- All ON CONFLICT to be idempotent

-- User
INSERT INTO users (id, slug, display_name, timezone, language, email, avatar_url, bio) VALUES
    ('d0000000-0000-0000-0000-000000000001', 'de', 'De', 'Asia/Shanghai', 'zh-CN',
     'de@agibar.ai', 'https://avatars.githubusercontent.com/u/de',
     'AGI Bar 创始人 · AI 基础设施探索者')
ON CONFLICT (slug) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    email = EXCLUDED.email,
    avatar_url = EXCLUDED.avatar_url,
    bio = EXCLUDED.bio;

-- Auth bindings
INSERT INTO auth_bindings (user_id, provider, provider_id, provider_data) VALUES
    ('d0000000-0000-0000-0000-000000000001', 'github', 'de-github',
     '{"login": "anthropic-de", "email": "de@agibar.ai"}'),
    ('d0000000-0000-0000-0000-000000000001', 'email', 'de@agibar.ai', '{}'),
    ('d0000000-0000-0000-0000-000000000001', 'wechat', 'wxid_de_agibar',
     '{"nickname": "De"}')
ON CONFLICT (provider, provider_id) DO NOTHING;

-- Connections (Agent platforms)
INSERT INTO connections (id, user_id, name, platform, trust_level, api_key_prefix) VALUES
    ('c0000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001',
     'Claude Desktop', 'claude', 4, 'ahk_clau'),
    ('c0000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000001',
     'Claude Code', 'claude', 4, 'ahk_code'),
    ('c0000000-0000-0000-0000-000000000003', 'd0000000-0000-0000-0000-000000000001',
     'ChatGPT Plus', 'gpt', 3, 'ahk_gpt4'),
    ('c0000000-0000-0000-0000-000000000004', 'd0000000-0000-0000-0000-000000000001',
     '飞书智能助手', 'feishu', 3, 'ahk_feis'),
    ('c0000000-0000-0000-0000-000000000005', 'd0000000-0000-0000-0000-000000000001',
     'Kimi', 'kimi', 3, 'ahk_kimi'),
    ('c0000000-0000-0000-0000-000000000006', 'd0000000-0000-0000-0000-000000000001',
     '智谱清言', 'zhipu', 2, 'ahk_zhip'),
    ('c0000000-0000-0000-0000-000000000007', 'd0000000-0000-0000-0000-000000000001',
     'MiniMax', 'minimax', 2, 'ahk_mini')
ON CONFLICT DO NOTHING;

-- Vault entries (placeholder encrypted bytes — real system uses AES-256-GCM)
INSERT INTO vault_entries (user_id, scope, encrypted_data, nonce, description, min_trust_level) VALUES
    ('d0000000-0000-0000-0000-000000000001', 'identity.personal', '\x00', '\x00', '身份证号: 110108199X0X1X2X3X', 4),
    ('d0000000-0000-0000-0000-000000000001', 'identity.passport', '\x00', '\x00', '护照号: E12XXXXX8', 4),
    ('d0000000-0000-0000-0000-000000000001', 'finance.bank.icbc', '\x00', '\x00', '工商银行储蓄卡: 6222 **** **** 1234', 4),
    ('d0000000-0000-0000-0000-000000000001', 'finance.bank.cmb', '\x00', '\x00', '招商银行信用卡: 6225 **** **** 5678', 4),
    ('d0000000-0000-0000-0000-000000000001', 'finance.alipay', '\x00', '\x00', '支付宝账号: de@agibar.ai', 4),
    ('d0000000-0000-0000-0000-000000000001', 'auth.github', '\x00', '\x00', 'GitHub PAT', 3),
    ('d0000000-0000-0000-0000-000000000001', 'auth.openai', '\x00', '\x00', 'OpenAI API Key', 3),
    ('d0000000-0000-0000-0000-000000000001', 'auth.anthropic', '\x00', '\x00', 'Anthropic API Key', 3),
    ('d0000000-0000-0000-0000-000000000001', 'auth.feishu', '\x00', '\x00', '飞书应用凭证', 3),
    ('d0000000-0000-0000-0000-000000000001', 'auth.aws', '\x00', '\x00', 'AWS Access Key', 3),
    ('d0000000-0000-0000-0000-000000000001', 'auth.cloudflare', '\x00', '\x00', 'Cloudflare API Token', 3),
    ('d0000000-0000-0000-0000-000000000001', 'auth.wechat_mp', '\x00', '\x00', '微信公众号 AppID + Secret', 3),
    ('d0000000-0000-0000-0000-000000000001', 'contact.phone', '\x00', '\x00', '手机号: 138XXXX1234', 4),
    ('d0000000-0000-0000-0000-000000000001', 'contact.address', '\x00', '\x00', '住址: 北京市海淀区XXXX', 4)
ON CONFLICT (user_id, scope) DO NOTHING;

-- Roles
INSERT INTO roles (user_id, name, role_type, allowed_paths, allowed_vault_scopes, lifecycle) VALUES
    ('d0000000-0000-0000-0000-000000000001', 'assistant', 'assistant', '{/}', '{*}', 'permanent'),
    ('d0000000-0000-0000-0000-000000000001', 'worker-agibar', 'worker', '{/memory/projects/agibar,/skills}', '{auth.feishu}', 'project'),
    ('d0000000-0000-0000-0000-000000000001', 'worker-cyberzen', 'worker', '{/memory/projects/cyberzen,/skills/cyberzen-write}', '{}', 'project'),
    ('d0000000-0000-0000-0000-000000000001', 'worker-policy', 'worker', '{/memory/projects/haidian-policy,/skills/policy-research}', '{}', 'project'),
    ('d0000000-0000-0000-0000-000000000001', 'delegate-alice', 'delegate', '{/memory/projects/agibar-collab}', '{}', 'project')
ON CONFLICT (user_id, name) DO NOTHING;

-- Projects
INSERT INTO projects (id, user_id, name, status, context_md) VALUES
    ('p0000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001',
     'AGI Bar', 'active',
     '# AGI Bar\n\nAI 从业者社区。核心成员 50+，公众号关注 5000+。\n\n当前重点：Agent Hub 基础设施、社区线下活动、内容输出。'),
    ('p0000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000001',
     'CyberZen Writing', 'active',
     '# 禅与赛博朋克\n\n公众号写作项目。风格：信息密度高、不用句号结尾、段落简短。\n\n已发布 30+ 篇，主题覆盖 AI 政策、技术趋势、产品思考。'),
    ('p0000000-0000-0000-0000-000000000003', 'd0000000-0000-0000-0000-000000000001',
     'Haidian Policy', 'active',
     '# 海淀算力券政策研究\n\n跟踪海淀区 AI 算力补贴政策，分析对创业公司的实际影响。'),
    ('p0000000-0000-0000-0000-000000000004', 'd0000000-0000-0000-0000-000000000001',
     'Agent Hub', 'active',
     '# Agent Hub\n\nAI 时代的身份与信任基础设施。Phase 1 核心系统开发中。'),
    ('p0000000-0000-0000-0000-000000000005', 'd0000000-0000-0000-0000-000000000001',
     'ClawColony', 'archived',
     '# ClawColony\n\n之前的游戏项目，已归档。')
ON CONFLICT (user_id, name) DO NOTHING;

-- Project logs
INSERT INTO project_logs (project_id, source, role, action, summary, tags) VALUES
    ('p0000000-0000-0000-0000-000000000001', 'claude', 'assistant', 'planned_event', '规划了 4 月线下活动方案，确定主题为 Agent 协作', ARRAY['event', 'planning']),
    ('p0000000-0000-0000-0000-000000000001', 'feishu', 'assistant', 'synced_calendar', '同步了 AGI Bar 核心组 Q2 会议日程', ARRAY['calendar', 'sync']),
    ('p0000000-0000-0000-0000-000000000002', 'claude', 'assistant', 'wrote_article', '写了《算力券到底补贴了谁》，标题信息密度高', ARRAY['writing', 'policy']),
    ('p0000000-0000-0000-0000-000000000002', 'gpt', 'assistant', 'translated', '翻译了一篇英文 AI 政策分析为中文', ARRAY['writing', 'translation']),
    ('p0000000-0000-0000-0000-000000000003', 'claude', 'worker', 'researched', '整理了海淀区 2025-2026 算力券申报条件和补贴比例', ARRAY['policy', 'haidian']),
    ('p0000000-0000-0000-0000-000000000004', 'claude', 'assistant', 'implemented', 'Phase 1 核心系统 scaffold 完成，包括后端 API 和前端管理后台', ARRAY['dev', 'milestone']);

-- Memory profile
INSERT INTO memory_profile (user_id, category, content, source) VALUES
    ('d0000000-0000-0000-0000-000000000001', 'preferences',
     '写作风格：段落结尾不用句号、标题信息密度高、不用比喻、段落保持简短（3-5句）、中英混排时英文词前后不加空格、偏好用短句和破折号、数据优先于观点、标题不超过20字',
     'claude'),
    ('d0000000-0000-0000-0000-000000000001', 'relationships',
     'Alice — 产品经理，AGI Bar 核心成员，负责社区运营\nBob — 设计师，负责 Agent Hub 的 UI/UX\nCarol — VC，关注 AI infra 赛道\nDave — 开发者社区运营，在深圳',
     'claude'),
    ('d0000000-0000-0000-0000-000000000001', 'principles',
     '先做再说，不做PPT\n最小可行，够用就行\n代码即文档\n不预设用户需求，从真实痛点出发\n一个人能做的事不要两个人做\n技术选型：选无聊的那个',
     'claude')
ON CONFLICT (user_id, category) DO UPDATE SET content = EXCLUDED.content;

-- Memory scratch (recent days)
INSERT INTO memory_scratch (user_id, date, content, source) VALUES
    ('d0000000-0000-0000-0000-000000000001', CURRENT_DATE,
     '今天在 Claude Code 里完成了 Agent Hub Phase 1 的核心系统搭建，包括 Go 后端、React 前端、PostgreSQL schema、Scoped Token 鉴权、登录注册系统、批量导入 API',
     'claude'),
    ('d0000000-0000-0000-0000-000000000001', CURRENT_DATE - 1,
     '和 Alice 讨论了 Agent Hub 的冷启动策略，决定先从 MCP + AGI Bar 社区切入。晚上用 GPT 翻译了一篇关于 AI Agent 协作的论文',
     'claude'),
    ('d0000000-0000-0000-0000-000000000001', CURRENT_DATE - 2,
     '研究了 GitHub Personal Access Token 的设计，作为 Agent Hub Scoped Token 的参考。整理了海淀算力券最新政策变动',
     'claude')
ON CONFLICT (user_id, date, source) DO NOTHING;

-- Devices
INSERT INTO devices (user_id, name, device_type, brand, protocol, endpoint, skill_md, status) VALUES
    ('d0000000-0000-0000-0000-000000000001', 'living-room-light', 'light', 'Yeelight', 'http', 'http://192.168.1.100/api',
     '# 客厅灯 Yeelight\n\n支持操作：on, off, brightness(1-100), color_temp(2700-6500)\n\nHTTP API: POST /api with JSON body', 'online'),
    ('d0000000-0000-0000-0000-000000000001', 'bedroom-ac', 'ac', 'Midea', 'http', 'http://192.168.1.101/api',
     '# 卧室空调 美的\n\n支持操作：on, off, temperature(16-30), mode(cool/heat/auto)', 'online'),
    ('d0000000-0000-0000-0000-000000000001', 'nas-synology', 'nas', 'Synology', 'http', 'http://192.168.1.200:5000/api',
     '# NAS Synology DS920+\n\n支持操作：list_files, upload, download, search', 'online'),
    ('d0000000-0000-0000-0000-000000000001', 'study-desk-lamp', 'light', 'Xiaomi', 'mijia', 'mijia://lamp-001',
     '# 书房台灯 小米\n\n支持操作：on, off, brightness(1-100)', 'online')
ON CONFLICT (user_id, name) DO NOTHING;

-- File tree (key SKILL.md files)
INSERT INTO file_tree (user_id, path, is_directory, content, min_trust_level) VALUES
    ('d0000000-0000-0000-0000-000000000001', '/identity', true, NULL, 1),
    ('d0000000-0000-0000-0000-000000000001', '/identity/SKILL.md', false,
     '# 身份系统\n\n本目录管理用户身份信息。\n\n## 可用操作\n- 读取 profile.json 获取基本信息\n- 查看 bindings/ 了解已绑定的平台\n\n## 信任等级\n- L1: 仅名字和时区\n- L3+: 完整 profile\n- L4: 所有身份信息', 1),
    ('d0000000-0000-0000-0000-000000000001', '/skills', true, NULL, 2),
    ('d0000000-0000-0000-0000-000000000001', '/skills/cyberzen-write/SKILL.md', false,
     '# CyberZen 写作技能\n\n## 风格要求\n- 段落结尾不用句号\n- 标题信息密度高，不超过20字\n- 不用比喻，用数据和事实\n- 段落简短，3-5句\n- 中英混排不加空格\n\n## 输出格式\n- Markdown\n- 标题用 ## 开头\n- 关键数据加粗', 2),
    ('d0000000-0000-0000-0000-000000000001', '/skills/policy-research/SKILL.md', false,
     '# 政策研究技能\n\n## 能力\n- 政策文件解读\n- 补贴计算\n- 影响分析\n\n## 数据源\n- 海淀区政府官网\n- 北京市科委公告\n- 工信部文件', 2),
    ('d0000000-0000-0000-0000-000000000001', '/memory', true, NULL, 2),
    ('d0000000-0000-0000-0000-000000000001', '/memory/SKILL.md', false,
     '# 记忆系统\n\n## 三层结构\n1. **Profile** — 稳定偏好，极少变动\n2. **Projects** — 按项目组织的上下文和日志\n3. **Scratch** — 每日摘要，30天自动衰减\n\n## 使用方式\n- 读 profile/ 获取用户偏好\n- 读 projects/{name}/context.md 获取项目上下文\n- 写 projects/{name}/log.jsonl 记录事件\n- scratch/ 由系统自动管理', 2),
    ('d0000000-0000-0000-0000-000000000001', '/vault', true, NULL, 3),
    ('d0000000-0000-0000-0000-000000000001', '/vault/SKILL.md', false,
     '# 加密保险柜\n\n存储敏感信息，按 scope 分类，AES-256-GCM 加密。\n\n## Scope 命名规则\n- identity.* — 身份证件 (L4)\n- finance.* — 金融信息 (L4)\n- auth.* — 平台授权 (L3)\n- contact.* — 联系方式 (L4)\n\n## 调用方式\nGET /api/vault/{scope} — 需要对应信任等级', 3)
ON CONFLICT (user_id, path) DO NOTHING;

-- Inbox messages
INSERT INTO inbox_messages (user_id, from_address, to_address, subject, body, domain, action_type, tags, status, priority) VALUES
    ('d0000000-0000-0000-0000-000000000001',
     'assistant@de.hub', 'worker:policy@de.hub',
     '请调研海淀算力券 Q2 新政策',
     '海淀区刚发布了 2026 Q2 算力券申报通知，请整理以下信息：\n1. 申报条件变化\n2. 补贴比例调整\n3. 对 50 人以下创业公司的影响\n\n结果写入 projects/haidian-policy/context.md',
     'governance', 'task_request', ARRAY['policy', 'haidian', 'urgent'], 'incoming', 'urgent'),
    ('d0000000-0000-0000-0000-000000000001',
     'worker:cyberzen@de.hub', 'assistant@de.hub',
     '文章《算力券到底补贴了谁》已完成',
     '文章已按 CyberZen 风格完成，要点：\n- 标题：算力券到底补贴了谁（7字，信息密度高）\n- 全文 2400 字\n- 引用了 3 个具体数据点\n- 已写入 projects/cyberzen/drafts/',
     'kb', 'result', ARRAY['writing', 'done'], 'read', 'normal'),
    ('d0000000-0000-0000-0000-000000000001',
     'assistant@de.hub', 'assistant@de.hub',
     '用户偏好更新：标题长度从 25 字改为 20 字',
     '用户在今天的交互中明确表示标题不超过 20 字。之前的偏好是 25 字以内。已更新 profile/preferences.md',
     'kb', 'memory_sync', ARRAY['preference-change'], 'archived', 'normal')
ON CONFLICT DO NOTHING;

-- Activity log (recent)
INSERT INTO activity_log (user_id, connection_id, action, path) VALUES
    ('d0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000001', 'read', '/memory/profile/preferences.md'),
    ('d0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000001', 'write', '/memory/projects/agent-hub/log.jsonl'),
    ('d0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000003', 'read', '/memory/profile/preferences.md'),
    ('d0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000004', 'read', '/skills/lark-integration/SKILL.md'),
    ('d0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000001', 'search', '/memory'),
    ('d0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000002', 'write', '/memory/projects/agent-hub/context.md');
