package services

import (
	"context"
	"errors"
	"testing"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
)

func TestFileTreeServiceReadSystemSkillWithoutDB(t *testing.T) {
	svc := &FileTreeService{}

	entry, err := svc.Read(context.Background(), uuid.Nil, "/skills/portability/chatgpt/SKILL.md", models.TrustLevelGuest)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if entry == nil || entry.Kind != "skill" {
		t.Fatalf("expected skill entry, got %#v", entry)
	}
}

func TestFileTreeServiceListSystemDirectoriesWithoutDB(t *testing.T) {
	svc := &FileTreeService{}

	entries, err := svc.List(context.Background(), uuid.Nil, "/skills/portability", models.TrustLevelGuest)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
}

func TestFileTreeServiceListSkillSummariesIncludesSystemSkills(t *testing.T) {
	svc := &FileTreeService{}

	summaries, err := svc.ListSkillSummaries(context.Background(), uuid.Nil, models.TrustLevelGuest)
	if err != nil {
		t.Fatalf("ListSkillSummaries() error = %v", err)
	}
	if len(summaries) != 3 {
		t.Fatalf("expected 3 system skills, got %d", len(summaries))
	}
	for _, summary := range summaries {
		if summary.Source != "system" {
			t.Fatalf("expected source=system, got %q", summary.Source)
		}
		if !summary.ReadOnly {
			t.Fatalf("expected read_only summary for %q", summary.Name)
		}
	}
}

func TestFileTreeServiceRejectsWritesToProtectedSystemSkills(t *testing.T) {
	svc := &FileTreeService{}

	_, err := svc.WriteEntry(context.Background(), uuid.Nil, "/skills/portability/chatgpt/SKILL.md", "override", "text/markdown", models.FileTreeWriteOptions{})
	if !errors.Is(err, ErrReadOnlyPath) {
		t.Fatalf("WriteEntry() error = %v, want ErrReadOnlyPath", err)
	}

	err = svc.Delete(context.Background(), uuid.Nil, "/skills/portability/chatgpt/SKILL.md")
	if !errors.Is(err, ErrReadOnlyPath) {
		t.Fatalf("Delete() error = %v, want ErrReadOnlyPath", err)
	}
}
