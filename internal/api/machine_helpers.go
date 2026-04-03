package api

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
)

type SearchHit struct {
	Path    string  `json:"path"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

type SkillSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Server) buildAgentProfile(ctx context.Context, userID uuid.UUID, category string) (map[string]interface{}, error) {
	user, err := s.UserService.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	profiles, err := s.MemoryService.GetProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	if category != "" {
		filtered := make([]models.MemoryProfile, 0, len(profiles))
		for _, profile := range profiles {
			if profile.Category == category {
				filtered = append(filtered, profile)
			}
		}
		profiles = filtered
	}

	return map[string]interface{}{
		"slug":         user.Slug,
		"display_name": user.DisplayName,
		"timezone":     user.Timezone,
		"language":     user.Language,
		"profiles":     profiles,
	}, nil
}

func (s *Server) searchHub(ctx context.Context, userID uuid.UUID, trustLevel int, query, scope string) ([]SearchHit, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		scope = "all"
	}

	results := make([]SearchHit, 0, 64)
	if scope == "all" || scope == "memory" {
		entries, err := s.FileTreeService.Search(ctx, userID, query, trustLevel, "")
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			results = append(results, SearchHit{
				Path:    hubpath.StorageToPublic(entry.Path),
				Snippet: snippetText(entry.Content),
				Score:   1,
			})
		}
	}

	if scope == "all" || scope == "inbox" {
		messages, err := s.InboxService.Search(ctx, userID, query, "")
		if err != nil {
			return nil, err
		}
		for _, message := range messages {
			results = append(results, SearchHit{
				Path:    "/inbox/" + message.ID.String(),
				Snippet: snippetText(strings.TrimSpace(message.Subject + "\n" + message.Body)),
				Score:   0.9,
			})
		}
	}

	return results, nil
}

func (s *Server) listSkills(ctx context.Context, userID uuid.UUID, trustLevel int) ([]SkillSummary, error) {
	entries, err := s.FileTreeService.List(ctx, userID, "/skills", trustLevel)
	if err != nil {
		return nil, err
	}

	skills := make([]SkillSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDirectory {
			continue
		}

		name := hubpath.BaseName(entry.Path)
		skill := SkillSummary{Name: name}
		if skillDoc, err := s.FileTreeService.Read(ctx, userID, entry.Path+"SKILL.md", trustLevel); err == nil {
			skill.Description = skillDescription(skillDoc.Content)
		}
		skills = append(skills, skill)
	}
	return skills, nil
}

func skillDescription(markdown string) string {
	lines := strings.Split(markdown, "\n")
	paragraph := make([]string, 0, 4)
	started := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if started {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "#") && !started {
			continue
		}
		started = true
		paragraph = append(paragraph, line)
	}

	return strings.TrimSpace(strings.Join(paragraph, " "))
}

func snippetText(raw string) string {
	raw = strings.Join(strings.Fields(raw), " ")
	if len(raw) <= 180 {
		return raw
	}
	// Truncate at a valid UTF-8 rune boundary to avoid corrupting multi-byte characters.
	truncated := raw[:177]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return strings.TrimSpace(truncated) + "..."
}
