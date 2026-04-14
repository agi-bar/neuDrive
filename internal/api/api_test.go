package api

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// 1. Health endpoint
// ---------------------------------------------------------------------------

func TestHealthCheck(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}
	if body["service"] != "neudrive" {
		t.Errorf("expected service=neudrive, got %v", body["service"])
	}
	if _, ok := body["time"]; !ok {
		t.Error("expected time field in health response")
	}
}

func TestBodySizeLimitForPath(t *testing.T) {
	t.Run("mcp gets raised limit", func(t *testing.T) {
		if got := bodySizeLimitForPath("/mcp", 10<<20); got != maxMCPArchiveRequestBytes {
			t.Fatalf("bodySizeLimitForPath(/mcp) = %d, want %d", got, maxMCPArchiveRequestBytes)
		}
	})

	t.Run("skills import gets raised limit", func(t *testing.T) {
		if got := bodySizeLimitForPath("/agent/import/skills", 10<<20); got != maxSkillsArchiveRequestBytes {
			t.Fatalf("bodySizeLimitForPath(/agent/import/skills) = %d, want %d", got, maxSkillsArchiveRequestBytes)
		}
	})

	t.Run("ordinary paths keep fallback", func(t *testing.T) {
		if got := bodySizeLimitForPath("/api/tree/notes/demo.md", 10<<20); got != 10<<20 {
			t.Fatalf("bodySizeLimitForPath ordinary path = %d, want %d", got, 10<<20)
		}
	})
}

// ---------------------------------------------------------------------------
// 2. Auth flow
// ---------------------------------------------------------------------------

func TestAuthMeReturnsProfile(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/auth/me")
	if err != nil {
		t.Fatalf("GET /api/auth/me: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["slug"] != testUserSlug {
		t.Errorf("expected slug=%s, got %v", testUserSlug, body["slug"])
	}
	if body["id"] != testUserID.String() {
		t.Errorf("expected id=%s, got %v", testUserID, body["id"])
	}
}

func TestAuthMeWithoutTokenReturns401(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/auth/me")
	if err != nil {
		t.Fatalf("GET /api/auth/me (no auth): %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAuthMeWithInvalidTokenReturns401(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer invalid-jwt-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/auth/me (bad token): %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ---------------------------------------------------------------------------
// 3. Token CRUD
// ---------------------------------------------------------------------------

func TestTokenCreateUpdateListRevoke(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	// Create a token.
	createBody := map[string]interface{}{
		"name":            "test-token",
		"scopes":          []string{"read:profile", "read:tree"},
		"max_trust_level": 3,
		"expires_in_days": 7,
	}
	resp, err := authPost(ts, "/api/tokens", createBody)
	if err != nil {
		t.Fatalf("POST /api/tokens: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body := parseJSONRaw(resp)
		t.Fatalf("expected 201, got %d: %v", resp.StatusCode, body)
	}
	created := parseJSON(resp)
	rawToken, ok := created["token"].(string)
	if !ok || rawToken == "" {
		t.Fatal("expected raw token in create response")
	}
	scopedToken, ok := created["scoped_token"].(map[string]interface{})
	if !ok {
		t.Fatal("expected scoped_token object in response")
	}
	tokenID, _ := scopedToken["id"].(string)
	if tokenID == "" {
		t.Fatal("expected token id")
	}

	// List tokens.
	resp, err = authGet(ts, "/api/tokens")
	if err != nil {
		t.Fatalf("GET /api/tokens: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	listBody := parseJSON(resp)
	tokens, ok := listBody["tokens"].([]interface{})
	if !ok || len(tokens) == 0 {
		t.Fatal("expected at least one token in list")
	}

	// Get single token.
	resp, err = authGet(ts, "/api/tokens/"+tokenID)
	if err != nil {
		t.Fatalf("GET /api/tokens/{id}: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	detail := parseJSON(resp)
	if detail["name"] != "test-token" {
		t.Errorf("expected name=test-token, got %v", detail["name"])
	}

	// Update token name.
	resp, err = authPut(ts, "/api/tokens/"+tokenID, map[string]interface{}{
		"name": "renamed-token",
	})
	if err != nil {
		t.Fatalf("PUT /api/tokens/{id}: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body := parseJSONRaw(resp)
		t.Fatalf("expected 200 on update, got %d: %v", resp.StatusCode, body)
	}
	updated := parseJSON(resp)
	if updated["name"] != "renamed-token" {
		t.Errorf("expected renamed token, got %v", updated["name"])
	}

	// Verify list reflects updated name.
	resp, err = authGet(ts, "/api/tokens")
	if err != nil {
		t.Fatalf("GET /api/tokens after update: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after update, got %d", resp.StatusCode)
	}
	listBody = parseJSON(resp)
	tokens, ok = listBody["tokens"].([]interface{})
	if !ok || len(tokens) == 0 {
		t.Fatal("expected at least one token in list after update")
	}
	firstToken, ok := tokens[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected first token object in list")
	}
	if firstToken["name"] != "renamed-token" {
		t.Errorf("expected renamed token in list, got %v", firstToken["name"])
	}

	// Revoke token.
	resp, err = authDelete(ts, "/api/tokens/"+tokenID)
	if err != nil {
		t.Fatalf("DELETE /api/tokens/{id}: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	revokeBody := parseJSON(resp)
	if revokeBody["status"] != "revoked" {
		t.Errorf("expected status=revoked, got %v", revokeBody["status"])
	}

	// Revoking again should 404.
	resp, err = authDelete(ts, "/api/tokens/"+tokenID)
	if err != nil {
		t.Fatalf("DELETE /api/tokens/{id} (2nd): %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on double revoke, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestTokenCreateValidation(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	tests := []struct {
		name string
		body map[string]interface{}
		want int
	}{
		{
			name: "missing name",
			body: map[string]interface{}{"scopes": []string{"read:profile"}},
			want: http.StatusBadRequest,
		},
		{
			name: "missing scopes",
			body: map[string]interface{}{"name": "x"},
			want: http.StatusBadRequest,
		},
		{
			name: "empty scopes",
			body: map[string]interface{}{"name": "x", "scopes": []string{}},
			want: http.StatusBadRequest,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := authPost(ts, "/api/tokens", tc.body)
			if err != nil {
				t.Fatalf("POST /api/tokens: %v", err)
			}
			if resp.StatusCode != tc.want {
				t.Errorf("expected %d, got %d", tc.want, resp.StatusCode)
			}
			resp.Body.Close()
		})
	}
}

func TestTokenListScopes(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/tokens/scopes")
	if err != nil {
		t.Fatalf("GET /api/tokens/scopes: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	// The scopes endpoint uses respondOK, so data is inside the envelope.
	body := parseJSON(resp)
	scopes, ok := body["scopes"].([]interface{})
	if !ok || len(scopes) == 0 {
		t.Error("expected non-empty scopes list")
	}
	if _, ok := body["categories"]; !ok {
		t.Error("expected categories in response")
	}
	if _, ok := body["bundles"]; !ok {
		t.Error("expected bundles in response")
	}
}

// ---------------------------------------------------------------------------
// 4. File tree
// ---------------------------------------------------------------------------

func TestFileTreeList(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/tree/")
	if err != nil {
		t.Fatalf("GET /api/tree/: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestFileTreeReadFile(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/tree/skills/test.md")
	if err != nil {
		t.Fatalf("GET /api/tree/skills/test.md: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestFileTreeWrite(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	writeBody := map[string]interface{}{
		"content":   "# Hello\nThis is a test.",
		"mime_type": "text/markdown",
	}
	resp, err := authPut(ts, "/api/tree/test/hello.md", writeBody)
	if err != nil {
		t.Fatalf("PUT /api/tree/test/hello.md: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestFileTreeWithoutAuth(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/tree/")
	if err != nil {
		t.Fatalf("GET /api/tree/ (no auth): %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestFileTreeDelete(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authDelete(ts, "/api/tree/test/file.md")
	if err != nil {
		t.Fatalf("DELETE /api/tree/test/file.md: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestFileTreeReadSystemPortabilitySkill(t *testing.T) {
	ts, _ := newTestServerWithFileTree(&services.FileTreeService{})
	defer ts.Close()

	resp, err := authGet(ts, "/api/tree/skills/portability/chatgpt/SKILL.md")
	if err != nil {
		t.Fatalf("GET /api/tree/skills/portability/chatgpt/SKILL.md: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := parseJSON(resp)
	content, _ := body["content"].(string)
	if content == "" {
		t.Fatal("expected skill content")
	}
	if !strings.Contains(content, "## Current User Snapshot") {
		t.Fatalf("expected rendered snapshot block, got %q", content)
	}
	if !strings.Contains(content, "Connected to ChatGPT: unknown") {
		t.Fatalf("expected default snapshot, got %q", content)
	}
}

func TestFileTreeListSystemPortabilityDirectory(t *testing.T) {
	ts, _ := newTestServerWithFileTree(&services.FileTreeService{})
	defer ts.Close()

	resp, err := authGet(ts, "/api/tree/skills/portability/")
	if err != nil {
		t.Fatalf("GET /api/tree/skills/portability/: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := parseJSON(resp)
	children, ok := body["children"].([]interface{})
	if !ok {
		t.Fatalf("expected children array, got %#v", body["children"])
	}
	if len(children) != 4 {
		t.Fatalf("expected 4 platform directories, got %d", len(children))
	}
}

func TestFileTreeDownloadZipForFile(t *testing.T) {
	now := time.Date(2026, 4, 14, 16, 0, 0, 0, time.UTC)
	repo := stubFileTreeRepo{
		readFn: func(ctx context.Context, userID uuid.UUID, path string, trustLevel int) (*models.FileTreeEntry, error) {
			if path != "/notes/demo.md" {
				return nil, services.ErrEntryNotFound
			}
			return &models.FileTreeEntry{
				ID:          uuid.New(),
				UserID:      userID,
				Path:        path,
				Kind:        "note",
				Content:     "# demo\n",
				ContentType: "text/markdown",
				CreatedAt:   now,
				UpdatedAt:   now,
			}, nil
		},
	}
	ts, _ := newTestServerWithFileTree(services.NewFileTreeServiceWithRepo(repo))
	defer ts.Close()

	resp, err := authGet(ts, "/api/tree/archive?path=%2Fnotes%2Fdemo.md")
	if err != nil {
		t.Fatalf("GET /api/tree/archive: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/zip" {
		t.Fatalf("expected application/zip, got %q", got)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	if len(zr.File) != 1 {
		t.Fatalf("expected 1 file in archive, got %d", len(zr.File))
	}
	if zr.File[0].Name != "demo.md" {
		t.Fatalf("expected demo.md in archive, got %q", zr.File[0].Name)
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatalf("open zip file: %v", err)
	}
	content, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("read zip file: %v", err)
	}
	if string(content) != "# demo\n" {
		t.Fatalf("unexpected content %q", string(content))
	}
}

func TestFileTreeDownloadZipForDirectoryIncludesBinaryChildren(t *testing.T) {
	now := time.Date(2026, 4, 14, 16, 0, 0, 0, time.UTC)
	binaryEntry := &models.FileTreeEntry{
		ID:          uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Path:        "/skills/demo/assets/logo.png",
		Kind:        "skill_file",
		ContentType: "image/png",
		Metadata:    map[string]interface{}{"binary": true},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	repo := stubFileTreeRepo{
		readFn: func(ctx context.Context, userID uuid.UUID, path string, trustLevel int) (*models.FileTreeEntry, error) {
			switch path {
			case "/skills/demo":
				return &models.FileTreeEntry{
					ID:          uuid.New(),
					UserID:      userID,
					Path:        path,
					Kind:        "directory",
					IsDirectory: true,
					ContentType: "directory",
					CreatedAt:   now,
					UpdatedAt:   now,
				}, nil
			case "/skills/demo/SKILL.md":
				return &models.FileTreeEntry{
					ID:          uuid.New(),
					UserID:      userID,
					Path:        path,
					Kind:        "skill",
					Content:     "# Demo\n",
					ContentType: "text/markdown",
					CreatedAt:   now,
					UpdatedAt:   now,
				}, nil
			case binaryEntry.Path:
				entry := *binaryEntry
				entry.UserID = userID
				return &entry, nil
			default:
				return nil, services.ErrEntryNotFound
			}
		},
		listFn: func(ctx context.Context, userID uuid.UUID, path string, trustLevel int) ([]models.FileTreeEntry, error) {
			switch path {
			case "/skills/demo":
				return []models.FileTreeEntry{
					{
						ID:          uuid.New(),
						UserID:      userID,
						Path:        "/skills/demo/SKILL.md",
						Kind:        "skill",
						Content:     "# Demo\n",
						ContentType: "text/markdown",
						CreatedAt:   now,
						UpdatedAt:   now,
					},
					{
						ID:          uuid.New(),
						UserID:      userID,
						Path:        "/skills/demo/assets",
						Kind:        "directory",
						IsDirectory: true,
						ContentType: "directory",
						CreatedAt:   now,
						UpdatedAt:   now,
					},
				}, nil
			case "/skills/demo/assets":
				entry := *binaryEntry
				entry.UserID = userID
				return []models.FileTreeEntry{entry}, nil
			default:
				return nil, services.ErrEntryNotFound
			}
		},
		readBinaryFn: func(ctx context.Context, userID uuid.UUID, path string, trustLevel int) ([]byte, *models.FileTreeEntry, error) {
			if path != binaryEntry.Path {
				return nil, nil, services.ErrEntryNotFound
			}
			entry := *binaryEntry
			entry.UserID = userID
			return []byte{0x89, 'P', 'N', 'G'}, &entry, nil
		},
	}
	ts, _ := newTestServerWithFileTree(services.NewFileTreeServiceWithRepo(repo))
	defer ts.Close()

	resp, err := authGet(ts, "/api/tree/archive?path=%2Fskills%2Fdemo")
	if err != nil {
		t.Fatalf("GET /api/tree/archive: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	files := make(map[string][]byte, len(zr.File))
	for _, file := range zr.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", file.Name, err)
		}
		files[file.Name] = content
	}

	if _, ok := files["demo/"]; !ok {
		t.Fatal("expected demo/ directory entry in archive")
	}
	if got := string(files["demo/SKILL.md"]); got != "# Demo\n" {
		t.Fatalf("unexpected SKILL.md content %q", got)
	}
	if _, ok := files["demo/assets/"]; !ok {
		t.Fatal("expected demo/assets/ directory entry in archive")
	}
	if got := files["demo/assets/logo.png"]; !bytes.Equal(got, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("unexpected binary content %v", got)
	}
}

func TestFileTreeListDirectoryWithoutTrailingSlash(t *testing.T) {
	now := time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC)
	ts, _ := newTestServerWithFileTree(services.NewFileTreeServiceWithRepo(stubFileTreeRepo{
		readFn: func(_ context.Context, _ uuid.UUID, path string, _ int) (*models.FileTreeEntry, error) {
			if path != "/projects/demo" {
				return nil, services.ErrEntryNotFound
			}
			return &models.FileTreeEntry{
				Path:        "/projects/demo",
				Kind:        "directory",
				IsDirectory: true,
				ContentType: "directory",
				Version:     1,
				CreatedAt:   now,
				UpdatedAt:   now,
			}, nil
		},
		listFn: func(_ context.Context, _ uuid.UUID, path string, _ int) ([]models.FileTreeEntry, error) {
			if path != "/projects/demo" {
				return nil, services.ErrEntryNotFound
			}
			return []models.FileTreeEntry{{
				Path:          "/projects/demo/README.md",
				Kind:          "file",
				Content:       "# Demo\n",
				ContentType:   "text/markdown",
				Version:       1,
				MinTrustLevel: models.TrustLevelGuest,
				CreatedAt:     now,
				UpdatedAt:     now,
			}}, nil
		},
	}))
	defer ts.Close()

	resp, err := authGet(ts, "/api/tree/projects/demo")
	if err != nil {
		t.Fatalf("GET /api/tree/projects/demo: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := parseJSON(resp)
	children, ok := body["children"].([]interface{})
	if !ok {
		t.Fatalf("expected children array, got %#v", body["children"])
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	child, ok := children[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected child object, got %#v", children[0])
	}
	if child["path"] != "/projects/demo/README.md" {
		t.Fatalf("unexpected child path: %#v", child["path"])
	}
}

func TestFileTreeWriteProtectedSystemPortabilitySkill(t *testing.T) {
	ts, _ := newTestServerWithFileTree(&services.FileTreeService{})
	defer ts.Close()

	resp, err := authPut(ts, "/api/tree/skills/portability/chatgpt/SKILL.md", map[string]interface{}{
		"content":   "override",
		"mime_type": "text/markdown",
	})
	if err != nil {
		t.Fatalf("PUT /api/tree/skills/portability/chatgpt/SKILL.md: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestFileTreeDeleteProtectedSystemPortabilitySkill(t *testing.T) {
	ts, _ := newTestServerWithFileTree(&services.FileTreeService{})
	defer ts.Close()

	resp, err := authDelete(ts, "/api/tree/skills/portability/chatgpt/SKILL.md")
	if err != nil {
		t.Fatalf("DELETE /api/tree/skills/portability/chatgpt/SKILL.md: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// 5. Vault
// ---------------------------------------------------------------------------

func TestVaultListScopes(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/vault/scopes")
	if err != nil {
		t.Fatalf("GET /api/vault/scopes: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestVaultReadSecret(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/vault/auth.github")
	if err != nil {
		t.Fatalf("GET /api/vault/auth.github: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestVaultWrite(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authPut(ts, "/api/vault/auth.test", map[string]string{"data": "secret123"})
	if err != nil {
		t.Fatalf("PUT /api/vault/auth.test: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ---------------------------------------------------------------------------
// 6. Memory
// ---------------------------------------------------------------------------

func TestMemoryGetProfile(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/memory/profile")
	if err != nil {
		t.Fatalf("GET /api/memory/profile: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestMemoryUpdateProfile(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	updateBody := map[string]interface{}{
		"display_name": "New Name",
		"preferences":  map[string]string{"theme": "dark"},
	}
	resp, err := authPut(ts, "/api/memory/profile", updateBody)
	if err != nil {
		t.Fatalf("PUT /api/memory/profile: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ---------------------------------------------------------------------------
// 7. Projects
// ---------------------------------------------------------------------------

func TestProjectsList(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/projects")
	if err != nil {
		t.Fatalf("GET /api/projects: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestProjectGetByName(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/projects/my-app")
	if err != nil {
		t.Fatalf("GET /api/projects/my-app: %v", err)
	}
	// Handler calls real service; test server has no database, so expect 500.
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestProjectCreateAndLog(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	// Handler calls real service; test server has no database, so expect 500.

	// Create
	resp, err := authPost(ts, "/api/projects", map[string]string{"name": "proj1"})
	if err != nil {
		t.Fatalf("POST /api/projects: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Log action
	logBody := map[string]string{
		"message": "deployed v1.2.3",
		"level":   "info",
	}
	resp, err = authPost(ts, "/api/projects/proj1/log", logBody)
	if err != nil {
		t.Fatalf("POST /api/projects/proj1/log: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ---------------------------------------------------------------------------
// 8. Inbox
// ---------------------------------------------------------------------------

func TestInboxSendAndRead(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	// Handler calls real service; test server has no database, so expect 500.

	// Send message
	msgBody := map[string]string{
		"to":      "worker:planner@hub",
		"subject": "Task assignment",
		"body":    "Please review PR #42",
	}
	resp, err := authPost(ts, "/api/inbox/send", msgBody)
	if err != nil {
		t.Fatalf("POST /api/inbox/send: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Read inbox
	resp, err = authGet(ts, "/api/inbox/assistant")
	if err != nil {
		t.Fatalf("GET /api/inbox/assistant: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestInboxSendValidation(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	// Missing required fields -- the handler uses respondValidationError (422)
	resp, err := authPost(ts, "/api/inbox/send", map[string]string{
		"subject": "no recipient or body",
	})
	if err != nil {
		t.Fatalf("POST /api/inbox/send: %v", err)
	}
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestInboxArchive(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authPut(ts, "/api/inbox/00000000-0000-0000-0000-000000000001/archive", nil)
	if err != nil {
		t.Fatalf("PUT /api/inbox/some-msg-id/archive: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ---------------------------------------------------------------------------
// 9. Dashboard
// ---------------------------------------------------------------------------

func TestDashboardStats(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/dashboard/stats")
	if err != nil {
		t.Fatalf("GET /api/dashboard/stats: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	expectedFields := []string{"connections", "skills", "devices", "projects", "weekly_activity", "pending"}
	for _, f := range expectedFields {
		if _, ok := body[f]; !ok {
			t.Errorf("expected field %s in dashboard stats", f)
		}
	}
}

func TestDashboardStatsWithoutAuth(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/dashboard/stats")
	if err != nil {
		t.Fatalf("GET /api/dashboard/stats (no auth): %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ---------------------------------------------------------------------------
// Additional edge cases
// ---------------------------------------------------------------------------

func TestSearchWithoutQuery(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	// The handler uses respondValidationError (422)
	resp, err := authGet(ts, "/api/search")
	if err != nil {
		t.Fatalf("GET /api/search: %v", err)
	}
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing query, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSearchWithQuery(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/search?q=hello")
	if err != nil {
		t.Fatalf("GET /api/search?q=hello: %v", err)
	}
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 (no service), got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestMalformedJSONBody(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/tokens", bytes.NewReader([]byte("{invalid json")))
	req.Header.Set("Authorization", "Bearer "+generateTestJWT())
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/tokens (bad json): %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestTokenGetInvalidID(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/tokens/not-a-uuid")
	if err != nil {
		t.Fatalf("GET /api/tokens/not-a-uuid: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestTokenGetNonExistent(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/tokens/00000000-0000-0000-0000-000000000099")
	if err != nil {
		t.Fatalf("GET /api/tokens/{id}: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
