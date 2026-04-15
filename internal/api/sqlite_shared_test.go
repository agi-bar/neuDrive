package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agi-bar/neudrive/internal/auth"
	"github.com/agi-bar/neudrive/internal/config"
	"github.com/agi-bar/neudrive/internal/localgitsync"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/runtimecfg"
	"github.com/agi-bar/neudrive/internal/services"
	sqlitestorage "github.com/agi-bar/neudrive/internal/storage/sqlite"
	"github.com/agi-bar/neudrive/internal/vault"
	"github.com/google/uuid"
)

type testEnvelope struct {
	OK           bool            `json:"ok"`
	Data         json.RawMessage `json:"data"`
	LocalGitSync json.RawMessage `json:"local_git_sync,omitempty"`
	Error        struct {
		Message string `json:"message"`
	} `json:"error"`
}

type fakeGitHubTokenState struct {
	login      string
	fullName   string
	permission string
}

func newTestHTTPServer(t *testing.T, gitOpts ...localgitsync.Option) (*httptest.Server, *sqlitestorage.Store, string, string, string) {
	cfg := &config.Config{
		JWTSecret:            testJWTSecret,
		VaultMasterKey:       strings.Repeat("0", 64),
		CORSOrigins:          []string{"http://localhost:3000"},
		RateLimit:            100,
		MaxBodySize:          10 * 1024 * 1024,
		PublicBaseURL:        "http://127.0.0.1:0",
		EnableSystemSettings: true,
	}
	return newTestHTTPServerWithConfig(t, cfg, gitOpts...)
}

func newTestHTTPServerWithConfig(t *testing.T, cfg *config.Config, gitOpts ...localgitsync.Option) (*httptest.Server, *sqlitestorage.Store, string, string, string) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlitestorage.Open(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
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
	readBundle, err := store.CreateToken(ctx, user.ID, "read", []string{models.ScopeReadBundle}, models.TrustLevelWork, time.Hour)
	if err != nil {
		t.Fatalf("CreateToken read: %v", err)
	}
	writeBundle, err := store.CreateToken(ctx, user.ID, "write", []string{models.ScopeWriteBundle}, models.TrustLevelWork, time.Hour)
	if err != nil {
		t.Fatalf("CreateToken write: %v", err)
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
	localGitSyncSvc := localgitsync.New(store, v, gitOpts...)
	tokenGen := func(userID uuid.UUID, slug string) (string, error) {
		return auth.GenerateToken(userID, slug, cfg.JWTSecret)
	}
	authSvc := services.NewAuthServiceWithRepo(sqlitestorage.NewAuthRepo(store), tokenGen, nil)
	oauthSvc := services.NewOAuthServiceWithRepo(sqlitestorage.NewOAuthRepo(store), cfg.JWTSecret)

	s := NewServerWithDeps(ServerDeps{
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
	ts := httptest.NewServer(s.Router)
	t.Cleanup(ts.Close)
	return ts, store, admin.Token, readBundle.Token, writeBundle.Token
}

func newHostedTestHTTPServerWithConfig(t *testing.T, cfg *config.Config, gitOpts ...localgitsync.Option) (*httptest.Server, *sqlitestorage.Store, string) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlitestorage.Open(filepath.Join(t.TempDir(), "hosted.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
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
	hostedRoot := filepath.Join(t.TempDir(), "hosted-root")
	hostedGitOpts := append([]localgitsync.Option{
		localgitsync.WithExecutionMode(localgitsync.ExecutionModeHosted),
		localgitsync.WithHostedRoot(hostedRoot),
		localgitsync.WithGitMirrorPublicBaseURL("http://127.0.0.1:0"),
		localgitsync.WithStateSigningSecret(cfg.JWTSecret),
	}, gitOpts...)
	localGitSyncSvc := localgitsync.New(store, v, hostedGitOpts...)
	tokenGen := func(userID uuid.UUID, slug string) (string, error) {
		return auth.GenerateToken(userID, slug, cfg.JWTSecret)
	}
	authSvc := services.NewAuthServiceWithRepo(sqlitestorage.NewAuthRepo(store), tokenGen, nil)
	oauthSvc := services.NewOAuthServiceWithRepo(sqlitestorage.NewOAuthRepo(store), cfg.JWTSecret)

	s := NewServerWithDeps(ServerDeps{
		Storage:               "sqlite",
		Config:                cfg,
		UserService:           userSvc,
		AuthService:           authSvc,
		ConnectionService:     connSvc,
		FileTreeService:       fileTreeSvc,
		VaultService:          vaultSvc,
		MemoryService:         memorySvc,
		ProjectService:        projectSvc,
		RoleService:           roleSvc,
		InboxService:          inboxSvc,
		DeviceService:         deviceSvc,
		DashboardService:      dashboardSvc,
		TokenService:          tokenSvc,
		ImportService:         importSvc,
		ExportService:         exportSvc,
		SyncService:           syncSvc,
		OAuthService:          oauthSvc,
		LocalGitSync:          localGitSyncSvc,
		Vault:                 v,
		JWTSecret:             cfg.JWTSecret,
		GitHubClientID:        cfg.GithubClientID,
		GitHubClientSecret:    cfg.GithubClientSecret,
		GitHubAppClientID:     cfg.GitHubAppClientID,
		GitHubAppClientSecret: cfg.GitHubAppClientSecret,
		GitHubAppSlug:         cfg.GitHubAppSlug,
	})
	ts := httptest.NewServer(s.Router)
	t.Cleanup(ts.Close)
	return ts, store, admin.Token
}

func TestSQLiteSharedServerHealthAndAuth(t *testing.T) {
	ts, _, _, _, _ := newTestHTTPServer(t)

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", resp.StatusCode)
	}
	var health testEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if !health.OK || !bytes.Contains(health.Data, []byte(`"storage":"sqlite"`)) {
		t.Fatalf("unexpected health payload: %+v", health)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/login", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	authResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/auth/login: %v", err)
	}
	defer authResp.Body.Close()
	if authResp.StatusCode == http.StatusNotImplemented {
		t.Fatalf("expected shared auth route, got %d", authResp.StatusCode)
	}
	if authResp.StatusCode != http.StatusUnauthorized && authResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected auth status = %d", authResp.StatusCode)
	}

	rootResp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer rootResp.Body.Close()
	if rootResp.StatusCode != http.StatusOK {
		t.Fatalf("root status = %d", rootResp.StatusCode)
	}
	var rootBody bytes.Buffer
	if _, err := rootBody.ReadFrom(rootResp.Body); err != nil {
		t.Fatalf("read root body: %v", err)
	}
	if !strings.Contains(rootBody.String(), "<!doctype html>") && !strings.Contains(strings.ToLower(rootBody.String()), "<html") {
		t.Fatalf("expected embedded frontend HTML, got %q", rootBody.String())
	}
}

func TestPublicConfigUsesLocalModeInsteadOfStorageBackend(t *testing.T) {
	s := NewServerWithDeps(ServerDeps{
		Storage:      "postgres",
		LocalOwnerID: uuid.New(),
		Config: &config.Config{
			CORSOrigins:          []string{"http://localhost:3000"},
			RateLimit:            100,
			MaxBodySize:          10 * 1024 * 1024,
			EnableSystemSettings: true,
		},
	})
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/config")
	if err != nil {
		t.Fatalf("GET /api/config: %v", err)
	}
	defer resp.Body.Close()

	var payload testEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	for _, expected := range []string{`"storage":"postgres"`, `"local_mode":true`, `"system_settings_enabled":true`} {
		if !bytes.Contains(payload.Data, []byte(expected)) {
			t.Fatalf("expected %q in config payload: %s", expected, string(payload.Data))
		}
	}
}

func TestSQLiteSharedServerSystemSettingsDisabledBlocksLocalSettingsAPI(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:            testJWTSecret,
		VaultMasterKey:       strings.Repeat("0", 64),
		CORSOrigins:          []string{"http://localhost:3000"},
		RateLimit:            100,
		MaxBodySize:          10 * 1024 * 1024,
		PublicBaseURL:        "http://127.0.0.1:0",
		EnableSystemSettings: false,
	}
	ts, _, adminToken, _, _ := newTestHTTPServerWithConfig(t, cfg)

	status, blockedConfig := doJSON(t, http.MethodGet, ts.URL+"/api/local/config", adminToken, nil)
	if status != http.StatusForbidden || blockedConfig.OK {
		t.Fatalf("GET /api/local/config should be forbidden when system settings disabled: status=%d body=%+v", status, blockedConfig)
	}

	status, blockedMirror := doJSON(t, http.MethodGet, ts.URL+"/api/local/git-mirror", adminToken, nil)
	if status != http.StatusForbidden || blockedMirror.OK {
		t.Fatalf("GET /api/local/git-mirror should be forbidden when system settings disabled: status=%d body=%+v", status, blockedMirror)
	}

	resp, err := http.Get(ts.URL + "/api/config")
	if err != nil {
		t.Fatalf("GET /api/config: %v", err)
	}
	defer resp.Body.Close()
	var payload testEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode config payload: %v", err)
	}
	if !bytes.Contains(payload.Data, []byte(`"system_settings_enabled":false`)) {
		t.Fatalf("expected disabled system settings in public config: %s", string(payload.Data))
	}
}

func TestHostedGitMirrorAPIRemainsAvailableWhenSystemSettingsDisabled(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:            testJWTSecret,
		VaultMasterKey:       strings.Repeat("0", 64),
		CORSOrigins:          []string{"http://localhost:3000"},
		RateLimit:            100,
		MaxBodySize:          10 * 1024 * 1024,
		PublicBaseURL:        "http://127.0.0.1:0",
		EnableSystemSettings: false,
	}
	ts, _, adminToken := newHostedTestHTTPServerWithConfig(t, cfg)

	status, mirror := doJSON(t, http.MethodGet, ts.URL+"/api/git-mirror", adminToken, nil)
	if status != http.StatusOK || !mirror.OK {
		t.Fatalf("GET /api/git-mirror failed in hosted mode: status=%d body=%+v", status, mirror)
	}
	if !bytes.Contains(mirror.Data, []byte(`"execution_mode":"hosted"`)) {
		t.Fatalf("expected hosted execution mode in git mirror payload: %s", string(mirror.Data))
	}

	status, updated := doJSON(t, http.MethodPut, ts.URL+"/api/git-mirror", adminToken, []byte(`{
		"auto_commit_enabled": true,
		"auto_push_enabled": false,
		"auth_mode": "local_credentials",
		"remote_name": "origin",
		"remote_url": "https://github.com/acme/demo.git",
		"remote_branch": "main"
	}`))
	if status != http.StatusBadRequest || updated.OK {
		t.Fatalf("expected hosted local_credentials update to fail: status=%d body=%+v", status, updated)
	}
}

func TestSQLiteSharedServerRegisterLoginRefresh(t *testing.T) {
	ts, _, _, _, _ := newTestHTTPServer(t)

	registerBody := []byte(`{"email":"new@example.com","password":"hunter22","display_name":"New User","slug":"new-user"}`)
	registerReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/register", bytes.NewReader(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerResp, err := http.DefaultClient.Do(registerReq)
	if err != nil {
		t.Fatalf("POST /api/auth/register: %v", err)
	}
	defer registerResp.Body.Close()
	if registerResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(registerResp.Body)
		t.Fatalf("register status = %d body=%s", registerResp.StatusCode, string(body))
	}
	var registered models.AuthResponse
	if err := json.NewDecoder(registerResp.Body).Decode(&registered); err != nil {
		t.Fatalf("decode register: %v", err)
	}
	if registered.AccessToken == "" || registered.RefreshToken == "" {
		t.Fatalf("expected auth tokens in register response: %+v", registered)
	}

	meReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+registered.AccessToken)
	meResp, err := http.DefaultClient.Do(meReq)
	if err != nil {
		t.Fatalf("GET /api/auth/me: %v", err)
	}
	defer meResp.Body.Close()
	if meResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(meResp.Body)
		t.Fatalf("auth me status = %d body=%s", meResp.StatusCode, string(body))
	}
	var me testEnvelope
	if err := json.NewDecoder(meResp.Body).Decode(&me); err != nil {
		t.Fatalf("decode auth me: %v", err)
	}
	if !bytes.Contains(me.Data, []byte(`"slug":"new-user"`)) {
		t.Fatalf("unexpected auth me payload: %s", string(me.Data))
	}

	refreshReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/refresh", bytes.NewReader([]byte(`{"refresh_token":"`+registered.RefreshToken+`"}`)))
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshResp, err := http.DefaultClient.Do(refreshReq)
	if err != nil {
		t.Fatalf("POST /api/auth/refresh: %v", err)
	}
	defer refreshResp.Body.Close()
	if refreshResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(refreshResp.Body)
		t.Fatalf("refresh status = %d body=%s", refreshResp.StatusCode, string(body))
	}
	var refreshed models.AuthResponse
	if err := json.NewDecoder(refreshResp.Body).Decode(&refreshed); err != nil {
		t.Fatalf("decode refresh: %v", err)
	}
	if refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		t.Fatalf("expected auth tokens in refresh response: %+v", refreshed)
	}
}

func TestSQLiteSharedServerSkillsListDashboardRoute(t *testing.T) {
	ts, store, _, _, _ := newTestHTTPServer(t)
	ctx := context.Background()

	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	if _, err := store.WriteEntry(ctx, user.ID, "/skills/demo-bundle/SKILL.md", "---\nname: Demo Bundle\ndescription: Dashboard skill bundle test\n---\n# Demo Bundle\n", "text/markdown", models.FileTreeWriteOptions{}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}

	jwt, err := auth.GenerateToken(user.ID, user.Slug, testJWTSecret)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/skills", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/skills: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("skills status = %d body=%s", resp.StatusCode, string(body))
	}

	var payload testEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode skills payload: %v", err)
	}
	if !payload.OK {
		t.Fatalf("expected ok envelope: %+v", payload)
	}
	if !bytes.Contains(payload.Data, []byte(`"/skills/demo-bundle/SKILL.md"`)) {
		t.Fatalf("expected custom skill in payload: %s", string(payload.Data))
	}
	if !bytes.Contains(payload.Data, []byte(`"source":"system"`)) {
		t.Fatalf("expected system skills in payload: %s", string(payload.Data))
	}
}

func TestSQLiteSharedServerScopeGatingAndSyncFlow(t *testing.T) {
	ts, _, adminToken, readBundleToken, writeBundleToken := newTestHTTPServer(t)

	bundle := models.Bundle{
		Version:   models.BundleVersionV1,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Source:    "test",
		Mode:      "merge",
		Skills: map[string]models.BundleSkill{
			"demo": {
				Files: map[string]string{
					"SKILL.md": "# Demo\n",
				},
			},
		},
	}
	bundleBody, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("Marshal bundle: %v", err)
	}

	if status, body := doJSON(t, http.MethodPost, ts.URL+"/agent/import/bundle", readBundleToken, bundleBody); status != http.StatusForbidden || body.OK {
		t.Fatalf("read bundle token should not import: status=%d body=%+v", status, body)
	}
	if status, body := doJSON(t, http.MethodGet, ts.URL+"/agent/export/bundle", writeBundleToken, nil); status != http.StatusForbidden || body.OK {
		t.Fatalf("write bundle token should not export: status=%d body=%+v", status, body)
	}
	if status, body := doJSON(t, http.MethodPost, ts.URL+"/api/tokens/sync", writeBundleToken, []byte(`{"access":"both","ttl_minutes":30}`)); status != http.StatusForbidden || body.OK {
		t.Fatalf("write bundle token should not mint sync token: status=%d body=%+v", status, body)
	}

	status, preview := doJSON(t, http.MethodPost, ts.URL+"/agent/import/preview", adminToken, bundleBody)
	if status != http.StatusOK || !preview.OK {
		t.Fatalf("preview failed: status=%d body=%+v", status, preview)
	}
	status, imported := doJSON(t, http.MethodPost, ts.URL+"/agent/import/bundle", adminToken, bundleBody)
	if status != http.StatusOK || !imported.OK {
		t.Fatalf("import failed: status=%d body=%+v", status, imported)
	}
	if !bytes.Contains(imported.Data, []byte(`"skills_written":1`)) {
		t.Fatalf("unexpected import payload: %s", string(imported.Data))
	}

	status, exported := doJSON(t, http.MethodGet, ts.URL+"/agent/export/bundle", adminToken, nil)
	if status != http.StatusOK || !exported.OK {
		t.Fatalf("export failed: status=%d body=%+v", status, exported)
	}
	if !bytes.Contains(exported.Data, []byte(`"demo"`)) {
		t.Fatalf("unexpected export payload: %s", string(exported.Data))
	}

	status, syncToken := doJSON(t, http.MethodPost, ts.URL+"/api/tokens/sync", adminToken, []byte(`{"access":"both","ttl_minutes":30}`))
	if status != http.StatusCreated || !syncToken.OK {
		t.Fatalf("sync token creation failed: status=%d body=%+v", status, syncToken)
	}
	if !bytes.Contains(syncToken.Data, []byte(`"api_base":"`+ts.URL)) {
		t.Fatalf("unexpected sync token payload: %s", string(syncToken.Data))
	}
}

func TestSQLiteSharedServerWebDashboardEndpoints(t *testing.T) {
	ts, store, adminToken, _, _ := newTestHTTPServer(t)
	ctx := context.Background()
	userID, err := store.FirstUserID(ctx)
	if err != nil {
		t.Fatalf("FirstUserID: %v", err)
	}
	if err := store.UpsertProfile(ctx, userID, "preferences", "Keep responses concise.", "test"); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}
	if _, err := store.WriteScratchWithTitle(ctx, userID, "Scratch memory", "test", "scratch"); err != nil {
		t.Fatalf("WriteScratchWithTitle: %v", err)
	}
	if _, err := store.CreateProject(ctx, userID, "local-dashboard"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	status, me := doJSON(t, http.MethodGet, ts.URL+"/api/auth/me", adminToken, nil)
	if status != http.StatusOK || !me.OK {
		t.Fatalf("GET /api/auth/me failed: status=%d body=%+v", status, me)
	}
	if !bytes.Contains(me.Data, []byte(`"display_name":"Local Owner"`)) {
		t.Fatalf("unexpected auth me payload: %s", string(me.Data))
	}

	status, profile := doJSON(t, http.MethodGet, ts.URL+"/api/memory/profile", adminToken, nil)
	if status != http.StatusOK || !profile.OK {
		t.Fatalf("GET /api/memory/profile failed: status=%d body=%+v", status, profile)
	}
	if !bytes.Contains(profile.Data, []byte(`"preferences":{"preferences":"Keep responses concise."}`)) {
		t.Fatalf("unexpected profile payload: %s", string(profile.Data))
	}

	status, stats := doJSON(t, http.MethodGet, ts.URL+"/api/dashboard/stats", adminToken, nil)
	if status != http.StatusOK || !stats.OK {
		t.Fatalf("GET /api/dashboard/stats failed: status=%d body=%+v", status, stats)
	}
	for _, expected := range []string{`"files":`, `"memory":1`, `"profile":1`, `"projects":1`, `"skills":`} {
		if !bytes.Contains(stats.Data, []byte(expected)) {
			t.Fatalf("expected %q in stats payload: %s", expected, string(stats.Data))
		}
	}

	status, conflicts := doJSON(t, http.MethodGet, ts.URL+"/api/memory/conflicts", adminToken, nil)
	if status != http.StatusOK || !conflicts.OK || !bytes.Contains(conflicts.Data, []byte(`"conflicts":[]`)) {
		t.Fatalf("unexpected conflicts payload: status=%d body=%+v", status, conflicts)
	}
}

func TestSQLiteSharedServerWriteResponsesIncludeLocalGitSync(t *testing.T) {
	ts, store, adminToken, _, _ := newTestHTTPServer(t)
	ctx := context.Background()

	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	v, err := vault.NewVault(strings.Repeat("0", 64))
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	mirrorDir := filepath.Join(t.TempDir(), "mirror")
	mirrorSvc := localgitsync.New(store, v)
	if _, err := mirrorSvc.RegisterMirrorAndSync(ctx, user.ID, mirrorDir); err != nil {
		t.Fatalf("RegisterMirrorAndSync: %v", err)
	}

	status, body := doJSON(t, http.MethodPut, ts.URL+"/api/memory/profile", adminToken, []byte(`{"preferences":{"preferences":"Keep it concise."}}`))
	if status != http.StatusOK || !body.OK {
		t.Fatalf("profile update failed: status=%d body=%+v", status, body)
	}
	for _, expected := range []string{`"enabled":true`, `"synced":true`, `"path":"` + mirrorDir + `"`, `已同步到本地 Git 目录:`} {
		if !bytes.Contains(body.LocalGitSync, []byte(expected)) {
			t.Fatalf("expected %q in local_git_sync payload: %s", expected, string(body.LocalGitSync))
		}
	}

	mirrored, err := os.ReadFile(filepath.Join(mirrorDir, "memory", "profile", "preferences.md"))
	if err != nil {
		t.Fatalf("read mirrored profile: %v", err)
	}
	if !bytes.Contains(mirrored, []byte("Keep it concise.")) {
		t.Fatalf("unexpected mirrored profile content: %s", string(mirrored))
	}
}

func TestSQLiteSharedServerGitMirrorSettingsHideStoredGitHubToken(t *testing.T) {
	gh := newFakeGitHubServer(t, map[string]fakeGitHubTokenState{
		"ghp_valid": {
			login:      "octocat",
			fullName:   "acme/demo",
			permission: "write",
		},
	})
	ts, store, adminToken, _, _ := newTestHTTPServer(
		t,
		localgitsync.WithGitHubAPIBaseURL(gh.URL),
		localgitsync.WithHTTPClient(gh.Client()),
	)

	ctx := context.Background()
	v, err := vault.NewVault(strings.Repeat("0", 64))
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	mirrorSvc := localgitsync.New(
		store,
		v,
		localgitsync.WithGitHubAPIBaseURL(gh.URL),
		localgitsync.WithHTTPClient(gh.Client()),
	)
	if _, err := mirrorSvc.RegisterMirrorAndSync(ctx, user.ID, filepath.Join(t.TempDir(), "mirror")); err != nil {
		t.Fatalf("RegisterMirrorAndSync: %v", err)
	}

	status, updated := doJSON(t, http.MethodPut, ts.URL+"/api/local/git-mirror", adminToken, []byte(`{
		"auto_commit_enabled": true,
		"auto_push_enabled": false,
		"auth_mode": "github_token",
		"remote_url": "git@github.com:acme/demo.git",
		"remote_branch": "main",
		"github_token": "ghp_valid"
	}`))
	if status != http.StatusOK || !updated.OK {
		t.Fatalf("PUT /api/local/git-mirror failed: status=%d body=%+v", status, updated)
	}
	if bytes.Contains(updated.Data, []byte("ghp_valid")) {
		t.Fatalf("settings response must not echo raw github token: %s", string(updated.Data))
	}
	for _, expected := range []string{
		`"github_token_configured":true`,
		`"github_token_login":"octocat"`,
		`"github_repo_permission":"write"`,
		`"remote_url":"https://github.com/acme/demo.git"`,
	} {
		if !bytes.Contains(updated.Data, []byte(expected)) {
			t.Fatalf("expected %q in updated settings payload: %s", expected, string(updated.Data))
		}
	}

	status, fetched := doJSON(t, http.MethodGet, ts.URL+"/api/local/git-mirror", adminToken, nil)
	if status != http.StatusOK || !fetched.OK {
		t.Fatalf("GET /api/local/git-mirror failed: status=%d body=%+v", status, fetched)
	}
	if bytes.Contains(fetched.Data, []byte("ghp_valid")) {
		t.Fatalf("GET settings must not echo raw github token: %s", string(fetched.Data))
	}

	vaultSvc := services.NewVaultServiceWithRepo(sqlitestorage.NewVaultRepo(store), v)
	tokenValue, err := vaultSvc.Read(ctx, user.ID, "auth.github.git_mirror", models.TrustLevelFull)
	if err != nil {
		t.Fatalf("Read stored github token: %v", err)
	}
	if tokenValue != "ghp_valid" {
		t.Fatalf("stored github token = %q, want ghp_valid", tokenValue)
	}
}

func TestSQLiteSharedServerGitMirrorGitHubTestEndpointReturnsPermissionFailure(t *testing.T) {
	gh := newFakeGitHubServer(t, map[string]fakeGitHubTokenState{
		"ghp_readonly": {
			login:      "octocat",
			fullName:   "acme/demo",
			permission: "read",
		},
	})
	ts, _, adminToken, _, _ := newTestHTTPServer(
		t,
		localgitsync.WithGitHubAPIBaseURL(gh.URL),
		localgitsync.WithHTTPClient(gh.Client()),
	)

	status, tested := doJSON(t, http.MethodPost, ts.URL+"/api/local/git-mirror/github/test", adminToken, []byte(`{
		"remote_url": "git@github.com:acme/demo.git",
		"github_token": "ghp_readonly"
	}`))
	if status != http.StatusOK || !tested.OK {
		t.Fatalf("POST /api/local/git-mirror/github/test failed: status=%d body=%+v", status, tested)
	}
	for _, expected := range []string{
		`"ok":false`,
		`"permission":"read"`,
		`"normalized_remote_url":"https://github.com/acme/demo.git"`,
	} {
		if !bytes.Contains(tested.Data, []byte(expected)) {
			t.Fatalf("expected %q in github test payload: %s", expected, string(tested.Data))
		}
	}
}

func TestSQLiteSharedServerLocalConfigRoundTrip(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(runtimecfg.ConfigEnv, configPath)

	ts, _, adminToken, _, _ := newTestHTTPServer(t)

	status, fetched := doJSON(t, http.MethodGet, ts.URL+"/api/local/config", adminToken, nil)
	if status != http.StatusOK || !fetched.OK {
		t.Fatalf("GET /api/local/config failed: status=%d body=%+v", status, fetched)
	}

	var initial struct {
		Path string `json:"path"`
		Raw  string `json:"raw"`
	}
	if err := json.Unmarshal(fetched.Data, &initial); err != nil {
		t.Fatalf("unmarshal fetched config: %v", err)
	}
	if initial.Path != configPath {
		t.Fatalf("config path = %q, want %q", initial.Path, configPath)
	}
	if !strings.Contains(initial.Raw, `"version": 2`) {
		t.Fatalf("expected default config payload, got %s", initial.Raw)
	}

	body, err := json.Marshal(map[string]string{
		"raw": "{\n  \"version\": 2,\n  \"local\": {\n    \"listen_addr\": \"127.0.0.1:42690\"\n  },\n  \"extra\": {\n    \"keep\": true\n  }\n}",
	})
	if err != nil {
		t.Fatalf("Marshal body: %v", err)
	}
	status, updated := doJSON(t, http.MethodPut, ts.URL+"/api/local/config", adminToken, body)
	if status != http.StatusOK || !updated.OK {
		t.Fatalf("PUT /api/local/config failed: status=%d body=%+v", status, updated)
	}

	var saved struct {
		Path string `json:"path"`
		Raw  string `json:"raw"`
	}
	if err := json.Unmarshal(updated.Data, &saved); err != nil {
		t.Fatalf("unmarshal updated config: %v", err)
	}
	if saved.Path != configPath {
		t.Fatalf("saved path = %q, want %q", saved.Path, configPath)
	}
	if !strings.Contains(saved.Raw, "\"extra\": {\n    \"keep\": true\n  }") {
		t.Fatalf("expected unknown field to remain, got %s", saved.Raw)
	}

	persisted, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile persisted config: %v", err)
	}
	if string(persisted) != saved.Raw {
		t.Fatalf("persisted config mismatch\npersisted=%q\nsaved=%q", string(persisted), saved.Raw)
	}
}

func TestSQLiteSharedServerLocalConfigRejectsInvalidJSON(t *testing.T) {
	t.Setenv(runtimecfg.ConfigEnv, filepath.Join(t.TempDir(), "config.json"))

	ts, _, adminToken, _, _ := newTestHTTPServer(t)

	body, err := json.Marshal(map[string]string{"raw": "{invalid"})
	if err != nil {
		t.Fatalf("Marshal body: %v", err)
	}
	status, failed := doJSON(t, http.MethodPut, ts.URL+"/api/local/config", adminToken, body)
	if status != http.StatusBadRequest {
		t.Fatalf("PUT /api/local/config invalid json status = %d, body=%+v", status, failed)
	}
	if failed.OK {
		t.Fatalf("expected invalid config update to fail: %+v", failed)
	}
}

func TestSQLiteSharedServerTreeDirectoryWithoutTrailingSlashListsChildren(t *testing.T) {
	ts, _, adminToken, _, _ := newTestHTTPServer(t)

	status, wrote := doJSON(t, http.MethodPut, ts.URL+"/api/tree/notes/test.md", adminToken, []byte(`{"content":"hello","mime_type":"text/markdown"}`))
	if status != http.StatusOK || !wrote.OK {
		t.Fatalf("write note failed: status=%d body=%+v", status, wrote)
	}

	status, listed := doJSON(t, http.MethodGet, ts.URL+"/api/tree/notes", adminToken, nil)
	if status != http.StatusOK || !listed.OK {
		t.Fatalf("list notes dir failed: status=%d body=%+v", status, listed)
	}

	var node struct {
		Path     string `json:"path"`
		Children []struct {
			Path string `json:"path"`
		} `json:"children"`
	}
	if err := json.Unmarshal(listed.Data, &node); err != nil {
		t.Fatalf("unmarshal tree node: %v", err)
	}
	if node.Path != "/notes/" {
		t.Fatalf("unexpected node path: %q", node.Path)
	}
	if len(node.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(node.Children))
	}
	if node.Children[0].Path != "/notes/test.md" {
		t.Fatalf("unexpected child path: %q", node.Children[0].Path)
	}
}

func TestSQLiteSharedServerProjectsAndSkillsEndpoints(t *testing.T) {
	ts, store, adminToken, _, _ := newTestHTTPServer(t)
	ctx := context.Background()
	userID, err := store.FirstUserID(ctx)
	if err != nil {
		t.Fatalf("FirstUserID: %v", err)
	}
	if _, err := store.CreateProject(ctx, userID, "demo-project"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := store.AppendProjectLog(ctx, userID, "demo-project", models.ProjectLog{
		Source:    "test",
		Action:    "created",
		Summary:   "hello project",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendProjectLog: %v", err)
	}
	if _, err := store.WriteEntry(ctx, userID, "/skills/demo/SKILL.md", "# Demo\n", "text/markdown", models.FileTreeWriteOptions{
		MinTrustLevel: models.TrustLevelGuest,
	}); err != nil {
		t.Fatalf("WriteEntry skill: %v", err)
	}

	status, projects := doJSON(t, http.MethodGet, ts.URL+"/api/projects", adminToken, nil)
	if status != http.StatusOK || !projects.OK {
		t.Fatalf("GET /api/projects failed: status=%d body=%+v", status, projects)
	}
	for _, expected := range []string{`"name":"demo-project"`, `"status":"active"`} {
		if !bytes.Contains(projects.Data, []byte(expected)) {
			t.Fatalf("expected %q in projects payload: %s", expected, string(projects.Data))
		}
	}

	status, project := doJSON(t, http.MethodGet, ts.URL+"/api/projects/demo-project", adminToken, nil)
	if status != http.StatusOK || !project.OK {
		t.Fatalf("GET /api/projects/demo-project failed: status=%d body=%+v", status, project)
	}
	for _, expected := range []string{`"project"`, `"logs"`, `"created_at"`, `"hello project"`} {
		if !bytes.Contains(project.Data, []byte(expected)) {
			t.Fatalf("expected %q in project payload: %s", expected, string(project.Data))
		}
	}

	status, archived := doJSON(t, http.MethodPut, ts.URL+"/api/projects/demo-project/archive", adminToken, nil)
	if status != http.StatusOK || !archived.OK || !bytes.Contains(archived.Data, []byte(`"status":"archived"`)) {
		t.Fatalf("PUT /api/projects/demo-project/archive failed: status=%d body=%+v", status, archived)
	}

	status, skills := doJSON(t, http.MethodGet, ts.URL+"/api/tree/skills/", adminToken, nil)
	if status != http.StatusOK || !skills.OK {
		t.Fatalf("GET /api/tree/skills/ failed: status=%d body=%+v", status, skills)
	}
	for _, expected := range []string{`"/skills/demo"`, `"/skills/neudrive/"`} {
		if !bytes.Contains(skills.Data, []byte(expected)) {
			t.Fatalf("expected %q in skills payload: %s", expected, string(skills.Data))
		}
	}

	for _, hiddenPath := range []string{"/api/devices", "/api/roles", "/api/inbox/assistant?status=incoming"} {
		status, hidden := doJSON(t, http.MethodGet, ts.URL+hiddenPath, adminToken, nil)
		if status != http.StatusNotFound || hidden.OK {
			t.Fatalf("expected %s to be hidden: status=%d body=%+v", hiddenPath, status, hidden)
		}
	}

	status, root := doJSON(t, http.MethodGet, ts.URL+"/api/tree/", adminToken, nil)
	if status != http.StatusOK || !root.OK {
		t.Fatalf("GET /api/tree/ failed: status=%d body=%+v", status, root)
	}
	for _, expected := range []string{`"/projects"`, `"/skills/"`} {
		if !bytes.Contains(root.Data, []byte(expected)) {
			t.Fatalf("expected %q in root tree payload: %s", expected, string(root.Data))
		}
	}
	for _, unexpected := range []string{`"/devices"`, `"/roles"`, `"/inbox"`} {
		if bytes.Contains(root.Data, []byte(unexpected)) {
			t.Fatalf("did not expect %q in root tree payload: %s", unexpected, string(root.Data))
		}
	}
}

func TestSQLiteSharedServerImportSkillsZip(t *testing.T) {
	ts, store, _, _, _ := newTestHTTPServer(t)
	ctx := context.Background()
	userID, err := store.FirstUserID(ctx)
	if err != nil {
		t.Fatalf("FirstUserID: %v", err)
	}
	skillsToken, err := store.CreateToken(ctx, userID, "skills", []string{models.ScopeWriteSkills}, models.TrustLevelWork, time.Hour)
	if err != nil {
		t.Fatalf("CreateToken skills: %v", err)
	}

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	writeZipEntry := func(name string, data []byte) {
		t.Helper()
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create zip entry %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("Write zip entry %s: %v", name, err)
		}
	}
	writeZipEntry("claude-web-skill/SKILL.md", []byte("# Claude Web Skill\n\nImported from Claude Web.\n"))
	writeZipEntry("claude-web-skill/helper.py", []byte("print('hello from zip')\n"))
	writeZipEntry("claude-web-skill/assets/logo.png", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00})
	if err := zw.Close(); err != nil {
		t.Fatalf("Close zip writer: %v", err)
	}

	status, env := doMultipartForm(t, http.MethodPost, ts.URL+"/agent/import/skills", skillsToken.Token, "file", "neudrive-skills.zip", zipBuf.Bytes(), map[string]string{
		"platform": "claude-web",
	})
	if status != http.StatusOK || !env.OK {
		t.Fatalf("import skills zip failed: status=%d body=%+v", status, env)
	}
	if !bytes.Contains(env.Data, []byte(`"imported":3`)) {
		t.Fatalf("unexpected import payload: %s", string(env.Data))
	}
	if !bytes.Contains(env.Data, []byte(`"skills":["claude-web-skill"]`)) {
		t.Fatalf("expected imported skill names in payload: %s", string(env.Data))
	}

	entry, err := store.Read(ctx, userID, "/skills/claude-web-skill/SKILL.md", models.TrustLevelWork)
	if err != nil {
		t.Fatalf("Read SKILL.md: %v", err)
	}
	if !strings.Contains(entry.Content, "Imported from Claude Web") {
		t.Fatalf("unexpected SKILL.md content: %q", entry.Content)
	}
	binaryEntry, err := store.Read(ctx, userID, "/skills/claude-web-skill/assets/logo.png", models.TrustLevelWork)
	if err != nil {
		t.Fatalf("Read logo: %v", err)
	}
	blob, ok, err := store.ReadBlobByEntryID(ctx, binaryEntry.ID)
	if err != nil {
		t.Fatalf("ReadBlobByEntryID: %v", err)
	}
	if !ok || len(blob) == 0 {
		t.Fatalf("expected blob content for logo, ok=%t len=%d", ok, len(blob))
	}
	if binaryEntry.Metadata["capture_mode"] != "archive" || binaryEntry.Metadata["source_platform"] != "claude-web" {
		t.Fatalf("unexpected logo metadata: %+v", binaryEntry.Metadata)
	}
}

func TestSQLiteSharedServerFileTreeBrowseRegression(t *testing.T) {
	ts, _, adminToken, _, _ := newTestHTTPServer(t)

	writeBody := []byte(`{"content":"# Demo Skill\n","mime_type":"text/markdown"}`)
	status, wrote := doJSON(t, http.MethodPut, ts.URL+"/api/tree/skills/demo/SKILL.md", adminToken, writeBody)
	if status != http.StatusOK || !wrote.OK {
		t.Fatalf("write skill failed: status=%d body=%+v", status, wrote)
	}

	status, dirNoSlash := doJSON(t, http.MethodGet, ts.URL+"/api/tree/skills/demo", adminToken, nil)
	if status != http.StatusOK || !dirNoSlash.OK {
		t.Fatalf("browse dir without slash failed: status=%d body=%+v", status, dirNoSlash)
	}
	for _, expected := range []string{`"path":"/skills/demo/"`, `"is_dir":true`, `"name":"SKILL.md"`} {
		if !bytes.Contains(dirNoSlash.Data, []byte(expected)) {
			t.Fatalf("expected %q in dir browse payload: %s", expected, string(dirNoSlash.Data))
		}
	}

	status, dirWithSlash := doJSON(t, http.MethodGet, ts.URL+"/api/tree/skills/demo/", adminToken, nil)
	if status != http.StatusOK || !dirWithSlash.OK {
		t.Fatalf("browse dir with slash failed: status=%d body=%+v", status, dirWithSlash)
	}
	for _, expected := range []string{`"path":"/skills/demo/"`, `"is_dir":true`, `"name":"SKILL.md"`} {
		if !bytes.Contains(dirWithSlash.Data, []byte(expected)) {
			t.Fatalf("expected %q in slash browse payload: %s", expected, string(dirWithSlash.Data))
		}
	}

	status, systemDir := doJSON(t, http.MethodGet, ts.URL+"/api/tree/skills/portability/chatgpt", adminToken, nil)
	if status != http.StatusOK || !systemDir.OK {
		t.Fatalf("browse system dir without slash failed: status=%d body=%+v", status, systemDir)
	}
	for _, expected := range []string{`"path":"/skills/portability/chatgpt/"`, `"is_dir":true`, `"name":"SKILL.md"`} {
		if !bytes.Contains(systemDir.Data, []byte(expected)) {
			t.Fatalf("expected %q in system dir payload: %s", expected, string(systemDir.Data))
		}
	}

	status, systemSkill := doJSON(t, http.MethodGet, ts.URL+"/api/tree/skills/portability/chatgpt/SKILL.md", adminToken, nil)
	if status != http.StatusOK || !systemSkill.OK {
		t.Fatalf("read system skill failed: status=%d body=%+v", status, systemSkill)
	}
	for _, expected := range []string{`"kind":"skill"`, `## Current User Snapshot`, `Connected to ChatGPT: no`} {
		if !bytes.Contains(systemSkill.Data, []byte(expected)) {
			t.Fatalf("expected %q in rendered system skill payload: %s", expected, string(systemSkill.Data))
		}
	}
}

func TestSQLiteSharedServerTreeExposesSource(t *testing.T) {
	ts, _, adminToken, _, _ := newTestHTTPServer(t)

	writeBody := []byte(`{"content":"manual note","mime_type":"text/markdown","metadata":{"source":"manual"}}`)
	status, wrote := doJSON(t, http.MethodPut, ts.URL+"/api/tree/notes/source-demo.md", adminToken, writeBody)
	if status != http.StatusOK || !wrote.OK {
		t.Fatalf("write file failed: status=%d body=%+v", status, wrote)
	}

	status, read := doJSON(t, http.MethodGet, ts.URL+"/api/tree/notes/source-demo.md", adminToken, nil)
	if status != http.StatusOK || !read.OK {
		t.Fatalf("read file failed: status=%d body=%+v", status, read)
	}
	if !bytes.Contains(read.Data, []byte(`"source":"manual"`)) {
		t.Fatalf("expected source in tree payload: %s", string(read.Data))
	}
}

func TestSQLiteSharedServerTreeDefaultsNewFileSourceToManual(t *testing.T) {
	ts, _, adminToken, _, _ := newTestHTTPServer(t)

	status, wrote := doJSON(t, http.MethodPut, ts.URL+"/api/tree/notes/default-source.md", adminToken, []byte(`{"content":"manual note","mime_type":"text/markdown"}`))
	if status != http.StatusOK || !wrote.OK {
		t.Fatalf("write file failed: status=%d body=%+v", status, wrote)
	}

	status, read := doJSON(t, http.MethodGet, ts.URL+"/api/tree/notes/default-source.md", adminToken, nil)
	if status != http.StatusOK || !read.OK {
		t.Fatalf("read file failed: status=%d body=%+v", status, read)
	}
	if !bytes.Contains(read.Data, []byte(`"source":"manual"`)) {
		t.Fatalf("expected default manual source in tree payload: %s", string(read.Data))
	}
}

func TestSQLiteSharedServerTreeAllowsExplicitPlatformHeader(t *testing.T) {
	ts, _, adminToken, _, _ := newTestHTTPServer(t)

	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/tree/notes/platform-header.md", bytes.NewReader([]byte(`{"content":"header sourced","mime_type":"text/markdown"}`)))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-NeuDrive-Platform", "kimi")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("write file failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	status, read := doJSON(t, http.MethodGet, ts.URL+"/api/tree/notes/platform-header.md", adminToken, nil)
	if status != http.StatusOK || !read.OK {
		t.Fatalf("read file failed: status=%d body=%+v", status, read)
	}
	if !bytes.Contains(read.Data, []byte(`"source":"kimi"`)) {
		t.Fatalf("expected kimi source in tree payload: %s", string(read.Data))
	}
}

func TestSQLiteSharedServerImportSkillsJSONTracksPlatformSource(t *testing.T) {
	ts, store, adminToken, _, _ := newTestHTTPServer(t)
	ctx := context.Background()
	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}

	body := []byte(`{"skills":[{"path":"platform-demo/SKILL.md","content":"# Platform Demo\n","content_type":"text/markdown"}]}`)
	status, env := doJSON(t, http.MethodPost, ts.URL+"/api/import/skills?platform=claude-web", adminToken, body)
	if status != http.StatusOK || !env.OK {
		t.Fatalf("import skills json failed: status=%d body=%+v", status, env)
	}

	entry, err := store.Read(ctx, user.ID, "/skills/platform-demo/SKILL.md", models.TrustLevelWork)
	if err != nil {
		t.Fatalf("Read SKILL.md: %v", err)
	}
	if entry.Metadata["source_platform"] != "claude-web" {
		t.Fatalf("expected source_platform=claude-web, got %+v", entry.Metadata)
	}
}

func TestSQLiteSharedServerImportSkillsJSONAllowsExplicitSourcePlatform(t *testing.T) {
	ts, store, adminToken, _, _ := newTestHTTPServer(t)
	ctx := context.Background()
	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}

	body := []byte(`{"source_platform":"perplexity","skills":[{"path":"explicit-platform/SKILL.md","content":"# Explicit Platform\n","content_type":"text/markdown"}]}`)
	status, env := doJSON(t, http.MethodPost, ts.URL+"/api/import/skills", adminToken, body)
	if status != http.StatusOK || !env.OK {
		t.Fatalf("import skills json failed: status=%d body=%+v", status, env)
	}

	entry, err := store.Read(ctx, user.ID, "/skills/explicit-platform/SKILL.md", models.TrustLevelWork)
	if err != nil {
		t.Fatalf("Read SKILL.md: %v", err)
	}
	if entry.Metadata["source_platform"] != "perplexity" {
		t.Fatalf("expected source_platform=perplexity, got %+v", entry.Metadata)
	}
}

func TestSQLiteSharedServerAgentTreeDefaultsSourceToAgent(t *testing.T) {
	ts, _, adminToken, _, _ := newTestHTTPServer(t)

	status, wrote := doJSON(t, http.MethodPut, ts.URL+"/agent/tree/notes/agent-source.md", adminToken, []byte(`{"content":"agent note","content_type":"text/markdown"}`))
	if status != http.StatusOK || !wrote.OK {
		t.Fatalf("agent tree write failed: status=%d body=%+v", status, wrote)
	}

	status, read := doJSON(t, http.MethodGet, ts.URL+"/api/tree/notes/agent-source.md", adminToken, nil)
	if status != http.StatusOK || !read.OK {
		t.Fatalf("read file failed: status=%d body=%+v", status, read)
	}
	if !bytes.Contains(read.Data, []byte(`"source":"agent"`)) {
		t.Fatalf("expected agent source in tree payload: %s", string(read.Data))
	}
}

func TestSQLiteSharedServerAgentTreePrefersPlatformTokenSource(t *testing.T) {
	ts, store, _, _, _ := newTestHTTPServer(t)
	ctx := context.Background()
	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	codexToken, err := store.CreateToken(ctx, user.ID, "local platform codex", []string{models.ScopeReadTree, models.ScopeWriteTree}, models.TrustLevelWork, time.Hour)
	if err != nil {
		t.Fatalf("CreateToken codex: %v", err)
	}

	status, wrote := doJSON(t, http.MethodPut, ts.URL+"/agent/tree/notes/agent-platform-source.md", codexToken.Token, []byte(`{"content":"agent note","content_type":"text/markdown"}`))
	if status != http.StatusOK || !wrote.OK {
		t.Fatalf("agent tree write failed: status=%d body=%+v", status, wrote)
	}

	entry, err := store.Read(ctx, user.ID, "/notes/agent-platform-source.md", models.TrustLevelFull)
	if err != nil {
		t.Fatalf("Read agent-platform-source.md: %v", err)
	}
	if services.EntrySourceFromMetadata(entry.Metadata) != "codex" {
		t.Fatalf("expected codex source, got %+v", entry.Metadata)
	}
}

func TestSQLiteSharedServerAgentCreateProjectUsesAgentSource(t *testing.T) {
	ts, store, adminToken, _, _ := newTestHTTPServer(t)
	ctx := context.Background()
	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}

	status, env := doJSON(t, http.MethodPost, ts.URL+"/agent/projects", adminToken, []byte(`{"name":"agent-created"}`))
	if status != http.StatusCreated || !env.OK {
		t.Fatalf("agent create project failed: status=%d body=%+v", status, env)
	}

	entry, err := store.Read(ctx, user.ID, "/projects/agent-created/context.md", models.TrustLevelWork)
	if err != nil {
		t.Fatalf("Read project context: %v", err)
	}
	if services.EntrySourceFromMetadata(entry.Metadata) != "agent" {
		t.Fatalf("expected agent source, got %+v", entry.Metadata)
	}
}

func TestSQLiteSharedServerMCPSessionTracksPlatformSource(t *testing.T) {
	ts, store, _, _, _ := newTestHTTPServer(t)
	ctx := context.Background()
	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	token, err := store.CreateToken(ctx, user.ID, "mcp-test", []string{models.ScopeAdmin}, models.TrustLevelFull, time.Hour)
	if err != nil {
		t.Fatalf("CreateToken mcp-test: %v", err)
	}

	initReq, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"codex-mcp-client","title":"Codex","version":"0.118.0"}}}`))
	if err != nil {
		t.Fatalf("NewRequest initialize: %v", err)
	}
	initReq.Header.Set("Authorization", "Bearer "+token.Token)
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := http.DefaultClient.Do(initReq)
	if err != nil {
		t.Fatalf("initialize request failed: %v", err)
	}
	defer initResp.Body.Close()
	if initResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(initResp.Body)
		t.Fatalf("initialize status=%d body=%s", initResp.StatusCode, string(body))
	}
	sessionID := initResp.Header.Get("Mcp-Session-Id")
	if strings.TrimSpace(sessionID) == "" {
		t.Fatalf("expected Mcp-Session-Id header")
	}

	writeReq, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"/notes/mcp-session-source.md","content":"hello from mcp"}}}`))
	if err != nil {
		t.Fatalf("NewRequest write_file: %v", err)
	}
	writeReq.Header.Set("Authorization", "Bearer "+token.Token)
	writeReq.Header.Set("Content-Type", "application/json")
	writeReq.Header.Set("Mcp-Session-Id", sessionID)
	writeResp, err := http.DefaultClient.Do(writeReq)
	if err != nil {
		t.Fatalf("write_file request failed: %v", err)
	}
	defer writeResp.Body.Close()
	if writeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(writeResp.Body)
		t.Fatalf("write_file status=%d body=%s", writeResp.StatusCode, string(body))
	}

	status, read := doJSON(t, http.MethodGet, ts.URL+"/api/tree/notes/mcp-session-source.md", token.Token, nil)
	if status != http.StatusOK || !read.OK {
		t.Fatalf("read file failed: status=%d body=%+v", status, read)
	}
	if !bytes.Contains(read.Data, []byte(`"source":"codex"`)) {
		t.Fatalf("expected codex source in tree payload: %s", string(read.Data))
	}
}

func doJSON(t *testing.T, method, url, token string, body []byte) (int, testEnvelope) {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	var env testEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp.StatusCode, env
}

func newFakeGitHubServer(t *testing.T, states map[string]fakeGitHubTokenState) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		state, ok := states[authHeader]
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"bad credentials"}`))
			return
		}
		switch r.URL.Path {
		case "/user":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"login": state.login,
			})
		case "/repos/acme/demo":
			permissions := map[string]bool{
				"admin": state.permission == "admin",
				"push":  state.permission == "admin" || state.permission == "write",
				"pull":  state.permission == "admin" || state.permission == "write" || state.permission == "read",
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"full_name":   state.fullName,
				"permissions": permissions,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func doMultipartForm(t *testing.T, method, url, token, fieldName, filename string, payload []byte, fields map[string]string) (int, testEnvelope) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("WriteField %s: %v", key, err)
		}
	}
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(payload)); err != nil {
		t.Fatalf("Write multipart payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close multipart writer: %v", err)
	}

	req, err := http.NewRequest(method, url, &body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	var env testEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode multipart response: %v", err)
	}
	return resp.StatusCode, env
}

func TestAgentScratchAndEphemeralTokenEndpoints(t *testing.T) {
	ts, _, adminToken, _, _ := newTestHTTPServer(t)
	defer ts.Close()

	status, scratch := doJSON(t, http.MethodPost, ts.URL+"/agent/memory/scratch", adminToken, []byte(`{"content":"agent scratch note","title":"note"}`))
	if status != http.StatusCreated || !scratch.OK {
		t.Fatalf("POST /agent/memory/scratch failed: status=%d body=%+v", status, scratch)
	}
	for _, expected := range []string{`"imported_count":1`, `memory/scratch/`} {
		if !bytes.Contains(scratch.Data, []byte(expected)) {
			t.Fatalf("expected %q in scratch response: %s", expected, string(scratch.Data))
		}
	}

	status, syncToken := doJSON(t, http.MethodPost, ts.URL+"/agent/tokens/ephemeral", adminToken, []byte(`{"kind":"sync","purpose":"backup","access":"both","ttl_minutes":30}`))
	if status != http.StatusCreated || !syncToken.OK {
		t.Fatalf("POST /agent/tokens/ephemeral sync failed: status=%d body=%+v", status, syncToken)
	}
	for _, expected := range []string{`"token":"`, `"read:bundle"`, `"write:bundle"`} {
		if !bytes.Contains(syncToken.Data, []byte(expected)) {
			t.Fatalf("expected %q in sync token response: %s", expected, string(syncToken.Data))
		}
	}

	status, uploadToken := doJSON(t, http.MethodPost, ts.URL+"/agent/tokens/ephemeral", adminToken, []byte(`{"kind":"skills-upload","purpose":"skills","platform":"claude-web","ttl_minutes":30}`))
	if status != http.StatusCreated || !uploadToken.OK {
		t.Fatalf("POST /agent/tokens/ephemeral skills-upload failed: status=%d body=%+v", status, uploadToken)
	}
	for _, expected := range []string{`"upload_url":"`, `"browser_upload_url":"`, `"connectivity_probe_url":"`, `"write:skills"`} {
		if !bytes.Contains(uploadToken.Data, []byte(expected)) {
			t.Fatalf("expected %q in skills upload token response: %s", expected, string(uploadToken.Data))
		}
	}
}
