package platforms

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type coverageMatrixEntry struct {
	id       string
	target   string
	source   []string
	importer string
	verify   func(t *testing.T, claude *claudeLocalScanResult, codex *codexLocalScanResult)
}

func TestPlatformCoverageMatrixIsDocumentedAndBackedByFixtures(t *testing.T) {
	claudeHome := createClaudeMigrationFixtureTree(t)
	t.Setenv("HOME", claudeHome)
	claudeScan, err := scanLocalClaudeMigration()
	if err != nil {
		t.Fatalf("scanLocalClaudeMigration: %v", err)
	}

	codexHome := createCodexMigrationFixtureTree(t)
	t.Setenv("HOME", codexHome)
	codexScan, err := scanLocalCodexMigration()
	if err != nil {
		t.Fatalf("scanLocalCodexMigration: %v", err)
	}

	docPath := filepath.Join(platformRepoRoot(t), "docs", "platform-coverage-matrix.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", docPath, err)
	}
	doc := string(docBytes)

	entries := []coverageMatrixEntry{
		{id: "claude.profile-rules", target: "first-class", source: []string{"~/.claude/CLAUDE.md", "~/.claude/output-styles/"}, verify: func(t *testing.T, claude *claudeLocalScanResult, _ *codexLocalScanResult) {
			if len(claude.ProfileRules) == 0 {
				t.Fatal("expected Claude profile rules")
			}
		}},
		{id: "claude.memory", target: "first-class", source: []string{"~/.claude/agent-memory/"}, verify: func(t *testing.T, claude *claudeLocalScanResult, _ *codexLocalScanResult) {
			if len(claude.MemoryItems) == 0 {
				t.Fatal("expected Claude memory items")
			}
		}},
		{id: "claude.projects", target: "first-class", source: []string{"~/.claude.json"}, verify: func(t *testing.T, claude *claudeLocalScanResult, _ *codexLocalScanResult) {
			if len(claude.Inventory.Projects) == 0 {
				t.Fatal("expected Claude projects")
			}
		}},
		{id: "claude.bundles", target: "first-class", source: []string{"~/.claude/skills/"}, verify: func(t *testing.T, claude *claudeLocalScanResult, _ *codexLocalScanResult) {
			if len(claude.Inventory.Bundles) == 0 {
				t.Fatal("expected Claude bundles")
			}
		}},
		{id: "claude.conversations", target: "conversation archive", source: []string{"~/.claude/projects/**/*.jsonl"}, verify: func(t *testing.T, claude *claudeLocalScanResult, _ *codexLocalScanResult) {
			if len(claude.Inventory.Conversations) == 0 {
				t.Fatal("expected Claude conversations")
			}
		}},
		{id: "claude.automations", target: "structured metadata", source: []string{"~/.claude/scheduled-tasks/"}, verify: func(t *testing.T, claude *claudeLocalScanResult, _ *codexLocalScanResult) {
			if len(claude.Automations) == 0 {
				t.Fatal("expected Claude automations")
			}
		}},
		{id: "claude.tools", target: "structured metadata", source: []string{"~/.claude/plugins/"}, verify: func(t *testing.T, claude *claudeLocalScanResult, _ *codexLocalScanResult) {
			if len(claude.Tools) == 0 {
				t.Fatal("expected Claude tools")
			}
		}},
		{id: "claude.connections", target: "structured metadata", source: []string{"~/.claude.json", "~/.claude/settings.local.json"}, verify: func(t *testing.T, claude *claudeLocalScanResult, _ *codexLocalScanResult) {
			if len(claude.Connections) == 0 {
				t.Fatal("expected Claude connections")
			}
		}},
		{id: "claude.hooks", target: "exact snapshot", source: []string{"~/.claude/hooks/"}, verify: func(t *testing.T, claude *claudeLocalScanResult, _ *codexLocalScanResult) {
			found := false
			for _, file := range claude.Inventory.Files {
				if strings.Contains(file.Path, "agent/runtime/hooks") {
					found = true
					break
				}
			}
			if !found {
				t.Fatal("expected Claude hooks exact snapshot")
			}
		}},
		{id: "claude.official-export-zip", target: "dedicated importer", importer: "/api/import/claude-data"},
		{id: "claude.official-memory-export", target: "dedicated importer", importer: "/api/import/claude-memory"},
		{id: "claude.excluded.todos", target: "excluded", source: []string{"~/.claude/todos/"}, verify: func(t *testing.T, claude *claudeLocalScanResult, _ *codexLocalScanResult) {
			assertClaudeArchiveMissing(t, claude, "todos")
		}},
		{id: "claude.excluded.plans", target: "excluded", source: []string{"~/.claude/plans/"}, verify: func(t *testing.T, claude *claudeLocalScanResult, _ *codexLocalScanResult) {
			assertClaudeArchiveMissing(t, claude, "plans")
		}},
		{id: "claude.excluded.channels", target: "excluded", source: []string{"~/.claude/channels/"}, verify: func(t *testing.T, claude *claudeLocalScanResult, _ *codexLocalScanResult) {
			assertClaudeArchiveMissing(t, claude, "channels")
		}},
		{id: "claude.excluded.credentials-file", target: "excluded", source: []string{"~/.claude/.credentials.json"}, verify: func(t *testing.T, claude *claudeLocalScanResult, _ *codexLocalScanResult) {
			assertClaudeArchiveMissing(t, claude, "credentials.json")
		}},
		{id: "codex.profile-rules", target: "first-class", source: []string{"~/.codex/AGENTS.md", "~/.codex/rules/"}, verify: func(t *testing.T, _ *claudeLocalScanResult, codex *codexLocalScanResult) {
			if len(codex.ProfileRules) == 0 {
				t.Fatal("expected Codex profile rules")
			}
		}},
		{id: "codex.memory", target: "first-class", source: []string{"~/.codex/memories/"}, verify: func(t *testing.T, _ *claudeLocalScanResult, codex *codexLocalScanResult) {
			if len(codex.MemoryItems) == 0 {
				t.Fatal("expected Codex memory items")
			}
		}},
		{id: "codex.projects", target: "first-class", source: []string{"~/.codex/sessions/"}, verify: func(t *testing.T, _ *claudeLocalScanResult, codex *codexLocalScanResult) {
			if len(codex.Projects) == 0 {
				t.Fatal("expected Codex project summaries")
			}
		}},
		{id: "codex.conversations", target: "conversation archive", source: []string{"~/.codex/sessions/**/*.jsonl"}, verify: func(t *testing.T, _ *claudeLocalScanResult, codex *codexLocalScanResult) {
			if len(codex.Inventory.Conversations) == 0 {
				t.Fatal("expected Codex conversations")
			}
		}},
		{id: "codex.connections", target: "structured metadata", source: []string{"~/.codex/config.toml", "~/.codex/auth.json"}, verify: func(t *testing.T, _ *claudeLocalScanResult, codex *codexLocalScanResult) {
			if len(codex.Connections) == 0 {
				t.Fatal("expected Codex connections")
			}
		}},
		{id: "codex.skills.user", target: "first-class", source: []string{"~/.agents/skills/"}, verify: func(t *testing.T, _ *claudeLocalScanResult, codex *codexLocalScanResult) {
			if len(codex.Inventory.Bundles) == 0 {
				t.Fatal("expected Codex user skill bundles")
			}
		}},
		{id: "codex.skills.bundled", target: "first-class", source: []string{"~/.codex/skills/"}, verify: func(t *testing.T, _ *claudeLocalScanResult, codex *codexLocalScanResult) {
			found := false
			for _, bundle := range codex.Inventory.Bundles {
				if bundle.Kind == "bundled-skill" {
					found = true
					break
				}
			}
			if !found {
				t.Fatal("expected bundled Codex skills")
			}
		}},
		{id: "codex.automations", target: "structured metadata", source: []string{"~/.codex/automations/"}, verify: func(t *testing.T, _ *claudeLocalScanResult, codex *codexLocalScanResult) {
			if len(codex.Automations) == 0 {
				t.Fatal("expected Codex automations")
			}
		}},
		{id: "codex.tools.plugins", target: "structured metadata", source: []string{"~/.codex/.tmp/plugins/plugins/*/.codex-plugin/plugin.json"}, verify: func(t *testing.T, _ *claudeLocalScanResult, codex *codexLocalScanResult) {
			if len(codex.Tools) == 0 {
				t.Fatal("expected Codex plugin metadata")
			}
		}},
		{id: "codex.history", target: "exact snapshot", source: []string{"~/.codex/history.jsonl"}},
		{id: "codex.excluded.logs-db", target: "excluded", source: []string{"~/.codex/logs_2.sqlite"}},
		{id: "codex.excluded.state-db", target: "excluded", source: []string{"~/.codex/state_5.sqlite"}},
		{id: "codex.excluded.shell-snapshots", target: "excluded", source: []string{"~/.codex/shell_snapshots/"}},
		{id: "codex.excluded.worktrees", target: "excluded", source: []string{"~/.codex/worktrees/"}},
		{id: "codex.excluded.global-state", target: "excluded", source: []string{"~/.codex/.codex-global-state.json"}},
		{id: "codex.excluded.plugin-cache-assets", target: "excluded", source: []string{"~/.codex/.tmp/plugins/"}, verify: func(t *testing.T, _ *claudeLocalScanResult, codex *codexLocalScanResult) {
			for _, record := range codex.Tools {
				for _, source := range record.SourcePaths {
					if !strings.HasSuffix(source, "plugin.json") {
						t.Fatalf("expected Codex plugin import to keep manifest metadata only, got %s", source)
					}
				}
			}
		}},
	}

	routerPath := filepath.Join(platformRepoRoot(t), "internal", "api", "router.go")
	routerBytes, err := os.ReadFile(routerPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", routerPath, err)
	}
	router := string(routerBytes)

	for _, entry := range entries {
		if !strings.Contains(doc, entry.id) {
			t.Fatalf("coverage matrix missing id %q", entry.id)
		}
		if !strings.Contains(doc, entry.target) {
			t.Fatalf("coverage matrix missing target %q for %s", entry.target, entry.id)
		}
		if len(entry.source) == 0 && strings.TrimSpace(entry.importer) == "" {
			t.Fatalf("coverage entry %s must declare a source root or dedicated importer", entry.id)
		}
		if entry.importer != "" && !strings.Contains(router, entry.importer) {
			t.Fatalf("expected router to include dedicated importer %q for %s", entry.importer, entry.id)
		}
		if entry.verify != nil {
			entry.verify(t, claudeScan, codexScan)
		}
	}
}

func assertClaudeArchiveMissing(t *testing.T, scan *claudeLocalScanResult, needle string) {
	t.Helper()
	for _, file := range scan.Inventory.Files {
		if strings.Contains(file.Path, needle) {
			t.Fatalf("expected Claude archive to exclude %s, got %s", needle, file.Path)
		}
	}
}

func platformRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
