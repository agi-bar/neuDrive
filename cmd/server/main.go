package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/agi-bar/agenthub/internal/api"
	"github.com/agi-bar/agenthub/internal/auth"
	"github.com/agi-bar/agenthub/internal/config"
	"github.com/agi-bar/agenthub/internal/database"
	"github.com/agi-bar/agenthub/internal/jobs"
	"github.com/agi-bar/agenthub/internal/logger"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/agi-bar/agenthub/internal/vault"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// ---------------------------------------------------------------
	// Configuration
	// ---------------------------------------------------------------
	cfg, err := config.Load()
	if err != nil {
		// Logger not yet initialised; use a basic slog call.
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// ---------------------------------------------------------------
	// Logger
	// ---------------------------------------------------------------
	logger.Init(cfg.LogLevel, cfg.LogFormat)
	slog.Info("starting Agent Hub server...")

	// ---------------------------------------------------------------
	// Database
	// ---------------------------------------------------------------
	pool, err := database.InitDB(cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// ---------------------------------------------------------------
	// Migrations
	// ---------------------------------------------------------------
	migrationsDir := resolveMigrationsDir()
	if err := database.RunMigrations(pool, migrationsDir); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// ---------------------------------------------------------------
	// Vault
	// ---------------------------------------------------------------
	v, err := vault.NewVault(cfg.VaultMasterKey)
	if err != nil {
		slog.Error("failed to initialize vault", "error", err)
		os.Exit(1)
	}
	slog.Info("vault initialized")

	// ---------------------------------------------------------------
	// Services
	// ---------------------------------------------------------------
	userSvc := services.NewUserService(pool)

	// Token generator closure capturing the JWT secret.
	tokenGen := func(userID uuid.UUID, slug string) (string, error) {
		return auth.GenerateToken(userID, slug, cfg.JWTSecret)
	}

	// GitHub exchange closure capturing client credentials.
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
	collabSvc := services.NewCollaborationService(pool)
	webhookSvc := services.NewWebhookService(pool)
	oauthSvc := services.NewOAuthService(pool, cfg.JWTSecret)

	// Wire webhook triggers into services that emit events.
	inboxSvc.Webhook = webhookSvc
	memorySvc.Webhook = webhookSvc

	// ---------------------------------------------------------------
	// Seed default user if database is empty
	// ---------------------------------------------------------------
	seedDefaultUser(pool, userSvc)

	// ---------------------------------------------------------------
	// HTTP Server
	// ---------------------------------------------------------------
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
		collabSvc,
		webhookSvc,
		oauthSvc,
		v,
		cfg.JWTSecret,
		cfg.GithubClientID,
		cfg.GithubClientSecret,
	)

	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      srv.Router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start listening in background.
	go func() {
		slog.Info("server listening", "port", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// ---------------------------------------------------------------
	// Background Jobs
	// ---------------------------------------------------------------
	scheduler := jobs.NewScheduler(memorySvc, tokenSvc, inboxSvc, slog.Default())
	scheduler.Start(context.Background())

	// ---------------------------------------------------------------
	// Graceful shutdown
	// ---------------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")

	scheduler.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

// resolveMigrationsDir tries several common locations for the migrations
// directory relative to the executable and working directory.
func resolveMigrationsDir() string {
	// Try relative to the executable first.
	execPath, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(execPath), "..", "..", "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	// Fall back to working-directory-relative path.
	if info, err := os.Stat("migrations"); err == nil && info.IsDir() {
		return "migrations"
	}

	// Last resort: return the default and let RunMigrations handle missing dir.
	return "migrations"
}

// seedDefaultUser creates a default "admin" user when the users table is empty.
// This makes it possible to obtain a dev token immediately after first launch.
func seedDefaultUser(pool *pgxpool.Pool, userSvc *services.UserService) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		// Table may not exist yet if migrations haven't created it.
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

	// Also create an auth binding so the user can be looked up.
	_, err = pool.Exec(ctx,
		`INSERT INTO auth_bindings (id, user_id, provider, provider_id, provider_data, created_at)
		 VALUES ($1, $2, 'local', 'seed', '{}', $3)`,
		uuid.New(), userID, now)
	if err != nil {
		slog.Warn("failed to create seed auth binding", "error", err)
		return
	}

	// Verify the user was created.
	user, err := userSvc.GetBySlug(ctx, "admin")
	if err != nil {
		slog.Warn("seed user created but could not verify", "error", err)
		return
	}

	slog.Info("seed user created", "id", user.ID, "slug", user.Slug)

	// Suppress unused import of pgx if only used for ErrNoRows check.
	_ = pgx.ErrNoRows
}
