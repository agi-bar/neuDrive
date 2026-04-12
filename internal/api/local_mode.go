package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/agi-bar/agenthub/internal/localgitsync"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
)

func (s *Server) isLocalMode() bool {
	return s != nil && s.LocalOwnerID != uuid.Nil
}

func (s *Server) handleBootstrapLocalOwnerToken(w http.ResponseWriter, r *http.Request) {
	if !s.isLocalMode() {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local owner bootstrap is only available in local mode")
		return
	}
	if s.TokenService == nil {
		respondNotConfigured(w, "token service")
		return
	}
	tokenResp, err := s.TokenService.CreateToken(r.Context(), s.LocalOwnerID, models.CreateTokenRequest{
		Name:          "local owner",
		Scopes:        []string{models.ScopeAdmin},
		MaxTrustLevel: models.TrustLevelFull,
		ExpiresInDays: 365,
	})
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondCreated(w, tokenResp)
}

func (s *Server) handleAgentRegisterLocalGitMirror(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelFull, models.ScopeAdmin) {
		return
	}
	if s.LocalGitSync == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local git mirror is only available in local mode")
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	var req struct {
		OutputRoot string `json:"output_root"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	info, err := s.LocalGitSync.RegisterMirrorAndSync(r.Context(), userID, req.OutputRoot)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, info)
}

func (s *Server) handleAgentSyncLocalGitMirror(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelFull, models.ScopeAdmin) {
		return
	}
	if s.LocalGitSync == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local git mirror is only available in local mode")
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	info, err := s.LocalGitSync.SyncActiveMirror(r.Context(), userID)
	if err != nil && info == nil {
		respondInternalError(w, err)
		return
	}
	if info == nil {
		info = &localgitsync.SyncInfo{
			Enabled: false,
			Synced:  false,
		}
	}
	respondOK(w, info)
}
