package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/agi-bar/neudrive/internal/config"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
)

type stubExternalAuthRepo struct {
	created []models.AuthTransaction
}

func (r *stubExternalAuthRepo) CreateAuthTransaction(_ context.Context, txn models.AuthTransaction) error {
	r.created = append(r.created, txn)
	return nil
}

func (r *stubExternalAuthRepo) ConsumeAuthTransaction(context.Context, string, string, time.Time) (*models.AuthTransaction, error) {
	panic("unexpected ConsumeAuthTransaction call")
}

func (r *stubExternalAuthRepo) UpsertExternalIdentity(context.Context, models.ExternalIdentityUpsert, time.Time) (*models.User, *models.AuthBinding, error) {
	panic("unexpected UpsertExternalIdentity call")
}

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

func TestAuthProviderStartPocketLoginWrapsAuthorizeURL(t *testing.T) {
	var issuer string
	discovery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 issuer,
			"authorization_endpoint": issuer + "/oidc/authorize",
			"token_endpoint":         issuer + "/oidc/token",
			"jwks_uri":               issuer + "/jwks.json",
			"userinfo_endpoint":      issuer + "/oidc/userinfo",
		})
	}))
	defer discovery.Close()
	issuer = discovery.URL

	repo := &stubExternalAuthRepo{}
	cfg := &config.Config{
		JWTSecret:          testJWTSecret,
		VaultMasterKey:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CORSOrigins:        []string{"http://localhost:3000"},
		RateLimit:          100,
		MaxBodySize:        10 * 1024 * 1024,
		PublicBaseURL:      "https://neudrive.example.com",
		PocketProviderID:   "pocket",
		PocketIssuer:       issuer,
		PocketDiscoveryURL: issuer + "/.well-known/openid-configuration",
		PocketClientID:     "pocket-client",
		PocketClientSecret: "pocket-secret",
		PocketScopes:       []string{"openid", "profile", "email"},
	}
	server := NewServerWithDeps(ServerDeps{
		Config:              cfg,
		JWTSecret:           cfg.JWTSecret,
		ExternalAuthService: services.NewExternalAuthServiceWithRepo(repo, &services.AuthService{}, cfg),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/providers/pocket/start", strings.NewReader(`{"redirect_url":"/","action":"login"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			AuthorizationURL string `json:"authorization_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok response: %s", rec.Body.String())
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected one auth transaction, got %d", len(repo.created))
	}

	outer, err := url.Parse(envelope.Data.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse outer authorization url: %v", err)
	}
	if got := outer.String(); !strings.HasPrefix(got, issuer+"/login?redirect=") {
		t.Fatalf("expected wrapped pocket login url, got %q", got)
	}
	redirectParam := outer.Query().Get("redirect")
	if redirectParam == "" {
		t.Fatalf("expected redirect query param in %q", outer.String())
	}

	inner, err := url.Parse(redirectParam)
	if err != nil {
		t.Fatalf("parse inner authorization url: %v", err)
	}
	if got := inner.String(); !strings.HasPrefix(got, issuer+"/oidc/authorize?") {
		t.Fatalf("expected inner authorize url, got %q", got)
	}
	if got := inner.Query().Get("client_id"); got != "pocket-client" {
		t.Fatalf("unexpected client_id: %q", got)
	}
	if got := inner.Query().Get("redirect_uri"); got != "https://neudrive.example.com/api/auth/providers/pocket/callback" {
		t.Fatalf("unexpected callback url: %q", got)
	}
	if got := inner.Query().Get("response_type"); got != "code" {
		t.Fatalf("unexpected response_type: %q", got)
	}
	if got := inner.Query().Get("state"); got == "" {
		t.Fatalf("expected state in authorize url: %q", inner.String())
	}
	if got := inner.Query().Get("code_challenge"); got == "" {
		t.Fatalf("expected code_challenge in authorize url: %q", inner.String())
	}
}

func TestAuthProviderStartPocketSignupReturnsSignupPage(t *testing.T) {
	repo := &stubExternalAuthRepo{}
	cfg := &config.Config{
		JWTSecret:          testJWTSecret,
		VaultMasterKey:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CORSOrigins:        []string{"http://localhost:3000"},
		RateLimit:          100,
		MaxBodySize:        10 * 1024 * 1024,
		PocketProviderID:   "pocket",
		PocketIssuer:       "https://pocket.example.com",
		PocketClientID:     "pocket-client",
		PocketClientSecret: "pocket-secret",
		PocketScopes:       []string{"openid", "profile", "email"},
	}
	server := NewServerWithDeps(ServerDeps{
		Config:              cfg,
		JWTSecret:           cfg.JWTSecret,
		ExternalAuthService: services.NewExternalAuthServiceWithRepo(repo, &services.AuthService{}, cfg),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/providers/pocket/start", strings.NewReader(`{"redirect_url":"/","action":"signup"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			AuthorizationURL string `json:"authorization_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := envelope.Data.AuthorizationURL; got != "https://pocket.example.com/signup" {
		t.Fatalf("unexpected authorization_url: %q", got)
	}
	if len(repo.created) != 0 {
		t.Fatalf("expected no auth transactions for signup, got %d", len(repo.created))
	}
}

func TestAuthProviderStartGitHubSignupReturnsBadRequest(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:          testJWTSecret,
		VaultMasterKey:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CORSOrigins:        []string{"http://localhost:3000"},
		RateLimit:          100,
		MaxBodySize:        10 * 1024 * 1024,
		GithubClientID:     "gh-client",
		GithubClientSecret: "gh-secret",
	}
	server := NewServerWithDeps(ServerDeps{
		Config:              cfg,
		JWTSecret:           cfg.JWTSecret,
		ExternalAuthService: services.NewExternalAuthServiceWithRepo(&stubExternalAuthRepo{}, &services.AuthService{}, cfg),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/providers/github/start", strings.NewReader(`{"redirect_url":"/","action":"signup"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	var errResp APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if !strings.Contains(errResp.Message, "does not support signup") {
		t.Fatalf("unexpected error message: %q", errResp.Message)
	}
}

func TestAuthProviderStartRejectsInvalidAction(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:          testJWTSecret,
		VaultMasterKey:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CORSOrigins:        []string{"http://localhost:3000"},
		RateLimit:          100,
		MaxBodySize:        10 * 1024 * 1024,
		PocketProviderID:   "pocket",
		PocketIssuer:       "https://pocket.example.com",
		PocketClientID:     "pocket-client",
		PocketClientSecret: "pocket-secret",
		PocketScopes:       []string{"openid", "profile", "email"},
	}
	server := NewServerWithDeps(ServerDeps{
		Config:              cfg,
		JWTSecret:           cfg.JWTSecret,
		ExternalAuthService: services.NewExternalAuthServiceWithRepo(&stubExternalAuthRepo{}, &services.AuthService{}, cfg),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/providers/pocket/start", strings.NewReader(`{"redirect_url":"/","action":"bogus"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	var errResp APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if !strings.Contains(errResp.Message, "unsupported auth action") {
		t.Fatalf("unexpected error message: %q", errResp.Message)
	}
}
