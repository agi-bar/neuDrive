package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/agi-bar/neudrive/internal/localgitsync"
)

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
	info, err := s.LocalGitSync.QueueOrSyncActiveMirror(r.Context(), userID, "manual")
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
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondCreated(w, repo)
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
