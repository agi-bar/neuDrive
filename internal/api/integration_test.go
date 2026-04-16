package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Full integration tests against a live server.
//
// Run with:
//   NEUDRIVE_TEST_URL=http://localhost:8080 go test ./internal/api/ -run TestIntegration -v -count=1
//
// Requires: docker compose up (server + database running)
// ---------------------------------------------------------------------------

func baseURL() string {
	u := os.Getenv("NEUDRIVE_TEST_URL")
	if u == "" {
		return ""
	}
	return strings.TrimRight(u, "/")
}

func skipIfNoServer(t *testing.T) string {
	t.Helper()
	u := baseURL()
	if u == "" {
		t.Skip("NEUDRIVE_TEST_URL not set; skipping integration test")
	}
	return u
}

// apiCall is a helper for integration tests against the live server.
func apiCall(t *testing.T, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	base := baseURL()

	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, base+path, bodyReader)
	if err != nil {
		t.Fatalf("NewRequest %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(raw, &result)

	// Auto-unwrap envelope
	if result != nil {
		if ok, has := result["ok"]; has {
			if okBool, isBool := ok.(bool); isBool && okBool {
				if data, hasData := result["data"]; hasData {
					if dataMap, isMap := data.(map[string]any); isMap {
						return resp.StatusCode, dataMap
					}
				}
			}
		}
	}

	return resp.StatusCode, result
}

func mustStr(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing key %q in response: %v", key, m)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("key %q is not a string: %T = %v", key, v, v)
	}
	return s
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestIntegration_FullLifecycle(t *testing.T) {
	base := skipIfNoServer(t)
	_ = base

	slug := fmt.Sprintf("itest-%d", os.Getpid())
	email := slug + "@test.local"
	password := "testpass1234"

	var jwt string
	var userID string

	// -----------------------------------------------------------------------
	// 1. Register
	// -----------------------------------------------------------------------
	t.Run("Register", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/api/auth/register", "", map[string]any{
			"slug": slug, "email": email, "password": password,
		})
		// Auth endpoints return directly (no {ok, data} envelope)
		if status != 200 && status != 201 {
			t.Fatalf("register: expected 200/201, got %d: %v", status, body)
		}
		jwt = mustStr(t, body, "access_token")
		user := body["user"].(map[string]any)
		userID = mustStr(t, user, "id")
		if jwt == "" || userID == "" {
			t.Fatalf("register: empty jwt or userID")
		}
	})

	// -----------------------------------------------------------------------
	// 2. Login
	// -----------------------------------------------------------------------
	t.Run("Login", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/api/auth/login", "", map[string]any{
			"email": email, "password": password,
		})
		if status != 200 {
			t.Fatalf("login: expected 200, got %d: %v", status, body)
		}
		newJWT := mustStr(t, body, "access_token")
		if newJWT == "" {
			t.Fatal("login: empty access_token")
		}
		// Use fresh JWT from login
		jwt = newJWT
	})

	// -----------------------------------------------------------------------
	// 3. Get profile (auth/me)
	// -----------------------------------------------------------------------
	t.Run("GetMe", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/auth/me", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		gotSlug := mustStr(t, body, "slug")
		if gotSlug != slug {
			t.Errorf("slug: got %q, want %q", gotSlug, slug)
		}
	})

	// -----------------------------------------------------------------------
	// 4. Dashboard stats
	// -----------------------------------------------------------------------
	t.Run("DashboardStats", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/dashboard/stats", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		// Should have numeric keys
		for _, key := range []string{"connections", "files", "memory", "profile", "skills", "projects", "inbox"} {
			if _, ok := body[key]; !ok {
				t.Errorf("missing key %q", key)
			}
		}
	})

	// -----------------------------------------------------------------------
	// 5. Connections CRUD
	// -----------------------------------------------------------------------
	var connID string

	t.Run("Connections_Create", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/api/connections", jwt, map[string]any{
			"name": "Test Claude", "type": "claude", "trust_level": 4,
		})
		if status != 201 {
			t.Fatalf("expected 201, got %d: %v", status, body)
		}
		conn := body["connection"].(map[string]any)
		connID = mustStr(t, conn, "id")
		if connID == "" {
			t.Fatal("empty connection ID")
		}
		apiKey := mustStr(t, body, "api_key")
		if !strings.HasPrefix(apiKey, "ahk_") {
			t.Errorf("api_key should start with ahk_, got %q", apiKey[:8])
		}
	})

	t.Run("Connections_List", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/connections", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		conns := body["connections"].([]any)
		if len(conns) != 1 {
			t.Errorf("expected 1 connection, got %d", len(conns))
		}
	})

	t.Run("Connections_Update", func(t *testing.T) {
		status, body := apiCall(t, "PUT", "/api/connections/"+connID, jwt, map[string]any{
			"name": "Updated Claude", "trust_level": 3,
		})
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	t.Run("Connections_Delete", func(t *testing.T) {
		status, _ := apiCall(t, "DELETE", "/api/connections/"+connID, jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d", status)
		}
		// Verify deleted
		status, body := apiCall(t, "GET", "/api/connections", jwt, nil)
		if status != 200 {
			t.Fatalf("list after delete: expected 200, got %d", status)
		}
		conns, _ := body["connections"].([]any)
		if len(conns) != 0 {
			t.Errorf("expected 0 connections after delete, got %d", len(conns))
		}
	})

	// -----------------------------------------------------------------------
	// 6. Memory Profile CRUD
	// -----------------------------------------------------------------------
	t.Run("MemoryProfile_Update", func(t *testing.T) {
		status, _ := apiCall(t, "PUT", "/api/memory/profile", jwt, map[string]any{
			"preferences": map[string]string{
				"writing_style": "不用句号结尾、信息密度高",
				"principles":    "先思考再行动",
			},
		})
		if status != 200 {
			t.Fatalf("expected 200, got %d", status)
		}
	})

	t.Run("MemoryProfile_Read", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/memory/profile", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		prefs, ok := body["preferences"].(map[string]any)
		if !ok {
			t.Fatalf("preferences is not a map: %v", body["preferences"])
		}
		if prefs["writing_style"] != "不用句号结尾、信息密度高" {
			t.Errorf("writing_style: got %q", prefs["writing_style"])
		}
		if prefs["principles"] != "先思考再行动" {
			t.Errorf("principles: got %q", prefs["principles"])
		}
	})

	// -----------------------------------------------------------------------
	// 7. Vault CRUD (encrypt + decrypt round-trip)
	// -----------------------------------------------------------------------
	t.Run("Vault_Write", func(t *testing.T) {
		status, _ := apiCall(t, "PUT", "/api/vault/auth.openai", jwt, map[string]any{
			"data": "sk-test-secret-key-12345",
		})
		if status != 200 {
			t.Fatalf("expected 200, got %d", status)
		}
	})

	t.Run("Vault_Read", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/vault/auth.openai", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		data := mustStr(t, body, "data")
		if data != "sk-test-secret-key-12345" {
			t.Errorf("vault data: got %q, want %q", data, "sk-test-secret-key-12345")
		}
	})

	t.Run("Vault_ListScopes", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/vault/scopes", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		scopes, _ := body["scopes"].([]any)
		if len(scopes) < 1 {
			t.Errorf("expected at least 1 scope, got %d", len(scopes))
		}
	})

	t.Run("Vault_Delete", func(t *testing.T) {
		status, _ := apiCall(t, "DELETE", "/api/vault/auth.openai", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d", status)
		}
		// Verify deleted (should 404)
		status, _ = apiCall(t, "GET", "/api/vault/auth.openai", jwt, nil)
		if status != 404 {
			t.Errorf("expected 404 after delete, got %d", status)
		}
	})

	// -----------------------------------------------------------------------
	// 8. Projects CRUD
	// -----------------------------------------------------------------------
	t.Run("Projects_Create", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/api/projects", jwt, map[string]any{
			"name": "test-project",
		})
		if status != 201 {
			t.Fatalf("expected 201, got %d: %v", status, body)
		}
		name := mustStr(t, body, "name")
		if name != "test-project" {
			t.Errorf("project name: got %q", name)
		}
	})

	var projectUpdatedAt string

	t.Run("Projects_List", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/projects", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		projects, _ := body["projects"].([]any)
		if len(projects) != 1 {
			t.Errorf("expected 1 project, got %d", len(projects))
		}
		project, _ := projects[0].(map[string]any)
		projectUpdatedAt = mustStr(t, project, "updated_at")
		if projectUpdatedAt == "" {
			t.Fatal("expected project updated_at to be populated")
		}
	})

	t.Run("Projects_AppendLog", func(t *testing.T) {
		time.Sleep(5 * time.Millisecond)
		status, _ := apiCall(t, "POST", "/api/projects/test-project/log", jwt, map[string]any{
			"source": "claude", "action": "wrote_article", "summary": "写了一篇测试文章",
			"tags": []string{"writing", "test"},
		})
		if status != 201 {
			t.Fatalf("expected 201, got %d", status)
		}
	})

	t.Run("Projects_ListAfterLog", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/projects", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		projects, _ := body["projects"].([]any)
		if len(projects) != 1 {
			t.Fatalf("expected 1 project, got %d", len(projects))
		}
		project, _ := projects[0].(map[string]any)
		updatedAt := mustStr(t, project, "updated_at")
		if updatedAt == projectUpdatedAt {
			t.Fatalf("expected project updated_at to change after append log, still %q", updatedAt)
		}
	})

	t.Run("Projects_GetDetail", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/projects/test-project", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		project, _ := body["project"].(map[string]any)
		if project == nil {
			t.Fatal("missing 'project' in response")
		}
		logs, _ := body["logs"].([]any)
		if len(logs) != 1 {
			t.Errorf("expected 1 log entry, got %d", len(logs))
		}
	})

	t.Run("Projects_Archive", func(t *testing.T) {
		status, _ := apiCall(t, "PUT", "/api/projects/test-project/archive", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d", status)
		}
		// Verify archived (should not appear in active list)
		status, body := apiCall(t, "GET", "/api/projects", jwt, nil)
		if status != 200 {
			t.Fatalf("list after archive: expected 200, got %d", status)
		}
		projects, _ := body["projects"].([]any)
		if projects == nil {
			// null means empty, which is correct (archived project not in active list)
			return
		}
		if len(projects) != 0 {
			t.Errorf("expected 0 active projects after archive, got %d", len(projects))
		}
	})

	// -----------------------------------------------------------------------
	// 9. Tokens CRUD
	// -----------------------------------------------------------------------
	var tokenID string
	var scopedToken string

	t.Run("Tokens_Create", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/api/tokens", jwt, map[string]any{
			"name": "integration-test", "scopes": []string{"admin"},
			"max_trust_level": 4, "expires_in_days": 7,
		})
		if status != 201 {
			t.Fatalf("expected 201, got %d: %v", status, body)
		}
		scopedToken = mustStr(t, body, "token")
		if !strings.HasPrefix(scopedToken, "ndt_") {
			t.Errorf("token should start with ndt_, got %q", scopedToken[:8])
		}
		st := body["scoped_token"].(map[string]any)
		tokenID = mustStr(t, st, "id")
	})

	t.Run("Tokens_List", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/tokens", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		tokens, _ := body["tokens"].([]any)
		if len(tokens) < 1 {
			t.Errorf("expected at least 1 token, got %d", len(tokens))
		}
	})

	t.Run("Tokens_GetByID", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/tokens/"+tokenID, jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		name := mustStr(t, body, "name")
		if name != "integration-test" {
			t.Errorf("token name: got %q", name)
		}
	})

	t.Run("Tokens_Revoke", func(t *testing.T) {
		status, _ := apiCall(t, "DELETE", "/api/tokens/"+tokenID, jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d", status)
		}
	})

	// -----------------------------------------------------------------------
	// 10. Inbox (send + read)
	// -----------------------------------------------------------------------
	t.Run("Inbox_Send", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/api/inbox/send", jwt, map[string]any{
			"to": "assistant@" + userID, "subject": "Test Message", "body": "Hello from integration test",
		})
		if status != 201 {
			t.Fatalf("expected 201, got %d: %v", status, body)
		}
		msgID := mustStr(t, body, "id")
		if msgID == "" {
			t.Fatal("empty message ID")
		}
	})

	t.Run("Inbox_Read", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/inbox/assistant", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		// Messages may or may not match our query pattern; just check it doesn't error
	})

	// 12. Roles CRUD
	// -----------------------------------------------------------------------
	t.Run("Roles_List", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/roles", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		roles, _ := body["roles"].([]any)
		// Should have at least the default 'assistant' role
		if len(roles) < 1 {
			t.Errorf("expected at least 1 role (assistant), got %d", len(roles))
		}
	})

	t.Run("Roles_Create", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/api/roles", jwt, map[string]any{
			"name": "worker-research", "role_type": "worker",
			"allowed_paths": []string{"/projects/research/"},
		})
		if status != 201 {
			t.Fatalf("expected 201, got %d: %v", status, body)
		}
	})

	t.Run("Roles_Delete", func(t *testing.T) {
		status, _ := apiCall(t, "DELETE", "/api/roles/worker-research", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d", status)
		}
	})

	// -----------------------------------------------------------------------
	// 13. File Tree CRUD
	// -----------------------------------------------------------------------
	t.Run("FileTree_Write", func(t *testing.T) {
		status, _ := apiCall(t, "PUT", "/api/tree/skills/test-skill/SKILL.md", jwt, map[string]any{
			"content": "# Test Skill\n\nA skill for testing.", "mime_type": "text/markdown",
		})
		if status != 200 {
			t.Fatalf("expected 200, got %d", status)
		}
	})

	t.Run("FileTree_Read", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/tree/skills/test-skill/SKILL.md", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		content, _ := body["content"].(string)
		if !strings.Contains(content, "Test Skill") {
			t.Errorf("content: got %q", content)
		}
	})

	t.Run("FileTree_Search", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/search?q=Test+Skill", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		results, _ := body["results"].([]any)
		if len(results) < 1 {
			t.Errorf("expected at least 1 search result, got %d", len(results))
		}
	})

	t.Run("FileTree_Delete", func(t *testing.T) {
		status, _ := apiCall(t, "DELETE", "/api/tree/skills/test-skill/SKILL.md", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d", status)
		}
	})

	// -----------------------------------------------------------------------
	// 14. Agent API (scoped token auth)
	// -----------------------------------------------------------------------
	// Create a fresh non-revoked token for agent API tests
	var agentToken string

	t.Run("AgentAPI_CreateToken", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/api/tokens", jwt, map[string]any{
			"name": "agent-test", "scopes": []string{"admin"},
			"max_trust_level": 4, "expires_in_days": 1,
		})
		if status != 201 {
			t.Fatalf("expected 201, got %d: %v", status, body)
		}
		agentToken = mustStr(t, body, "token")
	})

	t.Run("AgentAPI_GetProfile", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/agent/memory/profile", agentToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		gotSlug := mustStr(t, body, "slug")
		if gotSlug != slug {
			t.Errorf("slug: got %q, want %q", gotSlug, slug)
		}
	})

	t.Run("AgentAPI_TreeWrite", func(t *testing.T) {
		status, _ := apiCall(t, "PUT", "/agent/tree/notes/test.md", agentToken, map[string]any{
			"content": "Agent wrote this note.", "content_type": "text/markdown",
		})
		if status != 200 {
			t.Fatalf("expected 200, got %d", status)
		}
	})

	t.Run("AgentAPI_TreeRead", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/agent/tree/notes/", agentToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	t.Run("AgentAPI_Search", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/agent/search?q=Agent+wrote", agentToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	t.Run("AgentAPI_VaultWrite_Read", func(t *testing.T) {
		// Write via JWT API
		status, _ := apiCall(t, "PUT", "/api/vault/auth.test-agent", jwt, map[string]any{
			"data": "agent-secret-value",
		})
		if status != 200 {
			t.Fatalf("vault write: expected 200, got %d", status)
		}
		// Read via Agent API
		status, body := apiCall(t, "GET", "/agent/vault/auth.test-agent", agentToken, nil)
		if status != 200 {
			t.Fatalf("agent vault read: expected 200, got %d: %v", status, body)
		}
		data := mustStr(t, body, "data")
		if data != "agent-secret-value" {
			t.Errorf("vault data via agent: got %q", data)
		}
	})

	t.Run("AgentAPI_SendMessage", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/agent/inbox/send", agentToken, map[string]any{
			"to": "assistant", "subject": "Agent Test", "body": "Message from agent API",
		})
		if status != 201 {
			t.Fatalf("expected 201, got %d: %v", status, body)
		}
	})

	t.Run("AgentAPI_GetInbox", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/agent/inbox/assistant", agentToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	// -----------------------------------------------------------------------
	// 15. Export
	// -----------------------------------------------------------------------
	t.Run("Export_JSON", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/export/json", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		// Should contain user data
		if body["user"] == nil && body["profile"] == nil {
			t.Error("export should contain user or profile data")
		}
	})

	// -----------------------------------------------------------------------
	// 16. Memory scratch
	// -----------------------------------------------------------------------
	t.Run("MemoryScratch_Get", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/memory/scratch", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		// scratch key should exist
		if _, ok := body["scratch"]; !ok {
			t.Error("missing 'scratch' key")
		}
	})

	// -----------------------------------------------------------------------
	// 17. Webhooks CRUD
	// -----------------------------------------------------------------------
	var webhookID string

	t.Run("Webhooks_Register", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/api/webhooks", jwt, map[string]any{
			"url": "https://example.com/webhook", "events": []string{"inbox.new"},
		})
		if status != 201 {
			t.Fatalf("expected 201, got %d: %v", status, body)
		}
		// ID is nested under "webhook" key
		wh, _ := body["webhook"].(map[string]any)
		if wh != nil {
			webhookID, _ = wh["id"].(string)
		}
		if webhookID == "" {
			webhookID, _ = body["id"].(string)
		}
		if webhookID == "" {
			t.Fatalf("empty webhook ID; body: %v", body)
		}
	})

	t.Run("Webhooks_List", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/webhooks", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	t.Run("Webhooks_Delete", func(t *testing.T) {
		status, _ := apiCall(t, "DELETE", "/api/webhooks/"+webhookID, jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d", status)
		}
	})

	// -----------------------------------------------------------------------
	// 18. Collaborations
	// -----------------------------------------------------------------------
	t.Run("Collaborations_List", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/collaborations", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	// -----------------------------------------------------------------------
	// 19. Auth: change password + sessions
	// -----------------------------------------------------------------------
	t.Run("Auth_ChangePassword", func(t *testing.T) {
		status, _ := apiCall(t, "POST", "/api/auth/change-password", jwt, map[string]any{
			"old_password": password, "new_password": "newpass5678",
		})
		if status != 200 {
			t.Fatalf("expected 200, got %d", status)
		}
		// Login with new password
		status, body := apiCall(t, "POST", "/api/auth/login", "", map[string]any{
			"email": email, "password": "newpass5678",
		})
		if status != 200 {
			t.Fatalf("login with new password: expected 200, got %d: %v", status, body)
		}
		jwt = mustStr(t, body, "access_token")
	})

	t.Run("Auth_Sessions", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/api/auth/sessions", jwt, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	// -----------------------------------------------------------------------
	// 20. Frontend accessible
	// -----------------------------------------------------------------------
	t.Run("Frontend_Accessible", func(t *testing.T) {
		resp, err := http.Get(baseURL() + "/")
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "neuDrive") {
			t.Error("frontend should contain 'neuDrive'")
		}
	})
}
