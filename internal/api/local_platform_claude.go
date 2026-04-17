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
		normalized := buildNormalizedConversationFromClaudeConversation(convo, platform, importedAt)
		rootPath := claudeConversationArchiveRoot(convo)
		transcriptPath := path.Join(rootPath, "conversation.md")
		conversationPath := path.Join(rootPath, "conversation.json")
		if _, err := s.FileTreeService.EnsureDirectoryWithMetadata(ctx, userID, rootPath, sqlitestorage.ConversationBundleDirectoryMetadata(normalized, transcriptPath, conversationPath), models.TrustLevelWork); err != nil {
			return err
		}
		transcript := renderNormalizedConversationMarkdown(normalized)
		metadata := map[string]interface{}{
			"source_platform": platform,
			"capture_mode":    "archive",
			"exactness":       fallbackAgentExactness(convo.Exactness),
			"source_paths":    convo.SourcePaths,
			"session_id":      strings.TrimSpace(convo.SessionID),
			"project_name":    strings.TrimSpace(convo.ProjectName),
		}
		if _, err := s.FileTreeService.WriteEntry(ctx, userID, transcriptPath, transcript, "text/markdown", models.FileTreeWriteOptions{
			Kind:          "file",
			MinTrustLevel: models.TrustLevelWork,
			Metadata: mergeConversationMetadata(metadata, map[string]interface{}{
				"import_kind":  "conversation_archive_transcript",
				"storage_mode": "canonical",
			}),
		}); err != nil {
			return err
		}
		conversationJSON, err := sqlitestorage.MarshalNormalizedConversationDocument(normalized, transcriptPath)
		if err != nil {
			return err
		}
		if _, err := s.FileTreeService.WriteEntry(ctx, userID, conversationPath, string(conversationJSON)+"\n", "application/json", models.FileTreeWriteOptions{
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
			Exactness:        fallbackAgentExactness(convo.Exactness),
		})
	}
	if len(manifest) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	indexPath := hubpath.ConversationIndexPath("claude-code")
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
	return renderNormalizedConversationMarkdown(buildNormalizedConversationFromClaudeConversation(
		convo,
		"claude-code",
		time.Now().UTC().Format(time.RFC3339),
	))
}

func claudeConversationPath(convo sqlitestorage.ClaudeConversation) string {
	return path.Join(claudeConversationArchiveRoot(convo), "conversation.md")
}

func claudeConversationArchiveRoot(convo sqlitestorage.ClaudeConversation) string {
	return strings.TrimSuffix(hubpath.ConversationDir("claude-code", claudeConversationArchiveKey(convo)), "/")
}

func claudeConversationArchiveKey(convo sqlitestorage.ClaudeConversation) string {
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

func buildNormalizedConversationFromClaudeConversation(convo sqlitestorage.ClaudeConversation, platform, importedAt string) sqlitestorage.NormalizedConversation {
	title := strings.TrimSpace(convo.Name)
	if title == "" {
		title = "Claude Code conversation"
	}
	turns := make([]sqlitestorage.NormalizedTurn, 0, len(convo.Messages))
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
		turns = append(turns, sqlitestorage.NormalizedTurn{
			ID:                    fmt.Sprintf("turn_%04d", index+1),
			Role:                  normalizeConversationRole(message.Role),
			At:                    timestamp,
			SourceMessageID:       strings.TrimSpace(message.ID),
			ParentSourceMessageID: strings.TrimSpace(message.ParentID),
			SourceMessageKind:     strings.TrimSpace(message.Kind),
			Parts:                 parts,
		})
	}
	return sqlitestorage.NormalizedConversation{
		Version:              "neudrive.conversation/v1",
		SourcePlatform:       platform,
		SourceConversationID: strings.TrimSpace(convo.SessionID),
		Title:                title,
		ImportedAt:           strings.TrimSpace(importedAt),
		ImportStrategy:       "claude-code-local-scan",
		CreatedAt:            strings.TrimSpace(convo.StartedAt),
		UpdatedAt:            updatedAt,
		ProjectName:          strings.TrimSpace(convo.ProjectName),
		Exactness:            fallbackAgentExactness(convo.Exactness),
		SourcePaths:          append([]string{}, convo.SourcePaths...),
		Provenance: map[string]interface{}{
			"summary":       strings.TrimSpace(convo.Summary),
			"message_count": len(turns),
			"source_format": "claude-code-jsonl",
		},
		Turns:     turns,
		TurnCount: len(turns),
	}
}

func normalizedPartsFromClaudeConversationMessage(message sqlitestorage.ClaudeConversationMessage) []sqlitestorage.NormalizedPart {
	parts := make([]sqlitestorage.NormalizedPart, 0, len(message.Parts))
	for _, part := range message.Parts {
		if normalized := normalizeStoredConversationPart(part); normalized != nil {
			parts = append(parts, *normalized)
		}
	}
	if len(parts) > 0 {
		return parts
	}
	content := strings.TrimSpace(message.Content)
	if content == "" {
		return nil
	}
	return []sqlitestorage.NormalizedPart{{
		Type: "text",
		Text: content,
	}}
}

func normalizeStoredConversationPart(part sqlitestorage.NormalizedPart) *sqlitestorage.NormalizedPart {
	part.Type = strings.TrimSpace(part.Type)
	part.Text = strings.TrimSpace(part.Text)
	part.Name = strings.TrimSpace(part.Name)
	part.ArgsText = strings.TrimSpace(part.ArgsText)
	part.FileName = strings.TrimSpace(part.FileName)
	part.MimeType = strings.TrimSpace(part.MimeType)

	switch part.Type {
	case "", "text", "thinking":
		if part.Text == "" {
			return nil
		}
		if part.Type == "" {
			part.Type = "text"
		}
		return &part
	case "tool_call":
		if part.Name == "" && part.ArgsText == "" {
			return nil
		}
		return &part
	case "tool_result":
		if part.Text == "" {
			return nil
		}
		return &part
	case "attachment":
		if part.FileName == "" && part.MimeType == "" {
			return nil
		}
		return &part
	default:
		if part.Text == "" {
			return nil
		}
		return &part
	}
}

func renderNormalizedConversationMarkdown(convo sqlitestorage.NormalizedConversation) string {
	title := strings.TrimSpace(convo.Title)
	if title == "" {
		title = "Conversation"
	}
	lines := []string{
		"---",
		fmt.Sprintf("title: \"%s\"", escapeConversationYAML(title)),
		fmt.Sprintf("source_platform: \"%s\"", escapeConversationYAML(convo.SourcePlatform)),
		fmt.Sprintf("imported_at: \"%s\"", escapeConversationYAML(convo.ImportedAt)),
		fmt.Sprintf("import_strategy: \"%s\"", escapeConversationYAML(convo.ImportStrategy)),
		fmt.Sprintf("turn_count: %d", len(convo.Turns)),
	}
	if strings.TrimSpace(convo.SourceConversationID) != "" {
		lines = append(lines, fmt.Sprintf("source_conversation_id: \"%s\"", escapeConversationYAML(convo.SourceConversationID)))
	}
	if strings.TrimSpace(convo.ProjectName) != "" {
		lines = append(lines, fmt.Sprintf("project_name: \"%s\"", escapeConversationYAML(convo.ProjectName)))
	}
	if strings.TrimSpace(convo.Exactness) != "" {
		lines = append(lines, fmt.Sprintf("exactness: \"%s\"", escapeConversationYAML(convo.Exactness)))
	}
	lines = append(lines, "---", "", "# "+title, "")
	lines = append(lines, "This is the primary readable archive for the conversation. `conversation.json` keeps the canonical normalized sidecar.")
	lines = append(lines, "")
	for index, turn := range convo.Turns {
		lines = append(lines, fmt.Sprintf("## %s %d", strings.Title(normalizeConversationRole(turn.Role)), index+1), "")
		if strings.TrimSpace(turn.At) != "" {
			lines = append(lines, fmt.Sprintf("_at: %s_", strings.TrimSpace(turn.At)), "")
		}
		if strings.TrimSpace(turn.SourceMessageKind) != "" {
			lines = append(lines, fmt.Sprintf("_kind: %s_", strings.TrimSpace(turn.SourceMessageKind)), "")
		}
		lines = append(lines, renderNormalizedTurnText(turn), "")
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

func renderNormalizedTurnText(turn sqlitestorage.NormalizedTurn) string {
	rendered := make([]string, 0, len(turn.Parts))
	for _, part := range turn.Parts {
		text := renderNormalizedPartText(part)
		if strings.TrimSpace(text) != "" {
			rendered = append(rendered, text)
		}
	}
	return strings.TrimSpace(strings.Join(rendered, "\n\n"))
}

func renderNormalizedPartText(part sqlitestorage.NormalizedPart) string {
	switch strings.TrimSpace(part.Type) {
	case "", "text":
		return strings.TrimSpace(part.Text)
	case "thinking":
		return "> Thinking (condensed)\n>\n> " + strings.ReplaceAll(strings.TrimSpace(part.Text), "\n", "\n> ")
	case "tool_call":
		lines := []string{fmt.Sprintf("### Tool Call: `%s`", strings.TrimSpace(part.Name))}
		if strings.TrimSpace(part.ArgsText) != "" {
			lines = append(lines, "", "```json", strings.TrimSpace(part.ArgsText), "```")
		}
		return strings.Join(lines, "\n")
	case "tool_result":
		if strings.TrimSpace(part.Text) == "" {
			return "### Tool Result"
		}
		return strings.Join([]string{"### Tool Result", "", "```text", strings.TrimSpace(part.Text), "```"}, "\n")
	case "attachment":
		lines := []string{"Attachment"}
		if strings.TrimSpace(part.FileName) != "" {
			lines = append(lines, "name: "+strings.TrimSpace(part.FileName))
		}
		if strings.TrimSpace(part.MimeType) != "" {
			lines = append(lines, "mime: "+strings.TrimSpace(part.MimeType))
		}
		return strings.Join(lines, "\n")
	default:
		return strings.TrimSpace(part.Text)
	}
}

func normalizeConversationRole(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "human", "user":
		return "user"
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	case "tool":
		return "tool"
	default:
		return "assistant"
	}
}

func escapeConversationYAML(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", " ")
	return replacer.Replace(strings.TrimSpace(value))
}

func mergeConversationMetadata(base, extra map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
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
