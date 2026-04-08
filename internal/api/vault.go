package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/go-chi/chi/v5"
)

type VaultEntry struct {
	Scope     string `json:"scope"`
	Data      string `json:"data"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type VaultWriteRequest struct {
	Data          string `json:"data"`
	Description   string `json:"description,omitempty"`
	MinTrustLevel *int   `json:"min_trust_level,omitempty"`
}

func (s *Server) HandleVaultListScopes(w http.ResponseWriter, r *http.Request) {
	if s.VaultService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "vault service not configured")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	trustLevel := trustLevelFromCtx(r.Context())

	scopes, err := s.VaultService.ListScopes(r.Context(), userID, trustLevel)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{
		"scopes": scopes,
	})
}

func (s *Server) HandleVaultRead(w http.ResponseWriter, r *http.Request) {
	if s.VaultService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "vault service not configured")
		return
	}
	scope := chi.URLParam(r, "scope")

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	trustLevel := trustLevelFromCtx(r.Context())

	plaintext, err := s.VaultService.Read(r.Context(), userID, scope, trustLevel)
	if err != nil {
		respondNotFound(w, "vault entry")
		return
	}
	if s.WebhookService != nil {
		go s.WebhookService.Trigger(context.Background(), userID, models.EventVaultAccess, map[string]interface{}{
			"scope":       scope,
			"trust_level": trustLevel,
		})
	}

	entry := &VaultEntry{
		Scope: scope,
		Data:  plaintext,
	}

	respondOK(w, entry)
}

func (s *Server) HandleVaultWrite(w http.ResponseWriter, r *http.Request) {
	if s.VaultService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "vault service not configured")
		return
	}
	scope := chi.URLParam(r, "scope")

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req VaultWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	minTrust := models.TrustLevelFull
	if req.MinTrustLevel != nil {
		minTrust = *req.MinTrustLevel
	}

	if err := s.VaultService.Write(r.Context(), userID, scope, req.Data, req.Description, minTrust); err != nil {
		respondInternalError(w, err)
		return
	}

	entry := &VaultEntry{
		Scope: scope,
		Data:  req.Data,
	}

	respondOK(w, entry)
}

func (s *Server) HandleVaultDelete(w http.ResponseWriter, r *http.Request) {
	if s.VaultService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "vault service not configured")
		return
	}
	scope := chi.URLParam(r, "scope")

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if err := s.VaultService.Delete(r.Context(), userID, scope); err != nil {
		respondNotFound(w, "vault entry")
		return
	}

	respondOK(w, map[string]string{"status": "deleted", "scope": scope})
}
