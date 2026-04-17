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

func TestBundleContextForPathInfersSyntheticConversationBundle(t *testing.T) {
	readDirectory := func(path string) (*models.FileTreeEntry, error) {
		return nil, ErrEntryNotFound
	}
	listDirectory := func(path string) ([]models.FileTreeEntry, error) {
		switch path {
		case "/conversations/claude-web/demo":
			return []models.FileTreeEntry{
				{
					Path:        "/conversations/claude-web/demo/conversation.md",
					Kind:        "file",
					Content:     "# Demo Conversation\n\nArchived transcript body.",
					ContentType: "text/markdown",
					Metadata:    map[string]interface{}{"source": "claude-web"},
				},
			}, nil
		default:
			return nil, ErrEntryNotFound
		}
	}

	context := BundleContextForPath("/conversations/claude-web/demo", readDirectory, listDirectory)
	if context == nil {
		t.Fatal("expected bundle context")
	}
	if context.Kind != BundleKindConversation {
		t.Fatalf("context kind = %q, want %q", context.Kind, BundleKindConversation)
	}
	if context.Path != "/conversations/claude-web/demo" {
		t.Fatalf("context path = %q, want /conversations/claude-web/demo", context.Path)
	}
	if context.Name != "Demo Conversation" {
		t.Fatalf("context name = %q, want Demo Conversation", context.Name)
	}
	if context.PrimaryPath != "/conversations/claude-web/demo/conversation.md" {
		t.Fatalf("context primary path = %q", context.PrimaryPath)
	}
}

func TestBundleContextForPathAscendsToSyntheticConversationBundle(t *testing.T) {
	readDirectory := func(path string) (*models.FileTreeEntry, error) {
		return nil, ErrEntryNotFound
	}
	listDirectory := func(path string) ([]models.FileTreeEntry, error) {
		switch path {
		case "/conversations/claude-web/demo/assets":
			return []models.FileTreeEntry{
				{
					Path:        "/conversations/claude-web/demo/assets/notes.md",
					Kind:        "file",
					Content:     "Nested note",
					ContentType: "text/markdown",
				},
			}, nil
		case "/conversations/claude-web/demo":
			return []models.FileTreeEntry{
				{
					Path:        "/conversations/claude-web/demo/conversation.md",
					Kind:        "file",
					Content:     "# Demo Conversation\n\nArchived transcript body.",
					ContentType: "text/markdown",
					Metadata:    map[string]interface{}{"source": "claude-web"},
				},
				{
					Path:        "/conversations/claude-web/demo/assets/notes.md",
					Kind:        "file",
					Content:     "Nested note",
					ContentType: "text/markdown",
				},
			}, nil
		default:
			return nil, ErrEntryNotFound
		}
	}

	context := BundleContextForPath("/conversations/claude-web/demo/assets", readDirectory, listDirectory)
	if context == nil {
		t.Fatal("expected bundle context")
	}
	if context.Kind != BundleKindConversation {
		t.Fatalf("context kind = %q, want %q", context.Kind, BundleKindConversation)
	}
	if context.Path != "/conversations/claude-web/demo" {
		t.Fatalf("context path = %q, want /conversations/claude-web/demo", context.Path)
	}
	if context.RelativePath != "assets" {
		t.Fatalf("context relative path = %q, want assets", context.RelativePath)
	}
}
