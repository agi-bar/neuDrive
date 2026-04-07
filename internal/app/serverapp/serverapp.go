package serverapp

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/api"
	"github.com/agi-bar/agenthub/internal/auth"
	"github.com/agi-bar/agenthub/internal/config"
	"github.com/agi-bar/agenthub/internal/database"
	"github.com/agi-bar/agenthub/internal/jobs"
	"github.com/agi-bar/agenthub/internal/localruntime"
	"github.com/agi-bar/agenthub/internal/localserver"
	"github.com/agi-bar/agenthub/internal/logger"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/agi-bar/agenthub/internal/vault"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	Storage            string
	SQLitePath         string
	ListenAddr         string
	DatabaseURL        string
	JWTSecret          string
	VaultMasterKey     string
	PublicBaseURL      string
	CORSOrigins        []string
	GithubClientID     string
	GithubClientSecret string
	LogLevel           string
	LogFormat          string
}

func Run(ctx context.Context, opts Options) error {
	storage := strings.ToLower(strings.TrimSpace(opts.Storage))
	if storage == "" {
		if strings.TrimSpace(opts.DatabaseURL) != "" {
			storage = "postgres"
		} else {
			storage = "sqlite"
		}
	}
	if storage == "sqlite" {
		sqlitePath := strings.TrimSpace(opts.SQLitePath)
		if sqlitePath == "" {
			sqlitePath = localruntime.DefaultSQLitePath()
		}
		return localserver.Run(ctx, localserver.Options{
			ListenAddr: opts.ListenAddr,
			SQLitePath: sqlitePath,
			BaseURL:    opts.PublicBaseURL,
		})
	}

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
		overrides["CORS_ORIGINS"] = ""
		for i, origin := range opts.CORSOrigins {
			if i > 0 {
				overrides["CORS_ORIGINS"] += ","
			}
			overrides["CORS_ORIGINS"] += origin
		}
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

	cfg, err := config.LoadWithOverrides(overrides)
	if err != nil {
		return err
	}

	listenAddr := opts.ListenAddr
	if listenAddr == "" {
		listenAddr = ":" + cfg.Port
	}

	logger.Init(cfg.LogLevel, cfg.LogFormat)
	slog.Info("starting Agent Hub server...", "listen", listenAddr)
	if cfg.CaptureOAuth {
		slog.Info("oauth capture enabled", "dir", cfg.CaptureDir)
	}

	pool, err := database.InitDB(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer database.Close()

	migrationsDir := resolveMigrationsDir()
	if err := database.RunMigrations(pool, migrationsDir); err != nil {
		return err
	}

	v, err := vault.NewVault(cfg.VaultMasterKey)
	if err != nil {
		return err
	}
	slog.Info("vault initialized")

	userSvc := services.NewUserService(pool)
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

	authSvc := services.NewAuthService(pool, tokenGen, ghExchange)
	connSvc := services.NewConnectionService(pool)
	fileTreeSvc := services.NewFileTreeService(pool)
	vaultSvc := services.NewVaultService(pool, v)
	memorySvc := services.NewMemoryService(pool, fileTreeSvc)
	roleSvc := services.NewRoleService(pool, fileTreeSvc)
	projectSvc := services.NewProjectService(pool, roleSvc, fileTreeSvc)
	summarySvc := services.NewSummaryService(pool, projectSvc)
	inboxSvc := services.NewInboxService(pool, fileTreeSvc)
	deviceSvc := services.NewDeviceService(pool, fileTreeSvc)
	dashboardSvc := services.NewDashboardService(pool)
	tokenSvc := services.NewTokenService(pool)
	importSvc := services.NewImportService(pool, fileTreeSvc, memorySvc, vaultSvc)
	exportSvc := services.NewExportService(fileTreeSvc, memorySvc, projectSvc, vaultSvc, deviceSvc, inboxSvc, roleSvc, userSvc)
	syncSvc := services.NewSyncService(pool, importSvc, exportSvc, fileTreeSvc, memorySvc)
	collabSvc := services.NewCollaborationService(pool)
	webhookSvc := services.NewWebhookService(pool)
	oauthSvc := services.NewOAuthService(pool, cfg.JWTSecret)

	inboxSvc.Webhook = webhookSvc
	memorySvc.Webhook = webhookSvc

	seedDefaultUser(pool, userSvc)

	srv := api.NewServer(
		cfg,
		userSvc,
		authSvc,
		connSvc,
		fileTreeSvc,
		vaultSvc,
		memorySvc,
		projectSvc,
		summarySvc,
		roleSvc,
		inboxSvc,
		deviceSvc,
		dashboardSvc,
		tokenSvc,
		importSvc,
		exportSvc,
		syncSvc,
		collabSvc,
		webhookSvc,
		oauthSvc,
		v,
		cfg.JWTSecret,
		cfg.GithubClientID,
		cfg.GithubClientSecret,
	)

	httpServer := &http.Server{
		Addr:         listenAddr,
		Handler:      srv.Router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", listener.Addr().String())
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	scheduler := jobs.NewScheduler(memorySvc, tokenSvc, inboxSvc, syncSvc, slog.Default())
	scheduler.Start(context.Background())
	defer scheduler.Stop()

	select {
	case <-ctx.Done():
	case err := <-serverErr:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
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
