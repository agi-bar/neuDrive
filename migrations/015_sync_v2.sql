CREATE TABLE IF NOT EXISTS sync_jobs (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id UUID NULL,
    direction VARCHAR(32) NOT NULL,
    transport VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    source TEXT NOT NULL DEFAULT '',
    mode VARCHAR(32) NOT NULL DEFAULT 'merge',
    filters JSONB NOT NULL DEFAULT '{}'::jsonb,
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_sync_jobs_user_id_created_at
    ON sync_jobs(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS sync_sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_id UUID NOT NULL REFERENCES sync_jobs(id) ON DELETE CASCADE,
    status VARCHAR(32) NOT NULL,
    format VARCHAR(32) NOT NULL,
    mode VARCHAR(32) NOT NULL DEFAULT 'merge',
    manifest JSONB NOT NULL,
    archive_size_bytes BIGINT NOT NULL,
    archive_sha256 VARCHAR(128) NOT NULL,
    chunk_size_bytes BIGINT NOT NULL,
    total_parts INTEGER NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    committed_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_sync_sessions_user_id_created_at
    ON sync_sessions(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_sync_sessions_status_expires_at
    ON sync_sessions(status, expires_at);

CREATE TABLE IF NOT EXISTS sync_session_parts (
    session_id UUID NOT NULL REFERENCES sync_sessions(id) ON DELETE CASCADE,
    part_index INTEGER NOT NULL,
    sha256 VARCHAR(128) NOT NULL,
    size_bytes BIGINT NOT NULL,
    data BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (session_id, part_index)
);

CREATE INDEX IF NOT EXISTS idx_sync_session_parts_session_id
    ON sync_session_parts(session_id);
