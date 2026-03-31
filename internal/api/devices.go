package api

import (
	"encoding/json"
	"net/http"

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

func HandleDevicesList(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// TODO: query database for devices belonging to user
	devices := []Device{}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"devices": devices,
	})
}

func HandleDeviceCall(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	user := GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req DeviceCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Action == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action is required"})
		return
	}

	// TODO: dispatch device call via appropriate service
	_ = user
	resp := &DeviceCallResponse{
		Device: name,
		Action: req.Action,
		Status: "dispatched",
	}

	writeJSON(w, http.StatusOK, resp)
}
