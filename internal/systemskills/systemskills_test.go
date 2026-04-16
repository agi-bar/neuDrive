package systemskills

import (
	"context"
	"strings"
	"testing"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
)

type stubConnections struct {
	items []models.Connection
}

func (s stubConnections) ListByUser(context.Context, uuid.UUID) ([]models.Connection, error) {
	return s.items, nil
}

type stubGrants struct {
	items []models.OAuthGrantResponse
}

func (s stubGrants) ListGrants(context.Context, uuid.UUID) ([]models.OAuthGrantResponse, error) {
	return s.items, nil
}

type stubProfiles struct {
	items []models.MemoryProfile
}

func (s stubProfiles) GetProfile(context.Context, uuid.UUID) ([]models.MemoryProfile, error) {
	return s.items, nil
}

type stubProjects struct {
	items []models.Project
}

func (s stubProjects) List(context.Context, uuid.UUID) ([]models.Project, error) {
	return s.items, nil
}

type stubSkills struct {
	items []models.SkillSummary
}

func (s stubSkills) ListSkillSummaries(context.Context, uuid.UUID, int) ([]models.SkillSummary, error) {
	return s.items, nil
}

func TestListEntriesPortabilityRoot(t *testing.T) {
	entries, ok := ListEntries("/skills/portability")
	if !ok {
		t.Fatal("expected portability root to be handled")
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 platform directories, got %d", len(entries))
	}
}

func TestListEntriesRootIncludesSkillsDirectory(t *testing.T) {
	entries, ok := ListEntries("/")
	if !ok {
		t.Fatal("expected / root to be handled")
	}
	if len(entries) != 1 {
		t.Fatalf("expected only skills root, got %d", len(entries))
	}
	if entries[0].Path != "/skills/" {
		t.Fatalf("expected /skills/ entry, got %q", entries[0].Path)
	}
}

func TestListEntriesSkillsRootIncludesNeuDrive(t *testing.T) {
	entries, ok := ListEntries("/skills")
	if !ok {
		t.Fatal("expected /skills root to be handled")
	}
	if len(entries) != 2 {
		t.Fatalf("expected neudrive + portability roots, got %d", len(entries))
	}
	paths := []string{entries[0].Path, entries[1].Path}
	if !strings.Contains(strings.Join(paths, " "), "/skills/neudrive/") {
		t.Fatalf("expected neudrive root in %v", paths)
	}
}

func TestReadEntryNeuDriveSkill(t *testing.T) {
	entry, ok, err := ReadEntry("/skills/neudrive/SKILL.md")
	if err != nil {
		t.Fatalf("ReadEntry() error = %v", err)
	}
	if !ok {
		t.Fatal("expected neudrive system skill to be found")
	}
	if entry.Kind != "skill" {
		t.Fatalf("expected kind=skill, got %q", entry.Kind)
	}
	if !strings.Contains(entry.Content, "neuDrive") {
		t.Fatalf("expected neuDrive skill content")
	}
	if got, _ := entry.Metadata["name"].(string); got != "neudrive" {
		t.Fatalf("expected skill name metadata, got %q", got)
	}
	if !strings.Contains(entry.Content, "$neudrive help") {
		t.Fatalf("expected help example in neuDrive skill content")
	}
}

func TestReadEntryChatGPTSkill(t *testing.T) {
	entry, ok, err := ReadEntry("/skills/portability/chatgpt/SKILL.md")
	if err != nil {
		t.Fatalf("ReadEntry() error = %v", err)
	}
	if !ok {
		t.Fatal("expected system skill to be found")
	}
	if entry.Kind != "skill" {
		t.Fatalf("expected kind=skill, got %q", entry.Kind)
	}
	if !strings.Contains(entry.Content, "ChatGPT Portability Manual") {
		t.Fatalf("expected ChatGPT handbook content")
	}
	if readOnly, _ := entry.Metadata["read_only"].(bool); !readOnly {
		t.Fatal("expected read_only metadata")
	}
}

func TestReadEntryGeneralPortabilitySkill(t *testing.T) {
	entry, ok, err := ReadEntry("/skills/portability/general/SKILL.md")
	if err != nil {
		t.Fatalf("ReadEntry() error = %v", err)
	}
	if !ok {
		t.Fatal("expected general portability skill to be found")
	}
	if entry.Kind != "skill" {
		t.Fatalf("expected kind=skill, got %q", entry.Kind)
	}
	if !strings.Contains(entry.Content, "General Platform Portability Manual") {
		t.Fatalf("expected general portability manual content")
	}
}

func TestBuildSnapshotNotConnected(t *testing.T) {
	snapshot := BuildSnapshot(context.Background(), uuid.Nil, models.TrustLevelFull, "chatgpt", SnapshotDeps{
		Connections: stubConnections{},
		Grants:      stubGrants{},
	})
	if snapshot.Connected != "no" {
		t.Fatalf("Connected = %q, want no", snapshot.Connected)
	}
	if !strings.Contains(snapshot.RecommendedNextStep, "Connect ChatGPT first") {
		t.Fatalf("unexpected next step: %q", snapshot.RecommendedNextStep)
	}
}

func TestBuildSnapshotProfileWithoutProjects(t *testing.T) {
	snapshot := BuildSnapshot(context.Background(), uuid.Nil, models.TrustLevelFull, "claude", SnapshotDeps{
		Connections: stubConnections{items: []models.Connection{{Platform: "claude"}}},
		Profiles:    stubProfiles{items: []models.MemoryProfile{{Category: "preferences", Content: "Use concise prose"}}},
		Projects:    stubProjects{},
		Skills: stubSkills{items: []models.SkillSummary{
			{Name: "writer", Path: "/skills/writer/SKILL.md", Source: "skills"},
			{Name: "portability/chatgpt", Path: "/skills/portability/chatgpt/SKILL.md", Source: "system", ReadOnly: true},
		}},
	})
	if snapshot.Connected != "yes" {
		t.Fatalf("Connected = %q, want yes", snapshot.Connected)
	}
	if !snapshot.ProfileDataPresent {
		t.Fatal("expected profile data to be detected")
	}
	if snapshot.ProjectsCount != 0 {
		t.Fatalf("ProjectsCount = %d, want 0", snapshot.ProjectsCount)
	}
	if snapshot.CustomSkillsCount != 1 {
		t.Fatalf("CustomSkillsCount = %d, want 1", snapshot.CustomSkillsCount)
	}
	if !strings.Contains(snapshot.RecommendedNextStep, "Migrate project context next") {
		t.Fatalf("unexpected next step: %q", snapshot.RecommendedNextStep)
	}
}

func TestRenderSkillDocumentIncludesSnapshot(t *testing.T) {
	entry, ok, err := ReadEntry("/skills/portability/codex/SKILL.md")
	if err != nil || !ok {
		t.Fatalf("ReadEntry() = ok:%v err:%v", ok, err)
	}

	rendered := RenderSkillDocument(entry.Content, "codex", Snapshot{
		Connected:           "unknown",
		ProfileDataPresent:  true,
		ProjectsCount:       2,
		CustomSkillsCount:   5,
		RecommendedNextStep: "Review knowledge files next.",
	})

	if strings.Contains(rendered, currentUserSnapshotPlaceholder) {
		t.Fatal("placeholder should be replaced")
	}
	if !strings.Contains(rendered, "Connected to Codex: unknown") {
		t.Fatalf("rendered content missing snapshot")
	}
}

func TestExportSkillFilesNeuDrive(t *testing.T) {
	files, err := ExportSkillFiles("neudrive")
	if err != nil {
		t.Fatalf("ExportSkillFiles() error = %v", err)
	}
	for _, required := range []string{
		"SKILL.md",
		"commands/create.md",
		"commands/export.md",
		"commands/log.md",
		"commands/import.md",
		"commands/ls.md",
		"commands/list.md",
		"commands/read.md",
		"commands/search.md",
		"commands/stats.md",
		"commands/status.md",
		"commands/token.md",
		"commands/write.md",
		"commands/help.md",
		"references/platforms/codex.md",
		"references/platforms/claude.md",
	} {
		if _, ok := files[required]; !ok {
			t.Fatalf("expected %s in exported skill files", required)
		}
	}
}
