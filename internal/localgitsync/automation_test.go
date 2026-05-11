package localgitsync

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agi-bar/neuDrive/internal/models"
	sqlitestorage "github.com/agi-bar/neuDrive/internal/storage/sqlite"
	"github.com/agi-bar/neuDrive/internal/vault"
	"github.com/google/uuid"
)

func TestSyncActiveMirrorAutoCommitCreatesOneCommitPerDirtySync(t *testing.T) {
	ctx := context.Background()
	store, svc, userID := newAutomationTestService(t)

	if _, err := store.WriteEntry(ctx, userID, "/notes/demo.md", "first", "text/markdown", models.FileTreeWriteOptions{}); err != nil {
		t.Fatalf("write initial note: %v", err)
	}
	if _, err := store.WriteEntry(ctx, userID, "/skills/demo/SKILL.md", "# Demo\n", "text/markdown", models.FileTreeWriteOptions{}); err != nil {
		t.Fatalf("write skill document: %v", err)
	}
	if _, err := store.WriteEntry(ctx, userID, "/skills/demo/references/notes.md", "reference", "text/markdown", models.FileTreeWriteOptions{}); err != nil {
		t.Fatalf("write skill reference: %v", err)
	}

	mirrorDir := filepath.Join(t.TempDir(), "mirror")
	if _, err := svc.RegisterMirrorAndSync(ctx, userID, mirrorDir); err != nil {
		t.Fatalf("RegisterMirrorAndSync: %v", err)
	}
	if got := readMirrorFile(t, mirrorDir, "skills/demo/SKILL.md"); got != "# Demo\n" {
		t.Fatalf("mirrored skill document = %q", got)
	}
	if got := readMirrorFile(t, mirrorDir, "skills/demo/references/notes.md"); got != "reference" {
		t.Fatalf("mirrored skill reference = %q", got)
	}
	for _, path := range []string{
		"identity/profile.json",
		"connections/connections.json",
		"vault/scopes.json",
		"_neudrive/projects.json",
		"_neudrive/metadata.json",
	} {
		if _, err := os.Stat(filepath.Join(mirrorDir, filepath.FromSlash(path))); !os.IsNotExist(err) {
			t.Fatalf("mirror wrote internal sidecar %s", path)
		}
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

	info, err := svc.SyncActiveMirror(ctx, userID, false)
	if err != nil {
		t.Fatalf("SyncActiveMirror dirty: %v", err)
	}
	if info == nil || !info.CommitCreated || info.LastCommitHash == "" {
		t.Fatalf("expected commit metadata in sync info: %+v", info)
	}

	if got := gitOutput(t, "git", "-C", mirrorDir, "rev-list", "--count", "HEAD"); got != "1" {
		t.Fatalf("commit count after dirty sync = %q, want 1", got)
	}

	info, err = svc.SyncActiveMirror(ctx, userID, false)
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

	info, err := svc.SyncActiveMirror(ctx, userID, false)
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

func TestUpdateMirrorSettingsLocalCredentialsRejectsGitHubHTTPSRemote(t *testing.T) {
	ctx := context.Background()
	_, svc, userID := newAutomationTestService(t)

	if _, err := svc.RegisterMirrorAndSync(ctx, userID, filepath.Join(t.TempDir(), "mirror")); err != nil {
		t.Fatalf("RegisterMirrorAndSync: %v", err)
	}
	_, err := svc.UpdateMirrorSettings(ctx, userID, MirrorSettingsUpdate{
		AutoCommitEnabled: true,
		AutoPushEnabled:   true,
		AuthMode:          AuthModeLocalCredentials,
		RemoteName:        DefaultRemoteName,
		RemoteURL:         "https://github.com/acme/demo.git",
		RemoteBranch:      "main",
	})
	if err == nil {
		t.Fatal("expected GitHub HTTPS remote to be rejected for local credentials")
	}
	if got := err.Error(); !strings.Contains(got, "git@github.com:owner/repo.git") || strings.Contains(got, "GitHub token") {
		t.Fatalf("unexpected error: %v", err)
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

	info, err := svc.SyncActiveMirror(ctx, userID, false)
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

func TestSyncActiveMirrorBlocksAndForceOverwritesRemoteChanges(t *testing.T) {
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
		t.Fatalf("write second note: %v", err)
	}
	if info, err := svc.SyncActiveMirror(ctx, userID, false); err != nil || info == nil || !info.PushSucceeded {
		t.Fatalf("initial push failed: info=%+v err=%v", info, err)
	}

	remoteWorktree := filepath.Join(t.TempDir(), "remote-worktree")
	gitOutput(t, "git", "clone", "--branch", "main", bareRemote, remoteWorktree)
	gitOutput(t, "git", "-C", remoteWorktree, "config", "user.email", "remote@example.com")
	gitOutput(t, "git", "-C", remoteWorktree, "config", "user.name", "Remote Editor")
	if err := os.WriteFile(filepath.Join(remoteWorktree, "remote-only.txt"), []byte("remote change\n"), 0o644); err != nil {
		t.Fatalf("write remote-only file: %v", err)
	}
	gitOutput(t, "git", "-C", remoteWorktree, "add", "remote-only.txt")
	gitOutput(t, "git", "-C", remoteWorktree, "commit", "-m", "remote change")
	gitOutput(t, "git", "-C", remoteWorktree, "push", "origin", "HEAD:main")

	if _, err := store.WriteEntry(ctx, userID, "/notes/demo.md", "third", "text/markdown", models.FileTreeWriteOptions{}); err != nil {
		t.Fatalf("write third note: %v", err)
	}
	info, err := svc.SyncActiveMirror(ctx, userID, false)
	if err != nil {
		t.Fatalf("conflict sync should be best effort, got error: %v", err)
	}
	if info == nil || !info.RemoteConflict || info.PushSucceeded || info.LastPushError == "" {
		t.Fatalf("expected remote conflict without push success: %+v", info)
	}
	if tree := gitOutput(t, "git", "--git-dir", bareRemote, "ls-tree", "-r", "--name-only", "main"); !strings.Contains(tree, "remote-only.txt") {
		t.Fatalf("remote should still contain manual change before overwrite, tree=%q", tree)
	}

	info, err = svc.SyncActiveMirror(ctx, userID, true)
	if err != nil {
		t.Fatalf("force overwrite sync: %v", err)
	}
	if info == nil || info.RemoteConflict || !info.PushSucceeded || info.LastPushError != "" {
		t.Fatalf("expected force-with-lease overwrite to push successfully: %+v", info)
	}
	tree := gitOutput(t, "git", "--git-dir", bareRemote, "ls-tree", "-r", "--name-only", "main")
	if strings.Contains(tree, "remote-only.txt") {
		t.Fatalf("remote-only file should be removed after overwrite, tree=%q", tree)
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

func readMirrorFile(t *testing.T, rootPath, publicPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(rootPath, filepath.FromSlash(publicPath)))
	if err != nil {
		t.Fatalf("read mirrored file %s: %v", publicPath, err)
	}
	return string(data)
}
