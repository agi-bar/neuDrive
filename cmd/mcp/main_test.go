package main

import "testing"

func TestResolveTokenPrefersExplicitValue(t *testing.T) {
	t.Setenv(defaultTokenEnvVar, "aht_from_env")

	token, err := resolveToken("aht_explicit", defaultTokenEnvVar)
	if err != nil {
		t.Fatalf("resolveToken returned error: %v", err)
	}
	if token != "aht_explicit" {
		t.Fatalf("expected explicit token, got %q", token)
	}
}

func TestResolveTokenFallsBackToEnvironment(t *testing.T) {
	t.Setenv(defaultTokenEnvVar, "aht_from_env")

	token, err := resolveToken("", defaultTokenEnvVar)
	if err != nil {
		t.Fatalf("resolveToken returned error: %v", err)
	}
	if token != "aht_from_env" {
		t.Fatalf("expected token from env, got %q", token)
	}
}

func TestResolveTokenSupportsCustomEnvironmentVariable(t *testing.T) {
	t.Setenv("CUSTOM_AGENTHUB_TOKEN", "aht_custom")

	token, err := resolveToken("", "CUSTOM_AGENTHUB_TOKEN")
	if err != nil {
		t.Fatalf("resolveToken returned error: %v", err)
	}
	if token != "aht_custom" {
		t.Fatalf("expected token from custom env, got %q", token)
	}
}

func TestResolveTokenErrorsWhenMissing(t *testing.T) {
	_, err := resolveToken("", "MISSING_TOKEN")
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}
