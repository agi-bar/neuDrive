package sqlite

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
)

func MarshalNormalizedConversationDocument(convo NormalizedConversation, transcriptPath string) ([]byte, error) {
	messageCount := conversationMessageCount(convo)
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
		"message_count":          messageCount,
		"transcript_path":        transcriptPath,
	}
	return json.MarshalIndent(document, "", "  ")
}

func ConversationBundleDirectoryMetadata(convo NormalizedConversation, transcriptPath, conversationPath string) map[string]interface{} {
	description := strings.TrimSpace(conversationBundleDescription(convo))
	title := strings.TrimSpace(convo.Title)
	startedAt, endedAt := conversationTimeline(convo)
	messageCount := conversationMessageCount(convo)
	metadata := services.BundleMetadata(models.BundleSummary{
		Kind:         services.BundleKindConversation,
		Name:         title,
		Source:       strings.TrimSpace(convo.SourcePlatform),
		Description:  description,
		Status:       "archived",
		PrimaryPath:  transcriptPath,
		Capabilities: []string{"transcript", "normalized"},
	})
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if title != "" {
		metadata["conversation_title"] = title
	}
	if transcriptPath != "" {
		metadata["conversation_transcript_path"] = transcriptPath
	}
	if conversationPath != "" {
		metadata["conversation_path"] = conversationPath
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
	if messageCount > 0 {
		metadata["turn_count"] = messageCount
		metadata["message_count"] = messageCount
		metadata["conversation_message_count"] = messageCount
	}
	if strings.TrimSpace(convo.ImportedAt) != "" {
		metadata["imported_at"] = strings.TrimSpace(convo.ImportedAt)
	}
	if startedAt != "" {
		metadata["conversation_started_at"] = startedAt
	}
	if endedAt != "" {
		metadata["conversation_ended_at"] = endedAt
	}
	if strings.TrimSpace(convo.Model) != "" {
		metadata["conversation_model"] = strings.TrimSpace(convo.Model)
	}
	if strings.TrimSpace(convo.ProjectName) != "" {
		metadata["conversation_project_name"] = strings.TrimSpace(convo.ProjectName)
	}
	if strings.TrimSpace(convo.SourceURL) != "" {
		metadata["conversation_source_url"] = strings.TrimSpace(convo.SourceURL)
	}
	return metadata
}

func conversationBundleDescription(convo NormalizedConversation) string {
	source := fallbackConversationSourcePlatform(convo.SourcePlatform)
	messageCount := conversationMessageCount(convo)
	startedAt, endedAt := conversationTimeline(convo)
	if messageCount > 0 && startedAt != "" && endedAt != "" {
		return fmt.Sprintf("Imported from %s with %d messages from %s to %s.", source, messageCount, startedAt, endedAt)
	}
	if messageCount > 0 {
		return fmt.Sprintf("Imported from %s with %d messages.", source, messageCount)
	}
	return fmt.Sprintf("Imported from %s.", source)
}

func conversationMessageCount(convo NormalizedConversation) int {
	if convo.TurnCount > 0 {
		return convo.TurnCount
	}
	return len(convo.Turns)
}

func conversationTimeline(convo NormalizedConversation) (string, string) {
	startedAt := strings.TrimSpace(convo.CreatedAt)
	if startedAt == "" {
		for _, turn := range convo.Turns {
			if at := strings.TrimSpace(turn.At); at != "" {
				startedAt = at
				break
			}
		}
	}
	endedAt := strings.TrimSpace(convo.UpdatedAt)
	if endedAt == "" {
		for index := len(convo.Turns) - 1; index >= 0; index-- {
			if at := strings.TrimSpace(convo.Turns[index].At); at != "" {
				endedAt = at
				break
			}
		}
	}
	return startedAt, endedAt
}

func fallbackConversationSourcePlatform(platform string) string {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return "unknown-platform"
	}
	return platform
}
