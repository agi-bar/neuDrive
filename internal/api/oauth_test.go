package api

import (
	"reflect"
	"testing"

	"github.com/agi-bar/agenthub/internal/models"
)

func TestEffectiveOAuthScopes_UsesRequestedScopesWhenPresent(t *testing.T) {
	app := &models.OAuthApp{Scopes: []string{"read:profile", "read:memory"}}

	scopes, scope := effectiveOAuthScopes(app, "read:tree search")

	wantScopes := []string{"read:tree", "search"}
	if !reflect.DeepEqual(scopes, wantScopes) {
		t.Fatalf("scopes mismatch: got %v want %v", scopes, wantScopes)
	}
	if scope != "read:tree search" {
		t.Fatalf("scope mismatch: got %q want %q", scope, "read:tree search")
	}
}

func TestEffectiveOAuthScopes_FallsBackToAppScopes(t *testing.T) {
	app := &models.OAuthApp{Scopes: []string{"read:profile", "read:memory", "offline_access"}}

	scopes, scope := effectiveOAuthScopes(app, "")

	wantScopes := []string{"read:profile", "read:memory", "offline_access"}
	if !reflect.DeepEqual(scopes, wantScopes) {
		t.Fatalf("scopes mismatch: got %v want %v", scopes, wantScopes)
	}
	if scope != "read:profile read:memory offline_access" {
		t.Fatalf("scope mismatch: got %q want %q", scope, "read:profile read:memory offline_access")
	}
}

func TestEffectiveOAuthScopes_EmptyWhenNothingRequestedOrRegistered(t *testing.T) {
	scopes, scope := effectiveOAuthScopes(nil, "")

	if len(scopes) != 0 {
		t.Fatalf("expected empty scopes, got %v", scopes)
	}
	if scope != "" {
		t.Fatalf("expected empty scope string, got %q", scope)
	}
}
