package api

import (
	"bytes"
	"net/http"
	"testing"
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
	if body["service"] != "agenthub" {
		t.Errorf("expected service=agenthub, got %v", body["service"])
	}
	if _, ok := body["time"]; !ok {
		t.Error("expected time field in health response")
	}
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

func TestTokenCreateListRevoke(t *testing.T) {
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["is_dir"] != true {
		t.Errorf("expected root to be a directory, got %v", body["is_dir"])
	}
}

func TestFileTreeReadFile(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/tree/skills/test.md")
	if err != nil {
		t.Fatalf("GET /api/tree/skills/test.md: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["path"] == nil {
		t.Error("expected path field")
	}
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["content"] != "# Hello\nThis is a test." {
		t.Errorf("expected written content, got %v", body["content"])
	}
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["status"] != "deleted" {
		t.Errorf("expected status=deleted, got %v", body["status"])
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if _, ok := body["scopes"]; !ok {
		t.Error("expected scopes field")
	}
}

func TestVaultReadSecret(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/vault/auth.github")
	if err != nil {
		t.Fatalf("GET /api/vault/auth.github: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["scope"] != "auth.github" {
		t.Errorf("expected scope=auth.github, got %v", body["scope"])
	}
}

func TestVaultWrite(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authPut(ts, "/api/vault/auth.test", map[string]string{"data": "secret123"})
	if err != nil {
		t.Fatalf("PUT /api/vault/auth.test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["scope"] != "auth.test" {
		t.Errorf("expected scope=auth.test, got %v", body["scope"])
	}
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["user_id"] == nil {
		t.Error("expected user_id field")
	}
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["display_name"] != "New Name" {
		t.Errorf("expected display_name=New Name, got %v", body["display_name"])
	}
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if _, ok := body["projects"]; !ok {
		t.Error("expected projects field")
	}
}

func TestProjectGetByName(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := authGet(ts, "/api/projects/my-app")
	if err != nil {
		t.Fatalf("GET /api/projects/my-app: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["name"] != "my-app" {
		t.Errorf("expected name=my-app, got %v", body["name"])
	}
}

func TestProjectCreateAndLog(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	// Create
	resp, err := authPost(ts, "/api/projects", map[string]string{"name": "proj1"})
	if err != nil {
		t.Fatalf("POST /api/projects: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
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
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ---------------------------------------------------------------------------
// 8. Inbox
// ---------------------------------------------------------------------------

func TestInboxSendAndRead(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

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
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	created := parseJSON(resp)
	if created["to"] != "worker:planner@hub" {
		t.Errorf("expected to=worker:planner@hub, got %v", created["to"])
	}

	// Read inbox
	resp, err = authGet(ts, "/api/inbox/assistant")
	if err != nil {
		t.Fatalf("GET /api/inbox/assistant: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["role"] != "assistant" {
		t.Errorf("expected role=assistant, got %v", body["role"])
	}
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

	resp, err := authPut(ts, "/api/inbox/some-msg-id/archive", nil)
	if err != nil {
		t.Fatalf("PUT /api/inbox/some-msg-id/archive: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["status"] != "archived" {
		t.Errorf("expected status=archived, got %v", body["status"])
	}
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
	expectedFields := []string{"file_count", "vault_scopes", "connections", "roles", "projects", "unread_messages", "registered_devices"}
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := parseJSON(resp)
	if body["query"] != "hello" {
		t.Errorf("expected query=hello, got %v", body["query"])
	}
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
