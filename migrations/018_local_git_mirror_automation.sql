ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS auto_commit_enabled BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS auto_push_enabled BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS auth_mode TEXT NOT NULL DEFAULT 'local_credentials';

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS remote_name TEXT NOT NULL DEFAULT 'origin';

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS remote_url TEXT NOT NULL DEFAULT '';

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS remote_branch TEXT NOT NULL DEFAULT 'main';

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS last_commit_at TIMESTAMPTZ;

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS last_commit_hash TEXT NOT NULL DEFAULT '';

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS last_push_at TIMESTAMPTZ;

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS last_push_error TEXT NOT NULL DEFAULT '';

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS github_token_verified_at TIMESTAMPTZ;

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS github_token_login TEXT NOT NULL DEFAULT '';

ALTER TABLE local_git_mirrors
    ADD COLUMN IF NOT EXISTS github_repo_permission TEXT NOT NULL DEFAULT '';
