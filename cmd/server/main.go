package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/agi-bar/agenthub/internal/api"
	"github.com/agi-bar/agenthub/internal/config"
	"github.com/agi-bar/agenthub/internal/database"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/agi-bar/agenthub/internal/vault"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("starting Agent Hub server...")

	// ---------------------------------------------------------------
	// Configuration
	// ---------------------------------------------------------------
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// ---------------------------------------------------------------
	// Database
	// ---------------------------------------------------------------
	pool, err := database.InitDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer database.Close()

	// ---------------------------------------------------------------
	// Migrations
	// ---------------------------------------------------------------
	migrationsDir := resolveMigrationsDir()
	if err := database.RunMigrations(pool, migrationsDir); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// ---------------------------------------------------------------
	// Vault
	// ---------------------------------------------------------------
	v, err := vault.NewVault(cfg.VaultMasterKey)
	if err != nil {
		log.Fatalf("failed to initialize vault: %v", err)
	}
	log.Println("vault initialized")

	// ---------------------------------------------------------------
	// Services
	// ---------------------------------------------------------------
	userSvc := services.NewUserService(pool)
	authSvc := services.NewAuthService(pool, cfg.JWTSecret, cfg.GithubClientID, cfg.GithubClientSecret)
	connSvc := services.NewConnectionService(pool)
	tokenSvc := services.NewTokenService(pool)

	// ---------------------------------------------------------------
	// Seed default user if database is empty
	// ---------------------------------------------------------------
	seedDefaultUser(pool, userSvc)

	// ---------------------------------------------------------------
	// HTTP Server
	// ---------------------------------------------------------------
	srv := api.NewServer(
		userSvc,
		authSvc,
		connSvc,
		v,
		cfg.JWTSecret,
		cfg.GithubClientID,
		cfg.GithubClientSecret,
	)
	srv.TokenService = tokenSvc

	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      srv.Router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start listening in background.
	go func() {
		log.Printf("server listening on :%s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// ---------------------------------------------------------------
	// Graceful shutdown
	// ---------------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}

	log.Println("server stopped")
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
		log.Printf("warning: could not check user count (table may not exist): %v", err)
		return
	}

	if count > 0 {
		return
	}

	log.Println("no users found, creating default seed user...")

	now := time.Now().UTC()
	userID := uuid.New()

	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, slug, display_name, timezone, language, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		userID, "admin", "Admin User", "UTC", "en", now)
	if err != nil {
		log.Printf("warning: failed to create seed user: %v", err)
		return
	}

	// Also create an auth binding so the user can be looked up.
	_, err = pool.Exec(ctx,
		`INSERT INTO auth_bindings (id, user_id, provider, provider_id, provider_data, created_at)
		 VALUES ($1, $2, 'local', 'seed', '{}', $3)`,
		uuid.New(), userID, now)
	if err != nil {
		log.Printf("warning: failed to create seed auth binding: %v", err)
		return
	}

	// Verify the user was created.
	user, err := userSvc.GetBySlug(ctx, "admin")
	if err != nil {
		log.Printf("warning: seed user created but could not verify: %v", err)
		return
	}

	log.Printf("seed user created: id=%s slug=%s", user.ID, user.Slug)

	// Suppress unused import of pgx if only used for ErrNoRows check.
	_ = pgx.ErrNoRows
}
