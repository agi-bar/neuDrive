package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"path"
	pathpkg "path"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
	"github.com/agi-bar/neudrive/internal/systemskills"
	"github.com/google/uuid"
)

func (s *Store) Read(ctx context.Context, userID uuid.UUID, rawPath string, trustLevel int) (*models.FileTreeEntry, error) {
	storagePath := hubpath.NormalizeStorage(rawPath)
	if entry, ok, err := systemskills.ReadEntry(storagePath); err != nil {
		return nil, err
	} else if ok {
		return entry, nil
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, path, kind, is_directory, content, content_type, metadata_json,
		        checksum, version, min_trust_level, created_at, updated_at, deleted_at
		   FROM file_tree
		  WHERE user_id = ? AND path = ? AND deleted_at IS NULL`,
		userID.String(),
		storagePath,
	)
	entry, err := scanFileTreeEntry(row)
	if err != nil {
		if errors.Is(err, services.ErrEntryNotFound) {
			return nil, services.ErrEntryNotFound
		}
		return nil, err
	}
	if entry.MinTrustLevel > trustLevel {
		return nil, fmt.Errorf("insufficient trust level")
	}
	if entry.IsDirectory {
		if snapshot, snapshotErr := s.Snapshot(ctx, userID, storagePath, trustLevel); snapshotErr == nil {
			enriched := services.EnrichBundleDirectoryEntry(*entry, snapshot.Entries)
			entry = &enriched
		}
	}
	return entry, nil
}

func (s *Store) List(ctx context.Context, userID uuid.UUID, rawPath string, trustLevel int) ([]models.FileTreeEntry, error) {
	storagePath := hubpath.NormalizeStorage(rawPath)
	if !strings.HasSuffix(storagePath, "/") {
		storagePath += "/"
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, path, kind, is_directory, content, content_type, metadata_json,
		        checksum, version, min_trust_level, created_at, updated_at, deleted_at
		   FROM file_tree
		  WHERE user_id = ? AND deleted_at IS NULL AND path LIKE ? AND min_trust_level <= ?
		  ORDER BY path ASC`,
		userID.String(),
		storagePath+"%",
		trustLevel,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]models.FileTreeEntry{}
	descendantsByChild := map[string][]models.FileTreeEntry{}
	for rows.Next() {
		entry, err := scanFileTreeEntry(rows)
		if err != nil {
			return nil, err
		}
		if entry.Path == storagePath {
			continue
		}
		rest := strings.TrimPrefix(entry.Path, storagePath)
		if rest == "" {
			continue
		}
		name := strings.Split(rest, "/")[0]
		childPath := pathpkg.Join(strings.TrimSuffix(storagePath, "/"), name)
		if entry.IsDirectory || strings.Contains(rest, "/") {
			childPath += "/"
		}
		childPath = hubpath.NormalizeStorage(childPath)
		descendantsByChild[childPath] = append(descendantsByChild[childPath], *entry)
		if _, ok := seen[childPath]; ok {
			continue
		}
		if strings.Contains(rest, "/") {
			now := entry.UpdatedAt
			seen[childPath] = models.FileTreeEntry{
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
		seen[childPath] = *entry
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]models.FileTreeEntry, 0, len(seen))
	for childPath, entry := range seen {
		if entry.IsDirectory {
			entry = services.EnrichBundleDirectoryEntry(entry, descendantsByChild[childPath])
		}
		out = append(out, entry)
	}
	if systemEntries, handled := systemskills.ListEntries(storagePath); handled {
		if systemskills.IsProtectedPath(storagePath) {
			out = mergeEntries(out, systemEntries)
		} else {
			out = mergeEntries(systemEntries, out)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDirectory == out[j].IsDirectory {
			return out[i].Path < out[j].Path
		}
		return out[i].IsDirectory && !out[j].IsDirectory
	})
	return out, nil
}

func (s *Store) Snapshot(ctx context.Context, userID uuid.UUID, rawPath string, trustLevel int) (*models.EntrySnapshot, error) {
	storagePath := hubpath.NormalizeStorage(rawPath)
	like := storagePath
	if storagePath != "/" && !strings.HasSuffix(like, "/") {
		like += "/"
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, path, kind, is_directory, content, content_type, metadata_json,
		        checksum, version, min_trust_level, created_at, updated_at, deleted_at
		   FROM file_tree
		  WHERE user_id = ? AND deleted_at IS NULL AND min_trust_level <= ?
		    AND (path = ? OR path LIKE ?)
		  ORDER BY path ASC`,
		userID.String(),
		trustLevel,
		storagePath,
		like+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]models.FileTreeEntry, 0, 32)
	for rows.Next() {
		entry, err := scanFileTreeEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, *entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		if _, ok, err := systemskills.ReadEntry(storagePath); err == nil && ok {
			entry, _, _ := systemskills.ReadEntry(storagePath)
			entries = append(entries, *entry)
		} else if !handledSystemPrefix(storagePath) {
			return nil, services.ErrEntryNotFound
		}
	}
	if systemEntries, handled := systemskills.ListEntries(storagePath); handled {
		entries = mergeEntries(entries, systemEntries)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return &models.EntrySnapshot{
		Path:         hubpath.NormalizePublic(storagePath),
		Cursor:       snapshotCursor(entries),
		RootChecksum: rootChecksum(entries),
		Entries:      entries,
	}, nil
}

func (s *Store) Search(ctx context.Context, userID uuid.UUID, query string, trustLevel int, rawPrefix string) ([]models.FileTreeEntry, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	prefix := hubpath.NormalizeStorage(rawPrefix)
	if prefix == "" {
		prefix = "/"
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, path, kind, is_directory, content, content_type, metadata_json,
		        checksum, version, min_trust_level, created_at, updated_at, deleted_at
		   FROM file_tree
		  WHERE user_id = ? AND deleted_at IS NULL AND min_trust_level <= ?
		    AND path LIKE ? AND (content LIKE ? OR path LIKE ?)
		  ORDER BY updated_at DESC, path ASC`,
		userID.String(),
		trustLevel,
		prefixLike(prefix),
		"%"+query+"%",
		"%"+query+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]models.FileTreeEntry, 0, 16)
	for rows.Next() {
		entry, err := scanFileTreeEntry(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *entry)
	}
	return results, rows.Err()
}

func (s *Store) WriteEntry(ctx context.Context, userID uuid.UUID, rawPath, content, contentType string, opts models.FileTreeWriteOptions) (*models.FileTreeEntry, error) {
	storagePath := hubpath.NormalizeStorage(rawPath)
	if systemskills.IsProtectedPath(storagePath) {
		return nil, services.ErrReadOnlyPath
	}
	if contentType == "" {
		contentType = "text/plain"
	}
	if err := s.ensureParentDirectories(ctx, userID, storagePath); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	current, _ := s.readCurrentEntryTx(ctx, tx, userID, storagePath)
	metadata := mergeMetadata(nil, opts.Metadata)
	kind := strings.TrimSpace(opts.Kind)
	if kind == "" {
		kind = classifyEntryKind(storagePath, false)
	}
	if current == nil {
		metadata = services.WithSourceContextMetadata(metadata, ctx)
		metadata = services.SkillMetadataForPath(storagePath, content, metadata)
		if err := s.enforceStorageQuotaTx(ctx, tx, userID, nil, int64(len(content))); err != nil {
			return nil, err
		}
		entry := &models.FileTreeEntry{
			ID:            uuid.New(),
			UserID:        userID,
			Path:          storagePath,
			Kind:          kind,
			IsDirectory:   false,
			Content:       content,
			ContentType:   contentType,
			Metadata:      metadata,
			Checksum:      entryChecksum(hubpath.NormalizePublic(storagePath), content, contentType, metadata),
			Version:       1,
			MinTrustLevel: maxTrust(opts.MinTrustLevel, models.TrustLevelGuest),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO file_tree (
				id, user_id, path, kind, is_directory, content, content_type, metadata_json,
				checksum, version, min_trust_level, created_at, updated_at, deleted_at
			) VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
			entry.ID.String(),
			userID.String(),
			entry.Path,
			entry.Kind,
			entry.Content,
			entry.ContentType,
			encodeJSON(entry.Metadata),
			entry.Checksum,
			entry.Version,
			entry.MinTrustLevel,
			timeText(entry.CreatedAt),
			timeText(entry.UpdatedAt),
		); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return entry, nil
	}
	if opts.ExpectedVersion != nil && current.Version != *opts.ExpectedVersion {
		return nil, services.ErrOptimisticLockConflict
	}
	if opts.ExpectedChecksum != "" && current.Checksum != opts.ExpectedChecksum {
		return nil, services.ErrOptimisticLockConflict
	}
	metadata = mergeMetadata(current.Metadata, opts.Metadata)
	metadata = services.WithSourceContextMetadata(metadata, ctx)
	metadata = services.SkillMetadataForPath(storagePath, content, metadata)
	if err := s.enforceStorageQuotaTx(ctx, tx, userID, current, int64(len(content))); err != nil {
		return nil, err
	}
	current.Kind = kind
	current.IsDirectory = false
	current.Content = content
	current.ContentType = contentType
	current.Metadata = metadata
	current.Version++
	current.MinTrustLevel = maxTrust(opts.MinTrustLevel, current.MinTrustLevel)
	current.UpdatedAt = now
	current.DeletedAt = nil
	current.Checksum = entryChecksum(hubpath.NormalizePublic(storagePath), content, contentType, metadata)
	if _, err := tx.ExecContext(ctx,
		`UPDATE file_tree
		    SET kind = ?, is_directory = 0, content = ?, content_type = ?, metadata_json = ?,
		        checksum = ?, version = ?, min_trust_level = ?, updated_at = ?, deleted_at = NULL
		  WHERE id = ?`,
		current.Kind,
		current.Content,
		current.ContentType,
		encodeJSON(current.Metadata),
		current.Checksum,
		current.Version,
		current.MinTrustLevel,
		timeText(current.UpdatedAt),
		current.ID.String(),
	); err != nil {
		return nil, err
	}
	_, _ = tx.ExecContext(ctx, `DELETE FROM file_blobs WHERE entry_id = ?`, current.ID.String())
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return current, nil
}

func (s *Store) WriteBinaryEntry(ctx context.Context, userID uuid.UUID, rawPath string, data []byte, contentType string, opts models.FileTreeWriteOptions) (*models.FileTreeEntry, error) {
	storagePath := hubpath.NormalizeStorage(rawPath)
	if systemskills.IsProtectedPath(storagePath) {
		return nil, services.ErrReadOnlyPath
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := s.ensureParentDirectories(ctx, userID, storagePath); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	current, _ := s.readCurrentEntryTx(ctx, tx, userID, storagePath)
	metadata := mergeMetadata(nil, opts.Metadata)
	kind := strings.TrimSpace(opts.Kind)
	if kind == "" {
		kind = classifyEntryKind(storagePath, false)
	}
	if current == nil {
		metadata = services.WithSourceContextMetadata(metadata, ctx)
		metadata = mergeMetadata(metadata, binaryMetadata(data))
		if err := s.enforceStorageQuotaTx(ctx, tx, userID, nil, int64(len(data))); err != nil {
			return nil, err
		}
		current = &models.FileTreeEntry{
			ID:            uuid.New(),
			UserID:        userID,
			Path:          storagePath,
			Kind:          kind,
			IsDirectory:   false,
			Content:       "",
			ContentType:   contentType,
			Metadata:      metadata,
			Checksum:      entryChecksum(hubpath.NormalizePublic(storagePath), "", contentType, metadata),
			Version:       1,
			MinTrustLevel: maxTrust(opts.MinTrustLevel, models.TrustLevelGuest),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO file_tree (
				id, user_id, path, kind, is_directory, content, content_type, metadata_json,
				checksum, version, min_trust_level, created_at, updated_at, deleted_at
			) VALUES (?, ?, ?, ?, 0, '', ?, ?, ?, ?, ?, ?, ?, NULL)`,
			current.ID.String(),
			userID.String(),
			current.Path,
			current.Kind,
			current.ContentType,
			encodeJSON(current.Metadata),
			current.Checksum,
			current.Version,
			current.MinTrustLevel,
			timeText(current.CreatedAt),
			timeText(current.UpdatedAt),
		)
		if err != nil {
			return nil, err
		}
	} else {
		if opts.ExpectedVersion != nil && current.Version != *opts.ExpectedVersion {
			return nil, services.ErrOptimisticLockConflict
		}
		if opts.ExpectedChecksum != "" && current.Checksum != opts.ExpectedChecksum {
			return nil, services.ErrOptimisticLockConflict
		}
		metadata = mergeMetadata(current.Metadata, opts.Metadata)
		metadata = services.WithSourceContextMetadata(metadata, ctx)
		metadata = mergeMetadata(metadata, binaryMetadata(data))
		if err := s.enforceStorageQuotaTx(ctx, tx, userID, current, int64(len(data))); err != nil {
			return nil, err
		}
		current.Kind = kind
		current.Content = ""
		current.ContentType = contentType
		current.Metadata = metadata
		current.Version++
		current.MinTrustLevel = maxTrust(opts.MinTrustLevel, current.MinTrustLevel)
		current.UpdatedAt = now
		current.DeletedAt = nil
		current.Checksum = entryChecksum(hubpath.NormalizePublic(storagePath), "", contentType, metadata)
		_, err := tx.ExecContext(ctx,
			`UPDATE file_tree
			    SET kind = ?, is_directory = 0, content = '', content_type = ?, metadata_json = ?,
			        checksum = ?, version = ?, min_trust_level = ?, updated_at = ?, deleted_at = NULL
			  WHERE id = ?`,
			current.Kind,
			current.ContentType,
			encodeJSON(current.Metadata),
			current.Checksum,
			current.Version,
			current.MinTrustLevel,
			timeText(current.UpdatedAt),
			current.ID.String(),
		)
		if err != nil {
			return nil, err
		}
	}
	hash := sha256.Sum256(data)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO file_blobs (entry_id, user_id, data, size_bytes, sha256, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(entry_id) DO UPDATE SET
		   data = excluded.data,
		   size_bytes = excluded.size_bytes,
		   sha256 = excluded.sha256,
		   updated_at = excluded.updated_at`,
		current.ID.String(),
		userID.String(),
		data,
		len(data),
		hex.EncodeToString(hash[:]),
		timeText(now),
		timeText(now),
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return current, nil
}

func (s *Store) Delete(ctx context.Context, userID uuid.UUID, rawPath string) error {
	storagePath, err := s.resolveDeletePath(ctx, userID, rawPath)
	if err != nil {
		return err
	}
	if systemskills.IsProtectedPath(storagePath) {
		return services.ErrReadOnlyPath
	}

	snapshot, err := s.Snapshot(ctx, userID, storagePath, models.TrustLevelFull)
	if err != nil {
		return err
	}
	if len(snapshot.Entries) == 0 {
		if _, handled := systemskills.ListEntries(storagePath); handled {
			return nil
		}
		return services.ErrEntryNotFound
	}

	entriesToDelete := deletableEntriesForDeletion(storagePath, snapshot.Entries)
	if len(entriesToDelete) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, entry := range entriesToDelete {
		if _, err := tx.ExecContext(ctx, `DELETE FROM file_blobs WHERE entry_id = ?`, entry.ID.String()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM file_tree WHERE user_id = ? AND path = ?`, userID.String(), entry.Path); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) ReadBinary(ctx context.Context, userID uuid.UUID, rawPath string, trustLevel int) ([]byte, *models.FileTreeEntry, error) {
	entry, err := s.Read(ctx, userID, rawPath, trustLevel)
	if err != nil {
		return nil, nil, err
	}
	data, ok, err := s.ReadBlobByEntryID(ctx, entry.ID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, entry, fmt.Errorf("blob not found")
	}
	return data, entry, nil
}

func (s *Store) ReadBlobByEntryID(ctx context.Context, entryID uuid.UUID) ([]byte, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT data FROM file_blobs WHERE entry_id = ?`, entryID.String())
	var data []byte
	if err := row.Scan(&data); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no rows") {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

func (s *Store) ListSkillSummaries(ctx context.Context, userID uuid.UUID, trustLevel int) ([]models.SkillSummary, error) {
	summaries := append([]models.SkillSummary{}, systemskills.SkillSummaries()...)
	for _, root := range []string{"/skills"} {
		snapshot, err := s.Snapshot(ctx, userID, root, trustLevel)
		if err != nil {
			if errors.Is(err, services.ErrEntryNotFound) {
				continue
			}
			return nil, err
		}
		for _, entry := range snapshot.Entries {
			if entry.IsDirectory || !strings.HasSuffix(hubpath.NormalizePublic(entry.Path), "/SKILL.md") {
				continue
			}
			resolved := services.SkillMetadataForPath(entry.Path, entry.Content, entry.Metadata)
			name := path.Base(path.Dir(hubpath.NormalizePublic(entry.Path)))
			if value, ok := resolved["name"].(string); ok && strings.TrimSpace(value) != "" {
				name = value
			}
			description, _ := resolved["description"].(string)
			whenToUse, _ := resolved["when_to_use"].(string)
			readOnly, _ := resolved["read_only"].(bool)
			capabilities := stringSliceValue(resolved["bundle_capabilities"])
			if len(capabilities) == 0 {
				capabilities = []string{"instructions"}
			}
			summaries = append(summaries, models.SkillSummary{
				Name:          name,
				Path:          hubpath.NormalizePublic(entry.Path),
				BundlePath:    path.Dir(hubpath.NormalizePublic(entry.Path)),
				PrimaryPath:   hubpath.NormalizePublic(entry.Path),
				Source:        services.EntrySourceFromMetadata(resolved),
				ReadOnly:      readOnly,
				Description:   description,
				WhenToUse:     whenToUse,
				Capabilities:  capabilities,
				AllowedTools:  stringSliceValue(resolved["allowed_tools"]),
				Tags:          stringSliceValue(resolved["tags"]),
				MinTrustLevel: entry.MinTrustLevel,
			})
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

func (s *Store) ensureParentDirectories(ctx context.Context, userID uuid.UUID, storagePath string) error {
	parents := []string{}
	for dir := pathpkg.Dir(strings.TrimSuffix(storagePath, "/")); dir != "." && dir != "/" && dir != ""; dir = pathpkg.Dir(dir) {
		parents = append(parents, dir)
	}
	for i := len(parents) - 1; i >= 0; i-- {
		if err := s.EnsureDirectory(ctx, userID, parents[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) EnsureDirectory(ctx context.Context, userID uuid.UUID, rawPath string) error {
	return s.EnsureDirectoryWithMetadata(ctx, userID, rawPath, nil, models.TrustLevelGuest)
}

func (s *Store) EnsureDirectoryWithMetadata(ctx context.Context, userID uuid.UUID, rawPath string, metadata map[string]interface{}, minTrustLevel int) error {
	storagePath := hubpath.NormalizeStorage(rawPath)
	if storagePath == "/" || storagePath == "" {
		return nil
	}
	if err := s.ensureParentDirectories(ctx, userID, storagePath); err != nil {
		return err
	}
	now := time.Now().UTC()
	minTrust := minTrustLevel
	if minTrust <= 0 {
		minTrust = models.TrustLevelGuest
	}
	meta := mergeMetadata(nil, metadata)
	meta = services.WithSourceContextMetadata(meta, ctx)

	current, err := s.readCurrentEntry(ctx, userID, storagePath)
	if err == nil && current != nil {
		mergedMeta := mergeMetadata(current.Metadata, meta)
		checksum := entryChecksum(hubpath.NormalizePublic(storagePath), "", "directory", mergedMeta)
		dirKind := services.BundleEntryKindForMetadata(mergedMeta)
		nextTrust := current.MinTrustLevel
		if nextTrust <= 0 || minTrust < nextTrust {
			nextTrust = minTrust
		}
		_, err = s.db.ExecContext(ctx,
			`UPDATE file_tree
			    SET kind = ?, is_directory = 1, content = '', content_type = 'directory', metadata_json = ?,
			        checksum = ?, min_trust_level = ?, updated_at = ?, deleted_at = NULL
			  WHERE user_id = ? AND path = ?`,
			dirKind,
			encodeJSON(mergedMeta),
			checksum,
			nextTrust,
			timeText(now),
			userID.String(),
			storagePath,
		)
		return err
	}
	if err != nil && !errors.Is(err, services.ErrEntryNotFound) {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO file_tree (
			id, user_id, path, kind, is_directory, content, content_type, metadata_json,
			checksum, version, min_trust_level, created_at, updated_at, deleted_at
		) VALUES (?, ?, ?, ?, 1, '', 'directory', ?, ?, 1, ?, ?, ?, NULL)`,
		uuid.New().String(),
		userID.String(),
		storagePath,
		services.BundleEntryKindForMetadata(meta),
		encodeJSON(meta),
		entryChecksum(hubpath.NormalizePublic(storagePath), "", "directory", meta),
		minTrust,
		timeText(now),
		timeText(now),
	)
	return err
}

func (s *Store) readCurrentEntry(ctx context.Context, userID uuid.UUID, storagePath string) (*models.FileTreeEntry, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, path, kind, is_directory, content, content_type, metadata_json,
		        checksum, version, min_trust_level, created_at, updated_at, deleted_at
		   FROM file_tree
		  WHERE user_id = ? AND path = ? AND deleted_at IS NULL`,
		userID.String(),
		storagePath,
	)
	return scanFileTreeEntry(row)
}

func (s *Store) readCurrentEntryTx(ctx context.Context, tx *sql.Tx, userID uuid.UUID, storagePath string) (*models.FileTreeEntry, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT id, user_id, path, kind, is_directory, content, content_type, metadata_json,
		        checksum, version, min_trust_level, created_at, updated_at, deleted_at
		   FROM file_tree
		  WHERE user_id = ? AND path = ? AND deleted_at IS NULL`,
		userID.String(),
		storagePath,
	)
	return scanFileTreeEntry(row)
}

func (s *Store) resolveDeletePath(ctx context.Context, userID uuid.UUID, rawPath string) (string, error) {
	storagePath := hubpath.NormalizeStorage(rawPath)
	if systemskills.IsProtectedPath(storagePath) {
		return "", services.ErrReadOnlyPath
	}
	if strings.HasSuffix(storagePath, "/") {
		return storagePath, nil
	}

	entry, err := s.Read(ctx, userID, storagePath, models.TrustLevelFull)
	if err == nil {
		return entry.Path, nil
	}
	if err != nil && !errors.Is(err, services.ErrEntryNotFound) {
		return "", err
	}

	dirPath := storagePath + "/"
	entry, err = s.Read(ctx, userID, dirPath, models.TrustLevelFull)
	if err == nil {
		return entry.Path, nil
	}
	if err != nil && !errors.Is(err, services.ErrEntryNotFound) {
		return "", err
	}

	return storagePath, nil
}

type fileTreeScanner interface {
	Scan(dest ...any) error
}

func scanFileTreeEntry(row fileTreeScanner) (*models.FileTreeEntry, error) {
	var (
		id            string
		userID        string
		pathValue     string
		kind          string
		isDirectory   bool
		content       string
		contentType   string
		metadataJSON  string
		checksum      string
		version       int64
		minTrustLevel int
		createdAt     string
		updatedAt     string
		deletedAt     *string
	)
	if err := row.Scan(&id, &userID, &pathValue, &kind, &isDirectory, &content, &contentType, &metadataJSON, &checksum, &version, &minTrustLevel, &createdAt, &updatedAt, &deletedAt); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no rows") {
			return nil, services.ErrEntryNotFound
		}
		return nil, err
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	entry := &models.FileTreeEntry{
		ID:            parsedID,
		UserID:        parsedUserID,
		Path:          pathValue,
		Kind:          kind,
		IsDirectory:   isDirectory,
		Content:       content,
		ContentType:   contentType,
		Metadata:      decodeJSONMap(metadataJSON),
		Checksum:      checksum,
		Version:       version,
		MinTrustLevel: minTrustLevel,
		CreatedAt:     mustParseTime(createdAt),
		UpdatedAt:     mustParseTime(updatedAt),
	}
	if deletedAt != nil {
		ts := mustParseTime(*deletedAt)
		entry.DeletedAt = &ts
	}
	return entry, nil
}

func prefixLike(storagePath string) string {
	storagePath = hubpath.NormalizeStorage(storagePath)
	if storagePath == "/" {
		return "/%"
	}
	if strings.HasSuffix(storagePath, "/") {
		return storagePath + "%"
	}
	return storagePath + "/%"
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

func handledSystemPrefix(storagePath string) bool {
	_, handled := systemskills.ListEntries(storagePath)
	return handled
}

func mergeEntries(primary, secondary []models.FileTreeEntry) []models.FileTreeEntry {
	seen := map[string]struct{}{}
	out := make([]models.FileTreeEntry, 0, len(primary)+len(secondary))
	for _, set := range [][]models.FileTreeEntry{primary, secondary} {
		for _, entry := range set {
			key := hubpath.NormalizePublic(entry.Path)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, entry)
		}
	}
	return out
}

func snapshotCursor(entries []models.FileTreeEntry) int64 {
	var maxCursor int64
	for _, entry := range entries {
		if entry.UpdatedAt.UnixNano() > maxCursor {
			maxCursor = entry.UpdatedAt.UnixNano()
		}
	}
	return maxCursor
}

func rootChecksum(entries []models.FileTreeEntry) string {
	if len(entries) == 0 {
		return entryChecksum("/", "", "directory", map[string]interface{}{})
	}
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, entry.Path+":"+entry.Checksum)
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func entryChecksum(pathValue, content, contentType string, metadata map[string]interface{}) string {
	payload, _ := json.Marshal(map[string]interface{}{
		"path":         pathValue,
		"content":      content,
		"content_type": contentType,
		"metadata":     metadata,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func mergeMetadata(base, overlay map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func classifyEntryKind(rawPath string, isDirectory bool) string {
	if isDirectory {
		return "directory"
	}
	publicPath := hubpath.NormalizePublic(rawPath)
	switch {
	case strings.HasPrefix(publicPath, "/memory/profile/"):
		return "memory_profile"
	case strings.HasPrefix(publicPath, "/memory/scratch/"):
		return "memory_scratch"
	case strings.HasPrefix(publicPath, "/projects/") && strings.HasSuffix(publicPath, "/context.md"):
		return "project_context"
	case strings.HasPrefix(publicPath, "/projects/") && strings.HasSuffix(publicPath, "/log.jsonl"):
		return "project_log"
	case strings.HasSuffix(publicPath, "/SKILL.md"):
		return "skill"
	default:
		return "file"
	}
}

func binaryMetadata(data []byte) map[string]interface{} {
	sum := sha256.Sum256(data)
	return map[string]interface{}{
		"binary":       true,
		"blob_storage": "sqlite",
		"size_bytes":   len(data),
		"sha256":       hex.EncodeToString(sum[:]),
	}
}

func isBinaryMetadata(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	value, ok := metadata["binary"]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == "true"
	default:
		return false
	}
}

func maxTrust(values ...int) int {
	out := 0
	for _, value := range values {
		if value > out {
			out = value
		}
	}
	if out == 0 {
		return models.TrustLevelGuest
	}
	return out
}

func detectContentTypeFromPath(pathValue string) string {
	if ext := strings.TrimSpace(strings.ToLower(path.Ext(pathValue))); ext != "" {
		if guess := mime.TypeByExtension(ext); guess != "" {
			return guess
		}
	}
	return "text/plain"
}

func stringSliceValue(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	default:
		return nil
	}
}
