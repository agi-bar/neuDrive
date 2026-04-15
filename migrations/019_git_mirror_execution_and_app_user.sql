ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS execution_mode TEXT NOT NULL DEFAULT 'local';

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS sync_state TEXT NOT NULL DEFAULT 'idle';

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS sync_requested_at TIMESTAMPTZ;

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS sync_started_at TIMESTAMPTZ;

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS sync_next_attempt_at TIMESTAMPTZ;

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS sync_attempt_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS github_app_user_login TEXT NOT NULL DEFAULT '';

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS github_app_user_authorized_at TIMESTAMPTZ;

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS github_app_user_refresh_expires_at TIMESTAMPTZ;
