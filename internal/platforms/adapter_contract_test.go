package platforms

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type adapterContractCase struct {
	id               string
	displayName      string
	configRel        string
	expectedFiles    int
	expectedExports  []string
	assertConnect    func(t *testing.T, home, daemonURL, logText string, connPath string)
	assertDisconnect func(t *testing.T, home string)
}

func TestAdapterContracts(t *testing.T) {
	cases := []adapterContractCase{
		{
			id:            "claude-code",
			displayName:   "Claude Code",
			configRel:     ".claude.json",
			expectedFiles: 2,
			expectedExports: []string{
				filepath.Join("connections", "claude.json"),
				filepath.Join("projects", "projects", "demo.md"),
			},
			assertConnect: func(t *testing.T, home, daemonURL, logText string, _ string) {
				t.Helper()
				if !strings.Contains(logText, "ARG=mcp ARG=add ARG=--scope ARG=user ARG=--transport ARG=http") {
					t.Fatalf("expected claude add invocation in shim log: %s", logText)
				}
				if !strings.Contains(logText, "Authorization: Bearer ") || !strings.Contains(logText, daemonURL+"/mcp") {
					t.Fatalf("expected claude auth header and daemon url in shim log: %s", logText)
				}
				for _, expected := range []string{
					filepath.Join(home, ".claude", "skills", "agenthub", "SKILL.md"),
					filepath.Join(home, ".claude", "commands", "agenthub.md"),
				} {
					if _, err := os.Stat(expected); err != nil {
						t.Fatalf("expected managed claude entrypoint %s: %v", expected, err)
					}
				}
			},
			assertDisconnect: func(t *testing.T, home string) {
				t.Helper()
				for _, target := range []string{
					filepath.Join(home, ".claude", "skills", "agenthub"),
					filepath.Join(home, ".claude", "commands", "agenthub.md"),
				} {
					if _, err := os.Stat(target); !os.IsNotExist(err) {
						t.Fatalf("expected claude managed path removed: %s", target)
					}
				}
			},
		},
		{
			id:            "codex",
			displayName:   "Codex CLI",
			configRel:     filepath.Join(".codex", "config.toml"),
			expectedFiles: 3,
			expectedExports: []string{
				filepath.Join("profile", "config.toml"),
				filepath.Join("profile", "AGENTS.md"),
				filepath.Join("skills", "skills", "sample", "SKILL.md"),
			},
			assertConnect: func(t *testing.T, home, daemonURL, logText string, _ string) {
				t.Helper()
				if !strings.Contains(logText, "ARG=mcp ARG=add ARG=agenthub-local") {
					t.Fatalf("expected codex add invocation in shim log: %s", logText)
				}
				for _, needle := range []string{"AGENTHUB_TOKEN=", "AGENTHUB_STORAGE=sqlite", "PUBLIC_BASE_URL=http://127.0.0.1:42690", "ARG=mcp ARG=stdio"} {
					if !strings.Contains(logText, needle) {
						t.Fatalf("expected %q in codex shim log: %s", needle, logText)
					}
				}
				skillPath := filepath.Join(home, ".agents", "skills", "agenthub", "SKILL.md")
				if _, err := os.Stat(skillPath); err != nil {
					t.Fatalf("expected managed codex skill %s: %v", skillPath, err)
				}
			},
			assertDisconnect: func(t *testing.T, home string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "agenthub")); !os.IsNotExist(err) {
					t.Fatalf("expected codex managed skill removed")
				}
			},
		},
		{
			id:            "gemini-cli",
			displayName:   "Gemini CLI",
			configRel:     filepath.Join(".gemini", "settings.json"),
			expectedFiles: 2,
			expectedExports: []string{
				filepath.Join("connections", "mcp-oauth-tokens.json"),
				filepath.Join("profile", "settings.json"),
			},
			assertConnect: func(t *testing.T, home, daemonURL, logText string, _ string) {
				t.Helper()
				if !strings.Contains(logText, "ARG=mcp ARG=add ARG=--scope ARG=user ARG=--transport ARG=http") {
					t.Fatalf("expected gemini add invocation in shim log: %s", logText)
				}
				if !strings.Contains(logText, "Authorization: Bearer ") || !strings.Contains(logText, daemonURL+"/mcp") {
					t.Fatalf("expected gemini auth header and daemon url in shim log: %s", logText)
				}
			},
			assertDisconnect: func(t *testing.T, home string) {
				t.Helper()
			},
		},
		{
			id:            "cursor-agent",
			displayName:   "Cursor Agent",
			configRel:     filepath.Join(".cursor", "mcp.json"),
			expectedFiles: 2,
			expectedExports: []string{
				filepath.Join("connections", "mcp.json"),
				filepath.Join("projects", "projects", "demo.md"),
			},
			assertConnect: func(t *testing.T, home, daemonURL, logText string, connPath string) {
				t.Helper()
				data, err := os.ReadFile(connPath)
				if err != nil {
					t.Fatalf("read cursor config: %v", err)
				}
				var payload map[string]any
				if err := json.Unmarshal(data, &payload); err != nil {
					t.Fatalf("decode cursor config: %v", err)
				}
				servers, _ := payload["mcpServers"].(map[string]any)
				server, _ := servers[LocalServerName].(map[string]any)
				if server == nil {
					t.Fatalf("expected %s entry in cursor mcp config: %s", LocalServerName, string(data))
				}
				if got, _ := server["url"].(string); got != daemonURL+"/mcp" {
					t.Fatalf("unexpected cursor daemon url: %q", got)
				}
			},
			assertDisconnect: func(t *testing.T, home string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(home, ".cursor", "mcp.json"))
				if err != nil {
					t.Fatalf("read cursor config after disconnect: %v", err)
				}
				if strings.Contains(string(data), LocalServerName) {
					t.Fatalf("expected %s removed from cursor config: %s", LocalServerName, string(data))
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			home, cfg, daemonURL, shimLog := configurePlatformTestEnv(t)
			ctx := context.Background()

			adapter, err := Resolve(tc.id)
			if err != nil {
				t.Fatalf("Resolve(%s): %v", tc.id, err)
			}

			status := adapter.Detect(cfg, daemonURL)
			if status.ID != tc.id || status.DisplayName != tc.displayName {
				t.Fatalf("unexpected detect identity: %+v", status)
			}
			if !status.Installed {
				t.Fatalf("expected installed=true for %s", tc.id)
			}
			if status.Connected {
				t.Fatalf("expected disconnected status for %s", tc.id)
			}
			if status.ConfigPath != filepath.Join(home, filepath.FromSlash(tc.configRel)) {
				t.Fatalf("unexpected config path for %s: %q", tc.id, status.ConfigPath)
			}
			if status.DaemonTarget != daemonURL+"/mcp" {
				t.Fatalf("unexpected daemon target for %s: %q", tc.id, status.DaemonTarget)
			}
			if len(status.SupportedDomains) == 0 {
				t.Fatalf("expected supported domains for %s", tc.id)
			}
			if len(status.Sources) == 0 {
				t.Fatalf("expected discovered sources for %s", tc.id)
			}
			if status.AgentMediated == "" {
				t.Fatalf("expected agent mediated state for %s", tc.id)
			}

			connection, err := EnsureConnection(ctx, cfg, tc.id, "/tmp/agenthub-test", daemonURL)
			if err != nil {
				t.Fatalf("EnsureConnection(%s): %v", tc.id, err)
			}
			saved := cfg.Local.Connections[tc.id]
			if strings.TrimSpace(saved.Token) == "" || strings.TrimSpace(saved.TokenID) == "" {
				t.Fatalf("expected token saved for %s: %+v", tc.id, saved)
			}
			if saved.LastPlatformURL != daemonURL+"/mcp" {
				t.Fatalf("unexpected last platform url for %s: %q", tc.id, saved.LastPlatformURL)
			}
			if connection.Transport == "" {
				t.Fatalf("expected transport for %s", tc.id)
			}
			if tc.id == "codex" {
				if saved.EntrypointType != "skill" || !strings.Contains(saved.EntrypointPath, filepath.Join(".agents", "skills", "agenthub")) {
					t.Fatalf("unexpected codex entrypoint metadata: %+v", saved)
				}
			}
			if tc.id == "claude-code" {
				if saved.EntrypointType != "command" || !strings.Contains(saved.EntrypointPath, filepath.Join(".claude", "commands", "agenthub.md")) {
					t.Fatalf("unexpected claude entrypoint metadata: %+v", saved)
				}
			}

			logText := readShimLog(t, shimLog)
			tc.assertConnect(t, home, daemonURL, logText, filepath.Join(home, filepath.FromSlash(tc.configRel)))

			status = adapter.Detect(cfg, daemonURL)
			if !status.Connected {
				t.Fatalf("expected connected status for %s", tc.id)
			}
			if tc.id == "codex" && (!status.EntrypointInstalled || status.EntrypointType != "skill") {
				t.Fatalf("expected codex skill entrypoint installed: %+v", status)
			}
			if tc.id == "claude-code" && (!status.EntrypointInstalled || status.EntrypointType != "command") {
				t.Fatalf("expected claude command entrypoint installed: %+v", status)
			}

			importResult, err := ImportIntoLocalHub(ctx, cfg, tc.id)
			if err != nil {
				t.Fatalf("ImportIntoLocalHub(%s): %v", tc.id, err)
			}
			if importResult.Files != tc.expectedFiles {
				t.Fatalf("unexpected import file count for %s: got %d want %d", tc.id, importResult.Files, tc.expectedFiles)
			}

			exportRoot := filepath.Join(t.TempDir(), "export")
			exportResult, err := ExportFromLocalHub(ctx, cfg, tc.id, exportRoot)
			if err != nil {
				t.Fatalf("ExportFromLocalHub(%s): %v", tc.id, err)
			}
			if exportResult.Files != tc.expectedFiles {
				t.Fatalf("unexpected export file count for %s: got %d want %d", tc.id, exportResult.Files, tc.expectedFiles)
			}
			for _, rel := range tc.expectedExports {
				if _, err := os.Stat(filepath.Join(exportRoot, filepath.FromSlash(rel))); err != nil {
					t.Fatalf("expected exported path for %s: %s (%v)", tc.id, rel, err)
				}
			}

			if err := Disconnect(ctx, cfg, tc.id); err != nil {
				t.Fatalf("Disconnect(%s): %v", tc.id, err)
			}
			if _, ok := cfg.Local.Connections[tc.id]; ok {
				t.Fatalf("expected connection removed for %s", tc.id)
			}
			if err := Disconnect(ctx, cfg, tc.id); err != nil {
				t.Fatalf("Disconnect(%s) second time: %v", tc.id, err)
			}
			tc.assertDisconnect(t, home)

			if tc.id != "cursor-agent" {
				logText = readShimLog(t, shimLog)
				if !strings.Contains(logText, "ARG=remove") {
					t.Fatalf("expected remove invocation for %s in shim log: %s", tc.id, logText)
				}
			}
		})
	}
}
