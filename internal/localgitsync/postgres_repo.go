package localgitsync

import (
	"context"
	"fmt"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepo struct {
	db *pgxpool.Pool
}

func NewPostgresRepo(db *pgxpool.Pool) *PostgresRepo {
	if db == nil {
		return nil
	}
	return &PostgresRepo{db: db}
}

func (r *PostgresRepo) GetActiveLocalGitMirror(ctx context.Context, userID uuid.UUID) (*models.LocalGitMirror, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("local git mirror repo not configured")
	}
	var mirror models.LocalGitMirror
	err := r.db.QueryRow(ctx,
		`SELECT user_id, root_path, is_active, auto_commit_enabled, auto_push_enabled, auth_mode, remote_name,
		        remote_url, remote_branch, git_initialized_at, last_synced_at, last_error, last_commit_at,
		        last_commit_hash, last_push_at, last_push_error, github_token_verified_at, github_token_login,
		        github_repo_permission, created_at, updated_at
		   FROM local_git_mirrors
		  WHERE user_id = $1 AND is_active = true
		  LIMIT 1`,
		userID,
	).Scan(
		&mirror.UserID,
		&mirror.RootPath,
		&mirror.IsActive,
		&mirror.AutoCommitEnabled,
		&mirror.AutoPushEnabled,
		&mirror.AuthMode,
		&mirror.RemoteName,
		&mirror.RemoteURL,
		&mirror.RemoteBranch,
		&mirror.GitInitializedAt,
		&mirror.LastSyncedAt,
		&mirror.LastError,
		&mirror.LastCommitAt,
		&mirror.LastCommitHash,
		&mirror.LastPushAt,
		&mirror.LastPushError,
		&mirror.GitHubTokenVerifiedAt,
		&mirror.GitHubTokenLogin,
		&mirror.GitHubRepoPermission,
		&mirror.CreatedAt,
		&mirror.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &mirror, nil
}

func (r *PostgresRepo) UpsertActiveLocalGitMirror(ctx context.Context, mirror models.LocalGitMirror) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("local git mirror repo not configured")
	}
	now := time.Now().UTC()
	if mirror.CreatedAt.IsZero() {
		mirror.CreatedAt = now
	}
	if mirror.UpdatedAt.IsZero() {
		mirror.UpdatedAt = now
	}
	_, err := r.db.Exec(ctx,
		`INSERT INTO local_git_mirrors (
			user_id, root_path, is_active, auto_commit_enabled, auto_push_enabled, auth_mode, remote_name,
			remote_url, remote_branch, git_initialized_at, last_synced_at, last_error, last_commit_at,
			last_commit_hash, last_push_at, last_push_error, github_token_verified_at, github_token_login,
			github_repo_permission, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		ON CONFLICT (user_id) DO UPDATE SET
			root_path = EXCLUDED.root_path,
			is_active = EXCLUDED.is_active,
			auto_commit_enabled = EXCLUDED.auto_commit_enabled,
			auto_push_enabled = EXCLUDED.auto_push_enabled,
			auth_mode = EXCLUDED.auth_mode,
			remote_name = EXCLUDED.remote_name,
			remote_url = EXCLUDED.remote_url,
			remote_branch = EXCLUDED.remote_branch,
			git_initialized_at = EXCLUDED.git_initialized_at,
			last_synced_at = EXCLUDED.last_synced_at,
			last_error = EXCLUDED.last_error,
			last_commit_at = EXCLUDED.last_commit_at,
			last_commit_hash = EXCLUDED.last_commit_hash,
			last_push_at = EXCLUDED.last_push_at,
			last_push_error = EXCLUDED.last_push_error,
			github_token_verified_at = EXCLUDED.github_token_verified_at,
			github_token_login = EXCLUDED.github_token_login,
			github_repo_permission = EXCLUDED.github_repo_permission,
			updated_at = EXCLUDED.updated_at`,
		mirror.UserID,
		mirror.RootPath,
		mirror.IsActive,
		mirror.AutoCommitEnabled,
		mirror.AutoPushEnabled,
		mirror.AuthMode,
		mirror.RemoteName,
		mirror.RemoteURL,
		mirror.RemoteBranch,
		mirror.GitInitializedAt,
		mirror.LastSyncedAt,
		mirror.LastError,
		mirror.LastCommitAt,
		mirror.LastCommitHash,
		mirror.LastPushAt,
		mirror.LastPushError,
		mirror.GitHubTokenVerifiedAt,
		mirror.GitHubTokenLogin,
		mirror.GitHubRepoPermission,
		mirror.CreatedAt,
		mirror.UpdatedAt,
	)
	return err
}

func (r *PostgresRepo) UpdateLocalGitMirrorState(
	ctx context.Context,
	userID uuid.UUID,
	lastSyncedAt *time.Time,
	lastError string,
	gitInitializedAt *time.Time,
) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("local git mirror repo not configured")
	}
	_, err := r.db.Exec(ctx,
		`UPDATE local_git_mirrors
		    SET last_synced_at = $1,
		        last_error = $2,
		        git_initialized_at = COALESCE($3, git_initialized_at),
		        updated_at = $4
		  WHERE user_id = $5`,
		lastSyncedAt,
		lastError,
		gitInitializedAt,
		time.Now().UTC(),
		userID,
	)
	return err
}
