package localserver

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

	"github.com/agi-bar/agenthub/internal/localstore"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/go-chi/chi/v5"
)

type testEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func newTestHTTPServer(t *testing.T) (*httptest.Server, *localstore.Store, string, string, string) {
	t.Helper()
	ctx := context.Background()
	store, err := localstore.Open(filepath.Join(t.TempDir(), "local.db"))
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

	s := &Server{
		store:   store,
		baseURL: "http://127.0.0.1:0",
		router:  chi.NewRouter(),
	}
	s.routes()
	ts := httptest.NewServer(s.router)
	s.baseURL = ts.URL
	t.Cleanup(ts.Close)
	return ts, store, admin.Token, readBundle.Token, writeBundle.Token
}

func TestLocalServerHealthAndDisabledOAuth(t *testing.T) {
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
	disabledResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/auth/login: %v", err)
	}
	defer disabledResp.Body.Close()
	if disabledResp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("disabled auth status = %d", disabledResp.StatusCode)
	}
	var disabled testEnvelope
	if err := json.NewDecoder(disabledResp.Body).Decode(&disabled); err != nil {
		t.Fatalf("decode disabled: %v", err)
	}
	if disabled.OK || disabled.Error.Message == "" {
		t.Fatalf("unexpected disabled payload: %+v", disabled)
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

func TestLocalServerScopeGatingAndSyncFlow(t *testing.T) {
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

func TestLocalServerWebDashboardEndpoints(t *testing.T) {
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

func TestLocalServerImportSkillsZip(t *testing.T) {
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
