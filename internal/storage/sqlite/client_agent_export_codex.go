package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
)

func (c *Client) importCodexInventory(ctx context.Context, platform string, inventory CodexInventory, result *AgentImportResult) error {
	for _, bundle := range inventory.Bundles {
		if err := c.importCodexBundle(ctx, platform, bundle, result); err != nil {
			return err
		}
	}
	if err := c.importCodexConversations(ctx, platform, inventory.Conversations, result); err != nil {
		return err
	}
	return nil
}

func (c *Client) importCodexBundle(ctx context.Context, platform string, bundle ClaudeBundle, result *AgentImportResult) error {
	if len(bundle.Files) == 0 {
		return nil
	}
	bundleName := codexBundleTargetName(bundle)
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
		if err := c.writeClaudeFileRecord(ctx, target, platform, file, "skill_file", models.TrustLevelWork); err != nil {
			return err
		}
		result.Paths = append(result.Paths, target)
	}
	if !hasSkill {
		target := filepath.ToSlash(filepath.Join("/skills", bundleName, "SKILL.md"))
		content := renderSyntheticCodexSkill(bundle, bundleName)
		if _, err := c.store.WriteEntry(ctx, c.userID, target, content, "text/markdown", models.FileTreeWriteOptions{
			Kind:          "skill_file",
			MinTrustLevel: models.TrustLevelWork,
			Metadata: map[string]interface{}{
				"source_platform": platform,
				"capture_mode":    "agent",
				"exactness":       fallbackExactness(bundle.Exactness),
				"source_kind":     normalizeCodexBundleKind(bundle.Kind),
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

func (c *Client) importCodexConversations(ctx context.Context, platform string, conversations []ClaudeConversation, result *AgentImportResult) error {
	if len(conversations) == 0 {
		return nil
	}
	type manifestEntry struct {
		RootPath         string   `json:"root_path"`
		TranscriptPath   string   `json:"transcript_path"`
		ConversationPath string   `json:"conversation_path"`
		Title            string   `json:"title"`
		SessionID        string   `json:"session_id,omitempty"`
		ProjectName      string   `json:"project_name,omitempty"`
		StartedAt        string   `json:"started_at,omitempty"`
		MessageCount     int      `json:"message_count"`
		SourcePaths      []string `json:"source_paths,omitempty"`
		Exactness        string   `json:"exactness,omitempty"`
	}
	manifest := make([]manifestEntry, 0, len(conversations))
	importedAt := time.Now().UTC().Format(time.RFC3339)
	for _, convo := range conversations {
		if len(convo.Messages) == 0 {
			continue
		}
		normalized := buildNormalizedConversationFromCodexConversation(convo, platform, importedAt)
		rootPath := codexConversationArchiveRoot(convo)
		transcriptPath := path.Join(rootPath, "conversation.md")
		conversationPath := path.Join(rootPath, "conversation.json")
		if err := c.store.EnsureDirectoryWithMetadata(ctx, c.userID, rootPath, ConversationBundleDirectoryMetadata(normalized, transcriptPath, conversationPath), models.TrustLevelWork); err != nil {
			return err
		}
		transcript := renderNormalizedConversationMarkdown(normalized)
		metadata := map[string]interface{}{
			"source_platform": platform,
			"capture_mode":    "archive",
			"exactness":       fallbackExactness(convo.Exactness),
			"source_paths":    convo.SourcePaths,
			"session_id":      strings.TrimSpace(convo.SessionID),
			"project_name":    strings.TrimSpace(convo.ProjectName),
		}
		if _, err := c.store.WriteEntry(ctx, c.userID, transcriptPath, transcript, "text/markdown", models.FileTreeWriteOptions{
			Kind:          "file",
			MinTrustLevel: models.TrustLevelWork,
			Metadata: mergeConversationMetadata(metadata, map[string]interface{}{
				"import_kind":  "conversation_archive_transcript",
				"storage_mode": "canonical",
			}),
		}); err != nil {
			return err
		}
		conversationJSON, err := MarshalNormalizedConversationDocument(normalized, transcriptPath)
		if err != nil {
			return err
		}
		if _, err := c.store.WriteEntry(ctx, c.userID, conversationPath, string(conversationJSON)+"\n", "application/json", models.FileTreeWriteOptions{
			Kind:          "file",
			MinTrustLevel: models.TrustLevelWork,
			Metadata: mergeConversationMetadata(metadata, map[string]interface{}{
				"import_kind":  "conversation_archive_normalized",
				"storage_mode": "canonical",
			}),
		}); err != nil {
			return err
		}
		result.Conversations++
		result.Imported++
		result.Paths = append(result.Paths, transcriptPath, conversationPath)
		manifest = append(manifest, manifestEntry{
			RootPath:         rootPath,
			TranscriptPath:   transcriptPath,
			ConversationPath: conversationPath,
			Title:            strings.TrimSpace(convo.Name),
			SessionID:        strings.TrimSpace(convo.SessionID),
			ProjectName:      strings.TrimSpace(convo.ProjectName),
			StartedAt:        strings.TrimSpace(convo.StartedAt),
			MessageCount:     len(convo.Messages),
			SourcePaths:      append([]string{}, convo.SourcePaths...),
			Exactness:        fallbackExactness(convo.Exactness),
		})
	}
	if len(manifest) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	indexPath := hubpath.ConversationIndexPath("codex")
	if _, err := c.store.WriteEntry(ctx, c.userID, indexPath, string(data)+"\n", "application/json", models.FileTreeWriteOptions{
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

func codexBundleTargetName(bundle ClaudeBundle) string {
	name := normalizeClaudeName(bundle.Name, "codex-skill")
	switch normalizeCodexBundleKind(bundle.Kind) {
	case "skill":
		return name
	case "bundled-skill":
		return "codex-bundled-" + name
	default:
		return "codex-skill-" + name
	}
}

func renderSyntheticCodexSkill(bundle ClaudeBundle, bundleName string) string {
	title := strings.TrimSpace(bundle.Name)
	if title == "" {
		title = bundleName
	}
	description := strings.TrimSpace(bundle.Description)
	if description == "" {
		description = fmt.Sprintf("Imported from Codex %s %q.", normalizeCodexBundleKind(bundle.Kind), title)
	}
	lines := []string{
		"---",
		fmt.Sprintf("name: %s", bundleName),
		fmt.Sprintf("description: %q", description),
		"source: codex",
		"---",
		fmt.Sprintf("# %s", title),
		"",
		fmt.Sprintf("Imported from Codex `%s` assets.", normalizeCodexBundleKind(bundle.Kind)),
	}
	return strings.Join(lines, "\n") + "\n"
}

func codexConversationPath(convo ClaudeConversation) string {
	return path.Join(codexConversationArchiveRoot(convo), "conversation.md")
}

func codexConversationArchiveRoot(convo ClaudeConversation) string {
	return strings.TrimSuffix(hubpath.ConversationDir("codex", codexConversationArchiveKey(convo)), "/")
}

func codexConversationArchiveKey(convo ClaudeConversation) string {
	date := "unknown"
	if parsed, ok := parseClaudeTimestamp(convo.StartedAt); ok {
		date = parsed.UTC().Format("2006-01-02")
	}
	slug := normalizeClaudeName(convo.Name, "conversation")
	if strings.TrimSpace(convo.SessionID) != "" {
		slug = fmt.Sprintf("%s-%s", slug, normalizeClaudeName(convo.SessionID, "session"))
	}
	return date + "-" + slug + "-compact"
}

func buildNormalizedConversationFromCodexConversation(convo ClaudeConversation, platform, importedAt string) NormalizedConversation {
	title := strings.TrimSpace(convo.Name)
	if title == "" {
		title = "Codex conversation"
	}
	turns := make([]NormalizedTurn, 0, len(convo.Messages))
	updatedAt := strings.TrimSpace(convo.StartedAt)
	for index, message := range convo.Messages {
		content := strings.TrimSpace(message.Content)
		parts := normalizedPartsFromClaudeConversationMessage(message)
		if content == "" && len(parts) == 0 {
			continue
		}
		timestamp := strings.TrimSpace(message.Timestamp)
		if timestamp != "" {
			updatedAt = timestamp
		}
		turns = append(turns, NormalizedTurn{
			ID:                    fmt.Sprintf("turn_%04d", index+1),
			Role:                  normalizeConversationRole(message.Role),
			At:                    timestamp,
			SourceMessageID:       strings.TrimSpace(message.ID),
			ParentSourceMessageID: strings.TrimSpace(message.ParentID),
			SourceMessageKind:     strings.TrimSpace(message.Kind),
			Parts:                 parts,
		})
	}
	return NormalizedConversation{
		Version:              "neudrive.conversation/v1",
		SourcePlatform:       platform,
		SourceConversationID: strings.TrimSpace(convo.SessionID),
		Title:                title,
		ImportedAt:           strings.TrimSpace(importedAt),
		ImportStrategy:       "codex-local-scan",
		CreatedAt:            strings.TrimSpace(convo.StartedAt),
		UpdatedAt:            updatedAt,
		ProjectName:          strings.TrimSpace(convo.ProjectName),
		Exactness:            fallbackExactness(convo.Exactness),
		SourcePaths:          append([]string{}, convo.SourcePaths...),
		Provenance: map[string]interface{}{
			"summary":       strings.TrimSpace(convo.Summary),
			"message_count": len(turns),
			"source_format": "codex-jsonl",
		},
		Turns:     turns,
		TurnCount: len(turns),
	}
}

func normalizeCodexBundleKind(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "skill":
		return "skill"
	case "bundled-skill":
		return "bundled-skill"
	default:
		return "skill"
	}
}
