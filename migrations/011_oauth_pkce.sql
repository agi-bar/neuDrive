ALTER TABLE oauth_codes
    ADD COLUMN IF NOT EXISTS code_challenge VARCHAR(512) DEFAULT '',
    ADD COLUMN IF NOT EXISTS code_challenge_method VARCHAR(32) DEFAULT '';
