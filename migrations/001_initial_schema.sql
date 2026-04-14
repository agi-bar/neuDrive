-- neuDrive: AI Identity and Trust Infrastructure
-- Migration 001: Initial Schema
-- Description: Creates all core tables for the neuDrive system

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(64) UNIQUE NOT NULL,
    display_name VARCHAR(256),
    timezone VARCHAR(64) DEFAULT 'UTC',
    language VARCHAR(16) DEFAULT 'zh-CN',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Auth bindings (GitHub, WeChat, Email, etc.)
CREATE TABLE IF NOT EXISTS auth_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL,  -- 'github', 'wechat', 'email'
    provider_id VARCHAR(256) NOT NULL,
    provider_data JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(provider, provider_id)
);

-- Connections (linked Agent platforms)
CREATE TABLE IF NOT EXISTS connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(256) NOT NULL,
    platform VARCHAR(64) NOT NULL,  -- 'claude', 'gpt', 'feishu', etc.
    trust_level INTEGER NOT NULL DEFAULT 1 CHECK (trust_level BETWEEN 1 AND 4),
    api_key_hash VARCHAR(512),  -- hashed API key for this connection
    api_key_prefix VARCHAR(8),  -- first few chars for display
    config JSONB DEFAULT '{}',
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- File tree entries (the core data model)
CREATE TABLE IF NOT EXISTS file_tree (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    path VARCHAR(1024) NOT NULL,
    is_directory BOOLEAN DEFAULT false,
    content TEXT,
    content_type VARCHAR(64) DEFAULT 'text/markdown',
    metadata JSONB DEFAULT '{}',
    min_trust_level INTEGER DEFAULT 1 CHECK (min_trust_level BETWEEN 1 AND 4),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, path)
);

-- Create GIN index for full-text search on file content
CREATE INDEX IF NOT EXISTS idx_file_tree_content_search
    ON file_tree USING GIN (to_tsvector('simple', coalesce(content, '')));
CREATE INDEX IF NOT EXISTS idx_file_tree_user_path
    ON file_tree(user_id, path);
CREATE INDEX IF NOT EXISTS idx_file_tree_user_directory
    ON file_tree(user_id, path text_pattern_ops) WHERE is_directory = true;

-- Vault entries (encrypted secrets)
CREATE TABLE IF NOT EXISTS vault_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope VARCHAR(256) NOT NULL,  -- e.g., 'identity.personal', 'auth.feishu'
    encrypted_data BYTEA NOT NULL,
    nonce BYTEA NOT NULL,  -- AES-GCM nonce
    description VARCHAR(512),
    min_trust_level INTEGER DEFAULT 4 CHECK (min_trust_level BETWEEN 1 AND 4),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, scope)
);

-- Roles
CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(256) NOT NULL,  -- 'assistant', 'worker-projectname', 'delegate-taskname'
    role_type VARCHAR(32) NOT NULL DEFAULT 'worker',  -- 'assistant', 'worker', 'delegate'
    config JSONB DEFAULT '{}',  -- allowed paths, vault scopes, lifecycle, etc.
    allowed_paths TEXT[] DEFAULT '{}',
    allowed_vault_scopes TEXT[] DEFAULT '{}',
    lifecycle VARCHAR(32) DEFAULT 'permanent',  -- 'session', 'project', 'permanent'
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, name)
);

-- Projects
CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(256) NOT NULL,
    status VARCHAR(32) DEFAULT 'active',  -- 'active', 'archived'
    context_md TEXT DEFAULT '',  -- project context (human-readable)
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, name)
);

-- Project logs (append-only structured event log)
CREATE TABLE IF NOT EXISTS project_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source VARCHAR(64) NOT NULL,  -- 'claude', 'gpt', etc.
    role VARCHAR(64) DEFAULT 'assistant',
    action VARCHAR(256) NOT NULL,
    summary TEXT NOT NULL,
    artifacts TEXT[] DEFAULT '{}',
    tags TEXT[] DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_project_logs_project
    ON project_logs(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_project_logs_tags
    ON project_logs USING GIN(tags);

-- Memory profile (stable user preferences)
CREATE TABLE IF NOT EXISTS memory_profile (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category VARCHAR(64) NOT NULL,  -- 'preferences', 'relationships', 'principles'
    content TEXT NOT NULL,
    source VARCHAR(64),  -- which platform contributed this
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, category)
);

-- Memory scratch (auto-decaying daily summaries)
CREATE TABLE IF NOT EXISTS memory_scratch (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date DATE NOT NULL DEFAULT CURRENT_DATE,
    content TEXT NOT NULL,
    source VARCHAR(64),
    expires_at TIMESTAMPTZ DEFAULT (NOW() + INTERVAL '30 days'),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, date, source)
);

-- Inbox messages (Agent communication)
CREATE TABLE IF NOT EXISTS inbox_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Envelope
    from_address VARCHAR(512) NOT NULL,  -- e.g., 'assistant@de.hub'
    to_address VARCHAR(512) NOT NULL,
    thread_id VARCHAR(256),
    priority VARCHAR(16) DEFAULT 'normal',  -- 'normal', 'urgent'
    action_required BOOLEAN DEFAULT false,
    ttl INTERVAL,
    expires_at TIMESTAMPTZ,

    -- Metadata
    domain VARCHAR(64),  -- 'governance', 'kb', 'collab', 'tools', 'outreach'
    action_type VARCHAR(64),  -- 'task_request', 'info', 'result', 'alert', 'handoff', 'memory_sync'
    tags TEXT[] DEFAULT '{}',
    context_hash VARCHAR(128),

    -- Content
    subject VARCHAR(512) NOT NULL,
    body TEXT NOT NULL,
    structured_payload JSONB DEFAULT '{}',
    attachments TEXT[] DEFAULT '{}',

    -- Status
    status VARCHAR(32) DEFAULT 'incoming',  -- 'incoming', 'read', 'archived'

    created_at TIMESTAMPTZ DEFAULT NOW(),
    archived_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_inbox_user_status
    ON inbox_messages(user_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_inbox_to_address
    ON inbox_messages(to_address, status);
CREATE INDEX IF NOT EXISTS idx_inbox_thread
    ON inbox_messages(thread_id) WHERE thread_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_inbox_tags
    ON inbox_messages USING GIN(tags);
CREATE INDEX IF NOT EXISTS idx_inbox_content_search
    ON inbox_messages USING GIN (to_tsvector('simple', coalesce(subject, '') || ' ' || coalesce(body, '')));

-- Devices
CREATE TABLE IF NOT EXISTS devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(256) NOT NULL,
    device_type VARCHAR(64),  -- 'light', 'ac', 'nas', 'printer', etc.
    brand VARCHAR(128),
    protocol VARCHAR(64),  -- 'http', 'mqtt', 'homekit', 'mijia'
    endpoint VARCHAR(512),
    skill_md TEXT,  -- SKILL.md content
    config JSONB DEFAULT '{}',
    status VARCHAR(32) DEFAULT 'online',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, name)
);

-- Collaboration links (cross-user sharing)
CREATE TABLE IF NOT EXISTS collaborations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    guest_user_id UUID NOT NULL REFERENCES users(id),
    shared_paths TEXT[] NOT NULL,  -- paths the guest can access
    permissions VARCHAR(16) DEFAULT 'read',  -- 'read', 'readwrite'
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(owner_user_id, guest_user_id)
);

-- Activity log (for dashboard stats)
CREATE TABLE IF NOT EXISTS activity_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    connection_id UUID REFERENCES connections(id) ON DELETE SET NULL,
    action VARCHAR(64) NOT NULL,
    path VARCHAR(1024),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_activity_log_user_time
    ON activity_log(user_id, created_at DESC);

-- updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply updated_at triggers
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_connections_updated_at BEFORE UPDATE ON connections
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_file_tree_updated_at BEFORE UPDATE ON file_tree
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_vault_entries_updated_at BEFORE UPDATE ON vault_entries
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_projects_updated_at BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_memory_profile_updated_at BEFORE UPDATE ON memory_profile
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_devices_updated_at BEFORE UPDATE ON devices
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
