package api

import (
	"encoding/json"
	"net/http"

	"github.com/agi-bar/agenthub/internal/models"
)

func (s *Server) handleAgentImportBundle(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	if s.ImportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "import service not configured")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	var bundle models.Bundle
	if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	result, err := s.ImportService.ImportBundle(r.Context(), userID, bundle)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}

	respondOK(w, result)
}

func (s *Server) handleAgentPreviewBundle(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	if s.ImportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "import service not configured")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	var bundle models.Bundle
	if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	result, err := s.ImportService.PreviewBundle(r.Context(), userID, bundle)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}

	respondOK(w, result)
}

func (s *Server) handleAgentExportBundle(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeReadBundle) {
		return
	}
	if s.ExportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "export service not configured")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	bundle, err := s.ExportService.ExportBundle(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, bundle)
}
