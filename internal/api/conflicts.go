package api

import (
	"encoding/json"
	"net/http"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// handleListConflicts returns all pending memory conflicts for the authenticated user.
func (s *Server) handleListConflicts(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	conflicts, err := s.MemoryService.ListConflicts(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if conflicts == nil {
		conflicts = []models.MemoryConflict{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"conflicts": conflicts})
}

// handleResolveConflict resolves a specific memory conflict.
func (s *Server) handleResolveConflict(w http.ResponseWriter, r *http.Request) {
	_, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	idStr := chi.URLParam(r, "id")
	conflictID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid conflict id"})
		return
	}

	var req struct {
		Resolution string `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := s.MemoryService.ResolveConflict(r.Context(), conflictID, req.Resolution); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved", "resolution": req.Resolution})
}
