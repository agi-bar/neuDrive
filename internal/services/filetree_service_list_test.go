package services

import (
	"reflect"
	"testing"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
)

func TestImmediateChildEntriesRootShowsOnlyDirectChildren(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)

	entries := []models.FileTreeEntry{
		{
			Path:        "/hello.md",
			Kind:        "file",
			ContentType: "text/markdown",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
		{
			Path:        "/projects/demo/context.md",
			Kind:        "project_context",
			ContentType: "text/markdown",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
		{
			Path:        "/memory/profile/preferences.md",
			Kind:        "memory_profile",
			ContentType: "text/markdown",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
		{
			Path:        "/skills/demo/SKILL.md",
			Kind:        "skill",
			ContentType: "text/markdown",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
	}

	got := immediateChildEntries("/", userID, entries)

	gotPaths := make([]string, 0, len(got))
	for _, entry := range got {
		gotPaths = append(gotPaths, hubpath.NormalizePublic(entry.Path))
	}

	wantPaths := []string{
		"/memory/",
		"/projects/",
		"/skills/",
		"/hello.md",
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("immediateChildEntries(/) paths = %#v, want %#v", gotPaths, wantPaths)
	}
}

func TestImmediateChildEntriesSkillsRootCollapsesBundleFiles(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)

	entries := []models.FileTreeEntry{
		{
			Path:        "/skills/neudrive/SKILL.md",
			Kind:        "skill",
			ContentType: "text/markdown",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
		{
			Path:        "/skills/legacy-demo/SKILL.md",
			Kind:        "skill",
			ContentType: "text/markdown",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
		{
			Path:        "/skills/notes.md",
			Kind:        "file",
			ContentType: "text/markdown",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
		{
			Path:        "/skills/portability/chatgpt/SKILL.md",
			Kind:        "skill",
			ContentType: "text/markdown",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
	}

	got := immediateChildEntries("/skills", userID, entries)

	gotPaths := make([]string, 0, len(got))
	gotKinds := make(map[string]bool, len(got))
	for _, entry := range got {
		publicPath := hubpath.NormalizePublic(entry.Path)
		gotPaths = append(gotPaths, publicPath)
		gotKinds[publicPath] = entry.IsDirectory
	}

	wantPaths := []string{
		"/skills/legacy-demo/",
		"/skills/neudrive/",
		"/skills/portability/",
		"/skills/notes.md",
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("immediateChildEntries() paths = %#v, want %#v", gotPaths, wantPaths)
	}

	for _, path := range []string{"/skills/neudrive/", "/skills/legacy-demo/", "/skills/portability/"} {
		if !gotKinds[path] {
			t.Fatalf("expected %s to be rendered as a directory", path)
		}
	}
	if gotKinds["/skills/notes.md"] {
		t.Fatal("expected /skills/notes.md to remain a file")
	}
}

func TestImmediateChildEntriesProjectDirectoryKeepsFilesAndNestedFolders(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)

	entries := []models.FileTreeEntry{
		{
			Path:        "/projects/demo/context.md",
			Kind:        "project_context",
			ContentType: "text/markdown",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
		{
			Path:        "/projects/demo/log.jsonl",
			Kind:        "project_log",
			ContentType: "application/jsonl",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
		{
			Path:        "/projects/demo/docs/notes.md",
			Kind:        "file",
			ContentType: "text/markdown",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
	}

	got := immediateChildEntries("/projects/demo", userID, entries)

	gotPaths := make([]string, 0, len(got))
	gotKinds := make(map[string]bool, len(got))
	for _, entry := range got {
		publicPath := hubpath.NormalizePublic(entry.Path)
		gotPaths = append(gotPaths, publicPath)
		gotKinds[publicPath] = entry.IsDirectory
	}

	wantPaths := []string{
		"/projects/demo/docs/",
		"/projects/demo/context.md",
		"/projects/demo/log.jsonl",
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("immediateChildEntries(/projects/demo) paths = %#v, want %#v", gotPaths, wantPaths)
	}
	if !gotKinds["/projects/demo/docs/"] {
		t.Fatal("expected nested docs child to collapse into a directory")
	}
}

func TestImmediateChildEntriesMemoryRootShowsOnlySections(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)

	entries := []models.FileTreeEntry{
		{
			Path:        "/memory/profile/preferences.md",
			Kind:        "memory_profile",
			ContentType: "text/markdown",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
		{
			Path:        "/memory/scratch/2026-04-09/note.md",
			Kind:        "memory_scratch",
			ContentType: "text/markdown",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
	}

	got := immediateChildEntries("/memory", userID, entries)

	gotPaths := make([]string, 0, len(got))
	for _, entry := range got {
		gotPaths = append(gotPaths, hubpath.NormalizePublic(entry.Path))
	}

	wantPaths := []string{
		"/memory/profile/",
		"/memory/scratch/",
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("immediateChildEntries(/memory) paths = %#v, want %#v", gotPaths, wantPaths)
	}
}
