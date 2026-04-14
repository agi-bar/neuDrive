package cli

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agi-bar/agenthub/internal/runtimecfg"
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
	if !strings.Contains(stdout, "$agenthub git init") || !strings.Contains(stdout, "$agenthub git pull") {
		t.Fatalf("expected codex git chat usage in output: %s", stdout)
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
	if !strings.Contains(stdout, filepath.Join(home, ".claude", "commands", "agenthub.md")) {
		t.Fatalf("expected claude command path in output: %s", stdout)
	}
	if !strings.Contains(stdout, "/agenthub git init") || !strings.Contains(stdout, "/agenthub git pull") {
		t.Fatalf("expected claude git chat usage in output: %s", stdout)
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

	mustRunAgenthub(t, binary, env, "disconnect", "claude")
	cfg = loadCLIConfigForTest(t, configPath)
	if _, ok := cfg.Local.Connections["claude-code"]; ok {
		t.Fatal("expected claude connection removed after disconnect")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "agenthub")); !os.IsNotExist(err) {
		t.Fatal("expected claude managed skill removed after disconnect")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "commands", "agenthub.md")); !os.IsNotExist(err) {
		t.Fatal("expected claude managed command removed after disconnect")
	}

	mustRunAgenthub(t, binary, env, "daemon", "stop")
}

func TestAgenthubGitInitPullAndAutoSync(t *testing.T) {
	binary := buildAgenthubBinary(t)
	env, _, _, _, workDir := isolatedAgenthubEnv(t)
	home := envValue(env, "HOME")
	seedCLIPlatformFixtures(t, home)
	env, _ = installCLIPlatformShims(t, env, "codex")

	mustRunAgenthub(t, binary, env, "connect", "codex")
	stdout, _ := mustRunAgenthub(t, binary, env, "import", "platform", "codex")
	if !strings.Contains(stdout, "Imported codex using mode=agent") {
		t.Fatalf("unexpected initial import output: %s", stdout)
	}

	mirrorDir := filepath.Join(workDir, "git-mirror")
	stdout, _ = mustRunAgenthub(t, binary, env, "git", "init", "--output", mirrorDir)
	if !strings.Contains(stdout, "本地 Git 镜像目录: "+mirrorDir) {
		t.Fatalf("expected mirror path in output: %s", stdout)
	}
	if !strings.Contains(stdout, "已同步到本地 Git 目录: "+mirrorDir) {
		t.Fatalf("expected sync message in output: %s", stdout)
	}
	for _, expected := range []string{
		filepath.Join(mirrorDir, ".git"),
		filepath.Join(mirrorDir, "README.md"),
		filepath.Join(mirrorDir, "_agenthub", "metadata.json"),
		filepath.Join(mirrorDir, "memory", "profile", "codex-agent.md"),
	} {
		if _, err := os.Stat(expected); err != nil {
			t.Fatalf("expected mirror artifact %s: %v", expected, err)
		}
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "import", "platform", "codex", "--mode", "files")
	if !strings.Contains(stdout, "已同步到本地 Git 目录: "+mirrorDir) {
		t.Fatalf("expected auto-sync message after import: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(mirrorDir, "platforms", "codex", "profile", "config.toml")); err != nil {
		t.Fatalf("expected mirrored codex platform file: %v", err)
	}

	if err := os.Remove(filepath.Join(mirrorDir, "memory", "profile", "codex-agent.md")); err != nil {
		t.Fatalf("remove mirrored file before git pull: %v", err)
	}
	stdout, _ = mustRunAgenthub(t, binary, env, "git", "pull")
	if !strings.Contains(stdout, "本地 Git 镜像目录: "+mirrorDir) {
		t.Fatalf("expected mirror path after git pull: %s", stdout)
	}
	if !strings.Contains(stdout, "已同步到本地 Git 目录: "+mirrorDir) {
		t.Fatalf("expected sync message after git pull: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(mirrorDir, "memory", "profile", "codex-agent.md")); err != nil {
		t.Fatalf("expected git pull to restore mirrored file: %v", err)
	}
}

func TestAgenthubGitInitUsesConfiguredMirrorPath(t *testing.T) {
	binary := buildAgenthubBinary(t)
	env, configPath, _, _, workDir := isolatedAgenthubEnv(t)
	home := envValue(env, "HOME")
	seedCLIPlatformFixtures(t, home)
	env, _ = installCLIPlatformShims(t, env, "codex")

	mustRunAgenthub(t, binary, env, "connect", "codex")
	stdout, _ := mustRunAgenthub(t, binary, env, "import", "codex")
	if !strings.Contains(stdout, "Imported codex using mode=agent") {
		t.Fatalf("unexpected initial import output: %s", stdout)
	}

	cfg := loadCLIConfigForTest(t, configPath)
	configuredMirrorDir := filepath.Join(workDir, "configured-git-mirror")
	cfg.Local.GitMirrorPath = configuredMirrorDir
	if err := runtimecfg.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("save config with git mirror path: %v", err)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "git", "init")
	if !strings.Contains(stdout, "本地 Git 镜像目录: "+configuredMirrorDir) {
		t.Fatalf("expected configured mirror path in output: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(configuredMirrorDir, ".git")); err != nil {
		t.Fatalf("expected configured mirror repo initialized: %v", err)
	}
}

func TestAgenthubGitInitWritesDefaultMirrorPathToConfig(t *testing.T) {
	binary := buildAgenthubBinary(t)
	env, configPath, _, _, workDir := isolatedAgenthubEnv(t)
	home := envValue(env, "HOME")
	seedCLIPlatformFixtures(t, home)
	env, _ = installCLIPlatformShims(t, env, "codex")

	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir work dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	mustRunAgenthub(t, binary, env, "connect", "codex")
	stdout, _ := mustRunAgenthub(t, binary, env, "import", "codex")
	if !strings.Contains(stdout, "Imported codex using mode=agent") {
		t.Fatalf("unexpected initial import output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "git", "init")
	expectedMirrorDir := filepath.Join(workDir, "agenthub-export", "git-mirror")
	if !strings.Contains(stdout, "本地 Git 镜像目录: "+expectedMirrorDir) {
		t.Fatalf("expected default mirror path in output: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(expectedMirrorDir, ".git")); err != nil {
		t.Fatalf("expected default mirror repo initialized: %v", err)
	}

	cfg := loadCLIConfigForTest(t, configPath)
	if cfg.Local.GitMirrorPath != runtimecfg.DefaultGitMirrorPath {
		t.Fatalf("expected default git_mirror_path %q in config, got %q", runtimecfg.DefaultGitMirrorPath, cfg.Local.GitMirrorPath)
	}
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
