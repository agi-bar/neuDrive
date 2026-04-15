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
		        COALESCE(execution_mode, 'local'), COALESCE(sync_state, 'idle'),
		        auto_commit_enabled, auto_push_enabled, COALESCE(auth_mode, ''), COALESCE(remote_name, ''),
		        COALESCE(remote_url, ''), COALESCE(remote_branch, ''),
		        COALESCE(git_initialized_at, ''), COALESCE(last_synced_at, ''),
		        COALESCE(last_error, ''), COALESCE(last_commit_at, ''), COALESCE(last_commit_hash, ''),
		        COALESCE(last_push_at, ''), COALESCE(last_push_error, ''),
		        COALESCE(sync_requested_at, ''), COALESCE(sync_started_at, ''), COALESCE(sync_next_attempt_at, ''),
		        COALESCE(sync_attempt_count, 0),
		        COALESCE(github_token_verified_at, ''), COALESCE(github_token_login, ''),
		        COALESCE(github_repo_permission, ''), COALESCE(github_app_user_login, ''),
		        COALESCE(github_app_user_authorized_at, ''), COALESCE(github_app_user_refresh_expires_at, ''),
		        created_at, updated_at
		   FROM local_git_mirrors
		  WHERE user_id = ? AND is_active = 1
		  LIMIT 1`,
		userID.String(),
	)

	var (
		rawUserID                     string
		rootPath                      string
		isActive                      bool
		executionMode                 string
		syncState                     string
		autoCommitEnabled             bool
		autoPushEnabled               bool
		authMode                      string
		remoteName                    string
		remoteURL                     string
		remoteBranch                  string
		gitInitializedAt              string
		lastSyncedAt                  string
		lastError                     string
		lastCommitAt                  string
		lastCommitHash                string
		lastPushAt                    string
		lastPushError                 string
		syncRequestedAt               string
		syncStartedAt                 string
		syncNextAttemptAt             string
		syncAttemptCount              int
		githubTokenVerifiedAt         string
		githubTokenLogin              string
		githubRepoPermission          string
		githubAppUserLogin            string
		githubAppUserAuthorizedAt     string
		githubAppUserRefreshExpiresAt string
		createdAt                     string
		updatedAt                     string
	)
	if err := row.Scan(
		&rawUserID,
		&rootPath,
		&isActive,
		&executionMode,
		&syncState,
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
		&syncRequestedAt,
		&syncStartedAt,
		&syncNextAttemptAt,
		&syncAttemptCount,
		&githubTokenVerifiedAt,
		&githubTokenLogin,
		&githubRepoPermission,
		&githubAppUserLogin,
		&githubAppUserAuthorizedAt,
		&githubAppUserRefreshExpiresAt,
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
		UserID:                    parsedUserID,
		RootPath:                  rootPath,
		IsActive:                  isActive,
		ExecutionMode:             executionMode,
		SyncState:                 syncState,
		AutoCommitEnabled:         autoCommitEnabled,
		AutoPushEnabled:           autoPushEnabled,
		AuthMode:                  authMode,
		RemoteName:                remoteName,
		RemoteURL:                 remoteURL,
		RemoteBranch:              remoteBranch,
		GitInitializedAt:          nullableTime(gitInitializedAt),
		LastSyncedAt:              nullableTime(lastSyncedAt),
		LastError:                 lastError,
		LastCommitAt:              nullableTime(lastCommitAt),
		LastCommitHash:            lastCommitHash,
		LastPushAt:                nullableTime(lastPushAt),
		LastPushError:             lastPushError,
		SyncRequestedAt:           nullableTime(syncRequestedAt),
		SyncStartedAt:             nullableTime(syncStartedAt),
		SyncNextAttemptAt:         nullableTime(syncNextAttemptAt),
		SyncAttemptCount:          syncAttemptCount,
		GitHubTokenVerifiedAt:     nullableTime(githubTokenVerifiedAt),
		GitHubTokenLogin:          githubTokenLogin,
		GitHubRepoPermission:      githubRepoPermission,
		GitHubAppUserLogin:        githubAppUserLogin,
		GitHubAppAuthorizedAt:     nullableTime(githubAppUserAuthorizedAt),
		GitHubAppRefreshExpiresAt: nullableTime(githubAppUserRefreshExpiresAt),
		CreatedAt:                 mustParseTime(createdAt),
		UpdatedAt:                 mustParseTime(updatedAt),
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
			user_id, root_path, is_active, execution_mode, sync_state, auto_commit_enabled, auto_push_enabled,
			auth_mode, remote_name, remote_url, remote_branch, git_initialized_at, last_synced_at, last_error,
			last_commit_at, last_commit_hash, last_push_at, last_push_error, sync_requested_at, sync_started_at,
			sync_next_attempt_at, sync_attempt_count, github_token_verified_at, github_token_login,
			github_repo_permission, github_app_user_login, github_app_user_authorized_at,
			github_app_user_refresh_expires_at, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			root_path = excluded.root_path,
			is_active = excluded.is_active,
			execution_mode = excluded.execution_mode,
			sync_state = excluded.sync_state,
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
			sync_requested_at = excluded.sync_requested_at,
			sync_started_at = excluded.sync_started_at,
			sync_next_attempt_at = excluded.sync_next_attempt_at,
			sync_attempt_count = excluded.sync_attempt_count,
			github_token_verified_at = excluded.github_token_verified_at,
			github_token_login = excluded.github_token_login,
			github_repo_permission = excluded.github_repo_permission,
			github_app_user_login = excluded.github_app_user_login,
			github_app_user_authorized_at = excluded.github_app_user_authorized_at,
			github_app_user_refresh_expires_at = excluded.github_app_user_refresh_expires_at,
			updated_at = excluded.updated_at`,
		mirror.UserID.String(),
		mirror.RootPath,
		boolToSQLite(mirror.IsActive),
		mirror.ExecutionMode,
		mirror.SyncState,
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
		localGitMirrorNullableTimeText(mirror.SyncRequestedAt),
		localGitMirrorNullableTimeText(mirror.SyncStartedAt),
		localGitMirrorNullableTimeText(mirror.SyncNextAttemptAt),
		mirror.SyncAttemptCount,
		localGitMirrorNullableTimeText(mirror.GitHubTokenVerifiedAt),
		mirror.GitHubTokenLogin,
		mirror.GitHubRepoPermission,
		mirror.GitHubAppUserLogin,
		localGitMirrorNullableTimeText(mirror.GitHubAppAuthorizedAt),
		localGitMirrorNullableTimeText(mirror.GitHubAppRefreshExpiresAt),
		timeText(mirror.CreatedAt),
		timeText(mirror.UpdatedAt),
	)
	return err
}

func (s *Store) ListQueuedLocalGitMirrors(ctx context.Context, executionMode string, now time.Time, limit int) ([]models.LocalGitMirror, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id
		   FROM local_git_mirrors
		  WHERE is_active = 1
		    AND COALESCE(execution_mode, 'local') = ?
		    AND COALESCE(sync_state, 'idle') = 'queued'
		    AND (sync_next_attempt_at IS NULL OR sync_next_attempt_at = '' OR sync_next_attempt_at <= ?)
		  ORDER BY COALESCE(sync_requested_at, updated_at) ASC
		  LIMIT ?`,
		executionMode,
		timeText(now.UTC()),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mirrors := make([]models.LocalGitMirror, 0)
	userIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var rawUserID string
		if err := rows.Scan(&rawUserID); err != nil {
			return nil, err
		}
		userID, err := uuid.Parse(rawUserID)
		if err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, userID := range userIDs {
		mirror, err := s.GetActiveLocalGitMirror(ctx, userID)
		if err != nil {
			return nil, err
		}
		if mirror != nil {
			mirrors = append(mirrors, *mirror)
		}
	}
	return mirrors, nil
}

func (s *Store) ClaimQueuedLocalGitMirror(ctx context.Context, userID uuid.UUID, executionMode string, startedAt time.Time) (bool, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE local_git_mirrors
		    SET sync_state = 'running',
		        sync_started_at = ?,
		        sync_next_attempt_at = NULL,
		        updated_at = ?
		  WHERE user_id = ?
		    AND is_active = 1
		    AND COALESCE(execution_mode, 'local') = ?
		    AND COALESCE(sync_state, 'idle') = 'queued'
		    AND (sync_next_attempt_at IS NULL OR sync_next_attempt_at = '' OR sync_next_attempt_at <= ?)`,
		timeText(startedAt.UTC()),
		timeText(startedAt.UTC()),
		userID.String(),
		executionMode,
		timeText(startedAt.UTC()),
	)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
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
