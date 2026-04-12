CREATE TABLE IF NOT EXISTS local_git_mirrors (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    root_path TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    git_initialized_at TIMESTAMPTZ,
    last_synced_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
