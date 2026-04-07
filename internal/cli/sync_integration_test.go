package cli

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agi-bar/agenthub/internal/localruntime"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
)

var (
	agenthubBinaryOnce sync.Once
	agenthubBinaryPath string
	agenthubBinaryErr  error
)

type syncFixturePlan struct {
	SkillNames        []string              `json:"skill_names"`
	ExtraTextFiles    []syncFixtureTextFile `json:"extra_text_files"`
	BinaryAssignments map[string][]string   `json:"binary_assignments"`
}

type syncFixtureTextFile struct {
	Path   string `json:"path"`
	Source string `json:"source"`
	Repeat int    `json:"repeat"`
}

type cliSessionEnvelope struct {
	OK   bool                       `json:"ok"`
	Data models.SyncSessionResponse `json:"data"`
}

func TestAgenthubSyncLoginProfilesWhoAmI_LocalSQLite(t *testing.T) {
	binary := buildAgenthubBinary(t)
	env, configPath, statePath, _, _ := isolatedAgenthubEnv(t)
	mustRunAgenthub(t, binary, env, "sync", "history")
	cfg := loadCLIConfigForTest(t, configPath)
	state := loadRuntimeStateForTest(t, statePath)
	if strings.TrimSpace(cfg.Local.OwnerToken) == "" {
		t.Fatal("expected local owner token after bootstrap")
	}

	stdout, _ := mustRunAgenthub(t, binary, env,
		"sync", "login",
		"--profile", "localtest",
		"--token", cfg.Local.OwnerToken,
		"--api-base", state.APIBase,
	)
	if !strings.Contains(stdout, "Logged in to") {
		t.Fatalf("login output missing success text: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "sync", "profiles")
	if !strings.Contains(stdout, "localtest") {
		t.Fatalf("profiles output missing localtest: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "sync", "whoami", "--profile", "localtest")
	if !strings.Contains(stdout, "Current profile: localtest") {
		t.Fatalf("whoami output missing profile: %s", stdout)
	}
	if !strings.Contains(stdout, "Scopes: admin") {
		t.Fatalf("whoami output missing scopes: %s", stdout)
	}

	mustRunAgenthub(t, binary, env, "sync", "logout", "--profile", "localtest")
	updated := loadCLIConfigForTest(t, configPath)
	if updated.Profiles["localtest"].Token != "" {
		t.Fatal("expected logout to clear saved token")
	}
	mustRunAgenthub(t, binary, env, "daemon", "stop")
}

func TestAgenthubSyncLocalSQLiteRoundTrip_WithRealisticFixture(t *testing.T) {
	binary := buildAgenthubBinary(t)
	env, _, _, _, workDir := isolatedAgenthubEnv(t)
	sourceDir := materializeFixtureSource(t, 2)
	archivePath := filepath.Join(workDir, "fixture.ahubz")
	pulledPath := filepath.Join(workDir, "fixture-pulled.ahubz")

	mustRunAgenthub(t, binary, env, "sync", "export", "--source", sourceDir, "--format", "archive", "-o", archivePath)

	stdout, _ := mustRunAgenthub(t, binary, env, "sync", "preview", "--bundle", archivePath)
	if !strings.Contains(stdout, "\"fingerprint\"") {
		t.Fatalf("preview output missing fingerprint: %s", stdout)
	}

	mustRunAgenthub(t, binary, env, "sync", "push", "--bundle", archivePath, "--transport", "archive")
	mustRunAgenthub(t, binary, env, "sync", "pull", "--format", "archive", "-o", pulledPath)

	stdout, stderr, code := runAgenthub(t, binary, env, "sync", "diff", "--left", archivePath, "--right", pulledPath)
	if code != 0 {
		t.Fatalf("diff exit code = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Equal: yes") {
		t.Fatalf("diff output missing equality marker: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "sync", "history")
	if !strings.Contains(stdout, "\"direction\": \"import\"") || !strings.Contains(stdout, "\"direction\": \"export\"") {
		t.Fatalf("history output missing import/export jobs: %s", stdout)
	}
	mustRunAgenthub(t, binary, env, "daemon", "stop")
}

func TestAgenthubSyncResume_LocalSQLiteArchiveSession(t *testing.T) {
	binary := buildAgenthubBinary(t)
	env, configPath, statePath, _, workDir := isolatedAgenthubEnv(t)
	sourceDir := materializeFixtureSource(t, 12)
	archivePath := filepath.Join(workDir, "large.ahubz")
	pulledPath := filepath.Join(workDir, "large-pulled.ahubz")

	mustRunAgenthub(t, binary, env, "sync", "export", "--source", sourceDir, "--format", "archive", "-o", archivePath)
	mustRunAgenthub(t, binary, env, "sync", "history")

	cfg := loadCLIConfigForTest(t, configPath)
	state := loadRuntimeStateForTest(t, statePath)
	if strings.TrimSpace(cfg.Local.OwnerToken) == "" {
		t.Fatal("expected local owner token after history bootstrap")
	}

	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	_, manifest, err := services.ParseBundleArchive(archiveBytes)
	if err != nil {
		t.Fatalf("parse archive: %v", err)
	}
	started := startSyncSessionForTest(t, state.APIBase, cfg.Local.OwnerToken, archiveBytes, manifest)
	partSize := int(started.ChunkSizeBytes)
	if partSize <= 0 {
		t.Fatalf("invalid chunk size: %d", started.ChunkSizeBytes)
	}
	uploadSyncPartForTest(t, state.APIBase, cfg.Local.OwnerToken, started.SessionID.String(), 0, archiveBytes[:partSize])

	sessionFile := archivePath + ".session.json"
	sessionState := map[string]any{
		"api_base":            state.APIBase,
		"bundle_path":         archivePath,
		"session_id":          started.SessionID.String(),
		"preview_fingerprint": "",
		"profile":             "",
		"created_at":          time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(sessionState, "", "  ")
	if err != nil {
		t.Fatalf("marshal session state: %v", err)
	}
	if err := os.WriteFile(sessionFile, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write session state: %v", err)
	}

	mustRunAgenthub(t, binary, env, "sync", "resume", "--bundle", archivePath)
	mustRunAgenthub(t, binary, env, "sync", "pull", "--format", "archive", "-o", pulledPath)

	stdout, stderr, code := runAgenthub(t, binary, env, "sync", "diff", "--left", archivePath, "--right", pulledPath)
	if code != 0 {
		t.Fatalf("diff exit code = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Equal: yes") {
		t.Fatalf("diff output missing equality marker: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "sync", "history")
	if !strings.Contains(stdout, "\"transport\": \"archive\"") {
		t.Fatalf("history output missing archive jobs: %s", stdout)
	}
	mustRunAgenthub(t, binary, env, "daemon", "stop")
}

func TestAgenthubRemoteCommands_LocalSQLiteProfile(t *testing.T) {
	binary := buildAgenthubBinary(t)
	env, configPath, statePath, _, _ := isolatedAgenthubEnv(t)

	mustRunAgenthub(t, binary, env, "sync", "history")
	cfg := loadCLIConfigForTest(t, configPath)
	state := loadRuntimeStateForTest(t, statePath)
	if strings.TrimSpace(cfg.Local.OwnerToken) == "" {
		t.Fatal("expected local owner token after bootstrap")
	}

	stdout, _ := mustRunAgenthub(t, binary, env,
		"remote", "login", "local",
		"--url", state.APIBase,
		"--token", cfg.Local.OwnerToken,
	)
	if !strings.Contains(stdout, "Logged in to") {
		t.Fatalf("unexpected remote login output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "remote", "ls")
	if !strings.Contains(stdout, "* local\t"+state.APIBase) {
		t.Fatalf("unexpected remote ls output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "remote", "use", "local")
	if !strings.Contains(stdout, "Current profile: local") {
		t.Fatalf("unexpected remote use output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "remote", "whoami", "local")
	if !strings.Contains(stdout, "Current profile: local") || !strings.Contains(stdout, "Auth mode: scoped_token") {
		t.Fatalf("unexpected remote whoami output: %s", stdout)
	}

	mustRunAgenthub(t, binary, env, "remote", "logout", "local")
	updated := loadCLIConfigForTest(t, configPath)
	if updated.Profiles["local"].Token != "" {
		t.Fatal("expected remote logout to clear saved token")
	}
	mustRunAgenthub(t, binary, env, "daemon", "stop")
}

func buildAgenthubBinary(t *testing.T) string {
	t.Helper()
	agenthubBinaryOnce.Do(func() {
		root := repoRoot(t)
		binDir, err := os.MkdirTemp("", "agenthub-cli-bin-")
		if err != nil {
			agenthubBinaryErr = err
			return
		}
		agenthubBinaryPath = filepath.Join(binDir, "agenthub")
		cmd := exec.Command("go", "build", "-o", agenthubBinaryPath, "./cmd/agenthub")
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if err != nil {
			agenthubBinaryErr = fmt.Errorf("go build failed: %w\n%s", err, string(output))
			return
		}
	})
	if agenthubBinaryErr != nil {
		t.Fatal(agenthubBinaryErr)
	}
	return agenthubBinaryPath
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func isolatedAgenthubEnv(t *testing.T) ([]string, string, string, string, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	configHome := filepath.Join(root, "config")
	stateHome := filepath.Join(root, "state")
	dataHome := filepath.Join(root, "data")
	workDir := filepath.Join(root, "work")
	for _, dir := range []string{home, configHome, stateHome, dataHome, workDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	env := append([]string{}, os.Environ()...)
	configPath := filepath.Join(configHome, "agenthub", "config.json")
	statePath := filepath.Join(configHome, "agenthub", "runtime.json")
	env = appendOrReplaceEnv(env, "HOME", home)
	env = appendOrReplaceEnv(env, "XDG_CONFIG_HOME", configHome)
	env = appendOrReplaceEnv(env, "XDG_STATE_HOME", stateHome)
	env = appendOrReplaceEnv(env, "XDG_DATA_HOME", dataHome)
	env = appendOrReplaceEnv(env, "AGENTHUB_CONFIG", configPath)
	for _, key := range []string{
		"AGENTHUB_TOKEN",
		"AGENTHUB_SYNC_TOKEN",
		"AGENTHUB_API_BASE",
		"AGENTHUB_SYNC_API_BASE",
		"AGENTHUB_SYNC_PROFILE",
	} {
		env = appendOrReplaceEnv(env, key, "")
	}
	sqlitePath := filepath.Join(dataHome, "agenthub", "local.db")
	return env, configPath, statePath, sqlitePath, workDir
}

func runAgenthub(t *testing.T, binary string, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = env
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.String(), stderr.String(), exitErr.ExitCode()
	}
	t.Fatalf("run %v: %v", args, err)
	return "", "", 1
}

func mustRunAgenthub(t *testing.T, binary string, env []string, args ...string) (string, string) {
	t.Helper()
	stdout, stderr, code := runAgenthub(t, binary, env, args...)
	if code != 0 {
		t.Fatalf("agenthub %v exit=%d\nstdout:\n%s\nstderr:\n%s", args, code, stdout, stderr)
	}
	return stdout, stderr
}

func loadCLIConfigForTest(t *testing.T, path string) *localruntime.CLIConfig {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config %s: %v", path, err)
	}
	var cfg localruntime.CLIConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("decode config %s: %v", path, err)
	}
	return &cfg
}

func loadRuntimeStateForTest(t *testing.T, path string) *localruntime.RuntimeState {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime state %s: %v", path, err)
	}
	var state localruntime.RuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("decode runtime state %s: %v", path, err)
	}
	return &state
}

func startSyncSessionForTest(t *testing.T, apiBase, token string, archive []byte, manifest *models.BundleArchiveManifest) models.SyncSessionResponse {
	t.Helper()
	reqBody, err := json.Marshal(models.SyncStartSessionRequest{
		TransportVersion: models.SyncTransportVersionV1,
		Format:           models.BundleFormatArchive,
		Mode:             manifest.Mode,
		Manifest:         *manifest,
		ArchiveSizeBytes: int64(len(archive)),
		ArchiveSHA256:    manifest.ArchiveSHA256,
	})
	if err != nil {
		t.Fatalf("marshal start session: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(apiBase, "/")+"/agent/import/session", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("new start session request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("start session request: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read start session response: %v", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("start session status=%s body=%s", resp.Status, string(body))
	}
	var envelope cliSessionEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode start session response: %v", err)
	}
	return envelope.Data
}

func uploadSyncPartForTest(t *testing.T, apiBase, token, sessionID string, index int, payload []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, strings.TrimRight(apiBase, "/")+"/agent/import/session/"+sessionID+"/parts/"+strconv.Itoa(index), bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new upload part request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload part request: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read upload part response: %v", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("upload part status=%s body=%s", resp.Status, string(body))
	}
}

func loadRealisticSkillFixture(t *testing.T) map[string]string {
	t.Helper()
	reader, err := zip.OpenReader(filepath.Join(repoRoot(t), "internal", "services", "testdata", "ahub-sync.skill"))
	if err != nil {
		t.Fatalf("open skill fixture: %v", err)
	}
	defer reader.Close()
	files := make(map[string]string)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		relPath := strings.TrimPrefix(file.Name, "pkg-skill/")
		if relPath == file.Name || relPath == "" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open fixture entry %s: %v", file.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read fixture entry %s: %v", file.Name, err)
		}
		files[relPath] = string(data)
	}
	return files
}

func readRealisticBinaryFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "internal", "services", "testdata", "tiny.png"))
	if err != nil {
		t.Fatalf("read binary fixture: %v", err)
	}
	return data
}

func loadSyncFixturePlan(t *testing.T) syncFixturePlan {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "internal", "services", "testdata", "sync-fixture-plan.json"))
	if err != nil {
		t.Fatalf("read sync fixture plan: %v", err)
	}
	var plan syncFixturePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatalf("decode sync fixture plan: %v", err)
	}
	return plan
}

func materializeFixtureSource(t *testing.T, multiplier int) string {
	t.Helper()
	if multiplier <= 0 {
		multiplier = 1
	}
	baseFiles := loadRealisticSkillFixture(t)
	binary := readRealisticBinaryFixture(t)
	plan := loadSyncFixturePlan(t)
	root := t.TempDir()
	for _, skillName := range plan.SkillNames {
		skillRoot := filepath.Join(root, skillName)
		if err := os.MkdirAll(skillRoot, 0o755); err != nil {
			t.Fatalf("create skill root: %v", err)
		}
		for relPath, content := range baseFiles {
			target := filepath.Join(skillRoot, filepath.FromSlash(relPath))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("mkdir parent: %v", err)
			}
			if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
				t.Fatalf("write base file: %v", err)
			}
		}
		for _, extra := range plan.ExtraTextFiles {
			sourceContent := baseFiles[extra.Source]
			target := filepath.Join(skillRoot, filepath.FromSlash(extra.Path))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("mkdir extra parent: %v", err)
			}
			payload := strings.Repeat(sourceContent+"\n", extra.Repeat*multiplier)
			if err := os.WriteFile(target, []byte(payload), 0o644); err != nil {
				t.Fatalf("write extra file: %v", err)
			}
		}
		for _, relPath := range plan.BinaryAssignments[skillName] {
			target := filepath.Join(skillRoot, filepath.FromSlash(relPath))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("mkdir binary parent: %v", err)
			}
			if err := os.WriteFile(target, expandedBinaryFixture(binary, skillName+":"+relPath, multiplier), 0o644); err != nil {
				t.Fatalf("write binary file: %v", err)
			}
		}
	}
	return root
}

func expandedBinaryFixture(base []byte, seed string, multiplier int) []byte {
	if multiplier <= 0 {
		multiplier = 1
	}
	targetSize := len(base) + multiplier*(256<<10)
	payload := make([]byte, 0, targetSize)
	payload = append(payload, base...)
	counter := 0
	for len(payload) < targetSize {
		sum := sha256.Sum256([]byte(seed + ":" + strconv.Itoa(counter)))
		payload = append(payload, sum[:]...)
		counter++
	}
	return payload[:targetSize]
}
