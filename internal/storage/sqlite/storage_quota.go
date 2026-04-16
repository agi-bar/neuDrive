package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
	"github.com/google/uuid"
)

func (s *Store) enforceStorageQuotaTx(
	ctx context.Context,
	tx *sql.Tx,
	userID uuid.UUID,
	current *models.FileTreeEntry,
	requestedBytes int64,
) error {
	if s == nil || s.userStorageQuotaBytes <= 0 {
		return nil
	}

	currentBytes, err := s.entryStorageUsageTx(ctx, tx, current)
	if err != nil {
		return err
	}
	totalBytes, err := s.totalStorageUsageTx(ctx, tx, userID)
	if err != nil {
		return err
	}

	projectedBytes := totalBytes - currentBytes + requestedBytes
	if projectedBytes <= s.userStorageQuotaBytes {
		return nil
	}
	return &services.StorageQuotaExceededError{
		LimitBytes:     s.userStorageQuotaBytes,
		CurrentBytes:   totalBytes,
		RequestedBytes: requestedBytes,
		ProjectedBytes: projectedBytes,
	}
}

func (s *Store) totalStorageUsageTx(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (int64, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(
			CASE
				WHEN ft.is_directory = 1 THEN 0
				WHEN fb.entry_id IS NOT NULL THEN fb.size_bytes
				ELSE length(CAST(COALESCE(ft.content, '') AS BLOB))
			END
		), 0)
		  FROM file_tree ft
		  LEFT JOIN file_blobs fb ON fb.entry_id = ft.id
		 WHERE ft.user_id = ?
		   AND ft.deleted_at IS NULL
	`, userID.String())
	var total int64
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("sqlite.storageQuota: total usage: %w", err)
	}
	return total, nil
}

func (s *Store) entryStorageUsageTx(ctx context.Context, tx *sql.Tx, entry *models.FileTreeEntry) (int64, error) {
	if entry == nil || entry.IsDirectory {
		return 0, nil
	}

	row := tx.QueryRowContext(ctx, `
		SELECT CASE
			WHEN ft.is_directory = 1 THEN 0
			WHEN fb.entry_id IS NOT NULL THEN fb.size_bytes
			ELSE length(CAST(COALESCE(ft.content, '') AS BLOB))
		END
		  FROM file_tree ft
		  LEFT JOIN file_blobs fb ON fb.entry_id = ft.id
		 WHERE ft.id = ?
	`, entry.ID.String())
	var total int64
	if err := row.Scan(&total); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("sqlite.storageQuota: entry usage: %w", err)
	}
	return total, nil
}
