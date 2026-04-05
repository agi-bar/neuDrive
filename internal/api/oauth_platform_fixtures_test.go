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
	if len(parsed.RedirectURIs) != 1 || parsed.RedirectURIs[0] != "http://127.0.0.1:57463/callback" {
		t.Fatalf("unexpected redirect_uris: %v", parsed.RedirectURIs)
	}
	if len(parsed.GrantTypes) != 2 || parsed.GrantTypes[0] != "authorization_code" || parsed.GrantTypes[1] != "refresh_token" {
		t.Fatalf("unexpected grant_types: %v", parsed.GrantTypes)
	}
	if len(parsed.ResponseTypes) != 1 || parsed.ResponseTypes[0] != "code" {
		t.Fatalf("unexpected response_types: %v", parsed.ResponseTypes)
	}
}

func TestCapturedOAuthRequests_ChatGPTDynamicRegisterRequestParses(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/chatgpt/oauth_register.json")

	parsed, err := parseOAuthDynamicRegisterRequest(req)
	if err != nil {
		t.Fatalf("parseOAuthDynamicRegisterRequest returned error: %v", err)
	}

	if parsed.ClientName != "ChatGPT" {
		t.Fatalf("expected client_name ChatGPT, got %q", parsed.ClientName)
	}
	if parsed.TokenEndpointAuthMethod != "none" {
		t.Fatalf("expected token_endpoint_auth_method none, got %q", parsed.TokenEndpointAuthMethod)
	}
	if len(parsed.RedirectURIs) != 1 || parsed.RedirectURIs[0] != "https://chatgpt.com/connector/oauth/CAPTURED_CHATGPT_CONNECTOR" {
		t.Fatalf("unexpected redirect_uris: %v", parsed.RedirectURIs)
	}
	if len(parsed.GrantTypes) != 2 || parsed.GrantTypes[0] != "authorization_code" || parsed.GrantTypes[1] != "refresh_token" {
		t.Fatalf("unexpected grant_types: %v", parsed.GrantTypes)
	}
	if len(parsed.ResponseTypes) != 1 || parsed.ResponseTypes[0] != "code" {
		t.Fatalf("unexpected response_types: %v", parsed.ResponseTypes)
	}
}

func TestCapturedOAuthRequests_GeminiCLIDynamicRegisterRequestParses(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/gemini-cli/oauth_register.json")

	parsed, err := parseOAuthDynamicRegisterRequest(req)
	if err != nil {
		t.Fatalf("parseOAuthDynamicRegisterRequest returned error: %v", err)
	}

	if parsed.ClientName != "Gemini CLI MCP Client" {
		t.Fatalf("expected client_name Gemini CLI MCP Client, got %q", parsed.ClientName)
	}
	if parsed.TokenEndpointAuthMethod != "none" {
		t.Fatalf("expected token_endpoint_auth_method none, got %q", parsed.TokenEndpointAuthMethod)
	}
	if len(parsed.RedirectURIs) != 1 || parsed.RedirectURIs[0] != "http://localhost:65290/oauth/callback" {
		t.Fatalf("unexpected redirect_uris: %v", parsed.RedirectURIs)
	}
	if len(parsed.GrantTypes) != 2 || parsed.GrantTypes[0] != "authorization_code" || parsed.GrantTypes[1] != "refresh_token" {
		t.Fatalf("unexpected grant_types: %v", parsed.GrantTypes)
	}
	if len(parsed.ResponseTypes) != 1 || parsed.ResponseTypes[0] != "code" {
		t.Fatalf("unexpected response_types: %v", parsed.ResponseTypes)
	}
	if !strings.Contains(parsed.Scope, "offline_access") {
		t.Fatalf("expected offline_access in scope, got %q", parsed.Scope)
	}
}

func TestCapturedOAuthRequests_ClaudeWebInitializeGetsOAuthChallenge(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/claude-web/mcp_initialize.json")
	rec := httptest.NewRecorder()

	s := &Server{}
	s.handleMCPEndpoint(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated initialize, got %d", rec.Code)
	}

	authHeader := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(authHeader, `Bearer resource_metadata="https://atmospheric-intellectual-sleeping-conducting.trycloudflare.com/.well-known/oauth-protected-resource"`) {
		t.Fatalf("expected WWW-Authenticate to point to protected resource metadata, got %q", authHeader)
	}

	body := decodeResponseBody(t, rec)
	if body["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc 2.0 response, got %v", body)
	}
}

func TestCapturedOAuthRequests_GeminiCLIInitializeGetsOAuthChallenge(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/gemini-cli/mcp_initialize.json")
	rec := httptest.NewRecorder()

	s := &Server{}
	s.handleMCPEndpoint(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated initialize, got %d", rec.Code)
	}

	authHeader := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(authHeader, `Bearer resource_metadata="https://atmospheric-intellectual-sleeping-conducting.trycloudflare.com/.well-known/oauth-protected-resource"`) {
		t.Fatalf("expected WWW-Authenticate to point to protected resource metadata, got %q", authHeader)
	}

	body := decodeResponseBody(t, rec)
	if body["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc 2.0 response, got %v", body)
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
	if !strings.Contains(authHeader, `Bearer resource_metadata="https://atmospheric-intellectual-sleeping-conducting.trycloudflare.com/.well-known/oauth-protected-resource"`) {
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
	if body["resource"] != "https://atmospheric-intellectual-sleeping-conducting.trycloudflare.com/mcp" {
		t.Fatalf("expected resource metadata for captured host, got %v", body["resource"])
	}
}

func TestCapturedOAuthRequests_GeminiCLIHeadProbeGetsMethodNotAllowed(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/gemini-cli/mcp_head_probe.json")
	rec := httptest.NewRecorder()

	s := &Server{}
	s.handleMCPEndpoint(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for HEAD probe, got %d", rec.Code)
	}
}

func TestCapturedOAuthRequests_ProtectedResourceMetadataForClaudeWeb(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/claude-web/protected_resource_metadata.json")
	rec := httptest.NewRecorder()

	s := &Server{}
	s.handleProtectedResourceMetadata(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := decodeResponseBody(t, rec)
	if body["resource"] != "https://atmospheric-intellectual-sleeping-conducting.trycloudflare.com/mcp" {
		t.Fatalf("expected resource metadata for captured host, got %v", body["resource"])
	}
}

func TestCapturedOAuthRequests_ProtectedResourceMetadataForGeminiCLI(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/gemini-cli/protected_resource_metadata.json")
	rec := httptest.NewRecorder()

	s := &Server{}
	s.handleProtectedResourceMetadata(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := decodeResponseBody(t, rec)
	if body["resource"] != "https://atmospheric-intellectual-sleeping-conducting.trycloudflare.com/mcp" {
		t.Fatalf("expected resource metadata for captured host, got %v", body["resource"])
	}
}

func TestCapturedOAuthRequests_ChatGPTProbeGetsOAuthChallenge(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/chatgpt/mcp_probe.json")
	rec := httptest.NewRecorder()

	s := &Server{}
	s.handleMCPEndpoint(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated probe, got %d", rec.Code)
	}

	authHeader := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(authHeader, `Bearer resource_metadata="https://atmospheric-intellectual-sleeping-conducting.trycloudflare.com/.well-known/oauth-protected-resource"`) {
		t.Fatalf("expected WWW-Authenticate to point to protected resource metadata, got %q", authHeader)
	}
}

func TestCapturedOAuthRequests_ProtectedResourceMetadataForChatGPT(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/chatgpt/protected_resource_metadata.json")
	rec := httptest.NewRecorder()

	s := &Server{}
	s.handleProtectedResourceMetadata(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := decodeResponseBody(t, rec)
	if body["resource"] != "https://atmospheric-intellectual-sleeping-conducting.trycloudflare.com/mcp" {
		t.Fatalf("expected resource metadata for captured host, got %v", body["resource"])
	}
}

func TestCapturedOAuthRequests_ClaudeCodeAuthorizeApprovalFormParses(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/claude-code/oauth_authorize.json")

	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}

	if req.FormValue("client_id") != "https://claude.ai/oauth/claude-code-client-metadata" {
		t.Fatalf("unexpected client_id: %q", req.FormValue("client_id"))
	}
	if req.FormValue("redirect_uri") != "http://localhost:58969/callback" {
		t.Fatalf("unexpected redirect_uri: %q", req.FormValue("redirect_uri"))
	}
	if req.FormValue("state") != "_uzP1LIffESyobU7_jAwJZ7r1FuIroWGKOxKGi7M1a8" {
		t.Fatalf("unexpected state: %q", req.FormValue("state"))
	}
	if req.FormValue("action") != "approve" {
		t.Fatalf("unexpected action: %q", req.FormValue("action"))
	}
	if req.FormValue("_token") != "CAPTURED_CLAUDE_CODE_LOGIN_JWT" {
		t.Fatalf("unexpected _token placeholder: %q", req.FormValue("_token"))
	}
	if !strings.Contains(req.FormValue("scope"), "read:vault.auth") {
		t.Fatalf("expected read:vault.auth in scope, got %q", req.FormValue("scope"))
	}
}

func TestCapturedOAuthRequests_ClaudeWebAuthorizeApprovalFormParses(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/claude-web/oauth_authorize.json")

	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}

	if req.FormValue("client_id") != "https://claude.ai/oauth/mcp-oauth-client-metadata" {
		t.Fatalf("unexpected client_id: %q", req.FormValue("client_id"))
	}
	if req.FormValue("redirect_uri") != "https://claude.ai/api/mcp/auth_callback" {
		t.Fatalf("unexpected redirect_uri: %q", req.FormValue("redirect_uri"))
	}
	if req.FormValue("state") != "CAPTURED_CLAUDE_WEB_STATE" {
		t.Fatalf("unexpected state: %q", req.FormValue("state"))
	}
	if req.FormValue("action") != "approve" {
		t.Fatalf("unexpected action: %q", req.FormValue("action"))
	}
	if req.FormValue("_token") != "CAPTURED_CLAUDE_WEB_LOGIN_JWT" {
		t.Fatalf("unexpected _token placeholder: %q", req.FormValue("_token"))
	}
	if !strings.Contains(req.FormValue("scope"), "offline_access") {
		t.Fatalf("expected offline_access in scope, got %q", req.FormValue("scope"))
	}
}

func TestCapturedOAuthRequests_GeminiCLIAuthorizeApprovalFormParses(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/gemini-cli/oauth_authorize.json")

	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}

	if req.FormValue("client_id") != "ahc_captured_gemini_client" {
		t.Fatalf("unexpected client_id: %q", req.FormValue("client_id"))
	}
	if req.FormValue("redirect_uri") != "http://localhost:65290/oauth/callback" {
		t.Fatalf("unexpected redirect_uri: %q", req.FormValue("redirect_uri"))
	}
	if req.FormValue("state") != "CAPTURED_GEMINI_STATE" {
		t.Fatalf("unexpected state: %q", req.FormValue("state"))
	}
	if req.FormValue("action") != "approve" {
		t.Fatalf("unexpected action: %q", req.FormValue("action"))
	}
	if req.FormValue("_token") != "CAPTURED_GEMINI_LOGIN_JWT" {
		t.Fatalf("unexpected _token placeholder: %q", req.FormValue("_token"))
	}
	if !strings.Contains(req.FormValue("scope"), "offline_access") {
		t.Fatalf("expected offline_access in scope, got %q", req.FormValue("scope"))
	}
}

func TestCapturedOAuthRequests_ClaudeCodeAuthenticatedInitializeRequestParses(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/claude-code/mcp_initialize_authenticated.json")

	if got := req.Header.Get("Authorization"); got != "Bearer CAPTURED_CLAUDE_CODE_ACCESS_TOKEN" {
		t.Fatalf("unexpected Authorization header: %q", got)
	}

	var payload map[string]any
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if payload["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc 2.0, got %v", payload["jsonrpc"])
	}
	if payload["method"] != "initialize" {
		t.Fatalf("expected initialize method, got %v", payload["method"])
	}

	params, ok := payload["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected params object, got %T", payload["params"])
	}
	if params["protocolVersion"] != "2025-11-25" {
		t.Fatalf("unexpected protocolVersion: %v", params["protocolVersion"])
	}

	clientInfo, ok := params["clientInfo"].(map[string]any)
	if !ok {
		t.Fatalf("expected clientInfo object, got %T", params["clientInfo"])
	}
	if clientInfo["name"] != "claude-code" {
		t.Fatalf("unexpected clientInfo.name: %v", clientInfo["name"])
	}
	if clientInfo["version"] != "2.1.84" {
		t.Fatalf("unexpected clientInfo.version: %v", clientInfo["version"])
	}
}

func TestCapturedOAuthRequests_ClaudeWebAuthenticatedInitializeRequestParses(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/claude-web/mcp_initialize_authenticated.json")

	if got := req.Header.Get("Authorization"); got != "Bearer CAPTURED_CLAUDE_WEB_ACCESS_TOKEN" {
		t.Fatalf("unexpected Authorization header: %q", got)
	}

	var payload map[string]any
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if payload["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc 2.0, got %v", payload["jsonrpc"])
	}
	if payload["method"] != "initialize" {
		t.Fatalf("expected initialize method, got %v", payload["method"])
	}

	params, ok := payload["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected params object, got %T", payload["params"])
	}
	if params["protocolVersion"] != "2025-11-25" {
		t.Fatalf("unexpected protocolVersion: %v", params["protocolVersion"])
	}

	clientInfo, ok := params["clientInfo"].(map[string]any)
	if !ok {
		t.Fatalf("expected clientInfo object, got %T", params["clientInfo"])
	}
	if clientInfo["name"] != "Anthropic/Toolbox" {
		t.Fatalf("unexpected clientInfo.name: %v", clientInfo["name"])
	}
	if clientInfo["version"] != "1.0.0" {
		t.Fatalf("unexpected clientInfo.version: %v", clientInfo["version"])
	}
}

func TestCapturedOAuthRequests_GeminiCLIAuthenticatedInitializeRequestParses(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/gemini-cli/mcp_initialize_authenticated.json")

	if got := req.Header.Get("Authorization"); got != "Bearer CAPTURED_GEMINI_ACCESS_TOKEN" {
		t.Fatalf("unexpected Authorization header: %q", got)
	}

	var payload map[string]any
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if payload["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc 2.0, got %v", payload["jsonrpc"])
	}
	if payload["method"] != "initialize" {
		t.Fatalf("expected initialize method, got %v", payload["method"])
	}

	params, ok := payload["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected params object, got %T", payload["params"])
	}
	if params["protocolVersion"] != "2025-11-25" {
		t.Fatalf("unexpected protocolVersion: %v", params["protocolVersion"])
	}

	clientInfo, ok := params["clientInfo"].(map[string]any)
	if !ok {
		t.Fatalf("expected clientInfo object, got %T", params["clientInfo"])
	}
	if clientInfo["name"] != "gemini-cli-mcp-client" {
		t.Fatalf("unexpected clientInfo.name: %v", clientInfo["name"])
	}
	if clientInfo["version"] != "0.36.0" {
		t.Fatalf("unexpected clientInfo.version: %v", clientInfo["version"])
	}
}

func TestCapturedOAuthRequests_ChatGPTAuthorizeApprovalFormParses(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/chatgpt/oauth_authorize.json")

	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}

	if req.FormValue("client_id") != "ahc_captured_chatgpt_client" {
		t.Fatalf("unexpected client_id: %q", req.FormValue("client_id"))
	}
	if req.FormValue("redirect_uri") != "https://chatgpt.com/connector/oauth/CAPTURED_CHATGPT_CONNECTOR" {
		t.Fatalf("unexpected redirect_uri: %q", req.FormValue("redirect_uri"))
	}
	if req.FormValue("state") != "oauth_s_captured_chatgpt_state" {
		t.Fatalf("unexpected state: %q", req.FormValue("state"))
	}
	if req.FormValue("action") != "approve" {
		t.Fatalf("unexpected action: %q", req.FormValue("action"))
	}
	if req.FormValue("_token") != "CAPTURED_CHATGPT_LOGIN_JWT" {
		t.Fatalf("unexpected _token placeholder: %q", req.FormValue("_token"))
	}
	if !strings.Contains(req.FormValue("scope"), "read:vault.auth") {
		t.Fatalf("expected read:vault.auth in scope, got %q", req.FormValue("scope"))
	}
}

func TestCapturedOAuthRequests_ChatGPTAuthenticatedInitializeRequestParses(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/chatgpt/mcp_initialize_authenticated.json")

	if got := req.Header.Get("Authorization"); got != "Bearer CAPTURED_CHATGPT_ACCESS_TOKEN" {
		t.Fatalf("unexpected Authorization header: %q", got)
	}

	var payload map[string]any
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if payload["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc 2.0, got %v", payload["jsonrpc"])
	}
	if payload["method"] != "initialize" {
		t.Fatalf("expected initialize method, got %v", payload["method"])
	}

	params, ok := payload["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected params object, got %T", payload["params"])
	}
	if params["protocolVersion"] != "2025-11-25" {
		t.Fatalf("unexpected protocolVersion: %v", params["protocolVersion"])
	}

	clientInfo, ok := params["clientInfo"].(map[string]any)
	if !ok {
		t.Fatalf("expected clientInfo object, got %T", params["clientInfo"])
	}
	if clientInfo["name"] != "openai-mcp (ChatGPT)" {
		t.Fatalf("unexpected clientInfo.name: %v", clientInfo["name"])
	}
	if clientInfo["version"] != "1.0.0" {
		t.Fatalf("unexpected clientInfo.version: %v", clientInfo["version"])
	}

	capabilities, ok := params["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("expected capabilities object, got %T", params["capabilities"])
	}
	if _, ok := capabilities["experimental"].(map[string]any); !ok {
		t.Fatalf("expected experimental capability block, got %T", capabilities["experimental"])
	}
	if _, ok := capabilities["extensions"].(map[string]any); !ok {
		t.Fatalf("expected extensions capability block, got %T", capabilities["extensions"])
	}
}

func TestCapturedOAuthRequests_CodexAuthorizeApprovalFormParses(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/codex/oauth_authorize.json")

	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}

	if req.FormValue("client_id") != "ahc_618856aa85b01dee17f2fc3036af03dc" {
		t.Fatalf("unexpected client_id: %q", req.FormValue("client_id"))
	}
	if req.FormValue("redirect_uri") != "http://127.0.0.1:57463/callback" {
		t.Fatalf("unexpected redirect_uri: %q", req.FormValue("redirect_uri"))
	}
	if req.FormValue("state") != "27vmD6FjR4bF4CdZpSEI3g" {
		t.Fatalf("unexpected state: %q", req.FormValue("state"))
	}
	if req.FormValue("action") != "approve" {
		t.Fatalf("unexpected action: %q", req.FormValue("action"))
	}
	if req.FormValue("_token") != "CAPTURED_WEB_LOGIN_JWT" {
		t.Fatalf("unexpected _token placeholder: %q", req.FormValue("_token"))
	}
	if !strings.Contains(req.FormValue("scope"), "offline_access") {
		t.Fatalf("expected offline_access in scope, got %q", req.FormValue("scope"))
	}
}

func TestCapturedOAuthRequests_CodexInitializeRequestParses(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/codex/mcp_initialize.json")

	if got := req.Header.Get("Authorization"); got != "Bearer CAPTURED_OAUTH_ACCESS_TOKEN" {
		t.Fatalf("unexpected Authorization header: %q", got)
	}

	var payload map[string]any
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if payload["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc 2.0, got %v", payload["jsonrpc"])
	}
	if payload["method"] != "initialize" {
		t.Fatalf("expected initialize method, got %v", payload["method"])
	}

	params, ok := payload["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected params object, got %T", payload["params"])
	}
	if params["protocolVersion"] != "2025-06-18" {
		t.Fatalf("unexpected protocolVersion: %v", params["protocolVersion"])
	}

	clientInfo, ok := params["clientInfo"].(map[string]any)
	if !ok {
		t.Fatalf("expected clientInfo object, got %T", params["clientInfo"])
	}
	if clientInfo["name"] != "codex-mcp-client" {
		t.Fatalf("unexpected clientInfo.name: %v", clientInfo["name"])
	}
	if clientInfo["title"] != "Codex" {
		t.Fatalf("unexpected clientInfo.title: %v", clientInfo["title"])
	}
	if clientInfo["version"] != "0.118.0" {
		t.Fatalf("unexpected clientInfo.version: %v", clientInfo["version"])
	}
}

func TestCapturedOAuthRequests_AuthorizationServerMetadataSupportsPlatformClients(t *testing.T) {
	fixtures := []string{
		"testdata/oauth/claude-web/authorization_server_metadata.json",
		"testdata/oauth/claude-code/authorization_server_metadata.json",
		"testdata/oauth/codex/authorization_server_metadata.json",
		"testdata/oauth/chatgpt/authorization_server_metadata.json",
		"testdata/oauth/chatgpt/openid_configuration.json",
		"testdata/oauth/gemini-cli/authorization_server_metadata.json",
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
