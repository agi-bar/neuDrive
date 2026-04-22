package api

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/platforms"
	"github.com/agi-bar/neudrive/internal/runtimecfg"
	"github.com/agi-bar/neudrive/internal/services"
	sqlitestorage "github.com/agi-bar/neudrive/internal/storage/sqlite"
)

func TestLocalPlatformPreviewCacheRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := sqlitestorage.Open(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}

	server := &Server{
		FileTreeService: services.NewFileTreeServiceWithRepo(sqlitestorage.NewFileTreeRepo(store)),
	}
	preview := &platforms.ImportPreview{
		Platform:    "claude-code",
		DisplayName: "Claude Code",
		Mode:        platforms.ImportModeAgent,
		StartedAt:   "2026-04-16T20:00:00Z",
		CompletedAt: "2026-04-16T20:00:42Z",
		DurationMs:  42000,
		Categories: []platforms.ImportPreviewCategory{
			{Name: "conversations", Discovered: 12, Importable: 12},
		},
		Notes:       []string{"cached preview"},
		NextCommand: "neudrive import claude",
	}

	if err := server.writeLocalPlatformPreviewCache(ctx, user.ID, "claude-code", platforms.ImportModeAgent, preview); err != nil {
		t.Fatalf("writeLocalPlatformPreviewCache: %v", err)
	}

	entry, err := server.FileTreeService.Read(ctx, user.ID, "/platforms/claude-code/dashboard/latest-preview-agent.json", models.TrustLevelFull)
	if err != nil {
		t.Fatalf("Read(cache file): %v", err)
	}
	if entry.ContentType != "application/json" {
		t.Fatalf("cache content type = %q, want application/json", entry.ContentType)
	}

	cached, err := server.readLocalPlatformPreviewCache(ctx, user.ID, "claude-code", platforms.ImportModeAgent)
	if err != nil {
		t.Fatalf("readLocalPlatformPreviewCache: %v", err)
	}
	if cached == nil {
		t.Fatal("readLocalPlatformPreviewCache returned nil")
	}
	if cached.Platform != "claude-code" {
		t.Fatalf("cached.Platform = %q, want claude-code", cached.Platform)
	}
	if cached.DurationMs != 42000 {
		t.Fatalf("cached.DurationMs = %d, want 42000", cached.DurationMs)
	}
	if len(cached.Categories) != 1 || cached.Categories[0].Name != "conversations" {
		t.Fatalf("cached categories = %+v", cached.Categories)
	}
}

func TestLocalPlatformPreviewTaskLifecycle(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestorage.Open(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}

	originalPreviewImportFunc := previewImportFunc
	previewImportFunc = func(ctx context.Context, cfg *runtimecfg.CLIConfig, platform, rawMode string) (*platforms.ImportPreview, error) {
		time.Sleep(20 * time.Millisecond)
		return &platforms.ImportPreview{
			Platform:    platform,
			DisplayName: "Claude Code",
			Mode:        platforms.ImportModeAgent,
			StartedAt:   "2026-04-16T20:00:00Z",
			CompletedAt: "2026-04-16T20:00:20Z",
			DurationMs:  20000,
			Categories: []platforms.ImportPreviewCategory{
				{Name: "conversations", Discovered: 5, Importable: 5},
			},
			NextCommand: "neudrive import claude",
		}, nil
	}
	t.Cleanup(func() { previewImportFunc = originalPreviewImportFunc })

	server := &Server{
		FileTreeService: services.NewFileTreeServiceWithRepo(sqlitestorage.NewFileTreeRepo(store)),
	}

	adapter, err := platforms.Resolve("claude")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	task, err := server.startLocalPlatformPreviewTask(ctx, user.ID, adapter, platforms.ImportModeAgent)
	if err != nil {
		t.Fatalf("startLocalPlatformPreviewTask: %v", err)
	}
	if task == nil || task.Status == nil {
		t.Fatal("expected task status")
	}
	if task.Status.State != localPlatformPreviewTaskStateRunning {
		t.Fatalf("task.Status.State = %q, want running", task.Status.State)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, err := server.readLocalPlatformPreviewTask(ctx, user.ID, adapter.ID(), platforms.ImportModeAgent)
		if err != nil {
			t.Fatalf("readLocalPlatformPreviewTask: %v", err)
		}
		if current != nil && current.Status != nil && current.Status.State == localPlatformPreviewTaskStateSucceeded {
			if current.Preview == nil {
				t.Fatal("expected cached preview after task completion")
			}
			if current.Preview.Platform != adapter.ID() {
				t.Fatalf("current.Preview.Platform = %q, want %q", current.Preview.Platform, adapter.ID())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timed out waiting for local platform preview task to succeed")
}
