package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/agi-bar/agenthub/internal/vault"
	"github.com/google/uuid"
)

// JSON-RPC 2.0 types
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP protocol types
type MCPTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ResourceReadParams struct {
	URI string `json:"uri"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MCPServer handles MCP protocol for Agent Hub
type MCPServer struct {
	UserID     uuid.UUID
	TrustLevel int
	Scopes     []string

	FileTree    *services.FileTreeService
	Vault       *services.VaultService
	VaultCrypto *vault.Vault
	Memory      *services.MemoryService
	Project     *services.ProjectService
	Inbox       *services.InboxService
	Device      *services.DeviceService
	Dashboard   *services.DashboardService
	Import      *services.ImportService
}

func (s *MCPServer) HandleJSONRPC(req JSONRPCRequest) JSONRPCResponse {
	resp := JSONRPCResponse{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{},
				"resources": map[string]interface{}{"listChanged": false},
			},
			"serverInfo": map[string]interface{}{
				"name":    "agenthub",
				"version": "1.0.0",
			},
		}
	case "notifications/initialized":
		// No response needed for notifications
		resp.Result = map[string]interface{}{}
	case "tools/list":
		resp.Result = map[string]interface{}{"tools": s.getTools()}
	case "tools/call":
		var params ToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &RPCError{Code: -32602, Message: "invalid params"}
			return resp
		}
		result, isErr := s.callTool(params)
		resp.Result = map[string]interface{}{
			"content": []ContentBlock{{Type: "text", Text: result}},
			"isError": isErr,
		}
	case "resources/list":
		resp.Result = map[string]interface{}{"resources": s.getResources()}
	case "resources/read":
		var params ResourceReadParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &RPCError{Code: -32602, Message: "invalid params"}
			return resp
		}
		content, err := s.readResource(params.URI)
		if err != nil {
			resp.Error = &RPCError{Code: -32000, Message: err.Error()}
			return resp
		}
		resp.Result = map[string]interface{}{
			"contents": []map[string]interface{}{
				{"uri": params.URI, "mimeType": "text/plain", "text": content},
			},
		}
	case "ping":
		resp.Result = map[string]interface{}{}
	default:
		resp.Error = &RPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)}
	}
	return resp
}

func (s *MCPServer) getTools() []MCPTool {
	tools := []MCPTool{
		{
			Name:        "read_profile",
			Description: "读取用户偏好和个人信息 (preferences, relationships, principles)",
			InputSchema: jsonSchema(map[string]interface{}{
				"category": prop("string", "分类: preferences, relationships, principles (空=全部)"),
			}),
		},
		{
			Name:        "update_profile",
			Description: "更新用户的长期稳定偏好（极少变化的信息，如写作风格、沟通习惯、做事原则）。注意：日常交互中产生的信息应使用 save_memory 而非此工具",
			InputSchema: jsonSchema(map[string]interface{}{
				"category": prop("string", "分类: preferences, relationships, principles"),
				"content":  prop("string", "内容"),
			}, "category", "content"),
		},
		{
			Name:        "search_memory",
			Description: "全文搜索记忆和邮件存档",
			InputSchema: jsonSchema(map[string]interface{}{
				"query": prop("string", "搜索关键词"),
				"scope": prop("string", "搜索范围: memory, inbox, all (默认 all)"),
			}, "query"),
		},
		{
			Name:        "list_projects",
			Description: "列出所有项目",
			InputSchema: jsonSchema(map[string]interface{}{}),
		},
		{
			Name:        "create_project",
			Description: "创建新项目",
			InputSchema: jsonSchema(map[string]interface{}{
				"name": prop("string", "项目名称 (只允许字母、数字、横杠、下划线)"),
			}, "name"),
		},
		{
			Name:        "get_project",
			Description: "获取项目上下文和最近日志",
			InputSchema: jsonSchema(map[string]interface{}{
				"name": prop("string", "项目名称"),
			}, "name"),
		},
		{
			Name:        "log_action",
			Description: "向项目日志追加一条记录",
			InputSchema: jsonSchema(map[string]interface{}{
				"project": prop("string", "项目名称"),
				"action":  prop("string", "动作类型"),
				"summary": prop("string", "摘要"),
				"source":  prop("string", "来源平台 (claude, gpt, etc)"),
				"tags":    propArray("string", "标签"),
			}, "project", "action", "summary"),
		},
		{
			Name:        "list_directory",
			Description: "列出文件树中的目录内容",
			InputSchema: jsonSchema(map[string]interface{}{
				"path": prop("string", "目录路径 (如 /skills, /memory)"),
			}, "path"),
		},
		{
			Name:        "read_file",
			Description: "读取文件树中的文件",
			InputSchema: jsonSchema(map[string]interface{}{
				"path": prop("string", "文件路径"),
			}, "path"),
		},
		{
			Name:        "write_file",
			Description: "写入文件到文件树",
			InputSchema: jsonSchema(map[string]interface{}{
				"path":    prop("string", "文件路径"),
				"content": prop("string", "文件内容"),
			}, "path", "content"),
		},
		{
			Name:        "list_secrets",
			Description: "列出可用的保险柜 scope（不返回实际值）",
			InputSchema: jsonSchema(map[string]interface{}{}),
		},
		{
			Name:        "read_secret",
			Description: "读取保险柜中的加密信息（需要对应信任等级和权限）",
			InputSchema: jsonSchema(map[string]interface{}{
				"scope": prop("string", "scope 名称 (如 auth.github, identity.personal)"),
			}, "scope"),
		},
		{
			Name:        "list_skills",
			Description: "列出所有可用技能",
			InputSchema: jsonSchema(map[string]interface{}{}),
		},
		{
			Name:        "read_skill",
			Description: "读取技能的 SKILL.md",
			InputSchema: jsonSchema(map[string]interface{}{
				"name": prop("string", "技能名称"),
			}, "name"),
		},
		{
			Name:        "list_devices",
			Description: "列出注册的设备",
			InputSchema: jsonSchema(map[string]interface{}{}),
		},
		{
			Name:        "call_device",
			Description: "调用设备动作",
			InputSchema: jsonSchema(map[string]interface{}{
				"device": prop("string", "设备名称"),
				"action": prop("string", "动作"),
				"params": propObject("动作参数"),
			}, "device", "action"),
		},
		{
			Name:        "send_message",
			Description: "发送 Agent 间消息",
			InputSchema: jsonSchema(map[string]interface{}{
				"to":          prop("string", "收件地址 (如 worker:policy@de.hub)"),
				"subject":     prop("string", "主题"),
				"body":        prop("string", "正文（自包含，收件方无需前置信息）"),
				"domain":      prop("string", "领域: governance, kb, collab, tools"),
				"action_type": prop("string", "类型: task_request, info, result, alert, handoff, memory_sync"),
				"tags":        propArray("string", "标签"),
			}, "to", "subject", "body"),
		},
		{
			Name:        "read_inbox",
			Description: "读取收件箱消息",
			InputSchema: jsonSchema(map[string]interface{}{
				"role":   prop("string", "角色名称 (空=全部)"),
				"status": prop("string", "状态: incoming, read, archived (空=incoming)"),
			}),
		},
		{
			Name:        "get_stats",
			Description: "获取 Hub 统计概览",
			InputSchema: jsonSchema(map[string]interface{}{}),
		},
		{
			Name:        "save_memory",
			Description: "记住一条信息。当用户说「记住」「记一下」「别忘了」或需要保存任何信息供将来使用时，使用此工具。内容自动按日期归档到记忆库，可通过 search_memory 检索",
			InputSchema: jsonSchema(map[string]interface{}{
				"content": prop("string", "要记住的内容（支持 Markdown）"),
				"title":   prop("string", "简短标题（可选，用于分类，如 meeting-notes, todo, idea）"),
			}, "content"),
		},
		{
			Name:        "import_skill",
			Description: "导入 .skill 目录",
			InputSchema: jsonSchema(map[string]interface{}{
				"name":  prop("string", "技能名称"),
				"files": propObject("文件内容 map: {路径: 内容}"),
			}, "name", "files"),
		},
		{
			Name:        "import_claude_memory",
			Description: "批量导入 Claude 记忆导出文件（仅用于从 Claude 平台迁移历史记忆，日常记忆请用 save_memory）",
			InputSchema: jsonSchema(map[string]interface{}{
				"memories": propArray("object", "记忆条目 [{content, type, created_at}]"),
			}, "memories"),
		},
	}

	// Filter by scopes if token has limited scopes
	if len(s.Scopes) > 0 && !models.HasScope(s.Scopes, models.ScopeAdmin) {
		var filtered []MCPTool
		for _, t := range tools {
			if s.toolAllowed(t.Name) {
				filtered = append(filtered, t)
			}
		}
		return filtered
	}
	return tools
}

func (s *MCPServer) toolAllowed(name string) bool {
	scopeMap := map[string]string{
		"read_profile":         models.ScopeReadProfile,
		"update_profile":       models.ScopeWriteProfile,
		"search_memory":        models.ScopeReadMemory,
		"list_projects":        models.ScopeReadProjects,
		"create_project":       models.ScopeWriteProjects,
		"get_project":          models.ScopeReadProjects,
		"log_action":           models.ScopeWriteProjects,
		"list_directory":       models.ScopeReadTree,
		"read_file":            models.ScopeReadTree,
		"write_file":           models.ScopeWriteTree,
		"list_secrets":         models.ScopeReadVault,
		"read_secret":          models.ScopeReadVault,
		"list_skills":          models.ScopeReadSkills,
		"read_skill":           models.ScopeReadSkills,
		"list_devices":         models.ScopeReadDevices,
		"call_device":          models.ScopeCallDevices,
		"send_message":         models.ScopeWriteInbox,
		"read_inbox":           models.ScopeReadInbox,
		"get_stats":            models.ScopeReadProfile,
		"save_memory":          models.ScopeWriteMemory,
		"import_skill":         models.ScopeWriteSkills,
		"import_claude_memory": models.ScopeWriteMemory,
	}
	required, ok := scopeMap[name]
	if !ok {
		return false
	}
	return models.HasScope(s.Scopes, required)
}

func (s *MCPServer) callTool(params ToolCallParams) (string, bool) {
	ctx := context.Background()
	args := params.Arguments

	switch params.Name {
	case "read_profile":
		category, _ := args["category"].(string)
		profiles, err := s.Memory.GetProfile(ctx, s.UserID)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		if category != "" {
			for _, p := range profiles {
				if p.Category == category {
					return p.Content, false
				}
			}
			return fmt.Sprintf("category %q not found", category), true
		}
		result := ""
		for _, p := range profiles {
			result += fmt.Sprintf("## %s\n%s\n\n", p.Category, p.Content)
		}
		return result, false

	case "update_profile":
		category, _ := args["category"].(string)
		content, _ := args["content"].(string)
		if err := s.Memory.UpsertProfile(ctx, s.UserID, category, content, "mcp"); err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return "profile updated", false

	case "search_memory":
		query, _ := args["query"].(string)
		scope, _ := args["scope"].(string)
		if scope == "" {
			scope = "all"
		}

		var results []interface{}

		// Search file_tree (memory files)
		if scope == "memory" || scope == "all" {
			entries, err := s.FileTree.Search(ctx, s.UserID, query, s.TrustLevel, "/memory/")
			if err != nil {
				return fmt.Sprintf("error searching files: %v", err), true
			}
			for _, e := range entries {
				results = append(results, map[string]interface{}{
					"type":    "memory",
					"path":    e.Path,
					"content": e.Content,
				})
			}
		}

		// Search inbox messages
		if scope == "inbox" || scope == "all" {
			msgs, err := s.Inbox.Search(ctx, s.UserID, query, "")
			if err != nil {
				return fmt.Sprintf("error searching inbox: %v", err), true
			}
			for _, m := range msgs {
				results = append(results, map[string]interface{}{
					"type":    "inbox",
					"subject": m.Subject,
					"body":    m.Body,
					"from":    m.FromAddress,
					"date":    m.CreatedAt,
				})
			}
		}

		if len(results) == 0 {
			return "no results found", false
		}
		result, _ := json.MarshalIndent(results, "", "  ")
		return string(result), false

	case "list_projects":
		projects, err := s.Project.List(ctx, s.UserID)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		result, _ := json.MarshalIndent(projects, "", "  ")
		return string(result), false

	case "create_project":
		name, _ := args["name"].(string)
		if name == "" {
			return "error: project name is required", true
		}
		project, err := s.Project.Create(ctx, s.UserID, name)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		result, _ := json.MarshalIndent(project, "", "  ")
		return string(result), false

	case "get_project":
		name, _ := args["name"].(string)
		project, err := s.Project.Get(ctx, s.UserID, name)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		logs, _ := s.Project.GetLogs(ctx, project.ID, 20)
		out := map[string]interface{}{"project": project, "recent_logs": logs}
		result, _ := json.MarshalIndent(out, "", "  ")
		return string(result), false

	case "log_action":
		projectName, _ := args["project"].(string)
		project, err := s.Project.Get(ctx, s.UserID, projectName)
		if err != nil {
			return fmt.Sprintf("error: project %q not found", projectName), true
		}
		action, _ := args["action"].(string)
		summary, _ := args["summary"].(string)
		source, _ := args["source"].(string)
		if source == "" {
			source = "mcp"
		}
		tags := toStringSlice(args["tags"])
		logEntry := models.ProjectLog{
			ProjectID: project.ID,
			Source:    source,
			Role:      "assistant",
			Action:    action,
			Summary:   summary,
			Tags:      tags,
		}
		if err := s.Project.AppendLog(ctx, project.ID, logEntry); err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return "log entry added", false

	case "list_directory":
		path, _ := args["path"].(string)
		entries, err := s.FileTree.List(ctx, s.UserID, path, s.TrustLevel)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		result, _ := json.MarshalIndent(entries, "", "  ")
		return string(result), false

	case "read_file":
		path, _ := args["path"].(string)
		entry, err := s.FileTree.Read(ctx, s.UserID, path, s.TrustLevel)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return entry.Content, false

	case "write_file":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		if _, err := s.FileTree.Write(ctx, s.UserID, path, content, "text/markdown", s.TrustLevel); err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return "file written", false

	case "list_secrets":
		scopes, err := s.Vault.ListScopes(ctx, s.UserID, s.TrustLevel)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		result, _ := json.MarshalIndent(scopes, "", "  ")
		return string(result), false

	case "read_secret":
		scope, _ := args["scope"].(string)
		data, err := s.Vault.Read(ctx, s.UserID, scope, s.TrustLevel)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return string(data), false

	case "list_skills":
		entries, err := s.FileTree.List(ctx, s.UserID, "/skills", s.TrustLevel)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		result, _ := json.MarshalIndent(entries, "", "  ")
		return string(result), false

	case "read_skill":
		name, _ := args["name"].(string)
		path := fmt.Sprintf("/skills/%s/SKILL.md", name)
		entry, err := s.FileTree.Read(ctx, s.UserID, path, s.TrustLevel)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return entry.Content, false

	case "list_devices":
		devices, err := s.Device.List(ctx, s.UserID)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		result, _ := json.MarshalIndent(devices, "", "  ")
		return string(result), false

	case "call_device":
		device, _ := args["device"].(string)
		action, _ := args["action"].(string)
		callParams, _ := args["params"].(map[string]interface{})
		resp, err := s.Device.Call(ctx, s.UserID, device, action, callParams)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		result, _ := json.MarshalIndent(resp, "", "  ")
		return string(result), false

	case "send_message":
		to, _ := args["to"].(string)
		subject, _ := args["subject"].(string)
		body, _ := args["body"].(string)
		domain, _ := args["domain"].(string)
		actionType, _ := args["action_type"].(string)
		tags := toStringSlice(args["tags"])
		msg := models.InboxMessage{
			FromAddress: "assistant@hub",
			ToAddress:   to,
			Subject:     subject,
			Body:        body,
			Domain:      domain,
			ActionType:  actionType,
			Tags:        tags,
			Status:      "incoming",
			Priority:    "normal",
		}
		if _, err := s.Inbox.Send(ctx, s.UserID, msg); err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return "message sent", false

	case "read_inbox":
		role, _ := args["role"].(string)
		status, _ := args["status"].(string)
		if status == "" {
			status = "incoming"
		}
		msgs, err := s.Inbox.GetMessages(ctx, s.UserID, role, status)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		result, _ := json.MarshalIndent(msgs, "", "  ")
		return string(result), false

	case "get_stats":
		stats, err := s.Dashboard.GetStats(ctx, s.UserID)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		result, _ := json.MarshalIndent(stats, "", "  ")
		return string(result), false

	case "save_memory":
		content, _ := args["content"].(string)
		if content == "" {
			return "error: content is required", true
		}
		title, _ := args["title"].(string)

		// Generate path: /memory/scratch/{date}-{title}.md
		now := time.Now()
		filename := now.Format("2006-01-02")
		if title != "" {
			// Sanitize title for filename
			safe := strings.Map(func(r rune) rune {
				if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
					return r
				}
				if r >= 0x4e00 && r <= 0x9fff { // CJK characters
					return r
				}
				return '-'
			}, title)
			filename += "-" + safe
		}
		path := fmt.Sprintf("/memory/scratch/%s.md", filename)

		// If file already exists for today, append instead of overwrite
		existing, err := s.FileTree.Read(ctx, s.UserID, path, s.TrustLevel)
		if err == nil && existing.Content != "" {
			content = existing.Content + "\n\n---\n\n" + content
		}

		if _, err := s.FileTree.Write(ctx, s.UserID, path, content, "text/markdown", models.TrustLevelFull); err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return fmt.Sprintf("memory saved to %s", path), false

	case "import_skill":
		name, _ := args["name"].(string)
		filesRaw, _ := args["files"].(map[string]interface{})
		files := make(map[string]string)
		for k, v := range filesRaw {
			files[k], _ = v.(string)
		}
		count, err := s.Import.ImportSkill(ctx, s.UserID, name, files)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return fmt.Sprintf("imported %d files for skill %q", count, name), false

	case "import_claude_memory":
		memoriesRaw, _ := json.Marshal(args["memories"])
		count, err := s.Import.ImportClaudeMemory(ctx, s.UserID, memoriesRaw)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return fmt.Sprintf("imported %d memory items", count), false

	default:
		return fmt.Sprintf("unknown tool: %s", params.Name), true
	}
}

func (s *MCPServer) getResources() []MCPResource {
	ctx := context.Background()
	entries, err := s.FileTree.List(ctx, s.UserID, "/", s.TrustLevel)
	if err != nil {
		return nil
	}

	var resources []MCPResource
	for _, e := range entries {
		if !e.IsDirectory {
			resources = append(resources, MCPResource{
				URI:      fmt.Sprintf("agenthub://%s", strings.TrimPrefix(e.Path, "/")),
				Name:     e.Path,
				MimeType: e.ContentType,
			})
		}
	}

	// Add well-known resources
	wellKnown := []MCPResource{
		{URI: "agenthub://identity/profile.json", Name: "用户身份信息", MimeType: "application/json"},
		{URI: "agenthub://memory/SKILL.md", Name: "记忆系统说明", MimeType: "text/markdown"},
		{URI: "agenthub://vault/SKILL.md", Name: "保险柜说明", MimeType: "text/markdown"},
	}
	resources = append(resources, wellKnown...)
	return resources
}

func (s *MCPServer) readResource(uri string) (string, error) {
	path := strings.TrimPrefix(uri, "agenthub://")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Special: profile.json returns user profile as JSON
	if path == "/identity/profile.json" {
		ctx := context.Background()
		profiles, err := s.Memory.GetProfile(ctx, s.UserID)
		if err != nil {
			return "", err
		}
		result, _ := json.MarshalIndent(profiles, "", "  ")
		return string(result), nil
	}

	ctx := context.Background()
	entry, err := s.FileTree.Read(ctx, s.UserID, path, s.TrustLevel)
	if err != nil {
		return "", err
	}
	return entry.Content, nil
}

// RunStdio runs the MCP server on stdin/stdout (for Claude Code stdio transport)
func (s *MCPServer) RunStdio(stdin io.Reader, stdout io.Writer) error {
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			errResp := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &RPCError{Code: -32700, Message: "parse error"},
			}
			writeJSONLine(stdout, errResp)
			continue
		}

		resp := s.HandleJSONRPC(req)

		// Notifications (no ID) don't get responses
		if req.ID == nil && (strings.HasPrefix(req.Method, "notifications/")) {
			continue
		}

		writeJSONLine(stdout, resp)
	}
	return scanner.Err()
}

// ServeHTTP handles MCP over HTTP (for http transport)
func (s *MCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0", Error: &RPCError{Code: -32700, Message: "parse error"},
		})
		return
	}

	resp := s.HandleJSONRPC(req)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Helper: write JSON line to writer
func writeJSONLine(w io.Writer, v interface{}) {
	data, _ := json.Marshal(v)
	fmt.Fprintf(w, "%s\n", data)
}

// Helper: JSON Schema builder
func jsonSchema(properties map[string]interface{}, required ...string) map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func prop(typ, desc string) map[string]interface{} {
	return map[string]interface{}{"type": typ, "description": desc}
}

func propArray(itemType, desc string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": desc,
		"items":       map[string]interface{}{"type": itemType},
	}
}

func propObject(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "object", "description": desc}
}

func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
