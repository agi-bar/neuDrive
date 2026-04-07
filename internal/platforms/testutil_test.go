package platforms

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agi-bar/agenthub/internal/localruntime"
)

func configurePlatformTestEnv(t *testing.T) (string, *localruntime.CLIConfig, string, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	cacheHome := filepath.Join(root, "cache")
	goCache := filepath.Join(root, "gocache")
	for _, dir := range []string{cacheHome, goCache} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("GOCACHE", goCache)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
	seedPlatformFixtures(t, home)
	logPath := installPlatformShims(t, "claude", "codex", "gemini", "cursor-agent")
	cfg := &localruntime.CLIConfig{
		Version: 2,
		Local: localruntime.LocalConfig{
			Storage:        "sqlite",
			SQLitePath:     filepath.Join(root, "local.db"),
			JWTSecret:      strings.Repeat("a", 64),
			VaultMasterKey: strings.Repeat("b", 64),
			PublicBaseURL:  "http://127.0.0.1:42690",
			Connections:    map[string]localruntime.LocalConnection{},
		},
		Profiles: map[string]localruntime.SyncProfile{},
	}
	if err := localruntime.EnsureLocalDefaults(cfg); err != nil {
		t.Fatalf("EnsureLocalDefaults: %v", err)
	}
	return home, cfg, cfg.Local.PublicBaseURL, logPath
}

func seedPlatformFixtures(t *testing.T, home string) {
	t.Helper()
	root := filepath.Join(repoRoot(t), "internal", "platforms", "testdata")
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		dest := platformFixtureDestination(home, filepath.ToSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(dest, "config.toml") || strings.HasSuffix(dest, "mcp-oauth-tokens.json") || strings.HasSuffix(dest, "mcp.json") {
			mode = 0o600
		}
		return os.WriteFile(dest, data, mode)
	})
	if err != nil {
		t.Fatalf("seed platform fixtures: %v", err)
	}
}

func platformFixtureDestination(home, rel string) string {
	parts := strings.Split(rel, "/")
	switch parts[0] {
	case "codex":
		if len(parts) > 1 && parts[1] == "skills" {
			return filepath.Join(home, ".agents", filepath.FromSlash(strings.Join(parts[1:], "/")))
		}
		return filepath.Join(home, ".codex", filepath.FromSlash(strings.Join(parts[1:], "/")))
	case "claude":
		if len(parts) == 2 && parts[1] == "claude.json" {
			return filepath.Join(home, ".claude.json")
		}
		return filepath.Join(home, ".claude", filepath.FromSlash(strings.Join(parts[1:], "/")))
	case "gemini":
		return filepath.Join(home, ".gemini", filepath.FromSlash(strings.Join(parts[1:], "/")))
	case "cursor":
		return filepath.Join(home, ".cursor", filepath.FromSlash(strings.Join(parts[1:], "/")))
	default:
		return filepath.Join(home, filepath.FromSlash(rel))
	}
}

func installPlatformShims(t *testing.T, commands ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("platform shim binaries are only supported in unix-like environments")
	}
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "platform-shim.log")
	for _, name := range commands {
		script := "#!/bin/sh\nset -eu\nlog=\"${AGENTHUB_TEST_SHIM_LOG:-}\"\nif [ -n \"$log\" ]; then\n  {\n    printf 'CMD=%s' \"$0\"\n    for arg in \"$@\"; do printf ' ARG=%s' \"$arg\"; done\n    printf '\\n'\n    env | sort | grep -E '^(AGENTHUB_|DATABASE_URL=|JWT_SECRET=|VAULT_MASTER_KEY=|PUBLIC_BASE_URL=)' || true\n    printf '%s\\n' '--'\n  } >> \"$log\"\nfi\nif [ \"$(basename \"$0\")\" = \"codex\" ] && [ \"${1:-}\" = \"exec\" ]; then\n  out=\"\"\n  shift\n  while [ \"$#\" -gt 0 ]; do\n    case \"$1\" in\n      --output-last-message)\n        out=\"$2\"\n        shift 2\n        ;;\n      --output-schema)\n        shift 2\n        ;;\n      *)\n        shift\n        ;;\n    esac\n  done\n  payload='{\"platform\":\"codex\",\"command\":\"export\",\"profile_rules\":[{\"title\":\"Working style\",\"content\":\"Be concise and actionable.\",\"exactness\":\"derived\",\"source_paths\":[\"~/.codex/AGENTS.md\"],\"confidence\":0.95}],\"memory_items\":[{\"title\":\"Approval policy\",\"content\":\"User prefers never approval in the fixture config.\",\"exactness\":\"derived\",\"source_paths\":[\"~/.codex/config.toml\"],\"confidence\":0.91}],\"projects\":[{\"name\":\"codex-fixture\",\"context\":\"Imported from the Codex agent export shim.\",\"exactness\":\"derived\",\"source_paths\":[\"~/.codex/sessions/demo.md\"]}],\"automations\":[{\"name\":\"fixture-automation\",\"content\":\"Automation metadata\",\"exactness\":\"reference\"}],\"tools\":[{\"name\":\"fixture-tool\",\"content\":\"Tool metadata\",\"exactness\":\"reference\"}],\"connections\":[{\"name\":\"agenthub-local\",\"content\":\"Local MCP connection\",\"exactness\":\"exact\"}],\"archives\":[{\"name\":\"legacy-session\",\"content\":\"Archived session note\",\"exactness\":\"reference\"}],\"unsupported\":[{\"name\":\"cloud-memory\",\"content\":\"Cloud-only memory is not exported in fixture mode.\",\"exactness\":\"reference\"}],\"notes\":[\"fixture codex export\"]}'\n  if [ -n \"$out\" ]; then\n    printf '%s\\n' \"$payload\" > \"$out\"\n  else\n    printf '%s\\n' \"$payload\"\n  fi\n  exit 0\nfi\nif [ \"$(basename \"$0\")\" = \"claude\" ] && [ \"${1:-}\" = \"-p\" ]; then\n  payload='{\"platform\":\"claude-code\",\"command\":\"export\",\"profile_rules\":[{\"title\":\"Claude working style\",\"content\":\"Prefer concise summaries with explicit follow-ups.\",\"exactness\":\"derived\",\"source_paths\":[\"~/.claude.json\"],\"confidence\":0.93}],\"memory_items\":[{\"title\":\"Claude memory\",\"content\":\"Remember to preserve unsupported exports as archive notes.\",\"exactness\":\"derived\",\"source_paths\":[\"~/.claude/projects/demo.md\"],\"confidence\":0.88}],\"projects\":[{\"name\":\"claude-fixture\",\"context\":\"Imported from the Claude headless export shim.\",\"exactness\":\"derived\",\"source_paths\":[\"~/.claude/projects/demo.md\"]}],\"automations\":[{\"name\":\"claude-automation\",\"content\":\"Automation metadata\",\"exactness\":\"reference\"}],\"tools\":[{\"name\":\"claude-plugin\",\"content\":\"Plugin metadata\",\"exactness\":\"reference\"}],\"connections\":[{\"name\":\"agenthub-local\",\"content\":\"Claude MCP connection\",\"exactness\":\"exact\"}],\"archives\":[{\"name\":\"claude-archive\",\"content\":\"Archived Claude context\",\"exactness\":\"reference\"}],\"unsupported\":[{\"name\":\"cloud-session\",\"content\":\"Cloud-only Claude session not exported in fixture mode.\",\"exactness\":\"reference\"}],\"notes\":[\"fixture claude export\"]}'\n  printf '%s\\n' \"$payload\"\n  exit 0\nfi\nexit 0\n"
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("write shim %s: %v", name, err)
		}
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AGENTHUB_TEST_SHIM_LOG", logPath)
	return logPath
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readShimLog(t *testing.T, logPath string) string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read shim log: %v", err)
	}
	return string(data)
}
