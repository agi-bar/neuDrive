package localserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/localmcp"
	"github.com/agi-bar/agenthub/internal/localstore"
	"github.com/agi-bar/agenthub/internal/mcp"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/agi-bar/agenthub/internal/skillsarchive"
	"github.com/agi-bar/agenthub/internal/web"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Options struct {
	ListenAddr string
	SQLitePath string
	BaseURL    string
}

type Server struct {
	store   *localstore.Store
	baseURL string
	router  *chi.Mux
}

type contextKey string

const (
	ctxKeyUserID     contextKey = "user_id"
	ctxKeyTrustLevel contextKey = "trust_level"
	ctxKeyToken      contextKey = "token"
)

type FileNode struct {
	Path      string      `json:"path"`
	Name      string      `json:"name"`
	IsDir     bool        `json:"is_dir"`
	Kind      string      `json:"kind,omitempty"`
	Content   string      `json:"content,omitempty"`
	MimeType  string      `json:"mime_type,omitempty"`
	Size      int64       `json:"size,omitempty"`
	Version   int64       `json:"version,omitempty"`
	Checksum  string      `json:"checksum,omitempty"`
	Metadata  interface{} `json:"metadata,omitempty"`
	Children  []*FileNode `json:"children,omitempty"`
	CreatedAt string      `json:"created_at,omitempty"`
	UpdatedAt string      `json:"updated_at,omitempty"`
}

func Run(ctx context.Context, opts Options) error {
	store, err := localstore.Open(opts.SQLitePath)
	if err != nil {
		return err
	}
	defer store.Close()
	if _, err := store.EnsureOwner(ctx); err != nil {
		return err
	}
	s := &Server{
		store:   store,
		baseURL: strings.TrimRight(opts.BaseURL, "/"),
		router:  chi.NewRouter(),
	}
	s.routes()
	listener, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()
	httpServer := &http.Server{
		Addr:         opts.ListenAddr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("local sqlite daemon listening", "addr", listener.Addr().String(), "sqlite_path", opts.SQLitePath)
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()
	select {
	case <-ctx.Done():
	case err := <-serverErr:
		if err != nil {
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}

func (s *Server) routes() {
	r := s.router
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		s.respondOK(w, map[string]any{
			"status":     "ok",
			"service":    "agenthub-local",
			"storage":    "sqlite",
			"sqlitePath": s.store.Path(),
			"time":       time.Now().UTC().Format(time.RFC3339),
		})
	})
	r.Get("/api/config", func(w http.ResponseWriter, r *http.Request) {
		s.respondOK(w, map[string]any{
			"github_enabled":   false,
			"github_client_id": "",
			"local_mode":       true,
			"storage":          "sqlite",
		})
	})
	r.HandleFunc("/mcp", s.handleMCP)

	r.Post("/api/auth/login", s.localOnlyDisabled)
	r.Post("/api/auth/register", s.localOnlyDisabled)
	r.Post("/api/auth/logout", s.handleLogoutNoop)
	r.Get("/api/auth/github/callback", s.localOnlyDisabled)
	r.Post("/api/auth/github/callback", s.localOnlyDisabled)
	r.HandleFunc("/oauth/register", s.localOnlyDisabled)
	r.Get("/oauth/authorize", s.localOnlyDisabled)
	r.Post("/oauth/authorize", s.localOnlyDisabled)
	r.Post("/oauth/token", s.localOnlyDisabled)
	r.Get("/api/oauth/authorize-info", s.localOnlyDisabled)
	r.Get("/api/oauth/apps", s.localOnlyDisabled)
	r.Post("/api/oauth/apps", s.localOnlyDisabled)
	r.Delete("/api/oauth/apps/{id}", s.localOnlyDisabled)
	r.Get("/api/oauth/grants", s.localOnlyDisabled)
	r.Delete("/api/oauth/grants/{id}", s.localOnlyDisabled)

	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)
		r.Get("/api/auth/me", s.handleAuthMe)
		r.Get("/api/dashboard/stats", s.handleDashboardStats)
		r.Get("/api/tree/snapshot", s.handleTreeSnapshot)
		r.Get("/api/tree/*", s.handleTreeRead)
		r.Put("/api/tree/*", s.handleTreeWrite)
		r.Delete("/api/tree/*", s.handleTreeDelete)
		r.MethodFunc(http.MethodGet, "/api/search", s.handleSearch)
		r.MethodFunc(http.MethodPost, "/api/search", s.handleSearch)
		r.Get("/api/memory/profile", s.handleGetProfileView)
		r.Put("/api/memory/profile", s.handleUpdateProfile)
		r.Get("/api/memory/conflicts", s.handleListConflicts)
		r.Get("/api/vault/scopes", s.handleListVaultScopes)
		r.Post("/api/tokens/sync", s.handleCreateSyncToken)

		r.Get("/agent/auth/info", s.handleWhoAmI)
		r.Get("/agent/auth/whoami", s.handleWhoAmI)
		r.Get("/agent/skills", s.handleListSkills)
		r.Get("/agent/tree/snapshot", s.handleTreeSnapshot)
		r.Get("/agent/tree/*", s.handleTreeRead)
		r.Put("/agent/tree/*", s.handleTreeWrite)
		r.Delete("/agent/tree/*", s.handleTreeDelete)
		r.MethodFunc(http.MethodGet, "/agent/search", s.handleSearch)
		r.MethodFunc(http.MethodPost, "/agent/search", s.handleSearch)
		r.Get("/agent/memory/profile", s.handleGetProfiles)
		r.Put("/agent/memory/profile", s.handleUpdateProfile)

		r.Group(func(r chi.Router) {
			r.Post("/agent/import/skills", s.handleImportSkills)
			r.Post("/agent/import/preview", s.handlePreviewBundle)
			r.Post("/agent/import/bundle", s.handleImportBundle)
			r.Get("/agent/export/bundle", s.handleExportBundle)
			r.Post("/agent/import/session", s.handleStartSession)
			r.Put("/agent/import/session/{id}/parts/{index}", s.handleUploadPart)
			r.Get("/agent/import/session/{id}", s.handleGetSession)
			r.Post("/agent/import/session/{id}/commit", s.handleCommitSession)
			r.Delete("/agent/import/session/{id}", s.handleDeleteSession)
			r.Get("/agent/sync/jobs", s.handleListJobs)
			r.Get("/agent/sync/jobs/{id}", s.handleGetJob)
		})
	})

	r.NotFound(web.Handler().ServeHTTP)
}

func (s *Server) requirePermission(w http.ResponseWriter, r *http.Request, minTrustLevel int, requiredScopes ...string) bool {
	token := scopedTokenFromCtx(r.Context())
	if token == nil {
		s.respondError(w, http.StatusUnauthorized, "missing bearer token")
		return false
	}
	if token.MaxTrustLevel < minTrustLevel {
		s.respondError(w, http.StatusForbidden, "insufficient trust level")
		return false
	}
	if len(requiredScopes) == 0 || models.HasScope(token.Scopes, models.ScopeAdmin) {
		return true
	}
	for _, scope := range requiredScopes {
		if models.HasScope(token.Scopes, scope) {
			return true
		}
	}
	s.respondError(w, http.StatusForbidden, "required scope not granted")
	return false
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		rawToken := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if rawToken == "" {
			rawToken = strings.TrimSpace(r.Header.Get("X-API-Key"))
		}
		if rawToken == "" {
			s.respondError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		token, err := s.store.ValidateToken(r.Context(), rawToken)
		if err != nil {
			s.respondError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		if err := s.store.CheckRateLimit(r.Context(), token); err != nil {
			s.respondError(w, http.StatusTooManyRequests, err.Error())
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyUserID, token.UserID)
		ctx = context.WithValue(ctx, ctxKeyTrustLevel, token.MaxTrustLevel)
		ctx = context.WithValue(ctx, ctxKeyToken, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	rawToken := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if rawToken == "" {
		rawToken = strings.TrimSpace(r.Header.Get("X-API-Key"))
	}
	if rawToken == "" {
		s.respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	token, err := s.store.ValidateToken(r.Context(), rawToken)
	if err != nil {
		s.respondError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}
	if err := s.store.CheckRateLimit(r.Context(), token); err != nil {
		s.respondError(w, http.StatusTooManyRequests, err.Error())
		return
	}
	var req mcp.JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(mcp.JSONRPCResponse{JSONRPC: "2.0", Error: &mcp.RPCError{Code: -32700, Message: "parse error"}})
		return
	}
	server := &localmcp.Server{
		Store:      s.store,
		UserID:     token.UserID,
		TrustLevel: token.MaxTrustLevel,
		Scopes:     token.Scopes,
		BaseURL:    s.baseURL,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(server.HandleJSONRPC(req))
}

func (s *Server) handleCreateSyncToken(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelFull, models.ScopeAdmin) {
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		s.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req models.SyncTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ttlMinutes := req.TTLMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 30
	}
	if ttlMinutes < 5 {
		ttlMinutes = 5
	}
	if ttlMinutes > 120 {
		ttlMinutes = 120
	}
	access := strings.ToLower(strings.TrimSpace(req.Access))
	if access == "" {
		access = "push"
	}
	var scopes []string
	switch access {
	case "push":
		scopes = []string{models.ScopeWriteBundle}
	case "pull":
		scopes = []string{models.ScopeReadBundle}
	case "both":
		scopes = []string{models.ScopeReadBundle, models.ScopeWriteBundle}
	default:
		s.respondError(w, http.StatusBadRequest, "access must be push, pull, or both")
		return
	}
	created, err := s.store.CreateToken(r.Context(), userID, "sync-"+access, scopes, models.TrustLevelWork, time.Duration(ttlMinutes)*time.Minute)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.respondCreated(w, models.SyncTokenResponse{
		Token:     created.Token,
		ExpiresAt: created.ScopedToken.ExpiresAt,
		APIBase:   s.baseURL,
		Scopes:    scopes,
		Usage:     "Use this token for local bundle sync endpoints.",
	})
}

func (s *Server) handleLogoutNoop(w http.ResponseWriter, r *http.Request) {
	s.respondOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		s.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := s.store.UserByID(r.Context(), userID)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, map[string]any{
		"id":           user.ID,
		"slug":         user.Slug,
		"display_name": user.DisplayName,
		"email":        "",
		"avatar_url":   "",
		"bio":          "",
		"timezone":     user.Timezone,
		"language":     user.Language,
		"created_at":   user.CreatedAt,
	})
}

func (s *Server) handleWhoAmI(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromCtx(r.Context())
	trust := trustLevelFromCtx(r.Context())
	token := scopedTokenFromCtx(r.Context())
	user, _ := s.store.UserByID(r.Context(), userID)
	resp := models.AgentAuthInfo{
		UserID:     userID,
		AuthMode:   "scoped_token",
		TrustLevel: trust,
		APIBase:    s.baseURL,
	}
	if token != nil {
		resp.Scopes = append([]string{}, token.Scopes...)
		resp.ExpiresAt = &token.ExpiresAt
	}
	if user != nil {
		resp.UserSlug = user.Slug
	}
	s.respondOK(w, resp)
}

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadSkills) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	trust := trustLevelFromCtx(r.Context())
	skills, err := s.store.ListSkillSummaries(r.Context(), userID, trust)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, map[string]any{"skills": skills})
}

func (s *Server) handleTreeSnapshot(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadTree) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	trust := trustLevelFromCtx(r.Context())
	pathValue := r.URL.Query().Get("path")
	if pathValue == "" {
		pathValue = "/"
	}
	snapshot, err := s.store.Snapshot(r.Context(), userID, pathValue, trust)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	nodes := make([]*FileNode, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		nodes = append(nodes, fileTreeEntryToNode(entry))
	}
	s.respondOK(w, map[string]any{
		"path":          snapshot.Path,
		"cursor":        snapshot.Cursor,
		"root_checksum": snapshot.RootChecksum,
		"entries":       nodes,
	})
}

func (s *Server) handleTreeRead(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadTree) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	trust := trustLevelFromCtx(r.Context())
	pathValue := chi.URLParam(r, "*")
	if strings.HasSuffix(pathValue, "/") || pathValue == "" {
		entries, err := s.store.List(r.Context(), userID, pathValue, trust)
		if err != nil {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		children := make([]*FileNode, 0, len(entries))
		for _, entry := range entries {
			children = append(children, fileTreeEntryToNode(entry))
		}
		s.respondOK(w, &FileNode{Path: ensurePublic(pathValue), Name: path.Base(strings.TrimSuffix(pathValue, "/")), IsDir: true, Kind: "directory", Children: children})
		return
	}
	entry, err := s.store.Read(r.Context(), userID, pathValue, trust)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	s.respondOK(w, fileTreeEntryToNode(*entry))
}

func (s *Server) handleTreeWrite(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteTree) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	var req struct {
		Content          string                 `json:"content"`
		ContentType      string                 `json:"content_type,omitempty"`
		MimeType         string                 `json:"mime_type,omitempty"`
		IsDir            bool                   `json:"is_dir"`
		Metadata         map[string]interface{} `json:"metadata,omitempty"`
		MinTrustLevel    int                    `json:"min_trust_level,omitempty"`
		ExpectedVersion  *int64                 `json:"expected_version,omitempty"`
		ExpectedChecksum string                 `json:"expected_checksum,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	pathValue := chi.URLParam(r, "*")
	if req.IsDir {
		if err := s.store.EnsureDirectory(r.Context(), userID, pathValue); err != nil {
			s.respondInternal(w, err)
			return
		}
		s.respondOK(w, map[string]string{"status": "ok", "path": ensurePublic(pathValue)})
		return
	}
	mimeType := req.ContentType
	if mimeType == "" {
		mimeType = req.MimeType
	}
	entry, err := s.store.WriteEntry(r.Context(), userID, pathValue, req.Content, mimeType, models.FileTreeWriteOptions{
		Metadata:         req.Metadata,
		MinTrustLevel:    req.MinTrustLevel,
		ExpectedVersion:  req.ExpectedVersion,
		ExpectedChecksum: req.ExpectedChecksum,
	})
	if err != nil {
		if errors.Is(err, services.ErrOptimisticLockConflict) {
			s.respondError(w, http.StatusConflict, err.Error())
			return
		}
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, fileTreeEntryToNode(*entry))
}

func (s *Server) handleTreeDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteTree) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	if err := s.store.Delete(r.Context(), userID, chi.URLParam(r, "*")); err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	s.respondOK(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeSearch) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	trust := trustLevelFromCtx(r.Context())
	query := r.URL.Query().Get("q")
	if r.Method == http.MethodPost {
		var body struct {
			Query string `json:"query"`
			Scope string `json:"scope"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.Query != "" {
			query = body.Query
		}
	}
	results, err := s.store.Search(r.Context(), userID, query, trust, "/")
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(results))
	for _, entry := range results {
		payload = append(payload, map[string]any{
			"type":    entry.Kind,
			"path":    entry.Path,
			"snippet": strings.TrimSpace(entry.Content),
		})
	}
	s.respondOK(w, map[string]any{"query": query, "results": payload})
}

func (s *Server) handleGetProfiles(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadProfile) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	profiles, err := s.store.GetProfiles(r.Context(), userID)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, map[string]any{"profiles": profiles})
}

func (s *Server) handleGetProfileView(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadProfile) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	user, _ := s.store.UserByID(r.Context(), userID)
	profiles, err := s.store.GetProfiles(r.Context(), userID)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	preferences := make(map[string]string, len(profiles))
	for _, profile := range profiles {
		preferences[profile.Category] = profile.Content
	}
	displayName := ""
	if user != nil {
		displayName = user.DisplayName
	}
	if value := strings.TrimSpace(preferences["display_name"]); value != "" {
		displayName = value
	}
	s.respondOK(w, map[string]any{
		"user_id":      userID,
		"display_name": displayName,
		"preferences":  preferences,
	})
}

func (s *Server) handleListConflicts(w http.ResponseWriter, r *http.Request) {
	s.respondOK(w, map[string]any{"conflicts": []any{}})
}

func (s *Server) handleListVaultScopes(w http.ResponseWriter, r *http.Request) {
	s.respondOK(w, map[string]any{"scopes": []any{}})
}

func (s *Server) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		s.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	stats, err := s.store.DashboardStats(r.Context(), userID)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, stats)
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteProfile) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	var req struct {
		Category    string            `json:"category"`
		Content     string            `json:"content"`
		Source      string            `json:"source"`
		DisplayName string            `json:"display_name"`
		Preferences map[string]string `json:"preferences"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "agent"
	}
	if req.Category != "" {
		if err := s.store.UpsertProfile(r.Context(), userID, req.Category, req.Content, source); err != nil {
			s.respondInternal(w, err)
			return
		}
	}
	for category, content := range req.Preferences {
		if err := s.store.UpsertProfile(r.Context(), userID, category, content, source); err != nil {
			s.respondInternal(w, err)
			return
		}
	}
	if strings.TrimSpace(req.DisplayName) != "" {
		if err := s.store.UpsertProfile(r.Context(), userID, "display_name", req.DisplayName, source); err != nil {
			s.respondInternal(w, err)
			return
		}
	}
	s.handleGetProfiles(w, r)
}

func (s *Server) handlePreviewBundle(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := decodeAndPreviewBundle(r.Context(), s.store, userID, body)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.respondOK(w, result)
}

func (s *Server) handleImportBundle(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	var bundle models.Bundle
	if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := s.store.ImportBundle(r.Context(), userID, bundle)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.respondOK(w, result)
}

func (s *Server) handleExportBundle(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeReadBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	filters := parseBundleFilters(r)
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = models.BundleFormatJSON
	}
	if format == models.BundleFormatArchive {
		archive, _, err := s.store.ExportArchive(r.Context(), userID, filters)
		if err != nil {
			s.respondInternal(w, err)
			return
		}
		filename := fmt.Sprintf("agenthub-sync-%s.ahubz", time.Now().UTC().Format("2006-01-02"))
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		_, _ = w.Write(archive)
		return
	}
	bundle, err := s.store.ExportBundle(r.Context(), userID, filters)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, bundle)
}

func (s *Server) handleStartSession(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	var req models.SyncStartSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	resp, err := s.store.StartSession(r.Context(), userID, req)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.respondCreated(w, resp)
}

func (s *Server) handleUploadPart(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	var index int
	if _, err := fmt.Sscanf(chi.URLParam(r, "index"), "%d", &index); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid part index")
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	resp, err := s.store.UploadPart(r.Context(), userID, sessionID, index, data)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, services.ErrSyncPartConflict):
			status = http.StatusConflict
		case errors.Is(err, services.ErrSyncSessionNotFound):
			status = http.StatusNotFound
		case errors.Is(err, services.ErrSyncSessionExpired):
			status = http.StatusGone
		}
		s.respondError(w, status, err.Error())
		return
	}
	s.respondOK(w, resp)
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	resp, err := s.store.GetSession(r.Context(), userID, sessionID)
	if err != nil {
		if errors.Is(err, services.ErrSyncSessionNotFound) {
			s.respondError(w, http.StatusNotFound, "sync session not found")
			return
		}
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.respondOK(w, resp)
}

func (s *Server) handleCommitSession(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	var req models.SyncCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := s.store.CommitSession(r.Context(), userID, sessionID, req)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, services.ErrSyncSessionNotFound):
			status = http.StatusNotFound
		case errors.Is(err, services.ErrSyncSessionExpired):
			status = http.StatusGone
		case errors.Is(err, services.ErrSyncSessionIncomplete), errors.Is(err, services.ErrSyncPreviewDrift):
			status = http.StatusConflict
		}
		s.respondError(w, status, err.Error())
		return
	}
	s.respondOK(w, result)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	if err := s.store.AbortSession(r.Context(), userID, sessionID); err != nil {
		if errors.Is(err, services.ErrSyncSessionNotFound) {
			s.respondError(w, http.StatusNotFound, "sync session not found")
			return
		}
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.respondOK(w, map[string]string{"status": "aborted"})
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeReadBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	jobs, err := s.store.ListJobs(r.Context(), userID)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, map[string]any{"jobs": jobs})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeReadBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	job, err := s.store.GetJob(r.Context(), userID, jobID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "sync job not found")
		return
	}
	s.respondOK(w, job)
}

func (s *Server) handleImportSkills(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteSkills) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("parse multipart: %v", err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("read form file: %v", err))
		return
	}
	defer file.Close()

	buf, err := io.ReadAll(file)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("read file data: %v", err))
		return
	}
	entries, err := skillsarchive.ParseZipBytes(buf, header.Filename)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	platform := strings.TrimSpace(r.FormValue("platform"))
	if platform == "" {
		platform = strings.TrimSpace(r.URL.Query().Get("platform"))
	}
	if platform == "" {
		platform = "skills-archive"
	}
	archiveName := filepath.Base(strings.TrimSpace(header.Filename))
	if archiveName == "" {
		archiveName = "skills.zip"
	}

	result := map[string]any{
		"imported": 0,
		"skipped":  0,
		"errors":   []string{},
	}
	errorsList := make([]string, 0)

	for _, entry := range entries {
		target := path.Join("/skills", entry.SkillName, entry.RelPath)
		metadata := map[string]interface{}{
			"source_platform": platform,
			"source_archive":  archiveName,
			"capture_mode":    "archive",
		}
		contentType := skillsarchive.DetectContentType(entry.RelPath, entry.Data)
		if skillsarchive.LooksBinary(entry.RelPath, entry.Data) {
			if _, err := s.store.WriteBinaryEntry(r.Context(), userID, target, entry.Data, contentType, models.FileTreeWriteOptions{
				Kind:          "skill_asset",
				Metadata:      metadata,
				MinTrustLevel: models.TrustLevelGuest,
			}); err != nil {
				errorsList = append(errorsList, fmt.Sprintf("skill %s/%s: %v", entry.SkillName, entry.RelPath, err))
				continue
			}
		} else {
			if _, err := s.store.WriteEntry(r.Context(), userID, target, string(entry.Data), contentType, models.FileTreeWriteOptions{
				Kind:          "skill_file",
				Metadata:      metadata,
				MinTrustLevel: models.TrustLevelGuest,
			}); err != nil {
				errorsList = append(errorsList, fmt.Sprintf("skill %s/%s: %v", entry.SkillName, entry.RelPath, err))
				continue
			}
		}
		result["imported"] = result["imported"].(int) + 1
	}
	result["errors"] = errorsList
	s.respondOK(w, result)
}

func decodeAndPreviewBundle(ctx context.Context, store *localstore.Store, userID uuid.UUID, body []byte) (*models.BundlePreviewResult, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, fmt.Errorf("preview request body is required")
	}
	var bundle models.Bundle
	if err := json.Unmarshal(body, &bundle); err == nil && bundle.Version == models.BundleVersionV1 {
		return store.PreviewBundle(ctx, userID, bundle)
	}
	var request models.BundlePreviewRequest
	if err := json.Unmarshal(body, &request); err == nil {
		if request.Bundle != nil {
			return store.PreviewBundle(ctx, userID, *request.Bundle)
		}
		if request.Manifest != nil {
			return store.PreviewManifest(ctx, userID, *request.Manifest)
		}
	}
	var manifest models.BundleArchiveManifest
	if err := json.Unmarshal(body, &manifest); err == nil && manifest.Version == models.BundleVersionV2 {
		return store.PreviewManifest(ctx, userID, manifest)
	}
	return nil, fmt.Errorf("unsupported preview payload")
}

func parseBundleFilters(r *http.Request) models.BundleFilters {
	query := r.URL.Query()
	return models.BundleFilters{
		IncludeDomains: queryValues(query, "include_domain", "include_domains"),
		IncludeSkills:  queryValues(query, "include_skill", "include_skills"),
		ExcludeSkills:  queryValues(query, "exclude_skill", "exclude_skills"),
	}
}

func queryValues(values map[string][]string, singular, plural string) []string {
	var result []string
	for _, key := range []string{singular, plural} {
		for _, raw := range values[key] {
			for _, part := range strings.Split(raw, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					result = append(result, part)
				}
			}
		}
	}
	return result
}

func userIDFromCtx(ctx context.Context) (uuid.UUID, bool) {
	value, ok := ctx.Value(ctxKeyUserID).(uuid.UUID)
	return value, ok
}

func trustLevelFromCtx(ctx context.Context) int {
	value, _ := ctx.Value(ctxKeyTrustLevel).(int)
	return value
}

func scopedTokenFromCtx(ctx context.Context) *models.ScopedToken {
	value, _ := ctx.Value(ctxKeyToken).(*models.ScopedToken)
	return value
}

func fileTreeEntryToNode(entry models.FileTreeEntry) *FileNode {
	size := int64(len(entry.Content))
	if raw, ok := entry.Metadata["size_bytes"]; ok {
		switch typed := raw.(type) {
		case float64:
			size = int64(typed)
		case int:
			size = int64(typed)
		case int64:
			size = typed
		}
	}
	return &FileNode{
		Path:      ensurePublic(entry.Path),
		Name:      path.Base(strings.TrimSuffix(ensurePublic(entry.Path), "/")),
		IsDir:     entry.IsDirectory,
		Kind:      entry.Kind,
		Content:   entry.Content,
		MimeType:  entry.ContentType,
		Size:      size,
		Version:   entry.Version,
		Checksum:  entry.Checksum,
		Metadata:  entry.Metadata,
		CreatedAt: entry.CreatedAt.Format(time.RFC3339),
		UpdatedAt: entry.UpdatedAt.Format(time.RFC3339),
	}
}

func ensurePublic(pathValue string) string {
	if pathValue == "" {
		return "/"
	}
	return hubpath.NormalizePublic(pathValue)
}

func (s *Server) localOnlyDisabled(w http.ResponseWriter, r *http.Request) {
	message := "This capability is only available in remote/public service mode. Switch to Postgres self-hosted or the official Agent Hub service."
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte("<html><body><h1>Local SQLite mode</h1><p>" + message + "</p></body></html>"))
		return
	}
	s.respondError(w, http.StatusNotImplemented, message)
}

func (s *Server) respondOK(w http.ResponseWriter, data any) {
	s.respondJSON(w, http.StatusOK, map[string]any{"ok": true, "data": data})
}

func (s *Server) respondCreated(w http.ResponseWriter, data any) {
	s.respondJSON(w, http.StatusCreated, map[string]any{"ok": true, "data": data})
}

func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	s.respondJSON(w, status, map[string]any{"ok": false, "error": map[string]any{"message": message}})
}

func (s *Server) respondInternal(w http.ResponseWriter, err error) {
	slog.Error("local sqlite server error", "error", err)
	s.respondError(w, http.StatusInternalServerError, err.Error())
}

func (s *Server) respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
