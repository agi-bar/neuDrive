-- OAuth applications (third-party apps)
CREATE TABLE IF NOT EXISTS oauth_apps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(256) NOT NULL,
    client_id VARCHAR(64) UNIQUE NOT NULL,
    client_secret_hash VARCHAR(512) NOT NULL,
    redirect_uris TEXT[] NOT NULL,
    scopes TEXT[] DEFAULT '{}',
    description TEXT DEFAULT '',
    logo_url VARCHAR(512),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- OAuth authorization codes (short-lived)
CREATE TABLE IF NOT EXISTS oauth_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id UUID NOT NULL REFERENCES oauth_apps(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash VARCHAR(512) NOT NULL,
    scopes TEXT[] DEFAULT '{}',
    redirect_uri VARCHAR(1024) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_oauth_codes_hash ON oauth_codes(code_hash) WHERE used = false;

-- OAuth access grants (which users have authorized which apps)
CREATE TABLE IF NOT EXISTS oauth_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id UUID NOT NULL REFERENCES oauth_apps(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scopes TEXT[] DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(app_id, user_id)
);
