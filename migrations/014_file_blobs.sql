CREATE TABLE IF NOT EXISTS file_blobs (
    entry_id UUID PRIMARY KEY REFERENCES file_tree(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    data BYTEA NOT NULL,
    size_bytes BIGINT NOT NULL,
    sha256 VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_file_blobs_user_id
    ON file_blobs(user_id);
