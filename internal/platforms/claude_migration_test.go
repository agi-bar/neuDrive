package platforms

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agi-bar/neudrive/internal/runtimecfg"
)

func TestScanLocalClaudeMigrationBuildsTypedInventory(t *testing.T) {
	home := createClaudeMigrationFixtureTree(t)
	t.Setenv("HOME", home)

	scan, err := scanLocalClaudeMigration()
	if err != nil {
		t.Fatalf("scanLocalClaudeMigration: %v", err)
	}
	if len(scan.ProfileRules) < 2 {
		t.Fatalf("expected global Claude rules, got %+v", scan.ProfileRules)
	}
	if len(scan.MemoryItems) < 2 {
		t.Fatalf("expected agent and project memory items, got %+v", scan.MemoryItems)
	}
	if len(scan.Inventory.Bundles) == 0 {
		t.Fatal("expected at least one Claude bundle")
	}
	if len(scan.Inventory.Conversations) < 2 {
		t.Fatalf("expected project and subagent conversations, got %+v", scan.Inventory.Conversations)
	}
	if len(scan.Inventory.Projects) == 0 {
		t.Fatal("expected Claude project snapshots")
	}
	project := scan.Inventory.Projects[0]
	if strings.TrimSpace(project.Context) == "" {
		t.Fatalf("expected project context, got %+v", project)
	}
	if len(project.Files) == 0 {
		t.Fatalf("expected imported project files, got %+v", project)
	}
	if len(scan.Inventory.SensitiveFindings) == 0 {
		t.Fatal("expected sensitive findings from settings.local.json")
	}
	if len(scan.Inventory.VaultCandidates) == 0 {
		t.Fatal("expected vault candidates from settings.local.json")
	}
	redacted := false
	for _, file := range scan.Inventory.Files {
		if strings.Contains(file.Path, "settings.local.json") && strings.Contains(file.Content, "[REDACTED]") {
			redacted = true
			break
		}
	}
	if !redacted {
		t.Fatal("expected archived settings.local.json to be redacted")
	}
}

func TestPreviewImportClaudeIncludesMigrationCategories(t *testing.T) {
	home := createClaudeMigrationFixtureTree(t)
	t.Setenv("HOME", home)

	preview, err := PreviewImport(context.Background(), &runtimecfg.CLIConfig{}, "claude", "files")
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if preview.DisplayName != "Claude Code" {
		t.Fatalf("unexpected display name: %+v", preview)
	}
	if len(preview.Categories) == 0 {
		t.Fatal("expected preview categories")
	}
	names := map[string]bool{}
	for _, category := range preview.Categories {
		names[category.Name] = true
	}
	for _, required := range []string{"raw_platform_snapshot", "profile_rules", "memory_items", "claude_projects", "bundles", "conversations", "structured_archives"} {
		if !names[required] {
			t.Fatalf("missing preview category %q in %+v", required, preview.Categories)
		}
	}
	if len(preview.SensitiveFindings) == 0 {
		t.Fatal("expected preview sensitive findings")
	}
	if len(preview.VaultCandidates) == 0 {
		t.Fatal("expected preview vault candidates")
	}
	if preview.NextCommand != "neudrive import platform claude --mode files" {
		t.Fatalf("unexpected next command: %q", preview.NextCommand)
	}
}

func TestParseClaudeConversationFileHandlesLongJSONLLines(t *testing.T) {
	root := t.TempDir()
	longMessage := strings.Repeat("A", 128<<10)
	sessionPath := filepath.Join(root, "session.jsonl")
	writeFixtureFile(t, sessionPath, fmt.Sprintf("{\"type\":\"user\",\"timestamp\":\"2026-04-15T10:00:00Z\",\"message\":{\"role\":\"user\",\"content\":%q}}\n", longMessage))

	convo, ok, err := parseClaudeConversationFile(sessionPath)
	if err != nil {
		t.Fatalf("parseClaudeConversationFile: %v", err)
	}
	if !ok {
		t.Fatal("expected conversation to be parsed")
	}
	if len(convo.Messages) != 1 {
		t.Fatalf("expected 1 message, got %+v", convo.Messages)
	}
	if convo.Messages[0].Content != longMessage {
		t.Fatalf("expected long message to be preserved, got len=%d want=%d", len(convo.Messages[0].Content), len(longMessage))
	}
}

func createClaudeMigrationFixtureTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(home, "workspace", "claude-demo")
	writeFixtureFile(t, filepath.Join(home, ".claude", "CLAUDE.md"), "# Global Rules\n\nBe explicit about risks.\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "CLAUDE.local.md"), "# Local Rules\n\nFavor terse updates.\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "agent-memory", "team.md"), "Remember the release checklist.\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "settings.local.json"), "{\n  \"api_key\": \"sk-test-secret\",\n  \"theme\": \"compact\"\n}\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "skills", "release-helper", "SKILL.md"), "# Release Helper\n\nUse this skill to package releases.\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "skills", "release-helper", "scripts", "ship.sh"), "#!/bin/sh\necho release\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "projects", "demo-session", "memory", "remember.md"), "Document the migration choices.\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "projects", "demo-session", "session.jsonl"), strings.Join([]string{
		`{"type":"user","timestamp":"2026-04-15T10:00:00Z","message":{"role":"user","content":"Plan the release migration."}}`,
		`{"type":"assistant","timestamp":"2026-04-15T10:01:00Z","message":{"role":"assistant","content":"Start with a dry run and redact secrets."}}`,
	}, "\n")+"\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "projects", "demo-session", "subagents", "research.jsonl"), strings.Join([]string{
		`{"type":"user","timestamp":"2026-04-15T10:02:00Z","message":{"role":"user","content":"Research risks."}}`,
		`{"type":"assistant","timestamp":"2026-04-15T10:03:00Z","message":{"role":"assistant","content":"Sensitive settings must be redacted."}}`,
	}, "\n")+"\n")
	writeFixtureFile(t, filepath.Join(home, ".claude.json"), "{\n  \"projects\": {\n    \"~/workspace/claude-demo\": {\n      \"name\": \"Claude Demo\"\n    }\n  }\n}\n")
	writeFixtureFile(t, filepath.Join(workspace, "CLAUDE.md"), "# Workspace Context\n\nShip from the release branch.\n")
	writeFixtureFile(t, filepath.Join(workspace, "docs", "spec.md"), "# Spec\n\nThis workspace documents the migration.\n")
	writeFixtureFile(t, filepath.Join(workspace, ".mcp.json"), "{\n  \"mcpServers\": {}\n}\n")
	writeFixtureFile(t, filepath.Join(workspace, ".claude", "settings.local.json"), "{\n  \"authorization\": \"Bearer hidden-demo-token\"\n}\n")
	return home
}

func writeFixtureFile(t *testing.T, target, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", target, err)
	}
}
