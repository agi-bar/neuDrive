CREATE TABLE IF NOT EXISTS memory_conflicts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category VARCHAR(64) NOT NULL,
    source_a VARCHAR(64) NOT NULL,
    content_a TEXT NOT NULL,
    source_b VARCHAR(64) NOT NULL,
    content_b TEXT NOT NULL,
    status VARCHAR(32) DEFAULT 'pending', -- pending, resolved_keep_a, resolved_keep_b, resolved_keep_both, dismissed
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_memory_conflicts_user ON memory_conflicts(user_id, status);
