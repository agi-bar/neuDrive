package api

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseOAuthTokenRequestSupportsCapturedCodexRequest(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/codex/oauth_token_authorization_code.json")

	parsed, err := parseOAuthTokenRequest(req)
	if err != nil {
		t.Fatalf("parseOAuthTokenRequest returned error: %v", err)
	}

	if parsed.GrantType != "authorization_code" {
		t.Fatalf("expected grant_type authorization_code, got %q", parsed.GrantType)
	}
	if parsed.Code != "3a3d2158fcd328523a0f876be57e7d0a430cfca15a3f358b5e929096c777d7a7" {
		t.Fatalf("expected code from form body, got %q", parsed.Code)
	}
	if parsed.CodeVerifier != "69HX5XXfF9DoS7CwkS5_H-MFh-Jv9BNqOUMBQE7MwPs" {
		t.Fatalf("expected code_verifier from captured form body, got %q", parsed.CodeVerifier)
	}
	if parsed.RedirectURI != "http://127.0.0.1:57463/callback" {
		t.Fatalf("expected redirect_uri from form body, got %q", parsed.RedirectURI)
	}
	if parsed.ClientID != "ahc_captured_codex_client" {
		t.Fatalf("expected client_id from basic auth, got %q", parsed.ClientID)
	}
	if parsed.ClientSecret != "ahs_captured_codex_secret" {
		t.Fatalf("expected client_secret from basic auth, got %q", parsed.ClientSecret)
	}
}

func TestParseOAuthTokenRequestSupportsCapturedClaudeCodeRequest(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/claude-code/oauth_token_authorization_code.json")

	parsed, err := parseOAuthTokenRequest(req)
	if err != nil {
		t.Fatalf("parseOAuthTokenRequest returned error: %v", err)
	}

	if parsed.GrantType != "authorization_code" {
		t.Fatalf("expected grant_type authorization_code, got %q", parsed.GrantType)
	}
	if parsed.Code != "51c18b8948dba2b77a5f74cd1758bd6812e3d955144131352726b1a2f47cdbe7" {
		t.Fatalf("expected code from form body, got %q", parsed.Code)
	}
	if parsed.CodeVerifier != "tWHtUB53iJqliblWB3T..GEhAC5DSJ8XJs5yAdsTfek" {
		t.Fatalf("expected code_verifier from captured form body, got %q", parsed.CodeVerifier)
	}
	if parsed.RedirectURI != "http://localhost:58969/callback" {
		t.Fatalf("expected redirect_uri from form body, got %q", parsed.RedirectURI)
	}
	if parsed.ClientID != "https://claude.ai/oauth/claude-code-client-metadata" {
		t.Fatalf("expected client_id from form body, got %q", parsed.ClientID)
	}
	if parsed.ClientSecret != "" {
		t.Fatalf("expected empty client_secret for public client metadata flow, got %q", parsed.ClientSecret)
	}
}

func TestParseOAuthTokenRequestSupportsCapturedClaudeWebRequest(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/claude-web/oauth_token_authorization_code.json")

	parsed, err := parseOAuthTokenRequest(req)
	if err != nil {
		t.Fatalf("parseOAuthTokenRequest returned error: %v", err)
	}

	if parsed.GrantType != "authorization_code" {
		t.Fatalf("expected grant_type authorization_code, got %q", parsed.GrantType)
	}
	if parsed.Code != "captured_claude_web_auth_code" {
		t.Fatalf("expected code from form body, got %q", parsed.Code)
	}
	if parsed.CodeVerifier != "CAPTURED_CLAUDE_WEB_CODE_VERIFIER" {
		t.Fatalf("expected code_verifier from form body, got %q", parsed.CodeVerifier)
	}
	if parsed.RedirectURI != "https://claude.ai/api/mcp/auth_callback" {
		t.Fatalf("expected redirect_uri from form body, got %q", parsed.RedirectURI)
	}
	if parsed.ClientID != "https://claude.ai/oauth/mcp-oauth-client-metadata" {
		t.Fatalf("expected client_id from form body, got %q", parsed.ClientID)
	}
	if parsed.ClientSecret != "" {
		t.Fatalf("expected empty client_secret for claude web connector flow, got %q", parsed.ClientSecret)
	}
}

func TestParseOAuthTokenRequestSupportsCapturedChatGPTRequest(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/chatgpt/oauth_token_authorization_code.json")

	parsed, err := parseOAuthTokenRequest(req)
	if err != nil {
		t.Fatalf("parseOAuthTokenRequest returned error: %v", err)
	}

	if parsed.GrantType != "authorization_code" {
		t.Fatalf("expected grant_type authorization_code, got %q", parsed.GrantType)
	}
	if parsed.Code != "captured_chatgpt_auth_code" {
		t.Fatalf("expected code from form body, got %q", parsed.Code)
	}
	if parsed.CodeVerifier != "CAPTURED_CHATGPT_CODE_VERIFIER" {
		t.Fatalf("expected code_verifier from form body, got %q", parsed.CodeVerifier)
	}
	if parsed.RedirectURI != "https://chatgpt.com/connector/oauth/CAPTURED_CHATGPT_CONNECTOR" {
		t.Fatalf("expected redirect_uri from form body, got %q", parsed.RedirectURI)
	}
	if parsed.ClientID != "ahc_captured_chatgpt_client" {
		t.Fatalf("expected client_id from form body, got %q", parsed.ClientID)
	}
	if parsed.ClientSecret != "ahs_captured_chatgpt_secret" {
		t.Fatalf("expected client_secret from form body, got %q", parsed.ClientSecret)
	}
}

func TestParseOAuthTokenRequestSupportsCapturedGeminiCLIRequest(t *testing.T) {
	req := newRequestFromFixture(t, "testdata/oauth/gemini-cli/oauth_token_authorization_code.json")

	parsed, err := parseOAuthTokenRequest(req)
	if err != nil {
		t.Fatalf("parseOAuthTokenRequest returned error: %v", err)
	}

	if parsed.GrantType != "authorization_code" {
		t.Fatalf("expected grant_type authorization_code, got %q", parsed.GrantType)
	}
	if parsed.Code != "captured_gemini_auth_code" {
		t.Fatalf("expected code from form body, got %q", parsed.Code)
	}
	if parsed.CodeVerifier != "CAPTURED_GEMINI_CODE_VERIFIER" {
		t.Fatalf("expected code_verifier from form body, got %q", parsed.CodeVerifier)
	}
	if parsed.RedirectURI != "http://localhost:65290/oauth/callback" {
		t.Fatalf("expected redirect_uri from form body, got %q", parsed.RedirectURI)
	}
	if parsed.ClientID != "ahc_captured_gemini_client" {
		t.Fatalf("expected client_id from form body, got %q", parsed.ClientID)
	}
	if parsed.ClientSecret != "ahs_captured_gemini_secret" {
		t.Fatalf("expected client_secret from form body, got %q", parsed.ClientSecret)
	}
}

func TestParseOAuthTokenRequestRejectsConflictingBasicAuth(t *testing.T) {
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {"test-code"},
		"client_id":    {"body-client"},
		"redirect_uri": {"http://127.0.0.1:51431/callback"},
	}

	req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("header-client", "test-secret")

	_, err := parseOAuthTokenRequest(req)
	if err == nil {
		t.Fatal("expected conflicting basic auth parameters to fail")
	}
	if err != errOAuthConflictingClientAuth {
		t.Fatalf("expected errOAuthConflictingClientAuth, got %v", err)
	}
}
