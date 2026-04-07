package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgenthubPlatformCommands_LocalSQLiteFixture(t *testing.T) {
	binary := buildAgenthubBinary(t)
	env, configPath, _, _, workDir := isolatedAgenthubEnv(t)
	home := envValue(env, "HOME")
	seedCLIPlatformFixtures(t, home)
	env, shimLog := installCLIPlatformShims(t, env, "claude", "codex", "gemini", "cursor-agent")

	stdout, _ := mustRunAgenthub(t, binary, env, "platform", "ls")
	for _, platform := range []string{"claude-code", "codex", "gemini-cli", "cursor-agent"} {
		if !strings.Contains(stdout, platform+"\tinstalled=true") {
			t.Fatalf("platform ls missing %s in output: %s", platform, stdout)
		}
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "ls")
	if !strings.Contains(stdout, "codex\tinstalled=true") {
		t.Fatalf("ls alias missing codex: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "platform", "show", "codex")
	if !strings.Contains(stdout, "Platform: Codex CLI") || !strings.Contains(stdout, "Discovered sources:") {
		t.Fatalf("unexpected platform show output: %s", stdout)
	}
	if !strings.Contains(stdout, filepath.Join(home, ".codex", "config.toml")) {
		t.Fatalf("expected codex config path in output: %s", stdout)
	}
	if !strings.Contains(stdout, "Entrypoint type: skill") || !strings.Contains(stdout, "Agent-mediated export: supported") {
		t.Fatalf("expected codex entrypoint metadata in output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "ls", "codex")
	if !strings.Contains(stdout, "Platform: Codex CLI") {
		t.Fatalf("ls codex alias mismatch: %s", stdout)
	}

	mustRunAgenthub(t, binary, env, "connect", "codex")
	cfg := loadCLIConfigForTest(t, configPath)
	if strings.TrimSpace(cfg.Local.Connections["codex"].Token) == "" {
		t.Fatal("expected saved codex connection token after connect")
	}
	if strings.TrimSpace(cfg.Local.Connections["codex"].EntrypointPath) == "" {
		t.Fatal("expected saved codex entrypoint metadata after connect")
	}
	logData, err := os.ReadFile(shimLog)
	if err != nil {
		t.Fatalf("read shim log: %v", err)
	}
	logText := string(logData)
	if !strings.Contains(logText, "ARG=add") || !strings.Contains(logText, "AGENTHUB_TOKEN=") {
		t.Fatalf("unexpected shim log after connect: %s", logText)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "platform", "show", "codex")
	if !strings.Contains(stdout, "Connected: true") || !strings.Contains(stdout, "Entrypoint installed: true") {
		t.Fatalf("expected connected codex status: %s", stdout)
	}
	if !strings.Contains(stdout, filepath.Join(home, ".agents", "skills", "agenthub")) {
		t.Fatalf("expected codex skill path in output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "import", "codex")
	if !strings.Contains(stdout, "Imported codex using mode=agent") {
		t.Fatalf("unexpected import output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "import", "codex", "--mode", "files")
	if !strings.Contains(stdout, "using mode=files") {
		t.Fatalf("unexpected files import output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "import", "codex", "--mode", "all")
	if !strings.Contains(stdout, "using mode=all") {
		t.Fatalf("unexpected all import output: %s", stdout)
	}

	exportDir := filepath.Join(workDir, "codex-export")
	stdout, _ = mustRunAgenthub(t, binary, env, "export", "codex", "--output", exportDir)
	if !strings.Contains(stdout, "Exported ") {
		t.Fatalf("unexpected export output: %s", stdout)
	}
	for _, expected := range []string{
		filepath.Join(exportDir, "profile", "config.toml"),
		filepath.Join(exportDir, "profile", "AGENTS.md"),
	} {
		if _, err := os.Stat(expected); err != nil {
			t.Fatalf("expected exported file %s: %v", expected, err)
		}
	}

	mustRunAgenthub(t, binary, env, "disconnect", "codex")
	cfg = loadCLIConfigForTest(t, configPath)
	if _, ok := cfg.Local.Connections["codex"]; ok {
		t.Fatal("expected codex connection removed after disconnect")
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "agenthub")); !os.IsNotExist(err) {
		t.Fatal("expected codex managed skill removed after disconnect")
	}
	logData, err = os.ReadFile(shimLog)
	if err != nil {
		t.Fatalf("read shim log: %v", err)
	}
	if !strings.Contains(string(logData), "ARG=remove") {
		t.Fatalf("expected remove invocation in shim log: %s", string(logData))
	}

	mustRunAgenthub(t, binary, env, "daemon", "stop")
}
