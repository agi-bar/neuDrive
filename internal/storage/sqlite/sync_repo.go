package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
	"github.com/agi-bar/neudrive/internal/systemskills"
	"github.com/google/uuid"
)

type SyncRepo struct {
	Store *Store
}

func NewSyncRepo(store *Store) services.SyncRepo {
	return &SyncRepo{Store: store}
}

func (r *SyncRepo) newBundleServices() (*services.FileTreeService, *services.MemoryService, *services.ImportService, *services.ExportService, *services.SyncService) {
	fileTree := services.NewFileTreeServiceWithRepo(NewFileTreeRepo(r.Store))
	memory := services.NewMemoryServiceWithRepo(NewMemoryRepo(r.Store), fileTree)
	project := services.NewProjectServiceWithRepo(NewProjectRepo(r.Store), nil, fileTree)
	importSvc := services.NewImportService(nil, fileTree, memory, nil)
	exportSvc := services.NewExportService(fileTree, memory, project, nil, nil, nil, nil)
	syncSvc := services.NewSyncService(nil, importSvc, exportSvc, fileTree, memory)
	return fileTree, memory, importSvc, exportSvc, syncSvc
}

func (r *SyncRepo) ExportBundleJSON(ctx context.Context, userID uuid.UUID, filters models.BundleFilters) (*models.Bundle, error) {
	profiles, err := r.Store.GetProfiles(ctx, userID)
	if err != nil {
		return nil, err
	}
	scratch, err := r.Store.GetScratchActive(ctx, userID)
	if err != nil {
		return nil, err
	}
	snapshot, err := r.Store.Snapshot(ctx, userID, "/skills", models.TrustLevelFull)
	if err != nil && err != services.ErrEntryNotFound {
		return nil, err
	}

	bundle := &models.Bundle{
		Version:   models.BundleVersionV1,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Source:    "neudrive-local",
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
				data, ok, err := r.Store.ReadBlobByEntryID(ctx, entry.ID)
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

func (r *SyncRepo) ExportArchive(ctx context.Context, userID uuid.UUID, filters models.BundleFilters) ([]byte, *models.BundleArchiveManifest, error) {
	bundle, err := r.ExportBundleJSON(ctx, userID, filters)
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
		Source:      "neudrive-local",
		Mode:        manifest.Mode,
		Filters:     filters,
		Summary:     syncSummaryFromBundleStats(bundle.Stats),
		CreatedAt:   now,
		UpdatedAt:   now,
		CompletedAt: &now,
	}
	if err := r.insertSyncJob(ctx, job); err != nil {
		return nil, nil, err
	}
	return archive, manifest, nil
}

func (r *SyncRepo) InsertJob(ctx context.Context, job models.SyncJob) error {
	return r.insertSyncJob(ctx, job)
}

func (r *SyncRepo) FinishJob(ctx context.Context, jobID, userID uuid.UUID, status string, summary models.SyncJobSummary, errorMessage string) error {
	return r.finishSyncJob(ctx, jobID, userID, status, summary, errorMessage)
}

func (r *SyncRepo) StartSession(ctx context.Context, userID uuid.UUID, req models.SyncStartSessionRequest) (*models.SyncSessionResponse, error) {
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

	db := r.Store.DB()
	if _, err := db.ExecContext(ctx,
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
	if err := r.insertSyncJob(ctx, models.SyncJob{
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
	return r.sessionResponse(ctx, session)
}

func (r *SyncRepo) UploadPart(ctx context.Context, userID, sessionID uuid.UUID, index int, data []byte) (*models.SyncSessionResponse, error) {
	session, err := r.loadSession(ctx, userID, sessionID)
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
	db := r.Store.DB()

	row := db.QueryRowContext(ctx, `SELECT part_hash FROM sync_session_parts WHERE session_id = ? AND part_index = ?`, sessionID.String(), index)
	var existingHash string
	if err := row.Scan(&existingHash); err == nil {
		if existingHash != partHash {
			return nil, services.ErrSyncPartConflict
		}
		_, _ = db.ExecContext(ctx, `UPDATE sync_session_parts SET updated_at = ? WHERE session_id = ? AND part_index = ?`, timeText(time.Now().UTC()), sessionID.String(), index)
		return r.sessionResponse(ctx, *session)
	}

	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO sync_session_parts (session_id, part_index, part_hash, data, size_bytes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sessionID.String(), index, partHash, data, len(data), timeText(now), timeText(now),
	); err != nil {
		return nil, err
	}
	received, _ := r.receivedParts(ctx, sessionID)
	status := models.SyncSessionStatusUploading
	if len(received) == session.TotalParts {
		status = models.SyncSessionStatusReady
	}
	_, _ = db.ExecContext(ctx, `UPDATE sync_sessions SET status = ?, updated_at = ? WHERE id = ?`, status, timeText(now), sessionID.String())
	session.Status = status
	session.UpdatedAt = now
	return r.sessionResponse(ctx, *session)
}

func (r *SyncRepo) GetSession(ctx context.Context, userID, sessionID uuid.UUID) (*models.SyncSessionResponse, error) {
	session, err := r.loadSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	return r.sessionResponse(ctx, *session)
}

func (r *SyncRepo) AbortSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	session, err := r.loadSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	db := r.Store.DB()
	if _, err := db.ExecContext(ctx, `UPDATE sync_sessions SET status = ?, updated_at = ? WHERE id = ?`, models.SyncSessionStatusAborted, timeText(now), sessionID.String()); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM sync_session_parts WHERE session_id = ?`, sessionID.String()); err != nil {
		return err
	}
	return r.finishSyncJob(ctx, session.JobID, userID, models.SyncJobStatusAborted, sessionSummary(session.Manifest.Stats), "session aborted")
}

func (r *SyncRepo) CommitSession(ctx context.Context, userID, sessionID uuid.UUID, req models.SyncCommitRequest) (*models.BundleImportResult, error) {
	session, err := r.loadSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status == models.SyncSessionStatusExpired || time.Now().UTC().After(session.ExpiresAt) {
		return nil, services.ErrSyncSessionExpired
	}
	received, err := r.receivedParts(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(received) != session.TotalParts {
		return nil, services.ErrSyncSessionIncomplete
	}
	archiveData, err := r.readArchiveData(ctx, sessionID)
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
	_, _, importSvc, _, syncSvc := r.newBundleServices()
	if req.PreviewFingerprint != "" {
		preview, err := syncSvc.PreviewBundle(ctx, userID, *bundle)
		if err != nil {
			return nil, err
		}
		if preview.Fingerprint != req.PreviewFingerprint {
			return nil, services.ErrSyncPreviewDrift
		}
	}
	result, err := importSvc.ImportBundle(ctx, userID, *bundle)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	_, _ = r.Store.DB().ExecContext(ctx, `UPDATE sync_sessions SET status = ?, updated_at = ?, committed_at = ? WHERE id = ?`, models.SyncSessionStatusCommitted, timeText(now), timeText(now), sessionID.String())
	_, _ = r.Store.DB().ExecContext(ctx, `DELETE FROM sync_session_parts WHERE session_id = ?`, sessionID.String())
	if err := r.finishSyncJob(ctx, session.JobID, userID, models.SyncJobStatusSucceeded, syncSummaryFromBundleStats(manifest.Stats), ""); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *SyncRepo) ListJobs(ctx context.Context, userID uuid.UUID) ([]models.SyncJob, error) {
	rows, err := r.Store.DB().QueryContext(ctx,
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

func (r *SyncRepo) GetJob(ctx context.Context, userID, jobID uuid.UUID) (*models.SyncJob, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT id, user_id, session_id, direction, transport, status, source, mode, filters_json, summary_json, error, created_at, updated_at, completed_at
		   FROM sync_jobs
		  WHERE user_id = ? AND id = ?`,
		userID.String(),
		jobID.String(),
	)
	return scanSyncJob(row)
}

func (r *SyncRepo) CleanExpiredSessions(ctx context.Context) (*services.SyncCleanupResult, error) {
	rows, err := r.Store.DB().QueryContext(ctx,
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
		_ = r.Store.DB().QueryRowContext(ctx, `SELECT COALESCE(SUM(size_bytes), 0) FROM sync_session_parts WHERE session_id = ?`, session.sessionID).Scan(&bytes)
		if _, err := r.Store.DB().ExecContext(ctx, `DELETE FROM sync_session_parts WHERE session_id = ?`, session.sessionID); err != nil {
			return nil, err
		}
		if _, err := r.Store.DB().ExecContext(ctx, `UPDATE sync_sessions SET status = ?, updated_at = ? WHERE id = ?`, models.SyncSessionStatusExpired, timeText(time.Now().UTC()), session.sessionID); err != nil {
			return nil, err
		}
		jobID, jobErr := uuid.Parse(session.jobID)
		userID, userErr := uuid.Parse(session.userID)
		if jobErr == nil && userErr == nil {
			_ = r.finishSyncJob(ctx, jobID, userID, models.SyncJobStatusAborted, models.SyncJobSummary{}, "session expired")
		}
		result.ExpiredSessions++
		result.DeletedBytes += bytes
	}
	return result, nil
}

func (r *SyncRepo) sessionResponse(ctx context.Context, session models.SyncSession) (*models.SyncSessionResponse, error) {
	received, _ := r.receivedParts(ctx, session.ID)
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
	}, nil
}

func (r *SyncRepo) loadSession(ctx context.Context, userID, sessionID uuid.UUID) (*models.SyncSession, error) {
	row := r.Store.DB().QueryRowContext(ctx,
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

func (r *SyncRepo) receivedParts(ctx context.Context, sessionID uuid.UUID) ([]int, error) {
	rows, err := r.Store.DB().QueryContext(ctx, `SELECT part_index FROM sync_session_parts WHERE session_id = ? ORDER BY part_index ASC`, sessionID.String())
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

func (r *SyncRepo) readArchiveData(ctx context.Context, sessionID uuid.UUID) ([]byte, error) {
	rows, err := r.Store.DB().QueryContext(ctx, `SELECT data FROM sync_session_parts WHERE session_id = ? ORDER BY part_index ASC`, sessionID.String())
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

func (r *SyncRepo) insertSyncJob(ctx context.Context, job models.SyncJob) error {
	_, err := r.Store.DB().ExecContext(ctx,
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

func (r *SyncRepo) finishSyncJob(ctx context.Context, jobID, userID uuid.UUID, status string, summary models.SyncJobSummary, errorText string) error {
	now := time.Now().UTC()
	_, err := r.Store.DB().ExecContext(ctx,
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

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
