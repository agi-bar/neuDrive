package main

import (
	"testing"

	"github.com/agi-bar/agenthub/internal/app/mcpapp"
)

func TestResolveTokenPrefersExplicitValue(t *testing.T) {
	t.Setenv(mcpapp.DefaultTokenEnvVar, "aht_from_env")

	token, err := mcpapp.ResolveToken("aht_explicit", mcpapp.DefaultTokenEnvVar)
	if err != nil {
		t.Fatalf("resolveToken returned error: %v", err)
	}
	if token != "aht_explicit" {
		t.Fatalf("expected explicit token, got %q", token)
	}
}

func TestResolveTokenFallsBackToEnvironment(t *testing.T) {
	t.Setenv(mcpapp.DefaultTokenEnvVar, "aht_from_env")

	token, err := mcpapp.ResolveToken("", mcpapp.DefaultTokenEnvVar)
	if err != nil {
		t.Fatalf("resolveToken returned error: %v", err)
	}
	if token != "aht_from_env" {
		t.Fatalf("expected token from env, got %q", token)
	}
}

func TestResolveTokenSupportsCustomEnvironmentVariable(t *testing.T) {
	t.Setenv("CUSTOM_AGENTHUB_TOKEN", "aht_custom")

	token, err := mcpapp.ResolveToken("", "CUSTOM_AGENTHUB_TOKEN")
	if err != nil {
		t.Fatalf("resolveToken returned error: %v", err)
	}
	if token != "aht_custom" {
		t.Fatalf("expected token from custom env, got %q", token)
	}
}

func TestResolveTokenErrorsWhenMissing(t *testing.T) {
	_, err := mcpapp.ResolveToken("", "MISSING_TOKEN")
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}
