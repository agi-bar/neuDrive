package services

import (
	"testing"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
)

func TestEnrichBundleDirectoryEntryDetectsConversationBundle(t *testing.T) {
	now := time.Date(2026, 4, 17, 4, 0, 0, 0, time.UTC)
	entry := models.FileTreeEntry{
		Path:          "/conversations/claude-web/demo/",
		Kind:          "directory",
		IsDirectory:   true,
		ContentType:   "directory",
		Metadata:      map[string]interface{}{},
		CreatedAt:     now,
		UpdatedAt:     now,
		MinTrustLevel: models.TrustLevelGuest,
	}

	descendants := []models.FileTreeEntry{
		{
			Path:          "/conversations/claude-web/demo/conversation.md",
			Kind:          "file",
			Content:       "# Demo Conversation\n\nArchived transcript body.",
			ContentType:   "text/markdown",
			Metadata:      map[string]interface{}{"source": "claude-web"},
			CreatedAt:     now,
			UpdatedAt:     now,
			MinTrustLevel: models.TrustLevelGuest,
		},
	}

	enriched := EnrichBundleDirectoryEntry(entry, descendants)
	if enriched.Kind != EntryKindConversationBundle {
		t.Fatalf("entry kind = %q, want %q", enriched.Kind, EntryKindConversationBundle)
	}
	if got := metadataString(enriched.Metadata, "bundle_kind"); got != BundleKindConversation {
		t.Fatalf("bundle_kind = %q, want %q", got, BundleKindConversation)
	}
	if got := metadataString(enriched.Metadata, "bundle_name"); got != "Demo Conversation" {
		t.Fatalf("bundle_name = %q, want %q", got, "Demo Conversation")
	}
	if got := metadataString(enriched.Metadata, "bundle_primary_path"); got != "/conversations/claude-web/demo/conversation.md" {
		t.Fatalf("bundle_primary_path = %q", got)
	}
}

func TestEnrichBundleDirectoryEntryKeepsConversationPlatformDirectoryPlain(t *testing.T) {
	now := time.Date(2026, 4, 17, 4, 0, 0, 0, time.UTC)
	entry := models.FileTreeEntry{
		Path:          "/conversations/claude-web/",
		Kind:          "directory",
		IsDirectory:   true,
		ContentType:   "directory",
		Metadata:      map[string]interface{}{},
		CreatedAt:     now,
		UpdatedAt:     now,
		MinTrustLevel: models.TrustLevelGuest,
	}

	descendants := []models.FileTreeEntry{
		{
			Path:          "/conversations/claude-web/demo/conversation.md",
			Kind:          "file",
			Content:       "# Demo Conversation\n\nArchived transcript body.",
			ContentType:   "text/markdown",
			Metadata:      map[string]interface{}{"source": "claude-web"},
			CreatedAt:     now,
			UpdatedAt:     now,
			MinTrustLevel: models.TrustLevelGuest,
		},
	}

	enriched := EnrichBundleDirectoryEntry(entry, descendants)
	if enriched.Kind != "directory" {
		t.Fatalf("entry kind = %q, want directory", enriched.Kind)
	}
	if got := metadataString(enriched.Metadata, "bundle_kind"); got != "" {
		t.Fatalf("bundle_kind = %q, want empty", got)
	}
}
