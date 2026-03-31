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
	ctxKeyUserID     contextKey = "user_id"
	ctxKeyUserSlug   contextKey = "user_slug"
	ctxKeyConnection contextKey = "connection"
	ctxKeyTrustLevel contextKey = "trust_level"
)

// Server holds the HTTP router and all service dependencies.
type Server struct {
	Router             *chi.Mux
	UserService        *services.UserService
	ConnectionService  *services.ConnectionService
	Vault              *vaultpkg.Vault
	AuthHandler        *auth.Handler
	JWTSecret          string
	GitHubClientID     string
	GitHubClientSecret string
}

// NewServer creates a fully wired Server with routes configured.
func NewServer(
	userSvc *services.UserService,
	connSvc *services.ConnectionService,
	vault *vaultpkg.Vault,
	jwtSecret string,
	ghClientID string,
	ghClientSecret string,
) *Server {
	s := &Server{
		Router:             chi.NewRouter(),
		UserService:        userSvc,
		ConnectionService:  connSvc,
		Vault:              vault,
		JWTSecret:          jwtSecret,
		GitHubClientID:     ghClientID,
		GitHubClientSecret: ghClientSecret,
	}
	s.AuthHandler = auth.NewHandler(userSvc, jwtSecret, ghClientID, ghClientSecret)
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
	r.Post("/api/auth/github/callback", s.AuthHandler.HandleGitHubCallback)
	r.Post("/api/auth/token/dev", s.AuthHandler.HandleDevToken)

	// Authenticated routes (JWT Bearer)
	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)

		r.Get("/api/auth/me", s.AuthHandler.HandleMe)

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

// apiKeyMiddleware authenticates requests exclusively via X-API-Key.
// It sets user_id, connection, and trust_level in context and updates last_used_at.
func (s *Server) apiKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := auth.ExtractAPIKey(r)
		if apiKey == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing X-API-Key header"})
			return
		}

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
// Agent API handlers — authenticated via X-API-Key, trust-level filtered
// ---------------------------------------------------------------------------

func (s *Server) handleAgentTreeList(w http.ResponseWriter, r *http.Request) {
	conn := connectionFromCtx(r.Context())
	if conn == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if conn.TrustLevel < models.TrustLevelCollaborate {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient trust level"})
		return
	}
	path := chi.URLParam(r, "*")
	if path == "" {
		path = "/"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"path": path, "children": []interface{}{}})
}

func (s *Server) handleAgentSearch(w http.ResponseWriter, r *http.Request) {
	conn := connectionFromCtx(r.Context())
	if conn == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if conn.TrustLevel < models.TrustLevelCollaborate {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient trust level"})
		return
	}
	query := r.URL.Query().Get("q")
	writeJSON(w, http.StatusOK, map[string]interface{}{"query": query, "results": []interface{}{}})
}

func (s *Server) handleAgentTreeWrite(w http.ResponseWriter, r *http.Request) {
	conn := connectionFromCtx(r.Context())
	if conn == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if conn.TrustLevel < models.TrustLevelWork {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient trust level"})
		return
	}
	path := chi.URLParam(r, "*")
	writeJSON(w, http.StatusOK, map[string]interface{}{"path": path, "status": "written"})
}

func (s *Server) handleAgentVaultRead(w http.ResponseWriter, r *http.Request) {
	conn := connectionFromCtx(r.Context())
	if conn == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	scope := chi.URLParam(r, "scope")
	// Vault requires at least Work level; personal scope requires Full.
	if conn.TrustLevel < models.TrustLevelWork {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient trust level"})
		return
	}
	if strings.HasPrefix(scope, "personal") && conn.TrustLevel < models.TrustLevelFull {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient trust level for personal vault"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"scope": scope, "data": ""})
}

func (s *Server) handleAgentGetInbox(w http.ResponseWriter, r *http.Request) {
	conn := connectionFromCtx(r.Context())
	if conn == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if conn.TrustLevel < models.TrustLevelCollaborate {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient trust level"})
		return
	}
	role := chi.URLParam(r, "role")
	writeJSON(w, http.StatusOK, map[string]interface{}{"role": role, "messages": []interface{}{}})
}

func (s *Server) handleAgentSendMessage(w http.ResponseWriter, r *http.Request) {
	conn := connectionFromCtx(r.Context())
	if conn == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if conn.TrustLevel < models.TrustLevelCollaborate {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient trust level"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "sent"})
}

func (s *Server) handleAgentCallDevice(w http.ResponseWriter, r *http.Request) {
	conn := connectionFromCtx(r.Context())
	if conn == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if conn.TrustLevel < models.TrustLevelWork {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient trust level"})
		return
	}
	name := chi.URLParam(r, "name")
	writeJSON(w, http.StatusOK, map[string]interface{}{"device": name, "status": "dispatched"})
}

func (s *Server) handleAgentGetProfile(w http.ResponseWriter, r *http.Request) {
	conn := connectionFromCtx(r.Context())
	if conn == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if conn.TrustLevel < models.TrustLevelGuest {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient trust level"})
		return
	}

	user, err := s.UserService.GetByID(r.Context(), conn.UserID)
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
