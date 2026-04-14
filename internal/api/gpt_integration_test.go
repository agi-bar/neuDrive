package api

import (
	"fmt"
	"net/http"
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// Agent API integration tests against a live server.
// These tests verify the unified /agent/* endpoints used by both the
// browser extension/SDK and ChatGPT GPT Actions.
//
// Run with:
//   NEUDRIVE_TEST_URL=http://localhost:8080 go test ./internal/api/ -run TestAgent_FullLifecycle -v -count=1
//
// Requires: docker compose up (server + database running)
// ---------------------------------------------------------------------------

func TestAgent_FullLifecycle(t *testing.T) {
	base := skipIfNoServer(t)
	_ = base

	slug := fmt.Sprintf("agent-test-%d", os.Getpid())
	email := slug + "@test.local"
	password := "agentpass1234"

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
			"name": "agent-test", "scopes": []string{"admin"},
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
		status, body := apiCall(t, "GET", "/agent/memory/profile", scopedToken, nil)
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

	// -----------------------------------------------------------------------
	// Search (POST body — used by ChatGPT Actions)
	// -----------------------------------------------------------------------
	t.Run("Search_POST", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/agent/search", scopedToken, map[string]any{
			"query": "test query",
		})
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		if body["query"] != "test query" {
			t.Errorf("query: got %v", body["query"])
		}
	})

	t.Run("Search_POST_MissingQuery", func(t *testing.T) {
		status, _ := apiCall(t, "POST", "/agent/search", scopedToken, map[string]any{})
		if status != 422 {
			t.Fatalf("expected 422, got %d", status)
		}
	})

	// -----------------------------------------------------------------------
	// Projects
	// -----------------------------------------------------------------------
	t.Run("ListProjects", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/agent/projects", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	t.Run("GetProject", func(t *testing.T) {
		// Create a project first
		apiCall(t, "POST", "/agent/projects", scopedToken, map[string]any{"name": "test-proj"})

		status, body := apiCall(t, "GET", "/agent/projects/test-proj", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	t.Run("LogAction", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/agent/projects/test-proj/log", scopedToken, map[string]any{
			"action": "test", "summary": "Agent API log entry",
		})
		if status != 201 {
			t.Fatalf("expected 201, got %d: %v", status, body)
		}
	})

	// -----------------------------------------------------------------------
	// Skills
	// -----------------------------------------------------------------------
	t.Run("ListSkills", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/agent/skills", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	// -----------------------------------------------------------------------
	// Devices
	// -----------------------------------------------------------------------
	t.Run("ListDevices", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/agent/devices", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	// -----------------------------------------------------------------------
	// Messaging
	// -----------------------------------------------------------------------
	t.Run("SendMessage", func(t *testing.T) {
		status, body := apiCall(t, "POST", "/agent/inbox/send", scopedToken, map[string]any{
			"to": "assistant", "subject": "Agent Test", "body": "Hello from agent",
		})
		if status != 201 {
			t.Fatalf("expected 201, got %d: %v", status, body)
		}
	})

	t.Run("GetInbox", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/agent/inbox", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
		_ = body
	})

	// -----------------------------------------------------------------------
	// Vault
	// -----------------------------------------------------------------------
	t.Run("ListSecrets", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/agent/vault/scopes", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	// -----------------------------------------------------------------------
	// Dashboard
	// -----------------------------------------------------------------------
	t.Run("DashboardStats", func(t *testing.T) {
		status, body := apiCall(t, "GET", "/agent/dashboard/stats", scopedToken, nil)
		if status != 200 {
			t.Fatalf("expected 200, got %d: %v", status, body)
		}
	})

	// -----------------------------------------------------------------------
	// Auth enforcement
	// -----------------------------------------------------------------------
	t.Run("NoToken_Returns401", func(t *testing.T) {
		endpoints := []struct {
			method, path string
		}{
			{"GET", "/agent/memory/profile"},
			{"GET", "/agent/projects"},
			{"GET", "/agent/skills"},
			{"GET", "/agent/devices"},
			{"GET", "/agent/inbox"},
			{"GET", "/agent/vault/scopes"},
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
		status, _ := apiCall(t, "GET", "/agent/memory/profile", "ndt_invalid_token_12345", nil)
		if status != 401 {
			t.Fatalf("expected 401, got %d", status)
		}
	})
}
