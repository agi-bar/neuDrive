package services

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
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

var (
	ErrSyncSessionNotFound   = errors.New("sync session not found")
	ErrSyncSessionExpired    = errors.New("sync session expired")
	ErrSyncSessionIncomplete = errors.New("sync session is incomplete")
	ErrSyncPartConflict      = errors.New("sync session part conflict")
	ErrSyncPreviewDrift      = errors.New("sync preview drift detected")
)

type SyncCleanupResult struct {
	ExpiredSessions int64
	DeletedParts    int64
	DeletedBytes    int64
}

type SyncService struct {
	db            *pgxpool.Pool
	repo          SyncRepo
	importSvc     *ImportService
	exportSvc     *ExportService
	fileTree      *FileTreeService
	memory        *MemoryService
	chunkSize     int64
	autoThreshold int
}

func NewSyncService(
	db *pgxpool.Pool,
	importSvc *ImportService,
	exportSvc *ExportService,
	fileTree *FileTreeService,
	memory *MemoryService,
) *SyncService {
	return &SyncService{
		db:            db,
		importSvc:     importSvc,
		exportSvc:     exportSvc,
		fileTree:      fileTree,
		memory:        memory,
		chunkSize:     models.DefaultSyncChunkSize,
		autoThreshold: models.DefaultArchiveAutoThreshold,
	}
}

func NewSyncServiceWithRepo(
	repo SyncRepo,
	importSvc *ImportService,
	exportSvc *ExportService,
	fileTree *FileTreeService,
	memory *MemoryService,
) *SyncService {
	return &SyncService{
		repo:          repo,
		importSvc:     importSvc,
		exportSvc:     exportSvc,
		fileTree:      fileTree,
		memory:        memory,
		chunkSize:     models.DefaultSyncChunkSize,
		autoThreshold: models.DefaultArchiveAutoThreshold,
	}
}

func (s *SyncService) PreviewBundle(ctx context.Context, userID uuid.UUID, bundle models.Bundle) (*models.BundlePreviewResult, error) {
	if s.importSvc == nil {
		return nil, fmt.Errorf("sync.PreviewBundle: import service not configured")
	}
	preview, err := s.importSvc.PreviewBundle(ctx, userID, bundle)
	if err != nil {
		return nil, err
	}
	preview.Fingerprint = bundlePreviewFingerprint(preview)
	return preview, nil
}

func (s *SyncService) PreviewManifest(ctx context.Context, userID uuid.UUID, manifest models.BundleArchiveManifest) (*models.BundlePreviewResult, error) {
	if s.fileTree == nil || s.memory == nil {
		return nil, fmt.Errorf("sync.PreviewManifest: required services not configured")
	}
	if manifest.Version != models.BundleVersionV2 {
		return nil, fmt.Errorf("sync.PreviewManifest: unsupported bundle version %q", manifest.Version)
	}

	mode := normalizeBundleMode(manifest.Mode)
	if mode == "" {
		return nil, fmt.Errorf("sync.PreviewManifest: invalid mode %q", manifest.Mode)
	}

	preview := &models.BundlePreviewResult{
		Version: manifest.Version,
		Mode:    mode,
		Skills:  make(map[string]models.BundleSkillPreview, len(manifest.SkillFiles)),
	}

	profileKeys := make([]string, 0, len(manifest.ProfileFiles))
	for category := range manifest.ProfileFiles {
		profileKeys = append(profileKeys, category)
	}
	sort.Strings(profileKeys)
	for _, category := range profileKeys {
		fullPath := hubpath.ProfilePath(category)
		action, err := s.previewManifestTextPath(ctx, userID, fullPath, manifest.ProfileFiles[category].SHA256, "text/markdown")
		if err != nil {
			return nil, err
		}
		entry := models.BundlePreviewEntry{
			Path:   fullPath,
			Action: action,
			Kind:   "profile",
		}
		preview.Profile = append(preview.Profile, entry)
		applyBundlePreviewAction(&preview.Summary, action)
	}

	for _, item := range manifest.MemoryItems {
		validated := validatedBundleMemoryItem{
			title:  item.Title,
			source: item.Source,
		}
		if strings.TrimSpace(item.CreatedAt) != "" {
			createdAt, err := time.Parse(time.RFC3339, item.CreatedAt)
			if err != nil {
				return nil, fmt.Errorf("sync.PreviewManifest: memory %q: invalid created_at %q", item.ID, item.CreatedAt)
			}
			validated.createdAt = createdAt.UTC()
		}
		if strings.TrimSpace(item.ExpiresAt) != "" {
			expiresAt, err := time.Parse(time.RFC3339, item.ExpiresAt)
			if err != nil {
				return nil, fmt.Errorf("sync.PreviewManifest: memory %q: invalid expires_at %q", item.ID, item.ExpiresAt)
			}
			ts := expiresAt.UTC()
			validated.expiresAt = &ts
		}
		scratchPath := importedScratchPath(validated)
		action, err := s.previewManifestTextPath(ctx, userID, scratchPath, item.SHA256, "text/markdown")
		if err != nil {
			return nil, err
		}
		entry := models.BundlePreviewEntry{
			Path:   scratchPath,
			Action: action,
			Kind:   "memory",
		}
		preview.Memory = append(preview.Memory, entry)
		applyBundlePreviewAction(&preview.Summary, action)
	}

	skillNames := make([]string, 0, len(manifest.SkillFiles))
	for skillName := range manifest.SkillFiles {
		skillNames = append(skillNames, skillName)
	}
	sort.Strings(skillNames)
	for _, skillName := range skillNames {
		if err := validateSlug(skillName, 128); err != nil {
			return nil, fmt.Errorf("sync.PreviewManifest: invalid skill name %q: %w", skillName, err)
		}
		skillRoot := path.Join("/skills", skillName)
		snapshot, err := s.fileTree.Snapshot(ctx, userID, skillRoot, models.TrustLevelFull)
		if err != nil {
			if errors.Is(err, ErrEntryNotFound) {
				snapshot = &models.EntrySnapshot{Path: skillRoot}
			} else {
				return nil, err
			}
		}

		existing := make(map[string]models.FileTreeEntry, len(snapshot.Entries))
		for _, entry := range snapshot.Entries {
			if entry.IsDirectory {
				continue
			}
			publicPath := hubpath.NormalizePublic(entry.Path)
			relPath := strings.TrimPrefix(publicPath, strings.TrimSuffix(skillRoot, "/")+"/")
			existing[relPath] = entry
		}

		skillPreview := models.BundleSkillPreview{}
		declared := make(map[string]struct{}, len(manifest.SkillFiles[skillName]))
		relPaths := make([]string, 0, len(manifest.SkillFiles[skillName]))
		for relPath := range manifest.SkillFiles[skillName] {
			relPaths = append(relPaths, relPath)
		}
		sort.Strings(relPaths)
		for _, relPath := range relPaths {
			entryMeta := manifest.SkillFiles[skillName][relPath]
			declared[relPath] = struct{}{}
			current, hasCurrent := existing[relPath]
			var currentEntry *models.FileTreeEntry
			var currentBlob []byte
			var blobExists bool
			if hasCurrent {
				currentEntry = &current
				currentBlob, blobExists, err = s.fileTree.ReadBlobByEntryID(ctx, current.ID)
				if err != nil {
					return nil, err
				}
			}

			action := "create"
			if entryMeta.Binary {
				action = previewBinaryHashAction(currentEntry, currentBlob, blobExists, entryMeta.SHA256, entryMeta.ContentType)
			} else {
				action = previewTextHashAction(currentEntry, blobExists, entryMeta.SHA256, entryMeta.ContentType)
			}
			kind := "text"
			if entryMeta.Binary {
				kind = "binary"
			}
			skillPreview.Files = append(skillPreview.Files, models.BundlePreviewEntry{
				Path:   path.Join(skillRoot, relPath),
				Action: action,
				Kind:   kind,
			})
			applyBundlePreviewAction(&skillPreview.Summary, action)
			applyBundlePreviewAction(&preview.Summary, action)
		}

		if mode == bundleModeMirror {
			existingPaths := sortedEntryKeys(existing)
			for _, relPath := range existingPaths {
				if _, ok := declared[relPath]; ok {
					continue
				}
				skillPreview.Files = append(skillPreview.Files, models.BundlePreviewEntry{
					Path:   path.Join(skillRoot, relPath),
					Action: "delete",
					Kind:   "file",
				})
				applyBundlePreviewAction(&skillPreview.Summary, "delete")
				applyBundlePreviewAction(&preview.Summary, "delete")
			}
		}

		preview.Skills[skillName] = skillPreview
	}

	preview.Fingerprint = bundlePreviewFingerprint(preview)
	return preview, nil
}

func (s *SyncService) ImportBundleJSON(ctx context.Context, userID uuid.UUID, bundle models.Bundle) (*models.BundleImportResult, error) {
	if s.importSvc == nil {
		return nil, fmt.Errorf("sync.ImportBundleJSON: import service not configured")
	}

	jobID := uuid.New()
	if err := s.insertSyncJob(ctx, models.SyncJob{
		ID:        jobID,
		UserID:    userID,
		Direction: models.SyncJobDirectionImport,
		Transport: models.SyncJobTransportJSON,
		Status:    models.SyncJobStatusRunning,
		Source:    bundle.Source,
		Mode:      normalizeBundleMode(bundle.Mode),
		Summary:   syncSummaryFromBundleStats(bundle.Stats),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		return nil, err
	}

	result, err := s.importSvc.ImportBundle(ctx, userID, bundle)
	if err != nil {
		_ = s.finishSyncJob(ctx, jobID, userID, models.SyncJobStatusFailed, models.SyncJobSummary{}, err.Error())
		return nil, err
	}
	if err := s.finishSyncJob(ctx, jobID, userID, models.SyncJobStatusSucceeded, syncSummaryFromImportResult(result), ""); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SyncService) ExportBundleJSON(ctx context.Context, userID uuid.UUID, filters models.BundleFilters) (*models.Bundle, error) {
	if s.repo != nil {
		return s.repo.ExportBundleJSON(ctx, userID, filters)
	}
	bundle, err := s.exportBundle(ctx, userID, filters)
	if err != nil {
		return nil, err
	}
	jobID := uuid.New()
	job := models.SyncJob{
		ID:        jobID,
		UserID:    userID,
		Direction: models.SyncJobDirectionExport,
		Transport: models.SyncJobTransportJSON,
		Status:    models.SyncJobStatusSucceeded,
		Source:    "neudrive",
		Mode:      bundleModeMerge,
		Filters:   filters,
		Summary:   syncSummaryFromBundleStats(bundle.Stats),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	now := time.Now().UTC()
	job.CompletedAt = &now
	if err := s.insertSyncJob(ctx, job); err != nil {
		return nil, err
	}
	return bundle, nil
}

func (s *SyncService) ExportArchive(ctx context.Context, userID uuid.UUID, filters models.BundleFilters) ([]byte, *models.BundleArchiveManifest, error) {
	if s.repo != nil {
		return s.repo.ExportArchive(ctx, userID, filters)
	}
	bundle, err := s.exportBundle(ctx, userID, filters)
	if err != nil {
		return nil, nil, err
	}
	archive, manifest, err := BuildBundleArchive(*bundle, filters)
	if err != nil {
		return nil, nil, err
	}
	jobID := uuid.New()
	job := models.SyncJob{
		ID:        jobID,
		UserID:    userID,
		Direction: models.SyncJobDirectionExport,
		Transport: models.SyncJobTransportArchive,
		Status:    models.SyncJobStatusSucceeded,
		Source:    "neudrive",
		Mode:      manifest.Mode,
		Filters:   filters,
		Summary:   syncSummaryFromBundleStats(bundle.Stats),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	now := time.Now().UTC()
	job.CompletedAt = &now
	if err := s.insertSyncJob(ctx, job); err != nil {
		return nil, nil, err
	}
	return archive, manifest, nil
}

func (s *SyncService) StartSession(ctx context.Context, userID uuid.UUID, req models.SyncStartSessionRequest) (*models.SyncSessionResponse, error) {
	if s.repo != nil {
		return s.repo.StartSession(ctx, userID, req)
	}
	if s.db == nil {
		return nil, fmt.Errorf("sync.StartSession: database not configured")
	}
	if req.TransportVersion == "" {
		req.TransportVersion = models.SyncTransportVersionV1
	}
	if req.TransportVersion != models.SyncTransportVersionV1 {
		return nil, fmt.Errorf("sync.StartSession: unsupported transport version %q", req.TransportVersion)
	}
	if req.Format == "" {
		req.Format = models.BundleFormatArchive
	}
	if req.Format != models.BundleFormatArchive {
		return nil, fmt.Errorf("sync.StartSession: unsupported format %q", req.Format)
	}
	if req.ArchiveSizeBytes <= 0 {
		return nil, fmt.Errorf("sync.StartSession: archive_size_bytes must be greater than zero")
	}
	if strings.TrimSpace(req.ArchiveSHA256) == "" {
		return nil, fmt.Errorf("sync.StartSession: archive_sha256 is required")
	}
	if req.Manifest.Version != models.BundleVersionV2 {
		return nil, fmt.Errorf("sync.StartSession: manifest version must be %q", models.BundleVersionV2)
	}

	mode := normalizeBundleMode(req.Mode)
	if mode == "" {
		mode = normalizeBundleMode(req.Manifest.Mode)
	}
	if mode == "" {
		return nil, fmt.Errorf("sync.StartSession: invalid mode %q", req.Mode)
	}
	req.Manifest.Mode = mode
	if strings.TrimSpace(req.Manifest.ArchiveSHA256) == "" {
		req.Manifest.ArchiveSHA256 = req.ArchiveSHA256
	}
	if !strings.EqualFold(req.Manifest.ArchiveSHA256, req.ArchiveSHA256) {
		return nil, fmt.Errorf("sync.StartSession: manifest archive_sha256 does not match request")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(24 * time.Hour)
	sessionID := uuid.New()
	jobID := uuid.New()
	totalParts := int((req.ArchiveSizeBytes + s.chunkSize - 1) / s.chunkSize)
	summary := syncSummaryFromBundleStats(req.Manifest.Stats)

	manifestJSON, err := json.Marshal(req.Manifest)
	if err != nil {
		return nil, fmt.Errorf("sync.StartSession: marshal manifest: %w", err)
	}
	filtersJSON, err := json.Marshal(req.Manifest.Filters)
	if err != nil {
		return nil, fmt.Errorf("sync.StartSession: marshal filters: %w", err)
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return nil, fmt.Errorf("sync.StartSession: marshal summary: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("sync.StartSession: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO sync_jobs (
			id, user_id, session_id, direction, transport, status, source, mode,
			filters, summary, error, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, '', $11, $11)`,
		jobID, userID, sessionID, models.SyncJobDirectionImport, models.SyncJobTransportArchive,
		models.SyncJobStatusRunning, req.Manifest.Source, mode, filtersJSON, summaryJSON, now,
	); err != nil {
		return nil, fmt.Errorf("sync.StartSession: insert job: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO sync_sessions (
			id, user_id, job_id, status, format, mode, manifest,
			archive_size_bytes, archive_sha256, chunk_size_bytes, total_parts,
			expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13)`,
		sessionID, userID, jobID, models.SyncSessionStatusUploading, req.Format, mode,
		manifestJSON, req.ArchiveSizeBytes, req.ArchiveSHA256, s.chunkSize, totalParts,
		expiresAt, now,
	); err != nil {
		return nil, fmt.Errorf("sync.StartSession: insert session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("sync.StartSession: commit tx: %w", err)
	}

	return &models.SyncSessionResponse{
		SessionID:      sessionID,
		JobID:          jobID,
		Status:         models.SyncSessionStatusUploading,
		ChunkSizeBytes: s.chunkSize,
		TotalParts:     totalParts,
		ExpiresAt:      expiresAt,
		Mode:           mode,
		Summary:        summary,
		MissingParts:   makePartIndexes(totalParts),
	}, nil
}

func (s *SyncService) UploadPart(ctx context.Context, userID, sessionID uuid.UUID, index int, data []byte) (*models.SyncSessionResponse, error) {
	if s.repo != nil {
		return s.repo.UploadPart(ctx, userID, sessionID, index, data)
	}
	if s.db == nil {
		return nil, fmt.Errorf("sync.UploadPart: database not configured")
	}

	session, received, err := s.loadSessionState(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if err := ensureActiveSession(session); err != nil {
		if errors.Is(err, ErrSyncSessionExpired) {
			_ = s.markSessionExpired(ctx, session)
		}
		return nil, err
	}
	if index < 0 || index >= session.TotalParts {
		return nil, fmt.Errorf("sync.UploadPart: part index %d out of range", index)
	}

	sum := sha256.Sum256(data)
	partHash := hex.EncodeToString(sum[:])
	now := time.Now().UTC()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("sync.UploadPart: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var existingHash string
	var existingSize int64
	err = tx.QueryRow(ctx,
		`SELECT sha256, size_bytes FROM sync_session_parts
		 WHERE session_id = $1 AND part_index = $2`,
		sessionID, index,
	).Scan(&existingHash, &existingSize)
	if err == nil {
		if existingHash != partHash || existingSize != int64(len(data)) {
			return nil, ErrSyncPartConflict
		}
		if _, err := tx.Exec(ctx,
			`UPDATE sync_session_parts SET updated_at = $3
			 WHERE session_id = $1 AND part_index = $2`,
			sessionID, index, now,
		); err != nil {
			return nil, fmt.Errorf("sync.UploadPart: touch part: %w", err)
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		if _, err := tx.Exec(ctx,
			`INSERT INTO sync_session_parts (
				session_id, part_index, sha256, size_bytes, data, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $6)`,
			sessionID, index, partHash, len(data), data, now,
		); err != nil {
			return nil, fmt.Errorf("sync.UploadPart: insert part: %w", err)
		}
		received[index] = struct{}{}
	} else {
		return nil, fmt.Errorf("sync.UploadPart: load part: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("sync.UploadPart: commit part: %w", err)
	}

	session, received, err = s.loadSessionState(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}

	status := models.SyncSessionStatusUploading
	if len(received) == session.TotalParts {
		status = models.SyncSessionStatusReady
	}
	if session.Status != status {
		if err := s.updateSessionStatus(ctx, session.ID, userID, status, nil); err != nil {
			return nil, err
		}
		session.Status = status
	}

	return syncSessionResponse(session, received), nil
}

func (s *SyncService) GetSession(ctx context.Context, userID, sessionID uuid.UUID) (*models.SyncSessionResponse, error) {
	if s.repo != nil {
		return s.repo.GetSession(ctx, userID, sessionID)
	}
	session, received, err := s.loadSessionState(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != models.SyncSessionStatusCommitted && time.Now().UTC().After(session.ExpiresAt) {
		if err := s.markSessionExpired(ctx, session); err == nil {
			session.Status = models.SyncSessionStatusExpired
		}
	}
	return syncSessionResponse(session, received), nil
}

func (s *SyncService) AbortSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	if s.repo != nil {
		return s.repo.AbortSession(ctx, userID, sessionID)
	}
	session, _, err := s.loadSessionState(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("sync.AbortSession: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE sync_sessions
		 SET status = $3, updated_at = $4
		 WHERE id = $1 AND user_id = $2`,
		sessionID, userID, models.SyncSessionStatusAborted, now,
	); err != nil {
		return fmt.Errorf("sync.AbortSession: update session: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM sync_session_parts WHERE session_id = $1`,
		sessionID,
	); err != nil {
		return fmt.Errorf("sync.AbortSession: delete parts: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE sync_jobs
		 SET status = $3, updated_at = $4, completed_at = $4
		 WHERE id = $1 AND user_id = $2`,
		session.JobID, userID, models.SyncJobStatusAborted, now,
	); err != nil {
		return fmt.Errorf("sync.AbortSession: update job: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("sync.AbortSession: commit tx: %w", err)
	}
	return nil
}

func (s *SyncService) CommitSession(ctx context.Context, userID, sessionID uuid.UUID, req models.SyncCommitRequest) (*models.BundleImportResult, error) {
	if s.repo != nil {
		return s.repo.CommitSession(ctx, userID, sessionID, req)
	}
	if s.db == nil {
		return nil, fmt.Errorf("sync.CommitSession: database not configured")
	}
	if s.importSvc == nil {
		return nil, fmt.Errorf("sync.CommitSession: import service not configured")
	}

	session, received, err := s.loadSessionState(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if err := ensureActiveSession(session); err != nil {
		if errors.Is(err, ErrSyncSessionExpired) {
			_ = s.markSessionExpired(ctx, session)
		}
		return nil, err
	}
	if len(received) != session.TotalParts {
		return nil, ErrSyncSessionIncomplete
	}

	archiveData, err := s.loadArchiveData(ctx, sessionID, session.TotalParts)
	if err != nil {
		return nil, err
	}
	bundle, manifest, err := ParseBundleArchive(archiveData)
	if err != nil {
		_ = s.finishSyncJob(ctx, session.JobID, userID, models.SyncJobStatusFailed, models.SyncJobSummary{}, err.Error())
		return nil, err
	}
	if !strings.EqualFold(manifest.ArchiveSHA256, session.ArchiveSHA256) {
		err := fmt.Errorf("sync.CommitSession: archive sha mismatch")
		_ = s.finishSyncJob(ctx, session.JobID, userID, models.SyncJobStatusFailed, models.SyncJobSummary{}, err.Error())
		return nil, err
	}

	if strings.TrimSpace(req.PreviewFingerprint) != "" {
		preview, err := s.PreviewManifest(ctx, userID, *manifest)
		if err != nil {
			return nil, err
		}
		if preview.Fingerprint != req.PreviewFingerprint {
			return nil, ErrSyncPreviewDrift
		}
	}

	result, err := s.importSvc.ImportBundle(ctx, userID, *bundle)
	if err != nil {
		_ = s.finishSyncJob(ctx, session.JobID, userID, models.SyncJobStatusFailed, models.SyncJobSummary{}, err.Error())
		return nil, err
	}

	now := time.Now().UTC()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("sync.CommitSession: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE sync_sessions
		 SET status = $3, updated_at = $4, committed_at = $4
		 WHERE id = $1 AND user_id = $2`,
		sessionID, userID, models.SyncSessionStatusCommitted, now,
	); err != nil {
		return nil, fmt.Errorf("sync.CommitSession: update session: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM sync_session_parts WHERE session_id = $1`,
		sessionID,
	); err != nil {
		return nil, fmt.Errorf("sync.CommitSession: delete parts: %w", err)
	}

	summaryJSON, err := json.Marshal(syncSummaryFromImportResult(result))
	if err != nil {
		return nil, fmt.Errorf("sync.CommitSession: marshal summary: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE sync_jobs
		 SET status = $3, summary = $4, error = '', updated_at = $5, completed_at = $5
		 WHERE id = $1 AND user_id = $2`,
		session.JobID, userID, models.SyncJobStatusSucceeded, summaryJSON, now,
	); err != nil {
		return nil, fmt.Errorf("sync.CommitSession: update job: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("sync.CommitSession: commit tx: %w", err)
	}
	return result, nil
}

func (s *SyncService) ListJobs(ctx context.Context, userID uuid.UUID) ([]models.SyncJob, error) {
	if s.repo != nil {
		return s.repo.ListJobs(ctx, userID)
	}
	if s.db == nil {
		return nil, fmt.Errorf("sync.ListJobs: database not configured")
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, session_id, direction, transport, status, source, mode,
		        filters, summary, error, created_at, updated_at, completed_at
		 FROM sync_jobs
		 WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("sync.ListJobs: %w", err)
	}
	defer rows.Close()

	var jobs []models.SyncJob
	for rows.Next() {
		job, err := scanSyncJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sync.ListJobs: rows: %w", err)
	}
	return jobs, nil
}

func (s *SyncService) GetJob(ctx context.Context, userID, jobID uuid.UUID) (*models.SyncJob, error) {
	if s.repo != nil {
		return s.repo.GetJob(ctx, userID, jobID)
	}
	if s.db == nil {
		return nil, fmt.Errorf("sync.GetJob: database not configured")
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, session_id, direction, transport, status, source, mode,
		        filters, summary, error, created_at, updated_at, completed_at
		 FROM sync_jobs
		 WHERE id = $1 AND user_id = $2`,
		jobID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("sync.GetJob: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrSyncSessionNotFound
	}
	job, err := scanSyncJob(rows)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *SyncService) CleanExpiredSessions(ctx context.Context) (*SyncCleanupResult, error) {
	if s.repo != nil {
		return s.repo.CleanExpiredSessions(ctx)
	}
	if s.db == nil {
		return nil, fmt.Errorf("sync.CleanExpiredSessions: database not configured")
	}

	now := time.Now().UTC()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("sync.CleanExpiredSessions: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx,
		`SELECT id, user_id, job_id
		 FROM sync_sessions
		 WHERE status NOT IN ($1, $2, $3)
		   AND expires_at < $4`,
		models.SyncSessionStatusCommitted,
		models.SyncSessionStatusAborted,
		models.SyncSessionStatusExpired,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("sync.CleanExpiredSessions: load expired sessions: %w", err)
	}

	type expiredSession struct {
		ID     uuid.UUID
		UserID uuid.UUID
		JobID  uuid.UUID
	}
	var expired []expiredSession
	for rows.Next() {
		var item expiredSession
		if err := rows.Scan(&item.ID, &item.UserID, &item.JobID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("sync.CleanExpiredSessions: scan expired session: %w", err)
		}
		expired = append(expired, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("sync.CleanExpiredSessions: expired rows: %w", err)
	}
	rows.Close()

	result := &SyncCleanupResult{}
	if len(expired) > 0 {
		expiredIDs := make([]uuid.UUID, 0, len(expired))
		jobIDs := make([]uuid.UUID, 0, len(expired))
		for _, item := range expired {
			expiredIDs = append(expiredIDs, item.ID)
			jobIDs = append(jobIDs, item.JobID)
		}
		tag, err := tx.Exec(ctx,
			`UPDATE sync_sessions
			 SET status = $2, updated_at = $3
			 WHERE id = ANY($1::uuid[])`,
			expiredIDs, models.SyncSessionStatusExpired, now,
		)
		if err != nil {
			return nil, fmt.Errorf("sync.CleanExpiredSessions: expire sessions: %w", err)
		}
		result.ExpiredSessions = tag.RowsAffected()
		if _, err := tx.Exec(ctx,
			`UPDATE sync_jobs
			 SET status = $2,
			     error = CASE WHEN error = '' THEN 'sync session expired' ELSE error END,
			     updated_at = $3,
			     completed_at = COALESCE(completed_at, $3)
			 WHERE id = ANY($1::uuid[])
			   AND status = $4`,
			jobIDs, models.SyncJobStatusFailed, now, models.SyncJobStatusRunning,
		); err != nil {
			return nil, fmt.Errorf("sync.CleanExpiredSessions: update jobs: %w", err)
		}
	}

	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(p.size_bytes), 0)
		 FROM sync_session_parts p
		 JOIN sync_sessions s ON s.id = p.session_id
		 WHERE s.status IN ($1, $2, $3)
		    OR s.expires_at < $4`,
		models.SyncSessionStatusCommitted,
		models.SyncSessionStatusAborted,
		models.SyncSessionStatusExpired,
		now,
	).Scan(&result.DeletedParts, &result.DeletedBytes); err != nil {
		return nil, fmt.Errorf("sync.CleanExpiredSessions: aggregate parts: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM sync_session_parts p
		 USING sync_sessions s
		 WHERE p.session_id = s.id
		   AND (s.status IN ($1, $2, $3) OR s.expires_at < $4)`,
		models.SyncSessionStatusCommitted,
		models.SyncSessionStatusAborted,
		models.SyncSessionStatusExpired,
		now,
	); err != nil {
		return nil, fmt.Errorf("sync.CleanExpiredSessions: delete parts: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("sync.CleanExpiredSessions: commit tx: %w", err)
	}
	return result, nil
}

func (s *SyncService) exportBundle(ctx context.Context, userID uuid.UUID, filters models.BundleFilters) (*models.Bundle, error) {
	if s.exportSvc == nil {
		return nil, fmt.Errorf("sync.exportBundle: export service not configured")
	}
	fullBundle, err := s.exportSvc.ExportBundle(ctx, userID)
	if err != nil {
		return nil, err
	}
	filtered := filterBundle(*fullBundle, filters)
	return &filtered, nil
}

func (s *SyncService) previewManifestTextPath(ctx context.Context, userID uuid.UUID, fullPath, desiredSHA, desiredContentType string) (string, error) {
	entry, _, blobExists, err := s.importSvc.loadPreviewEntry(ctx, userID, fullPath)
	if err != nil {
		return "", err
	}
	return previewTextHashAction(entry, blobExists, desiredSHA, desiredContentType), nil
}

func syncSessionResponse(session *models.SyncSession, received map[int]struct{}) *models.SyncSessionResponse {
	receivedParts := make([]int, 0, len(received))
	for idx := range received {
		receivedParts = append(receivedParts, idx)
	}
	sort.Ints(receivedParts)

	missing := make([]int, 0, session.TotalParts-len(receivedParts))
	for idx := 0; idx < session.TotalParts; idx++ {
		if _, ok := received[idx]; !ok {
			missing = append(missing, idx)
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
		Summary:        syncSummaryFromBundleStats(session.Manifest.Stats),
		ReceivedParts:  receivedParts,
		MissingParts:   missing,
	}
}

func (s *SyncService) loadSessionState(ctx context.Context, userID, sessionID uuid.UUID) (*models.SyncSession, map[int]struct{}, error) {
	if s.db == nil {
		return nil, nil, fmt.Errorf("sync.loadSessionState: database not configured")
	}
	var (
		manifestJSON []byte
		session      models.SyncSession
	)
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, job_id, status, format, mode, manifest,
		        archive_size_bytes, archive_sha256, chunk_size_bytes, total_parts,
		        expires_at, created_at, updated_at, committed_at
		 FROM sync_sessions
		 WHERE id = $1 AND user_id = $2`,
		sessionID, userID,
	).Scan(
		&session.ID, &session.UserID, &session.JobID, &session.Status, &session.Format,
		&session.Mode, &manifestJSON, &session.ArchiveSizeBytes, &session.ArchiveSHA256,
		&session.ChunkSizeBytes, &session.TotalParts, &session.ExpiresAt, &session.CreatedAt,
		&session.UpdatedAt, &session.CommittedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrSyncSessionNotFound
		}
		return nil, nil, fmt.Errorf("sync.loadSessionState: %w", err)
	}
	if err := json.Unmarshal(manifestJSON, &session.Manifest); err != nil {
		return nil, nil, fmt.Errorf("sync.loadSessionState: unmarshal manifest: %w", err)
	}

	rows, err := s.db.Query(ctx,
		`SELECT part_index FROM sync_session_parts
		 WHERE session_id = $1
		 ORDER BY part_index ASC`,
		sessionID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("sync.loadSessionState: load parts: %w", err)
	}
	defer rows.Close()
	received := make(map[int]struct{}, session.TotalParts)
	for rows.Next() {
		var idx int
		if err := rows.Scan(&idx); err != nil {
			return nil, nil, fmt.Errorf("sync.loadSessionState: scan part: %w", err)
		}
		received[idx] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("sync.loadSessionState: part rows: %w", err)
	}
	return &session, received, nil
}

func ensureActiveSession(session *models.SyncSession) error {
	if session.Status == models.SyncSessionStatusCommitted {
		return fmt.Errorf("sync session already committed")
	}
	if session.Status == models.SyncSessionStatusAborted {
		return fmt.Errorf("sync session has been aborted")
	}
	if session.Status == models.SyncSessionStatusExpired || time.Now().UTC().After(session.ExpiresAt) {
		return ErrSyncSessionExpired
	}
	return nil
}

func (s *SyncService) markSessionExpired(ctx context.Context, session *models.SyncSession) error {
	if s.db == nil {
		return fmt.Errorf("sync.markSessionExpired: database not configured")
	}
	if session == nil {
		return nil
	}
	if session.Status == models.SyncSessionStatusCommitted || session.Status == models.SyncSessionStatusAborted {
		return nil
	}
	now := time.Now().UTC()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("sync.markSessionExpired: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE sync_sessions
		 SET status = $3, updated_at = $4
		 WHERE id = $1 AND user_id = $2
		   AND status NOT IN ($5, $6)`,
		session.ID, session.UserID, models.SyncSessionStatusExpired, now,
		models.SyncSessionStatusCommitted, models.SyncSessionStatusAborted,
	); err != nil {
		return fmt.Errorf("sync.markSessionExpired: update session: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE sync_jobs
		 SET status = $3,
		     error = CASE WHEN error = '' THEN 'sync session expired' ELSE error END,
		     updated_at = $4,
		     completed_at = COALESCE(completed_at, $4)
		 WHERE id = $1 AND user_id = $2
		   AND status = $5`,
		session.JobID, session.UserID, models.SyncJobStatusFailed, now, models.SyncJobStatusRunning,
	); err != nil {
		return fmt.Errorf("sync.markSessionExpired: update job: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("sync.markSessionExpired: commit tx: %w", err)
	}
	session.Status = models.SyncSessionStatusExpired
	session.UpdatedAt = now
	return nil
}

func (s *SyncService) loadArchiveData(ctx context.Context, sessionID uuid.UUID, totalParts int) ([]byte, error) {
	rows, err := s.db.Query(ctx,
		`SELECT data FROM sync_session_parts
		 WHERE session_id = $1
		 ORDER BY part_index ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("sync.loadArchiveData: %w", err)
	}
	defer rows.Close()

	var buf bytes.Buffer
	count := 0
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("sync.loadArchiveData: scan: %w", err)
		}
		buf.Write(data)
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sync.loadArchiveData: rows: %w", err)
	}
	if count != totalParts {
		return nil, ErrSyncSessionIncomplete
	}
	return buf.Bytes(), nil
}

func (s *SyncService) updateSessionStatus(ctx context.Context, sessionID, userID uuid.UUID, status string, committedAt *time.Time) error {
	if s.db == nil {
		return fmt.Errorf("sync.updateSessionStatus: database not configured")
	}
	now := time.Now().UTC()
	if committedAt != nil {
		_, err := s.db.Exec(ctx,
			`UPDATE sync_sessions
			 SET status = $3, updated_at = $4, committed_at = $5
			 WHERE id = $1 AND user_id = $2`,
			sessionID, userID, status, now, *committedAt,
		)
		if err != nil {
			return fmt.Errorf("sync.updateSessionStatus: %w", err)
		}
		return nil
	}
	_, err := s.db.Exec(ctx,
		`UPDATE sync_sessions
		 SET status = $3, updated_at = $4
		 WHERE id = $1 AND user_id = $2`,
		sessionID, userID, status, now,
	)
	if err != nil {
		return fmt.Errorf("sync.updateSessionStatus: %w", err)
	}
	return nil
}

func (s *SyncService) insertSyncJob(ctx context.Context, job models.SyncJob) error {
	if s.repo != nil {
		return s.repo.InsertJob(ctx, job)
	}
	if s.db == nil {
		return fmt.Errorf("sync.insertSyncJob: database not configured")
	}
	filtersJSON, err := json.Marshal(job.Filters)
	if err != nil {
		return fmt.Errorf("sync.insertSyncJob: marshal filters: %w", err)
	}
	summaryJSON, err := json.Marshal(job.Summary)
	if err != nil {
		return fmt.Errorf("sync.insertSyncJob: marshal summary: %w", err)
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = job.CreatedAt
	}
	_, err = s.db.Exec(ctx,
		`INSERT INTO sync_jobs (
			id, user_id, session_id, direction, transport, status, source, mode,
			filters, summary, error, created_at, updated_at, completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		job.ID, job.UserID, job.SessionID, job.Direction, job.Transport, job.Status,
		job.Source, job.Mode, filtersJSON, summaryJSON, job.Error, job.CreatedAt,
		job.UpdatedAt, job.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("sync.insertSyncJob: %w", err)
	}
	return nil
}

func (s *SyncService) finishSyncJob(ctx context.Context, jobID, userID uuid.UUID, status string, summary models.SyncJobSummary, errorMessage string) error {
	if s.repo != nil {
		return s.repo.FinishJob(ctx, jobID, userID, status, summary, errorMessage)
	}
	if s.db == nil {
		return fmt.Errorf("sync.finishSyncJob: database not configured")
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("sync.finishSyncJob: marshal summary: %w", err)
	}
	now := time.Now().UTC()
	_, err = s.db.Exec(ctx,
		`UPDATE sync_jobs
		 SET status = $3, summary = $4, error = $5, updated_at = $6, completed_at = $6
		 WHERE id = $1 AND user_id = $2`,
		jobID, userID, status, summaryJSON, errorMessage, now,
	)
	if err != nil {
		return fmt.Errorf("sync.finishSyncJob: %w", err)
	}
	return nil
}

func scanSyncJob(rows pgx.Rows) (models.SyncJob, error) {
	var (
		job         models.SyncJob
		sessionID   *uuid.UUID
		filtersJSON []byte
		summaryJSON []byte
	)
	if err := rows.Scan(
		&job.ID, &job.UserID, &sessionID, &job.Direction, &job.Transport, &job.Status,
		&job.Source, &job.Mode, &filtersJSON, &summaryJSON, &job.Error,
		&job.CreatedAt, &job.UpdatedAt, &job.CompletedAt,
	); err != nil {
		return models.SyncJob{}, fmt.Errorf("sync.scanSyncJob: scan: %w", err)
	}
	job.SessionID = sessionID
	if len(filtersJSON) > 0 {
		if err := json.Unmarshal(filtersJSON, &job.Filters); err != nil {
			return models.SyncJob{}, fmt.Errorf("sync.scanSyncJob: filters: %w", err)
		}
	}
	if len(summaryJSON) > 0 {
		if err := json.Unmarshal(summaryJSON, &job.Summary); err != nil {
			return models.SyncJob{}, fmt.Errorf("sync.scanSyncJob: summary: %w", err)
		}
	}
	return job, nil
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

func previewTextHashAction(entry *models.FileTreeEntry, blobExists bool, desiredSHA, desiredContentType string) string {
	if entry == nil {
		return "create"
	}
	if blobExists {
		return "update"
	}
	currentSHA := sha256Hex([]byte(entry.Content))
	if currentSHA == desiredSHA && entry.ContentType == desiredContentType {
		return "skip"
	}
	return "update"
}

func previewBinaryHashAction(entry *models.FileTreeEntry, blob []byte, blobExists bool, desiredSHA, desiredContentType string) string {
	if entry == nil {
		return "create"
	}
	if !blobExists {
		return "update"
	}
	if sha256Hex(blob) == desiredSHA && entry.ContentType == desiredContentType {
		return "skip"
	}
	return "update"
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func bundlePreviewFingerprint(preview *models.BundlePreviewResult) string {
	if preview == nil {
		return ""
	}
	var builder strings.Builder
	builder.WriteString(preview.Version)
	builder.WriteString("|")
	builder.WriteString(preview.Mode)
	builder.WriteString("|")
	writePreviewEntries(&builder, preview.Profile)
	builder.WriteString("|")
	writePreviewEntries(&builder, preview.Memory)
	builder.WriteString("|")
	skillNames := make([]string, 0, len(preview.Skills))
	for name := range preview.Skills {
		skillNames = append(skillNames, name)
	}
	sort.Strings(skillNames)
	for _, name := range skillNames {
		builder.WriteString(name)
		builder.WriteString(":")
		writePreviewEntries(&builder, preview.Skills[name].Files)
		builder.WriteString(";")
	}
	return sha256Hex([]byte(builder.String()))
}

func writePreviewEntries(builder *strings.Builder, entries []models.BundlePreviewEntry) {
	for _, entry := range entries {
		builder.WriteString(entry.Path)
		builder.WriteString("|")
		builder.WriteString(entry.Action)
		builder.WriteString("|")
		builder.WriteString(entry.Kind)
		builder.WriteString(";")
	}
}

func filterBundle(bundle models.Bundle, filters models.BundleFilters) models.Bundle {
	domains := normalizeDomains(filters.IncludeDomains)
	includeSkills := normalizedSkillSet(filters.IncludeSkills)
	excludeSkills := normalizedSkillSet(filters.ExcludeSkills)

	filtered := models.Bundle{
		Version:   bundle.Version,
		CreatedAt: bundle.CreatedAt,
		Source:    bundle.Source,
		Mode:      bundle.Mode,
		Profile:   map[string]string{},
		Skills:    map[string]models.BundleSkill{},
		Memory:    []models.BundleMemoryItem{},
	}

	if domains["profile"] {
		for category, content := range bundle.Profile {
			filtered.Profile[category] = content
		}
	}
	if domains["memory"] {
		filtered.Memory = append(filtered.Memory, bundle.Memory...)
	}
	if domains["skills"] {
		for skillName, skill := range bundle.Skills {
			if systemskills.IsProtectedPath(path.Join("/skills", skillName, "SKILL.md")) {
				continue
			}
			if len(includeSkills) > 0 {
				if _, ok := includeSkills[skillName]; !ok {
					continue
				}
			}
			if _, excluded := excludeSkills[skillName]; excluded {
				continue
			}
			filtered.Skills[skillName] = cloneBundleSkill(skill)
		}
	}

	filtered.Stats = recalculateBundleStats(filtered)
	return filtered
}

func cloneBundleSkill(skill models.BundleSkill) models.BundleSkill {
	cloned := models.BundleSkill{
		Files:       map[string]string{},
		BinaryFiles: map[string]models.BundleBlobFile{},
	}
	for relPath, content := range skill.Files {
		cloned.Files[relPath] = content
	}
	for relPath, blob := range skill.BinaryFiles {
		cloned.BinaryFiles[relPath] = blob
	}
	return cloned
}

func recalculateBundleStats(bundle models.Bundle) models.BundleStats {
	stats := models.BundleStats{
		TotalSkills:  len(bundle.Skills),
		ProfileItems: len(bundle.Profile),
		MemoryItems:  len(bundle.Memory),
	}
	for _, content := range bundle.Profile {
		stats.TotalBytes += int64(len(content))
	}
	for _, item := range bundle.Memory {
		stats.TotalBytes += int64(len(item.Content))
	}
	for _, skill := range bundle.Skills {
		for _, content := range skill.Files {
			stats.TotalFiles++
			stats.TotalBytes += int64(len(content))
		}
		for _, blob := range skill.BinaryFiles {
			stats.TotalFiles++
			stats.BinaryFiles++
			stats.TotalBytes += blob.SizeBytes
		}
	}
	return stats
}

func normalizeDomains(domains []string) map[string]bool {
	normalized := map[string]bool{
		"profile": true,
		"memory":  true,
		"skills":  true,
	}
	if len(domains) == 0 {
		return normalized
	}
	normalized = map[string]bool{}
	for _, domain := range domains {
		switch strings.ToLower(strings.TrimSpace(domain)) {
		case "profile", "memory", "skills":
			normalized[strings.ToLower(strings.TrimSpace(domain))] = true
		}
	}
	return normalized
}

func normalizedSkillSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name != "" {
			result[name] = struct{}{}
		}
	}
	return result
}

func makePartIndexes(total int) []int {
	values := make([]int, total)
	for i := 0; i < total; i++ {
		values[i] = i
	}
	return values
}

func BuildBundleArchive(bundle models.Bundle, filters models.BundleFilters) ([]byte, *models.BundleArchiveManifest, error) {
	filtered := filterBundle(bundle, filters)

	payloads := make(map[string][]byte)
	manifest := &models.BundleArchiveManifest{
		Version:      models.BundleVersionV2,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		Source:       filtered.Source,
		Mode:         normalizeBundleMode(filtered.Mode),
		Domains:      manifestDomains(filtered),
		Filters:      filters,
		ProfileFiles: map[string]models.BundleArchiveEntry{},
		MemoryItems:  []models.BundleArchiveMemoryItem{},
		SkillFiles:   map[string]map[string]models.BundleArchiveEntry{},
		Stats:        filtered.Stats,
	}
	if manifest.Mode == "" {
		manifest.Mode = bundleModeMerge
	}

	profileKeys := sortedStringKeys(filtered.Profile)
	for _, category := range profileKeys {
		archivePath := path.Join("payload", "profile", category+".md")
		data := []byte(filtered.Profile[category])
		payloads[archivePath] = data
		manifest.ProfileFiles[category] = archiveEntryForPayload(archivePath, false, "text/markdown", data)
	}

	for _, item := range filtered.Memory {
		createdAt, expiresAt, err := parseBundleMemoryTimes(item)
		if err != nil {
			return nil, nil, err
		}
		memoryID := importedScratchLegacyID(item.Source, item.Title, createdAt).String()
		archivePath := path.Join("payload", "memory", memoryID+".md")
		data := []byte(item.Content)
		payloads[archivePath] = data
		manifestItem := models.BundleArchiveMemoryItem{
			ID:          memoryID,
			Title:       item.Title,
			Source:      item.Source,
			CreatedAt:   createdAt.UTC().Format(time.RFC3339),
			ArchivePath: archivePath,
			ContentType: "text/markdown",
			SizeBytes:   int64(len(data)),
			SHA256:      sha256Hex(data),
		}
		if expiresAt != nil {
			manifestItem.ExpiresAt = expiresAt.UTC().Format(time.RFC3339)
		}
		manifest.MemoryItems = append(manifest.MemoryItems, manifestItem)
	}

	skillNames := make([]string, 0, len(filtered.Skills))
	for skillName := range filtered.Skills {
		skillNames = append(skillNames, skillName)
	}
	sort.Strings(skillNames)
	for _, skillName := range skillNames {
		skill := filtered.Skills[skillName]
		manifest.SkillFiles[skillName] = map[string]models.BundleArchiveEntry{}

		textPaths := sortedStringKeys(skill.Files)
		for _, relPath := range textPaths {
			archivePath := path.Join("payload", "skills", skillName, relPath)
			data := []byte(skill.Files[relPath])
			payloads[archivePath] = data
			manifest.SkillFiles[skillName][relPath] = archiveEntryForPayload(archivePath, false, contentTypeFromExt(relPath), data)
		}

		binaryKeys := make([]string, 0, len(skill.BinaryFiles))
		for relPath := range skill.BinaryFiles {
			binaryKeys = append(binaryKeys, relPath)
		}
		sort.Strings(binaryKeys)
		for _, relPath := range binaryKeys {
			blob := skill.BinaryFiles[relPath]
			data, contentType, err := decodeBundleBlob(relPath, blob)
			if err != nil {
				return nil, nil, err
			}
			archivePath := path.Join("payload", "skills", skillName, relPath)
			payloads[archivePath] = data
			manifest.SkillFiles[skillName][relPath] = archiveEntryForPayload(archivePath, true, contentType, data)
		}
	}

	manifest.ArchiveSHA256 = archiveManifestHash(*manifest)

	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("sync.BuildBundleArchive: marshal manifest: %w", err)
	}
	if err := writeArchiveFile(zipWriter, "manifest.json", manifestBytes); err != nil {
		return nil, nil, err
	}

	payloadPaths := make([]string, 0, len(payloads))
	for archivePath := range payloads {
		payloadPaths = append(payloadPaths, archivePath)
	}
	sort.Strings(payloadPaths)
	for _, archivePath := range payloadPaths {
		if err := writeArchiveFile(zipWriter, archivePath, payloads[archivePath]); err != nil {
			return nil, nil, err
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, nil, fmt.Errorf("sync.BuildBundleArchive: close zip: %w", err)
	}
	return buffer.Bytes(), manifest, nil
}

func ParseBundleArchive(data []byte) (*models.Bundle, *models.BundleArchiveManifest, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("sync.ParseBundleArchive: open zip: %w", err)
	}

	var manifest models.BundleArchiveManifest
	payloads := make(map[string][]byte)
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			return nil, nil, fmt.Errorf("sync.ParseBundleArchive: open %s: %w", file.Name, err)
		}
		content, readErr := io.ReadAll(rc)
		rc.Close()
		if readErr != nil {
			return nil, nil, fmt.Errorf("sync.ParseBundleArchive: read %s: %w", file.Name, readErr)
		}
		if file.Name == "manifest.json" {
			if err := json.Unmarshal(content, &manifest); err != nil {
				return nil, nil, fmt.Errorf("sync.ParseBundleArchive: decode manifest: %w", err)
			}
			continue
		}
		payloads[file.Name] = content
	}
	if manifest.Version != models.BundleVersionV2 {
		return nil, nil, fmt.Errorf("sync.ParseBundleArchive: unsupported manifest version %q", manifest.Version)
	}
	if got := archiveManifestHash(manifest); got != manifest.ArchiveSHA256 {
		return nil, nil, fmt.Errorf("sync.ParseBundleArchive: archive sha mismatch")
	}

	bundle := &models.Bundle{
		Version:   models.BundleVersionV1,
		CreatedAt: manifest.CreatedAt,
		Source:    manifest.Source,
		Mode:      manifest.Mode,
		Profile:   map[string]string{},
		Skills:    map[string]models.BundleSkill{},
		Memory:    []models.BundleMemoryItem{},
		Stats:     manifest.Stats,
	}

	profileKeys := make([]string, 0, len(manifest.ProfileFiles))
	for category := range manifest.ProfileFiles {
		profileKeys = append(profileKeys, category)
	}
	sort.Strings(profileKeys)
	for _, category := range profileKeys {
		entry := manifest.ProfileFiles[category]
		content, ok := payloads[entry.ArchivePath]
		if !ok {
			return nil, nil, fmt.Errorf("sync.ParseBundleArchive: missing payload %q", entry.ArchivePath)
		}
		if err := validateArchivePayload(entry, content); err != nil {
			return nil, nil, err
		}
		bundle.Profile[category] = string(content)
	}

	for _, item := range manifest.MemoryItems {
		content, ok := payloads[item.ArchivePath]
		if !ok {
			return nil, nil, fmt.Errorf("sync.ParseBundleArchive: missing payload %q", item.ArchivePath)
		}
		if err := validateArchivePayload(models.BundleArchiveEntry{
			ArchivePath: item.ArchivePath,
			ContentType: item.ContentType,
			SizeBytes:   item.SizeBytes,
			SHA256:      item.SHA256,
		}, content); err != nil {
			return nil, nil, err
		}
		bundle.Memory = append(bundle.Memory, models.BundleMemoryItem{
			Content:   string(content),
			Title:     item.Title,
			Source:    item.Source,
			CreatedAt: item.CreatedAt,
			ExpiresAt: item.ExpiresAt,
		})
	}

	skillNames := make([]string, 0, len(manifest.SkillFiles))
	for skillName := range manifest.SkillFiles {
		skillNames = append(skillNames, skillName)
	}
	sort.Strings(skillNames)
	for _, skillName := range skillNames {
		skill := models.BundleSkill{
			Files:       map[string]string{},
			BinaryFiles: map[string]models.BundleBlobFile{},
		}
		relPaths := make([]string, 0, len(manifest.SkillFiles[skillName]))
		for relPath := range manifest.SkillFiles[skillName] {
			relPaths = append(relPaths, relPath)
		}
		sort.Strings(relPaths)
		for _, relPath := range relPaths {
			entry := manifest.SkillFiles[skillName][relPath]
			content, ok := payloads[entry.ArchivePath]
			if !ok {
				return nil, nil, fmt.Errorf("sync.ParseBundleArchive: missing payload %q", entry.ArchivePath)
			}
			if err := validateArchivePayload(entry, content); err != nil {
				return nil, nil, err
			}
			if entry.Binary {
				skill.BinaryFiles[relPath] = models.BundleBlobFile{
					ContentBase64: base64.StdEncoding.EncodeToString(content),
					ContentType:   entry.ContentType,
					SizeBytes:     int64(len(content)),
					SHA256:        entry.SHA256,
				}
			} else {
				skill.Files[relPath] = string(content)
			}
		}
		bundle.Skills[skillName] = skill
	}
	return bundle, &manifest, nil
}

func archiveEntryForPayload(archivePath string, binary bool, contentType string, data []byte) models.BundleArchiveEntry {
	return models.BundleArchiveEntry{
		ArchivePath: archivePath,
		Binary:      binary,
		ContentType: contentType,
		SizeBytes:   int64(len(data)),
		SHA256:      sha256Hex(data),
	}
}

func writeArchiveFile(zipWriter *zip.Writer, archivePath string, data []byte) error {
	writer, err := zipWriter.Create(archivePath)
	if err != nil {
		return fmt.Errorf("sync.writeArchiveFile: create %s: %w", archivePath, err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("sync.writeArchiveFile: write %s: %w", archivePath, err)
	}
	return nil
}

func validateArchivePayload(entry models.BundleArchiveEntry, content []byte) error {
	if entry.SizeBytes > 0 && int64(len(content)) != entry.SizeBytes {
		return fmt.Errorf("sync.validateArchivePayload: size mismatch for %s", entry.ArchivePath)
	}
	if entry.SHA256 != "" && sha256Hex(content) != entry.SHA256 {
		return fmt.Errorf("sync.validateArchivePayload: sha mismatch for %s", entry.ArchivePath)
	}
	return nil
}

func manifestDomains(bundle models.Bundle) []string {
	domains := []string{}
	if len(bundle.Profile) > 0 {
		domains = append(domains, "profile")
	}
	if len(bundle.Memory) > 0 {
		domains = append(domains, "memory")
	}
	if len(bundle.Skills) > 0 {
		domains = append(domains, "skills")
	}
	return domains
}

func archiveManifestHash(manifest models.BundleArchiveManifest) string {
	clean := manifest
	clean.ArchiveSHA256 = ""
	var builder strings.Builder
	builder.WriteString(clean.Version)
	builder.WriteString("|")
	builder.WriteString(clean.CreatedAt)
	builder.WriteString("|")
	builder.WriteString(clean.Source)
	builder.WriteString("|")
	builder.WriteString(clean.Mode)
	builder.WriteString("|")

	domains := append([]string(nil), clean.Domains...)
	sort.Strings(domains)
	for _, domain := range domains {
		builder.WriteString(domain)
		builder.WriteString(",")
	}
	builder.WriteString("|")

	writeStringSlice(&builder, clean.Filters.IncludeDomains)
	builder.WriteString("|")
	writeStringSlice(&builder, clean.Filters.IncludeSkills)
	builder.WriteString("|")
	writeStringSlice(&builder, clean.Filters.ExcludeSkills)
	builder.WriteString("|")

	profileKeys := make([]string, 0, len(clean.ProfileFiles))
	for category := range clean.ProfileFiles {
		profileKeys = append(profileKeys, category)
	}
	sort.Strings(profileKeys)
	for _, category := range profileKeys {
		builder.WriteString(category)
		builder.WriteString("=")
		writeArchiveEntryHash(&builder, clean.ProfileFiles[category])
		builder.WriteString(";")
	}
	builder.WriteString("|")

	memoryItems := append([]models.BundleArchiveMemoryItem(nil), clean.MemoryItems...)
	sort.Slice(memoryItems, func(i, j int) bool { return memoryItems[i].ID < memoryItems[j].ID })
	for _, item := range memoryItems {
		builder.WriteString(item.ID)
		builder.WriteString("|")
		builder.WriteString(item.Title)
		builder.WriteString("|")
		builder.WriteString(item.Source)
		builder.WriteString("|")
		builder.WriteString(item.CreatedAt)
		builder.WriteString("|")
		builder.WriteString(item.ExpiresAt)
		builder.WriteString("|")
		builder.WriteString(item.ArchivePath)
		builder.WriteString("|")
		builder.WriteString(item.ContentType)
		builder.WriteString("|")
		builder.WriteString(fmt.Sprintf("%d", item.SizeBytes))
		builder.WriteString("|")
		builder.WriteString(item.SHA256)
		builder.WriteString(";")
	}
	builder.WriteString("|")

	skillNames := make([]string, 0, len(clean.SkillFiles))
	for skillName := range clean.SkillFiles {
		skillNames = append(skillNames, skillName)
	}
	sort.Strings(skillNames)
	for _, skillName := range skillNames {
		builder.WriteString(skillName)
		builder.WriteString("{")
		relPaths := make([]string, 0, len(clean.SkillFiles[skillName]))
		for relPath := range clean.SkillFiles[skillName] {
			relPaths = append(relPaths, relPath)
		}
		sort.Strings(relPaths)
		for _, relPath := range relPaths {
			builder.WriteString(relPath)
			builder.WriteString("=")
			writeArchiveEntryHash(&builder, clean.SkillFiles[skillName][relPath])
			builder.WriteString(";")
		}
		builder.WriteString("}")
	}
	builder.WriteString("|")
	builder.WriteString(fmt.Sprintf("%d|%d|%d|%d|%d|%d",
		clean.Stats.TotalSkills,
		clean.Stats.TotalFiles,
		clean.Stats.TotalBytes,
		clean.Stats.BinaryFiles,
		clean.Stats.ProfileItems,
		clean.Stats.MemoryItems,
	))
	return sha256Hex([]byte(builder.String()))
}

func writeArchiveEntryHash(builder *strings.Builder, entry models.BundleArchiveEntry) {
	builder.WriteString(entry.ArchivePath)
	builder.WriteString("|")
	if entry.Binary {
		builder.WriteString("1")
	} else {
		builder.WriteString("0")
	}
	builder.WriteString("|")
	builder.WriteString(entry.ContentType)
	builder.WriteString("|")
	builder.WriteString(fmt.Sprintf("%d", entry.SizeBytes))
	builder.WriteString("|")
	builder.WriteString(entry.SHA256)
}

func writeStringSlice(builder *strings.Builder, values []string) {
	cloned := append([]string(nil), values...)
	sort.Strings(cloned)
	for _, value := range cloned {
		builder.WriteString(strings.TrimSpace(value))
		builder.WriteString(",")
	}
}
