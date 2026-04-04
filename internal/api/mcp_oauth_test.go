package api

import (
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
