package localserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
