package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	pathpkg "path"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/systemskills"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *FileTreeService) WriteBinaryEntry(
	ctx context.Context,
	userID uuid.UUID,
	path string,
	data []byte,
	contentType string,
	opts models.FileTreeWriteOptions,
) (*models.FileTreeEntry, error) {
	if s.repo != nil {
		return s.repo.WriteBinaryEntry(ctx, userID, path, data, contentType, opts)
	}
	storagePath := hubpath.NormalizeStorage(path)
	if systemskills.IsProtectedPath(storagePath) {
		return nil, ErrReadOnlyPath
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if s.db == nil {
		return nil, fmt.Errorf("filetree.WriteBinaryEntry: database not configured")
	}

	parentDirs := make([]string, 0, 4)
	for dir := pathpkg.Dir(strings.TrimSuffix(storagePath, "/")); dir != "." && dir != "/" && dir != ""; dir = pathpkg.Dir(dir) {
		parentDirs = append(parentDirs, dir)
	}
	for i := len(parentDirs) - 1; i >= 0; i-- {
		if err := s.EnsureDirectory(ctx, userID, parentDirs[i]); err != nil {
			return nil, fmt.Errorf("filetree.WriteBinaryEntry: ensure parent dir %q: %w", parentDirs[i], err)
		}
	}

	now := time.Now().UTC()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("filetree.WriteBinaryEntry: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	current, err := s.lockEntry(ctx, tx, userID, storagePath)
	if err != nil && !errors.Is(err, ErrEntryNotFound) {
		return nil, err
	}

	metadata := mergeMetadata(nil, opts.Metadata)
	minTrust := opts.MinTrustLevel
	if minTrust <= 0 {
		minTrust = models.TrustLevelGuest
	}
	kind := strings.TrimSpace(opts.Kind)
	if kind == "" {
		kind = classifyEntryKind(storagePath, false)
	}

	if current != nil {
		if opts.ExpectedVersion != nil && current.Version != *opts.ExpectedVersion {
			return nil, ErrOptimisticLockConflict
		}
		if opts.ExpectedChecksum != "" && current.Checksum != opts.ExpectedChecksum {
			return nil, ErrOptimisticLockConflict
		}
		metadata = mergeMetadata(current.Metadata, opts.Metadata)
		metadata = WithSourceContextMetadata(metadata, ctx)
		metadata = mergeMetadata(metadata, binaryMetadata(data))
		checksum := entryChecksum(hubpath.NormalizePublic(storagePath), "", contentType, metadata)

		var updated models.FileTreeEntry
		err = tx.QueryRow(ctx,
			fmt.Sprintf(`UPDATE file_tree
			 SET kind = $3,
			     is_directory = false,
			     content = '',
			     content_type = $4,
			     metadata = $5,
			     checksum = $6,
			     version = version + 1,
			     min_trust_level = $7,
			     deleted_at = NULL,
			     updated_at = $8
			 WHERE user_id = $1 AND path = $2
			 RETURNING %s`, fileTreeSelectColumns),
			userID, current.Path, kind, contentType, metadata, checksum, minTrust, now,
		).Scan(
			&updated.ID,
			&updated.UserID,
			&updated.Path,
			&updated.Kind,
			&updated.IsDirectory,
			&updated.Content,
			&updated.ContentType,
			&updated.Metadata,
			&updated.Checksum,
			&updated.Version,
			&updated.MinTrustLevel,
			&updated.CreatedAt,
			&updated.UpdatedAt,
			&updated.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("filetree.WriteBinaryEntry: update: %w", err)
		}
		if err := s.upsertBlobTx(ctx, tx, updated.ID, userID, data, now); err != nil {
			return nil, err
		}
		if err := s.insertEntryVersion(ctx, tx, &updated, "update"); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("filetree.WriteBinaryEntry: commit update: %w", err)
		}
		return &updated, nil
	}

	metadata = WithSourceContextMetadata(metadata, ctx)
	metadata = mergeMetadata(metadata, binaryMetadata(data))
	checksum := entryChecksum(hubpath.NormalizePublic(storagePath), "", contentType, metadata)

	entry := &models.FileTreeEntry{
		ID:            uuid.New(),
		UserID:        userID,
		Path:          storagePath,
		Kind:          kind,
		IsDirectory:   false,
		Content:       "",
		ContentType:   contentType,
		Metadata:      metadata,
		Checksum:      checksum,
		Version:       1,
		MinTrustLevel: minTrust,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO file_tree (
			id, user_id, path, kind, is_directory, content, content_type, metadata,
			checksum, version, min_trust_level, created_at, updated_at
		) VALUES ($1, $2, $3, $4, false, '', $5, $6, $7, 1, $8, $9, $9)`,
		entry.ID, entry.UserID, entry.Path, entry.Kind, entry.ContentType,
		entry.Metadata, entry.Checksum, entry.MinTrustLevel, entry.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("filetree.WriteBinaryEntry: insert: %w", err)
	}
	if err := s.upsertBlobTx(ctx, tx, entry.ID, userID, data, now); err != nil {
		return nil, err
	}
	if err := s.insertEntryVersion(ctx, tx, entry, "create"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("filetree.WriteBinaryEntry: commit insert: %w", err)
	}
	return entry, nil
}

func (s *FileTreeService) ReadBinary(ctx context.Context, userID uuid.UUID, path string, trustLevel int) ([]byte, *models.FileTreeEntry, error) {
	if s.repo != nil {
		return s.repo.ReadBinary(ctx, userID, path, trustLevel)
	}
	entry, err := s.Read(ctx, userID, path, trustLevel)
	if err != nil {
		return nil, nil, err
	}
	data, ok, err := s.ReadBlobByEntryID(ctx, entry.ID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, entry, fmt.Errorf("filetree.ReadBinary: blob not found for %s", hubpath.NormalizePublic(path))
	}
	return data, entry, nil
}

func (s *FileTreeService) ReadBlobByEntryID(ctx context.Context, entryID uuid.UUID) ([]byte, bool, error) {
	if reader, ok := s.repo.(interface {
		ReadBlobByEntryID(context.Context, uuid.UUID) ([]byte, bool, error)
	}); ok {
		return reader.ReadBlobByEntryID(ctx, entryID)
	}
	if s.db == nil {
		return nil, false, fmt.Errorf("filetree.ReadBlobByEntryID: database not configured")
	}
	var data []byte
	err := s.db.QueryRow(ctx, `SELECT data FROM file_blobs WHERE entry_id = $1`, entryID).Scan(&data)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("filetree.ReadBlobByEntryID: %w", err)
	}
	return data, true, nil
}

func (s *FileTreeService) upsertBlobTx(ctx context.Context, tx pgx.Tx, entryID, userID uuid.UUID, data []byte, now time.Time) error {
	_, hashHex := binaryMetadataWithHash(data)
	return s.upsertBlobTxWithSHA(ctx, tx, entryID, userID, data, hashHex, now)
}

func (s *FileTreeService) upsertBlobTxWithSHA(ctx context.Context, tx pgx.Tx, entryID, userID uuid.UUID, data []byte, sha256Hex string, now time.Time) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO file_blobs (entry_id, user_id, data, size_bytes, sha256, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)
		 ON CONFLICT (entry_id) DO UPDATE SET
		   user_id = EXCLUDED.user_id,
		   data = EXCLUDED.data,
		   size_bytes = EXCLUDED.size_bytes,
		   sha256 = EXCLUDED.sha256,
		   updated_at = EXCLUDED.updated_at`,
		entryID, userID, data, len(data), sha256Hex, now,
	)
	if err != nil {
		return fmt.Errorf("filetree.upsertBlobTx: %w", err)
	}
	return nil
}

func (s *FileTreeService) deleteBlobTx(ctx context.Context, tx pgx.Tx, entryID uuid.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM file_blobs WHERE entry_id = $1`, entryID)
	if err != nil {
		return fmt.Errorf("filetree.deleteBlobTx: %w", err)
	}
	return nil
}

func binaryMetadata(data []byte) map[string]interface{} {
	metadata, _ := binaryMetadataWithHash(data)
	return metadata
}

func binaryMetadataWithHash(data []byte) (map[string]interface{}, string) {
	hash := sha256.Sum256(data)
	shaHex := hex.EncodeToString(hash[:])
	return map[string]interface{}{
		"binary":       true,
		"blob_storage": "db",
		"size_bytes":   len(data),
		"sha256":       shaHex,
	}, shaHex
}
