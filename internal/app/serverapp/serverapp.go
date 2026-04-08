package serverapp

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/app/appcore"
	"github.com/agi-bar/agenthub/internal/jobs"
	"github.com/agi-bar/agenthub/internal/logger"
	"github.com/agi-bar/agenthub/internal/runtimecfg"
	"github.com/agi-bar/agenthub/internal/services"
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
			sqlitePath = runtimecfg.DefaultSQLitePath()
		}
		opts.SQLitePath = sqlitePath
	}

	listenAddr := opts.ListenAddr
	if listenAddr == "" {
		listenAddr = ":42690"
	}

	app, err := appcore.Build(ctx, appcore.Options{
		Storage:            storage,
		SQLitePath:         opts.SQLitePath,
		DatabaseURL:        opts.DatabaseURL,
		JWTSecret:          opts.JWTSecret,
		VaultMasterKey:     opts.VaultMasterKey,
		PublicBaseURL:      opts.PublicBaseURL,
		CORSOrigins:        opts.CORSOrigins,
		GithubClientID:     opts.GithubClientID,
		GithubClientSecret: opts.GithubClientSecret,
		LogLevel:           opts.LogLevel,
		LogFormat:          opts.LogFormat,
		RunMigrations:      storage == "postgres",
	})
	if err != nil {
		return err
	}
	defer func() { _ = app.Close() }()

	cfg := app.Config
	if cfg != nil && opts.ListenAddr == "" {
		listenAddr = ":" + cfg.Port
	}

	logLevel := opts.LogLevel
	logFormat := opts.LogFormat
	if cfg != nil {
		if logLevel == "" {
			logLevel = cfg.LogLevel
		}
		if logFormat == "" {
			logFormat = cfg.LogFormat
		}
	}
	logger.Init(logLevel, logFormat)
	slog.Info("starting Agent Hub server...", "listen", listenAddr)
	if cfg != nil && cfg.CaptureOAuth {
		slog.Info("oauth capture enabled", "dir", cfg.CaptureDir)
	}

	httpServer := &http.Server{
		Addr:         listenAddr,
		Handler:      app.HTTPHandler,
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

	if app.Storage == "postgres" && app.MemoryService != nil && app.InboxService != nil && app.SyncService != nil {
		if tokenSvc, ok := app.TokenService.(*services.TokenService); ok {
			scheduler := jobs.NewScheduler(app.MemoryService, tokenSvc, app.InboxService, app.SyncService, slog.Default())
			scheduler.Start(context.Background())
			defer scheduler.Stop()
		}
	}

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
