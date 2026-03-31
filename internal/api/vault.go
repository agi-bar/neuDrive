package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type VaultEntry struct {
	Scope     string `json:"scope"`
	Data      string `json:"data"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type VaultWriteRequest struct {
	Data string `json:"data"`
}

func HandleVaultListScopes(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// TODO: query database for vault scopes belonging to user
	scopes := []string{}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"scopes": scopes,
	})
}

func HandleVaultRead(w http.ResponseWriter, r *http.Request) {
	scope := chi.URLParam(r, "scope")

	user := GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// TODO: read from database and decrypt using vault service
	_ = user
	entry := &VaultEntry{
		Scope: scope,
		Data:  "",
	}

	writeJSON(w, http.StatusOK, entry)
}

func HandleVaultWrite(w http.ResponseWriter, r *http.Request) {
	scope := chi.URLParam(r, "scope")

	user := GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req VaultWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// TODO: encrypt and store in database
	_ = user
	entry := &VaultEntry{
		Scope: scope,
		Data:  req.Data,
	}

	writeJSON(w, http.StatusOK, entry)
}

func HandleVaultDelete(w http.ResponseWriter, r *http.Request) {
	scope := chi.URLParam(r, "scope")

	user := GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// TODO: delete vault entry from database
	_ = user
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "scope": scope})
}
