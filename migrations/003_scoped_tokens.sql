-- Scoped tokens (like GitHub Personal Access Tokens)
CREATE TABLE IF NOT EXISTS scoped_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(256) NOT NULL,            -- Human-readable name: "Claude Desktop Token"
    token_hash VARCHAR(512) NOT NULL,       -- SHA-256 hash of the token
    token_prefix VARCHAR(12) NOT NULL,      -- First 8 chars for display: "ndt_abcd..."

    -- Scopes (granular permissions)
    scopes TEXT[] NOT NULL DEFAULT '{}',    -- e.g., ['read:profile', 'write:memory', 'read:vault.auth.*']

    -- Trust level ceiling (token can never exceed this)
    max_trust_level INTEGER NOT NULL DEFAULT 3 CHECK (max_trust_level BETWEEN 1 AND 4),

    -- Expiration
    expires_at TIMESTAMPTZ NOT NULL,

    -- Rate limiting
    rate_limit INTEGER DEFAULT 1000,        -- requests per hour
    request_count INTEGER DEFAULT 0,
    rate_limit_reset_at TIMESTAMPTZ DEFAULT NOW(),

    -- Metadata
    last_used_at TIMESTAMPTZ,
    last_used_ip VARCHAR(64),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    revoked_at TIMESTAMPTZ               -- NULL = active, set = revoked
);

CREATE INDEX IF NOT EXISTS idx_scoped_tokens_user ON scoped_tokens(user_id) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_scoped_tokens_hash ON scoped_tokens(token_hash) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_scoped_tokens_prefix ON scoped_tokens(token_prefix);
