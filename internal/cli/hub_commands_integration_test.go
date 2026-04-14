package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgenthubHubCommands_LocalSQLiteFixture(t *testing.T) {
	binary := buildAgenthubBinary(t)
	env, configPath, statePath, _, workDir := isolatedAgenthubEnv(t)

	stdout, _ := mustRunAgenthub(t, binary, env, "write", "profile/preferences", "Keep it concise.")
	if !strings.Contains(stdout, "Updated profile/preferences.") {
		t.Fatalf("unexpected profile write output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "read", "profile/preferences")
	if !strings.Contains(stdout, "Keep it concise.") {
		t.Fatalf("unexpected profile read output: %s", stdout)
	}

	cfg := loadCLIConfigForTest(t, configPath)
	state := loadRuntimeStateForTest(t, statePath)
	seedLocalSecretForHubTest(t, state.APIBase, cfg.Local.OwnerToken, "auth.github.test", "fixture secret value")

	stdout, _ = mustRunAgenthub(t, binary, env, "ls", "secret")
	if !strings.Contains(stdout, "secret/auth.github.test") {
		t.Fatalf("unexpected secret ls output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "read", "secret/auth.github.test")
	if !strings.Contains(stdout, "fixture secret value") {
		t.Fatalf("unexpected secret read output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "write", "memory", "Remember Alpha detail")
	if !strings.Contains(stdout, "Saved memory note.") {
		t.Fatalf("unexpected memory write output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "search", "Alpha", "memory")
	if !strings.Contains(stdout, "Remember Alpha detail") {
		t.Fatalf("unexpected memory search output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "create", "project", "demo")
	if !strings.Contains(stdout, "Created project/demo.") {
		t.Fatalf("unexpected create project output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "log", "project/demo", "--action", "note", "--summary", "First note")
	if !strings.Contains(stdout, "Logged note on project/demo.") {
		t.Fatalf("unexpected project log output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "read", "project/demo")
	if !strings.Contains(stdout, "name: demo") || !strings.Contains(stdout, "First note") {
		t.Fatalf("unexpected project read output: %s", stdout)
	}

	skillDir := filepath.Join(workDir, "demo-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Demo Skill\n\nUse this for testing.\n"), 0o644); err != nil {
		t.Fatalf("write skill doc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "helper.py"), []byte("print('hello')\n"), 0o644); err != nil {
		t.Fatalf("write skill helper: %v", err)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "import", "skill", skillDir)
	if !strings.Contains(stdout, "Imported skill/demo-skill") {
		t.Fatalf("unexpected skill import output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "ls", "skill/demo-skill")
	if !strings.Contains(stdout, "file\tskill/demo-skill/SKILL.md") || !strings.Contains(stdout, "file\tskill/demo-skill/helper.py") {
		t.Fatalf("unexpected skill ls output: %s", stdout)
	}

	profileJSON := filepath.Join(workDir, "profile.json")
	if err := os.WriteFile(profileJSON, []byte(`{"preferences":"Prefer short answers","principles":"Stay factual"}`), 0o644); err != nil {
		t.Fatalf("write profile import file: %v", err)
	}
	stdout, _ = mustRunAgenthub(t, binary, env, "import", "profile", profileJSON)
	if !strings.Contains(stdout, "Imported 2 profile categories.") {
		t.Fatalf("unexpected profile import output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "read", "profile/principles")
	if !strings.Contains(stdout, "Stay factual") {
		t.Fatalf("unexpected principles read output: %s", stdout)
	}

	memoryFile := filepath.Join(workDir, "memory-note.md")
	if err := os.WriteFile(memoryFile, []byte("Imported memory from file"), 0o644); err != nil {
		t.Fatalf("write memory import file: %v", err)
	}
	stdout, _ = mustRunAgenthub(t, binary, env, "import", "memory", memoryFile)
	if !strings.Contains(stdout, "Imported 1 memory item(s).") {
		t.Fatalf("unexpected memory import output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "search", "Imported memory from file", "memory")
	if !strings.Contains(stdout, "Imported memory from file") {
		t.Fatalf("unexpected imported memory search output: %s", stdout)
	}

	projectDir := filepath.Join(workDir, "project-import")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project import dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "notes.md"), []byte("Project import note"), 0o644); err != nil {
		t.Fatalf("write project import note: %v", err)
	}
	stdout, _ = mustRunAgenthub(t, binary, env, "import", "project", projectDir, "--name", "imported")
	if !strings.Contains(stdout, "Imported 1 files into project/imported.") {
		t.Fatalf("unexpected project import output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "read", "project/imported/notes.md")
	if !strings.Contains(stdout, "Project import note") {
		t.Fatalf("unexpected imported project file read: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "token", "create", "--kind", "sync", "--purpose", "backup")
	if !strings.Contains(stdout, "token: ") || !strings.Contains(stdout, "usage: neudrive sync login") {
		t.Fatalf("unexpected sync token output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "token", "create", "--kind", "skills-upload", "--purpose", "skills")
	if !strings.Contains(stdout, "upload_url: ") || !strings.Contains(stdout, "browser_upload_url: ") {
		t.Fatalf("unexpected skills upload token output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "stats")
	for _, expected := range []string{"files:", "memory:", "profile:", "projects:", "skills:"} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("stats output missing %q: %s", expected, stdout)
		}
	}

	mustRunAgenthub(t, binary, env, "daemon", "stop")
}

func seedLocalSecretForHubTest(t *testing.T, apiBase, token, scope, value string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"data":        value,
		"description": "fixture secret",
	})
	if err != nil {
		t.Fatalf("marshal secret request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, strings.TrimRight(apiBase, "/")+"/agent/vault/"+scope, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new secret request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("seed secret request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("seed secret status: %s", resp.Status)
	}
}
