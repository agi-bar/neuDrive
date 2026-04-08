package mcpapp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/agi-bar/agenthub/internal/app/appcore"
	"github.com/agi-bar/agenthub/internal/mcp"
	"github.com/agi-bar/agenthub/internal/runtimecfg"
)

const DefaultTokenEnvVar = "AGENTHUB_TOKEN"

type Options struct {
	Storage        string
	SQLitePath     string
	Token          string
	TokenEnv       string
	DatabaseURL    string
	JWTSecret      string
	VaultMasterKey string
	PublicBaseURL  string
}

func RunStdio(ctx context.Context, opts Options) error {
	if ctx == nil {
		ctx = context.Background()
	}
	tokenEnv := opts.TokenEnv
	if tokenEnv == "" {
		tokenEnv = DefaultTokenEnvVar
	}
	resolvedToken, err := ResolveToken(opts.Token, tokenEnv)
	if err != nil {
		return err
	}

	storage := appcore.ResolveStorageBackend(opts.Storage, opts.SQLitePath, opts.DatabaseURL, appcore.DefaultLocalStorage)
	if storage == "sqlite" {
		sqlitePath := strings.TrimSpace(opts.SQLitePath)
		if sqlitePath == "" {
			sqlitePath = runtimecfg.DefaultSQLitePath()
		}
		opts.SQLitePath = sqlitePath
	}

	app, err := appcore.Build(ctx, appcore.Options{
		Storage:        storage,
		SQLitePath:     opts.SQLitePath,
		DatabaseURL:    opts.DatabaseURL,
		JWTSecret:      opts.JWTSecret,
		VaultMasterKey: opts.VaultMasterKey,
		PublicBaseURL:  opts.PublicBaseURL,
	})
	if err != nil {
		return err
	}
	defer func() { _ = app.Close() }()

	handler, err := app.NewMCPServer(resolvedToken)
	if err != nil {
		return err
	}

	baseURL := opts.PublicBaseURL
	if baseURL == "" && app.Config != nil {
		baseURL = app.Config.PublicBaseURL
	}
	if baseURL != "" {
		fmt.Fprintf(os.Stderr, "agenthub mcp stdio: storage=%s base_url=%s, waiting for requests...\n", app.Storage, baseURL)
	} else {
		fmt.Fprintf(os.Stderr, "agenthub mcp stdio: storage=%s, waiting for requests...\n", app.Storage)
	}
	if runner, ok := handler.(interface {
		RunStdio(in *os.File, out *os.File) error
	}); ok {
		if err := runner.RunStdio(os.Stdin, os.Stdout); err != nil {
			slog.Error("stdio error", "error", err)
			return err
		}
		return nil
	}
	if err := mcp.RunStdioHandler(handler, os.Stdin, os.Stdout); err != nil {
		slog.Error("stdio error", "error", err)
		return err
	}
	return nil
}

func ResolveToken(explicitToken, tokenEnvName string) (string, error) {
	token := strings.TrimSpace(explicitToken)
	if token != "" {
		return token, nil
	}
	envName := strings.TrimSpace(tokenEnvName)
	if envName == "" {
		envName = DefaultTokenEnvVar
	}
	token = strings.TrimSpace(os.Getenv(envName))
	if token != "" {
		return token, nil
	}
	return "", fmt.Errorf("missing token: provide --token or set %s", envName)
}
