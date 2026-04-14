package localgitsync

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agi-bar/neudrive/internal/models"
	sqlitestorage "github.com/agi-bar/neudrive/internal/storage/sqlite"
	"github.com/agi-bar/neudrive/internal/vault"
	"github.com/google/uuid"
)

func TestSyncActiveMirrorAutoCommitCreatesOneCommitPerDirtySync(t *testing.T) {
	ctx := context.Background()
	store, svc, userID := newAutomationTestService(t)

	if _, err := store.WriteEntry(ctx, userID, "/notes/demo.md", "first", "text/markdown", models.FileTreeWriteOptions{}); err != nil {
		t.Fatalf("write initial note: %v", err)
	}

	mirrorDir := filepath.Join(t.TempDir(), "mirror")
	if _, err := svc.RegisterMirrorAndSync(ctx, userID, mirrorDir); err != nil {
		t.Fatalf("RegisterMirrorAndSync: %v", err)
	}
	if _, err := svc.UpdateMirrorSettings(ctx, userID, MirrorSettingsUpdate{
		AutoCommitEnabled: true,
		AutoPushEnabled:   false,
		AuthMode:          AuthModeLocalCredentials,
		RemoteName:        DefaultRemoteName,
		RemoteBranch:      DefaultRemoteBranch,
	}); err != nil {
		t.Fatalf("UpdateMirrorSettings: %v", err)
	}

	if _, err := store.WriteEntry(ctx, userID, "/notes/demo.md", "second", "text/markdown", models.FileTreeWriteOptions{}); err != nil {
		t.Fatalf("write updated note: %v", err)
	}

	info, err := svc.SyncActiveMirror(ctx, userID)
	if err != nil {
		t.Fatalf("SyncActiveMirror dirty: %v", err)
	}
	if info == nil || !info.CommitCreated || info.LastCommitHash == "" {
		t.Fatalf("expected commit metadata in sync info: %+v", info)
	}

	if got := gitOutput(t, "git", "-C", mirrorDir, "rev-list", "--count", "HEAD"); got != "1" {
		t.Fatalf("commit count after dirty sync = %q, want 1", got)
	}

	info, err = svc.SyncActiveMirror(ctx, userID)
	if err != nil {
		t.Fatalf("SyncActiveMirror clean: %v", err)
	}
	if info.CommitCreated {
		t.Fatalf("expected clean sync not to create a commit: %+v", info)
	}
	if got := gitOutput(t, "git", "-C", mirrorDir, "rev-list", "--count", "HEAD"); got != "1" {
		t.Fatalf("commit count after clean sync = %q, want 1", got)
	}
}

func TestSyncActiveMirrorAutoPushLocalCredentials(t *testing.T) {
	ctx := context.Background()
	store, svc, userID := newAutomationTestService(t)

	if _, err := store.WriteEntry(ctx, userID, "/notes/demo.md", "first", "text/markdown", models.FileTreeWriteOptions{}); err != nil {
		t.Fatalf("write initial note: %v", err)
	}

	mirrorDir := filepath.Join(t.TempDir(), "mirror")
	if _, err := svc.RegisterMirrorAndSync(ctx, userID, mirrorDir); err != nil {
		t.Fatalf("RegisterMirrorAndSync: %v", err)
	}

	bareRemote := filepath.Join(t.TempDir(), "remote.git")
	gitOutput(t, "git", "init", "--bare", bareRemote)

	if _, err := svc.UpdateMirrorSettings(ctx, userID, MirrorSettingsUpdate{
		AutoCommitEnabled: true,
		AutoPushEnabled:   true,
		AuthMode:          AuthModeLocalCredentials,
		RemoteName:        DefaultRemoteName,
		RemoteURL:         bareRemote,
		RemoteBranch:      "main",
	}); err != nil {
		t.Fatalf("UpdateMirrorSettings: %v", err)
	}

	if _, err := store.WriteEntry(ctx, userID, "/notes/demo.md", "second", "text/markdown", models.FileTreeWriteOptions{}); err != nil {
		t.Fatalf("write updated note: %v", err)
	}

	info, err := svc.SyncActiveMirror(ctx, userID)
	if err != nil {
		t.Fatalf("SyncActiveMirror: %v", err)
	}
	if info == nil || !info.PushAttempted || !info.PushSucceeded {
		t.Fatalf("expected successful push info: %+v", info)
	}
	if got := gitOutput(t, "git", "--git-dir", bareRemote, "rev-parse", "--verify", "refs/heads/main"); len(got) != 40 {
		t.Fatalf("expected remote main branch sha, got %q", got)
	}
}

func TestSyncActiveMirrorPushFailureDoesNotFailTheWrite(t *testing.T) {
	ctx := context.Background()
	store, svc, userID := newAutomationTestService(t)

	if _, err := store.WriteEntry(ctx, userID, "/notes/demo.md", "first", "text/markdown", models.FileTreeWriteOptions{}); err != nil {
		t.Fatalf("write initial note: %v", err)
	}

	mirrorDir := filepath.Join(t.TempDir(), "mirror")
	if _, err := svc.RegisterMirrorAndSync(ctx, userID, mirrorDir); err != nil {
		t.Fatalf("RegisterMirrorAndSync: %v", err)
	}

	if _, err := svc.UpdateMirrorSettings(ctx, userID, MirrorSettingsUpdate{
		AutoCommitEnabled: true,
		AutoPushEnabled:   true,
		AuthMode:          AuthModeLocalCredentials,
		RemoteName:        DefaultRemoteName,
		RemoteURL:         filepath.Join(t.TempDir(), "missing.git"),
		RemoteBranch:      "main",
	}); err != nil {
		t.Fatalf("UpdateMirrorSettings: %v", err)
	}

	if _, err := store.WriteEntry(ctx, userID, "/notes/demo.md", "second", "text/markdown", models.FileTreeWriteOptions{}); err != nil {
		t.Fatalf("write updated note: %v", err)
	}

	info, err := svc.SyncActiveMirror(ctx, userID)
	if err != nil {
		t.Fatalf("push failure should be best effort, got error: %v", err)
	}
	if info == nil || !info.PushAttempted || info.PushSucceeded || info.LastPushError == "" {
		t.Fatalf("expected push failure metadata without sync failure: %+v", info)
	}
	if got := gitOutput(t, "git", "-C", mirrorDir, "rev-list", "--count", "HEAD"); got != "1" {
		t.Fatalf("expected local commit despite push failure, got %q", got)
	}
}

func newAutomationTestService(t *testing.T) (*sqlitestorage.Store, *Service, uuid.UUID) {
	t.Helper()
	store, err := sqlitestorage.Open(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user, err := store.EnsureOwner(context.Background())
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}

	v, err := vault.NewVault(strings.Repeat("0", 64))
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}

	return store, New(store, v), user.ID
}

func gitOutput(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Env = gitCommandEnv(nil)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out))
}
