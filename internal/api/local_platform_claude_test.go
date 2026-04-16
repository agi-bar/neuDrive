package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/platforms"
	sqlitestorage "github.com/agi-bar/neudrive/internal/storage/sqlite"
)

func TestSQLiteSharedServerImportsClaudeTypedInventory(t *testing.T) {
	ts, store, adminToken, _, _ := newTestHTTPServer(t)
	ctx := context.Background()
	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}

	conversation := sqlitestorage.ClaudeConversation{
		Name:        "Release Planning",
		SessionID:   "session-123",
		ProjectName: "claude-demo",
		StartedAt:   "2026-04-15T10:00:00Z",
		Exactness:   "exact",
		SourcePaths: []string{"/Users/demo/.claude/projects/demo-session/session.jsonl"},
		Messages: []sqlitestorage.ClaudeConversationMessage{
			{Role: "user", Content: "Plan the migration.", Timestamp: "2026-04-15T10:00:00Z", Kind: "user"},
			{Role: "assistant", Content: "Run a dry run first.", Timestamp: "2026-04-15T10:01:00Z", Kind: "assistant"},
		},
	}
	body, err := json.Marshal(localPlatformImportRequest{
		Platform: "claude-code",
		AgentPayload: &sqlitestorage.AgentExportPayload{
			Platform: "claude-code",
			Command:  "export",
			Claude: &sqlitestorage.ClaudeInventory{
				Projects: []sqlitestorage.ClaudeProjectSnapshot{
					{
						Name:        "claude-demo",
						Context:     "Imported project context.",
						Exactness:   "exact",
						SourcePaths: []string{"/Users/demo/workspace/claude-demo"},
						Files: []sqlitestorage.ClaudeFileRecord{
							{
								Path:        "docs/spec.md",
								Content:     "# Spec\n\nImported from Claude.\n",
								ContentType: "text/markdown",
								Exactness:   "exact",
								SourcePath:  "/Users/demo/workspace/claude-demo/docs/spec.md",
							},
						},
					},
				},
				Bundles: []sqlitestorage.ClaudeBundle{
					{
						Name:        "release-helper",
						Kind:        "skill",
						Description: "Claude skill bundle",
						Exactness:   "exact",
						SourcePaths: []string{"/Users/demo/.claude/skills/release-helper"},
						Files: []sqlitestorage.ClaudeFileRecord{
							{
								Path:        "SKILL.md",
								Content:     "# Release Helper\n\nImported skill.\n",
								ContentType: "text/markdown",
								Exactness:   "exact",
								SourcePath:  "/Users/demo/.claude/skills/release-helper/SKILL.md",
							},
						},
					},
				},
				Conversations: []sqlitestorage.ClaudeConversation{conversation},
				Files: []sqlitestorage.ClaudeFileRecord{
					{
						Path:        "agent/runtime/settings.local.json",
						Content:     "{\n  \"api_key\": \"[REDACTED]\"\n}\n",
						ContentType: "application/json",
						Exactness:   "exact",
						SourcePath:  "/Users/demo/.claude/settings.local.json",
					},
				},
				SensitiveFindings: []sqlitestorage.AgentSensitiveFinding{
					{
						Title:           "api_key in settings.local.json",
						Detail:          "Potential plaintext secret discovered during Claude Code migration scan.",
						Severity:        "high",
						SourcePaths:     []string{"/Users/demo/.claude/settings.local.json"},
						RedactedExample: "\"api_key\": [REDACTED]",
					},
				},
				VaultCandidates: []sqlitestorage.AgentVaultCandidate{
					{
						Scope:       "claude.settings-local-json.api-key",
						Description: "Candidate vault scope for api_key",
						SourcePaths: []string{"/Users/demo/.claude/settings.local.json"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	status, env := doJSON(t, http.MethodPost, ts.URL+"/agent/local/platform/import", adminToken, body)
	if status != http.StatusOK || !env.OK {
		t.Fatalf("import local platform failed: status=%d env=%+v", status, env)
	}

	var resp localPlatformImportResponse
	if err := json.Unmarshal(env.Data, &resp); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	if resp.Agent == nil {
		t.Fatal("expected agent import result")
	}
	if resp.Agent.Bundles != 1 || resp.Agent.Conversations != 1 || resp.Agent.Projects != 1 || resp.Agent.ProjectFiles != 1 {
		t.Fatalf("unexpected Claude import counts: %+v", resp.Agent)
	}
	if resp.Agent.SensitiveFindings != 1 || resp.Agent.VaultCandidates != 1 {
		t.Fatalf("expected sensitive findings and vault candidates: %+v", resp.Agent)
	}

	for _, target := range []string{
		"/projects/claude-demo/context.md",
		"/projects/claude-demo/docs/spec.md",
		"/skills/release-helper/SKILL.md",
		claudeConversationPath(conversation),
		"/memory/conversations/claude-code/index.json",
		"/platforms/claude-code/agent/sensitive-findings.json",
	} {
		entry, err := store.Read(ctx, user.ID, target, models.TrustLevelWork)
		if err != nil {
			t.Fatalf("Read(%s): %v", target, err)
		}
		if strings.TrimSpace(entry.Content) == "" && !entry.IsDirectory {
			t.Fatalf("expected content at %s", target)
		}
	}
}

func TestSQLiteSharedServerLocalPlatformPreviewClaude(t *testing.T) {
	home := createClaudeDashboardFixture(t)
	t.Setenv("HOME", home)

	ts, _, adminToken, _, _ := newTestHTTPServer(t)
	body, err := json.Marshal(localPlatformDashboardRequest{
		Platform: "claude",
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
	if preview.DisplayName != "Claude Code" {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if len(preview.Categories) == 0 {
		t.Fatal("expected preview categories")
	}
	if len(preview.SensitiveFindings) == 0 || len(preview.VaultCandidates) == 0 {
		t.Fatalf("expected preview findings and vault candidates: %+v", preview)
	}

	var raw map[string]any
	if err := json.Unmarshal(env.Data, &raw); err != nil {
		t.Fatalf("Unmarshal raw preview: %v", err)
	}
	if _, ok := raw["display_name"]; !ok {
		t.Fatalf("expected snake_case display_name in raw preview: %s", string(env.Data))
	}
	if _, ok := raw["categories"]; !ok {
		t.Fatalf("expected snake_case categories in raw preview: %s", string(env.Data))
	}
}

func TestSQLiteSharedServerLocalPlatformImportClaude(t *testing.T) {
	home := createClaudeDashboardFixture(t)
	t.Setenv("HOME", home)

	ts, store, adminToken, _, _ := newTestHTTPServer(t)
	ctx := context.Background()
	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}

	body, err := json.Marshal(localPlatformDashboardRequest{
		Platform: "claude",
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
	if resp.Agent.Conversations == 0 || resp.Agent.Bundles == 0 || resp.Agent.SensitiveFindings == 0 {
		t.Fatalf("expected Claude import details in %+v", resp.Agent)
	}

	for _, target := range []string{
		"/memory/conversations/claude-code/index.json",
		"/platforms/claude-code/agent/sensitive-findings.json",
		"/skills/release-helper/SKILL.md",
	} {
		entry, err := store.Read(ctx, user.ID, target, models.TrustLevelWork)
		if err != nil {
			t.Fatalf("Read(%s): %v", target, err)
		}
		if strings.TrimSpace(entry.Content) == "" && !entry.IsDirectory {
			t.Fatalf("expected content at %s", target)
		}
	}
}

func createClaudeDashboardFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(home, "workspace", "claude-demo")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".claude", "CLAUDE.md"), "# Global Rules\n\nBe explicit about risks.\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".claude", "agent-memory", "team.md"), "Remember the release checklist.\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".claude", "settings.local.json"), "{\n  \"api_key\": \"sk-test-secret\",\n  \"theme\": \"compact\"\n}\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".claude", "skills", "release-helper", "SKILL.md"), "# Release Helper\n\nUse this skill to package releases.\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".claude", "projects", "demo-session", "memory", "remember.md"), "Document the migration choices.\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".claude", "projects", "demo-session", "session.jsonl"), strings.Join([]string{
		`{"type":"user","timestamp":"2026-04-15T10:00:00Z","message":{"role":"user","content":"Plan the release migration."}}`,
		`{"type":"assistant","timestamp":"2026-04-15T10:01:00Z","message":{"role":"assistant","content":"Start with a dry run and redact secrets."}}`,
	}, "\n")+"\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".claude", "projects", "demo-session", "subagents", "research.jsonl"), strings.Join([]string{
		`{"type":"user","timestamp":"2026-04-15T10:02:00Z","message":{"role":"user","content":"Research risks."}}`,
		`{"type":"assistant","timestamp":"2026-04-15T10:03:00Z","message":{"role":"assistant","content":"Sensitive settings must be redacted."}}`,
	}, "\n")+"\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(home, ".claude.json"), "{\n  \"projects\": {\n    \"~/workspace/claude-demo\": {\n      \"name\": \"Claude Demo\"\n    }\n  }\n}\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(workspace, "CLAUDE.md"), "# Workspace Context\n\nShip from the release branch.\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(workspace, "docs", "spec.md"), "# Spec\n\nThis workspace documents the migration.\n")
	writeClaudeDashboardFixtureFile(t, filepath.Join(workspace, ".mcp.json"), "{\n  \"mcpServers\": {}\n}\n")
	return home
}

func writeClaudeDashboardFixtureFile(t *testing.T, target, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", target, err)
	}
}
