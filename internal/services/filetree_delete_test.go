package services

import (
	"reflect"
	"testing"

	"github.com/agi-bar/agenthub/internal/models"
)

func TestDeletableEntriesForDeletionSkipsProtectedDescendants(t *testing.T) {
	entries := []models.FileTreeEntry{
		{Path: "/skills/"},
		{Path: "/skills/custom/"},
		{Path: "/skills/custom/SKILL.md"},
		{Path: "/skills/custom/assets/logo.txt"},
		{Path: "/skills/agenthub/"},
		{Path: "/skills/agenthub/SKILL.md"},
		{Path: "/skills/portability/"},
		{Path: "/skills/portability/chatgpt/SKILL.md"},
	}

	deletable := deletableEntriesForDeletion("/skills", entries)
	paths := make([]string, 0, len(deletable))
	for _, entry := range deletable {
		paths = append(paths, entry.Path)
	}

	want := []string{
		"/skills/custom/assets/logo.txt",
		"/skills/custom/SKILL.md",
		"/skills/custom/",
		"/skills/",
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("deletable paths mismatch:\n got %v\nwant %v", paths, want)
	}
}
