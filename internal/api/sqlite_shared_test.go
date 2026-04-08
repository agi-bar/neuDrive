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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agi-bar/agenthub/internal/auth"
	"github.com/agi-bar/agenthub/internal/config"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	sqlitestorage "github.com/agi-bar/agenthub/internal/storage/sqlite"
	"github.com/agi-bar/agenthub/internal/vault"
	"github.com/google/uuid"
)

type testEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func newTestHTTPServer(t *testing.T) (*httptest.Server, *sqlitestorage.Store, string, string, string) {
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

	cfg := &config.Config{
		JWTSecret:      testJWTSecret,
		VaultMasterKey: strings.Repeat("0", 64),
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
	tokenGen := func(userID uuid.UUID, slug string) (string, error) {
		return auth.GenerateToken(userID, slug, cfg.JWTSecret)
	}
	authSvc := services.NewAuthServiceWithRepo(sqlitestorage.NewAuthRepo(store), tokenGen, nil)
	oauthSvc := services.NewOAuthServiceWithRepo(sqlitestorage.NewOAuthRepo(store), cfg.JWTSecret)

	s := NewServerWithDeps(ServerDeps{
		Storage:            "sqlite",
		Config:             cfg,
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
		Vault:              v,
		JWTSecret:          cfg.JWTSecret,
		GitHubClientID:     cfg.GithubClientID,
		GitHubClientSecret: cfg.GithubClientSecret,
	})
	ts := httptest.NewServer(s.Router)
	t.Cleanup(ts.Close)
	return ts, store, admin.Token, readBundle.Token, writeBundle.Token
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
	status, createdDevice := doJSON(t, http.MethodPost, ts.URL+"/api/devices", adminToken, []byte(`{"name":"desk-light","device_type":"light","protocol":"http","endpoint":"http://127.0.0.1/device"}`))
	if status != http.StatusCreated || !createdDevice.OK {
		t.Fatalf("POST /api/devices failed: status=%d body=%+v", status, createdDevice)
	}
	status, createdRole := doJSON(t, http.MethodPost, ts.URL+"/api/roles", adminToken, []byte(`{"name":"researcher","role_type":"worker","lifecycle":"project","allowed_paths":["/projects","/skills"]}`))
	if status != http.StatusCreated || !createdRole.OK {
		t.Fatalf("POST /api/roles failed: status=%d body=%+v", status, createdRole)
	}
	inboxPayload := []byte(`{"to":"assistant","subject":"Test message","body":"hello inbox"}`)
	status, sentInbox := doJSON(t, http.MethodPost, ts.URL+"/api/inbox/send", adminToken, inboxPayload)
	if status != http.StatusCreated || !sentInbox.OK {
		t.Fatalf("POST /api/inbox/send failed: status=%d body=%+v", status, sentInbox)
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
	for _, expected := range []string{`"/skills/demo"`, `"/skills/agenthub/"`} {
		if !bytes.Contains(skills.Data, []byte(expected)) {
			t.Fatalf("expected %q in skills payload: %s", expected, string(skills.Data))
		}
	}

	status, devices := doJSON(t, http.MethodGet, ts.URL+"/api/devices", adminToken, nil)
	if status != http.StatusOK || !devices.OK || !bytes.Contains(devices.Data, []byte(`"name":"desk-light"`)) {
		t.Fatalf("GET /api/devices failed: status=%d body=%+v", status, devices)
	}

	status, roles := doJSON(t, http.MethodGet, ts.URL+"/api/roles", adminToken, nil)
	if status != http.StatusOK || !roles.OK || !bytes.Contains(roles.Data, []byte(`"name":"researcher"`)) {
		t.Fatalf("GET /api/roles failed: status=%d body=%+v", status, roles)
	}

	status, inbox := doJSON(t, http.MethodGet, ts.URL+"/api/inbox/assistant?status=incoming", adminToken, nil)
	if status != http.StatusOK || !inbox.OK || !bytes.Contains(inbox.Data, []byte(`"subject":"Test message"`)) {
		t.Fatalf("GET /api/inbox/assistant failed: status=%d body=%+v", status, inbox)
	}

	status, root := doJSON(t, http.MethodGet, ts.URL+"/api/tree/", adminToken, nil)
	if status != http.StatusOK || !root.OK {
		t.Fatalf("GET /api/tree/ failed: status=%d body=%+v", status, root)
	}
	for _, expected := range []string{`"/devices"`, `"/roles"`, `"/inbox"`, `"/projects"`} {
		if !bytes.Contains(root.Data, []byte(expected)) {
			t.Fatalf("expected %q in root tree payload: %s", expected, string(root.Data))
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

	status, env := doMultipartForm(t, http.MethodPost, ts.URL+"/agent/import/skills", skillsToken.Token, "file", "agenthub-skills.zip", zipBuf.Bytes(), map[string]string{
		"platform": "claude-web",
	})
	if status != http.StatusOK || !env.OK {
		t.Fatalf("import skills zip failed: status=%d body=%+v", status, env)
	}
	if !bytes.Contains(env.Data, []byte(`"imported":3`)) {
		t.Fatalf("unexpected import payload: %s", string(env.Data))
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
	for _, expected := range []string{`"path":"/skills/demo"`, `"is_dir":true`, `"kind":"directory"`} {
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
