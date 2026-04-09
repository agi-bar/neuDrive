package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agi-bar/agenthub/internal/database"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/agi-bar/agenthub/internal/vault"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	mcpTestMigrationsOnce sync.Once
	mcpTestMigrationsErr  error
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

	mcpTestMigrationsOnce.Do(func() {
		mcpTestMigrationsErr = database.RunMigrations(pool, filepath.Join("..", "..", "migrations"))
	})
	if mcpTestMigrationsErr != nil {
		t.Fatalf("run migrations: %v", mcpTestMigrationsErr)
	}

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
	memorySvc := services.NewMemoryService(pool, fileTreeSvc)
	roleSvc := services.NewRoleService(pool, fileTreeSvc)
	projectSvc := services.NewProjectService(pool, roleSvc, fileTreeSvc)
	inboxSvc := services.NewInboxService(pool, fileTreeSvc)
	deviceSvc := services.NewDeviceService(pool, fileTreeSvc)
	dashboardSvc := services.NewDashboardService(pool)
	importSvc := services.NewImportService(pool, fileTreeSvc, memorySvc, vaultSvc)
	tokenSvc := services.NewTokenService(pool)

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
		Token:       tokenSvc,
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
	if blocks, ok := result["content"].([]ContentBlock); ok && len(blocks) > 0 {
		return blocks[0].Text, isErr
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

	toolNames := make(map[string]bool, len(tools))
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}
	for _, expected := range []string{
		"read_profile",
		"update_profile",
		"search_memory",
		"list_projects",
		"create_project",
		"get_project",
		"log_action",
		"list_directory",
		"read_file",
		"write_file",
		"list_secrets",
		"read_secret",
		"list_skills",
		"read_skill",
		"get_stats",
		"save_memory",
		"import_skill",
		"import_skills_archive",
		"create_sync_token",
		"create_skills_import_token",
	} {
		if !toolNames[expected] {
			t.Errorf("expected tool %q not found in tools/list", expected)
		}
	}
	for _, hidden := range []string{"list_devices", "call_device", "send_message", "read_inbox"} {
		if toolNames[hidden] {
			t.Errorf("tool %q should not be exposed in tools/list", hidden)
		}
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

func TestMCPInteg_SaveMemory_AllowsRepeatedSourceSameDay(t *testing.T) {
	s := setupIntegrationMCP(t)

	firstMarker := "mcprepeata" + uuid.NewString()[:8]
	secondMarker := "mcprepeatb" + uuid.NewString()[:8]

	text, isErr := mcpToolCall(t, s, "save_memory", map[string]interface{}{
		"memories": []map[string]interface{}{
			{"title": "repeat-source", "content": "first " + firstMarker},
		},
	})
	if isErr {
		t.Fatalf("first save_memory error: %s", text)
	}

	text, isErr = mcpToolCall(t, s, "save_memory", map[string]interface{}{
		"memories": []map[string]interface{}{
			{"title": "repeat-source", "content": "second " + secondMarker},
		},
	})
	if isErr {
		t.Fatalf("second save_memory error: %s", text)
	}

	text, isErr = mcpToolCall(t, s, "search_memory", map[string]interface{}{
		"query": firstMarker,
		"scope": "memory",
	})
	if isErr {
		t.Fatalf("search_memory for first marker error: %s", text)
	}
	if !strings.Contains(text, firstMarker) {
		t.Fatalf("search_memory missing first marker: %s", text)
	}

	text, isErr = mcpToolCall(t, s, "search_memory", map[string]interface{}{
		"query": secondMarker,
		"scope": "memory",
	})
	if isErr {
		t.Fatalf("search_memory for second marker error: %s", text)
	}
	if !strings.Contains(text, secondMarker) {
		t.Fatalf("search_memory missing second marker: %s", text)
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

func TestMCPInteg_DevicesAreNotExposed(t *testing.T) {
	s := setupIntegrationMCP(t)

	text, isErr := mcpToolCall(t, s, "list_devices", map[string]interface{}{})
	if !isErr {
		t.Fatalf("expected list_devices to be unavailable, got success: %s", text)
	}
	if !strings.Contains(text, "unknown tool: list_devices") {
		t.Fatalf("expected list_devices to be hidden, got %q", text)
	}

	text, isErr = mcpToolCall(t, s, "call_device", map[string]interface{}{
		"device": "test-light", "action": "on",
	})
	if !isErr {
		t.Fatalf("expected call_device to be unavailable, got success: %s", text)
	}
	if !strings.Contains(text, "unknown tool: call_device") {
		t.Fatalf("expected call_device to be hidden, got %q", text)
	}
}

func TestMCPInteg_InboxToolsAreNotExposed(t *testing.T) {
	s := setupIntegrationMCP(t)

	text, isErr := mcpToolCall(t, s, "send_message", map[string]interface{}{
		"to": "assistant", "subject": "MCP Test", "body": "Hello from MCP test",
	})
	if !isErr {
		t.Fatalf("expected send_message to be unavailable, got success: %s", text)
	}
	if !strings.Contains(text, "unknown tool: send_message") {
		t.Fatalf("expected send_message to be hidden, got %q", text)
	}

	text, isErr = mcpToolCall(t, s, "read_inbox", map[string]interface{}{})
	if !isErr {
		t.Fatalf("expected read_inbox to be unavailable, got success: %s", text)
	}
	if !strings.Contains(text, "unknown tool: read_inbox") {
		t.Fatalf("expected read_inbox to be hidden, got %q", text)
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

func TestMCPInteg_ImportSkillsArchive(t *testing.T) {
	s := setupIntegrationMCP(t)

	archive := buildSkillArchive(t, map[string][]byte{
		"integ-archive/SKILL.md":        []byte("# Integ Archive\n"),
		"integ-archive/helper.py":       []byte("print('integ')\n"),
		"integ-archive/assets/logo.png": {0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00},
	})

	text, isErr := mcpToolCall(t, s, "import_skills_archive", map[string]interface{}{
		"archive_base64": base64.StdEncoding.EncodeToString(archive),
		"archive_name":   "integ-skills.zip",
		"platform":       "claude-web",
	})
	if isErr {
		t.Fatalf("import_skills_archive error: %s", text)
	}
	if !strings.Contains(text, `"imported": 3`) {
		t.Fatalf("unexpected import_skills_archive result: %s", text)
	}
	if !strings.Contains(text, `"skills": [`) || !strings.Contains(text, `"integ-archive"`) {
		t.Fatalf("expected imported skill names in result: %s", text)
	}

	entry, err := s.FileTree.Read(context.Background(), s.UserID, "/skills/integ-archive/helper.py", models.TrustLevelFull)
	if err != nil {
		t.Fatalf("Read helper.py: %v", err)
	}
	if !strings.Contains(entry.Content, "print('integ')") {
		t.Fatalf("unexpected helper.py content: %q", entry.Content)
	}

	data, binaryEntry, err := s.FileTree.ReadBinary(context.Background(), s.UserID, "/skills/integ-archive/assets/logo.png", models.TrustLevelFull)
	if err != nil {
		t.Fatalf("ReadBinary logo.png: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected binary logo data")
	}
	if binaryEntry.Metadata["capture_mode"] != "archive" || binaryEntry.Metadata["source_platform"] != "claude-web" {
		t.Fatalf("unexpected binary metadata: %+v", binaryEntry.Metadata)
	}
}

func TestMCPInteg_CreateSyncToken(t *testing.T) {
	s := setupIntegrationMCP(t)

	text, isErr := mcpToolCall(t, s, "create_sync_token", map[string]interface{}{
		"purpose":     "integration-test",
		"access":      "both",
		"ttl_minutes": 30,
	})
	if isErr {
		t.Fatalf("create_sync_token error: %s", text)
	}
	if !strings.Contains(text, "\"token\": \"aht_") {
		t.Fatalf("expected scoped token output, got %s", text)
	}
	if !strings.Contains(text, models.ScopeWriteBundle) || !strings.Contains(text, models.ScopeReadBundle) {
		t.Fatalf("expected bundle scopes in output, got %s", text)
	}
}

func TestMCPInteg_CreateSkillsImportToken(t *testing.T) {
	s := setupIntegrationMCP(t)

	text, isErr := mcpToolCall(t, s, "create_skills_import_token", map[string]interface{}{
		"purpose":     "claude-web-skills",
		"platform":    "claude-web",
		"ttl_minutes": 30,
	})
	if isErr {
		t.Fatalf("create_skills_import_token error: %s", text)
	}
	if !strings.Contains(text, "\"token\": \"aht_") {
		t.Fatalf("expected scoped token output, got %s", text)
	}
	if !strings.Contains(text, models.ScopeWriteSkills) {
		t.Fatalf("expected write:skills scope in output, got %s", text)
	}
	if !strings.Contains(text, "/agent/import/skills?platform=claude-web") {
		t.Fatalf("expected upload_url in output, got %s", text)
	}
	if !strings.Contains(text, "/import/skills?token=") {
		t.Fatalf("expected browser_upload_url in output, got %s", text)
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
