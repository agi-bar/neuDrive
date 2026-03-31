package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Connection struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	TrustLevel int    `json:"trust_level"`
	Config     string `json:"config,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type CreateConnectionRequest struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	TrustLevel int    `json:"trust_level"`
	Config     string `json:"config,omitempty"`
}

type UpdateConnectionRequest struct {
	Name       string `json:"name,omitempty"`
	Status     string `json:"status,omitempty"`
	TrustLevel int    `json:"trust_level,omitempty"`
	Config     string `json:"config,omitempty"`
}

func HandleConnectionsList(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r.Context())
	if user == nil {
		respondUnauthorized(w)
		return
	}

	// TODO: query database for connections belonging to user
	connections := []Connection{}

	respondOK(w, map[string]interface{}{
		"connections": connections,
	})
}

func HandleConnectionsCreate(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r.Context())
	if user == nil {
		respondUnauthorized(w)
		return
	}

	var req CreateConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.Type == "" {
		respondValidationError(w, "name,type", "name and type are required")
		return
	}

	// TODO: insert connection into database
	_ = user
	conn := &Connection{
		ID:         "generated-id",
		Name:       req.Name,
		Type:       req.Type,
		Status:     "active",
		TrustLevel: req.TrustLevel,
		Config:     req.Config,
	}

	respondCreated(w, conn)
}

func HandleConnectionsUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	user := GetUser(r.Context())
	if user == nil {
		respondUnauthorized(w)
		return
	}

	var req UpdateConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	// TODO: update connection in database
	_ = user
	conn := &Connection{
		ID:         id,
		Name:       req.Name,
		Status:     req.Status,
		TrustLevel: req.TrustLevel,
		Config:     req.Config,
	}

	respondOK(w, conn)
}

func HandleConnectionsDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	user := GetUser(r.Context())
	if user == nil {
		respondUnauthorized(w)
		return
	}

	// TODO: delete connection from database
	_ = user
	respondOK(w, map[string]string{"status": "deleted", "id": id})
}
