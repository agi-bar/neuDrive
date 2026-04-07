package localruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadConfigRoundTrip(t *testing.T) {
	t.Setenv(ConfigEnv, filepath.Join(t.TempDir(), "config.json"))
	path, cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.CurrentProfile = "official"
	cfg.Profiles["official"] = SyncProfile{APIBase: "https://agenthub.agi.bar", Token: "aht_test"}
	cfg.Local.DatabaseURL = "postgres://agenthub:test@localhost:5432/agenthub?sslmode=disable"
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
}

func TestChoosePortReturnsSavedPortWhenAvailable(t *testing.T) {
	ln, err := os.CreateTemp(t.TempDir(), "port-check")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = ln.Close()

	port, err := choosePort("127.0.0.1:42695")
	if err != nil {
		t.Fatalf("choosePort: %v", err)
	}
	if port < DefaultPortStart || port > DefaultPortEnd {
		t.Fatalf("port out of range: %d", port)
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
	if cfg.Local.JWTSecret == "" || cfg.Local.VaultMasterKey == "" {
		t.Fatal("expected local secrets to be generated")
	}
}
