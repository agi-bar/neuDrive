package localgitsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	sqlitestorage "github.com/agi-bar/neudrive/internal/storage/sqlite"
	"github.com/agi-bar/neudrive/internal/vault"
	"github.com/google/uuid"
)

type fakeGitHubAppServerState struct {
	login          string
	permission     string
	repoAccessible bool
}

func TestUpdateMirrorSettingsGitHubAppUserValidatesRepoOnSave(t *testing.T) {
	svc, userID := newGitHubAppSettingsTestService(t, fakeGitHubAppServerState{
		login:          "octocat",
		permission:     "write",
		repoAccessible: true,
	})

	ctx := context.Background()
	if err := svc.writeStoredGitHubAppRefreshToken(ctx, userID, "refresh-token"); err != nil {
		t.Fatalf("writeStoredGitHubAppRefreshToken: %v", err)
	}

	settings, err := svc.UpdateMirrorSettings(ctx, userID, MirrorSettingsUpdate{
		AutoCommitEnabled: true,
		AutoPushEnabled:   false,
		AuthMode:          AuthModeGitHubAppUser,
		RemoteName:        DefaultRemoteName,
		RemoteURL:         "git@github.com:acme/demo.git",
		RemoteBranch:      DefaultRemoteBranch,
	})
	if err != nil {
		t.Fatalf("UpdateMirrorSettings: %v", err)
	}
	if got, want := settings.RemoteURL, "https://github.com/acme/demo.git"; got != want {
		t.Fatalf("remote_url = %q, want %q", got, want)
	}
	if got, want := settings.GitHubRepoPermission, "write"; got != want {
		t.Fatalf("github_repo_permission = %q, want %q", got, want)
	}
	if !settings.GitHubAppUserConnected {
		t.Fatalf("expected GitHub App connection metadata in settings: %+v", settings)
	}
}

func TestUpdateMirrorSettingsGitHubAppUserRejectsInaccessibleRepo(t *testing.T) {
	svc, userID := newGitHubAppSettingsTestService(t, fakeGitHubAppServerState{
		login:          "octocat",
		permission:     "write",
		repoAccessible: false,
	})

	ctx := context.Background()
	if err := svc.writeStoredGitHubAppRefreshToken(ctx, userID, "refresh-token"); err != nil {
		t.Fatalf("writeStoredGitHubAppRefreshToken: %v", err)
	}

	_, err := svc.UpdateMirrorSettings(ctx, userID, MirrorSettingsUpdate{
		AutoCommitEnabled: true,
		AutoPushEnabled:   false,
		AuthMode:          AuthModeGitHubAppUser,
		RemoteName:        DefaultRemoteName,
		RemoteURL:         "https://github.com/acme/demo.git",
		RemoteBranch:      DefaultRemoteBranch,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot access this repository") {
		t.Fatalf("expected inaccessible repo error, got %v", err)
	}
}

func TestUpdateMirrorSettingsGitHubAppUserRejectsReadOnlyAutoPush(t *testing.T) {
	svc, userID := newGitHubAppSettingsTestService(t, fakeGitHubAppServerState{
		login:          "octocat",
		permission:     "read",
		repoAccessible: true,
	})

	ctx := context.Background()
	if err := svc.writeStoredGitHubAppRefreshToken(ctx, userID, "refresh-token"); err != nil {
		t.Fatalf("writeStoredGitHubAppRefreshToken: %v", err)
	}

	_, err := svc.UpdateMirrorSettings(ctx, userID, MirrorSettingsUpdate{
		AutoCommitEnabled: true,
		AutoPushEnabled:   true,
		AuthMode:          AuthModeGitHubAppUser,
		RemoteName:        DefaultRemoteName,
		RemoteURL:         "https://github.com/acme/demo.git",
		RemoteBranch:      DefaultRemoteBranch,
	})
	if err == nil || !strings.Contains(err.Error(), "does not have push access") {
		t.Fatalf("expected push access error, got %v", err)
	}
}

func newGitHubAppSettingsTestService(t *testing.T, state fakeGitHubAppServerState) (*Service, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlitestorage.Open(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	v, err := vault.NewVault(strings.Repeat("0", 64))
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	server := newFakeGitHubAppServer(t, state)
	svc := New(
		store,
		v,
		WithGitHubAPIBaseURL(server.URL),
		WithGitHubBaseURL(server.URL),
		WithGitHubAppConfig("client-id", "client-secret", "neudrive"),
		WithHTTPClient(server.Client()),
	)
	if _, err := svc.RegisterMirrorAndSync(ctx, user.ID, filepath.Join(t.TempDir(), "mirror")); err != nil {
		t.Fatalf("RegisterMirrorAndSync: %v", err)
	}
	return svc, user.ID
}

func newFakeGitHubAppServer(t *testing.T, state fakeGitHubAppServerState) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login/oauth/access_token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":             "ghu_access",
				"refresh_token":            "ghu_refresh_next",
				"expires_in":               3600,
				"refresh_token_expires_in": 7200,
				"token_type":               "bearer",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			if strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"login": state.login})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/demo":
			if !state.repoAccessible {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"not found"}`))
				return
			}
			permissions := map[string]bool{
				"admin": state.permission == "admin",
				"push":  state.permission == "admin" || state.permission == "write",
				"pull":  state.permission == "admin" || state.permission == "write" || state.permission == "read",
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":           "demo",
				"full_name":      "acme/demo",
				"default_branch": "main",
				"clone_url":      "https://github.com/acme/demo.git",
				"permissions":    permissions,
				"owner": map[string]any{
					"login": "acme",
					"type":  "Organization",
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		}
	}))
	t.Cleanup(server.Close)
	return server
}
