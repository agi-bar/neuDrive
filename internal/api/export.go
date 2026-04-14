package api

import (
	"fmt"
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// GET /api/export/zip
// ---------------------------------------------------------------------------

// handleExportZip streams a zip archive of all user data as a download.
func (s *Server) handleExportZip(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if s.ExportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "export service not configured")
		return
	}

	// Generate a filename with a timestamp.
	filename := fmt.Sprintf("neudrive-export-%s.zip", time.Now().UTC().Format("2006-01-02"))

	// Set headers for zip download. We stream directly, so no Content-Length.
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if err := s.ExportService.ExportToZip(r.Context(), userID, w); err != nil {
		// If we haven't written any bytes yet, we can still send an error.
		// But since we're streaming, headers are already sent once Write is called.
		// Log the error; the client will get a truncated zip.
		http.Error(w, "export failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// ---------------------------------------------------------------------------
// GET /api/export/json
// ---------------------------------------------------------------------------

// handleExportJSON returns all user data as a single JSON document.
func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if s.ExportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "export service not configured")
		return
	}

	data, err := s.ExportService.ExportToJSON(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, data)
}
