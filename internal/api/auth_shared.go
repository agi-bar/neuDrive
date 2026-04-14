package api

import (
	"encoding/json"
	"net/http"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	if s.AuthService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "auth service not configured")
		return
	}

	user, err := s.AuthService.GetProfile(r.Context(), userID)
	if err != nil {
		respondNotFound(w, "user")
		return
	}
	respondOK(w, map[string]interface{}{
		"id":           user.ID,
		"slug":         user.Slug,
		"display_name": user.DisplayName,
		"email":        user.Email,
		"avatar_url":   user.AvatarURL,
		"bio":          user.Bio,
		"timezone":     user.Timezone,
		"language":     user.Language,
		"created_at":   user.CreatedAt,
	})
}

func (s *Server) handleAuthUpdateMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	if s.AuthService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "auth service not configured")
		return
	}

	var req models.UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	user, err := s.AuthService.UpdateProfile(r.Context(), userID, req.DisplayName, req.Bio, req.Timezone, req.Language)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOKWithLocalGitSync(w, map[string]interface{}{
		"id":           user.ID,
		"slug":         user.Slug,
		"display_name": user.DisplayName,
		"email":        user.Email,
		"avatar_url":   user.AvatarURL,
		"bio":          user.Bio,
		"timezone":     user.Timezone,
		"language":     user.Language,
		"created_at":   user.CreatedAt,
	}, s.syncLocalGitMirror(r.Context(), userID))
}

func (s *Server) handleAuthChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	if s.AuthService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "auth service not configured")
		return
	}

	var req models.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	if err := s.AuthService.ChangePassword(r.Context(), userID, req.OldPassword, req.NewPassword); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleAuthListSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	if s.AuthService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "auth service not configured")
		return
	}

	sessions, err := s.AuthService.ListSessions(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, sessions)
}

func (s *Server) handleAuthRevokeSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	if s.AuthService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "auth service not configured")
		return
	}

	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid session id")
		return
	}
	if err := s.AuthService.RevokeSession(r.Context(), userID, sessionID); err != nil {
		respondNotFound(w, "session")
		return
	}
	respondOK(w, map[string]string{"status": "ok"})
}
