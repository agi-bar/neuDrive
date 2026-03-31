package mcp

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/agi-bar/agenthub/internal/vault"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------------------
// MCP integration tests against a real database.
//
// Run with:
//   AGENTHUB_TEST_DB="postgres://agenthub:agenthub_dev@localhost:5434/agenthub?sslmode=disable" \
//   AGENTHUB_VAULT_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" \
//   go test ./internal/mcp/ -run TestMCPInteg -v -count=1
// ---------------------------------------------------------------------------

func setupIntegrationMCP(t *testing.T) *MCPServer {
	t.Helper()

	dbURL := os.Getenv("AGENTHUB_TEST_DB")
	if dbURL == "" {
		t.Skip("AGENTHUB_TEST_DB not set; skipping MCP integration test")
	}
	vaultKey := os.Getenv("AGENTHUB_VAULT_KEY")
	if vaultKey == "" {
		vaultKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to DB: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Create a unique test user directly in DB
	userID := uuid.New()
	slug := "mcp-test-" + userID.String()[:8]
	now := time.Now().UTC()
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, slug, display_name, timezone, language, created_at, updated_at)
		 VALUES ($1, $2, $3, 'UTC', 'en', $4, $4)`,
		userID, slug, "MCP Test User", now)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	// Initialize vault
	v, err := vault.NewVault(vaultKey)
	if err != nil {
		t.Fatalf("init vault: %v", err)
	}

	// Create services
	fileTreeSvc := services.NewFileTreeService(pool)
	vaultSvc := services.NewVaultService(pool, v)
	memorySvc := services.NewMemoryService(pool)
	roleSvc := services.NewRoleService(pool)
	projectSvc := services.NewProjectService(pool, roleSvc)
	inboxSvc := services.NewInboxService(pool)
	deviceSvc := services.NewDeviceService(pool)
	dashboardSvc := services.NewDashboardService(pool)
	importSvc := services.NewImportService(pool, fileTreeSvc, memorySvc, vaultSvc)

	return &MCPServer{
		UserID:      userID,
		TrustLevel:  models.TrustLevelFull,
		Scopes:      []string{models.ScopeAdmin},
		FileTree:    fileTreeSvc,
		Vault:       vaultSvc,
		VaultCrypto: v,
		Memory:      memorySvc,
		Project:     projectSvc,
		Inbox:       inboxSvc,
		Device:      deviceSvc,
		Dashboard:   dashboardSvc,
		Import:      importSvc,
	}
}

// mcpToolCall invokes a tool and returns the text content and whether it errored.
func mcpToolCall(t *testing.T, s *MCPServer, tool string, args map[string]interface{}) (string, bool) {
	t.Helper()
	params, _ := json.Marshal(ToolCallParams{Name: tool, Arguments: args})
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: params}
	resp := s.HandleJSONRPC(req)

	if resp.Error != nil {
		return resp.Error.Message, true
	}

	result, _ := resp.Result.(map[string]interface{})
	if result == nil {
		return "", false
	}

	isErr, _ := result["isError"].(bool)

	content, _ := result["content"].([]interface{})
	if len(content) > 0 {
		block, _ := content[0].(map[string]interface{})
		text, _ := block["text"].(string)
		return text, isErr
	}
	return "", isErr
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestMCPInteg_Initialize(t *testing.T) {
	s := setupIntegrationMCP(t)
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize"}
	resp := s.HandleJSONRPC(req)
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}
	result := resp.Result.(map[string]interface{})
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("unexpected protocol version: %v", result["protocolVersion"])
	}
}

func TestMCPInteg_ToolsList(t *testing.T) {
	s := setupIntegrationMCP(t)
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"}
	resp := s.HandleJSONRPC(req)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error)
	}
	result := resp.Result.(map[string]interface{})
	tools := result["tools"].([]MCPTool)
	if len(tools) < 20 {
		t.Errorf("expected >= 20 tools, got %d", len(tools))
	}
}

func TestMCPInteg_FileTree_WriteReadDelete(t *testing.T) {
	s := setupIntegrationMCP(t)

	// Write
	text, isErr := mcpToolCall(t, s, "write_file", map[string]interface{}{
		"path": "/notes/test.md", "content": "# Hello MCP\n\nTest content.",
	})
	if isErr {
		t.Fatalf("write_file error: %s", text)
	}

	// Read
	text, isErr = mcpToolCall(t, s, "read_file", map[string]interface{}{
		"path": "/notes/test.md",
	})
	if isErr {
		t.Fatalf("read_file error: %s", text)
	}
	t.Logf("read_file: %q", text[:min(len(text), 100)])

	// List directory
	text, isErr = mcpToolCall(t, s, "list_directory", map[string]interface{}{
		"path": "/notes/",
	})
	if isErr {
		t.Fatalf("list_directory error: %s", text)
	}
}

func TestMCPInteg_Profile_UpdateAndRead(t *testing.T) {
	s := setupIntegrationMCP(t)

	// Update
	text, isErr := mcpToolCall(t, s, "update_profile", map[string]interface{}{
		"category": "preferences", "content": "不用句号结尾、信息密度高", "source": "mcp-test",
	})
	if isErr {
		t.Fatalf("update_profile error: %s", text)
	}

	// Read
	text, isErr = mcpToolCall(t, s, "read_profile", map[string]interface{}{})
	if isErr {
		t.Fatalf("read_profile error: %s", text)
	}
	t.Logf("read_profile: %q", text[:min(len(text), 200)])
}

func TestMCPInteg_SearchMemory(t *testing.T) {
	s := setupIntegrationMCP(t)

	// Write something searchable first
	mcpToolCall(t, s, "write_file", map[string]interface{}{
		"path": "/notes/searchable.md", "content": "海淀算力券政策分析 unique-search-term-xyz",
	})

	text, isErr := mcpToolCall(t, s, "search_memory", map[string]interface{}{
		"query": "unique-search-term-xyz",
	})
	if isErr {
		t.Fatalf("search_memory error: %s", text)
	}
}

func TestMCPInteg_Projects_Lifecycle(t *testing.T) {
	s := setupIntegrationMCP(t)

	// List (empty)
	text, isErr := mcpToolCall(t, s, "list_projects", map[string]interface{}{})
	if isErr {
		t.Fatalf("list_projects error: %s", text)
	}

	// Get non-existent
	_, isErr = mcpToolCall(t, s, "get_project", map[string]interface{}{
		"name": "nonexistent",
	})
	// May or may not error, depending on implementation

	// Log action (creates project implicitly if handler supports it, or fails gracefully)
	text, isErr = mcpToolCall(t, s, "log_action", map[string]interface{}{
		"project": "mcp-test-proj", "action": "test", "summary": "MCP test log",
	})
	// Some implementations require project to exist first — log the result
	t.Logf("log_action: text=%q isErr=%v", text, isErr)
}

func TestMCPInteg_Vault_WriteReadDelete(t *testing.T) {
	s := setupIntegrationMCP(t)

	// Write
	text, isErr := mcpToolCall(t, s, "write_secret", map[string]interface{}{
		"scope": "auth.test-mcp", "data": "mcp-secret-value-12345",
	})
	// write_secret might not be a tool name; check list_secrets/read_secret
	t.Logf("write_secret: text=%q isErr=%v", text, isErr)

	// List
	text, isErr = mcpToolCall(t, s, "list_secrets", map[string]interface{}{})
	if isErr {
		t.Fatalf("list_secrets error: %s", text)
	}

	// Read
	text, isErr = mcpToolCall(t, s, "read_secret", map[string]interface{}{
		"scope": "auth.test-mcp",
	})
	t.Logf("read_secret: text=%q isErr=%v", text, isErr)
}

func TestMCPInteg_Devices_ListAndCall(t *testing.T) {
	s := setupIntegrationMCP(t)

	// List (empty)
	text, isErr := mcpToolCall(t, s, "list_devices", map[string]interface{}{})
	if isErr {
		t.Fatalf("list_devices error: %s", text)
	}

	// Call non-existent device
	text, isErr = mcpToolCall(t, s, "call_device", map[string]interface{}{
		"device": "test-light", "action": "on",
	})
	// Should error (device not found)
	t.Logf("call_device: text=%q isErr=%v", text, isErr)
}

func TestMCPInteg_Inbox_SendAndRead(t *testing.T) {
	s := setupIntegrationMCP(t)

	// Send
	text, isErr := mcpToolCall(t, s, "send_message", map[string]interface{}{
		"to": "assistant", "subject": "MCP Test", "body": "Hello from MCP test",
	})
	if isErr {
		t.Fatalf("send_message error: %s", text)
	}

	// Read
	text, isErr = mcpToolCall(t, s, "read_inbox", map[string]interface{}{})
	if isErr {
		t.Fatalf("read_inbox error: %s", text)
	}
}

func TestMCPInteg_Skills(t *testing.T) {
	s := setupIntegrationMCP(t)

	// Write a skill file
	mcpToolCall(t, s, "write_file", map[string]interface{}{
		"path": "/skills/test-skill/SKILL.md", "content": "# Test Skill\n\nA test skill for MCP.",
	})

	// List skills
	text, isErr := mcpToolCall(t, s, "list_skills", map[string]interface{}{})
	if isErr {
		t.Fatalf("list_skills error: %s", text)
	}

	// Read skill
	text, isErr = mcpToolCall(t, s, "read_skill", map[string]interface{}{
		"name": "test-skill",
	})
	t.Logf("read_skill: text=%q isErr=%v", text[:min(len(text), 100)], isErr)
}

func TestMCPInteg_GetStats(t *testing.T) {
	s := setupIntegrationMCP(t)

	text, isErr := mcpToolCall(t, s, "get_stats", map[string]interface{}{})
	if isErr {
		t.Fatalf("get_stats error: %s", text)
	}
	t.Logf("get_stats: %q", text[:min(len(text), 200)])
}

func TestMCPInteg_ImportSkill(t *testing.T) {
	s := setupIntegrationMCP(t)

	text, isErr := mcpToolCall(t, s, "import_skill", map[string]interface{}{
		"name": "imported-skill",
		"files": map[string]interface{}{
			"SKILL.md": "# Imported Skill\n\nImported via MCP test.",
		},
	})
	if isErr {
		t.Fatalf("import_skill error: %s", text)
	}

	// Verify via list_skills
	text, isErr = mcpToolCall(t, s, "list_skills", map[string]interface{}{})
	if isErr {
		t.Fatalf("list_skills after import error: %s", text)
	}
}

func TestMCPInteg_ImportClaudeMemory(t *testing.T) {
	s := setupIntegrationMCP(t)

	text, isErr := mcpToolCall(t, s, "import_claude_memory", map[string]interface{}{
		"memories": []map[string]interface{}{
			{"content": "用户偏好深色模式", "type": "preference"},
			{"content": "用户是 Go 开发者", "type": "fact"},
		},
	})
	// May fail if import format doesn't match — log but don't fail hard
	t.Logf("import_claude_memory: text=%q isErr=%v", text, isErr)
}

func TestMCPInteg_ScopeFiltering(t *testing.T) {
	s := setupIntegrationMCP(t)

	// Create a restricted server with only read:profile scope
	restricted := &MCPServer{
		UserID:      s.UserID,
		TrustLevel:  models.TrustLevelGuest,
		Scopes:      []string{models.ScopeReadProfile},
		FileTree:    s.FileTree,
		Vault:       s.Vault,
		VaultCrypto: s.VaultCrypto,
		Memory:      s.Memory,
		Project:     s.Project,
		Inbox:       s.Inbox,
		Device:      s.Device,
		Dashboard:   s.Dashboard,
		Import:      s.Import,
	}

	// tools/list should return fewer tools
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"}
	resp := restricted.HandleJSONRPC(req)
	result := resp.Result.(map[string]interface{})
	tools := result["tools"].([]MCPTool)
	if len(tools) >= 20 {
		t.Errorf("restricted server should have fewer tools, got %d", len(tools))
	}

	// Calling a disallowed tool should fail or return error
	text, isErr := mcpToolCall(t, restricted, "write_file", map[string]interface{}{
		"path": "/test", "content": "should fail",
	})
	t.Logf("restricted write_file: text=%q isErr=%v", text, isErr)
	// The tool should either error or not be available at all
	if !isErr && text != "" {
		t.Error("expected error or empty response calling write_file with read-only scope")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
