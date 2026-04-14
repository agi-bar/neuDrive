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
		`SELECT user_id, root_path, is_active, COALESCE(git_initialized_at, ''), COALESCE(last_synced_at, ''),
		        COALESCE(last_error, ''), created_at, updated_at
		   FROM local_git_mirrors
		  WHERE user_id = ? AND is_active = 1
		  LIMIT 1`,
		userID.String(),
	)

	var (
		rawUserID        string
		rootPath         string
		isActive         bool
		gitInitializedAt string
		lastSyncedAt     string
		lastError        string
		createdAt        string
		updatedAt        string
	)
	if err := row.Scan(&rawUserID, &rootPath, &isActive, &gitInitializedAt, &lastSyncedAt, &lastError, &createdAt, &updatedAt); err != nil {
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
		UserID:           parsedUserID,
		RootPath:         rootPath,
		IsActive:         isActive,
		GitInitializedAt: nullableTime(gitInitializedAt),
		LastSyncedAt:     nullableTime(lastSyncedAt),
		LastError:        lastError,
		CreatedAt:        mustParseTime(createdAt),
		UpdatedAt:        mustParseTime(updatedAt),
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
			user_id, root_path, is_active, git_initialized_at, last_synced_at, last_error, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			root_path = excluded.root_path,
			is_active = excluded.is_active,
			git_initialized_at = excluded.git_initialized_at,
			last_synced_at = excluded.last_synced_at,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at`,
		mirror.UserID.String(),
		mirror.RootPath,
		boolToSQLite(mirror.IsActive),
		localGitMirrorNullableTimeText(mirror.GitInitializedAt),
		localGitMirrorNullableTimeText(mirror.LastSyncedAt),
		mirror.LastError,
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
