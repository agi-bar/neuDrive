package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/agi-bar/agenthub/internal/config"
	"github.com/agi-bar/agenthub/internal/database"
	"github.com/agi-bar/agenthub/internal/mcp"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/agi-bar/agenthub/internal/vault"
)

func main() {
	token := flag.String("token", "", "Scoped access token (aht_...)")
	flag.Parse()

	if *token == "" {
		fmt.Fprintln(os.Stderr, "error: --token is required")
		fmt.Fprintln(os.Stderr, "usage: agenthub-mcp --token aht_xxxxx")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	db, err := database.InitDB(cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Validate token
	tokenSvc := services.NewTokenService(db)
	scopedToken, err := tokenSvc.ValidateToken(context.Background(), *token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid token: %v\n", err)
		os.Exit(1)
	}

	// Initialize vault
	var v *vault.Vault
	if cfg.VaultMasterKey != "" {
		v, err = vault.NewVault(cfg.VaultMasterKey)
		if err != nil {
			slog.Error("vault init failed", "error", err)
			os.Exit(1)
		}
	}

	// Create services
	fileTreeSvc := services.NewFileTreeService(db)
	vaultSvc := services.NewVaultService(db, v)
	memorySvc := services.NewMemoryService(db, fileTreeSvc)
	roleSvc := services.NewRoleService(db, fileTreeSvc)
	projectSvc := services.NewProjectService(db, roleSvc, fileTreeSvc)
	inboxSvc := services.NewInboxService(db, fileTreeSvc)
	deviceSvc := services.NewDeviceService(db, fileTreeSvc)
	dashboardSvc := services.NewDashboardService(db)
	importSvc := services.NewImportService(db, fileTreeSvc, memorySvc, vaultSvc)

	// Create MCP server
	server := &mcp.MCPServer{
		UserID:      scopedToken.UserID,
		TrustLevel:  scopedToken.MaxTrustLevel,
		Scopes:      scopedToken.Scopes,
		FileTree:    fileTreeSvc,
		Vault:       vaultSvc,
		VaultCrypto: v,
		Memory:      memorySvc,
		Project:     projectSvc,
		Inbox:       inboxSvc,
		Device:      deviceSvc,
		Dashboard:   dashboardSvc,
		Import:      importSvc,
	}

	// Run stdio transport
	fmt.Fprintln(os.Stderr, "agenthub-mcp: connected, waiting for requests...")
	if err := server.RunStdio(os.Stdin, os.Stdout); err != nil {
		slog.Error("stdio error", "error", err)
		os.Exit(1)
	}
}
