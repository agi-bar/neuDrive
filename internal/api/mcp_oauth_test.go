package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/agi-bar/agenthub/internal/config"
)

func TestBaseURLUsesConfiguredPublicBaseURL(t *testing.T) {
	s := &Server{Config: &config.Config{PublicBaseURL: "https://agenthub.agi.bar"}}
	req := httptest.NewRequest("GET", "http://internal/.well-known/oauth-protected-resource", nil)
	req.Host = "internal.service"

	if got := s.baseURL(req); got != "https://agenthub.agi.bar" {
		t.Fatalf("expected configured public base URL, got %q", got)
	}
}

func TestBaseURLFallsBackToForwardedHTTPS(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest("GET", "http://internal/.well-known/oauth-protected-resource", nil)
	req.Host = "agenthub.agi.bar"
	req.Header.Set("X-Forwarded-Proto", "https")

	if got := s.baseURL(req); got != "https://agenthub.agi.bar" {
		t.Fatalf("expected forwarded https base URL, got %q", got)
	}
}

func TestAuthorizationServerMetadataIncludesClientSecretBasic(t *testing.T) {
	s := &Server{Config: &config.Config{PublicBaseURL: "https://agenthub.agi.bar"}}
	req := httptest.NewRequest("GET", "https://agenthub.agi.bar/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()

	s.handleAuthorizationServerMetadata(rec, req)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode metadata response: %v", err)
	}

	methods, ok := body["token_endpoint_auth_methods_supported"].([]any)
	if !ok {
		t.Fatalf("missing token_endpoint_auth_methods_supported: %v", body)
	}

	found := false
	for _, method := range methods {
		if method == "client_secret_basic" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected client_secret_basic in token_endpoint_auth_methods_supported, got %v", methods)
	}
}
