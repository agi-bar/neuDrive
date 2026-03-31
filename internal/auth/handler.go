package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler holds dependencies for auth HTTP handlers.
type Handler struct {
	UserService        *services.UserService
	AuthService        *services.AuthService
	JWTSecret          string
	GitHubClientID     string
	GitHubClientSecret string
}

// NewHandler creates a new auth Handler.
func NewHandler(userSvc *services.UserService, authSvc *services.AuthService, jwtSecret, ghClientID, ghClientSecret string) *Handler {
	return &Handler{
		UserService:        userSvc,
		AuthService:        authSvc,
		JWTSecret:          jwtSecret,
		GitHubClientID:     ghClientID,
		GitHubClientSecret: ghClientSecret,
	}
}

type githubCallbackRequest struct {
	Code string `json:"code"`
}

type devTokenRequest struct {
	Slug string `json:"slug"`
}

// HandleRegister handles POST /api/auth/register.
func (h *Handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	resp, err := h.AuthService.Register(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// HandleLogin handles POST /api/auth/login.
func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	userAgent := r.UserAgent()
	ipAddress := r.RemoteAddr

	resp, err := h.AuthService.Login(r.Context(), req, userAgent, ipAddress)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleRefresh handles POST /api/auth/refresh.
func (h *Handler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	var req models.RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	userAgent := r.UserAgent()
	ipAddress := r.RemoteAddr

	resp, err := h.AuthService.RefreshToken(r.Context(), req.RefreshToken, userAgent, ipAddress)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleLogout handles POST /api/auth/logout.
func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	var req models.RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.AuthService.Logout(r.Context(), req.RefreshToken); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleGitHubCallback handles POST /api/auth/github/callback.
func (h *Handler) HandleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	var req githubCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code is required"})
		return
	}

	userAgent := r.UserAgent()
	ipAddress := r.RemoteAddr

	resp, err := h.AuthService.GitHubLogin(r.Context(), req.Code, userAgent, ipAddress)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("github login failed: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleMe handles GET /api/auth/me.
func (h *Handler) HandleMe(w http.ResponseWriter, r *http.Request) {
	tokenStr, err := ExtractTokenFromHeader(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no token provided"})
		return
	}

	claims, err := ValidateToken(tokenStr, h.JWTSecret)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
		return
	}

	user, err := h.AuthService.GetProfile(r.Context(), claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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

// HandleUpdateMe handles PUT /api/auth/me.
func (h *Handler) HandleUpdateMe(w http.ResponseWriter, r *http.Request) {
	tokenStr, err := ExtractTokenFromHeader(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no token provided"})
		return
	}

	claims, err := ValidateToken(tokenStr, h.JWTSecret)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
		return
	}

	var req models.UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	user, err := h.AuthService.UpdateProfile(r.Context(), claims.UserID, req.DisplayName, req.Bio, req.Timezone, req.Language)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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

// HandleChangePassword handles POST /api/auth/change-password.
func (h *Handler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	tokenStr, err := ExtractTokenFromHeader(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no token provided"})
		return
	}

	claims, err := ValidateToken(tokenStr, h.JWTSecret)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
		return
	}

	var req models.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.AuthService.ChangePassword(r.Context(), claims.UserID, req.OldPassword, req.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleListSessions handles GET /api/auth/sessions.
func (h *Handler) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	tokenStr, err := ExtractTokenFromHeader(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no token provided"})
		return
	}

	claims, err := ValidateToken(tokenStr, h.JWTSecret)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
		return
	}

	sessions, err := h.AuthService.ListSessions(r.Context(), claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sessions"})
		return
	}

	writeJSON(w, http.StatusOK, sessions)
}

// HandleRevokeSession handles DELETE /api/auth/sessions/{id}.
func (h *Handler) HandleRevokeSession(w http.ResponseWriter, r *http.Request) {
	tokenStr, err := ExtractTokenFromHeader(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no token provided"})
		return
	}

	claims, err := ValidateToken(tokenStr, h.JWTSecret)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
		return
	}

	sessionIDStr := chi.URLParam(r, "id")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
		return
	}

	if err := h.AuthService.RevokeSession(r.Context(), claims.UserID, sessionID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleDevToken handles POST /api/auth/token/dev.
// DEV ONLY: creates a JWT for a user by slug without GitHub OAuth.
func (h *Handler) HandleDevToken(w http.ResponseWriter, r *http.Request) {
	var req devTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug is required"})
		return
	}

	user, err := h.UserService.GetBySlug(r.Context(), req.Slug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	token, err := GenerateToken(user.ID, user.Slug, h.JWTSecret)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": token,
		"user": map[string]interface{}{
			"id":           user.ID,
			"slug":         user.Slug,
			"display_name": user.DisplayName,
		},
	})
}

// Ensure strconv import is used (for HandleGitHubCallback's old flow)
var _ = strconv.Itoa

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
