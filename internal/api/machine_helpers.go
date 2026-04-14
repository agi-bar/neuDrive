package api

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
	"github.com/google/uuid"
)

type SearchHit struct {
	Path    string  `json:"path"`
	Source  string  `json:"source,omitempty"`
	Type    string  `json:"type"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
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

	prefixes := publicSearchPrefixes(scope)

	results := make([]SearchHit, 0, 64)
	seen := make(map[string]bool)
	for _, prefix := range prefixes {
		entries, err := s.FileTreeService.Search(ctx, userID, query, trustLevel, prefix)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			publicPath := hubpath.StorageToPublic(entry.Path)
			if isHiddenPublicFeaturePath(publicPath) {
				continue
			}
			if seen[publicPath] {
				continue
			}
			seen[publicPath] = true

			snippet := snippetText(entry.Content)
			if desc, ok := entry.Metadata["description"].(string); ok && snippet == "" {
				snippet = snippetText(desc)
			}
			results = append(results, SearchHit{
				Path:    publicPath,
				Source:  services.EntrySource(&entry),
				Type:    entry.Kind,
				Snippet: snippet,
				Score:   1,
			})
		}
	}

	return results, nil
}

func (s *Server) listSkills(ctx context.Context, userID uuid.UUID, trustLevel int) ([]models.SkillSummary, error) {
	return s.FileTreeService.ListSkillSummaries(ctx, userID, trustLevel)
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
