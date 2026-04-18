package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/platforms"
)

func TestSQLiteSharedServerLocalPlatformPreviewCodex(t *testing.T) {
	home := createCodexDashboardFixture(t)
	t.Setenv("HOME", home)

	ts, _, adminToken, _, _ := newTestHTTPServer(t)
	body, err := json.Marshal(localPlatformDashboardRequest{
		Platform: "codex",
		Mode:     "agent",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	status, env := doJSON(t, http.MethodPost, ts.URL+"/api/local/platform/preview", adminToken, body)
	if status != http.StatusOK || !env.OK {
		t.Fatalf("preview failed: status=%d env=%+v", status, env)
	}

	var preview platforms.ImportPreview
	if err := json.Unmarshal(env.Data, &preview); err != nil {
		t.Fatalf("Unmarshal preview: %v", err)
	}
	if preview.DisplayName != "Codex CLI" {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if len(preview.Categories) == 0 {
		t.Fatal("expected preview categories")
	}
	if len(preview.SensitiveFindings) == 0 || len(preview.VaultCandidates) == 0 {
		t.Fatalf("expected codex findings and vault candidates: %+v", preview)
	}
}

func TestSQLiteSharedServerLocalPlatformImportCodex(t *testing.T) {
	home := createCodexDashboardFixture(t)
	t.Setenv("HOME", home)

	ts, store, adminToken, _, _ := newTestHTTPServer(t)
	ctx := context.Background()
	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}

	body, err := json.Marshal(localPlatformDashboardRequest{
		Platform: "codex",
		Mode:     "agent",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	status, env := doJSON(t, http.MethodPost, ts.URL+"/api/local/platform/import", adminToken, body)
	if status != http.StatusOK || !env.OK {
		t.Fatalf("dashboard import failed: status=%d env=%+v", status, env)
	}

	var resp localPlatformDashboardImportResponse
	if err := json.Unmarshal(env.Data, &resp); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	if resp.Agent == nil {
		t.Fatalf("expected agent result, got %+v", resp)
	}
	if resp.Agent.ProfileCategories == 0 || resp.Agent.Projects == 0 || resp.Agent.SensitiveFindings == 0 || resp.Agent.VaultCandidates == 0 {
		t.Fatalf("expected codex import details in %+v", resp.Agent)
	}

	for _, target := range []string{
		hubpath.ProfilePath("codex-agent"),
		"/projects/neudrive/context.md",
		"/platforms/codex/agent/connections.json",
		"/platforms/codex/agent/sensitive-findings.json",
		"/platforms/codex/agent/vault-candidates.json",
	} {
		entry, err := store.Read(ctx, user.ID, target, models.TrustLevelFull)
		if err != nil {
			t.Fatalf("Read(%s): %v", target, err)
		}
		if strings.TrimSpace(entry.Content) == "" && !entry.IsDirectory {
			t.Fatalf("expected content at %s", target)
		}
	}
}

func createCodexDashboardFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".codex", "AGENTS.md"), "# Local Codex Notes\n\nKeep responses concise and actionable.\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".codex", "rules", "default.rules"), "prefix_rule(pattern=[\"go\", \"test\", \"./...\"], decision=\"allow\")\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".codex", "memories", "workspace.md"), "Remember the local import fixtures.\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".codex", "config.toml"), strings.Join([]string{
		`model = "gpt-5.4"`,
		`model_reasoning_effort = "high"`,
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
		`JWT_SECRET = "jwt_test_secret"`,
	}, "\n")+"\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".codex", "auth.json"), "{\n  \"auth_mode\": \"chatgpt\",\n  \"tokens\": {\n    \"access_token\": \"secret-access\",\n    \"refresh_token\": \"secret-refresh\"\n  },\n  \"last_refresh\": \"2026-04-16T10:00:00Z\"\n}\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".codex", "session_index.jsonl"), `{"id":"session-001","thread_name":"Explore project overview","updated_at":"2026-04-16T10:05:00Z"}`+"\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".codex", "sessions", "2026", "04", "16", "session-001.jsonl"), `{"timestamp":"2026-04-16T10:00:00Z","type":"session_meta","payload":{"id":"session-001","timestamp":"2026-04-16T10:00:00Z","cwd":"/Users/demo/workspace/neudrive","originator":"Codex Desktop","cli_version":"0.118.0","source":"desktop","model_provider":"openai"}}`+"\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".codex", "history.jsonl"), "{}\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".codex", "shell_snapshots", "fixture.sh"), "#!/bin/sh\necho fixture\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".agents", "skills", "sample", "SKILL.md"), "# Sample\n")
	return home
}
