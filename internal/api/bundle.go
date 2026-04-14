package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
)

func (s *Server) handleAgentImportBundle(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	if s.SyncService == nil && s.ImportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "sync service not configured")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	var bundle models.Bundle
	if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	var (
		result *models.BundleImportResult
		err    error
	)
	if s.SyncService != nil {
		result, err = s.SyncService.ImportBundleJSON(r.Context(), userID, bundle)
	} else {
		result, err = s.ImportService.ImportBundle(r.Context(), userID, bundle)
	}
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}

	respondOKWithLocalGitSync(w, result, s.syncLocalGitMirror(r.Context(), userID))
}

func (s *Server) handleAgentPreviewBundle(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	if s.SyncService == nil && s.ImportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "sync service not configured")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	result, err := s.decodeAndPreviewBundle(r.Context(), userID, body)
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
	if s.SyncService == nil && s.ExportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "sync service not configured")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	filters := parseBundleFilters(r)
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = models.BundleFormatJSON
	}

	if format == models.BundleFormatArchive {
		if s.SyncService == nil {
			respondError(w, http.StatusInternalServerError, ErrCodeInternal, "sync service not configured")
			return
		}
		archive, _, err := s.SyncService.ExportArchive(r.Context(), userID, filters)
		if err != nil {
			respondInternalError(w, err)
			return
		}
		filename := fmt.Sprintf("neudrive-sync-%s.ndrvz", time.Now().UTC().Format("2006-01-02"))
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		http.ServeContent(w, r, filename, time.Now().UTC(), bytes.NewReader(archive))
		return
	}

	var (
		bundle *models.Bundle
		err    error
	)
	if s.SyncService != nil {
		bundle, err = s.SyncService.ExportBundleJSON(r.Context(), userID, filters)
	} else {
		bundle, err = s.ExportService.ExportBundle(r.Context(), userID)
	}
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, bundle)
}

func (s *Server) decodeAndPreviewBundle(ctx context.Context, userID uuid.UUID, body []byte) (*models.BundlePreviewResult, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("preview request body is required")
	}

	var bundle models.Bundle
	if err := json.Unmarshal(body, &bundle); err == nil && bundle.Version == models.BundleVersionV1 {
		if s.SyncService != nil {
			return s.SyncService.PreviewBundle(ctx, userID, bundle)
		}
		return s.ImportService.PreviewBundle(ctx, userID, bundle)
	}

	var request models.BundlePreviewRequest
	if err := json.Unmarshal(body, &request); err == nil {
		if request.Bundle != nil {
			if s.SyncService != nil {
				return s.SyncService.PreviewBundle(ctx, userID, *request.Bundle)
			}
			return s.ImportService.PreviewBundle(ctx, userID, *request.Bundle)
		}
		if request.Manifest != nil {
			if s.SyncService == nil {
				return nil, fmt.Errorf("sync service not configured")
			}
			return s.SyncService.PreviewManifest(ctx, userID, *request.Manifest)
		}
	}

	var manifest models.BundleArchiveManifest
	if err := json.Unmarshal(body, &manifest); err == nil && manifest.Version == models.BundleVersionV2 {
		if s.SyncService == nil {
			return nil, fmt.Errorf("sync service not configured")
		}
		return s.SyncService.PreviewManifest(ctx, userID, manifest)
	}

	return nil, fmt.Errorf("unsupported preview payload")
}
