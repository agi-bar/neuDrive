package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/agi-bar/agenthub/internal/auth"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	vaultpkg "github.com/agi-bar/agenthub/internal/vault"
	"github.com/google/uuid"
)

// Context keys for authenticated request state.
type contextKey string

const (
	ctxKeyUserID      contextKey = "user_id"
	ctxKeyUserSlug    contextKey = "user_slug"
	ctxKeyConnection  contextKey = "connection"
	ctxKeyTrustLevel  contextKey = "trust_level"
	ctxKeyScopedToken contextKey = "scoped_token"
	ctxKeyScopes      contextKey = "scopes"
)

// Server holds the HTTP router and all service dependencies.
type Server struct {
	Router             *chi.Mux
	UserService        *services.UserService
	AuthService        *services.AuthService
	ConnectionService  *services.ConnectionService
	FileTreeService    *services.FileTreeService
	VaultService       *services.VaultService
	MemoryService      *services.MemoryService
	DeviceService      *services.DeviceService
	ProjectService     *services.ProjectService
	TokenService       *services.TokenService
	ImportService      *services.ImportService
	Vault              *vaultpkg.Vault
	AuthHandler        *auth.Handler
	JWTSecret          string
	GitHubClientID     string
	GitHubClientSecret string
}

// NewServer creates a fully wired Server with routes configured.
func NewServer(
	userSvc *services.UserService,
	authSvc *services.AuthService,
	connSvc *services.ConnectionService,
	vault *vaultpkg.Vault,
	jwtSecret string,
	ghClientID string,
	ghClientSecret string,
) *Server {
	s := &Server{
		Router:             chi.NewRouter(),
		UserService:        userSvc,
		AuthService:        authSvc,
		ConnectionService:  connSvc,
		Vault:              vault,
		JWTSecret:          jwtSecret,
		GitHubClientID:     ghClientID,
		GitHubClientSecret: ghClientSecret,
	}
	s.AuthHandler = auth.NewHandler(userSvc, authSvc, jwtSecret, ghClientID, ghClientSecret)
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	r := s.Router

	// Global middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-API-Key"},
		AllowCredentials: true,
	}))

	// Health
	r.Get("/api/health", s.healthCheck)

	// Auth (public)
	r.Post("/api/auth/register", s.AuthHandler.HandleRegister)
	r.Post("/api/auth/login", s.AuthHandler.HandleLogin)
	r.Post("/api/auth/refresh", s.AuthHandler.HandleRefresh)
	r.Post("/api/auth/logout", s.AuthHandler.HandleLogout)
	r.Post("/api/auth/github/callback", s.AuthHandler.HandleGitHubCallback)
	r.Post("/api/auth/token/dev", s.AuthHandler.HandleDevToken)

	// Authenticated routes (JWT Bearer)
	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)

		r.Get("/api/auth/me", s.AuthHandler.HandleMe)
		r.Put("/api/auth/me", s.AuthHandler.HandleUpdateMe)
		r.Post("/api/auth/change-password", s.AuthHandler.HandleChangePassword)
		r.Get("/api/auth/sessions", s.AuthHandler.HandleListSessions)
		r.Delete("/api/auth/sessions/{id}", s.AuthHandler.HandleRevokeSession)

		// File tree
		r.Get("/api/tree/*", HandleTreeRead)
		r.Put("/api/tree/*", HandleTreeWrite)
		r.Delete("/api/tree/*", HandleTreeDelete)
		r.Get("/api/search", HandleSearch)

		// Vault
		r.Get("/api/vault/scopes", HandleVaultListScopes)
		r.Get("/api/vault/{scope}", HandleVaultRead)
		r.Put("/api/vault/{scope}", HandleVaultWrite)
		r.Delete("/api/vault/{scope}", HandleVaultDelete)

		// Connections
		r.Get("/api/connections", HandleConnectionsList)
		r.Post("/api/connections", HandleConnectionsCreate)
		r.Put("/api/connections/{id}", HandleConnectionsUpdate)
		r.Delete("/api/connections/{id}", HandleConnectionsDelete)

		// Roles
		r.Get("/api/roles", HandleRolesList)
		r.Post("/api/roles", HandleRolesCreate)
		r.Delete("/api/roles/{name}", HandleRolesDelete)

		// Memory
		r.Get("/api/memory/profile", HandleMemoryProfileGet)
		r.Put("/api/memory/profile", HandleMemoryProfileUpdate)
		r.Get("/api/memory/scratch", s.handleGetScratch)

		// Projects
		r.Get("/api/projects", s.handleListProjects)
		r.Post("/api/projects", s.handleCreateProject)
		r.Get("/api/projects/{name}", s.handleGetProject)
		r.Post("/api/projects/{name}/log", s.handleAppendProjectLog)
		r.Put("/api/projects/{name}/archive", s.handleArchiveProject)

		// Inbox
		r.Get("/api/inbox/{role}", HandleInboxList)
		r.Post("/api/inbox/send", HandleInboxSend)
		r.Put("/api/inbox/{id}/archive", HandleInboxArchive)

		// Devices
		r.Get("/api/devices", HandleDevicesList)
		r.Post("/api/devices", s.handleRegisterDevice)
		r.Post("/api/devices/{name}/call", HandleDeviceCall)

		// Dashboard
		r.Get("/api/dashboard/stats", s.handleDashboardStats)

		// Import / Export (legacy)
		r.Post("/api/import/skills", s.HandleImportSkills)
		r.Post("/api/import/vault", s.HandleImportVault)
		r.Post("/api/import/devices", s.HandleImportDevices)
		r.Post("/api/import/full", s.HandleImportFull)
		r.Get("/api/export/full", s.HandleExportFull)

		// Import / Export (bulk API)
		r.Post("/api/import/skill", s.handleImportSkill)
		r.Post("/api/import/claude-memory", s.handleImportClaudeMemoryV2)
		r.Post("/api/import/profile", s.handleImportProfileV2)
		r.Post("/api/import/bulk", s.handleImportBulk)
		r.Get("/api/export/all", s.handleExportAll)

		// Tokens (scoped access tokens)
		r.Post("/api/tokens", s.handleCreateToken)
		r.Get("/api/tokens", s.handleListTokens)
		r.Get("/api/tokens/scopes", s.handleListScopes)
		r.Get("/api/tokens/{id}", s.handleGetToken)
		r.Delete("/api/tokens/{id}", s.handleRevokeToken)
		r.Post("/api/tokens/validate", s.handleValidateToken)
	})

	// Agent API (authenticated via X-API-Key)
	r.Group(func(r chi.Router) {
		r.Use(s.apiKeyMiddleware)

		r.Get("/agent/tree/*", s.handleAgentTreeList)
		r.Get("/agent/search", s.handleAgentSearch)
		r.Put("/agent/tree/*", s.handleAgentTreeWrite)
		r.Get("/agent/vault/{scope}", s.handleAgentVaultRead)
		r.Get("/agent/inbox/{role}", s.handleAgentGetInbox)
		r.Post("/agent/inbox/send", s.handleAgentSendMessage)
		r.Post("/agent/devices/{name}/call", s.handleAgentCallDevice)
		r.Get("/agent/memory/profile", s.handleAgentGetProfile)

		// Agent Import (bulk API)
		r.Post("/agent/import/skill", s.handleAgentImportSkill)
		r.Post("/agent/import/claude-memory", s.handleAgentImportClaudeMemory)
		r.Post("/agent/import/bulk", s.handleAgentImportBulk)
	})
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// authMiddleware checks for a Bearer JWT token first, then falls back to
// X-API-Key. On success it stores user_id and user_slug in the context.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try Bearer JWT first.
		tokenStr, err := auth.ExtractTokenFromHeader(r)
		if err == nil {
			claims, err := auth.ValidateToken(tokenStr, s.JWTSecret)
			if err == nil {
				ctx := context.WithValue(r.Context(), ctxKeyUserID, claims.UserID)
				ctx = context.WithValue(ctx, ctxKeyUserSlug, claims.Slug)
				ctx = context.WithValue(ctx, ctxKeyTrustLevel, models.TrustLevelFull)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Fall back to X-API-Key.
		apiKey := auth.ExtractAPIKey(r)
		if apiKey != "" {
			conn, err := s.lookupConnection(r.Context(), apiKey)
			if err == nil {
				ctx := context.WithValue(r.Context(), ctxKeyUserID, conn.UserID)
				ctx = context.WithValue(ctx, ctxKeyConnection, conn)
				ctx = context.WithValue(ctx, ctxKeyTrustLevel, conn.TrustLevel)
				// Fire-and-forget last_used_at update.
				go func() {
					if err := s.ConnectionService.UpdateLastUsed(context.Background(), conn.ID); err != nil {
						log.Printf("warning: failed to update last_used_at for connection %s: %v", conn.ID, err)
					}
				}()
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid authentication"})
	})
}

// apiKeyMiddleware authenticates requests for the Agent API.
// It supports:
//  1. Authorization: Bearer aht_xxxxx (scoped token — checked first)
//  2. X-API-Key: aht_xxxxx (scoped token via API key header)
//  3. X-API-Key: ahk_xxxxx (connection API key — legacy fallback)
//
// For scoped tokens: validates the token, checks rate limit, derives trust
// level from the token's max_trust_level, and injects scopes into context.
func (s *Server) apiKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Step 1: Check Authorization: Bearer aht_xxxxx header first.
		if bearerToken, err := auth.ExtractTokenFromHeader(r); err == nil {
			if strings.HasPrefix(bearerToken, "aht_") && s.TokenService != nil {
				s.handleScopedTokenAuth(w, r, next, bearerToken)
				return
			}
		}

		// Step 2: Check X-API-Key header.
		apiKey := auth.ExtractAPIKey(r)
		if apiKey == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authentication: provide Authorization: Bearer aht_xxx or X-API-Key header"})
			return
		}

		// Step 2a: Scoped token via X-API-Key (aht_ prefix).
		if strings.HasPrefix(apiKey, "aht_") && s.TokenService != nil {
			s.handleScopedTokenAuth(w, r, next, apiKey)
			return
		}

		// Step 2b: Legacy connection API key (ahk_ prefix or others).
		conn, err := s.lookupConnection(r.Context(), apiKey)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid API key"})
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyUserID, conn.UserID)
		ctx = context.WithValue(ctx, ctxKeyConnection, conn)
		ctx = context.WithValue(ctx, ctxKeyTrustLevel, conn.TrustLevel)

		// Fire-and-forget last_used_at update.
		go func() {
			if err := s.ConnectionService.UpdateLastUsed(context.Background(), conn.ID); err != nil {
				log.Printf("warning: failed to update last_used_at for connection %s: %v", conn.ID, err)
			}
		}()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// handleScopedTokenAuth validates a scoped token, checks rate limit,
// and sets context values. Writes an error response on failure.
func (s *Server) handleScopedTokenAuth(w http.ResponseWriter, r *http.Request, next http.Handler, rawToken string) {
	token, err := s.TokenService.ValidateToken(r.Context(), rawToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
		return
	}

	// Check rate limit.
	if err := s.TokenService.CheckRateLimit(r.Context(), token); err != nil {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
		return
	}

	ctx := context.WithValue(r.Context(), ctxKeyUserID, token.UserID)
	ctx = context.WithValue(ctx, ctxKeyScopedToken, token)
	ctx = context.WithValue(ctx, ctxKeyTrustLevel, token.MaxTrustLevel)
	ctx = context.WithValue(ctx, ctxKeyScopes, token.Scopes)
	next.ServeHTTP(w, r.WithContext(ctx))
}

// requireScope returns a middleware that checks whether the current request
// has the specified scope. If authentication was via a scoped token, the scope
// must be present (or the token must have ScopeAdmin). If authentication was
// via a legacy connection API key or JWT (no scopes in context), the request
// passes through (scopes are not enforced for those auth methods).
func requireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := scopedTokenFromCtx(r.Context())
			if token != nil {
				// Scoped token: enforce scope check.
				if !models.HasScope(token.Scopes, scope) {
					writeJSON(w, http.StatusForbidden, map[string]string{
						"error": "token missing required scope: " + scope,
					})
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// lookupConnection hashes the raw API key and looks it up in the connections table.
func (s *Server) lookupConnection(ctx context.Context, rawKey string) (*models.Connection, error) {
	hash := sha256.Sum256([]byte(rawKey))
	hashedKey := hex.EncodeToString(hash[:])
	return s.ConnectionService.GetByAPIKey(ctx, hashedKey)
}

// ---------------------------------------------------------------------------
// Context helpers
// ---------------------------------------------------------------------------

func userIDFromCtx(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ctxKeyUserID).(uuid.UUID)
	return id, ok
}

func connectionFromCtx(ctx context.Context) *models.Connection {
	c, _ := ctx.Value(ctxKeyConnection).(*models.Connection)
	return c
}

func trustLevelFromCtx(ctx context.Context) int {
	tl, ok := ctx.Value(ctxKeyTrustLevel).(int)
	if !ok {
		return 0
	}
	return tl
}

func scopedTokenFromCtx(ctx context.Context) *models.ScopedToken {
	t, _ := ctx.Value(ctxKeyScopedToken).(*models.ScopedToken)
	return t
}

func scopesFromCtx(ctx context.Context) []string {
	s, _ := ctx.Value(ctxKeyScopes).([]string)
	return s
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"service": "agenthub",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

func (s *Server) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// Basic stats - counts from connections for now.
	conns, _ := s.ConnectionService.ListByUser(r.Context(), userID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"file_count":         0,
		"vault_scopes":       0,
		"connections":        len(conns),
		"roles":              0,
		"projects":           0,
		"unread_messages":    0,
		"registered_devices": 0,
	})
}

// ---------------------------------------------------------------------------
// Stub handlers for routes that reference services not yet wired
// ---------------------------------------------------------------------------

func (s *Server) handleGetScratch(w http.ResponseWriter, r *http.Request) {
	_, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"scratch": map[string]interface{}{}})
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	_, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"projects": []interface{}{}})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	_, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	_, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"name": name, "logs": []interface{}{}})
}

func (s *Server) handleAppendProjectLog(w http.ResponseWriter, r *http.Request) {
	_, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "appended"})
}

func (s *Server) handleArchiveProject(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	_, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived", "name": name})
}

func (s *Server) handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	_, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "registered"})
}

// ---------------------------------------------------------------------------
// Agent API handlers — authenticated via X-API-Key or Bearer scoped token
// ---------------------------------------------------------------------------

// agentCheckAuth verifies the request is authenticated (via connection or scoped token)
// and that the trust level meets the minimum. For scoped tokens, also checks the required scope.
func (s *Server) agentCheckAuth(w http.ResponseWriter, r *http.Request, minTrust int, requiredScope string) bool {
	if _, ok := userIDFromCtx(r.Context()); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	if trustLevelFromCtx(r.Context()) < minTrust {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient trust level"})
		return false
	}
	// For scoped tokens, check the required scope.
	if token := scopedTokenFromCtx(r.Context()); token != nil && requiredScope != "" {
		if !models.HasScope(token.Scopes, requiredScope) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "token missing required scope: " + requiredScope})
			return false
		}
	}
	return true
}

func (s *Server) handleAgentTreeList(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadTree) {
		return
	}
	path := chi.URLParam(r, "*")
	if path == "" {
		path = "/"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"path": path, "children": []interface{}{}})
}

func (s *Server) handleAgentSearch(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeSearch) {
		return
	}
	query := r.URL.Query().Get("q")
	writeJSON(w, http.StatusOK, map[string]interface{}{"query": query, "results": []interface{}{}})
}

func (s *Server) handleAgentTreeWrite(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeWriteTree) {
		return
	}
	path := chi.URLParam(r, "*")
	writeJSON(w, http.StatusOK, map[string]interface{}{"path": path, "status": "written"})
}

func (s *Server) handleAgentVaultRead(w http.ResponseWriter, r *http.Request) {
	if _, ok := userIDFromCtx(r.Context()); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	trustLevel := trustLevelFromCtx(r.Context())
	scope := chi.URLParam(r, "scope")

	// Vault requires at least Work level; personal scope requires Full.
	if trustLevel < models.TrustLevelWork {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient trust level"})
		return
	}
	if strings.HasPrefix(scope, "personal") && trustLevel < models.TrustLevelFull {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient trust level for personal vault"})
		return
	}

	// For scoped tokens, check the specific vault sub-scope.
	if token := scopedTokenFromCtx(r.Context()); token != nil {
		requiredScope := models.ScopeReadVault
		if strings.HasPrefix(scope, "auth") {
			requiredScope = models.ScopeReadVaultAuth
		}
		if !models.HasScope(token.Scopes, requiredScope) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "token missing required scope: " + requiredScope})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"scope": scope, "data": ""})
}

func (s *Server) handleAgentGetInbox(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadInbox) {
		return
	}
	role := chi.URLParam(r, "role")
	writeJSON(w, http.StatusOK, map[string]interface{}{"role": role, "messages": []interface{}{}})
}

func (s *Server) handleAgentSendMessage(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeWriteInbox) {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "sent"})
}

func (s *Server) handleAgentCallDevice(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeCallDevices) {
		return
	}
	name := chi.URLParam(r, "name")
	writeJSON(w, http.StatusOK, map[string]interface{}{"device": name, "status": "dispatched"})
}

func (s *Server) handleAgentGetProfile(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelGuest, models.ScopeReadProfile) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	user, err := s.UserService.GetByID(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"slug":         user.Slug,
		"display_name": user.DisplayName,
		"timezone":     user.Timezone,
		"language":     user.Language,
	})
}
