package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
		log.Fatalf("config error: %v", err)
	}

	db, err := database.InitDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer db.Close()

	// Validate token
	tokenSvc := &services.TokenService{}
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
			log.Fatalf("vault init failed: %v", err)
		}
	}

	// Create services
	fileTreeSvc := &services.FileTreeService{}
	vaultSvc := &services.VaultService{}
	memorySvc := &services.MemoryService{}
	projectSvc := &services.ProjectService{}
	inboxSvc := &services.InboxService{}
	deviceSvc := &services.DeviceService{}
	dashboardSvc := &services.DashboardService{}
	importSvc := &services.ImportService{}

	// Create MCP server
	server := &mcp.MCPServer{
		UserID:     scopedToken.UserID,
		TrustLevel: scopedToken.MaxTrustLevel,
		Scopes:     scopedToken.Scopes,
		FileTree:   fileTreeSvc,
		Vault:      vaultSvc,
		VaultCrypto: v,
		Memory:     memorySvc,
		Project:    projectSvc,
		Inbox:      inboxSvc,
		Device:     deviceSvc,
		Dashboard:  dashboardSvc,
		Import:     importSvc,
	}

	// Run stdio transport
	fmt.Fprintln(os.Stderr, "agenthub-mcp: connected, waiting for requests...")
	if err := server.RunStdio(os.Stdin, os.Stdout); err != nil {
		log.Fatalf("stdio error: %v", err)
	}
}
