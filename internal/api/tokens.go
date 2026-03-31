package api

import (
	"encoding/json"
	"net/http"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// handleCreateToken creates a new scoped access token. The raw token is returned only once.
func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req models.CreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, "name", "name is required")
		return
	}
	if len(req.Scopes) == 0 {
		respondValidationError(w, "scopes", "at least one scope is required")
		return
	}
	if req.MaxTrustLevel < 1 || req.MaxTrustLevel > 4 {
		req.MaxTrustLevel = 3 // default to Work level
	}
	if req.ExpiresInDays < 1 {
		req.ExpiresInDays = 30 // default 30 days
	}

	resp, err := s.TokenService.CreateToken(r.Context(), userID, req)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}

	respondCreated(w, resp)
}

// handleListTokens returns all tokens (active and revoked) for the authenticated user.
func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	tokens, err := s.TokenService.ListTokens(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	// Convert to response objects (excludes hashes, adds computed fields).
	responses := make([]models.ScopedTokenResponse, 0, len(tokens))
	for i := range tokens {
		responses = append(responses, tokens[i].ToResponse())
	}

	respondOK(w, map[string]interface{}{
		"tokens": responses,
	})
}

// handleGetToken returns details for a single token including usage stats and scopes.
func (s *Server) handleGetToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	idStr := chi.URLParam(r, "id")
	tokenID, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid token ID")
		return
	}

	token, err := s.TokenService.GetByID(r.Context(), tokenID, userID)
	if err != nil {
		respondNotFound(w, "token")
		return
	}

	respondOK(w, token.ToResponse())
}

// handleRevokeToken revokes a single token by ID.
func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	idStr := chi.URLParam(r, "id")
	tokenID, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid token ID")
		return
	}

	if err := s.TokenService.RevokeToken(r.Context(), userID, tokenID); err != nil {
		respondNotFound(w, "token")
		return
	}

	respondOK(w, map[string]string{"status": "revoked"})
}

// handleValidateToken allows external services to validate a scoped token.
func (s *Server) handleValidateToken(w http.ResponseWriter, r *http.Request) {
	var req models.ValidateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, models.ValidateTokenResponse{
			Valid: false,
			Error: "invalid request body",
		})
		return
	}

	if req.Token == "" {
		writeJSON(w, http.StatusBadRequest, models.ValidateTokenResponse{
			Valid: false,
			Error: "token is required",
		})
		return
	}

	token, err := s.TokenService.ValidateToken(r.Context(), req.Token)
	if err != nil {
		writeJSON(w, http.StatusOK, models.ValidateTokenResponse{
			Valid: false,
			Error: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, models.ValidateTokenResponse{
		Valid:         true,
		UserID:        token.UserID.String(),
		Scopes:        token.Scopes,
		MaxTrustLevel: token.MaxTrustLevel,
		ExpiresAt:     token.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// handleListScopes returns the available scope definitions for UI display.
func (s *Server) handleListScopes(w http.ResponseWriter, r *http.Request) {
	respondOK(w, map[string]interface{}{
		"scopes":     models.AllScopes,
		"categories": models.ScopeCategories(),
		"bundles": map[string][]string{
			"read_only": models.ScopeBundleReadOnly,
			"agent":     models.ScopeBundleAgent,
			"full":      models.ScopeBundleFull,
		},
	})
}
