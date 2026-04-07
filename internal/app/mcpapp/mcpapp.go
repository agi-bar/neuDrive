package mcpapp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/agi-bar/agenthub/internal/config"
	"github.com/agi-bar/agenthub/internal/database"
	"github.com/agi-bar/agenthub/internal/localmcp"
	"github.com/agi-bar/agenthub/internal/localruntime"
	"github.com/agi-bar/agenthub/internal/localstore"
	"github.com/agi-bar/agenthub/internal/mcp"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/agi-bar/agenthub/internal/vault"
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
		store, err := localstore.Open(sqlitePath)
		if err != nil {
			return err
		}
		defer store.Close()
		if _, err := store.EnsureOwner(ctx); err != nil {
			return err
		}
		scopedToken, err := store.ValidateToken(ctx, resolvedToken)
		if err != nil {
			return fmt.Errorf("invalid token: %w", err)
		}
		server := &localmcp.Server{
			Store:      store,
			UserID:     scopedToken.UserID,
			TrustLevel: scopedToken.MaxTrustLevel,
			Scopes:     scopedToken.Scopes,
			BaseURL:    opts.PublicBaseURL,
		}
		fmt.Fprintln(os.Stderr, "agenthub mcp stdio: connected to local sqlite store, waiting for requests...")
		if err := server.RunStdio(os.Stdin, os.Stdout); err != nil {
			slog.Error("stdio error", "error", err)
			return err
		}
		return nil
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

	cfg, err := config.LoadWithOverrides(overrides)
	if err != nil {
		return err
	}

	db, err := database.InitDB(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	tokenSvc := services.NewTokenService(db)
	scopedToken, err := tokenSvc.ValidateToken(ctx, resolvedToken)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	var v *vault.Vault
	if cfg.VaultMasterKey != "" {
		v, err = vault.NewVault(cfg.VaultMasterKey)
		if err != nil {
			return err
		}
	}

	connectionSvc := services.NewConnectionService(db)
	fileTreeSvc := services.NewFileTreeService(db)
	vaultSvc := services.NewVaultService(db, v)
	memorySvc := services.NewMemoryService(db, fileTreeSvc)
	roleSvc := services.NewRoleService(db, fileTreeSvc)
	projectSvc := services.NewProjectService(db, roleSvc, fileTreeSvc)
	inboxSvc := services.NewInboxService(db, fileTreeSvc)
	deviceSvc := services.NewDeviceService(db, fileTreeSvc)
	dashboardSvc := services.NewDashboardService(db)
	importSvc := services.NewImportService(db, fileTreeSvc, memorySvc, vaultSvc)
	oauthSvc := services.NewOAuthService(db, cfg.JWTSecret)

	server := &mcp.MCPServer{
		UserID:      scopedToken.UserID,
		TrustLevel:  scopedToken.MaxTrustLevel,
		Scopes:      scopedToken.Scopes,
		BaseURL:     cfg.PublicBaseURL,
		Connection:  connectionSvc,
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
	}

	fmt.Fprintln(os.Stderr, "agenthub mcp stdio: connected, waiting for requests...")
	if err := server.RunStdio(os.Stdin, os.Stdout); err != nil {
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
