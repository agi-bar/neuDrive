package cli

import (
	"archive/zip"
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
	for _, expected := range []string{"dir\tprofile/", "dir\tmemory/", "dir\tproject/", "dir\tskill/", "dir\tsecret/", "dir\tplatform/"} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("ls root missing %q: %s", expected, stdout)
		}
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

	stdout, _ = mustRunAgenthub(t, binary, env, "platform", "show", "claude")
	if !strings.Contains(stdout, "Platform: Claude Code") || !strings.Contains(stdout, "Agent-mediated export: supported") {
		t.Fatalf("unexpected claude platform show output: %s", stdout)
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
	if !strings.Contains(logText, "ARG=add") || !strings.Contains(logText, "NEUDRIVE_TOKEN=") {
		t.Fatalf("unexpected shim log after connect: %s", logText)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "platform", "show", "codex")
	if !strings.Contains(stdout, "Connected: true") || !strings.Contains(stdout, "Entrypoint installed: true") {
		t.Fatalf("expected connected codex status: %s", stdout)
	}
	if !strings.Contains(stdout, filepath.Join(home, ".agents", "skills", "neudrive")) {
		t.Fatalf("expected codex skill path in output: %s", stdout)
	}
	if !strings.Contains(stdout, "$neudrive status") {
		t.Fatalf("expected codex status chat usage in output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "import", "platform", "codex")
	if !strings.Contains(stdout, "Imported codex using mode=agent") {
		t.Fatalf("unexpected import output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "browse", "--print-url", "/data/files")
	if !strings.Contains(stdout, "/data/files?local_token=") {
		t.Fatalf("unexpected browse URL output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "ls", "profile")
	if !strings.Contains(stdout, "file\tprofile/codex-agent") {
		t.Fatalf("expected imported profile file in ls profile: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "read", "profile/codex-agent")
	if !strings.Contains(stdout, "Be concise and actionable.") {
		t.Fatalf("unexpected read output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "import", "platform", "codex", "--mode", "files")
	if !strings.Contains(stdout, "using mode=files") {
		t.Fatalf("unexpected files import output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "import", "platform", "codex", "--mode", "all")
	if !strings.Contains(stdout, "using mode=all") {
		t.Fatalf("unexpected all import output: %s", stdout)
	}

	mustRunAgenthub(t, binary, env, "connect", "claude")
	cfg = loadCLIConfigForTest(t, configPath)
	if strings.TrimSpace(cfg.Local.Connections["claude-code"].Token) == "" {
		t.Fatal("expected saved claude connection token after connect")
	}
	stdout, _ = mustRunAgenthub(t, binary, env, "platform", "show", "claude")
	if !strings.Contains(stdout, "Connected: true") || !strings.Contains(stdout, "Entrypoint type: command") {
		t.Fatalf("expected connected claude status: %s", stdout)
	}
	if !strings.Contains(stdout, filepath.Join(home, ".claude", "commands", "neudrive.md")) {
		t.Fatalf("expected claude command path in output: %s", stdout)
	}
	if !strings.Contains(stdout, "/neudrive status") {
		t.Fatalf("expected claude status chat usage in output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "import", "platform", "claude")
	if !strings.Contains(stdout, "Imported claude using mode=agent") {
		t.Fatalf("unexpected claude import output: %s", stdout)
	}
	claudeSkillsZip := filepath.Join(workDir, "claude-web-skills.zip")
	writeTestSkillZip(t, claudeSkillsZip, map[string][]byte{
		"claude-web-skill/SKILL.md":        []byte("# Claude Web Skill\n\nImported from zip.\n"),
		"claude-web-skill/helper.py":       []byte("print('hello from claude web zip')\n"),
		"claude-web-skill/assets/logo.png": []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00},
	})
	stdout, _ = mustRunAgenthub(t, binary, env, "import", "platform", "claude", "--zip", claudeSkillsZip)
	if !strings.Contains(stdout, "Imported 3 files") || !strings.Contains(stdout, "into /skills using claude") {
		t.Fatalf("unexpected claude zip import output: %s", stdout)
	}
	stdout, _ = mustRunAgenthub(t, binary, env, "ls", "skill/claude-web-skill")
	if !strings.Contains(stdout, "file\tskill/claude-web-skill/SKILL.md") || !strings.Contains(stdout, "file\tskill/claude-web-skill/helper.py") || !strings.Contains(stdout, "dir\tskill/claude-web-skill/assets") {
		t.Fatalf("expected imported claude web skill files: %s", stdout)
	}
	stdout, _ = mustRunAgenthub(t, binary, env, "ls", "skill/claude-web-skill/assets")
	if !strings.Contains(stdout, "file\tskill/claude-web-skill/assets/logo.png") {
		t.Fatalf("expected imported claude web binary asset: %s", stdout)
	}
	stdout, _ = mustRunAgenthub(t, binary, env, "import", "platform", "claude", "--mode", "all")
	if !strings.Contains(stdout, "Imported claude using mode=all") {
		t.Fatalf("unexpected claude all import output: %s", stdout)
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
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "neudrive")); !os.IsNotExist(err) {
		t.Fatal("expected codex managed skill removed after disconnect")
	}
	logData, err = os.ReadFile(shimLog)
	if err != nil {
		t.Fatalf("read shim log: %v", err)
	}
	if !strings.Contains(string(logData), "ARG=remove") {
		t.Fatalf("expected remove invocation in shim log: %s", string(logData))
	}

	mustRunAgenthub(t, binary, env, "disconnect", "claude")
	cfg = loadCLIConfigForTest(t, configPath)
	if _, ok := cfg.Local.Connections["claude-code"]; ok {
		t.Fatal("expected claude connection removed after disconnect")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "neudrive")); !os.IsNotExist(err) {
		t.Fatal("expected claude managed skill removed after disconnect")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "commands", "neudrive.md")); !os.IsNotExist(err) {
		t.Fatal("expected claude managed command removed after disconnect")
	}

	mustRunAgenthub(t, binary, env, "daemon", "stop")
}

func writeTestSkillZip(t *testing.T, target string, files map[string][]byte) {
	t.Helper()
	f, err := os.Create(target)
	if err != nil {
		t.Fatalf("create zip %s: %v", target, err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip %s: %v", target, err)
	}
}
