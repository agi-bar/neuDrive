package services

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
)

func TestNormalizeBundleMode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", bundleModeMerge},
		{"merge", bundleModeMerge},
		{"mirror", bundleModeMirror},
		{"MERGE", bundleModeMerge},
		{"invalid", ""},
	}

	for _, tt := range tests {
		if got := normalizeBundleMode(tt.input); got != tt.want {
			t.Fatalf("normalizeBundleMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCleanBundleRelativePath(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"SKILL.md", "SKILL.md", false},
		{"references/guide.md", "references/guide.md", false},
		{"/references/guide.md", "references/guide.md", false},
		{"../secret.txt", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		got, err := cleanBundleRelativePath(tt.input)
		if (err != nil) != tt.wantErr {
			t.Fatalf("cleanBundleRelativePath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if err == nil && got != tt.want {
			t.Fatalf("cleanBundleRelativePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDecodeBundleBlob(t *testing.T) {
	data := []byte("binary payload")
	sum := sha256.Sum256(data)
	blob := models.BundleBlobFile{
		ContentBase64: base64.StdEncoding.EncodeToString(data),
		ContentType:   "application/octet-stream",
		SizeBytes:     int64(len(data)),
		SHA256:        hex.EncodeToString(sum[:]),
	}

	got, contentType, err := decodeBundleBlob("assets/file.bin", blob)
	if err != nil {
		t.Fatalf("decodeBundleBlob() error = %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("decodeBundleBlob() data mismatch: got %q want %q", got, data)
	}
	if contentType != "application/octet-stream" {
		t.Fatalf("decodeBundleBlob() contentType = %q", contentType)
	}
}

func TestParseBundleMemoryTimes(t *testing.T) {
	createdAt := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(48 * time.Hour)

	gotCreatedAt, gotExpiresAt, err := parseBundleMemoryTimes(models.BundleMemoryItem{
		Content:   "hello",
		CreatedAt: createdAt.Format(time.RFC3339),
		ExpiresAt: expiresAt.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("parseBundleMemoryTimes() error = %v", err)
	}
	if !gotCreatedAt.Equal(createdAt) {
		t.Fatalf("createdAt = %v, want %v", gotCreatedAt, createdAt)
	}
	if gotExpiresAt == nil || !gotExpiresAt.Equal(expiresAt) {
		t.Fatalf("expiresAt = %v, want %v", gotExpiresAt, expiresAt)
	}
}
