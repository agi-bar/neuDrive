package mcp

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agi-bar/neudrive/internal/localgitsync"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
	sqlitestorage "github.com/agi-bar/neudrive/internal/storage/sqlite"
)

func setupSQLiteMCPServer(t *testing.T) (context.Context, *sqlitestorage.Store, *models.User, *MCPServer) {
	t.Helper()

	ctx := context.Background()
	store, err := sqlitestorage.Open(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}

	fileTree := services.NewFileTreeServiceWithRepo(sqlitestorage.NewFileTreeRepo(store))
	memory := services.NewMemoryServiceWithRepo(sqlitestorage.NewMemoryRepo(store), nil)
	project := services.NewProjectServiceWithRepo(sqlitestorage.NewProjectRepo(store), nil, nil)
	tokenSvc := services.NewTokenServiceWithRepo(sqlitestorage.NewTokenRepo(store))
	importSvc := services.NewImportService(nil, fileTree, memory, nil)
	s := &MCPServer{
		FileTree:     fileTree,
		Memory:       memory,
		Project:      project,
		Import:       importSvc,
		Token:        tokenSvc,
		LocalGitSync: localgitsync.New(store, nil),
		UserID:       user.ID,
		TrustLevel:   models.TrustLevelFull,
		Scopes:       []string{models.ScopeAdmin},
		BaseURL:      "http://127.0.0.1:42690",
	}
	return ctx, store, user, s
}

func buildSkillArchive(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("Write(%s): %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close zip writer: %v", err)
	}
	return buf.Bytes()
}

func TestServerCoreToolsUseUnifiedServices(t *testing.T) {
	ctx, store, user, s := setupSQLiteMCPServer(t)

	resp := s.HandleJSONRPC(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  mustMarshalParams(t, ToolCallParams{Name: "update_profile", Arguments: map[string]interface{}{"category": "preferences", "content": "Keep it concise."}}),
	})
	if resp.Error != nil {
		t.Fatalf("update_profile error: %+v", resp.Error)
	}

	resp = s.HandleJSONRPC(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params:  mustMarshalParams(t, ToolCallParams{Name: "create_project", Arguments: map[string]interface{}{"name": "repo-test"}}),
	})
	if resp.Error != nil {
		t.Fatalf("create_project error: %+v", resp.Error)
	}

	resp = s.HandleJSONRPC(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params:  mustMarshalParams(t, ToolCallParams{Name: "save_memory", Arguments: map[string]interface{}{"memories": []map[string]interface{}{{"content": "remember this", "title": "note"}}}}),
	})
	if resp.Error != nil {
		t.Fatalf("save_memory error: %+v", resp.Error)
	}
	out := extractToolText(t, resp)
	if !strings.Contains(out, "saved 1 memories") {
		t.Fatalf("unexpected save_memory result: %s", out)
	}

	resp = s.HandleJSONRPC(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params:  mustMarshalParams(t, ToolCallParams{Name: "create_sync_token", Arguments: map[string]interface{}{"purpose": "backup", "access": "push", "ttl_minutes": 30}}),
	})
	if resp.Error != nil {
		t.Fatalf("create_sync_token error: %+v", resp.Error)
	}
	if !strings.Contains(extractToolText(t, resp), `"api_base": "http://127.0.0.1:42690"`) {
		t.Fatalf("unexpected create_sync_token payload: %s", extractToolText(t, resp))
	}

	profiles, err := s.Memory.GetProfile(ctx, user.ID)
	if err != nil || len(profiles) != 1 {
		t.Fatalf("GetProfile = %#v, %v", profiles, err)
	}
	projects, err := s.Project.List(ctx, user.ID)
	if err != nil || len(projects) != 1 {
		t.Fatalf("List projects = %#v, %v", projects, err)
	}
	tokens, err := store.ValidateToken(ctx, strings.TrimSpace(extractTokenFromJSON(extractToolText(t, resp))))
	if err != nil || tokens == nil {
		t.Fatalf("Validate created sync token: %v", err)
	}

	resp = s.HandleJSONRPC(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "tools/call",
		Params:  mustMarshalParams(t, ToolCallParams{Name: "write_file", Arguments: map[string]interface{}{"path": "/skills/demo/SKILL.md", "content": "# Demo\nhello"}}),
	})
	if resp.Error != nil {
		t.Fatalf("write_file error: %+v", resp.Error)
	}

	resp = s.HandleJSONRPC(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      6,
		Method:  "tools/call",
		Params:  mustMarshalParams(t, ToolCallParams{Name: "read_skill", Arguments: map[string]interface{}{"name": "demo"}}),
	})
	if resp.Error != nil {
		t.Fatalf("read_skill error: %+v", resp.Error)
	}
	if !strings.Contains(extractToolText(t, resp), "# Demo") {
		t.Fatalf("unexpected read_skill payload: %s", extractToolText(t, resp))
	}
}

func TestServerCoreToolsImportSkillPreservesFiles(t *testing.T) {
	ctx, store, user, s := setupSQLiteMCPServer(t)

	resp := s.HandleJSONRPC(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: mustMarshalParams(t, ToolCallParams{
			Name: "import_skill",
			Arguments: map[string]interface{}{
				"name": "single-skill",
				"files": map[string]interface{}{
					"SKILL.md":         "# Single Skill\n",
					"helper.py":        "print('hello')\n",
					"prompts/guide.md": "Use the helper.\n",
				},
			},
		}),
	})
	if resp.Error != nil {
		t.Fatalf("import_skill error: %+v", resp.Error)
	}
	if !strings.Contains(extractToolText(t, resp), `imported 3 files for skill "single-skill"`) {
		t.Fatalf("unexpected import_skill result: %s", extractToolText(t, resp))
	}

	entry, err := store.Read(ctx, user.ID, "/skills/single-skill/helper.py", models.TrustLevelFull)
	if err != nil {
		t.Fatalf("Read helper.py: %v", err)
	}
	if !strings.Contains(entry.Content, "print('hello')") {
		t.Fatalf("unexpected helper.py content: %q", entry.Content)
	}
	guide, err := store.Read(ctx, user.ID, "/skills/single-skill/prompts/guide.md", models.TrustLevelFull)
	if err != nil {
		t.Fatalf("Read prompts/guide.md: %v", err)
	}
	if !strings.Contains(guide.Content, "Use the helper.") {
		t.Fatalf("unexpected guide content: %q", guide.Content)
	}
}

func TestServerCoreToolsAppendLocalGitSyncMessage(t *testing.T) {
	ctx, _, user, s := setupSQLiteMCPServer(t)

	mirrorDir := filepath.Join(t.TempDir(), "mirror")
	if _, err := s.LocalGitSync.RegisterMirrorAndSync(ctx, user.ID, mirrorDir); err != nil {
		t.Fatalf("RegisterMirrorAndSync: %v", err)
	}

	resp := s.HandleJSONRPC(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  mustMarshalParams(t, ToolCallParams{Name: "write_file", Arguments: map[string]interface{}{"path": "/notes/mirror.md", "content": "hello mirror"}}),
	})
	if resp.Error != nil {
		t.Fatalf("write_file error: %+v", resp.Error)
	}
	out := extractToolText(t, resp)
	if !strings.Contains(out, "file written") || !strings.Contains(out, "已同步到本地 Git 目录: "+mirrorDir) {
		t.Fatalf("unexpected write_file output: %s", out)
	}

	data, err := os.ReadFile(filepath.Join(mirrorDir, "notes", "mirror.md"))
	if err != nil {
		t.Fatalf("read mirrored file: %v", err)
	}
	if string(data) != "hello mirror" {
		t.Fatalf("unexpected mirrored file content: %q", string(data))
	}
}

func TestServerCoreToolsImportSkillsArchivePreservesAssets(t *testing.T) {
	ctx, _, user, s := setupSQLiteMCPServer(t)

	archive := buildSkillArchive(t, map[string][]byte{
		"archive-demo/SKILL.md":        []byte("# Archive Demo\n"),
		"archive-demo/helper.py":       []byte("print('archive')\n"),
		"archive-demo/assets/logo.png": {0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00},
	})
	resp := s.HandleJSONRPC(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: mustMarshalParams(t, ToolCallParams{
			Name: "import_skills_archive",
			Arguments: map[string]interface{}{
				"archive_base64": base64.StdEncoding.EncodeToString(archive),
				"archive_name":   "neudrive-skills.zip",
				"platform":       "claude-web",
			},
		}),
	})
	if resp.Error != nil {
		t.Fatalf("import_skills_archive error: %+v", resp.Error)
	}
	if !strings.Contains(extractToolText(t, resp), `"imported": 3`) {
		t.Fatalf("unexpected import_skills_archive result: %s", extractToolText(t, resp))
	}

	textEntry, err := s.FileTree.Read(ctx, user.ID, "/skills/archive-demo/helper.py", models.TrustLevelFull)
	if err != nil {
		t.Fatalf("Read helper.py: %v", err)
	}
	if !strings.Contains(textEntry.Content, "print('archive')") {
		t.Fatalf("unexpected helper.py content: %q", textEntry.Content)
	}

	binaryData, binaryEntry, err := s.FileTree.ReadBinary(ctx, user.ID, "/skills/archive-demo/assets/logo.png", models.TrustLevelFull)
	if err != nil {
		t.Fatalf("ReadBinary logo.png: %v", err)
	}
	if len(binaryData) == 0 {
		t.Fatal("expected binary asset data")
	}
	if binaryEntry.Metadata["capture_mode"] != "archive" || binaryEntry.Metadata["source_platform"] != "claude-web" {
		t.Fatalf("unexpected binary metadata: %+v", binaryEntry.Metadata)
	}
}

func TestServerCoreToolsImportSkillsArchiveRejectsBadInputs(t *testing.T) {
	_, _, _, s := setupSQLiteMCPServer(t)

	tests := []struct {
		name          string
		archiveBase64 string
		wantSubstring string
	}{
		{
			name:          "invalid base64",
			archiveBase64: "!!!not-base64!!!",
			wantSubstring: "decode archive_base64",
		},
		{
			name:          "bad zip",
			archiveBase64: base64.StdEncoding.EncodeToString([]byte("not a zip")),
			wantSubstring: "open skills archive",
		},
		{
			name: "missing manifest",
			archiveBase64: base64.StdEncoding.EncodeToString(buildSkillArchive(t, map[string][]byte{
				"broken/helper.py": []byte("print('missing skill md')\n"),
			})),
			wantSubstring: "missing broken/SKILL.md",
		},
		{
			name:          "too large",
			archiveBase64: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'a'}, services.MaxSkillsArchiveBytes+1)),
			wantSubstring: "archive exceeds 50 MB limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := s.HandleJSONRPC(JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      1,
				Method:  "tools/call",
				Params: mustMarshalParams(t, ToolCallParams{
					Name: "import_skills_archive",
					Arguments: map[string]interface{}{
						"archive_base64": tt.archiveBase64,
					},
				}),
			})
			if resp.Error != nil {
				t.Fatalf("unexpected JSON-RPC error: %+v", resp.Error)
			}
			text := extractToolText(t, resp)
			if !strings.Contains(text, tt.wantSubstring) {
				t.Fatalf("expected %q in %q", tt.wantSubstring, text)
			}
		})
	}
}

func mustMarshalParams(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return data
}

func extractToolText(t *testing.T, resp JSONRPCResponse) string {
	t.Helper()
	payload, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected result type: %#v", resp.Result)
	}
	content, ok := payload["content"].([]ContentBlock)
	if ok && len(content) > 0 {
		return content[0].Text
	}
	raw, err := json.Marshal(payload["content"])
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		t.Fatalf("unmarshal content blocks: %v", err)
	}
	if len(blocks) == 0 {
		t.Fatalf("empty content blocks")
	}
	return blocks[0].Text
}

func extractTokenFromJSON(text string) string {
	var payload map[string]interface{}
	_ = json.Unmarshal([]byte(text), &payload)
	token, _ := payload["token"].(string)
	return token
}

func TestServerCoreToolsLogActionWithRepoProject(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestorage.Open(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	user, _ := store.EnsureOwner(ctx)

	projectSvc := services.NewProjectServiceWithRepo(sqlitestorage.NewProjectRepo(store), nil, nil)
	if _, err := projectSvc.Create(ctx, user.ID, "logs"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	s := &MCPServer{
		FileTree:   services.NewFileTreeServiceWithRepo(sqlitestorage.NewFileTreeRepo(store)),
		Project:    projectSvc,
		UserID:     user.ID,
		TrustLevel: models.TrustLevelFull,
		Scopes:     []string{models.ScopeAdmin},
		BaseURL:    "http://127.0.0.1:42690",
	}
	resp := s.HandleJSONRPC(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: mustMarshalParams(t, ToolCallParams{
			Name: "log_action",
			Arguments: map[string]interface{}{
				"project": "logs",
				"action":  "test",
				"summary": "repo-backed log",
				"source":  "test",
			},
		}),
	})
	if resp.Error != nil {
		t.Fatalf("log_action response error: %+v", resp.Error)
	}
	if strings.Contains(extractToolText(t, resp), "error:") {
		t.Fatalf("log_action returned tool error: %s", extractToolText(t, resp))
	}
	project, err := projectSvc.Get(ctx, user.ID, "logs")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	logs, err := projectSvc.GetLogs(ctx, project.ID, 10)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) != 1 || logs[0].Summary != "repo-backed log" {
		t.Fatalf("unexpected logs: %#v", logs)
	}
}
