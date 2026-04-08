package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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

func (s *Server) handleConnectionsList(w http.ResponseWriter, r *http.Request) {
	if s.ConnectionService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "connection service not configured")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	conns, err := s.ConnectionService.ListByUser(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{
		"connections": conns,
	})
}

func (s *Server) handleConnectionsCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.Type == "" {
		respondValidationError(w, "name,type", "name and type are required")
		return
	}
	if s.ConnectionService == nil {
		respondNotConfigured(w, "connection service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	conn, rawKey, err := s.ConnectionService.Create(r.Context(), userID, req.Name, req.Type, req.TrustLevel)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondCreated(w, map[string]interface{}{
		"connection": conn,
		"api_key":    rawKey,
	})
}

func (s *Server) handleConnectionsUpdate(w http.ResponseWriter, r *http.Request) {
	if s.ConnectionService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "connection service not configured")
		return
	}
	idStr := chi.URLParam(r, "id")
	connID, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid connection ID")
		return
	}

	if _, ok := userIDFromCtx(r.Context()); !ok {
		respondUnauthorized(w)
		return
	}

	var req UpdateConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	conn, err := s.ConnectionService.Update(r.Context(), connID, req.Name, req.TrustLevel)
	if err != nil {
		respondNotFound(w, "connection")
		return
	}

	respondOK(w, conn)
}

func (s *Server) handleConnectionsDelete(w http.ResponseWriter, r *http.Request) {
	if s.ConnectionService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "connection service not configured")
		return
	}
	idStr := chi.URLParam(r, "id")
	connID, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid connection ID")
		return
	}

	if _, ok := userIDFromCtx(r.Context()); !ok {
		respondUnauthorized(w)
		return
	}

	if err := s.ConnectionService.Delete(r.Context(), connID); err != nil {
		respondNotFound(w, "connection")
		return
	}

	respondOK(w, map[string]string{"status": "deleted", "id": idStr})
}
