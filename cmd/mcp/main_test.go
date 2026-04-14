package main

import (
	"testing"

	"github.com/agi-bar/neudrive/internal/app/mcpapp"
)

func TestResolveTokenPrefersExplicitValue(t *testing.T) {
	t.Setenv(mcpapp.DefaultTokenEnvVar, "ndt_from_env")

	token, err := mcpapp.ResolveToken("ndt_explicit", mcpapp.DefaultTokenEnvVar)
	if err != nil {
		t.Fatalf("resolveToken returned error: %v", err)
	}
	if token != "ndt_explicit" {
		t.Fatalf("expected explicit token, got %q", token)
	}
}

func TestResolveTokenFallsBackToEnvironment(t *testing.T) {
	t.Setenv(mcpapp.DefaultTokenEnvVar, "ndt_from_env")

	token, err := mcpapp.ResolveToken("", mcpapp.DefaultTokenEnvVar)
	if err != nil {
		t.Fatalf("resolveToken returned error: %v", err)
	}
	if token != "ndt_from_env" {
		t.Fatalf("expected token from env, got %q", token)
	}
}

func TestResolveTokenSupportsCustomEnvironmentVariable(t *testing.T) {
	t.Setenv("CUSTOM_NEUDRIVE_TOKEN", "ndt_custom")

	token, err := mcpapp.ResolveToken("", "CUSTOM_NEUDRIVE_TOKEN")
	if err != nil {
		t.Fatalf("resolveToken returned error: %v", err)
	}
	if token != "ndt_custom" {
		t.Fatalf("expected token from custom env, got %q", token)
	}
}

func TestResolveTokenErrorsWhenMissing(t *testing.T) {
	_, err := mcpapp.ResolveToken("", "MISSING_TOKEN")
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}
