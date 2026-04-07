package localstore

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/agi-bar/agenthub/internal/systemskills"
	"github.com/google/uuid"
)

func (s *Store) ExportBundle(ctx context.Context, userID uuid.UUID, filters models.BundleFilters) (*models.Bundle, error) {
	profiles, err := s.GetProfiles(ctx, userID)
	if err != nil {
		return nil, err
	}
	scratch, err := s.GetScratchActive(ctx, userID)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.Snapshot(ctx, userID, "/skills", models.TrustLevelFull)
	if err != nil && err != services.ErrEntryNotFound {
		return nil, err
	}
	bundle := &models.Bundle{
		Version:   models.BundleVersionV1,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Source:    "agenthub-local",
		Mode:      "merge",
		Profile:   map[string]string{},
		Skills:    map[string]models.BundleSkill{},
	}
	includeDomains := domainSet(filters.IncludeDomains)
	if includeDomains["profile"] || len(includeDomains) == 0 {
		for _, profile := range profiles {
			bundle.Profile[profile.Category] = profile.Content
			bundle.Stats.ProfileItems++
		}
	}
	if includeDomains["memory"] || len(includeDomains) == 0 {
		for _, item := range scratch {
			bundle.Memory = append(bundle.Memory, models.BundleMemoryItem{
				Content:   item.Content,
				Title:     item.Title,
				Source:    item.Source,
				CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339),
				ExpiresAt: formatOptionalTime(item.ExpiresAt),
			})
			bundle.Stats.MemoryItems++
		}
	}
	if (includeDomains["skills"] || len(includeDomains) == 0) && snapshot != nil {
		for _, entry := range snapshot.Entries {
			publicPath := hubpath.NormalizePublic(entry.Path)
			if entry.IsDirectory || !strings.HasPrefix(publicPath, "/skills/") {
				continue
			}
			skillName, relPath, ok := splitSkillPath(publicPath)
			if !ok || systemskills.IsProtectedPath(publicPath) {
				continue
			}
			if !skillIncluded(skillName, filters) {
				continue
			}
			skill := bundle.Skills[skillName]
			if skill.Files == nil {
				skill.Files = map[string]string{}
			}
			if skill.BinaryFiles == nil {
				skill.BinaryFiles = map[string]models.BundleBlobFile{}
			}
			if isBinaryMetadata(entry.Metadata) {
				data, ok, err := s.ReadBlobByEntryID(ctx, entry.ID)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, fmt.Errorf("blob missing for %s", publicPath)
				}
				hash := sha256.Sum256(data)
				skill.BinaryFiles[relPath] = models.BundleBlobFile{
					ContentBase64: base64.StdEncoding.EncodeToString(data),
					ContentType:   entry.ContentType,
					SizeBytes:     int64(len(data)),
					SHA256:        hex.EncodeToString(hash[:]),
				}
				bundle.Stats.BinaryFiles++
				bundle.Stats.TotalBytes += int64(len(data))
			} else {
				skill.Files[relPath] = entry.Content
				bundle.Stats.TotalBytes += int64(len(entry.Content))
			}
			bundle.Skills[skillName] = skill
			bundle.Stats.TotalFiles++
		}
	}
	bundle.Stats.TotalSkills = len(bundle.Skills)
	return bundle, nil
}

func (s *Store) PreviewBundle(ctx context.Context, userID uuid.UUID, bundle models.Bundle) (*models.BundlePreviewResult, error) {
	if bundle.Version != models.BundleVersionV1 {
		return nil, fmt.Errorf("unsupported bundle version %q", bundle.Version)
	}
	mode := normalizeBundleMode(bundle.Mode)
	if mode == "" {
		return nil, fmt.Errorf("invalid mode %q", bundle.Mode)
	}
	preview := &models.BundlePreviewResult{
		Version: bundle.Version,
		Mode:    mode,
		Skills:  map[string]models.BundleSkillPreview{},
	}
	profiles, _ := s.GetProfiles(ctx, userID)
	profileMap := map[string]string{}
	for _, profile := range profiles {
		profileMap[profile.Category] = profile.Content
	}
	for category, content := range bundle.Profile {
		action := "create"
		if existing, ok := profileMap[category]; ok {
			if existing == content {
				action = "skip"
			} else {
				action = "update"
			}
		}
		entry := models.BundlePreviewEntry{Path: hubpath.ProfilePath(category), Action: action, Kind: "profile"}
		preview.Profile = append(preview.Profile, entry)
		applyPreviewAction(&preview.Summary, action)
	}
	existingScratch, _ := s.GetScratchActive(ctx, userID)
	scratchPaths := map[string]string{}
	for _, item := range existingScratch {
		key := importedScratchPath(bundleMemoryToValidated(item.Source, item.Title, item.CreatedAt, item.ExpiresAt))
		scratchPaths[key] = item.Content
	}
	for _, item := range bundle.Memory {
		validated, err := validateBundleMemory(item)
		if err != nil {
			return nil, err
		}
		pathValue := importedScratchPath(validated)
		action := "create"
		if existing, ok := scratchPaths[pathValue]; ok {
			if existing == validated.content {
				action = "skip"
			} else {
				action = "update"
			}
		}
		entry := models.BundlePreviewEntry{Path: pathValue, Action: action, Kind: "memory"}
		preview.Memory = append(preview.Memory, entry)
		applyPreviewAction(&preview.Summary, action)
	}
	skillNames := sortedSkillNames(bundle.Skills)
	for _, skillName := range skillNames {
		skill := bundle.Skills[skillName]
		skillPreview := models.BundleSkillPreview{}
		existing, _ := s.Snapshot(ctx, userID, path.Join("/skills", skillName), models.TrustLevelFull)
		existingMap := map[string]models.FileTreeEntry{}
		if existing != nil {
			for _, entry := range existing.Entries {
				if entry.IsDirectory {
					continue
				}
				_, relPath, ok := splitSkillPath(hubpath.NormalizePublic(entry.Path))
				if ok {
					existingMap[relPath] = entry
				}
			}
		}
		declared := map[string]struct{}{}
		for relPath, content := range skill.Files {
			declared[relPath] = struct{}{}
			action := "create"
			if current, ok := existingMap[relPath]; ok {
				if current.Content == content && !isBinaryMetadata(current.Metadata) {
					action = "skip"
				} else {
					action = "update"
				}
			}
			entry := models.BundlePreviewEntry{Path: path.Join("/skills", skillName, relPath), Action: action, Kind: "text"}
			skillPreview.Files = append(skillPreview.Files, entry)
			applyPreviewAction(&skillPreview.Summary, action)
			applyPreviewAction(&preview.Summary, action)
		}
		for relPath, blob := range skill.BinaryFiles {
			declared[relPath] = struct{}{}
			action := "create"
			if current, ok := existingMap[relPath]; ok {
				currentHash, _ := current.Metadata["sha256"].(string)
				if isBinaryMetadata(current.Metadata) && currentHash == blob.SHA256 {
					action = "skip"
				} else {
					action = "update"
				}
			}
			entry := models.BundlePreviewEntry{Path: path.Join("/skills", skillName, relPath), Action: action, Kind: "binary"}
			skillPreview.Files = append(skillPreview.Files, entry)
			applyPreviewAction(&skillPreview.Summary, action)
			applyPreviewAction(&preview.Summary, action)
		}
		if mode == "mirror" {
			for relPath := range existingMap {
				if _, ok := declared[relPath]; ok {
					continue
				}
				entry := models.BundlePreviewEntry{Path: path.Join("/skills", skillName, relPath), Action: "delete", Kind: "file"}
				skillPreview.Files = append(skillPreview.Files, entry)
				applyPreviewAction(&skillPreview.Summary, "delete")
				applyPreviewAction(&preview.Summary, "delete")
			}
		}
		sort.Slice(skillPreview.Files, func(i, j int) bool { return skillPreview.Files[i].Path < skillPreview.Files[j].Path })
		preview.Skills[skillName] = skillPreview
	}
	preview.Fingerprint = previewFingerprint(preview)
	return preview, nil
}

func (s *Store) ImportBundle(ctx context.Context, userID uuid.UUID, bundle models.Bundle) (*models.BundleImportResult, error) {
	return s.importBundle(ctx, userID, bundle, nil)
}

func (s *Store) importBundle(ctx context.Context, userID uuid.UUID, bundle models.Bundle, existingJob *models.SyncJob) (*models.BundleImportResult, error) {
	if bundle.Version != models.BundleVersionV1 {
		return nil, fmt.Errorf("unsupported bundle version %q", bundle.Version)
	}
	mode := normalizeBundleMode(bundle.Mode)
	if mode == "" {
		return nil, fmt.Errorf("invalid mode %q", bundle.Mode)
	}
	jobID := uuid.Nil
	if existingJob != nil {
		jobID = existingJob.ID
	} else {
		jobID = uuid.New()
		if err := s.insertSyncJob(ctx, models.SyncJob{
			ID:        jobID,
			UserID:    userID,
			Direction: models.SyncJobDirectionImport,
			Transport: models.SyncJobTransportJSON,
			Status:    models.SyncJobStatusRunning,
			Source:    bundle.Source,
			Mode:      mode,
			Summary:   syncSummaryFromBundleStats(bundle.Stats),
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}); err != nil {
			return nil, err
		}
	}
	result := &models.BundleImportResult{Version: bundle.Version, Mode: mode}
	for category, content := range bundle.Profile {
		if err := s.UpsertProfile(ctx, userID, category, content, "bundle-import"); err != nil {
			_ = s.finishSyncJob(ctx, jobID, userID, models.SyncJobStatusFailed, models.SyncJobSummary{}, err.Error())
			return nil, err
		}
		result.ProfileCategories++
	}
	for _, item := range bundle.Memory {
		validated, err := validateBundleMemory(item)
		if err != nil {
			_ = s.finishSyncJob(ctx, jobID, userID, models.SyncJobStatusFailed, models.SyncJobSummary{}, err.Error())
			return nil, err
		}
		if _, err := s.ImportScratch(ctx, userID, validated.content, validated.source, validated.title, validated.createdAt, validated.expiresAt); err != nil {
			_ = s.finishSyncJob(ctx, jobID, userID, models.SyncJobStatusFailed, models.SyncJobSummary{}, err.Error())
			return nil, err
		}
		result.MemoryImported++
	}
	for _, skillName := range sortedSkillNames(bundle.Skills) {
		skill := bundle.Skills[skillName]
		declared := map[string]struct{}{}
		for relPath, content := range skill.Files {
			declared[relPath] = struct{}{}
			if _, err := s.WriteEntry(ctx, userID, path.Join("/skills", skillName, relPath), content, detectContentTypeFromPath(relPath), models.FileTreeWriteOptions{
				MinTrustLevel: models.TrustLevelGuest,
			}); err != nil {
				_ = s.finishSyncJob(ctx, jobID, userID, models.SyncJobStatusFailed, models.SyncJobSummary{}, err.Error())
				return nil, err
			}
			result.FilesWritten++
		}
		for relPath, blob := range skill.BinaryFiles {
			declared[relPath] = struct{}{}
			data, err := base64.StdEncoding.DecodeString(blob.ContentBase64)
			if err != nil {
				_ = s.finishSyncJob(ctx, jobID, userID, models.SyncJobStatusFailed, models.SyncJobSummary{}, err.Error())
				return nil, err
			}
			if _, err := s.WriteBinaryEntry(ctx, userID, path.Join("/skills", skillName, relPath), data, blob.ContentType, models.FileTreeWriteOptions{
				MinTrustLevel: models.TrustLevelGuest,
			}); err != nil {
				_ = s.finishSyncJob(ctx, jobID, userID, models.SyncJobStatusFailed, models.SyncJobSummary{}, err.Error())
				return nil, err
			}
			result.FilesWritten++
		}
		if mode == "mirror" {
			snapshot, err := s.Snapshot(ctx, userID, path.Join("/skills", skillName), models.TrustLevelFull)
			if err == nil {
				for _, entry := range snapshot.Entries {
					if entry.IsDirectory {
						continue
					}
					_, relPath, ok := splitSkillPath(hubpath.NormalizePublic(entry.Path))
					if !ok {
						continue
					}
					if _, ok := declared[relPath]; ok {
						continue
					}
					if err := s.Delete(ctx, userID, entry.Path); err != nil {
						_ = s.finishSyncJob(ctx, jobID, userID, models.SyncJobStatusFailed, models.SyncJobSummary{}, err.Error())
						return nil, err
					}
					result.FilesDeleted++
				}
			}
		}
		result.SkillsWritten++
	}
	if err := s.finishSyncJob(ctx, jobID, userID, models.SyncJobStatusSucceeded, syncSummaryFromImportResult(result), ""); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) ExportArchive(ctx context.Context, userID uuid.UUID, filters models.BundleFilters) ([]byte, *models.BundleArchiveManifest, error) {
	bundle, err := s.ExportBundle(ctx, userID, filters)
	if err != nil {
		return nil, nil, err
	}
	archive, manifest, err := services.BuildBundleArchive(*bundle, filters)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	job := models.SyncJob{
		ID:          uuid.New(),
		UserID:      userID,
		Direction:   models.SyncJobDirectionExport,
		Transport:   models.SyncJobTransportArchive,
		Status:      models.SyncJobStatusSucceeded,
		Source:      "agenthub-local",
		Mode:        manifest.Mode,
		Filters:     filters,
		Summary:     syncSummaryFromBundleStats(bundle.Stats),
		CreatedAt:   now,
		UpdatedAt:   now,
		CompletedAt: &now,
	}
	if err := s.insertSyncJob(ctx, job); err != nil {
		return nil, nil, err
	}
	return archive, manifest, nil
}

func (s *Store) PreviewManifest(ctx context.Context, userID uuid.UUID, manifest models.BundleArchiveManifest) (*models.BundlePreviewResult, error) {
	archive, err := materializeArchiveStub(manifest)
	if err != nil {
		return nil, err
	}
	_, parsedManifest, err := services.ParseBundleArchive(archive)
	if err != nil {
		return nil, err
	}
	return s.previewManifestMeta(ctx, userID, *parsedManifest)
}

func (s *Store) StartSession(ctx context.Context, userID uuid.UUID, req models.SyncStartSessionRequest) (*models.SyncSessionResponse, error) {
	if req.TransportVersion == "" {
		req.TransportVersion = models.SyncTransportVersionV1
	}
	if req.TransportVersion != models.SyncTransportVersionV1 {
		return nil, fmt.Errorf("unsupported transport version %q", req.TransportVersion)
	}
	if req.Format != models.BundleFormatArchive {
		return nil, fmt.Errorf("unsupported format %q", req.Format)
	}
	mode := normalizeBundleMode(req.Mode)
	if mode == "" {
		return nil, fmt.Errorf("invalid mode %q", req.Mode)
	}
	now := time.Now().UTC()
	sessionID := uuid.New()
	jobID := uuid.New()
	totalParts := int((req.ArchiveSizeBytes + models.DefaultSyncChunkSize - 1) / models.DefaultSyncChunkSize)
	if totalParts == 0 {
		totalParts = 1
	}
	session := models.SyncSession{
		ID:               sessionID,
		UserID:           userID,
		JobID:            jobID,
		Status:           models.SyncSessionStatusUploading,
		Format:           req.Format,
		Mode:             mode,
		Manifest:         req.Manifest,
		ArchiveSizeBytes: req.ArchiveSizeBytes,
		ArchiveSHA256:    req.ArchiveSHA256,
		ChunkSizeBytes:   models.DefaultSyncChunkSize,
		TotalParts:       totalParts,
		ExpiresAt:        now.Add(24 * time.Hour),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sync_sessions (
			id, user_id, job_id, status, format, mode, manifest_json, archive_size_bytes, archive_sha256,
			chunk_size_bytes, total_parts, expires_at, created_at, updated_at, committed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		session.ID.String(),
		userID.String(),
		jobID.String(),
		session.Status,
		session.Format,
		session.Mode,
		encodeJSON(session.Manifest),
		session.ArchiveSizeBytes,
		session.ArchiveSHA256,
		session.ChunkSizeBytes,
		session.TotalParts,
		timeText(session.ExpiresAt),
		timeText(session.CreatedAt),
		timeText(session.UpdatedAt),
	); err != nil {
		return nil, err
	}
	if err := s.insertSyncJob(ctx, models.SyncJob{
		ID:        jobID,
		UserID:    userID,
		SessionID: &sessionID,
		Direction: models.SyncJobDirectionImport,
		Transport: models.SyncJobTransportArchive,
		Status:    models.SyncJobStatusRunning,
		Source:    req.Manifest.Source,
		Mode:      mode,
		Filters:   req.Manifest.Filters,
		Summary:   syncSummaryFromBundleStats(req.Manifest.Stats),
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return nil, err
	}
	return s.sessionResponse(ctx, session), nil
}

func (s *Store) UploadPart(ctx context.Context, userID, sessionID uuid.UUID, index int, data []byte) (*models.SyncSessionResponse, error) {
	session, err := s.loadSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status == models.SyncSessionStatusExpired || time.Now().UTC().After(session.ExpiresAt) {
		return nil, services.ErrSyncSessionExpired
	}
	if index < 0 || index >= session.TotalParts {
		return nil, fmt.Errorf("part index %d out of range", index)
	}
	hash := sha256.Sum256(data)
	partHash := hex.EncodeToString(hash[:])
	row := s.db.QueryRowContext(ctx, `SELECT part_hash FROM sync_session_parts WHERE session_id = ? AND part_index = ?`, sessionID.String(), index)
	var existingHash string
	if err := row.Scan(&existingHash); err == nil {
		if existingHash != partHash {
			return nil, services.ErrSyncPartConflict
		}
		_, _ = s.db.ExecContext(ctx, `UPDATE sync_session_parts SET updated_at = ? WHERE session_id = ? AND part_index = ?`, timeText(time.Now().UTC()), sessionID.String(), index)
		return s.sessionResponse(ctx, *session), nil
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sync_session_parts (session_id, part_index, part_hash, data, size_bytes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sessionID.String(), index, partHash, data, len(data), timeText(now), timeText(now),
	); err != nil {
		return nil, err
	}
	received, _ := s.receivedParts(ctx, sessionID)
	status := models.SyncSessionStatusUploading
	if len(received) == session.TotalParts {
		status = models.SyncSessionStatusReady
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE sync_sessions SET status = ?, updated_at = ? WHERE id = ?`, status, timeText(now), sessionID.String())
	session.Status = status
	session.UpdatedAt = now
	return s.sessionResponse(ctx, *session), nil
}

func (s *Store) GetSession(ctx context.Context, userID, sessionID uuid.UUID) (*models.SyncSessionResponse, error) {
	session, err := s.loadSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	return s.sessionResponse(ctx, *session), nil
}

func (s *Store) AbortSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	session, err := s.loadSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `UPDATE sync_sessions SET status = ?, updated_at = ? WHERE id = ?`, models.SyncSessionStatusAborted, timeText(now), sessionID.String()); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sync_session_parts WHERE session_id = ?`, sessionID.String()); err != nil {
		return err
	}
	return s.finishSyncJob(ctx, session.JobID, userID, models.SyncJobStatusAborted, sessionSummary(session.Manifest.Stats), "session aborted")
}

func (s *Store) CommitSession(ctx context.Context, userID, sessionID uuid.UUID, req models.SyncCommitRequest) (*models.BundleImportResult, error) {
	session, err := s.loadSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status == models.SyncSessionStatusExpired || time.Now().UTC().After(session.ExpiresAt) {
		return nil, services.ErrSyncSessionExpired
	}
	received, err := s.receivedParts(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(received) != session.TotalParts {
		return nil, services.ErrSyncSessionIncomplete
	}
	archiveData, err := s.readArchiveData(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	bundle, manifest, err := services.ParseBundleArchive(archiveData)
	if err != nil {
		return nil, err
	}
	if session.ArchiveSHA256 != "" && !strings.EqualFold(manifest.ArchiveSHA256, session.ArchiveSHA256) {
		return nil, fmt.Errorf("archive sha mismatch")
	}
	if req.PreviewFingerprint != "" {
		preview, err := s.PreviewBundle(ctx, userID, *bundle)
		if err != nil {
			return nil, err
		}
		if preview.Fingerprint != req.PreviewFingerprint {
			return nil, services.ErrSyncPreviewDrift
		}
	}
	result, err := s.importBundle(ctx, userID, *bundle, &models.SyncJob{
		ID:        session.JobID,
		UserID:    userID,
		Direction: models.SyncJobDirectionImport,
		Transport: models.SyncJobTransportArchive,
		Status:    models.SyncJobStatusRunning,
		Source:    session.Manifest.Source,
		Mode:      session.Mode,
		Filters:   session.Manifest.Filters,
	})
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	_, _ = s.db.ExecContext(ctx, `UPDATE sync_sessions SET status = ?, updated_at = ?, committed_at = ? WHERE id = ?`, models.SyncSessionStatusCommitted, timeText(now), timeText(now), sessionID.String())
	_, _ = s.db.ExecContext(ctx, `DELETE FROM sync_session_parts WHERE session_id = ?`, sessionID.String())
	return result, nil
}

func (s *Store) ListJobs(ctx context.Context, userID uuid.UUID) ([]models.SyncJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, session_id, direction, transport, status, source, mode, filters_json, summary_json, error, created_at, updated_at, completed_at
		   FROM sync_jobs
		  WHERE user_id = ?
		  ORDER BY created_at DESC`,
		userID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]models.SyncJob, 0, 16)
	for rows.Next() {
		job, err := scanSyncJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *job)
	}
	return jobs, rows.Err()
}

func (s *Store) GetJob(ctx context.Context, userID, jobID uuid.UUID) (*models.SyncJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, session_id, direction, transport, status, source, mode, filters_json, summary_json, error, created_at, updated_at, completed_at
		   FROM sync_jobs
		  WHERE user_id = ? AND id = ?`,
		userID.String(),
		jobID.String(),
	)
	return scanSyncJob(row)
}

func (s *Store) CleanExpiredSyncSessions(ctx context.Context) (*services.SyncCleanupResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, job_id, user_id FROM sync_sessions
		  WHERE status NOT IN (?, ?, ?) AND expires_at <= ?`,
		models.SyncSessionStatusCommitted,
		models.SyncSessionStatusAborted,
		models.SyncSessionStatusExpired,
		timeText(time.Now().UTC()),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &services.SyncCleanupResult{}
	type expiredSession struct {
		sessionID string
		jobID     string
		userID    string
	}
	var sessions []expiredSession
	for rows.Next() {
		var item expiredSession
		if err := rows.Scan(&item.sessionID, &item.jobID, &item.userID); err != nil {
			return nil, err
		}
		sessions = append(sessions, item)
	}
	for _, session := range sessions {
		var bytes int64
		_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size_bytes), 0) FROM sync_session_parts WHERE session_id = ?`, session.sessionID).Scan(&bytes)
		if _, err := s.db.ExecContext(ctx, `DELETE FROM sync_session_parts WHERE session_id = ?`, session.sessionID); err != nil {
			return nil, err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE sync_sessions SET status = ?, updated_at = ? WHERE id = ?`, models.SyncSessionStatusExpired, timeText(time.Now().UTC()), session.sessionID); err != nil {
			return nil, err
		}
		jobID, jobErr := uuid.Parse(session.jobID)
		userID, userErr := uuid.Parse(session.userID)
		if jobErr == nil && userErr == nil {
			_ = s.finishSyncJob(ctx, jobID, userID, models.SyncJobStatusAborted, models.SyncJobSummary{}, "session expired")
		}
		result.ExpiredSessions++
		result.DeletedBytes += bytes
	}
	return result, nil
}

func (s *Store) sessionResponse(ctx context.Context, session models.SyncSession) *models.SyncSessionResponse {
	received, _ := s.receivedParts(ctx, session.ID)
	missing := make([]int, 0, session.TotalParts-len(received))
	receivedSet := map[int]struct{}{}
	for _, part := range received {
		receivedSet[part] = struct{}{}
	}
	for i := 0; i < session.TotalParts; i++ {
		if _, ok := receivedSet[i]; !ok {
			missing = append(missing, i)
		}
	}
	return &models.SyncSessionResponse{
		SessionID:      session.ID,
		JobID:          session.JobID,
		Status:         session.Status,
		ChunkSizeBytes: session.ChunkSizeBytes,
		TotalParts:     session.TotalParts,
		ExpiresAt:      session.ExpiresAt,
		Mode:           session.Mode,
		Summary:        sessionSummary(session.Manifest.Stats),
		ReceivedParts:  received,
		MissingParts:   missing,
	}
}

func (s *Store) loadSession(ctx context.Context, userID, sessionID uuid.UUID) (*models.SyncSession, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, job_id, status, format, mode, manifest_json, archive_size_bytes, archive_sha256,
		        chunk_size_bytes, total_parts, expires_at, created_at, updated_at, committed_at
		   FROM sync_sessions
		  WHERE id = ? AND user_id = ?`,
		sessionID.String(),
		userID.String(),
	)
	var (
		id               string
		userIDText       string
		jobIDText        string
		status           string
		format           string
		mode             string
		manifestJSON     string
		archiveSizeBytes int64
		archiveSHA       string
		chunkSizeBytes   int64
		totalParts       int
		expiresAt        string
		createdAt        string
		updatedAt        string
		committedAt      *string
	)
	if err := row.Scan(&id, &userIDText, &jobIDText, &status, &format, &mode, &manifestJSON, &archiveSizeBytes, &archiveSHA, &chunkSizeBytes, &totalParts, &expiresAt, &createdAt, &updatedAt, &committedAt); err != nil {
		return nil, services.ErrSyncSessionNotFound
	}
	var manifest models.BundleArchiveManifest
	_ = json.Unmarshal([]byte(manifestJSON), &manifest)
	parsedID, _ := uuid.Parse(id)
	parsedUserID, _ := uuid.Parse(userIDText)
	parsedJobID, _ := uuid.Parse(jobIDText)
	session := &models.SyncSession{
		ID:               parsedID,
		UserID:           parsedUserID,
		JobID:            parsedJobID,
		Status:           status,
		Format:           format,
		Mode:             mode,
		Manifest:         manifest,
		ArchiveSizeBytes: archiveSizeBytes,
		ArchiveSHA256:    archiveSHA,
		ChunkSizeBytes:   chunkSizeBytes,
		TotalParts:       totalParts,
		ExpiresAt:        mustParseTime(expiresAt),
		CreatedAt:        mustParseTime(createdAt),
		UpdatedAt:        mustParseTime(updatedAt),
	}
	if committedAt != nil {
		ts := mustParseTime(*committedAt)
		session.CommittedAt = &ts
	}
	return session, nil
}

func (s *Store) receivedParts(ctx context.Context, sessionID uuid.UUID) ([]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT part_index FROM sync_session_parts WHERE session_id = ? ORDER BY part_index ASC`, sessionID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var parts []int
	for rows.Next() {
		var idx int
		if err := rows.Scan(&idx); err != nil {
			return nil, err
		}
		parts = append(parts, idx)
	}
	return parts, rows.Err()
}

func (s *Store) readArchiveData(ctx context.Context, sessionID uuid.UUID) ([]byte, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM sync_session_parts WHERE session_id = ? ORDER BY part_index ASC`, sessionID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var combined []byte
	for rows.Next() {
		var part []byte
		if err := rows.Scan(&part); err != nil {
			return nil, err
		}
		combined = append(combined, part...)
	}
	return combined, rows.Err()
}

func (s *Store) insertSyncJob(ctx context.Context, job models.SyncJob) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sync_jobs (
			id, user_id, session_id, direction, transport, status, source, mode, filters_json,
			summary_json, error, created_at, updated_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID.String(),
		job.UserID.String(),
		uuidPtrString(job.SessionID),
		job.Direction,
		job.Transport,
		job.Status,
		job.Source,
		job.Mode,
		encodeJSON(job.Filters),
		encodeJSON(job.Summary),
		job.Error,
		timeText(job.CreatedAt),
		timeText(job.UpdatedAt),
		timePtrText(job.CompletedAt),
	)
	return err
}

func (s *Store) finishSyncJob(ctx context.Context, jobID, userID uuid.UUID, status string, summary models.SyncJobSummary, errorText string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE sync_jobs
		    SET status = ?, summary_json = ?, error = ?, updated_at = ?, completed_at = ?
		  WHERE id = ? AND user_id = ?`,
		status,
		encodeJSON(summary),
		errorText,
		timeText(now),
		timeText(now),
		jobID.String(),
		userID.String(),
	)
	return err
}

type syncJobScanner interface{ Scan(dest ...any) error }

func scanSyncJob(row syncJobScanner) (*models.SyncJob, error) {
	var (
		id          string
		userID      string
		sessionID   *string
		direction   string
		transport   string
		status      string
		source      string
		mode        string
		filtersJSON string
		summaryJSON string
		errorText   string
		createdAt   string
		updatedAt   string
		completedAt *string
	)
	if err := row.Scan(&id, &userID, &sessionID, &direction, &transport, &status, &source, &mode, &filtersJSON, &summaryJSON, &errorText, &createdAt, &updatedAt, &completedAt); err != nil {
		return nil, services.ErrSyncSessionNotFound
	}
	jobID, _ := uuid.Parse(id)
	userUUID, _ := uuid.Parse(userID)
	var filters models.BundleFilters
	_ = json.Unmarshal([]byte(filtersJSON), &filters)
	var summary models.SyncJobSummary
	_ = json.Unmarshal([]byte(summaryJSON), &summary)
	job := &models.SyncJob{
		ID:        jobID,
		UserID:    userUUID,
		Direction: direction,
		Transport: transport,
		Status:    status,
		Source:    source,
		Mode:      mode,
		Filters:   filters,
		Summary:   summary,
		Error:     errorText,
		CreatedAt: mustParseTime(createdAt),
		UpdatedAt: mustParseTime(updatedAt),
	}
	if sessionID != nil && strings.TrimSpace(*sessionID) != "" {
		parsedSessionID, _ := uuid.Parse(*sessionID)
		job.SessionID = &parsedSessionID
	}
	if completedAt != nil {
		ts := mustParseTime(*completedAt)
		job.CompletedAt = &ts
	}
	return job, nil
}

func normalizeBundleMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "merge":
		return "merge"
	case "mirror":
		return "mirror"
	default:
		return ""
	}
}

func domainSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			set[value] = true
		}
	}
	return set
}

func sortedSkillNames(skills map[string]models.BundleSkill) []string {
	names := make([]string, 0, len(skills))
	for name := range skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func splitSkillPath(publicPath string) (string, string, bool) {
	trimmed := strings.TrimPrefix(hubpath.NormalizePublic(publicPath), "/skills/")
	if trimmed == publicPath || trimmed == "" {
		return "", "", false
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func skillIncluded(name string, filters models.BundleFilters) bool {
	if len(filters.IncludeSkills) > 0 {
		found := false
		for _, include := range filters.IncludeSkills {
			if include == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, exclude := range filters.ExcludeSkills {
		if exclude == name {
			return false
		}
	}
	return true
}

type validatedBundleMemoryItem struct {
	content   string
	title     string
	source    string
	createdAt time.Time
	expiresAt *time.Time
}

func validateBundleMemory(item models.BundleMemoryItem) (validatedBundleMemoryItem, error) {
	if strings.TrimSpace(item.Content) == "" {
		return validatedBundleMemoryItem{}, fmt.Errorf("memory content is required")
	}
	createdAt := time.Now().UTC()
	if strings.TrimSpace(item.CreatedAt) != "" {
		ts, err := time.Parse(time.RFC3339, item.CreatedAt)
		if err != nil {
			return validatedBundleMemoryItem{}, fmt.Errorf("invalid memory created_at %q", item.CreatedAt)
		}
		createdAt = ts.UTC()
	}
	var expiresAt *time.Time
	if strings.TrimSpace(item.ExpiresAt) != "" {
		ts, err := time.Parse(time.RFC3339, item.ExpiresAt)
		if err != nil {
			return validatedBundleMemoryItem{}, fmt.Errorf("invalid memory expires_at %q", item.ExpiresAt)
		}
		utc := ts.UTC()
		expiresAt = &utc
	}
	return validatedBundleMemoryItem{
		content:   item.Content,
		title:     item.Title,
		source:    item.Source,
		createdAt: createdAt,
		expiresAt: expiresAt,
	}, nil
}

func bundleMemoryToValidated(source, title string, createdAt time.Time, expiresAt *time.Time) validatedBundleMemoryItem {
	return validatedBundleMemoryItem{source: source, title: title, createdAt: createdAt, expiresAt: expiresAt}
}

func importedScratchPath(item validatedBundleMemoryItem) string {
	slugBase := item.title
	if strings.TrimSpace(slugBase) == "" {
		slugBase = item.source
	}
	return hubpath.ScratchPath(item.createdAt, fmt.Sprintf("%s-%s", slugBase, uuid.NewSHA1(uuid.NameSpaceURL, []byte(item.source+item.title+item.createdAt.Format(time.RFC3339Nano))).String()[:8]))
}

func applyPreviewAction(summary *models.BundlePreviewSummary, action string) {
	switch action {
	case "create":
		summary.Create++
	case "update":
		summary.Update++
	case "delete":
		summary.Delete++
	case "conflict":
		summary.Conflict++
	default:
		summary.Skip++
	}
}

func previewFingerprint(preview *models.BundlePreviewResult) string {
	data, _ := json.Marshal(preview)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func syncSummaryFromBundleStats(stats models.BundleStats) models.SyncJobSummary {
	return models.SyncJobSummary{
		TotalSkills:  stats.TotalSkills,
		TotalFiles:   stats.TotalFiles,
		TotalBytes:   stats.TotalBytes,
		BinaryFiles:  stats.BinaryFiles,
		ProfileItems: stats.ProfileItems,
		MemoryItems:  stats.MemoryItems,
	}
}

func syncSummaryFromImportResult(result *models.BundleImportResult) models.SyncJobSummary {
	if result == nil {
		return models.SyncJobSummary{}
	}
	return models.SyncJobSummary{
		SkillsWritten:     result.SkillsWritten,
		FilesWritten:      result.FilesWritten,
		FilesDeleted:      result.FilesDeleted,
		ProfileCategories: result.ProfileCategories,
		MemoryImported:    result.MemoryImported,
	}
}

func sessionSummary(stats models.BundleStats) models.SyncJobSummary {
	return syncSummaryFromBundleStats(stats)
}

func uuidPtrString(value *uuid.UUID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func timePtrText(value *time.Time) any {
	if value == nil {
		return nil
	}
	return timeText(*value)
}

func materializeArchiveStub(manifest models.BundleArchiveManifest) ([]byte, error) {
	archive, _, err := services.BuildBundleArchive(models.Bundle{
		Version:   models.BundleVersionV1,
		CreatedAt: manifest.CreatedAt,
		Source:    manifest.Source,
		Mode:      manifest.Mode,
	}, manifest.Filters)
	return archive, err
}

func (s *Store) previewManifestMeta(ctx context.Context, userID uuid.UUID, manifest models.BundleArchiveManifest) (*models.BundlePreviewResult, error) {
	preview := &models.BundlePreviewResult{
		Version: manifest.Version,
		Mode:    normalizeBundleMode(manifest.Mode),
		Skills:  map[string]models.BundleSkillPreview{},
	}
	for category, entry := range manifest.ProfileFiles {
		action := "create"
		current, err := s.Read(ctx, userID, hubpath.ProfilePath(category), models.TrustLevelFull)
		if err == nil {
			if current.Checksum == entry.SHA256 {
				action = "skip"
			} else {
				action = "update"
			}
		}
		item := models.BundlePreviewEntry{Path: hubpath.ProfilePath(category), Action: action, Kind: "profile"}
		preview.Profile = append(preview.Profile, item)
		applyPreviewAction(&preview.Summary, action)
	}
	for _, item := range manifest.MemoryItems {
		entry := models.BundlePreviewEntry{Path: item.ArchivePath, Action: "create", Kind: "memory"}
		preview.Memory = append(preview.Memory, entry)
		applyPreviewAction(&preview.Summary, "create")
	}
	for skillName, files := range manifest.SkillFiles {
		skillPreview := models.BundleSkillPreview{}
		for relPath, entry := range files {
			kind := "text"
			if entry.Binary {
				kind = "binary"
			}
			item := models.BundlePreviewEntry{Path: path.Join("/skills", skillName, relPath), Action: "create", Kind: kind}
			skillPreview.Files = append(skillPreview.Files, item)
			applyPreviewAction(&skillPreview.Summary, "create")
			applyPreviewAction(&preview.Summary, "create")
		}
		preview.Skills[skillName] = skillPreview
	}
	preview.Fingerprint = previewFingerprint(preview)
	return preview, nil
}
