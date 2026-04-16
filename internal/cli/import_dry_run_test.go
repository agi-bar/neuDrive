package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportPlatformClaudeDryRunPreview(t *testing.T) {
	home := configureIsolatedCLIEnv(t)
	writeCLITestFile(t, filepath.Join(home, ".claude", "CLAUDE.md"), "# Global Rules\n\nBe explicit.\n")
	writeCLITestFile(t, filepath.Join(home, ".claude", "agent-memory", "team.md"), "Remember the release checklist.\n")
	writeCLITestFile(t, filepath.Join(home, ".claude", "settings.local.json"), "{\n  \"api_key\": \"sk-test-secret\"\n}\n")
	writeCLITestFile(t, filepath.Join(home, ".claude", "skills", "release-helper", "SKILL.md"), "# Release Helper\n")
	writeCLITestFile(t, filepath.Join(home, ".claude", "projects", "demo-session", "session.jsonl"), strings.Join([]string{
		`{"type":"user","timestamp":"2026-04-15T10:00:00Z","message":{"role":"user","content":"Plan the migration."}}`,
		`{"type":"assistant","timestamp":"2026-04-15T10:01:00Z","message":{"role":"assistant","content":"Run a dry run first."}}`,
	}, "\n")+"\n")
	writeCLITestFile(t, filepath.Join(home, ".claude.json"), "{\n  \"projects\": {}\n}\n")
	installClaudeDryRunShim(t)

	stdout, stderr, code := captureRunForTest(t, func() int {
		return Run([]string{"import", "platform", "claude", "--dry-run", "--mode", "agent"})
	})
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, needle := range []string{
		"Claude Code migration preview (mode=agent)",
		"conversations:",
		"Sensitive findings: 1",
		"Vault candidates: 1",
		"Next command: neudrive import platform claude --mode agent",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("expected %q in stdout=%q stderr=%q", needle, stdout, stderr)
		}
	}
}

func installClaudeDryRunShim(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	script := "#!/bin/sh\nset -eu\nif [ \"${1:-}\" = \"-p\" ]; then\n  printf '%s\\n' '{\"platform\":\"claude-code\",\"command\":\"export\",\"profile_rules\":[],\"memory_items\":[],\"projects\":[],\"automations\":[],\"tools\":[],\"connections\":[],\"archives\":[],\"unsupported\":[],\"notes\":[\"shimmed agent export\"]}'\n  exit 0\nfi\nexit 0\n"
	path := filepath.Join(binDir, "claude")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeCLITestFile(t *testing.T, target, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", target, err)
	}
}
