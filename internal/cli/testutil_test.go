package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agi-bar/agenthub/internal/runtimecfg"
)

func configureIsolatedCLIEnv(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("", "agenthub-cli-env-*")
	if err != nil {
		t.Fatalf("mktemp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(root)
	})
	home := filepath.Join(root, "home")
	configHome := filepath.Join(root, "config")
	stateHome := filepath.Join(root, "state")
	dataHome := filepath.Join(root, "data")
	cacheHome := filepath.Join(root, "cache")
	goCache := filepath.Join(root, "gocache")
	for _, dir := range []string{home, configHome, stateHome, dataHome, cacheHome, goCache} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("GOCACHE", goCache)
	t.Setenv(runtimecfg.ConfigEnv, filepath.Join(configHome, "agenthub", "config.json"))
	for _, key := range []string{
		"AGENTHUB_TOKEN",
		"AGENTHUB_SYNC_TOKEN",
		"AGENTHUB_API_BASE",
		"AGENTHUB_SYNC_API_BASE",
		"AGENTHUB_SYNC_PROFILE",
	} {
		t.Setenv(key, "")
	}
	return home
}

func runRootForTest(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	configureIsolatedCLIEnv(t)
	return captureRunForTest(t, func() int {
		return Run(args)
	})
}

func captureRunForTest(t *testing.T, fn func() int) (string, string, int) {
	t.Helper()
	origStdout := os.Stdout
	origStderr := os.Stderr
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	code := fn()
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	_, _ = io.Copy(&stdoutBuf, stdoutReader)
	_, _ = io.Copy(&stderrBuf, stderrReader)
	_ = stdoutReader.Close()
	_ = stderrReader.Close()
	return stdoutBuf.String(), stderrBuf.String(), code
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		entry := env[i]
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func prependPathEnv(env []string, dir string) []string {
	current := envValue(env, "PATH")
	if current == "" {
		return appendOrReplaceEnv(env, "PATH", dir)
	}
	return appendOrReplaceEnv(env, "PATH", dir+string(os.PathListSeparator)+current)
}

func appendOrReplaceEnv(env []string, key, value string) []string {
	prefix := key + "="
	replaced := false
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			replaced = true
			break
		}
	}
	if !replaced {
		env = append(env, prefix+value)
	}
	return env
}

func seedCLIPlatformFixtures(t *testing.T, home string) {
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
		dest := cliPlatformFixtureDestination(home, filepath.ToSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
	if err != nil {
		t.Fatalf("seed platform fixtures: %v", err)
	}
}

func cliPlatformFixtureDestination(home, rel string) string {
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

func installCLIPlatformShims(t *testing.T, env []string, commands ...string) ([]string, string) {
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
	env = prependPathEnv(env, binDir)
	env = appendOrReplaceEnv(env, "AGENTHUB_TEST_SHIM_LOG", logPath)
	return env, logPath
}
