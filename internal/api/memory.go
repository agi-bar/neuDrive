package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
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

func HandleMemoryProfileGet(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r.Context())
	if user == nil {
		respondUnauthorized(w)
		return
	}

	// TODO: query database for user profile
	profile := &UserProfile{
		UserID:      user.UserID,
		DisplayName: user.Username,
		Preferences: map[string]string{},
	}

	respondOK(w, profile)
}

func HandleMemoryProfileUpdate(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r.Context())
	if user == nil {
		respondUnauthorized(w)
		return
	}

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	// TODO: update user profile in database
	profile := &UserProfile{
		UserID:      user.UserID,
		DisplayName: req.DisplayName,
		Preferences: req.Preferences,
	}

	respondOK(w, profile)
}

func HandleMemoryProjectsList(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r.Context())
	if user == nil {
		respondUnauthorized(w)
		return
	}

	// TODO: query database for user projects
	_ = user
	projects := []Project{}

	respondOK(w, map[string]interface{}{
		"projects": projects,
	})
}

func HandleMemoryProjectGet(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	user := GetUser(r.Context())
	if user == nil {
		respondUnauthorized(w)
		return
	}

	// TODO: query database for specific project
	_ = user
	project := &Project{
		Name: name,
		Logs: []ProjectLog{},
	}

	respondOK(w, project)
}

func HandleMemoryProjectLog(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	user := GetUser(r.Context())
	if user == nil {
		respondUnauthorized(w)
		return
	}

	var req ProjectLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		respondValidationError(w, "message", "message is required")
		return
	}

	if req.Level == "" {
		req.Level = "info"
	}

	// TODO: insert log entry into database
	_ = user
	logEntry := &ProjectLog{
		ID:       "generated-id",
		Message:  req.Message,
		Level:    req.Level,
		Metadata: req.Metadata,
	}

	respondCreated(w, map[string]interface{}{
		"project": name,
		"log":     logEntry,
	})
}
