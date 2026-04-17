package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/agi-bar/neudrive/internal/platforms"
	"github.com/agi-bar/neudrive/internal/runtimecfg"
	sqlitestorage "github.com/agi-bar/neudrive/internal/storage/sqlite"
)

type localPlatformDashboardRequest struct {
	Platform string `json:"platform"`
	Mode     string `json:"mode"`
}

type localPlatformDashboardImportResponse struct {
	Platform string                           `json:"platform"`
	Mode     platforms.ImportMode             `json:"mode"`
	Files    *sqlitestorage.ImportResult      `json:"files,omitempty"`
	Agent    *sqlitestorage.AgentImportResult `json:"agent,omitempty"`
}

func (s *Server) handleLocalPlatformPreview(w http.ResponseWriter, r *http.Request) {
	if !s.isLocalMode() {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local platform preview is only available in local mode")
		return
	}
	if _, ok := userIDFromCtx(r.Context()); !ok {
		respondUnauthorized(w)
		return
	}

	var req localPlatformDashboardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Platform) == "" {
		req.Platform = "claude"
	}

	cfg, err := loadRuntimeCLIConfig()
	if err != nil {
		respondInternalError(w, err)
		return
	}
	preview, err := platforms.PreviewImport(r.Context(), cfg, req.Platform, req.Mode)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, preview)
}

func (s *Server) handleLocalPlatformImport(w http.ResponseWriter, r *http.Request) {
	if !s.isLocalMode() {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local platform import is only available in local mode")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req localPlatformDashboardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Platform) == "" {
		req.Platform = "claude"
	}

	cfg, err := loadRuntimeCLIConfig()
	if err != nil {
		respondInternalError(w, err)
		return
	}
	adapter, err := platforms.Resolve(req.Platform)
	if err != nil {
		respondValidationError(w, "platform", err.Error())
		return
	}
	mode, err := platforms.ParseImportMode(adapter.ID(), req.Mode)
	if err != nil {
		respondValidationError(w, "mode", err.Error())
		return
	}

	resp := &localPlatformDashboardImportResponse{
		Platform: adapter.ID(),
		Mode:     mode,
	}

	switch mode {
	case platforms.ImportModeFiles:
		resp.Files, err = s.importLocalPlatformSources(r.Context(), userID, adapter.ID(), adapter.DiscoverSources())
	case platforms.ImportModeAgent:
		var payload sqlitestorage.AgentExportPayload
		payload, err = platforms.PrepareAgentImportPayload(r.Context(), cfg, adapter.ID())
		if err == nil {
			resp.Agent, err = s.importLocalPlatformAgentPayload(r.Context(), userID, adapter.ID(), payload)
		}
	case platforms.ImportModeAll:
		var payload sqlitestorage.AgentExportPayload
		payload, err = platforms.PrepareAgentImportPayload(r.Context(), cfg, adapter.ID())
		if err == nil {
			resp.Agent, err = s.importLocalPlatformAgentPayload(r.Context(), userID, adapter.ID(), payload)
		}
		if err == nil {
			resp.Files, err = s.importLocalPlatformSources(r.Context(), userID, adapter.ID(), adapter.DiscoverSources())
		}
	}
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}

	respondOKWithLocalGitSync(w, resp, s.syncLocalGitMirror(r.Context(), userID))
}

func loadRuntimeCLIConfig() (*runtimecfg.CLIConfig, error) {
	_, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		return nil, err
	}
	if err := runtimecfg.EnsureLocalDefaults(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
