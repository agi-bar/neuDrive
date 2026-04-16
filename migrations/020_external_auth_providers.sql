ALTER TABLE auth_bindings
    ADD COLUMN IF NOT EXISTS provider_key VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS issuer VARCHAR(512) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS subject VARCHAR(512) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS email VARCHAR(512) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ;

UPDATE auth_bindings
   SET provider_key = provider
 WHERE provider_key = '';

UPDATE auth_bindings
   SET subject = provider_id
 WHERE subject = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_bindings_provider_key_subject
    ON auth_bindings(provider_key, subject)
    WHERE provider_key <> '' AND subject <> '';

CREATE TABLE IF NOT EXISTS auth_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_key VARCHAR(64) NOT NULL,
    state VARCHAR(512) NOT NULL,
    nonce VARCHAR(512) NOT NULL DEFAULT '',
    code_verifier VARCHAR(512) NOT NULL,
    redirect_url TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider_key, state)
);

CREATE INDEX IF NOT EXISTS idx_auth_transactions_provider_state
    ON auth_transactions(provider_key, state);
