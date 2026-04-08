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

	"github.com/agi-bar/agenthub/internal/api"
	"github.com/agi-bar/agenthub/internal/auth"
	"github.com/agi-bar/agenthub/internal/config"
	"github.com/agi-bar/agenthub/internal/database"
	"github.com/agi-bar/agenthub/internal/mcp"
	"github.com/agi-bar/agenthub/internal/runtimecfg"
	"github.com/agi-bar/agenthub/internal/services"
	sqlitestorage "github.com/agi-bar/agenthub/internal/storage/sqlite"
	"github.com/agi-bar/agenthub/internal/vault"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	Storage            string
	SQLitePath         string
	DatabaseURL        string
	JWTSecret          string
	VaultMasterKey     string
	PublicBaseURL      string
	CORSOrigins        []string
	GithubClientID     string
	GithubClientSecret string
	LogLevel           string
	LogFormat          string
	RunMigrations      bool
}

type App struct {
	Storage      string
	Config       *config.Config
	HTTPHandler  http.Handler
	NewMCPServer func(token string) (mcp.JSONRPCHandler, error)
	Close        func() error

	MemoryService *services.MemoryService
	TokenService  any
	InboxService  *services.InboxService
	SyncService   *services.SyncService
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
	if _, err := store.EnsureOwner(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}

	v, err := vault.NewVault(cfg.VaultMasterKey)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	fileTreeSvc := services.NewFileTreeServiceWithRepo(sqlitestorage.NewFileTreeRepo(store))
	memorySvc := services.NewMemoryServiceWithRepo(sqlitestorage.NewMemoryRepo(store), nil)
	userSvc := services.NewUserServiceWithRepo(sqlitestorage.NewUserRepo(store))
	connSvc := services.NewConnectionServiceWithRepo(sqlitestorage.NewConnectionRepo(store))
	vaultSvc := services.NewVaultServiceWithRepo(sqlitestorage.NewVaultRepo(store), v)
	roleSvc := services.NewRoleServiceWithRepo(sqlitestorage.NewRoleRepo(store), fileTreeSvc)
	inboxSvc := services.NewInboxServiceWithRepo(sqlitestorage.NewInboxRepo(store), fileTreeSvc)
	projectSvc := services.NewProjectServiceWithRepo(sqlitestorage.NewProjectRepo(store), roleSvc, fileTreeSvc)
	tokenSvc := services.NewTokenServiceWithRepo(sqlitestorage.NewTokenRepo(store))
	deviceSvc := services.NewDeviceServiceWithRepo(sqlitestorage.NewDeviceRepo(store), fileTreeSvc)
	importSvc := services.NewImportService(nil, fileTreeSvc, memorySvc, vaultSvc)
	exportSvc := services.NewExportService(fileTreeSvc, memorySvc, projectSvc, vaultSvc, deviceSvc, inboxSvc, roleSvc, userSvc)
	syncSvc := services.NewSyncServiceWithRepo(sqlitestorage.NewSyncRepo(store), importSvc, exportSvc, fileTreeSvc, memorySvc)
	dashboardSvc := services.NewDashboardServiceWithRepo(sqlitestorage.NewDashboardRepo(store))
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
	oauthSvc := services.NewOAuthServiceWithRepo(sqlitestorage.NewOAuthRepo(store), cfg.JWTSecret)
	httpServer := api.NewServerWithDeps(api.ServerDeps{
		Storage:            "sqlite",
		Config:             cfg,
		UserService:        userSvc,
		AuthService:        authSvc,
		ConnectionService:  connSvc,
		FileTreeService:    fileTreeSvc,
		VaultService:       vaultSvc,
		MemoryService:      memorySvc,
		ProjectService:     projectSvc,
		RoleService:        roleSvc,
		InboxService:       inboxSvc,
		DeviceService:      deviceSvc,
		DashboardService:   dashboardSvc,
		TokenService:       tokenSvc,
		ImportService:      importSvc,
		ExportService:      exportSvc,
		SyncService:        syncSvc,
		OAuthService:       oauthSvc,
		Vault:              v,
		JWTSecret:          cfg.JWTSecret,
		GitHubClientID:     cfg.GithubClientID,
		GitHubClientSecret: cfg.GithubClientSecret,
	})
	app := &App{
		Storage:       "sqlite",
		Config:        cfg,
		HTTPHandler:   httpServer.Router,
		MemoryService: memorySvc,
		TokenService:  tokenSvc,
		InboxService:  inboxSvc,
		SyncService:   syncSvc,
		NewMCPServer: func(token string) (mcp.JSONRPCHandler, error) {
			scopedToken, err := tokenSvc.ValidateToken(ctx, token)
			if err != nil {
				return nil, fmt.Errorf("invalid token: %w", err)
			}
			return &mcp.MCPServer{
				UserID:      scopedToken.UserID,
				TrustLevel:  scopedToken.MaxTrustLevel,
				Scopes:      scopedToken.Scopes,
				BaseURL:     cfg.PublicBaseURL,
				Connection:  connSvc,
				OAuth:       oauthSvc,
				FileTree:    fileTreeSvc,
				Vault:       vaultSvc,
				VaultCrypto: v,
				Memory:      memorySvc,
				Project:     projectSvc,
				Inbox:       inboxSvc,
				Device:      deviceSvc,
				Dashboard:   dashboardSvc,
				Import:      importSvc,
				Token:       tokenSvc,
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

	httpServer := api.NewServerWithDeps(api.ServerDeps{
		Storage:              "postgres",
		Config:               cfg,
		UserService:          deps.userSvc,
		AuthService:          deps.authSvc,
		ConnectionService:    deps.connSvc,
		FileTreeService:      deps.fileTreeSvc,
		VaultService:         deps.vaultSvc,
		MemoryService:        deps.memorySvc,
		ProjectService:       deps.projectSvc,
		SummaryService:       deps.summarySvc,
		RoleService:          deps.roleSvc,
		InboxService:         deps.inboxSvc,
		DeviceService:        deps.deviceSvc,
		DashboardService:     deps.dashboardSvc,
		TokenService:         deps.tokenSvc,
		ImportService:        deps.importSvc,
		ExportService:        deps.exportSvc,
		SyncService:          deps.syncSvc,
		CollaborationService: deps.collabSvc,
		WebhookService:       deps.webhookSvc,
		OAuthService:         deps.oauthSvc,
		Vault:                deps.vaultCrypto,
		JWTSecret:            cfg.JWTSecret,
		GitHubClientID:       cfg.GithubClientID,
		GitHubClientSecret:   cfg.GithubClientSecret,
	})

	app := &App{
		Storage:       "postgres",
		Config:        cfg,
		HTTPHandler:   httpServer.Router,
		MemoryService: deps.memorySvc,
		TokenService:  deps.tokenSvc,
		InboxService:  deps.inboxSvc,
		SyncService:   deps.syncSvc,
		NewMCPServer: func(token string) (mcp.JSONRPCHandler, error) {
			scopedToken, err := deps.tokenSvc.ValidateToken(ctx, token)
			if err != nil {
				return nil, fmt.Errorf("invalid token: %w", err)
			}
			return &mcp.MCPServer{
				UserID:      scopedToken.UserID,
				TrustLevel:  scopedToken.MaxTrustLevel,
				Scopes:      scopedToken.Scopes,
				BaseURL:     cfg.PublicBaseURL,
				Connection:  deps.connSvc,
				OAuth:       deps.oauthSvc,
				FileTree:    deps.fileTreeSvc,
				Vault:       deps.vaultSvc,
				VaultCrypto: deps.vaultCrypto,
				Memory:      deps.memorySvc,
				Project:     deps.projectSvc,
				Inbox:       deps.inboxSvc,
				Device:      deps.deviceSvc,
				Dashboard:   deps.dashboardSvc,
				Import:      deps.importSvc,
				Token:       deps.tokenSvc,
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
	userSvc      *services.UserService
	authSvc      *services.AuthService
	connSvc      *services.ConnectionService
	fileTreeSvc  *services.FileTreeService
	vaultSvc     *services.VaultService
	memorySvc    *services.MemoryService
	projectSvc   *services.ProjectService
	summarySvc   *services.SummaryService
	roleSvc      *services.RoleService
	inboxSvc     *services.InboxService
	deviceSvc    *services.DeviceService
	dashboardSvc *services.DashboardService
	tokenSvc     *services.TokenService
	importSvc    *services.ImportService
	exportSvc    *services.ExportService
	syncSvc      *services.SyncService
	collabSvc    *services.CollaborationService
	webhookSvc   *services.WebhookService
	oauthSvc     *services.OAuthService
	vaultCrypto  *vault.Vault
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
	connSvc := services.NewConnectionService(db)
	fileTreeSvc := services.NewFileTreeService(db)
	vaultSvc := services.NewVaultService(db, v)
	memorySvc := services.NewMemoryService(db, fileTreeSvc)
	roleSvc := services.NewRoleService(db, fileTreeSvc)
	projectSvc := services.NewProjectService(db, roleSvc, fileTreeSvc)
	summarySvc := services.NewSummaryService(db, projectSvc)
	inboxSvc := services.NewInboxService(db, fileTreeSvc)
	deviceSvc := services.NewDeviceService(db, fileTreeSvc)
	dashboardSvc := services.NewDashboardService(db)
	tokenSvc := services.NewTokenService(db)
	importSvc := services.NewImportService(db, fileTreeSvc, memorySvc, vaultSvc)
	exportSvc := services.NewExportService(fileTreeSvc, memorySvc, projectSvc, vaultSvc, deviceSvc, inboxSvc, roleSvc, userSvc)
	syncSvc := services.NewSyncService(db, importSvc, exportSvc, fileTreeSvc, memorySvc)
	collabSvc := services.NewCollaborationService(db)
	webhookSvc := services.NewWebhookService(db)
	oauthSvc := services.NewOAuthService(db, cfg.JWTSecret)

	inboxSvc.Webhook = webhookSvc
	memorySvc.Webhook = webhookSvc
	seedDefaultUser(db, userSvc)

	return &postgresDeps{
		userSvc:      userSvc,
		authSvc:      authSvc,
		connSvc:      connSvc,
		fileTreeSvc:  fileTreeSvc,
		vaultSvc:     vaultSvc,
		memorySvc:    memorySvc,
		projectSvc:   projectSvc,
		summarySvc:   summarySvc,
		roleSvc:      roleSvc,
		inboxSvc:     inboxSvc,
		deviceSvc:    deviceSvc,
		dashboardSvc: dashboardSvc,
		tokenSvc:     tokenSvc,
		importSvc:    importSvc,
		exportSvc:    exportSvc,
		syncSvc:      syncSvc,
		collabSvc:    collabSvc,
		webhookSvc:   webhookSvc,
		oauthSvc:     oauthSvc,
		vaultCrypto:  v,
	}, nil
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
		sqliteOpts.JWTSecret = "agenthub-local-sqlite-jwt-secret"
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
