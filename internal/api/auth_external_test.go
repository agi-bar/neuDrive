package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agi-bar/neudrive/internal/config"
	"github.com/agi-bar/neudrive/internal/services"
)

func TestAuthProvidersEndpointListsEnabledProviders(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:          testJWTSecret,
		VaultMasterKey:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CORSOrigins:        []string{"http://localhost:3000"},
		RateLimit:          100,
		MaxBodySize:        10 * 1024 * 1024,
		GithubClientID:     "gh-client",
		GithubClientSecret: "gh-secret",
		PocketProviderID:   "pocket",
		PocketIssuer:       "https://pocket.example.com",
		PocketClientID:     "pocket-client",
		PocketClientSecret: "pocket-secret",
		PocketScopes:       []string{"openid", "profile", "email"},
	}
	server := NewServerWithDeps(ServerDeps{
		Config:              cfg,
		JWTSecret:           cfg.JWTSecret,
		ExternalAuthService: services.NewExternalAuthService(nil, &services.AuthService{}, cfg),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/providers", nil)
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data []struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok response: %s", rec.Body.String())
	}
	if len(envelope.Data) != 2 {
		t.Fatalf("expected two providers, got %d: %s", len(envelope.Data), rec.Body.String())
	}
	for _, provider := range envelope.Data {
		if !provider.Enabled {
			t.Fatalf("expected provider %q to be enabled", provider.ID)
		}
	}
}

func TestNormalizeAuthRedirectURLRejectsForeignOrigins(t *testing.T) {
	server := &Server{Config: &config.Config{PublicBaseURL: "https://neudrive.example.com"}}
	req := httptest.NewRequest(http.MethodGet, "https://neudrive.example.com/login", nil)

	if got := server.normalizeAuthRedirectURL(req, "/oauth/authorize?client_id=abc"); got != "/oauth/authorize?client_id=abc" {
		t.Fatalf("expected relative redirect to be preserved, got %q", got)
	}
	if got := server.normalizeAuthRedirectURL(req, "https://evil.example.com/phish"); got != "/" {
		t.Fatalf("expected foreign redirect to be rejected, got %q", got)
	}
	if got := server.normalizeAuthRedirectURL(req, "https://neudrive.example.com/oauth/authorize?client_id=abc"); got != "https://neudrive.example.com/oauth/authorize?client_id=abc" {
		t.Fatalf("expected same-origin absolute redirect to be preserved, got %q", got)
	}
}
