package localgitsync

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScrubGitEnvRemovesGitScopedVariables(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"HOME=/tmp/home",
		"GIT_DIR=/tmp/repo/.git",
		"GIT_WORK_TREE=/tmp/repo",
		"GIT_AUTHOR_NAME=AgentHub",
	}

	got := scrubGitEnv(env)
	joined := strings.Join(got, "\n")

	if strings.Contains(joined, "GIT_DIR=") || strings.Contains(joined, "GIT_WORK_TREE=") || strings.Contains(joined, "GIT_AUTHOR_NAME=") {
		t.Fatalf("expected git-scoped variables removed, got %v", got)
	}
	if !strings.Contains(joined, "PATH=/usr/bin") || !strings.Contains(joined, "HOME=/tmp/home") {
		t.Fatalf("expected non-git variables preserved, got %v", got)
	}
}

func TestEnsureGitRepoIgnoresInheritedGitEnv(t *testing.T) {
	ctx := context.Background()
	outerRoot := filepath.Join(t.TempDir(), "outer")
	if err := os.MkdirAll(outerRoot, 0o755); err != nil {
		t.Fatalf("mkdir outer root: %v", err)
	}
	if output, err := exec.Command("git", "-C", outerRoot, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init outer repo: %v: %s", err, strings.TrimSpace(string(output)))
	}

	outerConfig := filepath.Join(outerRoot, ".git", "config")
	before, err := os.ReadFile(outerConfig)
	if err != nil {
		t.Fatalf("read outer config before: %v", err)
	}

	t.Setenv("GIT_DIR", filepath.Join(outerRoot, ".git"))
	t.Setenv("GIT_WORK_TREE", outerRoot)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(outerRoot, ".git", "index"))
	t.Setenv("GIT_AUTHOR_NAME", "AgentHub Test")

	mirrorRoot := filepath.Join(t.TempDir(), "mirror")
	if err := os.MkdirAll(mirrorRoot, 0o755); err != nil {
		t.Fatalf("mkdir mirror root: %v", err)
	}
	if _, err := ensureGitRepo(ctx, mirrorRoot, nil); err != nil {
		t.Fatalf("ensureGitRepo: %v", err)
	}

	if _, err := os.Stat(filepath.Join(mirrorRoot, ".git")); err != nil {
		t.Fatalf("expected mirror repo initialized: %v", err)
	}

	after, err := os.ReadFile(outerConfig)
	if err != nil {
		t.Fatalf("read outer config after: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("expected inherited git env not to mutate outer config\nbefore:\n%s\nafter:\n%s", string(before), string(after))
	}
}

func TestResolveMirrorRootExpandsUser(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := resolveMirrorRoot("~/mirror")
	if err != nil {
		t.Fatalf("resolveMirrorRoot: %v", err)
	}
	want := filepath.Join(home, "mirror")
	if got != want {
		t.Fatalf("resolveMirrorRoot = %q, want %q", got, want)
	}
}
