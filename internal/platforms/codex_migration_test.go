package platforms

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agi-bar/neudrive/internal/runtimecfg"
)

func TestScanLocalCodexMigrationBuildsStructuredInventory(t *testing.T) {
	home := createCodexMigrationFixtureTree(t)
	t.Setenv("HOME", home)

	scan, err := scanLocalCodexMigration()
	if err != nil {
		t.Fatalf("scanLocalCodexMigration: %v", err)
	}
	if len(scan.ProfileRules) < 2 {
		t.Fatalf("expected profile rules, got %+v", scan.ProfileRules)
	}
	if len(scan.MemoryItems) == 0 {
		t.Fatal("expected memory items")
	}
	if len(scan.Projects) == 0 {
		t.Fatal("expected project summaries")
	}
	if len(scan.Inventory.Bundles) < 2 {
		t.Fatalf("expected imported Codex skill bundles, got %+v", scan.Inventory.Bundles)
	}
	if len(scan.Inventory.Conversations) != 1 {
		t.Fatalf("expected one Codex conversation, got %+v", scan.Inventory.Conversations)
	}
	if len(scan.Tools) == 0 {
		t.Fatal("expected plugin metadata records")
	}
	if len(scan.Automations) == 0 {
		t.Fatal("expected automation metadata records")
	}
	for _, record := range scan.Automations {
		if record.Metadata["schedule"] != "FREQ=DAILY;BYHOUR=9;BYMINUTE=0" {
			t.Fatalf("expected parsed automation schedule, got %+v", record)
		}
	}
	for _, record := range scan.Archives {
		if record.Name == "runtime-state" {
			t.Fatalf("expected runtime-state archive to be excluded, got %+v", scan.Archives)
		}
	}
	convo := scan.Inventory.Conversations[0]
	if convo.ProjectName != "neudrive" {
		t.Fatalf("expected project name derived from workspace, got %+v", convo)
	}
	if len(convo.Messages) < 4 {
		t.Fatalf("expected conversation messages, got %+v", convo.Messages)
	}
	if convo.Messages[1].Parts[0].Type != "thinking" {
		t.Fatalf("expected reasoning part, got %+v", convo.Messages[1])
	}
	if convo.Messages[2].Parts[0].Type != "tool_call" {
		t.Fatalf("expected tool call part, got %+v", convo.Messages[2])
	}
	if convo.Messages[3].Parts[0].Type != "tool_result" {
		t.Fatalf("expected tool result part, got %+v", convo.Messages[3])
	}
}

func TestPreviewImportCodexIncludesBundlesAndConversations(t *testing.T) {
	home := createCodexMigrationFixtureTree(t)
	t.Setenv("HOME", home)

	preview, err := PreviewImport(context.Background(), &runtimecfg.CLIConfig{}, "codex", "agent")
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	names := map[string]bool{}
	for _, category := range preview.Categories {
		names[category.Name] = true
	}
	for _, required := range []string{"profile_rules", "memory_items", "projects", "bundles", "conversations", "agent_artifacts"} {
		if !names[required] {
			t.Fatalf("missing preview category %q in %+v", required, preview.Categories)
		}
	}
}

func TestValueBasedDiscoverSourcesExcludeRuntimeNoise(t *testing.T) {
	home := createCodexMigrationFixtureTree(t)
	createClaudeMigrationFixtureTreeWithRuntimeNoise(t, home)
	t.Setenv("HOME", home)

	codex, err := Resolve("codex")
	if err != nil {
		t.Fatalf("Resolve(codex): %v", err)
	}
	claude, err := Resolve("claude")
	if err != nil {
		t.Fatalf("Resolve(claude): %v", err)
	}

	assertMissingSource(t, codex.DiscoverSources(),
		filepath.Join(home, ".codex", "logs_2.sqlite"),
		filepath.Join(home, ".codex", "state_5.sqlite"),
		filepath.Join(home, ".codex", "shell_snapshots"),
		filepath.Join(home, ".codex", "worktrees"),
		filepath.Join(home, ".codex", ".codex-global-state.json"),
	)
	assertMissingSource(t, claude.DiscoverSources(),
		filepath.Join(home, ".claude", "todos"),
		filepath.Join(home, ".claude", "plans"),
		filepath.Join(home, ".claude", "channels"),
		filepath.Join(home, ".claude", ".credentials.json"),
	)
}

func createCodexMigrationFixtureTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	writeFixtureFile(t, filepath.Join(home, ".codex", "AGENTS.md"), "# Global\n\nKeep answers short.\n")
	writeFixtureFile(t, filepath.Join(home, ".codex", "rules", "default.rules"), "prefix_rule(pattern=[\"go\", \"test\", \"./...\"], decision=\"allow\")\n")
	writeFixtureFile(t, filepath.Join(home, ".codex", "memories", "team.md"), "Remember the migration fixtures.\n")
	writeFixtureFile(t, filepath.Join(home, ".codex", "config.toml"), strings.Join([]string{
		`model = "gpt-5.4"`,
		`approval_policy = "never"`,
		``,
		`[projects."/Users/demo/workspace/neudrive"]`,
		`trust_level = "trusted"`,
		``,
		`[mcp_servers.neudrive-local]`,
		`command = "/usr/local/bin/neu"`,
		`args = ["mcp", "stdio", "--token-env", "NEUDRIVE_TOKEN"]`,
		``,
		`[mcp_servers.neudrive-local.env]`,
		`NEUDRIVE_TOKEN = "ndt_test_secret"`,
	}, "\n")+"\n")
	writeFixtureFile(t, filepath.Join(home, ".codex", "auth.json"), "{\n  \"auth_mode\": \"chatgpt\",\n  \"tokens\": {\n    \"access_token\": \"secret-access\"\n  }\n}\n")
	writeFixtureFile(t, filepath.Join(home, ".codex", "session_index.jsonl"), `{"id":"session-001","thread_name":"Inspect import plan","updated_at":"2026-04-16T10:05:00Z"}`+"\n")
	writeFixtureFile(t, filepath.Join(home, ".codex", "sessions", "2026", "04", "16", "session-001.jsonl"), strings.Join([]string{
		`{"timestamp":"2026-04-16T10:00:00Z","type":"session_meta","payload":{"id":"session-001","timestamp":"2026-04-16T10:00:00Z","cwd":"/Users/demo/workspace/neudrive","originator":"Codex Desktop","cli_version":"0.118.0","source":"desktop","model_provider":"openai"}}`,
		`{"timestamp":"2026-04-16T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Plan the import migration."}]}}`,
		`{"timestamp":"2026-04-16T10:00:02Z","type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"Reviewing migration structure"}]}}`,
		`{"timestamp":"2026-04-16T10:00:03Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"rg --files\"}","call_id":"call-1"}}`,
		`{"timestamp":"2026-04-16T10:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"internal/platforms/codex_migration.go"}}`,
		`{"timestamp":"2026-04-16T10:00:05Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Start with a deterministic local scan."}]}}`,
	}, "\n")+"\n")
	writeFixtureFile(t, filepath.Join(home, ".codex", "automations", "auto-1", "automation.toml"), strings.Join([]string{
		`name = "Daily import review"`,
		`kind = "heartbeat"`,
		`status = "ACTIVE"`,
		`rrule = "FREQ=DAILY;BYHOUR=9;BYMINUTE=0"`,
	}, "\n")+"\n")
	writeFixtureFile(t, filepath.Join(home, ".agents", "skills", "sample", "SKILL.md"), "# Sample\n")
	writeFixtureFile(t, filepath.Join(home, ".codex", "skills", "builtin", "SKILL.md"), "# Builtin\n")
	writeFixtureFile(t, filepath.Join(home, ".codex", ".tmp", "plugins", "plugins", "sample-plugin", ".codex-plugin", "plugin.json"), "{\n  \"name\": \"sample-plugin\",\n  \"version\": \"1.0.0\",\n  \"description\": \"Sample plugin\",\n  \"category\": \"dev\",\n  \"skills\": [\"sample\"],\n  \"mcpServers\": {\"sample\": {}},\n  \"capabilities\": [\"search\"]\n}\n")
	writeFixtureFile(t, filepath.Join(home, ".codex", "logs_2.sqlite"), "ignore")
	writeFixtureFile(t, filepath.Join(home, ".codex", "state_5.sqlite"), "ignore")
	writeFixtureFile(t, filepath.Join(home, ".codex", ".codex-global-state.json"), "{}\n")
	writeFixtureFile(t, filepath.Join(home, ".codex", "shell_snapshots", "shell.sh"), "echo hi\n")
	writeFixtureFile(t, filepath.Join(home, ".codex", "worktrees", "state.txt"), "ignore\n")
	writeFixtureFile(t, filepath.Join(home, ".codex", "history.jsonl"), "{}\n")
	return home
}

func createClaudeMigrationFixtureTreeWithRuntimeNoise(t *testing.T, home string) {
	t.Helper()
	writeFixtureFile(t, filepath.Join(home, ".claude", "todos", "todo.md"), "skip\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "plans", "plan.md"), "skip\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", "channels", "main.md"), "skip\n")
	writeFixtureFile(t, filepath.Join(home, ".claude", ".credentials.json"), "{\n  \"api_key\": \"secret\"\n}\n")
}

func assertMissingSource(t *testing.T, sources []Source, targets ...string) {
	t.Helper()
	for _, target := range targets {
		for _, source := range sources {
			if source.Path == target {
				t.Fatalf("unexpected source discovered: %s", target)
			}
		}
	}
}
