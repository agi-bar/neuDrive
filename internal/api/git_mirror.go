package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/localgitsync"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/runtimecfg"
	"github.com/google/uuid"
)

const defaultHostedManualGitMirrorSyncWindow = time.Minute

func (s *Server) handleGitMirrorGet(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondNotConfigured(w, "git mirror service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	settings, err := s.LocalGitSync.GetMirrorSettings(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, settings)
}

func (s *Server) handleGitMirrorUpdate(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondNotConfigured(w, "git mirror service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	if err := s.ensureLocalGitMirrorInitialized(r.Context(), userID); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	var req localgitsync.MirrorSettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	settings, err := s.LocalGitSync.UpdateMirrorSettings(r.Context(), userID, req)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, settings)
}

func (s *Server) handleGitMirrorSync(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondNotConfigured(w, "git mirror service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	var req struct {
		ForceRemoteOverwrite bool `json:"force_remote_overwrite"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if retryAfter, limited, err := s.gitMirrorManualSyncRetryAfter(r.Context(), userID); err != nil {
		respondInternalError(w, err)
		return
	} else if limited {
		seconds := int(retryAfter.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		writeJSON(w, http.StatusTooManyRequests, struct {
			Code          string `json:"code"`
			Message       string `json:"message"`
			RetryAfterSec int    `json:"retry_after_sec"`
		}{
			Code:          ErrCodeRateLimit,
			Message:       "GitHub Backup is cooling down. Please try again shortly.",
			RetryAfterSec: seconds,
		})
		return
	}
	info, err := s.LocalGitSync.QueueOrSyncActiveMirror(r.Context(), userID, "manual", req.ForceRemoteOverwrite)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	if info == nil {
		info = &localgitsync.SyncInfo{
			Enabled:       false,
			ExecutionMode: localgitsync.ExecutionModeHosted,
			SyncState:     localgitsync.SyncStateIdle,
			Synced:        false,
		}
	}
	respondOK(w, info)
}

func (s *Server) ensureLocalGitMirrorInitialized(ctx context.Context, userID uuid.UUID) error {
	if !s.isLocalMode() {
		return nil
	}
	settings, err := s.LocalGitSync.GetMirrorSettings(ctx, userID)
	if err != nil {
		return err
	}
	if settings == nil || settings.Enabled {
		return nil
	}
	_, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		return err
	}
	if err := runtimecfg.EnsureLocalDefaults(cfg); err != nil {
		return err
	}
	_, err = s.LocalGitSync.RegisterMirrorAndSync(ctx, userID, cfg.Local.GitMirrorPath)
	return err
}

func (s *Server) gitMirrorManualSyncRetryAfter(ctx context.Context, userID uuid.UUID) (time.Duration, bool, error) {
	window := s.gitMirrorManualSyncWindow()
	if window <= 0 {
		return 0, false, nil
	}
	active, err := s.LocalGitSync.GetActiveMirror(ctx, userID)
	if err != nil || active == nil {
		return 0, false, err
	}
	latest := latestGitMirrorSyncAction(active)
	if latest == nil {
		return 0, false, nil
	}
	remaining := window - time.Since(*latest)
	if remaining <= 0 {
		return 0, false, nil
	}
	return remaining.Round(time.Second), true, nil
}

func (s *Server) gitMirrorManualSyncWindow() time.Duration {
	if s != nil && s.Config != nil && s.Config.GitMirrorManualSyncCooldownConfigured {
		return time.Duration(s.Config.GitMirrorManualSyncCooldownSeconds) * time.Second
	}
	if s != nil && s.isLocalMode() {
		return 0
	}
	return defaultHostedManualGitMirrorSyncWindow
}

func (s *Server) gitMirrorManualSyncCooldownSeconds() int {
	window := s.gitMirrorManualSyncWindow()
	if window <= 0 {
		return 0
	}
	seconds := int(window / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

func latestGitMirrorSyncAction(mirror *models.LocalGitMirror) *time.Time {
	if mirror == nil {
		return nil
	}
	var latest *time.Time
	for _, candidate := range []*time.Time{
		mirror.SyncRequestedAt,
		mirror.SyncStartedAt,
		mirror.LastSyncedAt,
		mirror.LastPushAt,
	} {
		if candidate == nil {
			continue
		}
		if latest == nil || candidate.After(*latest) {
			value := candidate.UTC()
			latest = &value
		}
	}
	return latest
}

func (s *Server) handleGitMirrorGitHubTest(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondNotConfigured(w, "git mirror service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	var req struct {
		RemoteURL   string `json:"remote_url"`
		GitHubToken string `json:"github_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	result, err := s.LocalGitSync.TestGitHubToken(r.Context(), userID, req.RemoteURL, req.GitHubToken)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, result)
}

func (s *Server) handleGitMirrorGitHubAppBrowserStart(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondNotConfigured(w, "git mirror service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	var req struct {
		ReturnTo string `json:"return_to"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	result, err := s.LocalGitSync.StartGitHubAppBrowserFlow(r.Context(), userID, req.ReturnTo)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, result)
}

func (s *Server) handleGitMirrorGitHubAppCallback(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondNotConfigured(w, "git mirror service")
		return
	}
	returnTo, err := s.LocalGitSync.CompleteGitHubAppBrowserFlow(r.Context(), r.URL.Query().Get("code"), r.URL.Query().Get("state"))
	target := returnTo
	if strings.TrimSpace(target) == "" {
		target = "/git-mirror"
	}
	if err != nil {
		target = addQueryValue(target, "github_app_error", err.Error())
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) handleGitMirrorGitHubAppDeviceStart(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondNotConfigured(w, "git mirror service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	result, err := s.LocalGitSync.StartGitHubAppDeviceFlow(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, result)
}

func (s *Server) handleGitMirrorGitHubAppDevicePoll(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondNotConfigured(w, "git mirror service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	var req struct {
		DeviceCode string `json:"device_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	result, err := s.LocalGitSync.PollGitHubAppDeviceFlow(r.Context(), userID, req.DeviceCode)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, result)
}

func (s *Server) handleGitMirrorGitHubAppDisconnect(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondNotConfigured(w, "git mirror service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	if err := s.LocalGitSync.DisconnectGitHubAppUser(r.Context(), userID); err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, map[string]string{"status": "disconnected"})
}

func (s *Server) respondGitHubAppPermissionUpdateRequired(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	if err := s.LocalGitSync.DisconnectGitHubAppUser(r.Context(), userID); err != nil {
		respondInternalError(w, err)
		return
	}
	respondError(
		w,
		http.StatusForbidden,
		ErrCodeGitHubAppPermissionUpdateRequired,
		"GitHub App permissions changed and need to be approved again. The old GitHub Backup connection was disconnected. Open GitHub to approve Repository Administration read/write access, then reconnect GitHub in neuDrive and create the backup repository again.",
	)
}

func (s *Server) handleGitMirrorGitHubAppReposList(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondNotConfigured(w, "git mirror service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	repos, err := s.LocalGitSync.ListGitHubAppRepos(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, repos)
}

func (s *Server) handleGitMirrorGitHubAppReposCreate(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondNotConfigured(w, "git mirror service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	var req localgitsync.GitHubMirrorRepoCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	repo, err := s.LocalGitSync.CreateGitHubAppRepo(r.Context(), userID, req)
	if err != nil {
		if localgitsync.IsGitHubAppPermissionUpdateRequired(err) {
			s.respondGitHubAppPermissionUpdateRequired(w, r, userID)
			return
		}
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondCreated(w, repo)
}

func (s *Server) handleGitMirrorGitHubAppDefaultBackupRepo(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondNotConfigured(w, "git mirror service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	if err := s.ensureLocalGitMirrorInitialized(r.Context(), userID); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	result, err := s.LocalGitSync.CreateOrReuseDefaultGitHubAppBackupRepo(r.Context(), userID)
	if err != nil {
		if localgitsync.IsGitHubAppPermissionUpdateRequired(err) {
			s.respondGitHubAppPermissionUpdateRequired(w, r, userID)
			return
		}
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, result)
}

func addQueryValue(target, key, value string) string {
	parsed, err := url.Parse(target)
	if err != nil {
		return target
	}
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
