package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/platforms"
	"github.com/agi-bar/neudrive/internal/runtimecfg"
	"github.com/agi-bar/neudrive/internal/services"
	sqlitestorage "github.com/agi-bar/neudrive/internal/storage/sqlite"
	"github.com/google/uuid"
)

var previewImportFunc = platforms.PreviewImport

type localPlatformDashboardRequest struct {
	Platform string `json:"platform"`
	Mode     string `json:"mode"`
}

type localPlatformPreviewCacheEnvelope struct {
	Version  string                   `json:"version"`
	Platform string                   `json:"platform"`
	Mode     platforms.ImportMode     `json:"mode"`
	SavedAt  string                   `json:"saved_at"`
	Preview  *platforms.ImportPreview `json:"preview"`
}

type localPlatformPreviewTaskStatus struct {
	Version       string               `json:"version"`
	JobID         string               `json:"job_id,omitempty"`
	Platform      string               `json:"platform"`
	DisplayName   string               `json:"display_name,omitempty"`
	Mode          platforms.ImportMode `json:"mode"`
	State         string               `json:"state"`
	StartedAt     string               `json:"started_at,omitempty"`
	UpdatedAt     string               `json:"updated_at,omitempty"`
	CompletedAt   string               `json:"completed_at,omitempty"`
	DurationMs    int64                `json:"duration_ms,omitempty"`
	ErrorMessage  string               `json:"error_message,omitempty"`
	ResultSavedAt string               `json:"result_saved_at,omitempty"`
}

type localPlatformPreviewTaskResponse struct {
	Status  *localPlatformPreviewTaskStatus `json:"status,omitempty"`
	Preview *platforms.ImportPreview        `json:"preview,omitempty"`
}

type localPlatformDashboardImportResponse struct {
	Platform string                           `json:"platform"`
	Mode     platforms.ImportMode             `json:"mode"`
	Files    *sqlitestorage.ImportResult      `json:"files,omitempty"`
	Agent    *sqlitestorage.AgentImportResult `json:"agent,omitempty"`
}

const (
	localPlatformPreviewCacheVersion = "neudrive.local_platform_preview/v1"
	localPlatformPreviewTaskVersion  = "neudrive.local_platform_preview_task/v1"

	localPlatformPreviewTaskStateIdle      = "idle"
	localPlatformPreviewTaskStateRunning   = "running"
	localPlatformPreviewTaskStateSucceeded = "succeeded"
	localPlatformPreviewTaskStateFailed    = "failed"
)

func (s *Server) handleLocalPlatformPreview(w http.ResponseWriter, r *http.Request) {
	if !s.isLocalMode() {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local platform preview is only available in local mode")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	adapter, mode, err := decodeLocalPlatformDashboardTarget(r)
	if err != nil {
		respondPreviewTargetError(w, err)
		return
	}

	cfg, err := loadRuntimeCLIConfig()
	if err != nil {
		respondInternalError(w, err)
		return
	}
	preview, err := previewImportFunc(r.Context(), cfg, adapter.ID(), string(mode))
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	if writeErr := s.writeLocalPlatformPreviewCache(s.requestSourceContext(r, "local-platform-preview"), userID, preview.Platform, preview.Mode, preview); writeErr != nil {
		slog.Warn("persist local platform preview cache", "platform", preview.Platform, "mode", preview.Mode, "error", writeErr)
	}
	respondOK(w, preview)
}

func (s *Server) handleLocalPlatformPreviewTask(w http.ResponseWriter, r *http.Request) {
	if !s.isLocalMode() {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local platform preview is only available in local mode")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	adapter, mode, err := decodeLocalPlatformDashboardTarget(r)
	if err != nil {
		respondPreviewTargetError(w, err)
		return
	}

	switch r.Method {
	case http.MethodGet:
		task, err := s.readLocalPlatformPreviewTask(r.Context(), userID, adapter.ID(), mode)
		if err != nil {
			respondInternalError(w, err)
			return
		}
		respondOK(w, task)
	case http.MethodPost:
		task, err := s.startLocalPlatformPreviewTask(r.Context(), userID, adapter, mode)
		if err != nil {
			respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
			return
		}
		respondOK(w, task)
	default:
		w.Header().Set("Allow", "GET, POST")
		respondError(w, http.StatusMethodNotAllowed, ErrCodeBadRequest, "method not allowed")
	}
}

func (s *Server) handleLocalPlatformPreviewCache(w http.ResponseWriter, r *http.Request) {
	if !s.isLocalMode() {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local platform preview is only available in local mode")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	adapter, mode, err := decodeLocalPlatformDashboardTarget(r)
	if err != nil {
		respondPreviewTargetError(w, err)
		return
	}
	preview, err := s.readLocalPlatformPreviewCache(r.Context(), userID, adapter.ID(), mode)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, preview)
}

func (s *Server) handleLocalPlatformImport(w http.ResponseWriter, r *http.Request) {
	if !s.isLocalMode() {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local platform import is only available in local mode")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	adapter, mode, err := decodeLocalPlatformDashboardTarget(r)
	if err != nil {
		respondPreviewTargetError(w, err)
		return
	}

	cfg, err := loadRuntimeCLIConfig()
	if err != nil {
		respondInternalError(w, err)
		return
	}

	resp := &localPlatformDashboardImportResponse{
		Platform: adapter.ID(),
		Mode:     mode,
	}

	switch mode {
	case platforms.ImportModeFiles:
		resp.Files, err = s.importLocalPlatformSources(r.Context(), userID, adapter.ID(), adapter.DiscoverSources())
	case platforms.ImportModeAgent:
		var payload sqlitestorage.AgentExportPayload
		payload, err = platforms.PrepareAgentImportPayload(r.Context(), cfg, adapter.ID())
		if err == nil {
			resp.Agent, err = s.importLocalPlatformAgentPayload(r.Context(), userID, adapter.ID(), payload)
		}
	case platforms.ImportModeAll:
		var payload sqlitestorage.AgentExportPayload
		payload, err = platforms.PrepareAgentImportPayload(r.Context(), cfg, adapter.ID())
		if err == nil {
			resp.Agent, err = s.importLocalPlatformAgentPayload(r.Context(), userID, adapter.ID(), payload)
		}
		if err == nil {
			resp.Files, err = s.importLocalPlatformSources(r.Context(), userID, adapter.ID(), adapter.DiscoverSources())
		}
	}
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}

	respondOKWithLocalGitSync(w, resp, s.syncLocalGitMirror(r.Context(), userID))
}

func decodeLocalPlatformDashboardTarget(r *http.Request) (platforms.Adapter, platforms.ImportMode, error) {
	var req localPlatformDashboardRequest
	switch r.Method {
	case http.MethodGet:
		req.Platform = r.URL.Query().Get("platform")
		req.Mode = r.URL.Query().Get("mode")
	default:
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, "", fmt.Errorf("invalid request body")
		}
	}
	if strings.TrimSpace(req.Platform) == "" {
		req.Platform = "claude"
	}
	adapter, err := platforms.Resolve(req.Platform)
	if err != nil {
		return nil, "", validationError("platform", err.Error())
	}
	mode, err := platforms.ParseImportMode(adapter.ID(), req.Mode)
	if err != nil {
		return nil, "", validationError("mode", err.Error())
	}
	return adapter, mode, nil
}

type fieldValidationError struct {
	field   string
	message string
}

func (e fieldValidationError) Error() string { return e.message }

func validationError(field, message string) error {
	return fieldValidationError{field: field, message: message}
}

func respondPreviewTargetError(w http.ResponseWriter, err error) {
	var validation fieldValidationError
	if errors.As(err, &validation) {
		respondValidationError(w, validation.field, validation.message)
		return
	}
	respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
}

func loadRuntimeCLIConfig() (*runtimecfg.CLIConfig, error) {
	_, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		return nil, err
	}
	if err := runtimecfg.EnsureLocalDefaults(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func localPlatformPreviewCachePath(platformID string, mode platforms.ImportMode) string {
	platformID = normalizeLocalPlatformPreviewPlatformID(platformID)
	filename := fmt.Sprintf("latest-preview-%s.json", strings.TrimSpace(string(mode)))
	return filepath.ToSlash(filepath.Join("/platforms", platformID, "dashboard", filename))
}

func localPlatformPreviewTaskPath(platformID string, mode platforms.ImportMode) string {
	platformID = normalizeLocalPlatformPreviewPlatformID(platformID)
	filename := fmt.Sprintf("latest-preview-%s.status.json", strings.TrimSpace(string(mode)))
	return filepath.ToSlash(filepath.Join("/platforms", platformID, "dashboard", filename))
}

func normalizeLocalPlatformPreviewPlatformID(platformID string) string {
	platformID = strings.TrimSpace(platformID)
	if platformID == "" {
		return "claude-code"
	}
	return platformID
}

func localPlatformPreviewJobKey(userID uuid.UUID, platformID string, mode platforms.ImportMode) string {
	return fmt.Sprintf("%s:%s:%s", userID.String(), normalizeLocalPlatformPreviewPlatformID(platformID), string(mode))
}

func (s *Server) isLocalPlatformPreviewJobRunning(userID uuid.UUID, platformID string, mode platforms.ImportMode, jobID string) bool {
	value, ok := s.localPlatformPreviewJobs.Load(localPlatformPreviewJobKey(userID, platformID, mode))
	if !ok {
		return false
	}
	activeJobID, _ := value.(string)
	if strings.TrimSpace(jobID) == "" {
		return activeJobID != ""
	}
	return activeJobID == jobID
}

func (s *Server) markLocalPlatformPreviewJobFinished(userID uuid.UUID, platformID string, mode platforms.ImportMode, jobID string) {
	key := localPlatformPreviewJobKey(userID, platformID, mode)
	if current, ok := s.localPlatformPreviewJobs.Load(key); ok {
		if activeJobID, _ := current.(string); strings.TrimSpace(jobID) == "" || activeJobID == jobID {
			s.localPlatformPreviewJobs.Delete(key)
		}
	}
}

func (s *Server) startLocalPlatformPreviewTask(ctx context.Context, userID uuid.UUID, adapter platforms.Adapter, mode platforms.ImportMode) (*localPlatformPreviewTaskResponse, error) {
	if s == nil || s.FileTreeService == nil {
		return nil, fmt.Errorf("file tree service not configured")
	}
	key := localPlatformPreviewJobKey(userID, adapter.ID(), mode)
	currentTask, err := s.readLocalPlatformPreviewTask(ctx, userID, adapter.ID(), mode)
	if err != nil {
		return nil, err
	}
	var cachedPreview *platforms.ImportPreview
	if currentTask != nil {
		cachedPreview = currentTask.Preview
	}
	jobID := uuid.NewString()
	if existing, loaded := s.localPlatformPreviewJobs.LoadOrStore(key, jobID); loaded {
		if currentTask != nil && currentTask.Status != nil {
			return currentTask, nil
		}
		jobID, _ = existing.(string)
		now := time.Now().UTC()
		status := &localPlatformPreviewTaskStatus{
			Version:     localPlatformPreviewTaskVersion,
			JobID:       jobID,
			Platform:    adapter.ID(),
			DisplayName: adapter.DisplayName(),
			Mode:        mode,
			State:       localPlatformPreviewTaskStateRunning,
			StartedAt:   now.Format(time.RFC3339),
			UpdatedAt:   now.Format(time.RFC3339),
		}
		return &localPlatformPreviewTaskResponse{Status: status, Preview: cachedPreview}, nil
	}

	startedAt := time.Now().UTC()
	status := &localPlatformPreviewTaskStatus{
		Version:     localPlatformPreviewTaskVersion,
		JobID:       jobID,
		Platform:    adapter.ID(),
		DisplayName: adapter.DisplayName(),
		Mode:        mode,
		State:       localPlatformPreviewTaskStateRunning,
		StartedAt:   startedAt.Format(time.RFC3339),
		UpdatedAt:   startedAt.Format(time.RFC3339),
	}
	if err := s.writeLocalPlatformPreviewTaskStatus(services.ContextWithSource(context.Background(), "local-platform-preview"), userID, status); err != nil {
		s.localPlatformPreviewJobs.Delete(key)
		return nil, err
	}

	go s.runLocalPlatformPreviewTask(userID, adapter, mode, status)
	return &localPlatformPreviewTaskResponse{
		Status:  status,
		Preview: cachedPreview,
	}, nil
}

func (s *Server) runLocalPlatformPreviewTask(userID uuid.UUID, adapter platforms.Adapter, mode platforms.ImportMode, status *localPlatformPreviewTaskStatus) {
	ctx := services.ContextWithSource(context.Background(), "local-platform-preview")
	defer s.markLocalPlatformPreviewJobFinished(userID, adapter.ID(), mode, status.JobID)

	cfg, err := loadRuntimeCLIConfig()
	if err != nil {
		s.finishLocalPlatformPreviewTask(ctx, userID, status, nil, err)
		return
	}
	preview, err := previewImportFunc(ctx, cfg, adapter.ID(), string(mode))
	s.finishLocalPlatformPreviewTask(ctx, userID, status, preview, err)
}

func (s *Server) finishLocalPlatformPreviewTask(ctx context.Context, userID uuid.UUID, status *localPlatformPreviewTaskStatus, preview *platforms.ImportPreview, runErr error) {
	if status == nil {
		return
	}
	completedAt := time.Now().UTC()
	next := *status
	next.DisplayName = strings.TrimSpace(next.DisplayName)
	next.UpdatedAt = completedAt.Format(time.RFC3339)
	next.CompletedAt = completedAt.Format(time.RFC3339)
	if preview != nil {
		next.Platform = preview.Platform
		next.DisplayName = preview.DisplayName
		next.Mode = preview.Mode
		if strings.TrimSpace(preview.StartedAt) != "" {
			next.StartedAt = preview.StartedAt
		}
		if strings.TrimSpace(preview.CompletedAt) != "" {
			next.CompletedAt = preview.CompletedAt
			next.UpdatedAt = preview.CompletedAt
		}
		if preview.DurationMs > 0 {
			next.DurationMs = preview.DurationMs
		}
	}
	if runErr != nil {
		next.State = localPlatformPreviewTaskStateFailed
		next.ErrorMessage = runErr.Error()
	} else {
		next.State = localPlatformPreviewTaskStateSucceeded
		next.ErrorMessage = ""
	}
	if preview != nil {
		if writeErr := s.writeLocalPlatformPreviewCache(ctx, userID, preview.Platform, preview.Mode, preview); writeErr != nil {
			slog.Warn("persist local platform preview cache", "platform", preview.Platform, "mode", preview.Mode, "error", writeErr)
			if runErr == nil {
				next.State = localPlatformPreviewTaskStateFailed
				next.ErrorMessage = writeErr.Error()
			}
		} else {
			next.ResultSavedAt = next.CompletedAt
		}
	}
	if next.DurationMs <= 0 && strings.TrimSpace(next.StartedAt) != "" {
		if startedAt, err := time.Parse(time.RFC3339, next.StartedAt); err == nil {
			next.DurationMs = completedAt.Sub(startedAt).Milliseconds()
		}
	}
	if err := s.writeLocalPlatformPreviewTaskStatus(ctx, userID, &next); err != nil {
		slog.Warn("persist local platform preview task status", "platform", next.Platform, "mode", next.Mode, "error", err)
	}
}

func (s *Server) readLocalPlatformPreviewTask(ctx context.Context, userID uuid.UUID, platformID string, mode platforms.ImportMode) (*localPlatformPreviewTaskResponse, error) {
	status, err := s.readLocalPlatformPreviewTaskStatus(ctx, userID, platformID, mode)
	if err != nil {
		return nil, err
	}
	preview, err := s.readLocalPlatformPreviewCache(ctx, userID, platformID, mode)
	if err != nil {
		return nil, err
	}
	if status != nil && status.State == localPlatformPreviewTaskStateRunning && !s.isLocalPlatformPreviewJobRunning(userID, platformID, mode, status.JobID) {
		now := time.Now().UTC()
		status.State = localPlatformPreviewTaskStateFailed
		status.UpdatedAt = now.Format(time.RFC3339)
		status.CompletedAt = now.Format(time.RFC3339)
		if strings.TrimSpace(status.ErrorMessage) == "" {
			status.ErrorMessage = "scan was interrupted"
		}
		if err := s.writeLocalPlatformPreviewTaskStatus(ctx, userID, status); err != nil {
			slog.Warn("persist interrupted local platform preview task status", "platform", platformID, "mode", mode, "error", err)
		}
	}
	if status == nil && preview != nil {
		status = &localPlatformPreviewTaskStatus{
			Version:       localPlatformPreviewTaskVersion,
			Platform:      preview.Platform,
			DisplayName:   preview.DisplayName,
			Mode:          preview.Mode,
			State:         localPlatformPreviewTaskStateSucceeded,
			StartedAt:     preview.StartedAt,
			UpdatedAt:     firstNonEmpty(preview.CompletedAt, preview.StartedAt),
			CompletedAt:   preview.CompletedAt,
			DurationMs:    preview.DurationMs,
			ResultSavedAt: firstNonEmpty(preview.CompletedAt, preview.StartedAt),
		}
	}
	return &localPlatformPreviewTaskResponse{
		Status:  status,
		Preview: preview,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Server) writeLocalPlatformPreviewCache(ctx context.Context, userID uuid.UUID, platformID string, mode platforms.ImportMode, preview *platforms.ImportPreview) error {
	if s == nil || s.FileTreeService == nil || userID == uuid.Nil || preview == nil {
		return nil
	}
	envelope := localPlatformPreviewCacheEnvelope{
		Version:  localPlatformPreviewCacheVersion,
		Platform: strings.TrimSpace(platformID),
		Mode:     mode,
		SavedAt:  preview.CompletedAt,
		Preview:  preview,
	}
	if strings.TrimSpace(envelope.SavedAt) == "" {
		envelope.SavedAt = preview.StartedAt
	}
	content, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"cache_kind":     "local_platform_preview",
		"cache_platform": envelope.Platform,
		"cache_mode":     string(mode),
	}
	_, err = s.FileTreeService.WriteEntry(ctx, userID, localPlatformPreviewCachePath(platformID, mode), string(content), "application/json", models.FileTreeWriteOptions{
		Metadata:      metadata,
		MinTrustLevel: models.TrustLevelFull,
	})
	return err
}

func (s *Server) readLocalPlatformPreviewCache(ctx context.Context, userID uuid.UUID, platformID string, mode platforms.ImportMode) (*platforms.ImportPreview, error) {
	if s == nil || s.FileTreeService == nil || userID == uuid.Nil {
		return nil, nil
	}
	entry, err := s.FileTreeService.Read(ctx, userID, localPlatformPreviewCachePath(platformID, mode), models.TrustLevelFull)
	if err != nil {
		if errors.Is(err, services.ErrEntryNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var envelope localPlatformPreviewCacheEnvelope
	if err := json.Unmarshal([]byte(entry.Content), &envelope); err != nil {
		return nil, nil
	}
	if envelope.Preview == nil {
		return nil, nil
	}
	return envelope.Preview, nil
}

func (s *Server) writeLocalPlatformPreviewTaskStatus(ctx context.Context, userID uuid.UUID, status *localPlatformPreviewTaskStatus) error {
	if s == nil || s.FileTreeService == nil || userID == uuid.Nil || status == nil {
		return nil
	}
	content, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"cache_kind":     "local_platform_preview_task",
		"cache_platform": status.Platform,
		"cache_mode":     string(status.Mode),
		"cache_state":    status.State,
	}
	_, err = s.FileTreeService.WriteEntry(ctx, userID, localPlatformPreviewTaskPath(status.Platform, status.Mode), string(content), "application/json", models.FileTreeWriteOptions{
		Metadata:      metadata,
		MinTrustLevel: models.TrustLevelFull,
	})
	return err
}

func (s *Server) readLocalPlatformPreviewTaskStatus(ctx context.Context, userID uuid.UUID, platformID string, mode platforms.ImportMode) (*localPlatformPreviewTaskStatus, error) {
	if s == nil || s.FileTreeService == nil || userID == uuid.Nil {
		return nil, nil
	}
	entry, err := s.FileTreeService.Read(ctx, userID, localPlatformPreviewTaskPath(platformID, mode), models.TrustLevelFull)
	if err != nil {
		if errors.Is(err, services.ErrEntryNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var status localPlatformPreviewTaskStatus
	if err := json.Unmarshal([]byte(entry.Content), &status); err != nil {
		return nil, nil
	}
	if strings.TrimSpace(status.Platform) == "" {
		status.Platform = normalizeLocalPlatformPreviewPlatformID(platformID)
	}
	if status.Mode == "" {
		status.Mode = mode
	}
	if strings.TrimSpace(status.State) == "" {
		status.State = localPlatformPreviewTaskStateIdle
	}
	if strings.TrimSpace(status.Version) == "" {
		status.Version = localPlatformPreviewTaskVersion
	}
	return &status, nil
}
