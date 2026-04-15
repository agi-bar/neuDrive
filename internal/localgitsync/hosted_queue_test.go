package localgitsync

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agi-bar/neudrive/internal/models"
	sqlitestorage "github.com/agi-bar/neudrive/internal/storage/sqlite"
	"github.com/agi-bar/neudrive/internal/vault"
)

func TestHostedQueuedMirrorSyncProcessesQueuedRows(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestorage.Open(filepath.Join(t.TempDir(), "hosted.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	v, err := vault.NewVault(strings.Repeat("0", 64))
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}

	hostedRoot := filepath.Join(t.TempDir(), "hosted-root")
	svc := New(
		store,
		v,
		WithExecutionMode(ExecutionModeHosted),
		WithHostedRoot(hostedRoot),
	)

	if _, err := svc.UpdateMirrorSettings(ctx, user.ID, MirrorSettingsUpdate{
		AutoCommitEnabled: true,
		AutoPushEnabled:   false,
		AuthMode:          AuthModeGitHubToken,
		RemoteName:        DefaultRemoteName,
		RemoteURL:         "https://github.com/acme/demo.git",
		RemoteBranch:      DefaultRemoteBranch,
	}); err != nil {
		t.Fatalf("UpdateMirrorSettings: %v", err)
	}
	if _, err := store.WriteEntry(ctx, user.ID, "/notes/demo.md", "queued", "text/markdown", models.FileTreeWriteOptions{}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	if info, err := svc.MarkMirrorQueued(ctx, user.ID, "write"); err != nil {
		t.Fatalf("MarkMirrorQueued: %v", err)
	} else if info == nil || info.SyncState != SyncStateQueued {
		t.Fatalf("expected queued sync info, got %+v", info)
	}
	if err := svc.RunQueuedGitMirrorSyncs(ctx, 10); err != nil {
		t.Fatalf("RunQueuedGitMirrorSyncs: %v", err)
	}

	active, err := svc.GetActiveMirror(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetActiveMirror: %v", err)
	}
	if active == nil {
		t.Fatal("expected hosted mirror row")
	}
	if active.SyncState != SyncStateIdle {
		t.Fatalf("sync_state = %q, want %q", active.SyncState, SyncStateIdle)
	}
	if active.LastSyncedAt == nil {
		t.Fatalf("expected last_synced_at to be set: %+v", active)
	}
	if got, want := active.RootPath, filepath.Join(hostedRoot, user.ID.String()); got != want {
		t.Fatalf("root_path = %q, want %q", got, want)
	}
	if got := gitOutput(t, "git", "-C", active.RootPath, "rev-list", "--count", "HEAD"); got != "1" {
		t.Fatalf("hosted mirror commit count = %q, want 1", got)
	}
}
