package services

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// GenerateAPIKey
// ---------------------------------------------------------------------------

func TestGenerateAPIKey(t *testing.T) {
	raw, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error: %v", err)
	}

	if !strings.HasPrefix(raw, "ahk_") {
		t.Errorf("raw key should start with 'ahk_', got %q", raw[:8])
	}
	// ahk_ + 64 hex chars (32 bytes)
	if len(raw) != 4+64 {
		t.Errorf("raw key length = %d, want %d", len(raw), 4+64)
	}
	// SHA-256 hash = 64 hex chars
	if len(hash) != 64 {
		t.Errorf("hashed key length = %d, want 64", len(hash))
	}
	if len(prefix) != 12 {
		t.Errorf("prefix length = %d, want 12", len(prefix))
	}
	if prefix != raw[:12] {
		t.Errorf("prefix should be first 12 chars of raw key")
	}
}

func TestGenerateAPIKeyUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		raw, _, _, err := GenerateAPIKey()
		if err != nil {
			t.Fatalf("GenerateAPIKey() error: %v", err)
		}
		if seen[raw] {
			t.Fatalf("duplicate key generated on iteration %d", i)
		}
		seen[raw] = true
	}
}

func TestHashAPIKeyConsistency(t *testing.T) {
	raw, hash, _, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error: %v", err)
	}
	// Hashing the same raw key should produce the same hash.
	h2 := HashAPIKey(raw)
	if h2 != hash {
		t.Errorf("HashAPIKey(raw) = %q, want %q", h2, hash)
	}
}

// ---------------------------------------------------------------------------
// generateToken
// ---------------------------------------------------------------------------

func TestGenerateToken(t *testing.T) {
	raw, hash, prefix, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken() error: %v", err)
	}

	if !strings.HasPrefix(raw, "ndt_") {
		t.Errorf("raw token should start with 'ndt_', got %q", raw[:8])
	}
	// ndt_ + 40 hex chars (20 bytes)
	if len(raw) != 4+40 {
		t.Errorf("raw token length = %d, want %d", len(raw), 4+40)
	}
	if len(hash) != 64 {
		t.Errorf("hashed token length = %d, want 64", len(hash))
	}
	if len(prefix) != 12 {
		t.Errorf("prefix length = %d, want 12", len(prefix))
	}

	// hashToken should match
	h2 := hashToken(raw)
	if h2 != hash {
		t.Errorf("hashToken(raw) = %q, want %q", h2, hash)
	}
}

func TestGenerateTokenUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		raw, _, _, err := generateToken()
		if err != nil {
			t.Fatalf("generateToken() error: %v", err)
		}
		if seen[raw] {
			t.Fatalf("duplicate token generated on iteration %d", i)
		}
		seen[raw] = true
	}
}

// ---------------------------------------------------------------------------
// OAuth helpers
// ---------------------------------------------------------------------------

func TestGenerateClientID(t *testing.T) {
	id, err := generateClientID()
	if err != nil {
		t.Fatalf("generateClientID() error: %v", err)
	}
	if !strings.HasPrefix(id, "ahc_") {
		t.Errorf("client ID should start with 'ahc_', got %q", id)
	}
	// ahc_ + 32 hex chars (16 bytes)
	if len(id) != 4+32 {
		t.Errorf("client ID length = %d, want %d", len(id), 4+32)
	}
}

func TestGenerateClientSecret(t *testing.T) {
	secret, err := generateClientSecret()
	if err != nil {
		t.Fatalf("generateClientSecret() error: %v", err)
	}
	if !strings.HasPrefix(secret, "ahs_") {
		t.Errorf("client secret should start with 'ahs_', got %q", secret)
	}
	// ahs_ + 64 hex chars (32 bytes)
	if len(secret) != 4+64 {
		t.Errorf("client secret length = %d, want %d", len(secret), 4+64)
	}
}

func TestGenerateAuthCode(t *testing.T) {
	code, err := generateAuthCode()
	if err != nil {
		t.Fatalf("generateAuthCode() error: %v", err)
	}
	// 64 hex chars (32 bytes)
	if len(code) != 64 {
		t.Errorf("auth code length = %d, want 64", len(code))
	}
}

// ---------------------------------------------------------------------------
// diffRatio (memory_service.go)
// ---------------------------------------------------------------------------

func TestDiffRatio(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want float64
	}{
		{"identical", "hello", "hello", 0.0},
		{"empty both", "", "", 0.0},
		{"one empty", "hello", "", 1.0},
		{"other empty", "", "hello", 1.0},
		{"completely different", "aaaaa", "bbbbb", 1.0},
		{"one char diff", "hello", "hellx", 0.2},
		{"length diff", "hello", "hell", 0.2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffRatio(tt.a, tt.b)
			if got < tt.want-0.01 || got > tt.want+0.01 {
				t.Errorf("diffRatio(%q, %q) = %f, want ~%f", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
