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
		respondUnauthorized(w)
		return
	}

	conflicts, err := s.MemoryService.ListConflicts(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if conflicts == nil {
		conflicts = []models.MemoryConflict{}
	}

	respondOK(w, map[string]interface{}{"conflicts": conflicts})
}

// handleResolveConflict resolves a specific memory conflict.
func (s *Server) handleResolveConflict(w http.ResponseWriter, r *http.Request) {
	_, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	idStr := chi.URLParam(r, "id")
	conflictID, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid conflict id")
		return
	}

	var req struct {
		Resolution string `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if err := s.MemoryService.ResolveConflict(r.Context(), conflictID, req.Resolution); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}

	respondOK(w, map[string]string{"status": "resolved", "resolution": req.Resolution})
}
