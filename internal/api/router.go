package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/auth"
	"github.com/agi-bar/agenthub/internal/config"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	vaultpkg "github.com/agi-bar/agenthub/internal/vault"
	"github.com/agi-bar/agenthub/internal/web"
	"github.com/go-chi/chi/v5"
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
	Router               *chi.Mux
	UserService          *services.UserService
	AuthService          *services.AuthService
	ConnectionService    *services.ConnectionService
	FileTreeService      *services.FileTreeService
	VaultService         *services.VaultService
	MemoryService        *services.MemoryService
	DeviceService        *services.DeviceService
	ProjectService       *services.ProjectService
	SummaryService       *services.SummaryService
	RoleService          *services.RoleService
	InboxService         *services.InboxService
	DashboardService     *services.DashboardService
	TokenService         *services.TokenService
	ImportService        *services.ImportService
	ExportService        *services.ExportService
	CollaborationService *services.CollaborationService
	WebhookService       *services.WebhookService
	OAuthService         *services.OAuthService
	Vault                *vaultpkg.Vault
	AuthHandler          *auth.Handler
	Config               *config.Config
	JWTSecret            string
	GitHubClientID       string
	GitHubClientSecret   string
}

// NewServer creates a fully wired Server with routes configured.
func NewServer(
	cfg *config.Config,
	userSvc *services.UserService,
	authSvc *services.AuthService,
	connSvc *services.ConnectionService,
	fileTreeSvc *services.FileTreeService,
	vaultSvc *services.VaultService,
	memorySvc *services.MemoryService,
	projectSvc *services.ProjectService,
	summarySvc *services.SummaryService,
	roleSvc *services.RoleService,
	inboxSvc *services.InboxService,
	deviceSvc *services.DeviceService,
	dashboardSvc *services.DashboardService,
	tokenSvc *services.TokenService,
	importSvc *services.ImportService,
	exportSvc *services.ExportService,
	collabSvc *services.CollaborationService,
	webhookSvc *services.WebhookService,
	oauthSvc *services.OAuthService,
	vault *vaultpkg.Vault,
	jwtSecret string,
	ghClientID string,
	ghClientSecret string,
) *Server {
	s := &Server{
		Router:               chi.NewRouter(),
		UserService:          userSvc,
		AuthService:          authSvc,
		ConnectionService:    connSvc,
		FileTreeService:      fileTreeSvc,
		VaultService:         vaultSvc,
		MemoryService:        memorySvc,
		ProjectService:       projectSvc,
		SummaryService:       summarySvc,
		RoleService:          roleSvc,
		InboxService:         inboxSvc,
		DeviceService:        deviceSvc,
		DashboardService:     dashboardSvc,
		TokenService:         tokenSvc,
		ImportService:        importSvc,
		CollaborationService: collabSvc,
		WebhookService:       webhookSvc,
		OAuthService:         oauthSvc,
		ExportService:        exportSvc,
		Vault:                vault,
		JWTSecret:            jwtSecret,
		Config:               cfg,
		GitHubClientID:       ghClientID,
		GitHubClientSecret:   ghClientSecret,
	}
	s.AuthHandler = auth.NewHandler(userSvc, authSvc, jwtSecret, ghClientID, ghClientSecret)
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	r := s.Router

	// Global middleware — applied in order:
	// 1. PanicRecovery  2. SecurityHeaders  3. CORS  4. RateLimit
	// 5. RequestID  6. Logging  7. MaxBodySize (default)
	rl := NewRateLimiter(s.Config.RateLimit, time.Minute)
	r.Use(PanicRecoveryMiddleware)
	r.Use(SecurityHeadersMiddleware)
	r.Use(CORSMiddleware(s.Config.CORSOrigins))
	r.Use(rl.Middleware)
	r.Use(RequestIDMiddleware)
	r.Use(CaptureOAuthMiddleware(s.Config))
	r.Use(LoggingMiddleware)
	r.Use(MaxBodySizeMiddleware(s.Config.MaxBodySize))

	// Health + public config
	r.Get("/api/health", s.healthCheck)
	r.Get("/api/config", s.handlePublicConfig)
	r.Get("/gpt/openapi.json", s.handleGPTOpenAPISchema)

	// Remote MCP endpoint — Streamable HTTP transport for Claude.ai Connectors
	r.HandleFunc("/mcp", s.handleMCPEndpoint)

	// OAuth 2.0 discovery for MCP (RFC 9728 + RFC 8414)
	r.Get("/.well-known/oauth-protected-resource", s.handleProtectedResourceMetadata)
	r.Get("/.well-known/oauth-protected-resource/*", s.handleProtectedResourceMetadata)
	r.Get("/.well-known/oauth-authorization-server", s.handleAuthorizationServerMetadata)
	r.Get("/.well-known/oauth-authorization-server/*", s.handleAuthorizationServerMetadata)
	r.Get("/.well-known/openid-configuration", s.handleAuthorizationServerMetadata)
	r.HandleFunc("/oauth/register", s.handleOAuthDynamicRegister)
	r.Post("/api/adapters/feishu/{slug}/events", s.handleFeishuEventCallback)

	// Auth (public)
	r.Post("/api/auth/register", s.AuthHandler.HandleRegister)
	r.Post("/api/auth/login", s.AuthHandler.HandleLogin)
	r.Post("/api/auth/refresh", s.AuthHandler.HandleRefresh)
	r.Post("/api/auth/logout", s.AuthHandler.HandleLogout)
	r.Get("/api/auth/github/callback", s.AuthHandler.HandleGitHubCallback)
	r.Post("/api/auth/github/callback", s.AuthHandler.HandleGitHubCallback)
	r.Post("/api/auth/token/dev", s.AuthHandler.HandleDevToken)

	// OAuth 2.0 Provider (public endpoints)
	// GET /oauth/authorize serves the SPA which renders the consent UI
	r.Get("/oauth/authorize", web.Handler().ServeHTTP)
	r.Post("/oauth/authorize", s.handleOAuthAuthorizePost)
	r.Get("/api/oauth/authorize-info", s.handleOAuthAuthorizeInfo)
	r.Post("/oauth/token", s.handleOAuthToken)

	// OAuth userinfo requires authentication
	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)
		r.Get("/oauth/userinfo", s.handleOAuthUserInfo)
	})

	// Authenticated routes (JWT Bearer)
	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)

		r.Get("/api/auth/me", s.AuthHandler.HandleMe)
		r.Put("/api/auth/me", s.AuthHandler.HandleUpdateMe)
		r.Post("/api/auth/change-password", s.AuthHandler.HandleChangePassword)
		r.Get("/api/auth/sessions", s.AuthHandler.HandleListSessions)
		r.Delete("/api/auth/sessions/{id}", s.AuthHandler.HandleRevokeSession)

		// File tree
		r.Get("/api/tree/snapshot", s.handleTreeSnapshot)
		r.Get("/api/tree/changes", s.handleTreeChanges)
		r.Get("/api/tree/*", s.handleTreeRead)
		r.Put("/api/tree/*", s.handleTreeWrite)
		r.Delete("/api/tree/*", s.handleTreeDelete)
		r.Get("/api/search", s.handleSearch)

		// Vault
		r.Get("/api/vault/scopes", s.HandleVaultListScopes)
		r.Get("/api/vault/{scope}", s.HandleVaultRead)
		r.Put("/api/vault/{scope}", s.HandleVaultWrite)
		r.Delete("/api/vault/{scope}", s.HandleVaultDelete)

		// Connections
		r.Get("/api/connections", s.handleConnectionsList)
		r.Post("/api/connections", s.handleConnectionsCreate)
		r.Put("/api/connections/{id}", s.handleConnectionsUpdate)
		r.Delete("/api/connections/{id}", s.handleConnectionsDelete)

		// Roles
		r.Get("/api/roles", s.handleRolesList)
		r.Post("/api/roles", s.handleRolesCreate)
		r.Delete("/api/roles/{name}", s.handleRolesDelete)

		// Memory
		r.Get("/api/memory/profile", s.handleMemoryProfileGet)
		r.Put("/api/memory/profile", s.handleMemoryProfileUpdate)
		r.Get("/api/memory/scratch", s.handleGetScratch)
		r.Post("/api/memory/scratch", s.handleWriteScratch)
		r.Get("/api/memory/conflicts", s.handleListConflicts)
		r.Post("/api/memory/conflicts/{id}/resolve", s.handleResolveConflict)

		// Projects
		r.Get("/api/projects", s.handleListProjects)
		r.Post("/api/projects", s.handleCreateProject)
		r.Get("/api/projects/{name}", s.handleGetProject)
		r.Post("/api/projects/{name}/log", s.handleAppendProjectLog)
		r.Put("/api/projects/{name}/archive", s.handleArchiveProject)
		r.Post("/api/projects/{name}/summarize", s.handleSummarizeProject)

		// Inbox (search must be before {role} to avoid matching "search" as role)
		r.Get("/api/inbox/search", s.handleInboxSearch)
		r.Get("/api/inbox/{role}", s.handleInboxList)
		r.Post("/api/inbox/send", s.handleInboxSend)
		r.Put("/api/inbox/{id}/archive", s.handleInboxArchive)

		// Devices
		r.Get("/api/devices", s.handleDevicesList)
		r.Post("/api/devices", s.handleRegisterDevice)
		r.Post("/api/devices/{name}/call", s.handleDeviceCall)

		// Dashboard
		r.Get("/api/dashboard/stats", s.handleDashboardStats)

		// GPT Setup
		r.Get("/api/gpt/setup", s.handleGPTSetup)

		// Import / Export (legacy) — 50MB body limit for imports
		r.Group(func(r chi.Router) {
			r.Use(MaxBodySizeMiddleware(50 << 20))
			r.Post("/api/import/skills", s.HandleImportSkills)
			r.Post("/api/import/vault", s.HandleImportVault)
			r.Post("/api/import/devices", s.HandleImportDevices)
			r.Post("/api/import/full", s.HandleImportFull)
		})
		r.Get("/api/export/full", s.HandleExportFull)

		// Import / Export (bulk API) — 50MB body limit for imports
		r.Group(func(r chi.Router) {
			r.Use(MaxBodySizeMiddleware(50 << 20))
			r.Post("/api/import/skill", s.handleImportSkill)
			r.Post("/api/import/claude-memory", s.handleImportClaudeMemoryV2)
			r.Post("/api/import/claude-data", s.HandleImportClaudeData)
			r.Post("/api/import/profile", s.handleImportProfileV2)
			r.Post("/api/import/bulk", s.handleImportBulk)
		})
		r.Get("/api/export/all", s.handleExportAll)
		r.Get("/api/export/zip", s.handleExportZip)
		r.Get("/api/export/json", s.handleExportJSON)

		// Collaborations
		r.Get("/api/collaborations", s.handleListCollaborations)
		r.Post("/api/collaborations", s.handleCreateCollaboration)
		r.Delete("/api/collaborations/{id}", s.handleRevokeCollaboration)

		// Tokens (scoped access tokens)
		r.Post("/api/tokens", s.handleCreateToken)
		r.Get("/api/tokens", s.handleListTokens)
		r.Get("/api/tokens/scopes", s.handleListScopes)
		r.Get("/api/tokens/{id}", s.handleGetToken)
		r.Put("/api/tokens/{id}", s.handleUpdateToken)
		r.Delete("/api/tokens/{id}", s.handleRevokeToken)
		r.Post("/api/tokens/validate", s.handleValidateToken)

		// Webhooks
		r.Get("/api/webhooks", s.handleListWebhooks)
		r.Post("/api/webhooks", s.handleRegisterWebhook)
		r.Delete("/api/webhooks/{id}", s.handleDeleteWebhook)
		r.Post("/api/webhooks/{id}/test", s.handleTestWebhook)

		// OAuth app management
		r.Get("/api/oauth/apps", s.handleListOAuthApps)
		r.Post("/api/oauth/apps", s.handleRegisterOAuthApp)
		r.Delete("/api/oauth/apps/{id}", s.handleDeleteOAuthApp)
		r.Get("/api/oauth/grants", s.handleListOAuthGrants)
		r.Delete("/api/oauth/grants/{id}", s.handleRevokeOAuthGrant)
	})

	// Agent API (authenticated via X-API-Key or Bearer scoped token)
	// ChatGPT GPT Actions also use these endpoints — schema at /gpt/openapi.json
	r.Group(func(r chi.Router) {
		r.Use(s.apiKeyMiddleware)

		r.Get("/agent/tree/snapshot", s.handleAgentTreeSnapshot)
		r.Get("/agent/tree/changes", s.handleAgentTreeChanges)
		r.Get("/agent/tree/*", s.handleAgentTreeList)
		r.Get("/agent/search", s.handleAgentSearch)
		r.Post("/agent/search", s.handleAgentSearch)
		r.Get("/agent/skills", s.handleAgentListSkills)
		r.Put("/agent/tree/*", s.handleAgentTreeWrite)
		r.Get("/agent/vault/scopes", s.handleAgentVaultListScopes)
		r.Get("/agent/vault/{scope}", s.handleAgentVaultRead)
		r.Put("/agent/vault/{scope}", s.handleAgentVaultWrite)
		r.Put("/agent/memory/profile", s.handleAgentUpdateProfile)
		r.Get("/agent/inbox", s.handleAgentGetInbox)
		r.Get("/agent/inbox/{role}", s.handleAgentGetInbox)
		r.Post("/agent/inbox/send", s.handleAgentSendMessage)
		r.Put("/agent/inbox/{id}/archive", s.handleAgentArchiveInbox)
		r.Post("/agent/projects", s.handleAgentCreateProject)
		r.Get("/agent/projects", s.handleAgentListProjects)
		r.Get("/agent/projects/{name}", s.handleAgentGetProject)
		r.Post("/agent/projects/{name}/log", s.handleAgentAppendProjectLog)
		r.Get("/agent/devices", s.handleAgentDevicesList)
		r.Get("/agent/dashboard/stats", s.handleDashboardStats)
		r.Post("/agent/devices/{name}/call", s.handleAgentCallDevice)
		r.Get("/agent/memory/profile", s.handleAgentGetProfile)

		// Agent cross-user shared access
		r.Get("/agent/shared/{owner_slug}/tree/*", s.handleAgentSharedTree)

		// Agent Import (bulk API)
		r.Post("/agent/import/profile", s.handleAgentImportProfile)
		r.Post("/agent/import/skill", s.handleAgentImportSkill)
		r.Post("/agent/import/claude-memory", s.handleAgentImportClaudeMemory)
		r.Post("/agent/import/bulk", s.handleAgentImportBulk)
		r.With(MaxBodySizeMiddleware(50<<20)).Post("/agent/import/preview", s.handleAgentPreviewBundle)
		r.With(MaxBodySizeMiddleware(50<<20)).Post("/agent/import/bundle", s.handleAgentImportBundle)
		r.Get("/agent/export/all", s.handleAgentExportAll)
		r.Get("/agent/export/bundle", s.handleAgentExportBundle)
	})

	// Embedded frontend (SPA) — catch-all for non-API routes.
	r.NotFound(web.Handler().ServeHTTP)
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// optionalAuthMiddleware tries to extract JWT but doesn't block if missing.
// Used for OAuth authorize page — if user is logged in, skip password prompt.
func (s *Server) optionalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		// No valid JWT — continue without auth (don't block)
		next.ServeHTTP(w, r)
	})
}

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
						slog.Warn("failed to update last_used_at", "error", err)
					}
				}()
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		respondUnauthorized(w)
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
			respondError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "missing authentication: provide Authorization: Bearer aht_xxx or X-API-Key header")
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
			respondError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "invalid API key")
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyUserID, conn.UserID)
		ctx = context.WithValue(ctx, ctxKeyConnection, conn)
		ctx = context.WithValue(ctx, ctxKeyTrustLevel, conn.TrustLevel)

		// Fire-and-forget last_used_at update.
		go func() {
			if err := s.ConnectionService.UpdateLastUsed(context.Background(), conn.ID); err != nil {
				slog.Warn("failed to update last_used_at", "error", err)
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
		respondError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "invalid or expired token")
		return
	}

	// Check rate limit.
	if err := s.TokenService.CheckRateLimit(r.Context(), token); err != nil {
		respondError(w, http.StatusTooManyRequests, ErrCodeRateLimit, err.Error())
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
					respondForbidden(w, "token missing required scope: "+scope)
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
	respondOK(w, map[string]interface{}{
		"status":  "ok",
		"service": "agenthub",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// handlePublicConfig returns non-sensitive configuration for the frontend.
func (s *Server) handlePublicConfig(w http.ResponseWriter, r *http.Request) {
	respondOK(w, map[string]interface{}{
		"github_client_id": s.GitHubClientID,
		"github_enabled":   s.GitHubClientID != "",
	})
}

// ---------------------------------------------------------------------------
// Memory: scratch
// ---------------------------------------------------------------------------

func (s *Server) handleGetScratch(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	entries, err := s.MemoryService.GetScratch(r.Context(), userID, 7)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{"scratch": entries})
}

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	projects, err := s.ProjectService.List(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{"projects": projects})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, "name", "project name is required")
		return
	}

	project, err := s.ProjectService.Create(r.Context(), userID, req.Name)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if s.WebhookService != nil {
		go s.WebhookService.Trigger(context.Background(), userID, models.EventProjectUpdate, map[string]interface{}{
			"project": project.Name,
			"action":  "created",
		})
	}

	respondCreated(w, project)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	project, err := s.ProjectService.Get(r.Context(), userID, name)
	if err != nil {
		respondNotFound(w, "project")
		return
	}

	logs, err := s.ProjectService.GetLogs(r.Context(), project.ID, 50)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{
		"project": project,
		"logs":    logs,
	})
}

func (s *Server) handleAppendProjectLog(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	project, err := s.ProjectService.Get(r.Context(), userID, name)
	if err != nil {
		respondNotFound(w, "project")
		return
	}

	var req struct {
		Source  string   `json:"source"`
		Action  string   `json:"action"`
		Summary string   `json:"summary"`
		Tags    []string `json:"tags,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.Summary == "" {
		respondValidationError(w, "summary", "summary is required")
		return
	}

	logEntry := models.ProjectLog{
		ProjectID: project.ID,
		Source:    req.Source,
		Action:    req.Action,
		Summary:   req.Summary,
		Tags:      req.Tags,
	}

	if err := s.ProjectService.AppendLog(r.Context(), project.ID, logEntry); err != nil {
		respondInternalError(w, err)
		return
	}
	if s.WebhookService != nil {
		go s.WebhookService.Trigger(context.Background(), userID, models.EventProjectUpdate, map[string]interface{}{
			"project": project.Name,
			"action":  req.Action,
			"summary": req.Summary,
		})
	}

	respondCreated(w, map[string]string{"status": "appended", "project": name})
}

func (s *Server) handleArchiveProject(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if err := s.ProjectService.Archive(r.Context(), userID, name); err != nil {
		respondNotFound(w, "project")
		return
	}
	if s.WebhookService != nil {
		go s.WebhookService.Trigger(context.Background(), userID, models.EventProjectUpdate, map[string]interface{}{
			"project": name,
			"action":  "archived",
		})
	}

	respondOK(w, map[string]string{"status": "archived", "name": name})
}

func (s *Server) handleSummarizeProject(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if s.SummaryService == nil {
		respondInternalError(w, fmt.Errorf("summary service not configured"))
		return
	}

	project, err := s.ProjectService.Get(r.Context(), userID, name)
	if err != nil {
		respondNotFound(w, "project")
		return
	}

	md, err := s.SummaryService.GenerateProjectSummary(r.Context(), project.ID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	if err := s.ProjectService.UpdateContext(r.Context(), userID, name, md); err != nil {
		respondInternalError(w, err)
		return
	}
	if s.WebhookService != nil {
		go s.WebhookService.Trigger(context.Background(), userID, models.EventProjectUpdate, map[string]interface{}{
			"project": name,
			"action":  "summarized",
		})
	}

	respondOK(w, map[string]interface{}{
		"status":     "summarized",
		"name":       name,
		"context_md": md,
	})
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

func (s *Server) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if s.DashboardService == nil {
		// Graceful fallback: return basic stats from manual connections plus
		// OAuth/MCP grants when the full dashboard service is not configured.
		count := 0
		if s.ConnectionService != nil {
			if conns, err := s.ConnectionService.ListByUser(r.Context(), userID); err == nil {
				count = len(conns)
			}
		}
		if s.OAuthService != nil {
			if grants, err := s.OAuthService.ListGrants(r.Context(), userID); err == nil {
				count += len(grants)
			}
		}
		respondOK(w, &models.DashboardStats{TotalConnections: count})
		return
	}

	stats, err := s.DashboardService.GetStats(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, stats)
}

// ---------------------------------------------------------------------------
// Agent API handlers — authenticated via X-API-Key or Bearer scoped token
// ---------------------------------------------------------------------------

// agentCheckAuth verifies the request is authenticated (via connection or scoped token)
// and that the trust level meets the minimum. For scoped tokens, also checks the required scope.
func (s *Server) agentCheckAuth(w http.ResponseWriter, r *http.Request, minTrust int, requiredScope string) bool {
	if _, ok := userIDFromCtx(r.Context()); !ok {
		respondUnauthorized(w)
		return false
	}
	if trustLevelFromCtx(r.Context()) < minTrust {
		respondForbidden(w, "insufficient trust level")
		return false
	}
	// For scoped tokens, check the required scope.
	if token := scopedTokenFromCtx(r.Context()); token != nil && requiredScope != "" {
		if !models.HasScope(token.Scopes, requiredScope) {
			respondForbidden(w, "token missing required scope: "+requiredScope)
			return false
		}
	}
	return true
}

func (s *Server) handleAgentTreeList(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadTree) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	trustLevel := trustLevelFromCtx(r.Context())
	path := chi.URLParam(r, "*")
	node, err := s.readOrListTreePath(r.Context(), userID, trustLevel, path)
	if err != nil {
		respondNotFound(w, "file")
		return
	}

	respondOK(w, node)
}

func (s *Server) handleAgentSearch(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeSearch) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	trustLevel := trustLevelFromCtx(r.Context())

	// Support both GET ?q= and POST {"query": "..."} for ChatGPT Actions compatibility.
	query := r.URL.Query().Get("q")
	scope := r.URL.Query().Get("scope")
	if r.Method == http.MethodPost {
		var body struct {
			Query string `json:"query"`
			Scope string `json:"scope"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if body.Query != "" {
				query = body.Query
			}
			if body.Scope != "" {
				scope = body.Scope
			}
		}
	}
	if strings.TrimSpace(query) == "" {
		respondValidationError(w, "q", "query parameter 'q' is required")
		return
	}

	results, err := s.searchHub(r.Context(), userID, trustLevel, query, scope)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{"query": query, "results": results})
}

func (s *Server) handleAgentTreeWrite(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeWriteTree) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	path := chi.URLParam(r, "*")

	var req struct {
		Content          string                 `json:"content"`
		ContentType      string                 `json:"content_type,omitempty"`
		Metadata         map[string]interface{} `json:"metadata,omitempty"`
		MinTrustLevel    int                    `json:"min_trust_level,omitempty"`
		ExpectedVersion  *int64                 `json:"expected_version,omitempty"`
		ExpectedChecksum string                 `json:"expected_checksum,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	contentType := req.ContentType
	if contentType == "" {
		contentType = "text/plain"
	}

	minTrustLevel := req.MinTrustLevel
	if minTrustLevel <= 0 {
		minTrustLevel = models.TrustLevelFull
	}
	entry, err := s.FileTreeService.WriteEntry(r.Context(), userID, path, req.Content, contentType, models.FileTreeWriteOptions{
		Metadata:         req.Metadata,
		MinTrustLevel:    minTrustLevel,
		ExpectedVersion:  req.ExpectedVersion,
		ExpectedChecksum: req.ExpectedChecksum,
	})
	if err != nil {
		if err == services.ErrOptimisticLockConflict {
			respondError(w, http.StatusConflict, ErrCodeConflict, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}

	respondOK(w, entry)
}

func (s *Server) handleAgentTreeSnapshot(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadTree) {
		return
	}
	s.handleTreeSnapshot(w, r)
}

func (s *Server) handleAgentTreeChanges(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadTree) {
		return
	}
	s.handleTreeChanges(w, r)
}

func (s *Server) handleAgentVaultRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	trustLevel := trustLevelFromCtx(r.Context())
	scope := chi.URLParam(r, "scope")

	// Vault requires at least Work level; personal scope requires Full.
	if trustLevel < models.TrustLevelWork {
		respondForbidden(w, "insufficient trust level")
		return
	}
	if strings.HasPrefix(scope, "personal") && trustLevel < models.TrustLevelFull {
		respondForbidden(w, "insufficient trust level for personal vault")
		return
	}

	// For scoped tokens, check the specific vault sub-scope.
	if token := scopedTokenFromCtx(r.Context()); token != nil {
		requiredScope := models.ScopeReadVault
		if strings.HasPrefix(scope, "auth") {
			requiredScope = models.ScopeReadVaultAuth
		}
		if !models.HasScope(token.Scopes, requiredScope) {
			respondForbidden(w, "token missing required scope: "+requiredScope)
			return
		}
	}

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

	respondOK(w, map[string]interface{}{"scope": scope, "data": plaintext})
}

func (s *Server) handleAgentGetInbox(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadInbox) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	role := chi.URLParam(r, "role")

	messages, err := s.InboxService.GetMessages(r.Context(), userID, role, "")
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{"role": role, "messages": messages})
}

func (s *Server) handleAgentSendMessage(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeWriteInbox) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())

	var req struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.To) == "" || strings.TrimSpace(req.Body) == "" {
		respondValidationError(w, "to,body", "to and body are required")
		return
	}

	msg := models.InboxMessage{
		FromAddress: "assistant@" + userID.String(),
		ToAddress:   req.To,
		Subject:     req.Subject,
		Body:        req.Body,
		Priority:    "normal",
	}

	sent, err := s.InboxService.Send(r.Context(), userID, msg)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondCreated(w, sent)
}

func (s *Server) handleAgentCallDevice(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeCallDevices) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	name := chi.URLParam(r, "name")

	var req struct {
		Action string                 `json:"action"`
		Params map[string]interface{} `json:"params,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	result, err := s.DeviceService.Call(r.Context(), userID, name, req.Action, req.Params)
	if err != nil {
		respondDeviceCallError(w, err)
		return
	}

	respondOK(w, result)
}

func (s *Server) handleAgentGetProfile(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelGuest, models.ScopeReadProfile) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	category := r.URL.Query().Get("category")
	profile, err := s.buildAgentProfile(r.Context(), userID, category)
	if err != nil {
		respondNotFound(w, "user")
		return
	}

	respondOK(w, profile)
}
