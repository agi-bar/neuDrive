-- AgentHub Foundation V1
-- Upgrade file_tree into the canonical entry store and backfill legacy domains.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE file_tree
    ADD COLUMN IF NOT EXISTS kind VARCHAR(64) NOT NULL DEFAULT 'file',
    ADD COLUMN IF NOT EXISTS checksum VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

UPDATE file_tree
SET kind = CASE WHEN is_directory THEN 'directory' ELSE 'file' END
WHERE kind IS NULL OR kind = '';

UPDATE file_tree
SET checksum = encode(digest(
        coalesce(path, '') || '|' ||
        coalesce(content, '') || '|' ||
        coalesce(content_type, '') || '|' ||
        coalesce(metadata::text, '{}'),
        'sha256'
    ), 'hex')
WHERE checksum = '';

CREATE TABLE IF NOT EXISTS entry_versions (
    cursor BIGSERIAL PRIMARY KEY,
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    entry_id UUID NOT NULL REFERENCES file_tree(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    path VARCHAR(1024) NOT NULL,
    kind VARCHAR(64) NOT NULL DEFAULT 'file',
    version BIGINT NOT NULL,
    change_type VARCHAR(16) NOT NULL,
    content TEXT,
    content_type VARCHAR(64) DEFAULT 'text/markdown',
    metadata JSONB NOT NULL DEFAULT '{}',
    checksum VARCHAR(128) NOT NULL,
    min_trust_level INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_entry_versions_user_cursor
    ON entry_versions(user_id, cursor DESC);
CREATE INDEX IF NOT EXISTS idx_entry_versions_user_path
    ON entry_versions(user_id, path, cursor DESC);
CREATE INDEX IF NOT EXISTS idx_file_tree_user_live_path
    ON file_tree(user_id, path)
    WHERE deleted_at IS NULL;

INSERT INTO file_tree (id, user_id, path, kind, is_directory, content, content_type, metadata, checksum, version, min_trust_level, created_at, updated_at)
SELECT
    gen_random_uuid(),
    u.id,
    '/identity/profile.json',
    'identity',
    false,
    jsonb_build_object(
        'id', u.id,
        'slug', u.slug,
        'display_name', coalesce(u.display_name, ''),
        'timezone', coalesce(u.timezone, 'UTC'),
        'language', coalesce(u.language, 'zh-CN'),
        'created_at', coalesce(u.created_at, NOW()),
        'updated_at', coalesce(u.updated_at, NOW())
    )::text,
    'application/json',
    jsonb_build_object('source', 'users', 'projection', true),
    encode(digest(
        '/identity/profile.json|' ||
        jsonb_build_object(
            'id', u.id,
            'slug', u.slug,
            'display_name', coalesce(u.display_name, ''),
            'timezone', coalesce(u.timezone, 'UTC'),
            'language', coalesce(u.language, 'zh-CN'),
            'created_at', coalesce(u.created_at, NOW()),
            'updated_at', coalesce(u.updated_at, NOW())
        )::text || '|application/json|' ||
        jsonb_build_object('source', 'users', 'projection', true)::text,
        'sha256'
    ), 'hex'),
    1,
    1,
    coalesce(u.created_at, NOW()),
    coalesce(u.updated_at, NOW())
FROM users u
WHERE NOT EXISTS (
    SELECT 1 FROM file_tree ft
    WHERE ft.user_id = u.id AND ft.path = '/identity/profile.json'
);

INSERT INTO file_tree (id, user_id, path, kind, is_directory, content, content_type, metadata, checksum, version, min_trust_level, created_at, updated_at)
SELECT
    gen_random_uuid(),
    mp.user_id,
    '/memory/profile/' || mp.category || '.md',
    'memory_profile',
    false,
    mp.content,
    'text/markdown',
    jsonb_build_object('source', coalesce(mp.source, 'legacy'), 'category', mp.category),
    encode(digest(
        '/memory/profile/' || mp.category || '.md|' ||
        coalesce(mp.content, '') || '|text/markdown|' ||
        jsonb_build_object('source', coalesce(mp.source, 'legacy'), 'category', mp.category)::text,
        'sha256'
    ), 'hex'),
    1,
    4,
    mp.created_at,
    mp.updated_at
FROM memory_profile mp
WHERE NOT EXISTS (
    SELECT 1 FROM file_tree ft
    WHERE ft.user_id = mp.user_id AND ft.path = '/memory/profile/' || mp.category || '.md'
);

INSERT INTO file_tree (id, user_id, path, kind, is_directory, content, content_type, metadata, checksum, version, min_trust_level, created_at, updated_at)
SELECT
    gen_random_uuid(),
    ms.user_id,
    '/memory/scratch/' || to_char(ms.date, 'YYYY-MM-DD') || '/' ||
        coalesce(NULLIF(regexp_replace(coalesce(ms.source, ''), '[^a-zA-Z0-9_-]+', '-', 'g'), ''), 'entry') ||
        '-' || left(ms.id::text, 8) || '.md',
    'memory_scratch',
    false,
    ms.content,
    'text/markdown',
    jsonb_build_object(
        'legacy_id', ms.id,
        'source', coalesce(ms.source, 'legacy'),
        'date', to_char(ms.date, 'YYYY-MM-DD'),
        'expires_at', ms.expires_at
    ),
    encode(digest(
        '/memory/scratch/' || to_char(ms.date, 'YYYY-MM-DD') || '/' ||
        coalesce(NULLIF(regexp_replace(coalesce(ms.source, ''), '[^a-zA-Z0-9_-]+', '-', 'g'), ''), 'entry') ||
        '-' || left(ms.id::text, 8) || '.md|' ||
        coalesce(ms.content, '') || '|text/markdown|' ||
        jsonb_build_object(
            'legacy_id', ms.id,
            'source', coalesce(ms.source, 'legacy'),
            'date', to_char(ms.date, 'YYYY-MM-DD'),
            'expires_at', ms.expires_at
        )::text,
        'sha256'
    ), 'hex'),
    1,
    4,
    ms.created_at,
    ms.created_at
FROM memory_scratch ms
WHERE NOT EXISTS (
    SELECT 1 FROM file_tree ft
    WHERE ft.user_id = ms.user_id
      AND ft.path = '/memory/scratch/' || to_char(ms.date, 'YYYY-MM-DD') || '/' ||
          coalesce(NULLIF(regexp_replace(coalesce(ms.source, ''), '[^a-zA-Z0-9_-]+', '-', 'g'), ''), 'entry') ||
          '-' || left(ms.id::text, 8) || '.md'
);

INSERT INTO file_tree (id, user_id, path, kind, is_directory, content, content_type, metadata, checksum, version, min_trust_level, created_at, updated_at)
SELECT
    gen_random_uuid(),
    p.user_id,
    '/projects/' || p.name || '/context.md',
    'project_context',
    false,
    coalesce(p.context_md, ''),
    'text/markdown',
    jsonb_build_object('project', p.name, 'status', p.status, 'metadata', coalesce(p.metadata, '{}'::jsonb)),
    encode(digest(
        '/projects/' || p.name || '/context.md|' ||
        coalesce(p.context_md, '') || '|text/markdown|' ||
        jsonb_build_object('project', p.name, 'status', p.status, 'metadata', coalesce(p.metadata, '{}'::jsonb))::text,
        'sha256'
    ), 'hex'),
    1,
    3,
    p.created_at,
    p.updated_at
FROM projects p
WHERE NOT EXISTS (
    SELECT 1 FROM file_tree ft
    WHERE ft.user_id = p.user_id AND ft.path = '/projects/' || p.name || '/context.md'
);

INSERT INTO file_tree (id, user_id, path, kind, is_directory, content, content_type, metadata, checksum, version, min_trust_level, created_at, updated_at)
SELECT
    gen_random_uuid(),
    p.user_id,
    '/projects/' || p.name || '/log.jsonl',
    'project_log',
    false,
    coalesce((
        SELECT string_agg(
            jsonb_build_object(
                'id', pl.id,
                'project_id', pl.project_id,
                'source', pl.source,
                'role', pl.role,
                'action', pl.action,
                'summary', pl.summary,
                'artifacts', coalesce(pl.artifacts, '{}'::text[]),
                'tags', coalesce(pl.tags, '{}'::text[]),
                'created_at', pl.created_at
            )::text,
            E'\n'
            ORDER BY pl.created_at
        )
        FROM project_logs pl
        WHERE pl.project_id = p.id
    ), ''),
    'application/x-ndjson',
    jsonb_build_object('project', p.name),
    encode(digest(
        '/projects/' || p.name || '/log.jsonl|' ||
        coalesce((
            SELECT string_agg(
                jsonb_build_object(
                    'id', pl.id,
                    'project_id', pl.project_id,
                    'source', pl.source,
                    'role', pl.role,
                    'action', pl.action,
                    'summary', pl.summary,
                    'artifacts', coalesce(pl.artifacts, '{}'::text[]),
                    'tags', coalesce(pl.tags, '{}'::text[]),
                    'created_at', pl.created_at
                )::text,
                E'\n'
                ORDER BY pl.created_at
            )
            FROM project_logs pl
            WHERE pl.project_id = p.id
        ), '') || '|application/x-ndjson|' ||
        jsonb_build_object('project', p.name)::text,
        'sha256'
    ), 'hex'),
    1,
    3,
    p.created_at,
    p.updated_at
FROM projects p
WHERE NOT EXISTS (
    SELECT 1 FROM file_tree ft
    WHERE ft.user_id = p.user_id AND ft.path = '/projects/' || p.name || '/log.jsonl'
);

INSERT INTO file_tree (id, user_id, path, kind, is_directory, content, content_type, metadata, checksum, version, min_trust_level, created_at, updated_at)
SELECT
    gen_random_uuid(),
    im.user_id,
    '/inbox/' ||
        regexp_replace(coalesce(im.to_address, 'default'), '/+', '_', 'g') || '/' ||
        coalesce(im.status, 'incoming') || '/' ||
        im.id::text || '.json',
    'inbox_message',
    false,
    jsonb_build_object(
        'id', im.id,
        'from_address', im.from_address,
        'to_address', im.to_address,
        'thread_id', im.thread_id,
        'priority', im.priority,
        'action_required', im.action_required,
        'ttl', im.ttl,
        'expires_at', im.expires_at,
        'domain', im.domain,
        'action_type', im.action_type,
        'tags', coalesce(im.tags, '{}'::text[]),
        'context_hash', im.context_hash,
        'subject', im.subject,
        'body', im.body,
        'structured_payload', coalesce(im.structured_payload, '{}'::jsonb),
        'attachments', coalesce(im.attachments, '{}'::text[]),
        'status', im.status,
        'created_at', im.created_at,
        'archived_at', im.archived_at
    )::text,
    'application/json',
    jsonb_build_object(
        'message_id', im.id,
        'role', im.to_address,
        'status', im.status,
        'domain', im.domain,
        'tags', coalesce(im.tags, '{}'::text[])
    ),
    encode(digest(
        '/inbox/' ||
        regexp_replace(coalesce(im.to_address, 'default'), '/+', '_', 'g') || '/' ||
        coalesce(im.status, 'incoming') || '/' ||
        im.id::text || '.json|' ||
        jsonb_build_object(
            'id', im.id,
            'from_address', im.from_address,
            'to_address', im.to_address,
            'thread_id', im.thread_id,
            'priority', im.priority,
            'action_required', im.action_required,
            'ttl', im.ttl,
            'expires_at', im.expires_at,
            'domain', im.domain,
            'action_type', im.action_type,
            'tags', coalesce(im.tags, '{}'::text[]),
            'context_hash', im.context_hash,
            'subject', im.subject,
            'body', im.body,
            'structured_payload', coalesce(im.structured_payload, '{}'::jsonb),
            'attachments', coalesce(im.attachments, '{}'::text[]),
            'status', im.status,
            'created_at', im.created_at,
            'archived_at', im.archived_at
        )::text || '|application/json|' ||
        jsonb_build_object(
            'message_id', im.id,
            'role', im.to_address,
            'status', im.status,
            'domain', im.domain,
            'tags', coalesce(im.tags, '{}'::text[])
        )::text,
        'sha256'
    ), 'hex'),
    1,
    2,
    im.created_at,
    coalesce(im.archived_at, im.created_at)
FROM inbox_messages im
WHERE NOT EXISTS (
    SELECT 1 FROM file_tree ft
    WHERE ft.user_id = im.user_id
      AND ft.path = '/inbox/' ||
          regexp_replace(coalesce(im.to_address, 'default'), '/+', '_', 'g') || '/' ||
          coalesce(im.status, 'incoming') || '/' ||
          im.id::text || '.json'
);

INSERT INTO file_tree (id, user_id, path, kind, is_directory, content, content_type, metadata, checksum, version, min_trust_level, created_at, updated_at)
SELECT
    gen_random_uuid(),
    d.user_id,
    '/devices/' || d.name || '/SKILL.md',
    'device_skill',
    false,
    coalesce(d.skill_md, ''),
    'text/markdown',
    jsonb_build_object(
        'device', d.name,
        'device_type', coalesce(d.device_type, ''),
        'protocol', coalesce(d.protocol, ''),
        'status', coalesce(d.status, 'online')
    ),
    encode(digest(
        '/devices/' || d.name || '/SKILL.md|' ||
        coalesce(d.skill_md, '') || '|text/markdown|' ||
        jsonb_build_object(
            'device', d.name,
            'device_type', coalesce(d.device_type, ''),
            'protocol', coalesce(d.protocol, ''),
            'status', coalesce(d.status, 'online')
        )::text,
        'sha256'
    ), 'hex'),
    1,
    2,
    d.created_at,
    d.updated_at
FROM devices d
WHERE NOT EXISTS (
    SELECT 1 FROM file_tree ft
    WHERE ft.user_id = d.user_id AND ft.path = '/devices/' || d.name || '/SKILL.md'
);

INSERT INTO file_tree (id, user_id, path, kind, is_directory, content, content_type, metadata, checksum, version, min_trust_level, created_at, updated_at)
SELECT
    gen_random_uuid(),
    d.user_id,
    '/devices/' || d.name || '/config.json',
    'device_config',
    false,
    coalesce(d.config, '{}'::jsonb)::text,
    'application/json',
    jsonb_build_object('device', d.name, 'projection', true),
    encode(digest(
        '/devices/' || d.name || '/config.json|' ||
        coalesce(d.config, '{}'::jsonb)::text || '|application/json|' ||
        jsonb_build_object('device', d.name, 'projection', true)::text,
        'sha256'
    ), 'hex'),
    1,
    2,
    d.created_at,
    d.updated_at
FROM devices d
WHERE NOT EXISTS (
    SELECT 1 FROM file_tree ft
    WHERE ft.user_id = d.user_id AND ft.path = '/devices/' || d.name || '/config.json'
);

INSERT INTO file_tree (id, user_id, path, kind, is_directory, content, content_type, metadata, checksum, version, min_trust_level, created_at, updated_at)
SELECT
    gen_random_uuid(),
    r.user_id,
    '/roles/' || r.name || '/SKILL.md',
    'role_skill',
    false,
    '# Role: ' || r.name || E'\n\n' ||
    'Type: ' || coalesce(r.role_type, 'worker') || E'\n' ||
    'Lifecycle: ' || coalesce(r.lifecycle, 'permanent') || E'\n\n' ||
    'Allowed paths: ' || array_to_string(coalesce(r.allowed_paths, '{}'::text[]), ', ') || E'\n' ||
    'Allowed vault scopes: ' || array_to_string(coalesce(r.allowed_vault_scopes, '{}'::text[]), ', ') || E'\n',
    'text/markdown',
    jsonb_build_object(
        'role', r.name,
        'role_type', r.role_type,
        'lifecycle', r.lifecycle,
        'allowed_paths', coalesce(r.allowed_paths, '{}'::text[]),
        'allowed_vault_scopes', coalesce(r.allowed_vault_scopes, '{}'::text[])
    ),
    encode(digest(
        '/roles/' || r.name || '/SKILL.md|' ||
        ('# Role: ' || r.name || E'\n\n' ||
        'Type: ' || coalesce(r.role_type, 'worker') || E'\n' ||
        'Lifecycle: ' || coalesce(r.lifecycle, 'permanent') || E'\n\n' ||
        'Allowed paths: ' || array_to_string(coalesce(r.allowed_paths, '{}'::text[]), ', ') || E'\n' ||
        'Allowed vault scopes: ' || array_to_string(coalesce(r.allowed_vault_scopes, '{}'::text[]), ', ') || E'\n') ||
        '|text/markdown|' ||
        jsonb_build_object(
            'role', r.name,
            'role_type', r.role_type,
            'lifecycle', r.lifecycle,
            'allowed_paths', coalesce(r.allowed_paths, '{}'::text[]),
            'allowed_vault_scopes', coalesce(r.allowed_vault_scopes, '{}'::text[])
        )::text,
        'sha256'
    ), 'hex'),
    1,
    2,
    r.created_at,
    r.created_at
FROM roles r
WHERE NOT EXISTS (
    SELECT 1 FROM file_tree ft
    WHERE ft.user_id = r.user_id AND ft.path = '/roles/' || r.name || '/SKILL.md'
);

INSERT INTO file_tree (id, user_id, path, kind, is_directory, content, content_type, metadata, checksum, version, min_trust_level, created_at, updated_at)
SELECT
    gen_random_uuid(),
    src.user_id,
    src.path,
    'directory',
    true,
    '',
    'directory',
    '{}'::jsonb,
    encode(digest(src.path || '||directory', 'sha256'), 'hex'),
    1,
    1,
    src.created_at,
    src.created_at
FROM (
    SELECT DISTINCT user_id, '/identity/' AS path, min(created_at) OVER (PARTITION BY user_id) AS created_at
    FROM file_tree
    WHERE path LIKE '/identity/%'
    UNION
    SELECT DISTINCT user_id, '/memory/' AS path, min(created_at) OVER (PARTITION BY user_id) AS created_at
    FROM file_tree
    WHERE path LIKE '/memory/%'
    UNION
    SELECT DISTINCT user_id, '/memory/profile/' AS path, min(created_at) OVER (PARTITION BY user_id) AS created_at
    FROM file_tree
    WHERE path LIKE '/memory/profile/%'
    UNION
    SELECT DISTINCT user_id, '/memory/scratch/' AS path, min(created_at) OVER (PARTITION BY user_id) AS created_at
    FROM file_tree
    WHERE path LIKE '/memory/scratch/%'
    UNION
    SELECT user_id, '/projects/' AS path, min(created_at) AS created_at
    FROM projects
    GROUP BY user_id
    UNION
    SELECT user_id, '/projects/' || name || '/' AS path, min(created_at) AS created_at
    FROM projects
    GROUP BY user_id, name
    UNION
    SELECT user_id, '/inbox/' AS path, min(created_at) AS created_at
    FROM inbox_messages
    GROUP BY user_id
    UNION
    SELECT user_id, '/inbox/' || regexp_replace(coalesce(to_address, 'default'), '/+', '_', 'g') || '/' AS path, min(created_at) AS created_at
    FROM inbox_messages
    GROUP BY user_id, to_address
    UNION
    SELECT user_id, '/inbox/' || regexp_replace(coalesce(to_address, 'default'), '/+', '_', 'g') || '/' || coalesce(status, 'incoming') || '/' AS path, min(created_at) AS created_at
    FROM inbox_messages
    GROUP BY user_id, to_address, status
    UNION
    SELECT user_id, '/devices/' AS path, min(created_at) AS created_at
    FROM devices
    GROUP BY user_id
    UNION
    SELECT user_id, '/devices/' || name || '/' AS path, min(created_at) AS created_at
    FROM devices
    GROUP BY user_id, name
    UNION
    SELECT user_id, '/roles/' AS path, min(created_at) AS created_at
    FROM roles
    GROUP BY user_id
    UNION
    SELECT user_id, '/roles/' || name || '/' AS path, min(created_at) AS created_at
    FROM roles
    GROUP BY user_id, name
) AS src
ON CONFLICT (user_id, path) DO NOTHING;

INSERT INTO entry_versions (entry_id, user_id, path, kind, version, change_type, content, content_type, metadata, checksum, min_trust_level, created_at)
SELECT
    ft.id,
    ft.user_id,
    ft.path,
    ft.kind,
    ft.version,
    CASE WHEN ft.deleted_at IS NULL THEN 'create' ELSE 'delete' END,
    coalesce(ft.content, ''),
    coalesce(ft.content_type, ''),
    coalesce(ft.metadata, '{}'::jsonb),
    ft.checksum,
    ft.min_trust_level,
    coalesce(ft.updated_at, ft.created_at, NOW())
FROM file_tree ft
WHERE NOT EXISTS (
    SELECT 1 FROM entry_versions ev
    WHERE ev.entry_id = ft.id AND ev.version = ft.version
);
