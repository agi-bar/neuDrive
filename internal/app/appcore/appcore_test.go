package appcore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/agi-bar/agenthub/internal/mcp"
	"github.com/agi-bar/agenthub/internal/models"
	sqlitestorage "github.com/agi-bar/agenthub/internal/storage/sqlite"
)

func TestBuildSQLiteAppProvidesHTTPAndMCP(t *testing.T) {
	ctx := context.Background()
	sqlitePath := filepath.Join(t.TempDir(), "agenthub.db")

	app, err := Build(ctx, Options{
		Storage:       "sqlite",
		SQLitePath:    sqlitePath,
		PublicBaseURL: "http://127.0.0.1:42690",
		RunMigrations: false,
	})
	if err != nil {
		t.Fatalf("Build sqlite app: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	store, err := sqlitestorage.Open(sqlitePath)
	if err != nil {
		t.Fatalf("Open sqlite store: %v", err)
	}
	defer store.Close()

	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	tokenResp, err := store.CreateToken(ctx, user.ID, "admin", []string{models.ScopeAdmin}, models.TrustLevelFull, time.Hour)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	handler, err := app.NewMCPServer(tokenResp.Token)
	if err != nil {
		t.Fatalf("NewMCPServer: %v", err)
	}
	resp := handler.HandleJSONRPC(mcp.JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize"})
	if resp.Error != nil {
		t.Fatalf("unexpected MCP initialize error: %+v", resp.Error)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	app.HTTPHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/health status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestResolveStorageBackend(t *testing.T) {
	t.Run("explicit storage wins", func(t *testing.T) {
		if got := ResolveStorageBackend("sqlite", "", "postgres://ignored", DefaultServerStorage); got != "sqlite" {
			t.Fatalf("got %q want sqlite", got)
		}
	})

	t.Run("database url beats default", func(t *testing.T) {
		if got := ResolveStorageBackend("", "", "postgres://db", DefaultLocalStorage); got != "postgres" {
			t.Fatalf("got %q want postgres", got)
		}
	})

	t.Run("sqlite path beats server default", func(t *testing.T) {
		if got := ResolveStorageBackend("", "/tmp/agenthub.db", "", DefaultServerStorage); got != "sqlite" {
			t.Fatalf("got %q want sqlite", got)
		}
	})

	t.Run("falls back to requested mode default", func(t *testing.T) {
		if got := ResolveStorageBackend("", "", "", DefaultServerStorage); got != "postgres" {
			t.Fatalf("got %q want postgres", got)
		}
		if got := ResolveStorageBackend("", "", "", DefaultLocalStorage); got != "sqlite" {
			t.Fatalf("got %q want sqlite", got)
		}
	})
}
