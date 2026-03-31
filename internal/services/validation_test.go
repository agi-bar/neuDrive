package services

import (
	"strings"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		maxLen  int
		wantErr bool
	}{
		{"valid simple", "hello", 64, false},
		{"valid with dots", "auth.openai", 128, false},
		{"valid with dashes", "my-project", 128, false},
		{"valid with underscores", "my_project", 128, false},
		{"valid mixed", "api-key.v2_prod", 128, false},
		{"empty string", "", 64, true},
		{"too long", strings.Repeat("a", 65), 64, true},
		{"exactly at limit", strings.Repeat("a", 64), 64, false},
		{"contains space", "hello world", 128, true},
		{"contains slash", "path/to", 128, true},
		{"contains colon", "scope:read", 128, true},
		{"contains unicode", "名前", 128, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSlug(tt.input, tt.maxLen)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSlug(%q, %d) error = %v, wantErr %v", tt.input, tt.maxLen, err, tt.wantErr)
			}
		})
	}
}

func TestValidateContentLength(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		maxBytes int
		wantErr  bool
	}{
		{"empty", "", 1024, false},
		{"within limit", "hello", 1024, false},
		{"exactly at limit", strings.Repeat("x", 1024), 1024, false},
		{"over limit", strings.Repeat("x", 1025), 1024, true},
		{"large content within 64KB", strings.Repeat("x", 64*1024), 64 * 1024, false},
		{"large content over 64KB", strings.Repeat("x", 64*1024+1), 64 * 1024, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContentLength(tt.content, tt.maxBytes)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateContentLength(len=%d, %d) error = %v, wantErr %v", len(tt.content), tt.maxBytes, err, tt.wantErr)
			}
		})
	}
}
