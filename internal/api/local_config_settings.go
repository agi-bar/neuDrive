package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/agi-bar/neudrive/internal/runtimecfg"
)

type localConfigResponse struct {
	Path string `json:"path"`
	Raw  string `json:"raw"`
}

func (s *Server) handleLocalConfigGet(w http.ResponseWriter, r *http.Request) {
	if !s.systemSettingsEnabled() {
		respondForbidden(w, "system settings are disabled")
		return
	}
	if !s.isLocalMode() {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local config is only available in local mode")
		return
	}
	if _, ok := userIDFromCtx(r.Context()); !ok {
		respondUnauthorized(w)
		return
	}
	path, raw, err := runtimecfg.LoadRawConfig("")
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, localConfigResponse{Path: path, Raw: raw})
}

func (s *Server) handleLocalConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.systemSettingsEnabled() {
		respondForbidden(w, "system settings are disabled")
		return
	}
	if !s.isLocalMode() {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "local config is only available in local mode")
		return
	}
	if _, ok := userIDFromCtx(r.Context()); !ok {
		respondUnauthorized(w)
		return
	}
	var req struct {
		Raw string `json:"raw"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Raw) == "" {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "config.json cannot be empty")
		return
	}
	if err := runtimecfg.SaveRawConfig("", req.Raw); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	path, raw, err := runtimecfg.LoadRawConfig("")
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, localConfigResponse{Path: path, Raw: raw})
}
