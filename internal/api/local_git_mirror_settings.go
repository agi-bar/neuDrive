package api

import (
	"encoding/json"
	"net/http"

	"github.com/agi-bar/neudrive/internal/localgitsync"
)

func (s *Server) handleLocalGitMirrorGet(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local git mirror is only available in local mode")
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

func (s *Server) handleLocalGitMirrorUpdate(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local git mirror is only available in local mode")
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

func (s *Server) handleLocalGitMirrorGitHubTest(w http.ResponseWriter, r *http.Request) {
	if s.LocalGitSync == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local git mirror is only available in local mode")
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
