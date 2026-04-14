package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
	"github.com/go-chi/chi/v5"
)

type Device struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	LastSeen  string `json:"last_seen,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type DeviceCallRequest struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params,omitempty"`
}

type DeviceCallResponse struct {
	Device string `json:"device"`
	Action string `json:"action"`
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
}

type RegisterDeviceRequest struct {
	Name       string                 `json:"name"`
	DeviceType string                 `json:"device_type"`
	Brand      string                 `json:"brand,omitempty"`
	Protocol   string                 `json:"protocol,omitempty"`
	Endpoint   string                 `json:"endpoint,omitempty"`
	SkillMD    string                 `json:"skill_md,omitempty"`
	Config     map[string]interface{} `json:"config,omitempty"`
}

func (s *Server) handleDevicesList(w http.ResponseWriter, r *http.Request) {
	if s.DeviceService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "device service not configured")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	devices, err := s.DeviceService.List(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{
		"devices": devices,
	})
}

func (s *Server) handleDeviceCall(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var req DeviceCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.Action == "" {
		respondValidationError(w, "action", "action is required")
		return
	}
	if s.DeviceService == nil {
		respondNotConfigured(w, "device service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var params map[string]interface{}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid params format")
			return
		}
	}

	result, err := s.DeviceService.Call(r.Context(), userID, name, req.Action, params)
	if err != nil {
		respondDeviceCallError(w, err)
		return
	}

	respondOK(w, result)
}

func (s *Server) handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	var req RegisterDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.DeviceType == "" {
		respondValidationError(w, "name,device_type", "name and device_type are required")
		return
	}
	if s.DeviceService == nil {
		respondNotConfigured(w, "device service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	device := models.Device{
		Name:       req.Name,
		DeviceType: req.DeviceType,
		Brand:      req.Brand,
		Protocol:   req.Protocol,
		Endpoint:   req.Endpoint,
		SkillMD:    req.SkillMD,
		Config:     req.Config,
	}

	registered, err := s.DeviceService.Register(r.Context(), userID, device)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondCreatedWithLocalGitSync(w, registered, s.syncLocalGitMirror(r.Context(), userID))
}

func respondDeviceCallError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrDeviceInvalidRequest):
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
	case errors.Is(err, services.ErrDeviceNotFound):
		respondError(w, http.StatusNotFound, ErrCodeNotFound, err.Error())
	case errors.Is(err, services.ErrDeviceUnsupported):
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, err.Error())
	case errors.Is(err, services.ErrDeviceUpstreamFailed):
		respondError(w, http.StatusBadGateway, ErrCodeInternal, err.Error())
	default:
		respondInternalError(w, err)
	}
}
