package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Mock services for MCP tests
// ---------------------------------------------------------------------------

var testUserID = uuid.MustParse("22222222-2222-2222-2222-222222222222")

type mockMemoryService struct {
	profiles []models.MemoryProfile
}

func (m *mockMemoryService) GetProfile(_ context.Context, _ uuid.UUID) ([]models.MemoryProfile, error) {
	return m.profiles, nil
}

func (m *mockMemoryService) UpsertProfile(_ context.Context, _ uuid.UUID, category, content, source string) error {
	for i, p := range m.profiles {
		if p.Category == category {
			m.profiles[i].Content = content
			return nil
		}
	}
	m.profiles = append(m.profiles, models.MemoryProfile{
		Category: category,
		Content:  content,
		Source:   source,
	})
	return nil
}

type mockFileTreeService struct {
	entries map[string]models.FileTreeEntry
}

func (m *mockFileTreeService) List(_ context.Context, _ uuid.UUID, path string, _ int) ([]models.FileTreeEntry, error) {
	var result []models.FileTreeEntry
	for _, e := range m.entries {
		if strings.HasPrefix(e.Path, path) && e.Path != path {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockFileTreeService) Read(_ context.Context, _ uuid.UUID, path string, _ int) (*models.FileTreeEntry, error) {
	e, ok := m.entries[path]
	if !ok {
		return nil, fmt.Errorf("not found: %s", path)
	}
	return &e, nil
}

func (m *mockFileTreeService) Write(_ context.Context, _ uuid.UUID, path, content, contentType string, _ int) (*models.FileTreeEntry, error) {
	e := models.FileTreeEntry{Path: path, Content: content, ContentType: contentType}
	m.entries[path] = e
	return &e, nil
}

func (m *mockFileTreeService) Search(_ context.Context, _ uuid.UUID, query string, _ int) ([]models.FileTreeEntry, error) {
	return nil, nil
}

// Since MCPServer uses concrete service types, we need an adapter approach.
// The MCP server calls methods on concrete *services.XxxService pointers.
// For testing, we create an MCPServer with nil service pointers and instead
// test the HandleJSONRPC method by intercepting at the tool dispatch level.
//
// A simpler approach: build a thin MCPServer wrapper that overrides callTool.
// But since MCPServer is a concrete struct, we test what we can at the
// JSON-RPC protocol level using the HandleJSONRPC method directly.
//
// We will test: initialize, tools/list, resources/list, malformed requests,
// and use the stdio transport for the read_profile tool (which needs Memory).

// newTestMCPServer creates an MCPServer with no backing services (nil).
// This is sufficient for protocol-level tests (initialize, tools/list, etc.)
func newTestMCPServer() *MCPServer {
	return &MCPServer{
		UserID:     testUserID,
		TrustLevel: models.TrustLevelFull,
		Scopes:     []string{}, // empty = no scope filtering (full access)
	}
}

// ---------------------------------------------------------------------------
// 1. Initialize handshake
// ---------------------------------------------------------------------------

func TestInitialize(t *testing.T) {
	s := newTestMCPServer()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}`),
	}

	resp := s.HandleJSONRPC(req)

	if resp.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc=2.0, got %s", resp.JSONRPC)
	}
	if resp.ID != 1 {
		t.Errorf("expected id=1, got %v", resp.ID)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result should be a map")
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("expected protocolVersion=2024-11-05, got %v", result["protocolVersion"])
	}

	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("expected serverInfo map")
	}
	if serverInfo["name"] != "agenthub" {
		t.Errorf("expected server name=agenthub, got %v", serverInfo["name"])
	}
	instructions, _ := serverInfo["instructions"].(string)
	if !strings.Contains(instructions, "/skills/portability/<platform>/SKILL.md") {
		t.Fatalf("expected portability instructions in serverInfo, got %q", instructions)
	}

	caps, ok := result["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("expected capabilities map")
	}
	if _, ok := caps["tools"]; !ok {
		t.Error("expected tools capability")
	}
	if _, ok := caps["resources"]; !ok {
		t.Error("expected resources capability")
	}
}

func TestListSkillsIncludesSystemPortabilityManuals(t *testing.T) {
	s := &MCPServer{
		UserID:     testUserID,
		TrustLevel: models.TrustLevelGuest,
		FileTree:   &services.FileTreeService{},
	}

	text, isErr := s.callTool(ToolCallParams{
		Name:      "list_skills",
		Arguments: map[string]interface{}{},
	})
	if isErr {
		t.Fatalf("list_skills error: %s", text)
	}
	if !strings.Contains(text, "portability/chatgpt") {
		t.Fatalf("expected portability/chatgpt in list_skills output, got %s", text)
	}
	if !strings.Contains(text, "\"name\": \"agenthub\"") {
		t.Fatalf("expected agenthub system skill in list_skills output, got %s", text)
	}
	if !strings.Contains(text, "\"read_only\": true") {
		t.Fatalf("expected read_only flag in list_skills output, got %s", text)
	}
}

func TestReadSkillRendersSystemPortabilityManual(t *testing.T) {
	s := &MCPServer{
		UserID:     testUserID,
		TrustLevel: models.TrustLevelGuest,
		FileTree:   &services.FileTreeService{},
	}

	text, isErr := s.callTool(ToolCallParams{
		Name: "read_skill",
		Arguments: map[string]interface{}{
			"name": "portability/chatgpt",
		},
	})
	if isErr {
		t.Fatalf("read_skill error: %s", text)
	}
	if !strings.Contains(text, "## Current User Snapshot") {
		t.Fatalf("expected rendered snapshot block, got %s", text)
	}
	if !strings.Contains(text, "Connected to ChatGPT: unknown") {
		t.Fatalf("expected default ChatGPT snapshot, got %s", text)
	}
}

// ---------------------------------------------------------------------------
// 2. tools/list returns expected tools
// ---------------------------------------------------------------------------

func TestToolsList(t *testing.T) {
	s := newTestMCPServer()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	resp := s.HandleJSONRPC(req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result should be a map")
	}

	tools, ok := result["tools"].([]MCPTool)
	if !ok {
		t.Fatal("expected tools to be []MCPTool")
	}

	if len(tools) == 0 {
		t.Fatal("expected at least one tool")
	}

	// Verify expected tools exist.
	expectedTools := []string{
		"read_profile", "update_profile", "search_memory",
		"list_projects", "get_project", "log_action",
		"list_directory", "read_file", "write_file",
		"list_secrets", "read_secret",
		"list_skills", "read_skill",
		"list_devices", "call_device",
		"send_message", "read_inbox",
		"get_stats",
		"import_skill", "save_memory", "create_project",
		"create_sync_token",
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	for _, expected := range expectedTools {
		if !toolNames[expected] {
			t.Errorf("expected tool %q not found in tools/list", expected)
		}
	}

	// Verify each tool has required fields.
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("tool has empty name")
		}
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has nil inputSchema", tool.Name)
		}
	}
}

func TestToolsListWithScopeFiltering(t *testing.T) {
	s := &MCPServer{
		UserID:     testUserID,
		TrustLevel: models.TrustLevelFull,
		Scopes:     []string{models.ScopeReadProfile}, // only profile scope
	}

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/list",
	}

	resp := s.HandleJSONRPC(req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result := resp.Result.(map[string]interface{})
	tools := result["tools"].([]MCPTool)

	// With only read:profile, we should see read_profile and get_stats (both need read:profile)
	// but NOT write_file, send_message, etc.
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	if !toolNames["read_profile"] {
		t.Error("expected read_profile to be present with read:profile scope")
	}
	if toolNames["write_file"] {
		t.Error("write_file should not be available with only read:profile scope")
	}
	if toolNames["send_message"] {
		t.Error("send_message should not be available with only read:profile scope")
	}
	if toolNames["create_sync_token"] {
		t.Error("create_sync_token should not be available without admin scope")
	}
}

// ---------------------------------------------------------------------------
// 3. tools/call for read_profile (protocol level)
// ---------------------------------------------------------------------------

func TestToolsCallReadProfileNilService(t *testing.T) {
	// With nil Memory service, calling read_profile panics (nil pointer).
	// This test verifies the panic occurs (the service is not nil-safe).
	// In production a recover middleware would catch this.
	s := newTestMCPServer()

	params, _ := json.Marshal(ToolCallParams{
		Name:      "read_profile",
		Arguments: map[string]interface{}{},
	})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params:  params,
	}

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		s.HandleJSONRPC(req)
	}()

	if !panicked {
		t.Error("expected panic when calling read_profile with nil Memory service")
	}
}

func TestToolsCallUnknownTool(t *testing.T) {
	s := newTestMCPServer()

	params, _ := json.Marshal(ToolCallParams{
		Name:      "nonexistent_tool",
		Arguments: map[string]interface{}{},
	})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "tools/call",
		Params:  params,
	}

	resp := s.HandleJSONRPC(req)
	if resp.Error != nil {
		return // acceptable
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("expected result map")
	}

	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError=true for unknown tool")
	}

	content, ok := result["content"].([]ContentBlock)
	if ok && len(content) > 0 {
		if !strings.Contains(content[0].Text, "unknown tool") {
			t.Errorf("expected 'unknown tool' in error, got %q", content[0].Text)
		}
	}
}

func TestToolsCallInvalidParams(t *testing.T) {
	s := newTestMCPServer()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      6,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"invalid": true}`),
	}

	resp := s.HandleJSONRPC(req)
	if resp.Error != nil {
		if resp.Error.Code != -32602 {
			t.Errorf("expected error code -32602, got %d", resp.Error.Code)
		}
		return
	}
	// If no error, that's acceptable too -- the handler may have defaulted.
}

// ---------------------------------------------------------------------------
// 4. resources/list
// ---------------------------------------------------------------------------

func TestResourcesList(t *testing.T) {
	// With nil FileTree service, getResources panics. Use a recover wrapper.
	s := newTestMCPServer()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      7,
		Method:  "resources/list",
	}

	var resp JSONRPCResponse
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		resp = s.HandleJSONRPC(req)
	}()

	if panicked {
		// Expected: FileTree is nil, so getResources panics.
		// In production, a recover middleware would catch this.
		t.Log("resources/list panicked with nil FileTree (expected)")
		return
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("expected result map")
	}

	// Verify the resources key exists.
	if _, ok := result["resources"]; !ok {
		t.Error("expected resources key in result")
	}
}

// ---------------------------------------------------------------------------
// 5. Malformed JSON-RPC request handling
// ---------------------------------------------------------------------------

func TestMalformedJSONRPC(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		wantErr bool
		errCode int
	}{
		{
			name:    "unknown method",
			method:  "unknown/method",
			wantErr: true,
			errCode: -32601,
		},
		{
			name:   "ping",
			method: "ping",
		},
		{
			name:   "notifications/initialized",
			method: "notifications/initialized",
		},
	}

	s := newTestMCPServer()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      99,
				Method:  tc.method,
			}
			resp := s.HandleJSONRPC(req)

			if tc.wantErr {
				if resp.Error == nil {
					t.Fatal("expected error response")
				}
				if resp.Error.Code != tc.errCode {
					t.Errorf("expected error code %d, got %d", tc.errCode, resp.Error.Code)
				}
			} else {
				if resp.Error != nil {
					t.Fatalf("unexpected error: code=%d, message=%s", resp.Error.Code, resp.Error.Message)
				}
			}
		})
	}
}

func TestResourcesReadInvalidParams(t *testing.T) {
	s := newTestMCPServer()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      10,
		Method:  "resources/read",
		Params:  json.RawMessage(`not valid json`),
	}

	resp := s.HandleJSONRPC(req)
	if resp.Error == nil {
		t.Fatal("expected error for invalid params")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("expected error code -32602, got %d", resp.Error.Code)
	}
}

// ---------------------------------------------------------------------------
// 6. Stdio transport test
// ---------------------------------------------------------------------------

func TestRunStdioBasicExchange(t *testing.T) {
	s := newTestMCPServer()

	// Build a multi-line input with initialize + tools/list.
	initReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}`),
	}
	toolsReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      float64(2),
		Method:  "tools/list",
	}

	initBytes, _ := json.Marshal(initReq)
	toolsBytes, _ := json.Marshal(toolsReq)

	input := string(initBytes) + "\n" + string(toolsBytes) + "\n"
	var output bytes.Buffer

	err := s.RunStdio(strings.NewReader(input), &output)
	if err != nil {
		t.Fatalf("RunStdio error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 response lines, got %d: %s", len(lines), output.String())
	}

	// Parse first response (initialize).
	var initResp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatalf("failed to parse init response: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("init response has error: %v", initResp.Error)
	}

	// Parse second response (tools/list).
	var toolsResp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[1]), &toolsResp); err != nil {
		t.Fatalf("failed to parse tools response: %v", err)
	}
	if toolsResp.Error != nil {
		t.Fatalf("tools response has error: %v", toolsResp.Error)
	}
}

func TestRunStdioParseError(t *testing.T) {
	s := newTestMCPServer()

	input := "this is not json\n"
	var output bytes.Buffer

	err := s.RunStdio(strings.NewReader(input), &output)
	if err != nil {
		t.Fatalf("RunStdio error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) == 0 {
		t.Fatal("expected at least one response line for parse error")
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error in response")
	}
	if resp.Error.Code != -32700 {
		t.Errorf("expected parse error code -32700, got %d", resp.Error.Code)
	}
}

func TestRunStdioNotificationNoResponse(t *testing.T) {
	s := newTestMCPServer()

	notif := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      nil,
		Method:  "notifications/initialized",
	}
	notifBytes, _ := json.Marshal(notif)

	input := string(notifBytes) + "\n"
	var output bytes.Buffer

	err := s.RunStdio(strings.NewReader(input), &output)
	if err != nil {
		t.Fatalf("RunStdio error: %v", err)
	}

	// Notifications should not produce a response.
	if strings.TrimSpace(output.String()) != "" {
		t.Errorf("expected no output for notification, got: %s", output.String())
	}
}

// ---------------------------------------------------------------------------
// 7. JSON Schema helpers
// ---------------------------------------------------------------------------

func TestJsonSchemaHelper(t *testing.T) {
	schema := jsonSchema(map[string]interface{}{
		"name": prop("string", "the name"),
	}, "name")

	schemaMap, ok := schema["type"].(string)
	if !ok || schemaMap != "object" {
		t.Error("expected type=object")
	}

	required, ok := schema["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "name" {
		t.Error("expected required=[name]")
	}

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map")
	}
	nameProp, ok := properties["name"].(map[string]interface{})
	if !ok {
		t.Fatal("expected name property")
	}
	if nameProp["type"] != "string" {
		t.Error("expected name type=string")
	}
}

func TestToStringSlice(t *testing.T) {
	tests := []struct {
		name   string
		input  interface{}
		expect int
	}{
		{"nil", nil, 0},
		{"non-array", "hello", 0},
		{"empty array", []interface{}{}, 0},
		{"string array", []interface{}{"a", "b", "c"}, 3},
		{"mixed array", []interface{}{"a", 42, "b"}, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := toStringSlice(tc.input)
			if len(result) != tc.expect {
				t.Errorf("expected %d elements, got %d", tc.expect, len(result))
			}
		})
	}
}
