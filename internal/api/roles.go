package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Role struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	TrustLevel  int      `json:"trust_level"`
	CreatedAt   string   `json:"created_at,omitempty"`
}

type CreateRoleRequest struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	RoleType           string   `json:"role_type,omitempty"`
	AllowedPaths       []string `json:"allowed_paths,omitempty"`
	AllowedVaultScopes []string `json:"allowed_vault_scopes,omitempty"`
	Lifecycle          string   `json:"lifecycle,omitempty"`
	// Deprecated fields kept for backward compat
	Permissions []string `json:"permissions"`
	TrustLevel  int      `json:"trust_level"`
}

func (s *Server) handleRolesList(w http.ResponseWriter, r *http.Request) {
	if s.RoleService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "role service not configured")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	roles, err := s.RoleService.List(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{
		"roles": roles,
	})
}

func (s *Server) handleRolesCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, "name", "role name is required")
		return
	}
	if s.RoleService == nil {
		respondNotConfigured(w, "role service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	roleType := req.RoleType
	if roleType == "" {
		roleType = "worker"
	}
	lifecycle := req.Lifecycle
	if lifecycle == "" {
		lifecycle = "permanent"
	}
	allowedPaths := req.AllowedPaths
	if allowedPaths == nil {
		allowedPaths = []string{"/"}
	}
	allowedVaultScopes := req.AllowedVaultScopes
	if allowedVaultScopes == nil {
		allowedVaultScopes = []string{}
	}

	role, err := s.RoleService.Create(r.Context(), userID, req.Name, roleType, allowedPaths, allowedVaultScopes, lifecycle)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondCreatedWithLocalGitSync(w, role, s.syncLocalGitMirror(r.Context(), userID))
}

func (s *Server) handleRolesDelete(w http.ResponseWriter, r *http.Request) {
	if s.RoleService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "role service not configured")
		return
	}
	name := chi.URLParam(r, "name")

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if err := s.RoleService.Delete(r.Context(), userID, name); err != nil {
		respondNotFound(w, "role")
		return
	}

	respondOKWithLocalGitSync(w, map[string]string{"status": "deleted", "name": name}, s.syncLocalGitMirror(r.Context(), userID))
}
