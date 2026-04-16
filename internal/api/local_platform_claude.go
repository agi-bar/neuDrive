package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/skillsarchive"
	sqlitestorage "github.com/agi-bar/neudrive/internal/storage/sqlite"
	"github.com/google/uuid"
)

func (s *Server) importClaudeLocalInventory(ctx context.Context, userID uuid.UUID, platform string, inventory sqlitestorage.ClaudeInventory, result *sqlitestorage.AgentImportResult) error {
	for _, project := range inventory.Projects {
		if err := s.importClaudeProject(ctx, userID, platform, project, result); err != nil {
			return err
		}
	}
	for _, bundle := range inventory.Bundles {
		if err := s.importClaudeBundle(ctx, userID, platform, bundle, result); err != nil {
			return err
		}
	}
	if err := s.importClaudeConversations(ctx, userID, platform, inventory.Conversations, result); err != nil {
		return err
	}
	for _, file := range inventory.Files {
		written, err := s.writeClaudeArchiveFile(ctx, userID, platform, file)
		if err != nil {
			return err
		}
		if written != "" {
			result.Artifacts++
			result.Archived++
			result.Paths = append(result.Paths, written)
		}
	}
	if written, err := s.writeLocalAgentArtifact(ctx, userID, platform, "sensitive-findings.json", inventory.SensitiveFindings); err != nil {
		return err
	} else if written != "" {
		result.Artifacts++
		result.Archived += len(inventory.SensitiveFindings)
		result.SensitiveFindings += len(inventory.SensitiveFindings)
		result.Paths = append(result.Paths, written)
	}
	if written, err := s.writeLocalAgentArtifact(ctx, userID, platform, "vault-candidates.json", inventory.VaultCandidates); err != nil {
		return err
	} else if written != "" {
		result.Artifacts++
		result.Archived += len(inventory.VaultCandidates)
		result.VaultCandidates += len(inventory.VaultCandidates)
		result.Paths = append(result.Paths, written)
	}
	return nil
}

func (s *Server) importClaudeProject(ctx context.Context, userID uuid.UUID, platform string, project sqlitestorage.ClaudeProjectSnapshot, result *sqlitestorage.AgentImportResult) error {
	name := normalizeClaudeName(project.Name, "claude-project")
	if name == "" {
		return nil
	}
	if _, err := s.ProjectService.Get(ctx, userID, name); err != nil {
		if _, err := s.ProjectService.Create(ctx, userID, name); err != nil {
			return err
		}
	}
	contextBody := renderClaudeProjectContext(project)
	if strings.TrimSpace(contextBody) != "" {
		if err := s.ProjectService.UpdateContext(ctx, userID, name, contextBody); err != nil {
			return err
		}
		if _, err := s.FileTreeService.WriteEntry(ctx, userID, hubpath.ProjectContextPath(name), contextBody, "text/markdown", models.FileTreeWriteOptions{
			Kind:          "project_context",
			MinTrustLevel: models.TrustLevelCollaborate,
			Metadata: map[string]interface{}{
				"source_platform": platform,
				"capture_mode":    "agent",
				"exactness":       fallbackAgentExactness(project.Exactness),
				"source_paths":    project.SourcePaths,
			},
		}); err != nil {
			return err
		}
		result.Projects++
		result.Imported++
		result.Paths = append(result.Paths, hubpath.ProjectContextPath(name))
	}
	for _, file := range project.Files {
		relPath := normalizeClaudeRelativePath(file.Path, file.SourcePath)
		if relPath == "" || relPath == "context.md" {
			continue
		}
		target := filepath.ToSlash(filepath.Join("/projects", name, relPath))
		if err := s.writeClaudeFileRecord(ctx, userID, target, platform, file, "project_file", models.TrustLevelCollaborate); err != nil {
			return err
		}
		result.ProjectFiles++
		result.Imported++
		result.Paths = append(result.Paths, target)
	}
	return nil
}

func (s *Server) importClaudeBundle(ctx context.Context, userID uuid.UUID, platform string, bundle sqlitestorage.ClaudeBundle, result *sqlitestorage.AgentImportResult) error {
	if len(bundle.Files) == 0 {
		return nil
	}
	bundleName := claudeBundleTargetName(bundle)
	if bundleName == "" {
		return nil
	}
	hasSkill := false
	for _, file := range bundle.Files {
		relPath := normalizeClaudeRelativePath(file.Path, file.SourcePath)
		if relPath == "" {
			continue
		}
		if strings.EqualFold(relPath, "SKILL.md") {
			hasSkill = true
		}
		target := filepath.ToSlash(filepath.Join("/skills", bundleName, relPath))
		if err := s.writeClaudeFileRecord(ctx, userID, target, platform, file, "skill_file", models.TrustLevelWork); err != nil {
			return err
		}
		result.Paths = append(result.Paths, target)
	}
	if !hasSkill {
		target := filepath.ToSlash(filepath.Join("/skills", bundleName, "SKILL.md"))
		content := renderSyntheticClaudeSkill(bundle, bundleName)
		if _, err := s.FileTreeService.WriteEntry(ctx, userID, target, content, "text/markdown", models.FileTreeWriteOptions{
			Kind:          "skill_file",
			MinTrustLevel: models.TrustLevelWork,
			Metadata: map[string]interface{}{
				"source_platform": platform,
				"capture_mode":    "agent",
				"exactness":       fallbackAgentExactness(bundle.Exactness),
				"source_kind":     normalizeClaudeKind(bundle.Kind),
				"source_paths":    bundle.SourcePaths,
			},
		}); err != nil {
			return err
		}
		result.Paths = append(result.Paths, target)
	}
	result.Bundles++
	result.Imported++
	return nil
}

func (s *Server) importClaudeConversations(ctx context.Context, userID uuid.UUID, platform string, conversations []sqlitestorage.ClaudeConversation, result *sqlitestorage.AgentImportResult) error {
	if len(conversations) == 0 {
		return nil
	}
	type manifestEntry struct {
		Path         string   `json:"path"`
		Title        string   `json:"title"`
		SessionID    string   `json:"session_id,omitempty"`
		ProjectName  string   `json:"project_name,omitempty"`
		StartedAt    string   `json:"started_at,omitempty"`
		MessageCount int      `json:"message_count"`
		SourcePaths  []string `json:"source_paths,omitempty"`
		Exactness    string   `json:"exactness,omitempty"`
	}
	manifest := make([]manifestEntry, 0, len(conversations))
	for _, convo := range conversations {
		if len(convo.Messages) == 0 {
			continue
		}
		target := claudeConversationPath(convo)
		content := renderClaudeConversationMarkdown(convo)
		if _, err := s.FileTreeService.WriteEntry(ctx, userID, target, content, "text/markdown", models.FileTreeWriteOptions{
			Kind:          "file",
			MinTrustLevel: models.TrustLevelWork,
			Metadata: map[string]interface{}{
				"source_platform": platform,
				"capture_mode":    "archive",
				"exactness":       fallbackAgentExactness(convo.Exactness),
				"source_paths":    convo.SourcePaths,
				"session_id":      strings.TrimSpace(convo.SessionID),
				"project_name":    strings.TrimSpace(convo.ProjectName),
			},
		}); err != nil {
			return err
		}
		result.Conversations++
		result.Imported++
		result.Paths = append(result.Paths, target)
		manifest = append(manifest, manifestEntry{
			Path:         target,
			Title:        strings.TrimSpace(convo.Name),
			SessionID:    strings.TrimSpace(convo.SessionID),
			ProjectName:  strings.TrimSpace(convo.ProjectName),
			StartedAt:    strings.TrimSpace(convo.StartedAt),
			MessageCount: len(convo.Messages),
			SourcePaths:  append([]string{}, convo.SourcePaths...),
			Exactness:    fallbackAgentExactness(convo.Exactness),
		})
	}
	if len(manifest) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	indexPath := "/memory/conversations/claude-code/index.json"
	if _, err := s.FileTreeService.WriteEntry(ctx, userID, indexPath, string(data)+"\n", "application/json", models.FileTreeWriteOptions{
		Kind:          "file",
		MinTrustLevel: models.TrustLevelWork,
		Metadata: map[string]interface{}{
			"source_platform": platform,
			"capture_mode":    "archive",
			"exactness":       "reference",
		},
	}); err != nil {
		return err
	}
	result.Artifacts++
	result.Archived++
	result.Paths = append(result.Paths, indexPath)
	return nil
}

func (s *Server) writeClaudeArchiveFile(ctx context.Context, userID uuid.UUID, platform string, file sqlitestorage.ClaudeFileRecord) (string, error) {
	relPath := normalizeClaudeRelativePath(file.Path, file.SourcePath)
	if relPath == "" {
		return "", nil
	}
	target := filepath.ToSlash(filepath.Join("/platforms", platform, relPath))
	if err := s.writeClaudeFileRecord(ctx, userID, target, platform, file, "file", models.TrustLevelWork); err != nil {
		return "", err
	}
	return target, nil
}

func (s *Server) writeClaudeFileRecord(ctx context.Context, userID uuid.UUID, target, platform string, file sqlitestorage.ClaudeFileRecord, kind string, trustLevel int) error {
	data, binary, err := decodeClaudeFileRecord(file)
	if err != nil {
		return err
	}
	contentType := strings.TrimSpace(file.ContentType)
	if contentType == "" {
		contentType = skillsarchive.DetectContentType(path.Base(target), data)
	}
	metadata := map[string]interface{}{
		"source_platform": platform,
		"capture_mode":    "agent",
		"exactness":       fallbackAgentExactness(file.Exactness),
		"source_paths":    mergedClaudeSourcePaths(file),
	}
	if binary {
		metadata["binary"] = true
		_, err = s.FileTreeService.WriteBinaryEntry(ctx, userID, target, data, contentType, models.FileTreeWriteOptions{
			Kind:          kind,
			MinTrustLevel: trustLevel,
			Metadata:      metadata,
		})
		return err
	}
	_, err = s.FileTreeService.WriteEntry(ctx, userID, target, string(data), contentType, models.FileTreeWriteOptions{
		Kind:          kind,
		MinTrustLevel: trustLevel,
		Metadata:      metadata,
	})
	return err
}

func decodeClaudeFileRecord(file sqlitestorage.ClaudeFileRecord) ([]byte, bool, error) {
	if strings.TrimSpace(file.ContentBase64) != "" {
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(file.ContentBase64))
		if err != nil {
			return nil, false, fmt.Errorf("decode %s: %w", file.Path, err)
		}
		return data, true, nil
	}
	return []byte(file.Content), false, nil
}

func mergedClaudeSourcePaths(file sqlitestorage.ClaudeFileRecord) []string {
	paths := make([]string, 0, len(file.SourcePaths)+1)
	if strings.TrimSpace(file.SourcePath) != "" {
		paths = append(paths, strings.TrimSpace(file.SourcePath))
	}
	for _, source := range file.SourcePaths {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		seen := false
		for _, existing := range paths {
			if existing == source {
				seen = true
				break
			}
		}
		if !seen {
			paths = append(paths, source)
		}
	}
	return paths
}

func claudeBundleTargetName(bundle sqlitestorage.ClaudeBundle) string {
	name := normalizeClaudeName(bundle.Name, "claude-bundle")
	switch normalizeClaudeKind(bundle.Kind) {
	case "skill":
		return name
	case "agent":
		return "claude-agent-" + name
	case "command":
		return "claude-command-" + name
	case "rule":
		return "claude-rule-" + name
	default:
		return "claude-bundle-" + name
	}
}

func renderSyntheticClaudeSkill(bundle sqlitestorage.ClaudeBundle, bundleName string) string {
	title := strings.TrimSpace(bundle.Name)
	if title == "" {
		title = bundleName
	}
	description := strings.TrimSpace(bundle.Description)
	if description == "" {
		description = fmt.Sprintf("Imported from Claude Code %s %q.", normalizeClaudeKind(bundle.Kind), title)
	}
	lines := []string{
		"---",
		fmt.Sprintf("name: %s", bundleName),
		fmt.Sprintf("description: %q", description),
		"source: claude-code",
		"---",
		fmt.Sprintf("# %s", title),
		"",
		fmt.Sprintf("Imported from Claude Code `%s` assets.", normalizeClaudeKind(bundle.Kind)),
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderClaudeProjectContext(project sqlitestorage.ClaudeProjectSnapshot) string {
	lines := []string{}
	if strings.TrimSpace(project.Context) != "" {
		lines = append(lines, strings.TrimSpace(project.Context), "")
	}
	lines = append(lines, fmt.Sprintf("- Exactness: %s", fallbackAgentExactness(project.Exactness)))
	if len(project.SourcePaths) > 0 {
		lines = append(lines, "- Source paths:")
		for _, source := range project.SourcePaths {
			source = strings.TrimSpace(source)
			if source == "" {
				continue
			}
			lines = append(lines, "  - "+source)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderClaudeConversationMarkdown(convo sqlitestorage.ClaudeConversation) string {
	title := strings.TrimSpace(convo.Name)
	if title == "" {
		title = "Claude Code conversation"
	}
	lines := []string{
		"# " + title,
		"",
	}
	if strings.TrimSpace(convo.ProjectName) != "" {
		lines = append(lines, fmt.Sprintf("Project: %s", strings.TrimSpace(convo.ProjectName)), "")
	}
	if strings.TrimSpace(convo.StartedAt) != "" {
		lines = append(lines, fmt.Sprintf("Started: %s", strings.TrimSpace(convo.StartedAt)), "")
	}
	if strings.TrimSpace(convo.Summary) != "" {
		lines = append(lines, fmt.Sprintf("Summary: %s", strings.TrimSpace(convo.Summary)), "")
	}
	lines = append(lines, fmt.Sprintf("Exactness: %s", fallbackAgentExactness(convo.Exactness)), "", "---", "")
	for _, message := range convo.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "assistant"
		}
		header := strings.Title(strings.ReplaceAll(role, "_", " "))
		if strings.TrimSpace(message.Timestamp) != "" {
			header = fmt.Sprintf("%s (%s)", header, strings.TrimSpace(message.Timestamp))
		}
		lines = append(lines, "## "+header, "", strings.TrimSpace(message.Content), "")
	}
	if len(convo.SourcePaths) > 0 {
		lines = append(lines, "Source paths:")
		for _, source := range convo.SourcePaths {
			source = strings.TrimSpace(source)
			if source == "" {
				continue
			}
			lines = append(lines, "- "+source)
		}
		lines = append(lines, "")
	}
	return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
}

func claudeConversationPath(convo sqlitestorage.ClaudeConversation) string {
	date := "unknown"
	if parsed, ok := parseClaudeTimestamp(convo.StartedAt); ok {
		date = parsed.UTC().Format("2006-01-02")
	}
	slug := normalizeClaudeName(convo.Name, "conversation")
	if strings.TrimSpace(convo.SessionID) != "" {
		slug = fmt.Sprintf("%s-%s", slug, normalizeClaudeName(convo.SessionID, "session"))
	}
	return filepath.ToSlash(filepath.Join("/memory/conversations/claude-code", date+"-"+slug+".md"))
}

func parseClaudeTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05 -0700 MST", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func normalizeClaudeKind(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "skill", "agent", "command", "rule":
		return raw
	default:
		return "bundle"
	}
}

func normalizeClaudeRelativePath(primary, fallback string) string {
	candidate := strings.TrimSpace(primary)
	if candidate == "" {
		candidate = strings.TrimSpace(fallback)
	}
	candidate = filepath.ToSlash(candidate)
	candidate = strings.TrimPrefix(candidate, "/")
	if candidate == "" {
		return ""
	}
	parts := []string{}
	for _, part := range strings.Split(candidate, "/") {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return ""
	}
	return path.Clean(strings.Join(parts, "/"))
}

func normalizeClaudeName(raw, fallback string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		raw = fallback
	}
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
}
