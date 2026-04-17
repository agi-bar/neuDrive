package platforms

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseClaudeConversationFilePreservesStructuredParts(t *testing.T) {
	t.Helper()

	content := strings.Join([]string{
		`{"uuid":"msg-1","type":"user","timestamp":"2026-04-16T20:00:01Z","message":{"role":"user","content":"Please inspect the repo"}}`,
		`{"uuid":"msg-2","parent_uuid":"msg-1","type":"assistant","timestamp":"2026-04-16T20:00:02Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Need to look around first"},{"type":"text","text":"I will inspect the repo."},{"type":"tool_use","name":"bash","input":{"command":"ls -la","cwd":"/tmp/demo"}}]}}`,
		`{"uuid":"msg-3","parent_uuid":"msg-2","type":"assistant","timestamp":"2026-04-16T20:00:03Z","message":{"role":"assistant","content":[{"type":"tool_result","content":[{"type":"text","text":"total 8"}]},{"type":"text","text":"Repo inspected."},{"type":"image","file_name":"diagram.png","mime_type":"image/png"}]}}`,
	}, "\n") + "\n"

	path := filepath.Join(t.TempDir(), "demo-session.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	convo, ok, err := parseClaudeConversationFile(path)
	if err != nil {
		t.Fatalf("parseClaudeConversationFile: %v", err)
	}
	if !ok {
		t.Fatal("parseClaudeConversationFile returned ok=false")
	}
	if convo.Name != "Please inspect the repo" {
		t.Fatalf("Name = %q, want %q", convo.Name, "Please inspect the repo")
	}
	if convo.Summary != "I will inspect the repo." {
		t.Fatalf("Summary = %q, want %q", convo.Summary, "I will inspect the repo.")
	}
	if len(convo.Messages) != 3 {
		t.Fatalf("len(Messages) = %d, want 3", len(convo.Messages))
	}

	second := convo.Messages[1]
	if second.ID != "msg-2" || second.ParentID != "msg-1" {
		t.Fatalf("unexpected ids for second message: %+v", second)
	}
	if len(second.Parts) != 3 {
		t.Fatalf("len(second.Parts) = %d, want 3", len(second.Parts))
	}
	if second.Parts[0].Type != "thinking" || second.Parts[0].Text != "Need to look around first" {
		t.Fatalf("unexpected thinking part: %+v", second.Parts[0])
	}
	if second.Parts[1].Type != "text" || second.Parts[1].Text != "I will inspect the repo." {
		t.Fatalf("unexpected text part: %+v", second.Parts[1])
	}
	if second.Parts[2].Type != "tool_call" || second.Parts[2].Name != "bash" || !strings.Contains(second.Parts[2].ArgsText, `"command": "ls -la"`) {
		t.Fatalf("unexpected tool_call part: %+v", second.Parts[2])
	}
	if !strings.Contains(second.Content, "[tool_call]") {
		t.Fatalf("second.Content missing tool_call marker: %q", second.Content)
	}

	third := convo.Messages[2]
	if len(third.Parts) != 3 {
		t.Fatalf("len(third.Parts) = %d, want 3", len(third.Parts))
	}
	if third.Parts[0].Type != "tool_result" || third.Parts[0].Text != "total 8" {
		t.Fatalf("unexpected tool_result part: %+v", third.Parts[0])
	}
	if third.Parts[2].Type != "attachment" || third.Parts[2].FileName != "diagram.png" || third.Parts[2].MimeType != "image/png" {
		t.Fatalf("unexpected attachment part: %+v", third.Parts[2])
	}
}
