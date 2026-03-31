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
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	TrustLevel  int      `json:"trust_level"`
}

func HandleRolesList(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r.Context())
	if user == nil {
		respondUnauthorized(w)
		return
	}

	// TODO: query database for roles belonging to user
	roles := []Role{}

	respondOK(w, map[string]interface{}{
		"roles": roles,
	})
}

func HandleRolesCreate(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r.Context())
	if user == nil {
		respondUnauthorized(w)
		return
	}

	var req CreateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, "name", "role name is required")
		return
	}

	// TODO: insert role into database
	_ = user
	role := &Role{
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions,
		TrustLevel:  req.TrustLevel,
	}

	respondCreated(w, role)
}

func HandleRolesDelete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	user := GetUser(r.Context())
	if user == nil {
		respondUnauthorized(w)
		return
	}

	// TODO: delete role from database
	_ = user
	respondOK(w, map[string]string{"status": "deleted", "name": name})
}
