package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/logger"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/skillsarchive"
	"github.com/agi-bar/neudrive/internal/systemskills"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type preparedSkillArchiveEntry struct {
	skillName   string
	relPath     string
	storagePath string
	contentType string
	kind        string
	metadata    map[string]interface{}
	content     string
	data        []byte
	binary      bool
	checksum    string
	blobSHA256  string
}

func (s *ImportService) importSkillsArchiveEntriesOptimized(ctx context.Context, userID uuid.UUID, entries []skillsarchive.Entry, platform, archiveName string) (*SkillsArchiveImportResult, error) {
	if strings.TrimSpace(platform) == "" {
		platform = "skills-archive"
	}
	archiveName = filepath.Base(strings.TrimSpace(archiveName))
	if archiveName == "" {
		archiveName = "skills.zip"
	}

	startedAt := time.Now()
	preparedEntries, totalBytes := prepareSkillArchiveEntries(entries, platform, archiveName)
	preparedAt := time.Now()

	tx, err := s.fileTree.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("import.ImportSkillsArchiveEntries: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	dirCache := map[string]*models.FileTreeEntry{}
	result := &SkillsArchiveImportResult{}
	for i, entry := range preparedEntries {
		savepoint := fmt.Sprintf("skills_import_entry_%d", i)
		if _, err := tx.Exec(ctx, "SAVEPOINT "+savepoint); err != nil {
			return nil, fmt.Errorf("import.ImportSkillsArchiveEntries: create savepoint: %w", err)
		}

		var writeErr error
		if entry.binary {
			writeErr = s.fileTree.writePreparedBinaryImportTx(ctx, tx, userID, entry, dirCache)
		} else {
			writeErr = s.fileTree.writePreparedTextImportTx(ctx, tx, userID, entry, dirCache)
		}
		if writeErr != nil {
			if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+savepoint); err != nil {
				return nil, fmt.Errorf("import.ImportSkillsArchiveEntries: rollback savepoint: %w", err)
			}
			if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT "+savepoint); err != nil {
				return nil, fmt.Errorf("import.ImportSkillsArchiveEntries: release failed savepoint: %w", err)
			}
			dirCache = map[string]*models.FileTreeEntry{}
			result.Errors = append(result.Errors, fmt.Sprintf("skill %s/%s: %v", entry.skillName, entry.relPath, writeErr))
			continue
		}
		if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT "+savepoint); err != nil {
			return nil, fmt.Errorf("import.ImportSkillsArchiveEntries: release savepoint: %w", err)
		}
		result.Imported++
		result.Skills = appendUniqueString(result.Skills, entry.skillName)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("import.ImportSkillsArchiveEntries: commit: %w", err)
	}

	logger.FromContext(ctx).Info("skills archive import bulk write completed",
		"entries", len(preparedEntries),
		"skills", len(result.Skills),
		"payload_bytes", totalBytes,
		"prepare_ms", preparedAt.Sub(startedAt).Milliseconds(),
		"write_ms", time.Since(preparedAt).Milliseconds(),
		"total_ms", time.Since(startedAt).Milliseconds(),
	)
	return result, nil
}

func prepareSkillArchiveEntries(entries []skillsarchive.Entry, platform, archiveName string) ([]preparedSkillArchiveEntry, int64) {
	prepared := make([]preparedSkillArchiveEntry, 0, len(entries))
	var totalBytes int64
	for _, entry := range entries {
		storagePath := hubpath.NormalizeStorage(filepath.ToSlash(filepath.Join("/skills", entry.SkillName, entry.RelPath)))
		baseMetadata := map[string]interface{}{
			"source_platform": platform,
			"source_archive":  archiveName,
			"capture_mode":    "archive",
		}
		contentType := skillsarchive.DetectContentType(entry.RelPath, entry.Data)
		if skillsarchive.LooksBinary(entry.RelPath, entry.Data) {
			hash := sha256.Sum256(entry.Data)
			hashHex := hex.EncodeToString(hash[:])
			metadata := mergeMetadata(baseMetadata, map[string]interface{}{
				"binary":       true,
				"blob_storage": "db",
				"size_bytes":   len(entry.Data),
				"sha256":       hashHex,
			})
			prepared = append(prepared, preparedSkillArchiveEntry{
				skillName:   entry.SkillName,
				relPath:     entry.RelPath,
				storagePath: storagePath,
				contentType: contentType,
				kind:        "skill_asset",
				metadata:    metadata,
				data:        entry.Data,
				binary:      true,
				checksum:    entryChecksum(hubpath.NormalizePublic(storagePath), "", contentType, metadata),
				blobSHA256:  hashHex,
			})
		} else {
			content := string(entry.Data)
			metadata := skillMetadataForPath(storagePath, content, baseMetadata)
			prepared = append(prepared, preparedSkillArchiveEntry{
				skillName:   entry.SkillName,
				relPath:     entry.RelPath,
				storagePath: storagePath,
				contentType: contentType,
				kind:        "skill_file",
				metadata:    metadata,
				content:     content,
				data:        entry.Data,
				checksum:    entryChecksum(hubpath.NormalizePublic(storagePath), content, contentType, metadata),
			})
		}
		totalBytes += int64(len(entry.Data))
	}
	return prepared, totalBytes
}

func (s *FileTreeService) writePreparedTextImportTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, entry preparedSkillArchiveEntry, dirCache map[string]*models.FileTreeEntry) error {
	storagePath := hubpath.NormalizeStorage(entry.storagePath)
	if systemskills.IsProtectedPath(storagePath) {
		return ErrReadOnlyPath
	}
	if _, err := s.ensureImportDirectoryTx(ctx, tx, userID, pathpkg.Dir(strings.TrimSuffix(storagePath, "/")), dirCache); err != nil {
		return err
	}

	now := time.Now().UTC()
	current, err := s.lockEntry(ctx, tx, userID, storagePath)
	if err != nil && !errors.Is(err, ErrEntryNotFound) {
		return err
	}

	minTrust := models.TrustLevelGuest
	if current != nil {
		if err := s.enforceStorageQuotaTx(ctx, tx, userID, current, int64(len(entry.content))); err != nil {
			return err
		}
		var updated models.FileTreeEntry
		err = tx.QueryRow(ctx,
			fmt.Sprintf(`UPDATE file_tree
			 SET kind = $3,
			     is_directory = false,
			     content = $4,
			     content_type = $5,
			     metadata = $6,
			     checksum = $7,
			     version = version + 1,
			     min_trust_level = $8,
			     deleted_at = NULL,
			     updated_at = $9
			 WHERE user_id = $1 AND path = $2
			 RETURNING %s`, fileTreeSelectColumns),
			userID, current.Path, entry.kind, entry.content, entry.contentType, entry.metadata, entry.checksum, minTrust, now,
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
			return fmt.Errorf("update file: %w", err)
		}
		if err := s.deleteBlobTx(ctx, tx, updated.ID); err != nil {
			return err
		}
		return s.insertEntryVersion(ctx, tx, &updated, "update")
	}

	record := &models.FileTreeEntry{
		ID:            uuid.New(),
		UserID:        userID,
		Path:          storagePath,
		Kind:          entry.kind,
		IsDirectory:   false,
		Content:       entry.content,
		ContentType:   entry.contentType,
		Metadata:      entry.metadata,
		Checksum:      entry.checksum,
		Version:       1,
		MinTrustLevel: minTrust,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.enforceStorageQuotaTx(ctx, tx, userID, nil, int64(len(entry.content))); err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO file_tree (
			id, user_id, path, kind, is_directory, content, content_type, metadata,
			checksum, version, min_trust_level, created_at, updated_at
		) VALUES ($1, $2, $3, $4, false, $5, $6, $7, $8, 1, $9, $10, $10)`,
		record.ID, record.UserID, record.Path, record.Kind, record.Content, record.ContentType,
		record.Metadata, record.Checksum, record.MinTrustLevel, record.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert file: %w", err)
	}
	return s.insertEntryVersion(ctx, tx, record, "create")
}

func (s *FileTreeService) writePreparedBinaryImportTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, entry preparedSkillArchiveEntry, dirCache map[string]*models.FileTreeEntry) error {
	storagePath := hubpath.NormalizeStorage(entry.storagePath)
	if systemskills.IsProtectedPath(storagePath) {
		return ErrReadOnlyPath
	}
	if _, err := s.ensureImportDirectoryTx(ctx, tx, userID, pathpkg.Dir(strings.TrimSuffix(storagePath, "/")), dirCache); err != nil {
		return err
	}

	now := time.Now().UTC()
	current, err := s.lockEntry(ctx, tx, userID, storagePath)
	if err != nil && !errors.Is(err, ErrEntryNotFound) {
		return err
	}

	minTrust := models.TrustLevelGuest
	if current != nil {
		if err := s.enforceStorageQuotaTx(ctx, tx, userID, current, int64(len(entry.data))); err != nil {
			return err
		}
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
			userID, current.Path, entry.kind, entry.contentType, entry.metadata, entry.checksum, minTrust, now,
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
			return fmt.Errorf("update binary file: %w", err)
		}
		if err := s.upsertBlobTxWithSHA(ctx, tx, updated.ID, userID, entry.data, entry.blobSHA256, now); err != nil {
			return err
		}
		return s.insertEntryVersion(ctx, tx, &updated, "update")
	}

	record := &models.FileTreeEntry{
		ID:            uuid.New(),
		UserID:        userID,
		Path:          storagePath,
		Kind:          entry.kind,
		IsDirectory:   false,
		Content:       "",
		ContentType:   entry.contentType,
		Metadata:      entry.metadata,
		Checksum:      entry.checksum,
		Version:       1,
		MinTrustLevel: minTrust,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.enforceStorageQuotaTx(ctx, tx, userID, nil, int64(len(entry.data))); err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO file_tree (
			id, user_id, path, kind, is_directory, content, content_type, metadata,
			checksum, version, min_trust_level, created_at, updated_at
		) VALUES ($1, $2, $3, $4, false, '', $5, $6, $7, 1, $8, $9, $9)`,
		record.ID, record.UserID, record.Path, record.Kind, record.ContentType,
		record.Metadata, record.Checksum, record.MinTrustLevel, record.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert binary file: %w", err)
	}
	if err := s.upsertBlobTxWithSHA(ctx, tx, record.ID, userID, entry.data, entry.blobSHA256, now); err != nil {
		return err
	}
	return s.insertEntryVersion(ctx, tx, record, "create")
}

func (s *FileTreeService) ensureImportDirectoryTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, rawPath string, dirCache map[string]*models.FileTreeEntry) (*models.FileTreeEntry, error) {
	storagePath := hubpath.NormalizeStorage(rawPath)
	if storagePath == "." || storagePath == "" || storagePath == "/" {
		return &models.FileTreeEntry{
			UserID:        userID,
			Path:          "/",
			Kind:          "directory",
			IsDirectory:   true,
			ContentType:   "directory",
			Metadata:      map[string]interface{}{},
			Checksum:      entryChecksum("/", "", "directory", map[string]interface{}{}),
			Version:       1,
			MinTrustLevel: models.TrustLevelGuest,
		}, nil
	}
	if !strings.HasSuffix(storagePath, "/") {
		storagePath += "/"
	}
	if entry, ok := dirCache[storagePath]; ok {
		return entry, nil
	}
	if systemskills.IsProtectedPath(storagePath) {
		return nil, ErrReadOnlyPath
	}

	parent := pathpkg.Dir(strings.TrimSuffix(storagePath, "/"))
	if parent != "." && parent != "/" && parent != "" {
		if _, err := s.ensureImportDirectoryTx(ctx, tx, userID, parent, dirCache); err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	checksum := entryChecksum(hubpath.NormalizePublic(storagePath), "", "directory", map[string]interface{}{})
	current, err := s.lockEntry(ctx, tx, userID, storagePath)
	if err != nil && !errors.Is(err, ErrEntryNotFound) {
		return nil, err
	}
	if current != nil {
		if current.IsDirectory && current.DeletedAt == nil {
			dirCache[storagePath] = current
			return current, nil
		}

		var updated models.FileTreeEntry
		err = tx.QueryRow(ctx,
			fmt.Sprintf(`UPDATE file_tree
			 SET kind = 'directory',
			     is_directory = true,
			     content = '',
			     content_type = 'directory',
			     metadata = $3,
			     checksum = $4,
			     version = CASE WHEN deleted_at IS NULL THEN version ELSE version + 1 END,
			     min_trust_level = LEAST(min_trust_level, $5),
			     deleted_at = NULL,
			     updated_at = $6
			 WHERE user_id = $1 AND path = $2
			 RETURNING %s`, fileTreeSelectColumns),
			userID, current.Path, map[string]interface{}{}, checksum, models.TrustLevelGuest, now,
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
			return nil, fmt.Errorf("update directory: %w", err)
		}
		if err := s.deleteBlobTx(ctx, tx, updated.ID); err != nil {
			return nil, err
		}
		if current.DeletedAt != nil {
			if err := s.insertEntryVersion(ctx, tx, &updated, "update"); err != nil {
				return nil, err
			}
		}
		dirCache[storagePath] = &updated
		return &updated, nil
	}

	entry := &models.FileTreeEntry{
		ID:            uuid.New(),
		UserID:        userID,
		Path:          storagePath,
		Kind:          "directory",
		IsDirectory:   true,
		Content:       "",
		ContentType:   "directory",
		Metadata:      map[string]interface{}{},
		Checksum:      checksum,
		Version:       1,
		MinTrustLevel: models.TrustLevelGuest,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO file_tree (
			id, user_id, path, kind, is_directory, content, content_type, metadata,
			checksum, version, min_trust_level, created_at, updated_at
		) VALUES ($1, $2, $3, 'directory', true, '', 'directory', $4, $5, 1, $6, $7, $7)
		ON CONFLICT (user_id, path) DO NOTHING`,
		entry.ID, entry.UserID, entry.Path, entry.Metadata, entry.Checksum, entry.MinTrustLevel, entry.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert directory: %w", err)
	}
	if err := s.insertEntryVersion(ctx, tx, entry, "create"); err != nil {
		return nil, err
	}
	dirCache[storagePath] = entry
	return entry, nil
}
