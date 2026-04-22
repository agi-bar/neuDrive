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
		return Run([]string{"import", "claude", "--dry-run"})
	})
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, needle := range []string{
		"Claude Code migration preview",
		"conversations:",
		"Sensitive findings: 1",
		"Vault candidates: 1",
		"Next command: neudrive import claude",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("expected %q in stdout=%q stderr=%q", needle, stdout, stderr)
		}
	}
}

func TestImportPlatformCodexDryRunPreviewHandlesLargeSessionIndex(t *testing.T) {
	home := configureIsolatedCLIEnv(t)
	writeCLITestFile(t, filepath.Join(home, ".codex", "AGENTS.md"), "# Local Rules\n\nKeep answers short.\n")
	writeCLITestFile(t, filepath.Join(home, ".codex", "memories", "team.md"), "Remember the dry-run fixture.\n")
	writeCLITestFile(t, filepath.Join(home, ".codex", "config.toml"), "model = \"gpt-5.4\"\n")
	writeCLITestFile(t, filepath.Join(home, ".agents", "skills", "sample", "SKILL.md"), "# Sample\n")
	longTitle := strings.Repeat("x", 70<<10)
	longInstructions := strings.Repeat("base-instructions ", 10<<10)
	writeCLITestFile(t, filepath.Join(home, ".codex", "session_index.jsonl"), "{\"id\":\"session-001\",\"thread_name\":\""+longTitle+"\",\"updated_at\":\"2026-04-18T10:05:00Z\"}\n")
	writeCLITestFile(t, filepath.Join(home, ".codex", "sessions", "2026", "04", "18", "session-001.jsonl"), strings.Join([]string{
		"{\"timestamp\":\"2026-04-18T10:00:00Z\",\"type\":\"session_meta\",\"payload\":{\"id\":\"session-001\",\"timestamp\":\"2026-04-18T10:00:00Z\",\"cwd\":\"/Users/demo/workspace/neudrive\",\"originator\":\"Codex Desktop\",\"cli_version\":\"0.118.0\",\"source\":\"desktop\",\"model_provider\":\"openai\",\"base_instructions\":{\"text\":\"" + longInstructions + "\"}}}",
		`{"timestamp":"2026-04-18T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Plan the dry run."}]}}`,
		`{"timestamp":"2026-04-18T10:00:02Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Use the deterministic preview path."}]}}`,
	}, "\n")+"\n")

	stdout, stderr, code := captureRunForTest(t, func() int {
		return Run([]string{"import", "codex", "--dry-run"})
	})
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, needle := range []string{
		"Codex CLI migration preview",
		"conversations:",
		"bundles:",
		"Next command: neudrive import codex",
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
