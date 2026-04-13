package platforms

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agi-bar/agenthub/internal/api"
	"github.com/agi-bar/agenthub/internal/auth"
	"github.com/agi-bar/agenthub/internal/config"
	"github.com/agi-bar/agenthub/internal/localgitsync"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/runtimecfg"
	"github.com/agi-bar/agenthub/internal/services"
	sqlitestorage "github.com/agi-bar/agenthub/internal/storage/sqlite"
	"github.com/agi-bar/agenthub/internal/vault"
	"github.com/google/uuid"
	"net/http/httptest"
)

const heavyPlatformIntegrationEnv = "AGENTHUB_RUN_PLATFORM_INTEGRATION"

func requirePlatformIntegration(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv(heavyPlatformIntegrationEnv)) != "1" {
		t.Skipf("skipping heavy platform integration test; set %s=1 to run", heavyPlatformIntegrationEnv)
	}
}

func configurePlatformTestEnv(t *testing.T) (string, *runtimecfg.CLIConfig, string, string) {
	t.Helper()
	requirePlatformIntegration(t)
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
	serverURL, ownerToken, ownerTokenID := startPlatformTestServer(t, filepath.Join(root, "server.db"))
	cfg := &runtimecfg.CLIConfig{
		Version: 2,
		Local: runtimecfg.LocalConfig{
			Storage:        "postgres",
			SQLitePath:     filepath.Join(root, "unused-client.db"),
			DatabaseURL:    "postgres://local-mode.example/agenthub?sslmode=disable",
			JWTSecret:      strings.Repeat("a", 64),
			VaultMasterKey: strings.Repeat("b", 64),
			PublicBaseURL:  serverURL,
			OwnerToken:     ownerToken,
			OwnerTokenID:   ownerTokenID,
			Connections:    map[string]runtimecfg.LocalConnection{},
		},
		Profiles: map[string]runtimecfg.SyncProfile{},
	}
	if err := runtimecfg.EnsureLocalDefaults(cfg); err != nil {
		t.Fatalf("EnsureLocalDefaults: %v", err)
	}
	return home, cfg, serverURL, logPath
}

func startPlatformTestServer(t *testing.T, dbPath string) (string, string, string) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlitestorage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open test store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	admin, err := store.CreateToken(ctx, user.ID, "admin", []string{models.ScopeAdmin}, models.TrustLevelFull, time.Hour)
	if err != nil {
		t.Fatalf("CreateToken admin: %v", err)
	}

	cfg := &config.Config{
		JWTSecret:      strings.Repeat("a", 64),
		VaultMasterKey: strings.Repeat("b", 64),
		CORSOrigins:    []string{"http://localhost:3000"},
		RateLimit:      100,
		MaxBodySize:    10 * 1024 * 1024,
		PublicBaseURL:  "http://127.0.0.1:0",
	}
	v, err := vault.NewVault(cfg.VaultMasterKey)
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	fileTreeSvc := services.NewFileTreeServiceWithRepo(sqlitestorage.NewFileTreeRepo(store))
	memorySvc := services.NewMemoryServiceWithRepo(sqlitestorage.NewMemoryRepo(store), nil)
	userSvc := services.NewUserServiceWithRepo(sqlitestorage.NewUserRepo(store))
	connSvc := services.NewConnectionServiceWithRepo(sqlitestorage.NewConnectionRepo(store))
	vaultSvc := services.NewVaultServiceWithRepo(sqlitestorage.NewVaultRepo(store), v)
	roleSvc := services.NewRoleServiceWithRepo(sqlitestorage.NewRoleRepo(store), fileTreeSvc)
	inboxSvc := services.NewInboxServiceWithRepo(sqlitestorage.NewInboxRepo(store), fileTreeSvc)
	projectSvc := services.NewProjectServiceWithRepo(sqlitestorage.NewProjectRepo(store), roleSvc, fileTreeSvc)
	tokenSvc := services.NewTokenServiceWithRepo(sqlitestorage.NewTokenRepo(store))
	deviceSvc := services.NewDeviceServiceWithRepo(sqlitestorage.NewDeviceRepo(store), fileTreeSvc)
	importSvc := services.NewImportService(nil, fileTreeSvc, memorySvc, vaultSvc)
	exportSvc := services.NewExportService(fileTreeSvc, memorySvc, projectSvc, vaultSvc, deviceSvc, inboxSvc, roleSvc, userSvc)
	syncSvc := services.NewSyncServiceWithRepo(sqlitestorage.NewSyncRepo(store), importSvc, exportSvc, fileTreeSvc, memorySvc)
	dashboardSvc := services.NewDashboardServiceWithRepo(sqlitestorage.NewDashboardRepo(store))
	localGitSyncSvc := localgitsync.NewWithDeps(store, fileTreeSvc, userSvc, connSvc, projectSvc, vaultSvc)
	tokenGen := func(userID uuid.UUID, slug string) (string, error) {
		return auth.GenerateToken(userID, slug, cfg.JWTSecret)
	}
	authSvc := services.NewAuthServiceWithRepo(sqlitestorage.NewAuthRepo(store), tokenGen, nil)
	oauthSvc := services.NewOAuthServiceWithRepo(sqlitestorage.NewOAuthRepo(store), cfg.JWTSecret)

	server := api.NewServerWithDeps(api.ServerDeps{
		Storage:            "sqlite",
		Config:             cfg,
		LocalOwnerID:       user.ID,
		UserService:        userSvc,
		AuthService:        authSvc,
		ConnectionService:  connSvc,
		FileTreeService:    fileTreeSvc,
		VaultService:       vaultSvc,
		MemoryService:      memorySvc,
		ProjectService:     projectSvc,
		RoleService:        roleSvc,
		InboxService:       inboxSvc,
		DeviceService:      deviceSvc,
		DashboardService:   dashboardSvc,
		TokenService:       tokenSvc,
		ImportService:      importSvc,
		ExportService:      exportSvc,
		SyncService:        syncSvc,
		OAuthService:       oauthSvc,
		LocalGitSync:       localGitSyncSvc,
		Vault:              v,
		JWTSecret:          cfg.JWTSecret,
		GitHubClientID:     cfg.GithubClientID,
		GitHubClientSecret: cfg.GithubClientSecret,
	})

	ts := httptest.NewServer(server.Router)
	t.Cleanup(ts.Close)
	return ts.URL, admin.Token, admin.ScopedToken.ID.String()
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
