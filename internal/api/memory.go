package api

import (
	"encoding/json"
	"net/http"
)

type UserProfile struct {
	UserID      string            `json:"user_id"`
	DisplayName string            `json:"display_name"`
	Preferences map[string]string `json:"preferences"`
	UpdatedAt   string            `json:"updated_at,omitempty"`
}

type Project struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Logs        []ProjectLog `json:"logs,omitempty"`
	CreatedAt   string       `json:"created_at,omitempty"`
	UpdatedAt   string       `json:"updated_at,omitempty"`
}

type ProjectLog struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Level     string `json:"level"`
	Metadata  string `json:"metadata,omitempty"`
	CreatedAt string `json:"created_at"`
}

type UpdateProfileRequest struct {
	DisplayName string            `json:"display_name"`
	Preferences map[string]string `json:"preferences"`
}

type ProjectLogRequest struct {
	Message  string `json:"message"`
	Level    string `json:"level"`
	Metadata string `json:"metadata,omitempty"`
}

func (s *Server) handleMemoryProfileGet(w http.ResponseWriter, r *http.Request) {
	if s.MemoryService == nil {
		respondNotConfigured(w, "memory service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	profiles, err := s.MemoryService.GetProfile(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	// Transform to the API response format: preferences map
	prefs := make(map[string]string)
	for _, p := range profiles {
		prefs[p.Category] = p.Content
	}

	profile := &UserProfile{
		UserID:      userID.String(),
		Preferences: prefs,
	}

	// Get user display name if available
	if user, err := s.UserService.GetByID(r.Context(), userID); err == nil {
		profile.DisplayName = user.DisplayName
	}

	respondOK(w, profile)
}

func (s *Server) handleMemoryProfileUpdate(w http.ResponseWriter, r *http.Request) {
	if s.MemoryService == nil {
		respondNotConfigured(w, "memory service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	// Upsert each preference as a separate memory profile entry
	for category, content := range req.Preferences {
		if err := s.MemoryService.UpsertProfile(r.Context(), userID, category, content, "web"); err != nil {
			respondInternalError(w, err)
			return
		}
	}

	// Update display name if provided
	if req.DisplayName != "" {
		if err := s.MemoryService.UpsertProfile(r.Context(), userID, "display_name", req.DisplayName, "web"); err != nil {
			respondInternalError(w, err)
			return
		}
	}

	profile := &UserProfile{
		UserID:      userID.String(),
		DisplayName: req.DisplayName,
		Preferences: req.Preferences,
	}

	respondOKWithLocalGitSync(w, profile, s.syncLocalGitMirror(r.Context(), userID))
}

func (s *Server) handleWriteScratch(w http.ResponseWriter, r *http.Request) {
	if s.MemoryService == nil {
		respondNotConfigured(w, "memory service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req struct {
		Content string `json:"content"`
		Source  string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.Content == "" {
		respondValidationError(w, "content", "content is required")
		return
	}
	if req.Source == "" {
		req.Source = "web"
	}

	if err := s.MemoryService.WriteScratch(r.Context(), userID, req.Content, req.Source); err != nil {
		respondInternalError(w, err)
		return
	}

	respondCreatedWithLocalGitSync(w, map[string]string{"status": "written"}, s.syncLocalGitMirror(r.Context(), userID))
}
