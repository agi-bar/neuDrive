package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleCreateSyncToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	if s.TokenService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "token service not configured")
		return
	}

	var req models.SyncTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	access := strings.ToLower(strings.TrimSpace(req.Access))
	if access == "" {
		access = "push"
	}
	ttlMinutes := req.TTLMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 30
	}
	if ttlMinutes < 5 {
		ttlMinutes = 5
	}
	if ttlMinutes > 120 {
		ttlMinutes = 120
	}

	scopes := []string{models.ScopeWriteBundle}
	usage := "Use this token with /agent/import/* sync endpoints."
	switch access {
	case "push":
		scopes = []string{models.ScopeWriteBundle}
		usage = "Use this token for bundle push, preview, and sync session upload."
	case "pull":
		scopes = []string{models.ScopeReadBundle}
		usage = "Use this token for bundle pull and sync history reads."
	case "both":
		scopes = []string{models.ScopeReadBundle, models.ScopeWriteBundle}
		usage = "Use this token for bundle push, pull, preview, and sync history reads."
	default:
		respondValidationError(w, "access", "access must be push, pull, or both")
		return
	}

	created, err := s.TokenService.CreateEphemeralToken(
		r.Context(),
		userID,
		fmt.Sprintf("sync-%s", access),
		scopes,
		models.TrustLevelWork,
		time.Duration(ttlMinutes)*time.Minute,
	)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}

	respondCreated(w, models.SyncTokenResponse{
		Token:     created.Token,
		ExpiresAt: created.ScopedToken.ExpiresAt,
		APIBase:   requestBaseURL(r),
		Scopes:    scopes,
		Usage:     usage,
	})
}

func (s *Server) handleAgentStartSyncSession(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	if s.SyncService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "sync service not configured")
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	var req models.SyncStartSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	resp, err := s.SyncService.StartSession(r.Context(), userID, req)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondCreated(w, resp)
}

func (s *Server) handleAgentUploadSyncPart(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	if s.SyncService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "sync service not configured")
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid session id")
		return
	}
	index, err := strconv.Atoi(chi.URLParam(r, "index"))
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid part index")
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	resp, err := s.SyncService.UploadPart(r.Context(), userID, sessionID, index, data)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, services.ErrSyncPartConflict):
			status = http.StatusConflict
		case errors.Is(err, services.ErrSyncSessionNotFound):
			status = http.StatusNotFound
		case errors.Is(err, services.ErrSyncSessionExpired):
			status = http.StatusGone
		}
		respondError(w, status, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleAgentGetSyncSession(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	if s.SyncService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "sync service not configured")
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid session id")
		return
	}
	resp, err := s.SyncService.GetSession(r.Context(), userID, sessionID)
	if err != nil {
		if errors.Is(err, services.ErrSyncSessionNotFound) {
			respondNotFound(w, "sync session")
			return
		}
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleAgentCommitSyncSession(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	if s.SyncService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "sync service not configured")
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid session id")
		return
	}
	var req models.SyncCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	resp, err := s.SyncService.CommitSession(r.Context(), userID, sessionID, req)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, services.ErrSyncSessionNotFound):
			status = http.StatusNotFound
		case errors.Is(err, services.ErrSyncSessionExpired):
			status = http.StatusGone
		case errors.Is(err, services.ErrSyncSessionIncomplete):
			status = http.StatusConflict
		case errors.Is(err, services.ErrSyncPreviewDrift):
			status = http.StatusConflict
		}
		respondError(w, status, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleAgentDeleteSyncSession(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	if s.SyncService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "sync service not configured")
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid session id")
		return
	}
	if err := s.SyncService.AbortSession(r.Context(), userID, sessionID); err != nil {
		if errors.Is(err, services.ErrSyncSessionNotFound) {
			respondNotFound(w, "sync session")
			return
		}
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, map[string]string{"status": "aborted"})
}

func (s *Server) handleAgentListSyncJobs(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeReadBundle) {
		return
	}
	if s.SyncService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "sync service not configured")
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	jobs, err := s.SyncService.ListJobs(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, map[string]any{"jobs": jobs})
}

func (s *Server) handleAgentGetSyncJob(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeReadBundle) {
		return
	}
	if s.SyncService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "sync service not configured")
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid job id")
		return
	}
	job, err := s.SyncService.GetJob(r.Context(), userID, jobID)
	if err != nil {
		if errors.Is(err, services.ErrSyncSessionNotFound) {
			respondNotFound(w, "sync job")
			return
		}
		respondInternalError(w, err)
		return
	}
	respondOK(w, job)
}

func parseBundleFilters(r *http.Request) models.BundleFilters {
	query := r.URL.Query()
	return models.BundleFilters{
		IncludeDomains: queryValues(query, "include_domain", "include_domains"),
		IncludeSkills:  queryValues(query, "include_skill", "include_skills"),
		ExcludeSkills:  queryValues(query, "exclude_skill", "exclude_skills"),
	}
}

func queryValues(values map[string][]string, singular, plural string) []string {
	var result []string
	for _, key := range []string{singular, plural} {
		for _, raw := range values[key] {
			for _, part := range strings.Split(raw, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					result = append(result, part)
				}
			}
		}
	}
	return result
}

func requestBaseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = forwarded
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}
