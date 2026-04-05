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
	if parsed.Code != "test-code" {
		t.Fatalf("expected code from form body, got %q", parsed.Code)
	}
	if parsed.CodeVerifier != "vW4HWGfgMh75-MFpAnrTH5cP_5p2aD4QTA3F4zGhH2k" {
		t.Fatalf("expected code_verifier from captured form body, got %q", parsed.CodeVerifier)
	}
	if parsed.RedirectURI != "http://127.0.0.1:51431/callback" {
		t.Fatalf("expected redirect_uri from form body, got %q", parsed.RedirectURI)
	}
	if parsed.ClientID != "test-client" {
		t.Fatalf("expected client_id from basic auth, got %q", parsed.ClientID)
	}
	if parsed.ClientSecret != "test-secret" {
		t.Fatalf("expected client_secret from basic auth, got %q", parsed.ClientSecret)
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
