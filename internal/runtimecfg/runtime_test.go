package runtimecfg

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndLoadConfigRoundTrip(t *testing.T) {
	t.Setenv(ConfigEnv, filepath.Join(t.TempDir(), "config.json"))
	path, cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.CurrentProfile = "official"
	cfg.Profiles["official"] = SyncProfile{APIBase: "https://neudrive.ai", Token: "ndt_test"}
	cfg.Local.DatabaseURL = "postgres://neudrive:test@localhost:5432/neudrive?sslmode=disable"
	cfg.Local.GitMirrorPath = "~/neudrive-mirror"
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	loadedPath, loaded, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig (2): %v", err)
	}
	if loadedPath != path {
		t.Fatalf("path mismatch: got %q want %q", loadedPath, path)
	}
	if loaded.CurrentProfile != "official" {
		t.Fatalf("current_profile mismatch: got %q", loaded.CurrentProfile)
	}
	if loaded.Local.DatabaseURL == "" {
		t.Fatal("expected local database url to round-trip")
	}
	if loaded.Local.GitMirrorPath != "~/neudrive-mirror" {
		t.Fatalf("git_mirror_path mismatch: got %q", loaded.Local.GitMirrorPath)
	}
}

func TestChoosePortReturnsSavedPortWhenAvailable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	chosen, err := choosePort(fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("choosePort: %v", err)
	}
	if chosen != port {
		t.Fatalf("expected saved port %d, got %d", port, chosen)
	}
}

func TestChooseEphemeralPortReturnsUsablePort(t *testing.T) {
	port, err := chooseEphemeralPort()
	if err != nil {
		t.Fatalf("chooseEphemeralPort: %v", err)
	}
	if port <= 0 {
		t.Fatalf("expected positive port, got %d", port)
	}
}

func TestEnsureLocalDefaultsPrefersSQLite(t *testing.T) {
	cfg := &CLIConfig{}
	if err := EnsureLocalDefaults(cfg); err != nil {
		t.Fatalf("EnsureLocalDefaults: %v", err)
	}
	if cfg.Local.Storage != DefaultStorage {
		t.Fatalf("storage mismatch: got %q want %q", cfg.Local.Storage, DefaultStorage)
	}
	if cfg.Local.SQLitePath == "" {
		t.Fatal("expected sqlite path to be populated")
	}
	if cfg.Local.DatabaseURL != "" {
		t.Fatalf("expected sqlite defaults to leave database URL empty, got %q", cfg.Local.DatabaseURL)
	}
	if cfg.Local.GitMirrorPath != DefaultGitMirrorPath {
		t.Fatalf("expected git mirror path default %q, got %q", DefaultGitMirrorPath, cfg.Local.GitMirrorPath)
	}
	if cfg.Local.JWTSecret == "" || cfg.Local.VaultMasterKey == "" {
		t.Fatal("expected local secrets to be generated")
	}
}

func TestReadFileWithLegacyFallbackUsesLegacyWhenDefaultMissing(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "config.json")
	legacyPath := filepath.Join(tempDir, "legacy-config.json")
	if err := os.WriteFile(legacyPath, []byte(`{"version":2}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := readFileWithLegacyFallback(path, legacyPath)
	if err != nil {
		t.Fatalf("readFileWithLegacyFallback: %v", err)
	}
	if got := string(data); got != `{"version":2}` {
		t.Fatalf("unexpected data: %q", got)
	}
}

func TestReadFileWithLegacyFallbackPrefersDefaultPath(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "config.json")
	legacyPath := filepath.Join(tempDir, "legacy-config.json")
	if err := os.WriteFile(path, []byte(`{"version":3}`), 0o600); err != nil {
		t.Fatalf("WriteFile(default): %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"version":2}`), 0o600); err != nil {
		t.Fatalf("WriteFile(legacy): %v", err)
	}
	data, err := readFileWithLegacyFallback(path, legacyPath)
	if err != nil {
		t.Fatalf("readFileWithLegacyFallback: %v", err)
	}
	if got := string(data); got != `{"version":3}` {
		t.Fatalf("unexpected data: %q", got)
	}
}

func TestLoadRawConfigReturnsDefaultObjectWhenMissing(t *testing.T) {
	t.Setenv(ConfigEnv, filepath.Join(t.TempDir(), "config.json"))

	path, raw, err := LoadRawConfig("")
	if err != nil {
		t.Fatalf("LoadRawConfig: %v", err)
	}
	if path == "" {
		t.Fatal("expected config path")
	}
	for _, expected := range []string{`"version": 2`, `"local": {}`} {
		if !strings.Contains(raw, expected) {
			t.Fatalf("expected %q in raw config: %s", expected, raw)
		}
	}
}

func TestSaveRawConfigNormalizesAndPersistsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	raw := "{\n  \"version\": 2,\n  \"local\": {\n    \"listen_addr\": \"127.0.0.1:42690\"\n  },\n  \"extra\": {\"keep\": true}\n}"
	if err := SaveRawConfig(path, raw); err != nil {
		t.Fatalf("SaveRawConfig: %v", err)
	}

	_, loadedRaw, err := LoadRawConfig(path)
	if err != nil {
		t.Fatalf("LoadRawConfig: %v", err)
	}
	if !strings.Contains(loadedRaw, "\"extra\": {\n    \"keep\": true\n  }") {
		t.Fatalf("expected unknown fields to persist, got: %s", loadedRaw)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != loadedRaw {
		t.Fatalf("persisted file mismatch\nfile=%q\nraw=%q", string(data), loadedRaw)
	}
}
