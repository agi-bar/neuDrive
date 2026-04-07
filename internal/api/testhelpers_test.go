package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/agi-bar/agenthub/internal/auth"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const testJWTSecret = "test-secret-key-for-unit-tests-only"

var testUserID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

const testUserSlug = "testuser"

// ---------------------------------------------------------------------------
// In-memory token store
// ---------------------------------------------------------------------------

type inMemoryTokenStore struct {
	tokens  map[uuid.UUID]models.ScopedToken
	byHash  map[string]uuid.UUID
	rawByID map[uuid.UUID]string
}

func newInMemoryTokenStore() *inMemoryTokenStore {
	return &inMemoryTokenStore{
		tokens:  make(map[uuid.UUID]models.ScopedToken),
		byHash:  make(map[string]uuid.UUID),
		rawByID: make(map[uuid.UUID]string),
	}
}

// ---------------------------------------------------------------------------
// Test server builder
// ---------------------------------------------------------------------------

func newTestServer() (*httptest.Server, *inMemoryTokenStore) {
	store := newInMemoryTokenStore()
	s := &Server{
		Router:    chi.NewRouter(),
		JWTSecret: testJWTSecret,
	}
	s.setupTestRoutes(store)
	ts := httptest.NewServer(s.Router)
	return ts, store
}

func newTestServerWithFileTree(fileTree *services.FileTreeService) (*httptest.Server, *inMemoryTokenStore) {
	store := newInMemoryTokenStore()
	s := &Server{
		Router:          chi.NewRouter(),
		JWTSecret:       testJWTSecret,
		FileTreeService: fileTree,
	}
	s.setupTestRoutes(store)
	ts := httptest.NewServer(s.Router)
	return ts, store
}

func (s *Server) setupTestRoutes(store *inMemoryTokenStore) {
	r := s.Router

	// Add panic recovery for tests (services are nil, handlers may panic)
	r.Use(PanicRecoveryMiddleware)

	// Health (uses respondOK -- from the real handler)
	r.Get("/api/health", s.healthCheck)

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)

		// Auth me (custom test handler using respondOK envelope)
		r.Get("/api/auth/me", func(w http.ResponseWriter, req *http.Request) {
			tokenStr, err := auth.ExtractTokenFromHeader(req)
			if err != nil {
				respondUnauthorized(w)
				return
			}
			claims, err := auth.ValidateToken(tokenStr, s.JWTSecret)
			if err != nil {
				respondUnauthorized(w)
				return
			}
			respondOK(w, map[string]interface{}{
				"id":           claims.UserID,
				"slug":         claims.Slug,
				"display_name": claims.Slug,
				"email":        "",
				"timezone":     "UTC",
				"language":     "en",
			})
		})

		// File tree
		r.Get("/api/tree/*", s.handleTreeRead)
		r.Put("/api/tree/*", s.handleTreeWrite)
		r.Delete("/api/tree/*", s.handleTreeDelete)
		r.Get("/api/search", s.handleSearch)

		// Vault
		r.Get("/api/vault/scopes", s.HandleVaultListScopes)
		r.Get("/api/vault/{scope}", s.HandleVaultRead)
		r.Put("/api/vault/{scope}", s.HandleVaultWrite)

		// Memory
		r.Get("/api/memory/profile", s.handleMemoryProfileGet)
		r.Put("/api/memory/profile", s.handleMemoryProfileUpdate)
		r.Post("/api/import/skills", s.HandleImportSkills)

		// Projects
		r.Get("/api/projects", s.handleListProjects)
		r.Post("/api/projects", s.handleCreateProject)
		r.Get("/api/projects/{name}", s.handleGetProject)
		r.Post("/api/projects/{name}/log", s.handleAppendProjectLog)

		// Inbox
		r.Get("/api/inbox/{role}", s.handleInboxList)
		r.Post("/api/inbox/send", s.handleInboxSend)
		r.Put("/api/inbox/{id}/archive", s.handleInboxArchive)

		// Connections
		r.Get("/api/connections", s.handleConnectionsList)
		r.Post("/api/connections", s.handleConnectionsCreate)
		r.Put("/api/connections/{id}", s.handleConnectionsUpdate)
		r.Delete("/api/connections/{id}", s.handleConnectionsDelete)

		// Devices
		r.Get("/api/devices", s.handleDevicesList)
		r.Post("/api/devices", s.handleRegisterDevice)
		r.Post("/api/devices/{name}/call", s.handleDeviceCall)

		// Roles
		r.Get("/api/roles", s.handleRolesList)
		r.Post("/api/roles", s.handleRolesCreate)
		r.Delete("/api/roles/{name}", s.handleRolesDelete)

		// Projects (archive)
		r.Put("/api/projects/{name}/archive", s.handleArchiveProject)

		// Collaborations
		r.Get("/api/collaborations", s.handleListCollaborations)

		// Memory conflicts
		r.Get("/api/memory/conflicts", s.handleListConflicts)

		// Dashboard (custom test handler using respondOK)
		r.Get("/api/dashboard/stats", func(w http.ResponseWriter, req *http.Request) {
			_, ok := userIDFromCtx(req.Context())
			if !ok {
				respondUnauthorized(w)
				return
			}
			respondOK(w, map[string]interface{}{
				"connections":     0,
				"skills":          0,
				"devices":         0,
				"projects":        0,
				"weekly_activity": []interface{}{},
				"pending":         []interface{}{},
			})
		})

		// Tokens (in-memory store)
		r.Post("/api/tokens", store.handleCreateToken)
		r.Get("/api/tokens", store.handleListTokens)
		r.Get("/api/tokens/scopes", s.handleListScopes)
		r.Get("/api/tokens/{id}", store.handleGetToken)
		r.Put("/api/tokens/{id}", store.handleUpdateToken)
		r.Delete("/api/tokens/{id}", store.handleRevokeToken)
	})
}

// ---------------------------------------------------------------------------
// In-memory token handler implementations
// ---------------------------------------------------------------------------

func (st *inMemoryTokenStore) handleCreateToken(w http.ResponseWriter, r *http.Request) {
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
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "name is required")
		return
	}
	if len(req.Scopes) == 0 {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "at least one scope is required")
		return
	}
	if req.MaxTrustLevel < 1 || req.MaxTrustLevel > 4 {
		req.MaxTrustLevel = 3
	}
	if req.ExpiresInDays < 1 {
		req.ExpiresInDays = 30
	}

	rawToken, tokenHash, tokenPrefix, _ := services.GenerateAPIKey()
	rawToken = "aht_" + rawToken[4:]

	id := uuid.New()
	now := time.Now().UTC()
	token := models.ScopedToken{
		ID:            id,
		UserID:        userID,
		Name:          req.Name,
		TokenHash:     tokenHash,
		TokenPrefix:   tokenPrefix,
		Scopes:        req.Scopes,
		MaxTrustLevel: req.MaxTrustLevel,
		ExpiresAt:     now.Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour),
		RateLimit:     1000,
		CreatedAt:     now,
	}
	st.tokens[id] = token
	st.byHash[tokenHash] = id
	st.rawByID[id] = rawToken

	respondCreated(w, models.CreateTokenResponse{
		Token:       rawToken,
		TokenPrefix: tokenPrefix,
		ScopedToken: token.ToResponse(),
	})
}

func (st *inMemoryTokenStore) handleListTokens(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	var tokens []models.ScopedTokenResponse
	for _, t := range st.tokens {
		if t.UserID == userID {
			tokens = append(tokens, t.ToResponse())
		}
	}
	if tokens == nil {
		tokens = []models.ScopedTokenResponse{}
	}
	respondOK(w, map[string]interface{}{"tokens": tokens})
}

func (st *inMemoryTokenStore) handleGetToken(w http.ResponseWriter, r *http.Request) {
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
	t, exists := st.tokens[tokenID]
	if !exists || t.UserID != userID {
		respondNotFound(w, "token")
		return
	}
	respondOK(w, t.ToResponse())
}

func (st *inMemoryTokenStore) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
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
	t, exists := st.tokens[tokenID]
	if !exists || t.UserID != userID || t.RevokedAt != nil {
		respondNotFound(w, "token")
		return
	}
	now := time.Now().UTC()
	t.RevokedAt = &now
	st.tokens[tokenID] = t
	respondOK(w, map[string]string{"status": "revoked"})
}

func (st *inMemoryTokenStore) handleUpdateToken(w http.ResponseWriter, r *http.Request) {
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

	var req models.UpdateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "name is required")
		return
	}

	t, exists := st.tokens[tokenID]
	if !exists || t.UserID != userID {
		respondNotFound(w, "token")
		return
	}

	t.Name = req.Name
	st.tokens[tokenID] = t
	respondOK(w, t.ToResponse())
}

// ---------------------------------------------------------------------------
// Test JWT helper
// ---------------------------------------------------------------------------

func generateTestJWT() string {
	token, err := auth.GenerateToken(testUserID, testUserSlug, testJWTSecret)
	if err != nil {
		panic("failed to generate test JWT: " + err.Error())
	}
	return token
}

// ---------------------------------------------------------------------------
// Request helpers
// ---------------------------------------------------------------------------

func authGet(ts *httptest.Server, path string) (*http.Response, error) {
	return authRequest(ts, http.MethodGet, path, nil)
}

func authPost(ts *httptest.Server, path string, body interface{}) (*http.Response, error) {
	return authRequest(ts, http.MethodPost, path, body)
}

func authPut(ts *httptest.Server, path string, body interface{}) (*http.Response, error) {
	return authRequest(ts, http.MethodPut, path, body)
}

func authDelete(ts *httptest.Server, path string) (*http.Response, error) {
	return authRequest(ts, http.MethodDelete, path, nil)
}

func authRequest(ts *httptest.Server, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, ts.URL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+generateTestJWT())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return http.DefaultClient.Do(req)
}

// parseJSON decodes a JSON response body into a map.
// For successful responses wrapped in {"ok": true, "data": {...}},
// it returns the inner "data" map. For error or unwrapped responses
// it returns the top-level map.
func parseJSON(resp *http.Response) map[string]interface{} {
	defer resp.Body.Close()
	var raw map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&raw)
	if raw == nil {
		return map[string]interface{}{}
	}
	// Unwrap APISuccess envelope: {"ok": true, "data": {...}}
	if okVal, hasOK := raw["ok"]; hasOK {
		if ok, isBool := okVal.(bool); isBool && ok {
			if data, hasData := raw["data"]; hasData {
				if dataMap, isMap := data.(map[string]interface{}); isMap {
					return dataMap
				}
			}
		}
	}
	return raw
}

// parseJSONRaw decodes a JSON response body without envelope unwrapping.
func parseJSONRaw(resp *http.Response) map[string]interface{} {
	defer resp.Body.Close()
	var raw map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&raw)
	if raw == nil {
		return map[string]interface{}{}
	}
	return raw
}
