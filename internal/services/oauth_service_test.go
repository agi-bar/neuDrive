package services

import (
	"reflect"
	"testing"
)

func TestNormalizeOAuthStringSlice(t *testing.T) {
	got := normalizeOAuthStringSlice([]string{" read:profile ", "", "search", "search", "  "})
	want := []string{"read:profile", "search", "search"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeOAuthStringSlice() = %v, want %v", got, want)
	}
}

func TestNormalizeOAuthGrantScopes_FallsBackToAppScopes(t *testing.T) {
	got := normalizeOAuthGrantScopes(nil, []string{"", "read:memory", " search "})
	want := []string{"read:memory", "search"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeOAuthGrantScopes() = %v, want %v", got, want)
	}
}

func TestNormalizeOAuthGrantScopes_PrefersGrantScopes(t *testing.T) {
	got := normalizeOAuthGrantScopes([]string{" read:tree "}, []string{"read:memory"})
	want := []string{"read:tree"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeOAuthGrantScopes() = %v, want %v", got, want)
	}
}
