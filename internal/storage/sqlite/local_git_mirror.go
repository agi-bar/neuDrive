package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
)

func (s *Store) GetActiveLocalGitMirror(ctx context.Context, userID uuid.UUID) (*models.LocalGitMirror, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT user_id, root_path, is_active,
		        auto_commit_enabled, auto_push_enabled, COALESCE(auth_mode, ''), COALESCE(remote_name, ''),
		        COALESCE(remote_url, ''), COALESCE(remote_branch, ''),
		        COALESCE(git_initialized_at, ''), COALESCE(last_synced_at, ''),
		        COALESCE(last_error, ''), COALESCE(last_commit_at, ''), COALESCE(last_commit_hash, ''),
		        COALESCE(last_push_at, ''), COALESCE(last_push_error, ''),
		        COALESCE(github_token_verified_at, ''), COALESCE(github_token_login, ''),
		        COALESCE(github_repo_permission, ''), created_at, updated_at
		   FROM local_git_mirrors
		  WHERE user_id = ? AND is_active = 1
		  LIMIT 1`,
		userID.String(),
	)

	var (
		rawUserID             string
		rootPath              string
		isActive              bool
		autoCommitEnabled     bool
		autoPushEnabled       bool
		authMode              string
		remoteName            string
		remoteURL             string
		remoteBranch          string
		gitInitializedAt      string
		lastSyncedAt          string
		lastError             string
		lastCommitAt          string
		lastCommitHash        string
		lastPushAt            string
		lastPushError         string
		githubTokenVerifiedAt string
		githubTokenLogin      string
		githubRepoPermission  string
		createdAt             string
		updatedAt             string
	)
	if err := row.Scan(
		&rawUserID,
		&rootPath,
		&isActive,
		&autoCommitEnabled,
		&autoPushEnabled,
		&authMode,
		&remoteName,
		&remoteURL,
		&remoteBranch,
		&gitInitializedAt,
		&lastSyncedAt,
		&lastError,
		&lastCommitAt,
		&lastCommitHash,
		&lastPushAt,
		&lastPushError,
		&githubTokenVerifiedAt,
		&githubTokenLogin,
		&githubRepoPermission,
		&createdAt,
		&updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	parsedUserID, err := uuid.Parse(rawUserID)
	if err != nil {
		return nil, err
	}
	return &models.LocalGitMirror{
		UserID:                parsedUserID,
		RootPath:              rootPath,
		IsActive:              isActive,
		AutoCommitEnabled:     autoCommitEnabled,
		AutoPushEnabled:       autoPushEnabled,
		AuthMode:              authMode,
		RemoteName:            remoteName,
		RemoteURL:             remoteURL,
		RemoteBranch:          remoteBranch,
		GitInitializedAt:      nullableTime(gitInitializedAt),
		LastSyncedAt:          nullableTime(lastSyncedAt),
		LastError:             lastError,
		LastCommitAt:          nullableTime(lastCommitAt),
		LastCommitHash:        lastCommitHash,
		LastPushAt:            nullableTime(lastPushAt),
		LastPushError:         lastPushError,
		GitHubTokenVerifiedAt: nullableTime(githubTokenVerifiedAt),
		GitHubTokenLogin:      githubTokenLogin,
		GitHubRepoPermission:  githubRepoPermission,
		CreatedAt:             mustParseTime(createdAt),
		UpdatedAt:             mustParseTime(updatedAt),
	}, nil
}

func (s *Store) UpsertActiveLocalGitMirror(ctx context.Context, mirror models.LocalGitMirror) error {
	now := time.Now().UTC()
	if mirror.CreatedAt.IsZero() {
		mirror.CreatedAt = now
	}
	mirror.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO local_git_mirrors (
			user_id, root_path, is_active, auto_commit_enabled, auto_push_enabled, auth_mode, remote_name, remote_url,
			remote_branch, git_initialized_at, last_synced_at, last_error, last_commit_at, last_commit_hash,
			last_push_at, last_push_error, github_token_verified_at, github_token_login, github_repo_permission,
			created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			root_path = excluded.root_path,
			is_active = excluded.is_active,
			auto_commit_enabled = excluded.auto_commit_enabled,
			auto_push_enabled = excluded.auto_push_enabled,
			auth_mode = excluded.auth_mode,
			remote_name = excluded.remote_name,
			remote_url = excluded.remote_url,
			remote_branch = excluded.remote_branch,
			git_initialized_at = excluded.git_initialized_at,
			last_synced_at = excluded.last_synced_at,
			last_error = excluded.last_error,
			last_commit_at = excluded.last_commit_at,
			last_commit_hash = excluded.last_commit_hash,
			last_push_at = excluded.last_push_at,
			last_push_error = excluded.last_push_error,
			github_token_verified_at = excluded.github_token_verified_at,
			github_token_login = excluded.github_token_login,
			github_repo_permission = excluded.github_repo_permission,
			updated_at = excluded.updated_at`,
		mirror.UserID.String(),
		mirror.RootPath,
		boolToSQLite(mirror.IsActive),
		boolToSQLite(mirror.AutoCommitEnabled),
		boolToSQLite(mirror.AutoPushEnabled),
		mirror.AuthMode,
		mirror.RemoteName,
		mirror.RemoteURL,
		mirror.RemoteBranch,
		localGitMirrorNullableTimeText(mirror.GitInitializedAt),
		localGitMirrorNullableTimeText(mirror.LastSyncedAt),
		mirror.LastError,
		localGitMirrorNullableTimeText(mirror.LastCommitAt),
		mirror.LastCommitHash,
		localGitMirrorNullableTimeText(mirror.LastPushAt),
		mirror.LastPushError,
		localGitMirrorNullableTimeText(mirror.GitHubTokenVerifiedAt),
		mirror.GitHubTokenLogin,
		mirror.GitHubRepoPermission,
		timeText(mirror.CreatedAt),
		timeText(mirror.UpdatedAt),
	)
	return err
}

func (s *Store) UpdateLocalGitMirrorState(
	ctx context.Context,
	userID uuid.UUID,
	lastSyncedAt *time.Time,
	lastError string,
	gitInitializedAt *time.Time,
) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE local_git_mirrors
		    SET last_synced_at = ?,
		        last_error = ?,
		        git_initialized_at = COALESCE(?, git_initialized_at),
		        updated_at = ?
		  WHERE user_id = ?`,
		localGitMirrorNullableTimeText(lastSyncedAt),
		lastError,
		localGitMirrorNullableTimeText(gitInitializedAt),
		timeText(time.Now().UTC()),
		userID.String(),
	)
	return err
}

func boolToSQLite(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableTime(value string) *time.Time {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	ts := mustParseTime(value)
	if ts.IsZero() {
		return nil
	}
	return &ts
}

func localGitMirrorNullableTimeText(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return timeText(value.UTC())
}
