package api

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
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/mcp"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/agi-bar/agenthub/internal/skillsarchive"
	sqlitestorage "github.com/agi-bar/agenthub/internal/storage/sqlite"
	"github.com/agi-bar/agenthub/internal/web"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type SQLiteOptions struct {
	ListenAddr string
	SQLitePath string
	BaseURL    string
}

type SQLiteServer struct {
	store     *sqlitestorage.Store
	fileTree  *services.FileTreeService
	memory    *services.MemoryService
	project   *services.ProjectService
	token     *services.TokenService
	dashboard *services.DashboardService
	importSvc *services.ImportService
	exportSvc *services.ExportService
	syncSvc   *services.SyncService
	baseURL   string
	router    *chi.Mux
}

func (s *SQLiteServer) ensureDeps() {
	if s == nil || s.store == nil {
		return
	}
	if s.fileTree == nil {
		s.fileTree = services.NewFileTreeServiceWithRepo(sqlitestorage.NewFileTreeRepo(s.store))
	}
	if s.memory == nil {
		s.memory = services.NewMemoryServiceWithRepo(sqlitestorage.NewMemoryRepo(s.store), nil)
	}
	if s.project == nil {
		s.project = services.NewProjectServiceWithRepo(sqlitestorage.NewProjectRepo(s.store), nil, nil)
	}
	if s.token == nil {
		s.token = services.NewTokenServiceWithRepo(sqlitestorage.NewTokenRepo(s.store))
	}
	if s.importSvc == nil {
		s.importSvc = services.NewImportService(nil, s.fileTree, s.memory, nil)
	}
	if s.exportSvc == nil {
		s.exportSvc = services.NewExportService(s.fileTree, s.memory, s.project, nil, nil, nil, nil, nil)
	}
	if s.dashboard == nil {
		s.dashboard = services.NewDashboardServiceWithRepo(sqlitestorage.NewDashboardRepo(s.store))
	}
	if s.syncSvc == nil {
		s.syncSvc = services.NewSyncServiceWithRepo(sqlitestorage.NewSyncRepo(s.store), s.importSvc, s.exportSvc, s.fileTree, s.memory)
	}
}

func NewSQLiteServer(store *sqlitestorage.Store, baseURL string) *SQLiteServer {
	s := &SQLiteServer{
		store:   store,
		baseURL: strings.TrimRight(baseURL, "/"),
		router:  chi.NewRouter(),
	}
	s.ensureDeps()
	s.routes()
	return s
}

func (s *SQLiteServer) Handler() http.Handler {
	if s == nil {
		return http.NotFoundHandler()
	}
	return s.router
}

func RunSQLite(ctx context.Context, opts SQLiteOptions) error {
	store, err := sqlitestorage.Open(opts.SQLitePath)
	if err != nil {
		return err
	}
	defer store.Close()
	if _, err := store.EnsureOwner(ctx); err != nil {
		return err
	}
	s := NewSQLiteServer(store, opts.BaseURL)
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

func (s *SQLiteServer) routes() {
	s.ensureDeps()
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
		r.Get("/api/projects", s.handleListProjects)
		r.Post("/api/projects", s.handleCreateProject)
		r.Get("/api/projects/{name}", s.handleGetProject)
		r.Post("/api/projects/{name}/log", s.handleAppendProjectLog)
		r.Put("/api/projects/{name}/archive", s.handleArchiveProject)
		r.Get("/api/skills", s.handleListSkills)
		r.Get("/api/devices", s.handleListDevices)
		r.Get("/api/roles", s.handleListRoles)
		r.Get("/api/inbox/{role}", s.handleInboxList)
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

func (s *SQLiteServer) requirePermission(w http.ResponseWriter, r *http.Request, minTrustLevel int, requiredScopes ...string) bool {
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

func (s *SQLiteServer) authMiddleware(next http.Handler) http.Handler {
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
		token, err := s.token.ValidateToken(r.Context(), rawToken)
		if err != nil {
			s.respondError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		if err := s.token.CheckRateLimit(r.Context(), token); err != nil {
			s.respondError(w, http.StatusTooManyRequests, err.Error())
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyUserID, token.UserID)
		ctx = context.WithValue(ctx, ctxKeyTrustLevel, token.MaxTrustLevel)
		ctx = context.WithValue(ctx, ctxKeyScopedToken, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *SQLiteServer) handleMCP(w http.ResponseWriter, r *http.Request) {
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
	token, err := s.token.ValidateToken(r.Context(), rawToken)
	if err != nil {
		s.respondError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}
	if err := s.token.CheckRateLimit(r.Context(), token); err != nil {
		s.respondError(w, http.StatusTooManyRequests, err.Error())
		return
	}
	server := &mcp.MCPServer{
		UserID:     token.UserID,
		TrustLevel: token.MaxTrustLevel,
		Scopes:     token.Scopes,
		BaseURL:    s.baseURL,
		FileTree:   s.fileTree,
		Memory:     s.memory,
		Project:    s.project,
		Dashboard:  s.dashboard,
		Import:     s.importSvc,
		Token:      s.token,
	}
	mcp.ServeHTTPHandler(server, w, r)
}

func (s *SQLiteServer) handleCreateSyncToken(w http.ResponseWriter, r *http.Request) {
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
	created, err := s.token.CreateEphemeralToken(r.Context(), userID, "sync-"+access, scopes, models.TrustLevelWork, time.Duration(ttlMinutes)*time.Minute)
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

func (s *SQLiteServer) handleLogoutNoop(w http.ResponseWriter, r *http.Request) {
	s.respondOK(w, map[string]string{"status": "ok"})
}

func (s *SQLiteServer) handleAuthMe(w http.ResponseWriter, r *http.Request) {
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

func (s *SQLiteServer) handleWhoAmI(w http.ResponseWriter, r *http.Request) {
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

func (s *SQLiteServer) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadSkills) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	trust := trustLevelFromCtx(r.Context())
	skills, err := s.fileTree.ListSkillSummaries(r.Context(), userID, trust)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, map[string]any{"skills": skills})
}

func (s *SQLiteServer) handleTreeSnapshot(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadTree) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	trust := trustLevelFromCtx(r.Context())
	pathValue := r.URL.Query().Get("path")
	if pathValue == "" {
		pathValue = "/"
	}
	snapshot, err := s.fileTree.Snapshot(r.Context(), userID, pathValue, trust)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	nodes := make([]*FileNode, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		entryCopy := entry
		nodes = append(nodes, fileTreeEntryToNode(&entryCopy))
	}
	s.respondOK(w, map[string]any{
		"path":          snapshot.Path,
		"cursor":        snapshot.Cursor,
		"root_checksum": snapshot.RootChecksum,
		"entries":       nodes,
	})
}

func (s *SQLiteServer) handleTreeRead(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadTree) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	trust := trustLevelFromCtx(r.Context())
	pathValue := chi.URLParam(r, "*")
	if strings.HasSuffix(pathValue, "/") || pathValue == "" {
		entries, err := s.fileTree.List(r.Context(), userID, pathValue, trust)
		if err != nil {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		children := make([]*FileNode, 0, len(entries))
		for _, entry := range entries {
			entryCopy := entry
			children = append(children, fileTreeEntryToNode(&entryCopy))
		}
		if pathValue == "" || pathValue == "/" {
			children = s.appendMissingRootNamespaces(r.Context(), userID, trust, children)
		}
		s.respondOK(w, &FileNode{Path: ensurePublic(pathValue), Name: path.Base(strings.TrimSuffix(pathValue, "/")), IsDir: true, Kind: "directory", Children: children})
		return
	}
	entry, err := s.fileTree.Read(r.Context(), userID, pathValue, trust)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	s.respondOK(w, fileTreeEntryToNode(entry))
}

func (s *SQLiteServer) handleTreeWrite(w http.ResponseWriter, r *http.Request) {
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
		if err := s.fileTree.EnsureDirectory(r.Context(), userID, pathValue); err != nil {
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
	entry, err := s.fileTree.WriteEntry(r.Context(), userID, pathValue, req.Content, mimeType, models.FileTreeWriteOptions{
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
	s.respondOK(w, fileTreeEntryToNode(entry))
}

func (s *SQLiteServer) handleTreeDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteTree) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	if err := s.fileTree.Delete(r.Context(), userID, chi.URLParam(r, "*")); err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	s.respondOK(w, map[string]string{"status": "deleted"})
}

func (s *SQLiteServer) handleSearch(w http.ResponseWriter, r *http.Request) {
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
	results, err := s.fileTree.Search(r.Context(), userID, query, trust, "/")
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

func (s *SQLiteServer) handleGetProfiles(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadProfile) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	profiles, err := s.memory.GetProfile(r.Context(), userID)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, map[string]any{"profiles": profiles})
}

func (s *SQLiteServer) handleGetProfileView(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadProfile) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	user, _ := s.store.UserByID(r.Context(), userID)
	profiles, err := s.memory.GetProfile(r.Context(), userID)
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

func (s *SQLiteServer) handleListConflicts(w http.ResponseWriter, r *http.Request) {
	s.respondOK(w, map[string]any{"conflicts": []any{}})
}

func (s *SQLiteServer) handleListVaultScopes(w http.ResponseWriter, r *http.Request) {
	s.respondOK(w, map[string]any{"scopes": []any{}})
}

func (s *SQLiteServer) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		s.respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	stats, err := s.dashboard.GetStats(r.Context(), userID)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, stats)
}

func (s *SQLiteServer) handleListProjects(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadProjects) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	projects, err := s.store.ListProjects(r.Context(), userID)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(projects))
	for _, project := range projects {
		payload = append(payload, projectPayload(project))
	}
	s.respondOK(w, map[string]any{"projects": payload})
}

func (s *SQLiteServer) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteProjects) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	project, err := s.store.CreateProject(r.Context(), userID, req.Name)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.respondCreated(w, projectPayload(*project))
}

func (s *SQLiteServer) handleGetProject(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadProjects) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	name := chi.URLParam(r, "name")
	project, err := s.store.GetProject(r.Context(), userID, name)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "project not found")
		return
	}
	logs, err := s.store.GetProjectLogs(r.Context(), userID, name, 50)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	logPayload := make([]map[string]any, 0, len(logs))
	for _, logEntry := range logs {
		logPayload = append(logPayload, projectLogPayload(logEntry))
	}
	s.respondOK(w, map[string]any{
		"project": projectPayload(*project),
		"logs":    logPayload,
	})
}

func (s *SQLiteServer) handleAppendProjectLog(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteProjects) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	name := chi.URLParam(r, "name")
	var req struct {
		Source  string   `json:"source"`
		Action  string   `json:"action"`
		Summary string   `json:"summary"`
		Tags    []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Summary) == "" {
		s.respondError(w, http.StatusBadRequest, "summary is required")
		return
	}
	if err := s.store.AppendProjectLog(r.Context(), userID, name, models.ProjectLog{
		Source:    req.Source,
		Action:    req.Action,
		Summary:   req.Summary,
		Tags:      req.Tags,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.respondCreated(w, map[string]string{"status": "appended", "project": name})
}

func (s *SQLiteServer) handleArchiveProject(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteProjects) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	name := chi.URLParam(r, "name")
	if err := s.store.ArchiveProject(r.Context(), userID, name); err != nil {
		s.respondError(w, http.StatusNotFound, "project not found")
		return
	}
	s.respondOK(w, map[string]string{"status": "archived", "name": name})
}

func (s *SQLiteServer) handleListDevices(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadDevices) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	devices, err := s.listDevicesFromTree(r.Context(), userID)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, map[string]any{"devices": devices})
}

func (s *SQLiteServer) handleListRoles(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadTree) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	roles, err := s.listRolesFromTree(r.Context(), userID)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, map[string]any{"roles": roles})
}

func (s *SQLiteServer) handleInboxList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelGuest, models.ScopeReadInbox) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	role := chi.URLParam(r, "role")
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "incoming"
	}
	messages, err := s.listInboxMessagesFromTree(r.Context(), userID, role, status)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, map[string]any{
		"role":     role,
		"messages": messages,
	})
}

func (s *SQLiteServer) appendMissingRootNamespaces(ctx context.Context, userID uuid.UUID, trust int, children []*FileNode) []*FileNode {
	seen := make(map[string]struct{}, len(children))
	for _, child := range children {
		seen[strings.TrimSuffix(ensurePublic(child.Path), "/")] = struct{}{}
	}
	for _, namespace := range []string{"/skills", "/devices", "/roles", "/inbox", "/notes"} {
		if _, ok := seen[namespace]; ok {
			continue
		}
		snapshot, err := s.store.Snapshot(ctx, userID, namespace, trust)
		if err != nil || len(snapshot.Entries) == 0 {
			continue
		}
		children = append(children, &FileNode{
			Path:      namespace,
			Name:      path.Base(namespace),
			IsDir:     true,
			Kind:      "directory",
			MimeType:  "directory",
			Metadata:  map[string]any{},
			CreatedAt: snapshot.Entries[0].CreatedAt.Format(time.RFC3339),
			UpdatedAt: snapshot.Entries[0].UpdatedAt.Format(time.RFC3339),
		})
	}
	return children
}

func (s *SQLiteServer) listDevicesFromTree(ctx context.Context, userID uuid.UUID) ([]models.Device, error) {
	snapshot, err := s.store.Snapshot(ctx, userID, "/devices", models.TrustLevelFull)
	if err != nil {
		if errors.Is(err, services.ErrEntryNotFound) {
			return []models.Device{}, nil
		}
		return nil, err
	}
	devices := map[string]models.Device{}
	for _, entry := range snapshot.Entries {
		if entry.IsDirectory || !strings.HasSuffix(entry.Path, "/SKILL.md") {
			continue
		}
		name := path.Base(path.Dir(ensurePublic(entry.Path)))
		device := devices[name]
		device.Name = name
		device.UserID = userID
		device.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("local-device:"+name))
		device.SkillMD = entry.Content
		device.CreatedAt = entry.CreatedAt
		device.UpdatedAt = entry.UpdatedAt
		if value, ok := entry.Metadata["device_type"].(string); ok {
			device.DeviceType = value
		}
		if value, ok := entry.Metadata["brand"].(string); ok {
			device.Brand = value
		}
		if value, ok := entry.Metadata["protocol"].(string); ok {
			device.Protocol = value
		}
		if value, ok := entry.Metadata["endpoint"].(string); ok {
			device.Endpoint = value
		}
		if value, ok := entry.Metadata["status"].(string); ok {
			device.Status = value
		}
		devices[name] = device
	}
	names := make([]string, 0, len(devices))
	for name := range devices {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]models.Device, 0, len(names))
	for _, name := range names {
		out = append(out, devices[name])
	}
	return out, nil
}

func (s *SQLiteServer) listRolesFromTree(ctx context.Context, userID uuid.UUID) ([]models.Role, error) {
	snapshot, err := s.store.Snapshot(ctx, userID, "/roles", models.TrustLevelFull)
	if err != nil {
		if errors.Is(err, services.ErrEntryNotFound) {
			return []models.Role{}, nil
		}
		return nil, err
	}
	roles := map[string]models.Role{}
	for _, entry := range snapshot.Entries {
		if entry.IsDirectory || !strings.HasSuffix(entry.Path, "/SKILL.md") {
			continue
		}
		name := path.Base(path.Dir(ensurePublic(entry.Path)))
		role := roles[name]
		role.Name = name
		role.UserID = userID
		role.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("local-role:"+name))
		role.CreatedAt = entry.CreatedAt
		if value, ok := entry.Metadata["role_type"].(string); ok {
			role.RoleType = value
		}
		if value, ok := entry.Metadata["lifecycle"].(string); ok {
			role.Lifecycle = value
		}
		if value := stringSlice(entry.Metadata["allowed_paths"]); len(value) > 0 {
			role.AllowedPaths = value
		}
		if value := stringSlice(entry.Metadata["allowed_vault_scopes"]); len(value) > 0 {
			role.AllowedVaultScopes = value
		}
		roles[name] = role
	}
	names := make([]string, 0, len(roles))
	for name := range roles {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]models.Role, 0, len(names))
	for _, name := range names {
		out = append(out, roles[name])
	}
	return out, nil
}

func (s *SQLiteServer) listInboxMessagesFromTree(ctx context.Context, userID uuid.UUID, role, status string) ([]models.InboxMessage, error) {
	snapshot, err := s.store.Snapshot(ctx, userID, "/inbox", models.TrustLevelFull)
	if err != nil {
		if errors.Is(err, services.ErrEntryNotFound) {
			return []models.InboxMessage{}, nil
		}
		return nil, err
	}
	messages := make([]models.InboxMessage, 0)
	for _, entry := range snapshot.Entries {
		if entry.IsDirectory || !strings.HasSuffix(entry.Path, ".json") {
			continue
		}
		var message models.InboxMessage
		if err := json.Unmarshal([]byte(entry.Content), &message); err != nil {
			continue
		}
		if role != "" && message.ToAddress != role {
			continue
		}
		if status != "" && message.Status != status {
			continue
		}
		messages = append(messages, message)
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].CreatedAt.After(messages[j].CreatedAt)
	})
	return messages, nil
}

func stringSlice(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}

func projectPayload(project models.Project) map[string]any {
	payload := map[string]any{
		"id":         project.ID,
		"user_id":    project.UserID,
		"name":       project.Name,
		"status":     project.Status,
		"context_md": project.ContextMD,
		"created_at": project.CreatedAt,
		"updated_at": project.UpdatedAt,
	}
	if lastActivity, _ := project.Metadata["last_activity"].(string); strings.TrimSpace(lastActivity) != "" {
		payload["last_activity"] = lastActivity
	}
	if len(project.Metadata) > 0 {
		payload["metadata"] = project.Metadata
	}
	return payload
}

func projectLogPayload(entry models.ProjectLog) map[string]any {
	return map[string]any{
		"id":         entry.ID,
		"project_id": entry.ProjectID,
		"source":     entry.Source,
		"action":     entry.Action,
		"summary":    entry.Summary,
		"message":    entry.Summary,
		"tags":       entry.Tags,
		"created_at": entry.CreatedAt,
		"timestamp":  entry.CreatedAt.Format(time.RFC3339),
	}
}

func (s *SQLiteServer) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
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
		if err := s.memory.UpsertProfile(r.Context(), userID, req.Category, req.Content, source); err != nil {
			s.respondInternal(w, err)
			return
		}
	}
	for category, content := range req.Preferences {
		if err := s.memory.UpsertProfile(r.Context(), userID, category, content, source); err != nil {
			s.respondInternal(w, err)
			return
		}
	}
	if strings.TrimSpace(req.DisplayName) != "" {
		if err := s.memory.UpsertProfile(r.Context(), userID, "display_name", req.DisplayName, source); err != nil {
			s.respondInternal(w, err)
			return
		}
	}
	s.handleGetProfiles(w, r)
}

func (s *SQLiteServer) handlePreviewBundle(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := s.decodeAndPreviewBundle(r.Context(), userID, body)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.respondOK(w, result)
}

func (s *SQLiteServer) handleImportBundle(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	var bundle models.Bundle
	if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := s.importSvc.ImportBundle(r.Context(), userID, bundle)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.respondOK(w, result)
}

func (s *SQLiteServer) handleExportBundle(w http.ResponseWriter, r *http.Request) {
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
		archive, _, err := s.syncSvc.ExportArchive(r.Context(), userID, filters)
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
	bundle, err := s.syncSvc.ExportBundleJSON(r.Context(), userID, filters)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, bundle)
}

func (s *SQLiteServer) handleStartSession(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	var req models.SyncStartSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	resp, err := s.syncSvc.StartSession(r.Context(), userID, req)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.respondCreated(w, resp)
}

func (s *SQLiteServer) handleUploadPart(w http.ResponseWriter, r *http.Request) {
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
	resp, err := s.syncSvc.UploadPart(r.Context(), userID, sessionID, index, data)
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

func (s *SQLiteServer) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	resp, err := s.syncSvc.GetSession(r.Context(), userID, sessionID)
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

func (s *SQLiteServer) handleCommitSession(w http.ResponseWriter, r *http.Request) {
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
	result, err := s.syncSvc.CommitSession(r.Context(), userID, sessionID, req)
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

func (s *SQLiteServer) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeWriteBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	if err := s.syncSvc.AbortSession(r.Context(), userID, sessionID); err != nil {
		if errors.Is(err, services.ErrSyncSessionNotFound) {
			s.respondError(w, http.StatusNotFound, "sync session not found")
			return
		}
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.respondOK(w, map[string]string{"status": "aborted"})
}

func (s *SQLiteServer) handleListJobs(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeReadBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	jobs, err := s.syncSvc.ListJobs(r.Context(), userID)
	if err != nil {
		s.respondInternal(w, err)
		return
	}
	s.respondOK(w, map[string]any{"jobs": jobs})
}

func (s *SQLiteServer) handleGetJob(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, models.TrustLevelWork, models.ScopeReadBundle) {
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	job, err := s.syncSvc.GetJob(r.Context(), userID, jobID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "sync job not found")
		return
	}
	s.respondOK(w, job)
}

func (s *SQLiteServer) handleImportSkills(w http.ResponseWriter, r *http.Request) {
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
			if _, err := s.fileTree.WriteBinaryEntry(r.Context(), userID, target, entry.Data, contentType, models.FileTreeWriteOptions{
				Kind:          "skill_asset",
				Metadata:      metadata,
				MinTrustLevel: models.TrustLevelGuest,
			}); err != nil {
				errorsList = append(errorsList, fmt.Sprintf("skill %s/%s: %v", entry.SkillName, entry.RelPath, err))
				continue
			}
		} else {
			if _, err := s.fileTree.WriteEntry(r.Context(), userID, target, string(entry.Data), contentType, models.FileTreeWriteOptions{
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

func (s *SQLiteServer) decodeAndPreviewBundle(ctx context.Context, userID uuid.UUID, body []byte) (*models.BundlePreviewResult, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, fmt.Errorf("preview request body is required")
	}
	var bundle models.Bundle
	if err := json.Unmarshal(body, &bundle); err == nil && bundle.Version == models.BundleVersionV1 {
		return s.importSvc.PreviewBundle(ctx, userID, bundle)
	}
	var request models.BundlePreviewRequest
	if err := json.Unmarshal(body, &request); err == nil {
		if request.Bundle != nil {
			return s.importSvc.PreviewBundle(ctx, userID, *request.Bundle)
		}
		if request.Manifest != nil {
			return s.syncSvc.PreviewManifest(ctx, userID, *request.Manifest)
		}
	}
	var manifest models.BundleArchiveManifest
	if err := json.Unmarshal(body, &manifest); err == nil && manifest.Version == models.BundleVersionV2 {
		return s.syncSvc.PreviewManifest(ctx, userID, manifest)
	}
	return nil, fmt.Errorf("unsupported preview payload")
}

func ensurePublic(pathValue string) string {
	if pathValue == "" {
		return "/"
	}
	return hubpath.NormalizePublic(pathValue)
}

func (s *SQLiteServer) localOnlyDisabled(w http.ResponseWriter, r *http.Request) {
	message := "This capability is only available in remote/public service mode. Switch to Postgres self-hosted or the official Agent Hub service."
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte("<html><body><h1>Local SQLite mode</h1><p>" + message + "</p></body></html>"))
		return
	}
	s.respondError(w, http.StatusNotImplemented, message)
}

func (s *SQLiteServer) respondOK(w http.ResponseWriter, data any) {
	s.respondJSON(w, http.StatusOK, map[string]any{"ok": true, "data": data})
}

func (s *SQLiteServer) respondCreated(w http.ResponseWriter, data any) {
	s.respondJSON(w, http.StatusCreated, map[string]any{"ok": true, "data": data})
}

func (s *SQLiteServer) respondError(w http.ResponseWriter, status int, message string) {
	s.respondJSON(w, status, map[string]any{"ok": false, "error": map[string]any{"message": message}})
}

func (s *SQLiteServer) respondInternal(w http.ResponseWriter, err error) {
	slog.Error("local sqlite server error", "error", err)
	s.respondError(w, http.StatusInternalServerError, err.Error())
}

func (s *SQLiteServer) respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
