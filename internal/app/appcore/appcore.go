package appcore

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/api"
	"github.com/agi-bar/neudrive/internal/auth"
	"github.com/agi-bar/neudrive/internal/config"
	"github.com/agi-bar/neudrive/internal/database"
	"github.com/agi-bar/neudrive/internal/localgitsync"
	"github.com/agi-bar/neudrive/internal/mcp"
	"github.com/agi-bar/neudrive/internal/runtimecfg"
	"github.com/agi-bar/neudrive/internal/services"
	sqlitestorage "github.com/agi-bar/neudrive/internal/storage/sqlite"
	"github.com/agi-bar/neudrive/internal/vault"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	Storage               string
	LocalMode             bool
	SQLitePath            string
	DatabaseURL           string
	JWTSecret             string
	VaultMasterKey        string
	PublicBaseURL         string
	CORSOrigins           []string
	GithubClientID        string
	GithubClientSecret    string
	GitHubAppClientID     string
	GitHubAppClientSecret string
	GitHubAppSlug         string
	GitMirrorHostedRoot   string
	LogLevel              string
	LogFormat             string
	RunMigrations         bool
}

type App struct {
	Storage      string
	Config       *config.Config
	HTTPHandler  http.Handler
	NewMCPServer func(token string) (mcp.JSONRPCHandler, error)
	Close        func() error

	MemoryService    *services.MemoryService
	TokenService     any
	InboxService     *services.InboxService
	SyncService      *services.SyncService
	GitMirrorService *localgitsync.Service
}

const (
	DefaultLocalStorage  = "sqlite"
	DefaultServerStorage = "postgres"
)

func ResolveStorageBackend(explicitStorage, explicitSQLitePath, explicitDatabaseURL, defaultStorage string) string {
	if storage := strings.ToLower(strings.TrimSpace(explicitStorage)); storage != "" {
		return storage
	}
	if strings.TrimSpace(explicitDatabaseURL) != "" {
		return "postgres"
	}
	if strings.TrimSpace(explicitSQLitePath) != "" {
		return "sqlite"
	}
	if storage := strings.ToLower(strings.TrimSpace(defaultStorage)); storage != "" {
		return storage
	}
	return DefaultLocalStorage
}

func Build(ctx context.Context, opts Options) (*App, error) {
	storage := ResolveStorageBackend(opts.Storage, opts.SQLitePath, opts.DatabaseURL, DefaultLocalStorage)

	switch storage {
	case "sqlite":
		return buildSQLite(ctx, opts)
	case "postgres":
		return buildPostgres(ctx, opts)
	default:
		return nil, fmt.Errorf("unsupported storage backend %q", storage)
	}
}

func buildSQLite(ctx context.Context, opts Options) (*App, error) {
	cfg, err := loadSQLiteConfig(opts)
	if err != nil {
		return nil, err
	}
	sqlitePath := strings.TrimSpace(opts.SQLitePath)
	if sqlitePath == "" {
		sqlitePath = runtimecfg.DefaultSQLitePath()
	}
	store, err := sqlitestorage.Open(sqlitePath)
	if err != nil {
		return nil, err
	}
	store.SetUserStorageQuotaBytes(cfg.UserStorageQuotaBytes)
	owner, err := store.EnsureOwner(ctx)
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	v, err := vault.NewVault(cfg.VaultMasterKey)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	fileTreeSvc := services.NewFileTreeServiceWithRepo(sqlitestorage.NewFileTreeRepo(store))
	fileTreeSvc.SetUserStorageQuotaBytes(cfg.UserStorageQuotaBytes)
	memorySvc := services.NewMemoryServiceWithRepo(sqlitestorage.NewMemoryRepo(store), nil)
	userSvc := services.NewUserServiceWithRepo(sqlitestorage.NewUserRepo(store))
	connSvc := services.NewConnectionServiceWithRepo(sqlitestorage.NewConnectionRepo(store))
	vaultSvc := services.NewVaultServiceWithRepo(sqlitestorage.NewVaultRepo(store), v)
	roleSvc := services.NewRoleServiceWithRepo(sqlitestorage.NewRoleRepo(store), fileTreeSvc)
	inboxSvc := services.NewInboxServiceWithRepo(sqlitestorage.NewInboxRepo(store), fileTreeSvc)
	projectSvc := services.NewProjectServiceWithRepo(sqlitestorage.NewProjectRepo(store), roleSvc, fileTreeSvc)
	tokenSvc := services.NewTokenServiceWithRepo(sqlitestorage.NewTokenRepo(store))
	importSvc := services.NewImportService(nil, fileTreeSvc, memorySvc, vaultSvc)
	exportSvc := services.NewExportService(fileTreeSvc, memorySvc, projectSvc, vaultSvc, inboxSvc, roleSvc, userSvc)
	syncSvc := services.NewSyncServiceWithRepo(sqlitestorage.NewSyncRepo(store), importSvc, exportSvc, fileTreeSvc, memorySvc)
	dashboardSvc := services.NewDashboardServiceWithRepo(sqlitestorage.NewDashboardRepo(store))
	executionMode := localgitsync.ExecutionModeHosted
	if opts.LocalMode {
		executionMode = localgitsync.ExecutionModeLocal
	}
	localGitSyncSvc := localgitsync.NewWithDeps(
		store,
		fileTreeSvc,
		userSvc,
		connSvc,
		projectSvc,
		vaultSvc,
		localgitsync.WithExecutionMode(executionMode),
		localgitsync.WithHostedRoot(cfg.GitMirrorHostedRoot),
		localgitsync.WithGitMirrorPublicBaseURL(cfg.PublicBaseURL),
		localgitsync.WithGitHubAppConfig(cfg.GitHubAppClientID, cfg.GitHubAppClientSecret, cfg.GitHubAppSlug),
		localgitsync.WithStateSigningSecret(cfg.JWTSecret),
	)
	tokenGen := func(userID uuid.UUID, slug string) (string, error) {
		return auth.GenerateToken(userID, slug, cfg.JWTSecret)
	}
	var ghExchange services.GitHubExchangeFunc
	if cfg.GithubClientID != "" && cfg.GithubClientSecret != "" {
		ghExchange = func(ctx context.Context, code string) (*services.GitHubUser, error) {
			ghUser, err := auth.ExchangeGitHubCode(ctx, cfg.GithubClientID, cfg.GithubClientSecret, code)
			if err != nil {
				return nil, err
			}
			return &services.GitHubUser{
				ID:    ghUser.ID,
				Login: ghUser.Login,
				Name:  ghUser.Name,
				Email: ghUser.Email,
			}, nil
		}
	}
	authSvc := services.NewAuthServiceWithRepo(sqlitestorage.NewAuthRepo(store), tokenGen, ghExchange)
	externalAuthSvc := services.NewExternalAuthServiceWithRepo(sqlitestorage.NewExternalAuthRepo(store), authSvc, cfg)
	oauthSvc := services.NewOAuthServiceWithRepo(sqlitestorage.NewOAuthRepo(store), cfg.JWTSecret)
	httpServer := api.NewServerWithDeps(api.ServerDeps{
		Storage:               "sqlite",
		Config:                cfg,
		LocalOwnerID:          owner.ID,
		UserService:           userSvc,
		AuthService:           authSvc,
		ExternalAuthService:   externalAuthSvc,
		ConnectionService:     connSvc,
		FileTreeService:       fileTreeSvc,
		VaultService:          vaultSvc,
		MemoryService:         memorySvc,
		ProjectService:        projectSvc,
		RoleService:           roleSvc,
		InboxService:          inboxSvc,
		DashboardService:      dashboardSvc,
		TokenService:          tokenSvc,
		ImportService:         importSvc,
		ExportService:         exportSvc,
		SyncService:           syncSvc,
		LocalGitSync:          localGitSyncSvc,
		OAuthService:          oauthSvc,
		Vault:                 v,
		JWTSecret:             cfg.JWTSecret,
		GitHubClientID:        cfg.GithubClientID,
		GitHubClientSecret:    cfg.GithubClientSecret,
		GitHubAppClientID:     cfg.GitHubAppClientID,
		GitHubAppClientSecret: cfg.GitHubAppClientSecret,
		GitHubAppSlug:         cfg.GitHubAppSlug,
	})
	app := &App{
		Storage:          "sqlite",
		Config:           cfg,
		HTTPHandler:      httpServer.Router,
		MemoryService:    memorySvc,
		TokenService:     tokenSvc,
		InboxService:     inboxSvc,
		SyncService:      syncSvc,
		GitMirrorService: localGitSyncSvc,
		NewMCPServer: func(token string) (mcp.JSONRPCHandler, error) {
			scopedToken, err := tokenSvc.ValidateToken(ctx, token)
			if err != nil {
				return nil, fmt.Errorf("invalid token: %w", err)
			}
			return &mcp.MCPServer{
				UserID:       scopedToken.UserID,
				TrustLevel:   scopedToken.MaxTrustLevel,
				Scopes:       scopedToken.Scopes,
				BaseURL:      cfg.PublicBaseURL,
				Source:       services.InferSourceFromTokenName(scopedToken.Name),
				Connection:   connSvc,
				OAuth:        oauthSvc,
				FileTree:     fileTreeSvc,
				Vault:        vaultSvc,
				VaultCrypto:  v,
				Memory:       memorySvc,
				Project:      projectSvc,
				Inbox:        inboxSvc,
				Dashboard:    dashboardSvc,
				Import:       importSvc,
				Token:        tokenSvc,
				LocalGitSync: localGitSyncSvc,
			}, nil
		},
		Close: store.Close,
	}
	return app, nil
}

func buildPostgres(ctx context.Context, opts Options) (*App, error) {
	cfg, err := loadConfig(opts)
	if err != nil {
		return nil, err
	}

	db, err := database.InitDB(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if opts.RunMigrations {
		if err := database.RunMigrations(db, resolveMigrationsDir()); err != nil {
			db.Close()
			return nil, err
		}
	}

	deps, err := buildPostgresDeps(ctx, db, cfg)
	if err != nil {
		db.Close()
		return nil, err
	}
	localOwnerID := uuid.Nil
	if opts.LocalMode {
		localOwnerID, err = ensureLocalPostgresOwner(ctx, db)
		if err != nil {
			db.Close()
			return nil, err
		}
	}

	executionMode := localgitsync.ExecutionModeHosted
	if opts.LocalMode {
		executionMode = localgitsync.ExecutionModeLocal
	}
	localGitSyncSvc := localgitsync.NewWithDeps(
		localgitsync.NewPostgresRepo(db),
		deps.fileTreeSvc,
		deps.userSvc,
		deps.connSvc,
		deps.projectSvc,
		deps.vaultSvc,
		localgitsync.WithExecutionMode(executionMode),
		localgitsync.WithHostedRoot(cfg.GitMirrorHostedRoot),
		localgitsync.WithGitMirrorPublicBaseURL(cfg.PublicBaseURL),
		localgitsync.WithGitHubAppConfig(cfg.GitHubAppClientID, cfg.GitHubAppClientSecret, cfg.GitHubAppSlug),
		localgitsync.WithStateSigningSecret(cfg.JWTSecret),
	)

	httpServer := api.NewServerWithDeps(api.ServerDeps{
		Storage:               "postgres",
		Config:                cfg,
		LocalOwnerID:          localOwnerID,
		UserService:           deps.userSvc,
		AuthService:           deps.authSvc,
		ExternalAuthService:   deps.externalAuthSvc,
		ConnectionService:     deps.connSvc,
		FileTreeService:       deps.fileTreeSvc,
		VaultService:          deps.vaultSvc,
		MemoryService:         deps.memorySvc,
		ProjectService:        deps.projectSvc,
		SummaryService:        deps.summarySvc,
		RoleService:           deps.roleSvc,
		InboxService:          deps.inboxSvc,
		DashboardService:      deps.dashboardSvc,
		TokenService:          deps.tokenSvc,
		ImportService:         deps.importSvc,
		ExportService:         deps.exportSvc,
		SyncService:           deps.syncSvc,
		CollaborationService:  deps.collabSvc,
		WebhookService:        deps.webhookSvc,
		OAuthService:          deps.oauthSvc,
		LocalGitSync:          localGitSyncSvc,
		Vault:                 deps.vaultCrypto,
		JWTSecret:             cfg.JWTSecret,
		GitHubClientID:        cfg.GithubClientID,
		GitHubClientSecret:    cfg.GithubClientSecret,
		GitHubAppClientID:     cfg.GitHubAppClientID,
		GitHubAppClientSecret: cfg.GitHubAppClientSecret,
		GitHubAppSlug:         cfg.GitHubAppSlug,
	})

	app := &App{
		Storage:          "postgres",
		Config:           cfg,
		HTTPHandler:      httpServer.Router,
		MemoryService:    deps.memorySvc,
		TokenService:     deps.tokenSvc,
		InboxService:     deps.inboxSvc,
		SyncService:      deps.syncSvc,
		GitMirrorService: localGitSyncSvc,
		NewMCPServer: func(token string) (mcp.JSONRPCHandler, error) {
			scopedToken, err := deps.tokenSvc.ValidateToken(ctx, token)
			if err != nil {
				return nil, fmt.Errorf("invalid token: %w", err)
			}
			return &mcp.MCPServer{
				UserID:       scopedToken.UserID,
				TrustLevel:   scopedToken.MaxTrustLevel,
				Scopes:       scopedToken.Scopes,
				BaseURL:      cfg.PublicBaseURL,
				Source:       services.InferSourceFromTokenName(scopedToken.Name),
				Connection:   deps.connSvc,
				OAuth:        deps.oauthSvc,
				FileTree:     deps.fileTreeSvc,
				Vault:        deps.vaultSvc,
				VaultCrypto:  deps.vaultCrypto,
				Memory:       deps.memorySvc,
				Project:      deps.projectSvc,
				Inbox:        deps.inboxSvc,
				Dashboard:    deps.dashboardSvc,
				Import:       deps.importSvc,
				Token:        deps.tokenSvc,
				LocalGitSync: localGitSyncSvc,
			}, nil
		},
		Close: func() error {
			db.Close()
			return nil
		},
	}
	return app, nil
}

type postgresDeps struct {
	userSvc         *services.UserService
	authSvc         *services.AuthService
	externalAuthSvc *services.ExternalAuthService
	connSvc         *services.ConnectionService
	fileTreeSvc     *services.FileTreeService
	vaultSvc        *services.VaultService
	memorySvc       *services.MemoryService
	projectSvc      *services.ProjectService
	summarySvc      *services.SummaryService
	roleSvc         *services.RoleService
	inboxSvc        *services.InboxService
	dashboardSvc    *services.DashboardService
	tokenSvc        *services.TokenService
	importSvc       *services.ImportService
	exportSvc       *services.ExportService
	syncSvc         *services.SyncService
	collabSvc       *services.CollaborationService
	webhookSvc      *services.WebhookService
	oauthSvc        *services.OAuthService
	vaultCrypto     *vault.Vault
}

func buildPostgresDeps(_ context.Context, db *pgxpool.Pool, cfg *config.Config) (*postgresDeps, error) {
	v, err := vault.NewVault(cfg.VaultMasterKey)
	if err != nil {
		return nil, err
	}

	userSvc := services.NewUserService(db)
	tokenGen := func(userID uuid.UUID, slug string) (string, error) {
		return auth.GenerateToken(userID, slug, cfg.JWTSecret)
	}

	var ghExchange services.GitHubExchangeFunc
	if cfg.GithubClientID != "" && cfg.GithubClientSecret != "" {
		ghExchange = func(ctx context.Context, code string) (*services.GitHubUser, error) {
			ghUser, err := auth.ExchangeGitHubCode(ctx, cfg.GithubClientID, cfg.GithubClientSecret, code)
			if err != nil {
				return nil, err
			}
			return &services.GitHubUser{
				ID:    ghUser.ID,
				Login: ghUser.Login,
				Name:  ghUser.Name,
				Email: ghUser.Email,
			}, nil
		}
	}

	authSvc := services.NewAuthService(db, tokenGen, ghExchange)
	externalAuthSvc := services.NewExternalAuthService(db, authSvc, cfg)
	connSvc := services.NewConnectionService(db)
	fileTreeSvc := services.NewFileTreeService(db)
	fileTreeSvc.SetUserStorageQuotaBytes(cfg.UserStorageQuotaBytes)
	vaultSvc := services.NewVaultService(db, v)
	memorySvc := services.NewMemoryService(db, fileTreeSvc)
	roleSvc := services.NewRoleService(db, fileTreeSvc)
	projectSvc := services.NewProjectService(db, roleSvc, fileTreeSvc)
	summarySvc := services.NewSummaryService(db, projectSvc)
	inboxSvc := services.NewInboxService(db, fileTreeSvc)
	dashboardSvc := services.NewDashboardService(db)
	tokenSvc := services.NewTokenService(db)
	importSvc := services.NewImportService(db, fileTreeSvc, memorySvc, vaultSvc)
	exportSvc := services.NewExportService(fileTreeSvc, memorySvc, projectSvc, vaultSvc, inboxSvc, roleSvc, userSvc)
	syncSvc := services.NewSyncService(db, importSvc, exportSvc, fileTreeSvc, memorySvc)
	collabSvc := services.NewCollaborationService(db)
	webhookSvc := services.NewWebhookService(db)
	oauthSvc := services.NewOAuthService(db, cfg.JWTSecret)

	inboxSvc.Webhook = webhookSvc
	memorySvc.Webhook = webhookSvc
	seedDefaultUser(db, userSvc)

	return &postgresDeps{
		userSvc:         userSvc,
		authSvc:         authSvc,
		externalAuthSvc: externalAuthSvc,
		connSvc:         connSvc,
		fileTreeSvc:     fileTreeSvc,
		vaultSvc:        vaultSvc,
		memorySvc:       memorySvc,
		projectSvc:      projectSvc,
		summarySvc:      summarySvc,
		roleSvc:         roleSvc,
		inboxSvc:        inboxSvc,
		dashboardSvc:    dashboardSvc,
		tokenSvc:        tokenSvc,
		importSvc:       importSvc,
		exportSvc:       exportSvc,
		syncSvc:         syncSvc,
		collabSvc:       collabSvc,
		webhookSvc:      webhookSvc,
		oauthSvc:        oauthSvc,
		vaultCrypto:     v,
	}, nil
}

func ensureLocalPostgresOwner(ctx context.Context, db *pgxpool.Pool) (uuid.UUID, error) {
	var existing uuid.UUID
	err := db.QueryRow(ctx, `SELECT id FROM users ORDER BY created_at ASC LIMIT 1`).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if err != pgx.ErrNoRows {
		return uuid.Nil, err
	}
	now := time.Now().UTC()
	ownerID := uuid.New()
	_, err = db.Exec(ctx,
		`INSERT INTO users (id, slug, display_name, email, avatar_url, bio, timezone, language, created_at, updated_at)
		 VALUES ($1, 'local', 'Local Owner', '', '', '', 'UTC', 'en', $2, $2)`,
		ownerID,
		now,
	)
	if err != nil {
		return uuid.Nil, err
	}
	return ownerID, nil
}

func loadConfig(opts Options) (*config.Config, error) {
	overrides := map[string]string{}
	if opts.DatabaseURL != "" {
		overrides["DATABASE_URL"] = opts.DatabaseURL
	}
	if opts.JWTSecret != "" {
		overrides["JWT_SECRET"] = opts.JWTSecret
	}
	if opts.VaultMasterKey != "" {
		overrides["VAULT_MASTER_KEY"] = opts.VaultMasterKey
	}
	if opts.PublicBaseURL != "" {
		overrides["PUBLIC_BASE_URL"] = opts.PublicBaseURL
	}
	if len(opts.CORSOrigins) > 0 {
		overrides["CORS_ORIGINS"] = strings.Join(opts.CORSOrigins, ",")
	}
	if opts.GithubClientID != "" {
		overrides["GITHUB_CLIENT_ID"] = opts.GithubClientID
	}
	if opts.GithubClientSecret != "" {
		overrides["GITHUB_CLIENT_SECRET"] = opts.GithubClientSecret
	}
	if opts.GitHubAppClientID != "" {
		overrides["GITHUB_APP_CLIENT_ID"] = opts.GitHubAppClientID
	}
	if opts.GitHubAppClientSecret != "" {
		overrides["GITHUB_APP_CLIENT_SECRET"] = opts.GitHubAppClientSecret
	}
	if opts.GitHubAppSlug != "" {
		overrides["GITHUB_APP_SLUG"] = opts.GitHubAppSlug
	}
	if opts.GitMirrorHostedRoot != "" {
		overrides["GIT_MIRROR_HOSTED_ROOT"] = opts.GitMirrorHostedRoot
	}
	if opts.LogLevel != "" {
		overrides["LOG_LEVEL"] = opts.LogLevel
	}
	if opts.LogFormat != "" {
		overrides["LOG_FORMAT"] = opts.LogFormat
	}
	return config.LoadWithOverrides(overrides)
}

func loadSQLiteConfig(opts Options) (*config.Config, error) {
	sqliteOpts := opts
	if strings.TrimSpace(sqliteOpts.JWTSecret) == "" {
		sqliteOpts.JWTSecret = "neudrive-local-sqlite-jwt-secret"
	}
	if strings.TrimSpace(sqliteOpts.VaultMasterKey) == "" {
		sqliteOpts.VaultMasterKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	}
	return loadConfig(sqliteOpts)
}

func resolveMigrationsDir() string {
	execPath, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(execPath), "..", "..", "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	if info, err := os.Stat("migrations"); err == nil && info.IsDir() {
		return "migrations"
	}

	return "migrations"
}

func seedDefaultUser(pool *pgxpool.Pool, userSvc *services.UserService) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		slog.Warn("could not check user count (table may not exist)", "error", err)
		return
	}
	if count > 0 {
		return
	}

	slog.Info("no users found, creating default seed user...")

	now := time.Now().UTC()
	userID := uuid.New()

	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, slug, display_name, timezone, language, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		userID, "admin", "Admin User", "UTC", "en", now)
	if err != nil {
		slog.Warn("failed to create seed user", "error", err)
		return
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO auth_bindings (id, user_id, provider, provider_id, provider_data, created_at)
		 VALUES ($1, $2, 'local', 'seed', '{}', $3)`,
		uuid.New(), userID, now)
	if err != nil {
		slog.Warn("failed to create seed auth binding", "error", err)
		return
	}

	user, err := userSvc.GetBySlug(ctx, "admin")
	if err != nil {
		slog.Warn("seed user created but could not verify", "error", err)
		return
	}

	slog.Info("seed user created", "id", user.ID, "slug", user.Slug)
	_ = pgx.ErrNoRows
}
