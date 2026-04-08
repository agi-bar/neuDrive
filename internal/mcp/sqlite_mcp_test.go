package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	sqlitestorage "github.com/agi-bar/agenthub/internal/storage/sqlite"
)

func TestServerCoreToolsUseUnifiedServices(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestorage.Open(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

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
		FileTree:   fileTree,
		Memory:     memory,
		Project:    project,
		Import:     importSvc,
		Token:      tokenSvc,
		UserID:     user.ID,
		TrustLevel: models.TrustLevelFull,
		Scopes:     []string{models.ScopeAdmin},
		BaseURL:    "http://127.0.0.1:42690",
	}

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

	profiles, err := memory.GetProfile(ctx, user.ID)
	if err != nil || len(profiles) != 1 {
		t.Fatalf("GetProfile = %#v, %v", profiles, err)
	}
	projects, err := project.List(ctx, user.ID)
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
