package api

import (
	"fmt"
	"net/http"
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// GPT Actions API integration tests against a live server.
//
// Run with:
//   AGENTHUB_TEST_URL=http://localhost:8080 go test ./internal/api/ -run TestGPT -v -count=1
//
// Requires: docker compose up (server + database running)
// ---------------------------------------------------------------------------

func TestGPT_FullLifecycle(t *testing.T) {
	base := skipIfNoServer(t)
	_ = base

	slug := fmt.Sprintf("gpt-test-%d", os.Getpid())
	email := slug + "@test.local"
	password := "gptpass1234"

	var jwt string
	var scopedToken string

	// -----------------------------------------------------------------------
	// Setup: register user + create scoped token
	// -----------------------------------------------------------------------
	t.Run("Setup", func(t *testing.T) {
		// Register
		status, body := apiCall(t, "POST", "/api/auth/register", "", map[string]any{
			"slug": slug, "email": email, "password": password,
		})
		if status != 200 && status != 201 {
			t.Fatalf("register: got %d: %v", status, body)
		}
		jwt = mustStr(t, body, "access_token")

		// Create scoped token with admin scope
		status, body = apiCall(t, "POST", "/api/tokens", jwt, map[string]any{
			"name": "gpt-test", "scopes": []string{"admin"},
			"max_trust_level": 4, "expires_in_days": 1,
		})
		if status != 201 {
			t.Fatalf("create token: got %d: %v", status, body)
		}
		scopedToken = mustStr(t, body, "token")
	})

	// -----------------------------------------------------------------------
	// Profile endpoints
	// -----------------------------------------------------------------------
	t.Run("GetProfile", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/gpt/profile", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		gotSlug := mustStr(t, body, "slug")
		if gotSlug != slug {
			t.Errorf("slug: got %q, want %q", gotSlug, slug)
		}
		if _, ok := body["display_name"]; !ok {
			t.Error("missing display_name")
		}
	})

	t.Run("GetPreferences", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/gpt/preferences", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		if _, ok := body["timezone"]; !ok {
			t.Error("missing timezone")
		}
		if _, ok := body["language"]; !ok {
			t.Error("missing language")
		}
	})

	// -----------------------------------------------------------------------
	// Search
	// -----------------------------------------------------------------------
	t.Run("Search", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/gpt/search", scopedToken, map[string]any{
			"query": "test query",
		})
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		if body["query"] != "test query" {
			t.Errorf("query: got %v", body["query"])
		}
	})

	t.Run("Search_MissingQuery", func(t *testing.T) {
		status, _ := apiCall(t, "POST", "/gpt/search", scopedToken, map[string]any{})
		if status != 400 {
			t.Fatalf("expected 400, got %d", status)
		}
	})

	// -----------------------------------------------------------------------
	// Projects
	// -----------------------------------------------------------------------
	t.Run("ListProjects", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/gpt/projects", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	t.Run("GetProject", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/gpt/project/test-proj", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		if body["name"] != "test-proj" {
			t.Errorf("name: got %v", body["name"])
		}
	})

	t.Run("Log", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/gpt/log", scopedToken, map[string]any{
			"project": "test-proj", "action": "test", "summary": "GPT log entry",
		})
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		if body["status"] != "logged" {
			t.Errorf("status: got %v", body["status"])
		}
	})

	t.Run("Log_MissingProject", func(t *testing.T) {
		status, _ := apiCall(t, "POST", "/gpt/log", scopedToken, map[string]any{
			"action": "test",
		})
		if status != 400 {
			t.Fatalf("expected 400, got %d", status)
		}
	})

	// -----------------------------------------------------------------------
	// Skills
	// -----------------------------------------------------------------------
	t.Run("ListSkills", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/gpt/skills", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	t.Run("GetSkill", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/gpt/skill/test-skill", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		if body["name"] != "test-skill" {
			t.Errorf("name: got %v", body["name"])
		}
	})

	// -----------------------------------------------------------------------
	// Devices
	// -----------------------------------------------------------------------
	t.Run("ListDevices", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/gpt/devices", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	t.Run("CallDevice", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/gpt/device/test-light", scopedToken, map[string]any{
			"action": "on", "params": map[string]any{"brightness": 80},
		})
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		if body["device"] != "test-light" {
			t.Errorf("device: got %v", body["device"])
		}
		if body["action"] != "on" {
			t.Errorf("action: got %v", body["action"])
		}
	})

	t.Run("CallDevice_MissingAction", func(t *testing.T) {
		status, _ := apiCall(t, "POST", "/gpt/device/test-light", scopedToken, map[string]any{})
		if status != 400 {
			t.Fatalf("expected 400, got %d", status)
		}
	})

	// -----------------------------------------------------------------------
	// Messaging
	// -----------------------------------------------------------------------
	t.Run("SendMessage", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/gpt/message", scopedToken, map[string]any{
			"to": "assistant", "subject": "GPT Test", "body": "Hello from GPT",
		})
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		if body["status"] != "sent" {
			t.Errorf("status: got %v", body["status"])
		}
	})

	t.Run("SendMessage_MissingTo", func(t *testing.T) {
		status, _ := apiCall(t, "POST", "/gpt/message", scopedToken, map[string]any{
			"subject": "test",
		})
		if status != 400 {
			t.Fatalf("expected 400, got %d", status)
		}
	})

	t.Run("GetInbox", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/gpt/inbox", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	// -----------------------------------------------------------------------
	// Vault
	// -----------------------------------------------------------------------
	t.Run("ListSecrets", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/gpt/secrets", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	t.Run("GetSecret", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/gpt/secret/auth.github", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		if body["scope"] != "auth.github" {
			t.Errorf("scope: got %v", body["scope"])
		}
	})

	// -----------------------------------------------------------------------
	// Auth enforcement
	// -----------------------------------------------------------------------
	t.Run("NoToken_Returns401", func(t *testing.T) {
		endpoints := []struct {
			method, path string
		}{
			{"GET", "/gpt/profile"},
			{"GET", "/gpt/preferences"},
			{"GET", "/gpt/projects"},
			{"GET", "/gpt/skills"},
			{"GET", "/gpt/devices"},
			{"GET", "/gpt/inbox"},
			{"GET", "/gpt/secrets"},
		}
		for _, ep := range endpoints {
			req, _ := http.NewRequest(ep.method, baseURL()+ep.path, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", ep.method, ep.path, err)
			}
			resp.Body.Close()
			if resp.StatusCode != 401 {
				t.Errorf("%s %s: expected 401, got %d", ep.method, ep.path, resp.StatusCode)
			}
		}
	})

	t.Run("InvalidToken_Returns401", func(t *testing.T) {
		status, _ := apiCall(t, "GET", "/gpt/profile", "aht_invalid_token_12345", nil)
		if status != 401 {
			t.Fatalf("expected 401, got %d", status)
		}
	})
}
