package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/agi-bar/agenthub/internal/services"
)

// Handler holds dependencies for auth HTTP handlers.
type Handler struct {
	UserService        *services.UserService
	JWTSecret          string
	GitHubClientID     string
	GitHubClientSecret string
}

// NewHandler creates a new auth Handler.
func NewHandler(userSvc *services.UserService, jwtSecret, ghClientID, ghClientSecret string) *Handler {
	return &Handler{
		UserService:        userSvc,
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

// HandleGitHubCallback handles POST /api/auth/github/callback.
// It receives a {code} body, exchanges it for a GitHub user, creates or updates
// the user via UserService, and returns a JWT along with user info.
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

	if h.GitHubClientID == "" || h.GitHubClientSecret == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "github oauth not configured"})
		return
	}

	ghUser, err := ExchangeGitHubCode(r.Context(), h.GitHubClientID, h.GitHubClientSecret, req.Code)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("github exchange failed: %v", err)})
		return
	}

	// Determine display name; fall back to login if name is empty.
	displayName := ghUser.Name
	if displayName == "" {
		displayName = ghUser.Login
	}

	// Create or update user in the database.
	user, err := h.UserService.CreateOrUpdateFromGitHub(
		r.Context(),
		strconv.Itoa(ghUser.ID),
		ghUser.Login,
		displayName,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to upsert user"})
		return
	}

	// Generate JWT.
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

// HandleMe handles GET /api/auth/me.
// It validates the JWT from the Authorization header and returns the current user info.
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

	user, err := h.UserService.GetByID(r.Context(), claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":           user.ID,
		"slug":         user.Slug,
		"display_name": user.DisplayName,
		"timezone":     user.Timezone,
		"language":     user.Language,
		"created_at":   user.CreatedAt,
	})
}

// HandleDevToken handles POST /api/auth/token/dev.
// DEV ONLY: creates a JWT for a user by slug without GitHub OAuth.
// This is intended for local development and testing.
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

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
