package services

import (
	"context"
	"errors"
	"fmt"
	pathpkg "path"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/systemskills"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const fileTreeSelectColumns = `
	id,
	user_id,
	path,
	kind,
	is_directory,
	COALESCE(content, ''),
	COALESCE(content_type, ''),
	COALESCE(metadata, '{}'),
	COALESCE(checksum, ''),
	COALESCE(version, 1),
	min_trust_level,
	created_at,
	updated_at,
	deleted_at
`

type FileTreeService struct {
	db                    *pgxpool.Pool
	repo                  FileTreeRepo
	userStorageQuotaBytes int64
}

func NewFileTreeService(db *pgxpool.Pool) *FileTreeService {
	return &FileTreeService{db: db}
}

func NewFileTreeServiceWithRepo(repo FileTreeRepo) *FileTreeService {
	return &FileTreeService{repo: repo}
}

func (s *FileTreeService) SetUserStorageQuotaBytes(limit int64) {
	if s == nil {
		return
	}
	if limit < 0 {
		limit = 0
	}
	s.userStorageQuotaBytes = limit
}

// List returns immediate children under the requested directory path.
func (s *FileTreeService) List(ctx context.Context, userID uuid.UUID, path string, trustLevel int) ([]models.FileTreeEntry, error) {
	if s.repo != nil {
		return s.repo.List(ctx, userID, path, trustLevel)
	}
	path = hubpath.NormalizeStorage(path)
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	systemEntries, handledBySystem := systemskills.ListEntries(path)
	if s.db == nil {
		if handledBySystem {
			return systemEntries, nil
		}
		return nil, fmt.Errorf("filetree.List: database not configured")
	}

	args := []interface{}{userID, trustLevel}
	args = append(args, s.prefixArgs(path)...)
	rows, err := s.db.Query(ctx,
		fmt.Sprintf(`SELECT %s
		 FROM file_tree
		 WHERE user_id = $1
		   AND deleted_at IS NULL
		   AND min_trust_level <= $2
		   AND %s
		 ORDER BY path ASC`, fileTreeSelectColumns, s.prefixCondition(path, 3)),
		args...)
	if err != nil {
		return nil, fmt.Errorf("filetree.List: %w", err)
	}
	defer rows.Close()

	entries, err := scanFileTreeEntries(rows)
	if err != nil {
		return nil, err
	}
	entries = immediateChildEntries(path, userID, entries)
	if !handledBySystem {
		return entries, nil
	}
	if systemskills.IsProtectedPath(path) {
		return mergeFileTreeEntries(entries, systemEntries), nil
	}
	return mergeFileTreeEntries(systemEntries, entries), nil
}

// Read returns a single live entry, respecting trust level.
func (s *FileTreeService) Read(ctx context.Context, userID uuid.UUID, path string, trustLevel int) (*models.FileTreeEntry, error) {
	if s.repo != nil {
		return s.repo.Read(ctx, userID, path, trustLevel)
	}
	path = hubpath.NormalizeStorage(path)
	if entry, ok, err := systemskills.ReadEntry(path); err != nil {
		return nil, fmt.Errorf("filetree.Read: %w", err)
	} else if ok {
		return entry, nil
	}
	if s.db == nil {
		return nil, ErrEntryNotFound
	}

	var entry models.FileTreeEntry
	err := s.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s
		 FROM file_tree
		 WHERE user_id = $1
		   AND deleted_at IS NULL
		   AND path = $2
		 LIMIT 1`, fileTreeSelectColumns),
		userID, path).
		Scan(
			&entry.ID,
			&entry.UserID,
			&entry.Path,
			&entry.Kind,
			&entry.IsDirectory,
			&entry.Content,
			&entry.ContentType,
			&entry.Metadata,
			&entry.Checksum,
			&entry.Version,
			&entry.MinTrustLevel,
			&entry.CreatedAt,
			&entry.UpdatedAt,
			&entry.DeletedAt,
		)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEntryNotFound
		}
		return nil, fmt.Errorf("filetree.Read: %w", err)
	}
	if entry.MinTrustLevel > trustLevel {
		return nil, fmt.Errorf("filetree.Read: insufficient trust level (need %d, have %d)", entry.MinTrustLevel, trustLevel)
	}
	if entry.IsDirectory {
		if snapshot, snapshotErr := s.Snapshot(ctx, userID, entry.Path, trustLevel); snapshotErr == nil {
			entry = EnrichBundleDirectoryEntry(entry, snapshot.Entries)
		}
	}
	return &entry, nil
}

// Write is the backwards-compatible file write entry point.
func (s *FileTreeService) Write(ctx context.Context, userID uuid.UUID, path, content, contentType string, minTrustLevel int) (*models.FileTreeEntry, error) {
	return s.WriteEntry(ctx, userID, path, content, contentType, models.FileTreeWriteOptions{
		MinTrustLevel: minTrustLevel,
	})
}

// WriteEntry creates or updates a live file entry with optimistic version checks.
func (s *FileTreeService) WriteEntry(
	ctx context.Context,
	userID uuid.UUID,
	path string,
	content string,
	contentType string,
	opts models.FileTreeWriteOptions,
) (*models.FileTreeEntry, error) {
	if s.repo != nil {
		return s.repo.WriteEntry(ctx, userID, path, content, contentType, opts)
	}
	storagePath := hubpath.NormalizeStorage(path)
	if systemskills.IsProtectedPath(storagePath) {
		return nil, ErrReadOnlyPath
	}
	if contentType == "" {
		contentType = "text/plain"
	}
	if s.db == nil {
		return nil, fmt.Errorf("filetree.WriteEntry: database not configured")
	}

	parentDirs := make([]string, 0, 4)
	for dir := pathpkg.Dir(strings.TrimSuffix(storagePath, "/")); dir != "." && dir != "/" && dir != ""; dir = pathpkg.Dir(dir) {
		parentDirs = append(parentDirs, dir)
	}
	for i := len(parentDirs) - 1; i >= 0; i-- {
		if err := s.EnsureDirectory(ctx, userID, parentDirs[i]); err != nil {
			return nil, fmt.Errorf("filetree.WriteEntry: ensure parent dir %q: %w", parentDirs[i], err)
		}
	}

	now := time.Now().UTC()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("filetree.WriteEntry: begin tx: %w", err)
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
		metadata = skillMetadataForPath(storagePath, content, metadata)
		if err := s.enforceStorageQuotaTx(ctx, tx, userID, current, int64(len(content))); err != nil {
			return nil, err
		}
		checksum := entryChecksum(hubpath.NormalizePublic(storagePath), content, contentType, metadata)

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
			userID, current.Path, kind, content, contentType, metadata, checksum, minTrust, now,
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
			return nil, fmt.Errorf("filetree.WriteEntry: update: %w", err)
		}
		if err := s.deleteBlobTx(ctx, tx, updated.ID); err != nil {
			return nil, err
		}

		if err := s.insertEntryVersion(ctx, tx, &updated, "update"); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("filetree.WriteEntry: commit update: %w", err)
		}
		return &updated, nil
	}

	metadata = WithSourceContextMetadata(metadata, ctx)
	metadata = skillMetadataForPath(storagePath, content, metadata)
	if err := s.enforceStorageQuotaTx(ctx, tx, userID, nil, int64(len(content))); err != nil {
		return nil, err
	}
	checksum := entryChecksum(hubpath.NormalizePublic(storagePath), content, contentType, metadata)

	entryID := uuid.New()
	entry := &models.FileTreeEntry{
		ID:            entryID,
		UserID:        userID,
		Path:          storagePath,
		Kind:          kind,
		IsDirectory:   false,
		Content:       content,
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
		) VALUES ($1, $2, $3, $4, false, $5, $6, $7, $8, 1, $9, $10, $10)`,
		entry.ID, entry.UserID, entry.Path, entry.Kind, entry.Content, entry.ContentType,
		entry.Metadata, entry.Checksum, entry.MinTrustLevel, entry.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("filetree.WriteEntry: insert: %w", err)
	}

	if err := s.insertEntryVersion(ctx, tx, entry, "create"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("filetree.WriteEntry: commit insert: %w", err)
	}
	return entry, nil
}

// Delete tombstones a deletable entry subtree while preserving protected descendants.
func (s *FileTreeService) Delete(ctx context.Context, userID uuid.UUID, path string) error {
	if s.repo != nil {
		return s.repo.Delete(ctx, userID, path)
	}
	storagePath, err := s.resolveDeletePath(ctx, userID, path)
	if err != nil {
		return err
	}
	if systemskills.IsProtectedPath(storagePath) {
		return ErrReadOnlyPath
	}
	if s.db == nil {
		return fmt.Errorf("filetree.Delete: database not configured")
	}

	snapshot, err := s.Snapshot(ctx, userID, storagePath, models.TrustLevelFull)
	if err != nil {
		return err
	}
	if len(snapshot.Entries) == 0 {
		if _, handled := systemskills.ListEntries(storagePath); handled {
			return nil
		}
		return ErrEntryNotFound
	}
	entriesToDelete := deletableEntriesForDeletion(storagePath, snapshot.Entries)
	if len(entriesToDelete) == 0 {
		return nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("filetree.Delete: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	for _, entry := range entriesToDelete {
		current, err := s.lockEntry(ctx, tx, userID, entry.Path)
		if err != nil {
			if errors.Is(err, ErrEntryNotFound) {
				continue
			}
			return err
		}

		nextVersion := current.Version + 1
		var deleted models.FileTreeEntry
		err = tx.QueryRow(ctx,
			fmt.Sprintf(`UPDATE file_tree
			 SET deleted_at = $3,
			     version = $4,
			     checksum = $5,
			     updated_at = $3
			 WHERE user_id = $1 AND path = $2
			 RETURNING %s`, fileTreeSelectColumns),
			userID, current.Path, now, nextVersion, deletedEntryChecksum(hubpath.NormalizePublic(current.Path), nextVersion),
		).Scan(
			&deleted.ID,
			&deleted.UserID,
			&deleted.Path,
			&deleted.Kind,
			&deleted.IsDirectory,
			&deleted.Content,
			&deleted.ContentType,
			&deleted.Metadata,
			&deleted.Checksum,
			&deleted.Version,
			&deleted.MinTrustLevel,
			&deleted.CreatedAt,
			&deleted.UpdatedAt,
			&deleted.DeletedAt,
		)
		if err != nil {
			return fmt.Errorf("filetree.Delete: update: %w", err)
		}
		if err := s.deleteBlobTx(ctx, tx, deleted.ID); err != nil {
			return err
		}

		if err := s.insertEntryVersion(ctx, tx, &deleted, "delete"); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("filetree.Delete: commit: %w", err)
	}
	return nil
}

// Search performs full-text search across live entries and indexed metadata.
func (s *FileTreeService) Search(ctx context.Context, userID uuid.UUID, query string, trustLevel int, pathPrefix string) ([]models.FileTreeEntry, error) {
	if s.repo != nil {
		return s.repo.Search(ctx, userID, query, trustLevel, pathPrefix)
	}
	args := []interface{}{userID, trustLevel}
	argIdx := 3

	where := []string{
		"user_id = $1",
		"deleted_at IS NULL",
		"min_trust_level <= $2",
		"is_directory = false",
	}

	if pathPrefix != "" {
		where = append(where, s.prefixCondition(pathPrefix, argIdx))
		prefixArgs := s.prefixArgs(pathPrefix)
		args = append(args, prefixArgs...)
		argIdx += len(prefixArgs)
	}

	searchTextExpr := "coalesce(content, '') || ' ' || coalesce(metadata::text, '')"
	sqlQuery := fmt.Sprintf(`SELECT %s
		FROM file_tree
		WHERE %s
		  AND (
		    to_tsvector('simple', %s) @@ plainto_tsquery('simple', $%d)
		    OR %s ILIKE $%d
		  )
		ORDER BY CASE
		    WHEN to_tsvector('simple', %s) @@ plainto_tsquery('simple', $%d) THEN 0
		    ELSE 1
		  END,
		  updated_at DESC
		LIMIT 50`,
		fileTreeSelectColumns,
		strings.Join(where, " AND "),
		searchTextExpr,
		argIdx,
		searchTextExpr,
		argIdx+1,
		searchTextExpr,
		argIdx,
	)
	args = append(args, query, "%"+query+"%")

	rows, err := s.db.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("filetree.Search: %w", err)
	}
	defer rows.Close()

	return scanFileTreeEntries(rows)
}

func (s *FileTreeService) EnsureDirectory(ctx context.Context, userID uuid.UUID, path string) error {
	if s.repo != nil {
		return s.repo.EnsureDirectory(ctx, userID, path)
	}
	_, err := s.EnsureDirectoryWithMetadata(ctx, userID, path, nil, models.TrustLevelGuest)
	return err
}

func (s *FileTreeService) EnsureDirectoryWithMetadata(
	ctx context.Context,
	userID uuid.UUID,
	path string,
	metadata map[string]interface{},
	minTrustLevel int,
) (*models.FileTreeEntry, error) {
	if s.repo != nil {
		// Repo-backed test/local adapters do not currently expose a dedicated
		// metadata-aware ensure operation. Fall back to creating the directory
		// so higher-level import flows can proceed; full metadata persistence is
		// still handled by the database-backed implementation below.
		if err := s.repo.EnsureDirectory(ctx, userID, path); err != nil {
			return nil, err
		}
		return s.repo.Read(ctx, userID, path, models.TrustLevelFull)
	}
	storagePath := hubpath.NormalizeStorage(path)
	if !strings.HasSuffix(storagePath, "/") {
		storagePath += "/"
	}
	if systemskills.IsProtectedPath(storagePath) {
		return nil, ErrReadOnlyPath
	}
	if storagePath == "/" {
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
	if s.db == nil {
		return nil, fmt.Errorf("filetree.EnsureDirectory: database not configured")
	}

	parentDirs := make([]string, 0, 4)
	for dir := pathpkg.Dir(strings.TrimSuffix(storagePath, "/")); dir != "." && dir != "/" && dir != ""; dir = pathpkg.Dir(dir) {
		parentDirs = append(parentDirs, dir)
	}
	for i := len(parentDirs) - 1; i >= 0; i-- {
		parent := parentDirs[i]
		if parent == "." || parent == "" {
			continue
		}
		if !strings.HasSuffix(parent, "/") {
			parent += "/"
		}
		if _, err := s.EnsureDirectoryWithMetadata(ctx, userID, parent, nil, models.TrustLevelGuest); err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	minTrust := minTrustLevel
	if minTrust <= 0 {
		minTrust = models.TrustLevelGuest
	}
	meta := mergeMetadata(nil, metadata)
	meta = WithSourceContextMetadata(meta, ctx)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("filetree.EnsureDirectory: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	current, err := s.lockEntry(ctx, tx, userID, storagePath)
	if err != nil && !errors.Is(err, ErrEntryNotFound) {
		return nil, err
	}

	if current != nil {
		mergedMeta := mergeMetadata(current.Metadata, meta)
		checksum := entryChecksum(hubpath.NormalizePublic(storagePath), "", "directory", mergedMeta)
		dirKind := bundleEntryKind(metadataString(mergedMeta, "bundle_kind"))
		var updated models.FileTreeEntry
		err = tx.QueryRow(ctx,
			fmt.Sprintf(`UPDATE file_tree
			 SET kind = $7,
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
			userID, current.Path, mergedMeta, checksum, minTrust, now, dirKind,
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
			return nil, fmt.Errorf("filetree.EnsureDirectory: update: %w", err)
		}
		if err := s.deleteBlobTx(ctx, tx, updated.ID); err != nil {
			return nil, err
		}

		// Directory ensure is idempotent; only emit a version when reviving a tombstone.
		if current.DeletedAt != nil {
			if err := s.insertEntryVersion(ctx, tx, &updated, "update"); err != nil {
				return nil, err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("filetree.EnsureDirectory: commit update: %w", err)
		}
		return &updated, nil
	}

	checksum := entryChecksum(hubpath.NormalizePublic(storagePath), "", "directory", meta)
	dirKind := bundleEntryKind(metadataString(meta, "bundle_kind"))

	entry := &models.FileTreeEntry{
		ID:            uuid.New(),
		UserID:        userID,
		Path:          storagePath,
		Kind:          dirKind,
		IsDirectory:   true,
		Content:       "",
		ContentType:   "directory",
		Metadata:      meta,
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
		) VALUES ($1, $2, $3, $4, true, '', 'directory', $5, $6, 1, $7, $8, $8)
		ON CONFLICT (user_id, path) DO NOTHING`,
		entry.ID, entry.UserID, entry.Path, entry.Kind, entry.Metadata, entry.Checksum, entry.MinTrustLevel, entry.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("filetree.EnsureDirectory: insert: %w", err)
	}

	if err := s.insertEntryVersion(ctx, tx, entry, "create"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("filetree.EnsureDirectory: commit insert: %w", err)
	}
	return entry, nil
}

func (s *FileTreeService) Snapshot(ctx context.Context, userID uuid.UUID, pathPrefix string, trustLevel int) (*models.EntrySnapshot, error) {
	if s.repo != nil {
		return s.repo.Snapshot(ctx, userID, pathPrefix, trustLevel)
	}
	pathPrefix = hubpath.NormalizeStorage(pathPrefix)
	rows, err := s.db.Query(ctx,
		fmt.Sprintf(`SELECT %s
		 FROM file_tree
		 WHERE user_id = $1
		   AND deleted_at IS NULL
		   AND min_trust_level <= $2
		   AND %s
		 ORDER BY path ASC`, fileTreeSelectColumns, s.prefixCondition(pathPrefix, 3)),
		append([]interface{}{userID, trustLevel}, s.prefixArgs(pathPrefix)...)...,
	)
	if err != nil {
		return nil, fmt.Errorf("filetree.Snapshot: list: %w", err)
	}
	defer rows.Close()

	entries, err := scanFileTreeEntries(rows)
	if err != nil {
		return nil, err
	}

	cursor, err := s.latestCursor(ctx, userID, pathPrefix)
	if err != nil {
		return nil, err
	}

	return &models.EntrySnapshot{
		Path:         hubpath.NormalizePublic(pathPrefix),
		Cursor:       cursor,
		RootChecksum: rootChecksum(entries),
		Entries:      entries,
	}, nil
}

func (s *FileTreeService) Changes(ctx context.Context, userID uuid.UUID, cursor int64, pathPrefix string, trustLevel int) ([]models.EntryChange, int64, error) {
	pathPrefix = hubpath.NormalizeStorage(pathPrefix)
	args := append([]interface{}{userID, cursor, trustLevel}, s.prefixArgs(pathPrefix)...)
	rows, err := s.db.Query(ctx,
		fmt.Sprintf(`SELECT
			cursor,
			id,
			entry_id,
			user_id,
			path,
			kind,
			version,
			change_type,
			COALESCE(content, ''),
			COALESCE(content_type, ''),
			COALESCE(metadata, '{}'),
			COALESCE(checksum, ''),
			min_trust_level,
			created_at
		 FROM entry_versions
		 WHERE user_id = $1
		   AND cursor > $2
		   AND min_trust_level <= $3
		   AND %s
		 ORDER BY cursor ASC`, s.entryVersionPrefixCondition(pathPrefix, 4)),
		args...,
	)
	if err != nil {
		return nil, cursor, fmt.Errorf("filetree.Changes: %w", err)
	}
	defer rows.Close()

	changes := make([]models.EntryChange, 0, 32)
	nextCursor := cursor
	for rows.Next() {
		var version models.EntryVersion
		if err := rows.Scan(
			&version.Cursor,
			&version.ID,
			&version.EntryID,
			&version.UserID,
			&version.Path,
			&version.Kind,
			&version.Version,
			&version.ChangeType,
			&version.Content,
			&version.ContentType,
			&version.Metadata,
			&version.Checksum,
			&version.MinTrustLevel,
			&version.CreatedAt,
		); err != nil {
			return nil, cursor, fmt.Errorf("filetree.Changes: scan: %w", err)
		}
		nextCursor = version.Cursor

		entry := models.FileTreeEntry{
			ID:            version.EntryID,
			UserID:        version.UserID,
			Path:          version.Path,
			Kind:          version.Kind,
			IsDirectory:   IsDirectoryLikeKind(version.Kind) || version.ContentType == "directory",
			Content:       version.Content,
			ContentType:   version.ContentType,
			Metadata:      version.Metadata,
			Checksum:      version.Checksum,
			Version:       version.Version,
			MinTrustLevel: version.MinTrustLevel,
			CreatedAt:     version.CreatedAt,
			UpdatedAt:     version.CreatedAt,
		}
		if version.ChangeType == "delete" {
			deletedAt := version.CreatedAt
			entry.DeletedAt = &deletedAt
		}

		changes = append(changes, models.EntryChange{
			Cursor:     version.Cursor,
			ChangeType: version.ChangeType,
			Entry:      entry,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, cursor, fmt.Errorf("filetree.Changes: rows: %w", err)
	}
	return changes, nextCursor, nil
}

func (s *FileTreeService) ListSkillSummaries(ctx context.Context, userID uuid.UUID, trustLevel int) ([]models.SkillSummary, error) {
	if s.repo != nil {
		return s.repo.ListSkillSummaries(ctx, userID, trustLevel)
	}
	summaries := append([]models.SkillSummary{}, systemskills.SkillSummaries()...)
	if s.db == nil {
		sort.Slice(summaries, func(i, j int) bool {
			if summaries[i].Source == summaries[j].Source {
				return summaries[i].Name < summaries[j].Name
			}
			return summaries[i].Source < summaries[j].Source
		})
		return summaries, nil
	}

	roots := []struct {
		path   string
		source string
	}{
		{path: "/skills", source: "skills"},
	}

	for _, root := range roots {
		snapshot, err := s.Snapshot(ctx, userID, root.path, trustLevel)
		if err != nil {
			if errors.Is(err, ErrEntryNotFound) {
				continue
			}
			return nil, err
		}
		for _, entry := range snapshot.Entries {
			if entry.IsDirectory || !strings.HasSuffix(hubpath.NormalizePublic(entry.Path), "/SKILL.md") {
				continue
			}
			summaries = append(summaries, entryToSkillSummary(entry, root.source))
		}
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Source == summaries[j].Source {
			return summaries[i].Name < summaries[j].Name
		}
		return summaries[i].Source < summaries[j].Source
	})
	return summaries, nil
}

func (s *FileTreeService) latestCursor(ctx context.Context, userID uuid.UUID, pathPrefix string) (int64, error) {
	var cursor int64
	err := s.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT COALESCE(MAX(cursor), 0)
		 FROM entry_versions
		 WHERE user_id = $1
		   AND %s`, s.entryVersionPrefixCondition(pathPrefix, 2)),
		append([]interface{}{userID}, s.prefixArgs(pathPrefix)...)...,
	).Scan(&cursor)
	if err != nil {
		return 0, fmt.Errorf("filetree.latestCursor: %w", err)
	}
	return cursor, nil
}

func (s *FileTreeService) lockEntry(ctx context.Context, tx pgx.Tx, userID uuid.UUID, path string) (*models.FileTreeEntry, error) {
	var entry models.FileTreeEntry
	err := tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s
		 FROM file_tree
		 WHERE user_id = $1
		   AND path = $2
		 LIMIT 1
		 FOR UPDATE`, fileTreeSelectColumns),
		userID, path,
	).Scan(
		&entry.ID,
		&entry.UserID,
		&entry.Path,
		&entry.Kind,
		&entry.IsDirectory,
		&entry.Content,
		&entry.ContentType,
		&entry.Metadata,
		&entry.Checksum,
		&entry.Version,
		&entry.MinTrustLevel,
		&entry.CreatedAt,
		&entry.UpdatedAt,
		&entry.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEntryNotFound
		}
		return nil, fmt.Errorf("filetree.lockEntry: %w", err)
	}
	return &entry, nil
}

func (s *FileTreeService) insertEntryVersion(ctx context.Context, tx pgx.Tx, entry *models.FileTreeEntry, changeType string) error {
	content := entry.Content
	if changeType == "delete" {
		content = ""
	}

	_, err := tx.Exec(ctx,
		`INSERT INTO entry_versions (
			id, entry_id, user_id, path, kind, version, change_type,
			content, content_type, metadata, checksum, min_trust_level, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13
		)`,
		uuid.New(), entry.ID, entry.UserID, entry.Path, entry.Kind, entry.Version,
		changeType, content, entry.ContentType, entry.Metadata, entry.Checksum,
		entry.MinTrustLevel, entry.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("filetree.insertEntryVersion: %w", err)
	}
	return nil
}

func (s *FileTreeService) prefixCondition(pathPrefix string, argStart int) string {
	if pathPrefix == "" || pathPrefix == "/" {
		return "TRUE"
	}

	return fmt.Sprintf("(path = $%d OR path LIKE $%d)", argStart, argStart+1)
}

func (s *FileTreeService) entryVersionPrefixCondition(pathPrefix string, argStart int) string {
	if pathPrefix == "" || pathPrefix == "/" {
		return "TRUE"
	}

	return fmt.Sprintf("(path = $%d OR path LIKE $%d)", argStart, argStart+1)
}

func (s *FileTreeService) prefixArgs(pathPrefix string) []interface{} {
	if pathPrefix == "" || pathPrefix == "/" {
		return nil
	}
	storage := hubpath.NormalizeStorage(pathPrefix)
	return []interface{}{storage, prefixMatchValue(storage)}
}

func prefixMatchValue(pathPrefix string) string {
	if pathPrefix == "" || pathPrefix == "/" {
		return "/%"
	}
	if strings.HasSuffix(pathPrefix, "/") {
		return pathPrefix + "%"
	}
	return pathPrefix + "/%"
}

func scanFileTreeEntries(rows pgx.Rows) ([]models.FileTreeEntry, error) {
	entries := make([]models.FileTreeEntry, 0, 16)
	for rows.Next() {
		var entry models.FileTreeEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.UserID,
			&entry.Path,
			&entry.Kind,
			&entry.IsDirectory,
			&entry.Content,
			&entry.ContentType,
			&entry.Metadata,
			&entry.Checksum,
			&entry.Version,
			&entry.MinTrustLevel,
			&entry.CreatedAt,
			&entry.UpdatedAt,
			&entry.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("filetree.scanEntries: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func immediateChildEntries(parentPath string, userID uuid.UUID, entries []models.FileTreeEntry) []models.FileTreeEntry {
	publicParentPath := hubpath.NormalizePublic(parentPath)
	if !strings.HasSuffix(publicParentPath, "/") {
		publicParentPath += "/"
	}

	children := make(map[string]models.FileTreeEntry)
	descendantsByChild := make(map[string][]models.FileTreeEntry)
	for _, entry := range entries {
		publicEntryPath := hubpath.NormalizePublic(entry.Path)
		if publicEntryPath == publicParentPath || !strings.HasPrefix(publicEntryPath, publicParentPath) {
			continue
		}

		rest := strings.TrimPrefix(publicEntryPath, publicParentPath)
		trimmedRest := strings.TrimSuffix(rest, "/")
		if trimmedRest == "" {
			continue
		}

		segments := strings.Split(trimmedRest, "/")
		childName := segments[0]
		childPath := pathpkg.Join(strings.TrimSuffix(publicParentPath, "/"), childName)
		isDirectChild := len(segments) == 1

		if entry.IsDirectory || !isDirectChild {
			childPath += "/"
		}
		childPath = hubpath.NormalizeStorage(childPath)
		descendantsByChild[childPath] = append(descendantsByChild[childPath], entry)

		if _, ok := children[childPath]; ok {
			continue
		}

		if !isDirectChild {
			now := entry.UpdatedAt
			children[childPath] = models.FileTreeEntry{
				ID:            uuid.Nil,
				UserID:        userID,
				Path:          childPath,
				Kind:          "directory",
				IsDirectory:   true,
				ContentType:   "directory",
				Metadata:      map[string]interface{}{},
				Checksum:      entryChecksum(hubpath.NormalizePublic(childPath), "", "directory", map[string]interface{}{}),
				Version:       1,
				MinTrustLevel: models.TrustLevelGuest,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
			continue
		}

		entry.Path = childPath
		children[childPath] = entry
	}

	out := make([]models.FileTreeEntry, 0, len(children))
	for childPath, entry := range children {
		if entry.IsDirectory {
			entry = EnrichBundleDirectoryEntry(entry, descendantsByChild[childPath])
		}
		out = append(out, entry)
	}
	return mergeFileTreeEntries(out)
}

func (s *FileTreeService) resolveDeletePath(ctx context.Context, userID uuid.UUID, rawPath string) (string, error) {
	storagePath := hubpath.NormalizeStorage(rawPath)
	if systemskills.IsProtectedPath(storagePath) {
		return "", ErrReadOnlyPath
	}
	if strings.HasSuffix(storagePath, "/") {
		return storagePath, nil
	}

	entry, err := s.Read(ctx, userID, storagePath, models.TrustLevelFull)
	if err == nil {
		return entry.Path, nil
	}
	if !errors.Is(err, ErrEntryNotFound) {
		return "", err
	}

	dirPath := storagePath + "/"
	entry, err = s.Read(ctx, userID, dirPath, models.TrustLevelFull)
	if err == nil {
		return entry.Path, nil
	}
	if err != nil && !errors.Is(err, ErrEntryNotFound) {
		return "", err
	}

	return storagePath, nil
}

func deletableEntriesForDeletion(targetPath string, entries []models.FileTreeEntry) []models.FileTreeEntry {
	targetPath = hubpath.NormalizeStorage(targetPath)
	targetBase := strings.TrimSuffix(targetPath, "/")
	prefix := targetBase + "/"

	deletable := make([]models.FileTreeEntry, 0, len(entries))
	for _, entry := range entries {
		entryPath := hubpath.NormalizeStorage(entry.Path)
		if systemskills.IsProtectedPath(entryPath) {
			continue
		}
		if entryPath == targetPath || entryPath == targetBase || entryPath == prefix || strings.HasPrefix(entryPath, prefix) {
			deletable = append(deletable, entry)
		}
	}

	sort.Slice(deletable, func(i, j int) bool {
		left := hubpath.NormalizeStorage(deletable[i].Path)
		right := hubpath.NormalizeStorage(deletable[j].Path)
		leftDepth := strings.Count(strings.Trim(left, "/"), "/")
		rightDepth := strings.Count(strings.Trim(right, "/"), "/")
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		if len(left) != len(right) {
			return len(left) > len(right)
		}
		return left > right
	})

	return deletable
}

func entryToSkillSummary(entry models.FileTreeEntry, source string) models.SkillSummary {
	resolvedSource := EntrySource(&entry)
	if resolvedSource == "" {
		resolvedSource = NormalizeSource(source)
	}
	summary := models.SkillSummary{
		Name:          hubpath.BaseName(pathpkg.Dir(hubpath.NormalizePublic(entry.Path))),
		Path:          hubpath.StorageToPublic(entry.Path),
		BundlePath:    hubpath.StorageToPublic(pathpkg.Dir(entry.Path)),
		PrimaryPath:   hubpath.StorageToPublic(entry.Path),
		Source:        resolvedSource,
		Description:   firstMarkdownParagraph(entry.Content),
		Capabilities:  []string{"instructions"},
		MinTrustLevel: entry.MinTrustLevel,
	}

	if value, ok := entry.Metadata["name"].(string); ok && strings.TrimSpace(value) != "" {
		summary.Name = value
	}
	if value, ok := entry.Metadata["description"].(string); ok && strings.TrimSpace(value) != "" {
		summary.Description = value
	}
	if value, ok := entry.Metadata["when_to_use"].(string); ok {
		summary.WhenToUse = value
	}
	if value, ok := entry.Metadata["read_only"].(bool); ok {
		summary.ReadOnly = value
	}
	if value := toStringSlice(entry.Metadata["allowed_tools"]); len(value) > 0 {
		summary.AllowedTools = value
	}
	if value := toStringSlice(entry.Metadata["tags"]); len(value) > 0 {
		summary.Tags = value
	}
	if value := toMap(entry.Metadata["arguments"]); len(value) > 0 {
		summary.Arguments = value
	}
	if value := toMap(entry.Metadata["activation"]); len(value) > 0 {
		summary.Activation = value
	}
	if value, ok := entry.Metadata["min_trust_level"].(int); ok && value > 0 {
		summary.MinTrustLevel = value
	}
	if value := toStringSlice(entry.Metadata["bundle_capabilities"]); len(value) > 0 {
		summary.Capabilities = uniqueSortedStrings(value)
	}
	return summary
}

func mergeFileTreeEntries(groups ...[]models.FileTreeEntry) []models.FileTreeEntry {
	merged := make(map[string]models.FileTreeEntry)
	for _, group := range groups {
		for _, entry := range group {
			merged[hubpath.NormalizePublic(entry.Path)] = entry
		}
	}

	entries := make([]models.FileTreeEntry, 0, len(merged))
	for _, entry := range merged {
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDirectory == entries[j].IsDirectory {
			return hubpath.NormalizePublic(entries[i].Path) < hubpath.NormalizePublic(entries[j].Path)
		}
		return entries[i].IsDirectory && !entries[j].IsDirectory
	})

	return entries
}
