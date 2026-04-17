package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agi-bar/neudrive/internal/models"
)

func TestImportAgentExportClaudeConversationWritesCanonicalArchive(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	client := &Client{store: store, userID: user.ID}

	payload := AgentExportPayload{
		Claude: &ClaudeInventory{
			Conversations: []ClaudeConversation{{
				Name:        "Demo Chat",
				SessionID:   "sess-123",
				ProjectName: "demo-project",
				Summary:     "Imported from Claude Code scan",
				StartedAt:   "2026-04-16T20:00:00Z",
				Exactness:   "exact",
				SourcePaths: []string{"/tmp/demo-chat.jsonl"},
				Messages: []ClaudeConversationMessage{
					{
						ID:        "msg-1",
						Role:      "user",
						Content:   "Hello from Claude Code",
						Timestamp: "2026-04-16T20:00:01Z",
						Kind:      "message",
					},
					{
						ID:        "msg-2",
						ParentID:  "msg-1",
						Role:      "assistant",
						Content:   "Hi there",
						Timestamp: "2026-04-16T20:00:02Z",
						Kind:      "message",
					},
				},
			}},
		},
	}

	result, err := client.ImportAgentExport(ctx, "claude-code", payload)
	if err != nil {
		t.Fatalf("ImportAgentExport: %v", err)
	}
	if result.Conversations != 1 {
		t.Fatalf("Conversations = %d, want 1", result.Conversations)
	}

	rootPath := "/conversations/claude-code/2026-04-16-demo-chat-sess-123-compact"
	transcriptPath := rootPath + "/conversation.md"
	conversationPath := rootPath + "/conversation.json"
	indexPath := "/conversations/claude-code/index.json"

	root, err := store.Read(ctx, user.ID, rootPath, models.TrustLevelWork)
	if err != nil {
		t.Fatalf("Read conversation root: %v", err)
	}
	for key, want := range map[string]interface{}{
		"conversation_title":           "Demo Chat",
		"source_platform":              "claude-code",
		"source_conversation_id":       "sess-123",
		"conversation_started_at":      "2026-04-16T20:00:00Z",
		"conversation_ended_at":        "2026-04-16T20:00:02Z",
		"conversation_project_name":    "demo-project",
		"conversation_message_count":   float64(2),
		"message_count":                float64(2),
		"turn_count":                   float64(2),
		"bundle_primary_path":          transcriptPath,
		"conversation_transcript_path": transcriptPath,
		"conversation_path":            conversationPath,
	} {
		if got := root.Metadata[key]; got != want {
			t.Fatalf("root metadata[%s] = %#v, want %#v", key, got, want)
		}
	}

	transcript, err := store.Read(ctx, user.ID, transcriptPath, models.TrustLevelWork)
	if err != nil {
		t.Fatalf("Read transcript: %v", err)
	}
	if !strings.Contains(transcript.Content, "# Demo Chat") {
		t.Fatalf("transcript missing title: %q", transcript.Content)
	}
	if !strings.Contains(transcript.Content, "## User 1") || !strings.Contains(transcript.Content, "## Assistant 2") {
		t.Fatalf("transcript missing rendered turns: %q", transcript.Content)
	}

	conversation, err := store.Read(ctx, user.ID, conversationPath, models.TrustLevelWork)
	if err != nil {
		t.Fatalf("Read conversation sidecar: %v", err)
	}
	for _, want := range []string{
		`"version": "neudrive.conversation/v1"`,
		`"import_strategy": "claude-code-local-scan"`,
		`"source_conversation_id": "sess-123"`,
		`"transcript_path": "` + transcriptPath + `"`,
		`"message_count": 2`,
	} {
		if !strings.Contains(conversation.Content, want) {
			t.Fatalf("conversation sidecar missing %s: %q", want, conversation.Content)
		}
	}

	index, err := store.Read(ctx, user.ID, indexPath, models.TrustLevelWork)
	if err != nil {
		t.Fatalf("Read index: %v", err)
	}
	for _, want := range []string{
		`"root_path": "` + rootPath + `"`,
		`"transcript_path": "` + transcriptPath + `"`,
		`"conversation_path": "` + conversationPath + `"`,
	} {
		if !strings.Contains(index.Content, want) {
			t.Fatalf("index missing %s: %q", want, index.Content)
		}
	}
}

func TestImportAgentExportClaudeConversationPreservesStructuredParts(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	client := &Client{store: store, userID: user.ID}

	payload := AgentExportPayload{
		Claude: &ClaudeInventory{
			Conversations: []ClaudeConversation{{
				Name:      "Structured Demo",
				SessionID: "structured-123",
				StartedAt: "2026-04-16T20:00:00Z",
				Messages: []ClaudeConversationMessage{
					{
						ID:        "msg-1",
						Role:      "assistant",
						Content:   "I inspected the repo.",
						Timestamp: "2026-04-16T20:00:01Z",
						Kind:      "assistant",
						Parts: []NormalizedPart{
							{Type: "thinking", Text: "Need to inspect files first"},
							{Type: "text", Text: "I inspected the repo."},
							{Type: "tool_call", Name: "bash", ArgsText: "{\n  \"command\": \"ls -la\"\n}"},
							{Type: "tool_result", Text: "total 8"},
						},
					},
				},
			}},
		},
	}

	if _, err := client.ImportAgentExport(ctx, "claude-code", payload); err != nil {
		t.Fatalf("ImportAgentExport: %v", err)
	}

	rootPath := "/conversations/claude-code/2026-04-16-structured-demo-structured-123-compact"
	transcriptPath := rootPath + "/conversation.md"
	conversationPath := rootPath + "/conversation.json"

	transcript, err := store.Read(ctx, user.ID, transcriptPath, models.TrustLevelWork)
	if err != nil {
		t.Fatalf("Read transcript: %v", err)
	}
	for _, want := range []string{
		"Thinking (condensed)",
		"### Tool Call: `bash`",
		"### Tool Result",
	} {
		if !strings.Contains(transcript.Content, want) {
			t.Fatalf("transcript missing %s: %q", want, transcript.Content)
		}
	}

	conversation, err := store.Read(ctx, user.ID, conversationPath, models.TrustLevelWork)
	if err != nil {
		t.Fatalf("Read conversation sidecar: %v", err)
	}
	for _, want := range []string{
		`"type": "thinking"`,
		`"type": "tool_call"`,
		`"name": "bash"`,
		`"type": "tool_result"`,
	} {
		if !strings.Contains(conversation.Content, want) {
			t.Fatalf("conversation sidecar missing %s: %q", want, conversation.Content)
		}
	}
}
