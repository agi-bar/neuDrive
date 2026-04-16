package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrStorageQuotaExceeded = errors.New("user storage quota exceeded")

type StorageQuotaExceededError struct {
	LimitBytes     int64
	CurrentBytes   int64
	RequestedBytes int64
	ProjectedBytes int64
}

func (e *StorageQuotaExceededError) Error() string {
	return fmt.Sprintf(
		"user storage quota exceeded: limit=%d bytes, current=%d bytes, requested=%d bytes, projected=%d bytes",
		e.LimitBytes,
		e.CurrentBytes,
		e.RequestedBytes,
		e.ProjectedBytes,
	)
}

func (e *StorageQuotaExceededError) Is(target error) bool {
	return target == ErrStorageQuotaExceeded
}

func (s *FileTreeService) enforceStorageQuotaTx(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
	current *models.FileTreeEntry,
	requestedBytes int64,
) error {
	if s == nil || s.userStorageQuotaBytes <= 0 {
		return nil
	}
	if err := s.lockUserStorageQuotaTx(ctx, tx, userID); err != nil {
		return err
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
	return &StorageQuotaExceededError{
		LimitBytes:     s.userStorageQuotaBytes,
		CurrentBytes:   totalBytes,
		RequestedBytes: requestedBytes,
		ProjectedBytes: projectedBytes,
	}
}

func (s *FileTreeService) lockUserStorageQuotaTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	if s == nil || s.userStorageQuotaBytes <= 0 {
		return nil
	}
	var locked uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("filetree.storageQuota: user %s not found", userID)
		}
		return fmt.Errorf("filetree.storageQuota: lock user: %w", err)
	}
	return nil
}

func (s *FileTreeService) totalStorageUsageTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (int64, error) {
	var total int64
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(
			CASE
				WHEN ft.is_directory THEN 0
				WHEN fb.entry_id IS NOT NULL THEN fb.size_bytes
				ELSE OCTET_LENGTH(COALESCE(ft.content, ''))
			END
		), 0)
		  FROM file_tree ft
		  LEFT JOIN file_blobs fb ON fb.entry_id = ft.id
		 WHERE ft.user_id = $1
		   AND ft.deleted_at IS NULL
	`, userID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("filetree.storageQuota: total usage: %w", err)
	}
	return total, nil
}

func (s *FileTreeService) entryStorageUsageTx(ctx context.Context, tx pgx.Tx, entry *models.FileTreeEntry) (int64, error) {
	if entry == nil || entry.IsDirectory {
		return 0, nil
	}

	var total int64
	err := tx.QueryRow(ctx, `
		SELECT CASE
			WHEN ft.is_directory THEN 0
			WHEN fb.entry_id IS NOT NULL THEN fb.size_bytes
			ELSE OCTET_LENGTH(COALESCE(ft.content, ''))
		END
		  FROM file_tree ft
		  LEFT JOIN file_blobs fb ON fb.entry_id = ft.id
		 WHERE ft.id = $1
	`, entry.ID).Scan(&total)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("filetree.storageQuota: entry usage: %w", err)
	}
	return total, nil
}
