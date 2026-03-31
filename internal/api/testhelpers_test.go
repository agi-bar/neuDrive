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

// testUserID is a fixed UUID used across tests.
var testUserID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

const testUserSlug = "testuser"

// ---------------------------------------------------------------------------
// Mock TokenService (wraps the real one can't be used without DB, so we
// implement the methods the handlers call via a thin shim layer).
// ---------------------------------------------------------------------------

// mockTokenService implements the subset of TokenService methods used by the
// API handlers. Because the handlers call s.TokenService.* directly on a
// concrete *services.TokenService, we cannot swap in an interface. Instead we
// build a thin Server that bypasses the real service by injecting handlers
// that manage an in-memory token store.
//
// To work around the concrete type, we create a testServer whose token
// handlers use the in-memory store directly and register them on a fresh chi
// router with identical routes.

type inMemoryTokenStore struct {
	tokens   map[uuid.UUID]models.ScopedToken
	byHash   map[string]uuid.UUID
	rawByID  map[uuid.UUID]string // raw token keyed by token ID (for validation tests)
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

// newTestServer returns an httptest.Server whose router matches the production
// routes but handlers either use in-memory state or are the real stub handlers
// from router.go (which don't need a DB for most endpoints).
func newTestServer() (*httptest.Server, *inMemoryTokenStore) {
	store := newInMemoryTokenStore()
	s := &Server{
		Router:    chi.NewRouter(),
		JWTSecret: testJWTSecret,
		// Most services are nil; handlers that are stubs (projects, dashboard, etc.)
		// only need userID from context, which our test middleware provides.
	}

	// We cannot set s.TokenService (requires DB), so we override token
	// handlers below.

	// Minimal AuthHandler for /api/auth/me.
	// We skip register/login (they need AuthService with DB).

	s.setupTestRoutes(store)

	ts := httptest.NewServer(s.Router)
	return ts, store
}

// setupTestRoutes mirrors the production routes but replaces DB-dependent
// handlers with test-friendly versions.
func (s *Server) setupTestRoutes(store *inMemoryTokenStore) {
	r := s.Router

	// Health
	r.Get("/api/health", s.healthCheck)

	// Auth endpoints (test-friendly: me only)
	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)

		r.Get("/api/auth/me", func(w http.ResponseWriter, req *http.Request) {
			tokenStr, err := auth.ExtractTokenFromHeader(req)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no token"})
				return
			}
			claims, err := auth.ValidateToken(tokenStr, s.JWTSecret)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"id":           claims.UserID,
				"slug":         claims.Slug,
				"display_name": claims.Slug,
				"email":        "",
				"timezone":     "UTC",
				"language":     "en",
			})
		})

		// File tree
		r.Get("/api/tree/*", HandleTreeRead)
		r.Put("/api/tree/*", HandleTreeWrite)
		r.Delete("/api/tree/*", HandleTreeDelete)
		r.Get("/api/search", HandleSearch)

		// Vault
		r.Get("/api/vault/scopes", HandleVaultListScopes)
		r.Get("/api/vault/{scope}", HandleVaultRead)
		r.Put("/api/vault/{scope}", HandleVaultWrite)

		// Memory
		r.Get("/api/memory/profile", HandleMemoryProfileGet)
		r.Put("/api/memory/profile", HandleMemoryProfileUpdate)

		// Projects (stubs from router.go)
		r.Get("/api/projects", s.handleListProjects)
		r.Post("/api/projects", s.handleCreateProject)
		r.Get("/api/projects/{name}", s.handleGetProject)
		r.Post("/api/projects/{name}/log", s.handleAppendProjectLog)

		// Inbox
		r.Get("/api/inbox/{role}", HandleInboxList)
		r.Post("/api/inbox/send", HandleInboxSend)
		r.Put("/api/inbox/{id}/archive", HandleInboxArchive)

		// Dashboard
		r.Get("/api/dashboard/stats", func(w http.ResponseWriter, req *http.Request) {
			_, ok := userIDFromCtx(req.Context())
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"file_count":         0,
				"vault_scopes":       0,
				"connections":        0,
				"roles":              0,
				"projects":           0,
				"unread_messages":    0,
				"registered_devices": 0,
			})
		})

		// Tokens (in-memory store)
		r.Post("/api/tokens", store.handleCreateToken)
		r.Get("/api/tokens", store.handleListTokens)
		r.Get("/api/tokens/scopes", s.handleListScopes)
		r.Get("/api/tokens/{id}", store.handleGetToken)
		r.Delete("/api/tokens/{id}", store.handleRevokeToken)
	})
}

// ---------------------------------------------------------------------------
// In-memory token handler implementations
// ---------------------------------------------------------------------------

func (st *inMemoryTokenStore) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req models.CreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if len(req.Scopes) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one scope is required"})
		return
	}
	if req.MaxTrustLevel < 1 || req.MaxTrustLevel > 4 {
		req.MaxTrustLevel = 3
	}
	if req.ExpiresInDays < 1 {
		req.ExpiresInDays = 30
	}

	rawToken, tokenHash, tokenPrefix := services.GenerateAPIKey() // re-use key gen
	rawToken = "aht_" + rawToken[4:]                              // replace prefix

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

	writeJSON(w, http.StatusCreated, models.CreateTokenResponse{
		Token:       rawToken,
		TokenPrefix: tokenPrefix,
		ScopedToken: token.ToResponse(),
	})
}

func (st *inMemoryTokenStore) handleListTokens(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
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
	writeJSON(w, http.StatusOK, map[string]interface{}{"tokens": tokens})
}

func (st *inMemoryTokenStore) handleGetToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	idStr := chi.URLParam(r, "id")
	tokenID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid token ID"})
		return
	}
	t, exists := st.tokens[tokenID]
	if !exists || t.UserID != userID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
		return
	}
	writeJSON(w, http.StatusOK, t.ToResponse())
}

func (st *inMemoryTokenStore) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	idStr := chi.URLParam(r, "id")
	tokenID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid token ID"})
		return
	}
	t, exists := st.tokens[tokenID]
	if !exists || t.UserID != userID || t.RevokedAt != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found or already revoked"})
		return
	}
	now := time.Now().UTC()
	t.RevokedAt = &now
	st.tokens[tokenID] = t
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
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
// Authenticated request helpers
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

// parseJSON decodes a response body into a map.
func parseJSON(resp *http.Response) map[string]interface{} {
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}
