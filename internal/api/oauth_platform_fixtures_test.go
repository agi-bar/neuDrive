package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

type capturedRequestFixture struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func loadCapturedRequestFixture(t *testing.T, path string) capturedRequestFixture {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", path, err)
	}

	var fixture capturedRequestFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("failed to parse fixture %s: %v", path, err)
	}
	return fixture
}

func newRequestFromFixture(t *testing.T, path string) *http.Request {
	t.Helper()

	fixture := loadCapturedRequestFixture(t, path)
	req := httptest.NewRequest(fixture.Method, fixture.URL, strings.NewReader(fixture.Body))

	parsedURL, err := url.Parse(fixture.URL)
	if err != nil {
		t.Fatalf("failed to parse fixture URL %s: %v", fixture.URL, err)
	}
	req.Host = parsedURL.Host

	for key, value := range fixture.Headers {
		req.Header.Set(key, value)
	}

	return req
}

func decodeResponseBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return body
}

func TestCapturedOAuthRequests_CodexDynamicRegisterRequestParses(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/codex/oauth_register.json")

	parsed, err := parseOAuthDynamicRegisterRequest(req)
	if err != nil {
		t.Fatalf("parseOAuthDynamicRegisterRequest returned error: %v", err)
	}

	if parsed.ClientName != "Codex" {
		t.Fatalf("expected client_name Codex, got %q", parsed.ClientName)
	}
	if parsed.TokenEndpointAuthMethod != "none" {
		t.Fatalf("expected token_endpoint_auth_method none, got %q", parsed.TokenEndpointAuthMethod)
	}
	if len(parsed.RedirectURIs) != 1 || parsed.RedirectURIs[0] != "http://127.0.0.1:51431/callback" {
		t.Fatalf("unexpected redirect_uris: %v", parsed.RedirectURIs)
	}
	if len(parsed.GrantTypes) != 2 || parsed.GrantTypes[0] != "authorization_code" || parsed.GrantTypes[1] != "refresh_token" {
		t.Fatalf("unexpected grant_types: %v", parsed.GrantTypes)
	}
	if len(parsed.ResponseTypes) != 1 || parsed.ResponseTypes[0] != "code" {
		t.Fatalf("unexpected response_types: %v", parsed.ResponseTypes)
	}
}

func TestCapturedOAuthRequests_ClaudeCodeInitializeGetsOAuthChallenge(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/claude-code/mcp_initialize.json")
	rec := httptest.NewRecorder()

	s := &Server{}
	s.handleMCPEndpoint(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated initialize, got %d", rec.Code)
	}

	authHeader := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(authHeader, `Bearer resource_metadata="http://127.0.0.1:8765/.well-known/oauth-protected-resource"`) {
		t.Fatalf("expected WWW-Authenticate to point to protected resource metadata, got %q", authHeader)
	}

	body := decodeResponseBody(t, rec)
	if body["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc 2.0 response, got %v", body)
	}
}

func TestCapturedOAuthRequests_ProtectedResourceMetadataForClaudeCode(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/claude-code/protected_resource_metadata.json")
	rec := httptest.NewRecorder()

	s := &Server{}
	s.handleProtectedResourceMetadata(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := decodeResponseBody(t, rec)
	if body["resource"] != "http://127.0.0.1:8765/mcp" {
		t.Fatalf("expected resource metadata for captured host, got %v", body["resource"])
	}
}

func TestCapturedOAuthRequests_AuthorizationServerMetadataSupportsPlatformClients(t *testing.T) {
	fixtures := []string{
		"testdata/oauth/claude-code/authorization_server_metadata.json",
		"testdata/oauth/codex/authorization_server_metadata.json",
	}

	for _, fixturePath := range fixtures {
		t.Run(fixturePath, func(t *testing.T) {
			req := newRequestFromFixture(t, fixturePath)
			rec := httptest.NewRecorder()

			s := &Server{}
			s.handleAuthorizationServerMetadata(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}

			body := decodeResponseBody(t, rec)
			methods, ok := body["token_endpoint_auth_methods_supported"].([]any)
			if !ok {
				t.Fatalf("missing token_endpoint_auth_methods_supported: %v", body)
			}

			required := map[string]bool{
				"client_secret_basic": false,
				"client_secret_post":  false,
				"none":                false,
			}
			for _, method := range methods {
				if key, ok := method.(string); ok {
					if _, exists := required[key]; exists {
						required[key] = true
					}
				}
			}
			for method, present := range required {
				if !present {
					t.Fatalf("expected %s in token_endpoint_auth_methods_supported, got %v", method, methods)
				}
			}
		})
	}
}
