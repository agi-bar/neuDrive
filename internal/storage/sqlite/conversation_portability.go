package sqlite

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
)

const (
	ConversationExportTargetClaude  = "claude"
	ConversationExportTargetChatGPT = "chatgpt"
)

func BuildConversationExportPaths(rootPath string) map[string]string {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return nil
	}
	return map[string]string{
		ConversationExportTargetClaude:  path.Join(rootPath, "resume-claude.md"),
		ConversationExportTargetChatGPT: path.Join(rootPath, "resume-chatgpt.md"),
	}
}

func MarshalNormalizedConversationDocument(convo NormalizedConversation, transcriptPath string, exportPaths map[string]string) ([]byte, error) {
	document := map[string]interface{}{
		"version":                convo.Version,
		"source_platform":        convo.SourcePlatform,
		"source_url":             convo.SourceURL,
		"source_conversation_id": convo.SourceConversationID,
		"title":                  convo.Title,
		"imported_at":            convo.ImportedAt,
		"import_strategy":        convo.ImportStrategy,
		"model":                  convo.Model,
		"created_at":             convo.CreatedAt,
		"updated_at":             convo.UpdatedAt,
		"project_name":           convo.ProjectName,
		"exactness":              convo.Exactness,
		"source_paths":           convo.SourcePaths,
		"provenance":             convo.Provenance,
		"turns":                  convo.Turns,
		"turn_count":             convo.TurnCount,
		"transcript_path":        transcriptPath,
	}
	if len(exportPaths) > 0 {
		document["exports"] = exportPaths
	}
	return json.MarshalIndent(document, "", "  ")
}

func ConversationBundleDirectoryMetadata(convo NormalizedConversation, transcriptPath, conversationPath string, exportPaths map[string]string) map[string]interface{} {
	description := strings.TrimSpace(conversationBundleDescription(convo))
	metadata := services.BundleMetadata(models.BundleSummary{
		Kind:         services.BundleKindConversation,
		Name:         strings.TrimSpace(convo.Title),
		Source:       strings.TrimSpace(convo.SourcePlatform),
		Description:  description,
		Status:       "archived",
		PrimaryPath:  transcriptPath,
		Capabilities: []string{"transcript", "normalized", "exports"},
	})
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if transcriptPath != "" {
		metadata["conversation_transcript_path"] = transcriptPath
	}
	if conversationPath != "" {
		metadata["conversation_path"] = conversationPath
	}
	if len(exportPaths) > 0 {
		metadata["conversation_exports"] = exportPaths
	}
	if strings.TrimSpace(convo.SourcePlatform) != "" {
		metadata["source_platform"] = strings.TrimSpace(convo.SourcePlatform)
	}
	if strings.TrimSpace(convo.SourceConversationID) != "" {
		metadata["source_conversation_id"] = strings.TrimSpace(convo.SourceConversationID)
	}
	if strings.TrimSpace(convo.ImportStrategy) != "" {
		metadata["import_strategy"] = strings.TrimSpace(convo.ImportStrategy)
	}
	if convo.TurnCount > 0 {
		metadata["turn_count"] = convo.TurnCount
	}
	if strings.TrimSpace(convo.ImportedAt) != "" {
		metadata["imported_at"] = strings.TrimSpace(convo.ImportedAt)
	}
	return metadata
}

func RenderConversationContinuationMarkdown(convo NormalizedConversation, target string) string {
	target = normalizeConversationExportTarget(target)
	title := strings.TrimSpace(convo.Title)
	if title == "" {
		title = "Conversation"
	}
	targetLabel := conversationExportTargetLabel(target)
	lines := []string{
		"---",
		fmt.Sprintf("title: \"%s\"", escapeConversationYAML(fmt.Sprintf("Resume %s in %s", title, targetLabel))),
		fmt.Sprintf("target_platform: \"%s\"", escapeConversationYAML(target)),
		fmt.Sprintf("source_platform: \"%s\"", escapeConversationYAML(convo.SourcePlatform)),
		fmt.Sprintf("generated_from: \"%s\"", escapeConversationYAML(convo.Version)),
		fmt.Sprintf("turn_count: %d", len(convo.Turns)),
	}
	if strings.TrimSpace(convo.SourceConversationID) != "" {
		lines = append(lines, fmt.Sprintf("source_conversation_id: \"%s\"", escapeConversationYAML(convo.SourceConversationID)))
	}
	if strings.TrimSpace(convo.ProjectName) != "" {
		lines = append(lines, fmt.Sprintf("project_name: \"%s\"", escapeConversationYAML(convo.ProjectName)))
	}
	lines = append(lines, "---", "")
	lines = append(lines, fmt.Sprintf("# Resume %s in %s", title, targetLabel), "")
	lines = append(lines, conversationExportIntro(targetLabel), "")
	lines = append(lines, "## Resume Prompt", "")
	lines = append(lines, fmt.Sprintf("Continue the conversation below. It was imported into neuDrive from `%s`.", fallbackConversationSourcePlatform(convo.SourcePlatform)))
	lines = append(lines, "- Preserve speaker order and chronology.")
	lines = append(lines, "- Treat tool calls and tool results as historical context unless the user explicitly asks to rerun them.")
	lines = append(lines, "- Do not restate the full transcript unless asked.")
	lines = append(lines, "- Continue naturally from the latest turn or respond to the next user message.", "")
	lines = append(lines, "## Transcript", "", "```text", renderConversationPortableTranscript(convo), "```")
	return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
}

func normalizeConversationExportTarget(target string) string {
	switch strings.TrimSpace(strings.ToLower(target)) {
	case ConversationExportTargetChatGPT:
		return ConversationExportTargetChatGPT
	default:
		return ConversationExportTargetClaude
	}
}

func conversationExportTargetLabel(target string) string {
	if normalizeConversationExportTarget(target) == ConversationExportTargetChatGPT {
		return "ChatGPT"
	}
	return "Claude"
}

func conversationExportIntro(targetLabel string) string {
	return fmt.Sprintf("Paste the prompt below into a new %s conversation or project chat when you want to continue from this archived context.", targetLabel)
}

func conversationBundleDescription(convo NormalizedConversation) string {
	source := fallbackConversationSourcePlatform(convo.SourcePlatform)
	if convo.TurnCount > 0 {
		return fmt.Sprintf("Imported from %s with %d turns.", source, convo.TurnCount)
	}
	return fmt.Sprintf("Imported from %s.", source)
}

func fallbackConversationSourcePlatform(platform string) string {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return "unknown-platform"
	}
	return platform
}

func renderConversationPortableTranscript(convo NormalizedConversation) string {
	blocks := make([]string, 0, len(convo.Turns))
	for _, turn := range convo.Turns {
		headerParts := []string{strings.Title(normalizeConversationRole(turn.Role))}
		if at := strings.TrimSpace(turn.At); at != "" {
			headerParts = append(headerParts, at)
		}
		if kind := strings.TrimSpace(turn.SourceMessageKind); kind != "" {
			headerParts = append(headerParts, kind)
		}
		bodyParts := make([]string, 0, len(turn.Parts))
		for _, part := range turn.Parts {
			if rendered := renderConversationPortablePart(part); strings.TrimSpace(rendered) != "" {
				bodyParts = append(bodyParts, rendered)
			}
		}
		if len(bodyParts) == 0 {
			continue
		}
		blocks = append(blocks, fmt.Sprintf("[%s]\n%s", strings.Join(headerParts, " | "), strings.Join(bodyParts, "\n\n")))
	}
	if len(blocks) == 0 {
		return "[Conversation]\n(no turns captured)"
	}
	return strings.Join(blocks, "\n\n")
}

func renderConversationPortablePart(part NormalizedPart) string {
	switch strings.TrimSpace(part.Type) {
	case "", "text":
		return strings.TrimSpace(part.Text)
	case "thinking":
		if strings.TrimSpace(part.Text) == "" {
			return ""
		}
		return fmt.Sprintf("Thinking:\n%s", strings.TrimSpace(part.Text))
	case "tool_call":
		lines := []string{fmt.Sprintf("Tool call: %s", fallbackConversationValue(strings.TrimSpace(part.Name), "tool"))}
		if args := strings.TrimSpace(part.ArgsText); args != "" {
			lines = append(lines, "Args:", args)
		}
		return strings.Join(lines, "\n")
	case "tool_result":
		if strings.TrimSpace(part.Text) == "" {
			return "Tool result captured."
		}
		return fmt.Sprintf("Tool result:\n%s", strings.TrimSpace(part.Text))
	case "attachment":
		meta := []string{}
		if name := strings.TrimSpace(part.FileName); name != "" {
			meta = append(meta, "name="+name)
		}
		if mime := strings.TrimSpace(part.MimeType); mime != "" {
			meta = append(meta, "mime="+mime)
		}
		if len(meta) == 0 {
			return "Attachment captured."
		}
		return "Attachment: " + strings.Join(meta, ", ")
	default:
		if strings.TrimSpace(part.Text) == "" {
			return ""
		}
		return fmt.Sprintf("%s:\n%s", strings.Title(strings.TrimSpace(part.Type)), strings.TrimSpace(part.Text))
	}
}

func fallbackConversationValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
