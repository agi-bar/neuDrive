-- Scoped access tokens (like GitHub PATs)
CREATE TABLE IF NOT EXISTS access_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(256) NOT NULL,             -- human-readable name like "Claude MCP Token"
    token_hash VARCHAR(512) NOT NULL,        -- SHA-256 hash of the token
    token_prefix VARCHAR(12) NOT NULL,       -- "aht_xxxx" for display

    -- Scopes (granular permissions)
    scopes TEXT[] NOT NULL DEFAULT '{}',     -- e.g. ['read:profile', 'write:memory', 'read:vault.auth']

    -- Trust level ceiling (token can't exceed this)
    max_trust_level INTEGER NOT NULL DEFAULT 3 CHECK (max_trust_level BETWEEN 1 AND 4),

    -- Expiration
    expires_at TIMESTAMPTZ,                  -- NULL = never expires

    -- Metadata
    last_used_at TIMESTAMPTZ,
    last_used_ip VARCHAR(64),
    use_count INTEGER DEFAULT 0,

    -- Status
    is_active BOOLEAN DEFAULT true,
    revoked_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_access_tokens_user ON access_tokens(user_id) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_access_tokens_hash ON access_tokens(token_hash) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_access_tokens_expires ON access_tokens(expires_at) WHERE is_active = true AND expires_at IS NOT NULL;
