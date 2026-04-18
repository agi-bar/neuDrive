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
	if len(scan.Automations) == 0 {
		t.Fatal("expected scheduled task metadata")
	}
	if len(scan.Tools) == 0 {
		t.Fatal("expected plugin metadata")
	}
	if len(scan.Connections) == 0 {
		t.Fatal("expected connection metadata")
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
	excluded := []string{"credentials.json", "todos", "plans", "channels", "plugins/installed_plugins.json"}
	for _, finding := range scan.Inventory.SensitiveFindings {
		if strings.Contains(finding.RedactedExample, "[REDACTED]") {
			redacted = true
			break
		}
	}
	for _, file := range scan.Inventory.Files {
		for _, needle := range excluded {
			if strings.Contains(file.Path, needle) {
				t.Fatalf("expected excluded runtime file to stay out of agent archives: %s", file.Path)
			}
		}
	}
	if !redacted {
		t.Fatal("expected sensitive findings to redact settings.local.json")
	}
	styleFound := false
	for _, rule := range scan.ProfileRules {
		if strings.Contains(rule.Title, "Output style: release.md") {
			styleFound = true
			break
		}
	}
	if !styleFound {
		t.Fatal("expected output style to be promoted into profile rules")
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

func TestParseClaudeConversationFilePreservesStructuredParts(t *testing.T) {
	t.Helper()

	content := strings.Join([]string{
		`{"uuid":"msg-1","type":"user","timestamp":"2026-04-16T20:00:01Z","message":{"role":"user","content":"Please inspect the repo"}}`,
		`{"uuid":"msg-2","parent_uuid":"msg-1","type":"assistant","timestamp":"2026-04-16T20:00:02Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Need to look around first"},{"type":"text","text":"I will inspect the repo."},{"type":"tool_use","name":"bash","input":{"command":"ls -la","cwd":"/tmp/demo"}}]}}`,
		`{"uuid":"msg-3","parent_uuid":"msg-2","type":"assistant","timestamp":"2026-04-16T20:00:03Z","message":{"role":"assistant","content":[{"type":"tool_result","content":[{"type":"text","text":"total 8"}]},{"type":"text","text":"Repo inspected."},{"type":"image","file_name":"diagram.png","mime_type":"image/png"}]}}`,
	}, "\n") + "\n"

	path := filepath.Join(t.TempDir(), "demo-session.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	convo, ok, err := parseClaudeConversationFile(path)
	if err != nil {
		t.Fatalf("parseClaudeConversationFile: %v", err)
	}
	if !ok {
		t.Fatal("parseClaudeConversationFile returned ok=false")
	}
	if convo.Name != "Please inspect the repo" {
		t.Fatalf("Name = %q, want %q", convo.Name, "Please inspect the repo")
	}
	if convo.Summary != "I will inspect the repo." {
		t.Fatalf("Summary = %q, want %q", convo.Summary, "I will inspect the repo.")
	}
	if len(convo.Messages) != 3 {
		t.Fatalf("len(Messages) = %d, want 3", len(convo.Messages))
	}

	second := convo.Messages[1]
	if second.ID != "msg-2" || second.ParentID != "msg-1" {
		t.Fatalf("unexpected ids for second message: %+v", second)
	}
	if len(second.Parts) != 3 {
		t.Fatalf("len(second.Parts) = %d, want 3", len(second.Parts))
	}
	if second.Parts[0].Type != "thinking" || second.Parts[0].Text != "Need to look around first" {
		t.Fatalf("unexpected thinking part: %+v", second.Parts[0])
	}
	if second.Parts[1].Type != "text" || second.Parts[1].Text != "I will inspect the repo." {
		t.Fatalf("unexpected text part: %+v", second.Parts[1])
	}
	if second.Parts[2].Type != "tool_call" || second.Parts[2].Name != "bash" || !strings.Contains(second.Parts[2].ArgsText, `"command": "ls -la"`) {
		t.Fatalf("unexpected tool_call part: %+v", second.Parts[2])
	}
	if !strings.Contains(second.Content, "[tool_call]") {
		t.Fatalf("second.Content missing tool_call marker: %q", second.Content)
	}

	third := convo.Messages[2]
	if len(third.Parts) != 3 {
		t.Fatalf("len(third.Parts) = %d, want 3", len(third.Parts))
	}
	if third.Parts[0].Type != "tool_result" || third.Parts[0].Text != "total 8" {
		t.Fatalf("unexpected tool_result part: %+v", third.Parts[0])
	}
	if third.Parts[2].Type != "attachment" || third.Parts[2].FileName != "diagram.png" || third.Parts[2].MimeType != "image/png" {
		t.Fatalf("unexpected attachment part: %+v", third.Parts[2])
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
	writeFixtureFile(t, filepath.Join(home, ".claude", ".credentials.json"), "{\n  \"refresh_token\": \"secret-refresh\"\n}\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "skills", "release-helper", "SKILL.md"), "# Release Helper\n\nUse this skill to package releases.\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "skills", "release-helper", "scripts", "ship.sh"), "#!/bin/sh\necho release\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "scheduled-tasks", "daily.toml"), "name = \"Daily release\"\nrrule = \"FREQ=DAILY;BYHOUR=9;BYMINUTE=0\"\nstatus = \"ACTIVE\"\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "output-styles", "release.md"), "Be crisp and list risks first.\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "hooks", "preflight.sh"), "#!/bin/sh\necho preflight\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), "[{\"name\":\"release-helper\",\"version\":\"1.0.0\",\"description\":\"Release support\"}]\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "todos", "todo.md"), "skip\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "plans", "plan.md"), "skip\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "channels", "main.md"), "skip\n")
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
