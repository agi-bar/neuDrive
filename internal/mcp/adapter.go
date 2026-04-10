package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/hubpath"
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

const (
	claudeWebInlineArchiveMaxZipBytes = 64 * 1024
	claudeWebInlineArchiveMaxZipLabel = "64 KB"
)

func prepareSkillsUploadWarning() string {
	return fmt.Sprintf(
		"For Claude Web, if a skills zip is over %s (%d bytes) or its size is unknown, do not read or base64 it into import_skills_archive. Do not cat base64(zip) or emit long archive strings into the conversation, because that can crash the session. Call prepare_skills_upload instead.",
		claudeWebInlineArchiveMaxZipLabel,
		claudeWebInlineArchiveMaxZipBytes,
	)
}

func prepareSkillsUploadProbeURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/test/post"
}

func prepareSkillsUploadAllowedDomain(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err == nil && parsed.Host != "" {
		return parsed.Hostname()
	}
	return "the Agent Hub host"
}

func prepareSkillsUploadConnectivityHelp(baseURL string) string {
	domain := prepareSkillsUploadAllowedDomain(baseURL)
	probeURL := prepareSkillsUploadProbeURL(baseURL)
	return fmt.Sprintf(
		"After prepare_skills_upload, first POST %s. If it returns 200, upload the zip directly from the sandbox with curl_example. If that POST fails in Claude Web, ask the user to open Settings -> Capabilities -> Code execution and file creation -> Additional allowed domains and add %s, then retry. If the user does not add it or direct upload is still blocked, fall back to browser_upload_url or ask the user to run curl_example manually.",
		probeURL,
		domain,
	)
}

func isHiddenMCPPath(rawPath string) bool {
	publicPath := hubpath.NormalizePublic(rawPath)
	for _, prefix := range []string{"/devices", "/roles", "/inbox"} {
		if publicPath == prefix || strings.HasPrefix(publicPath, prefix+"/") {
			return true
		}
	}
	return false
}

// MCPServer handles MCP protocol for Agent Hub
type MCPServer struct {
	UserID     uuid.UUID
	TrustLevel int
	Scopes     []string
	BaseURL    string

	Connection  *services.ConnectionService
	OAuth       *services.OAuthService
	FileTree    *services.FileTreeService
	Vault       *services.VaultService
	VaultCrypto *vault.Vault
	Memory      *services.MemoryService
	Project     *services.ProjectService
	Inbox       *services.InboxService
	Device      *services.DeviceService
	Dashboard   *services.DashboardService
	Import      *services.ImportService
	Token       *services.TokenService
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
				"name":         "agenthub",
				"version":      "1.0.0",
				"instructions": fmt.Sprintf("Start by reading agenthub://skills/agenthub/SKILL.md or calling read_skill with name=agenthub. For import, export, restore, migration, or connector work, read agenthub://skills/portability/<platform>/SKILL.md first or call read_skill with name=portability/<platform>. If no platform-specific manual exists, read agenthub://skills/portability/general/SKILL.md or call read_skill with name=portability/general. Use import_skill for one text/code skill directory. For Claude Web skills zips, stat the zip first: if it is over %s (%d bytes) or size is unknown, do not read or base64 it into MCP args, do not cat base64(zip), and do not emit long archive strings into the conversation; call prepare_skills_upload instead. After prepare_skills_upload, POST the returned connectivity_probe_url first. If it returns 200, use curl_example to upload directly from the sandbox. If the POST fails in Claude Web, ask the user to add the Agent Hub domain to Settings -> Capabilities -> Code execution and file creation -> Additional allowed domains, then retry; otherwise fall back to browser_upload_url or manual curl. Use import_skills_archive only for archives already known to be small enough for one tool call. Reserve write_file for single-file patching. Use list_skills to discover available manuals, and use read_file on /skills/... if your client prefers virtual file paths. Current public agent surface focuses on profile, memory, projects, skills, tree, and sync.", claudeWebInlineArchiveMaxZipLabel, claudeWebInlineArchiveMaxZipBytes),
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
			Description: "全文搜索记忆、项目和技能资料",
			InputSchema: jsonSchema(map[string]interface{}{
				"query": prop("string", "搜索关键词"),
				"scope": prop("string", "搜索范围: memory, projects, skills, all (默认 all)"),
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
			Description: "写入文件到文件树；适合单文件修补，不作为单个 skill 的正式导入主路径",
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
			Description: "列出所有可用技能，包含系统级 portability 手册（如 portability/chatgpt）",
			InputSchema: jsonSchema(map[string]interface{}{}),
		},
		{
			Name:        "read_skill",
			Description: "读取技能的 SKILL.md；当用户提到平台迁移时，也可读取 portability/<platform>，未知平台可读取 portability/general",
			InputSchema: jsonSchema(map[string]interface{}{
				"name": prop("string", "技能名称"),
			}, "name"),
		},
		{
			Name:        "get_stats",
			Description: "获取 Hub 统计概览",
			InputSchema: jsonSchema(map[string]interface{}{}),
		},
		{
			Name:        "save_memory",
			Description: "记住信息。当用户说「记住」「记一下」「别忘了」或需要保存任何信息供将来使用时，使用此工具。支持单条或多条批量保存。内容自动按日期归档到记忆库，可通过 search_memory 检索",
			InputSchema: jsonSchema(map[string]interface{}{
				"memories": propArray("object", "要记住的条目数组 [{content: '内容', title: '标题(可选)'}]，也可只传一条"),
			}, "memories"),
		},
		{
			Name:        "import_skill",
			Description: "导入单个 skill 目录（正式主路）；适合一个 skill 的完整文本文件集",
			InputSchema: jsonSchema(map[string]interface{}{
				"name":  prop("string", "技能名称"),
				"files": propObject("文件内容 map: {路径: 内容}"),
			}, "name", "files"),
		},
		{
			Name:        "prepare_skills_upload",
			Description: fmt.Sprintf("为 skills zip 上传准备 token、直传探测 URL、浏览器链接和 curl 命令。先检查 zip 大小；若 Claude Web zip 大于 %s 或大小未知，不要读取或 base64 化，直接调用此工具。调用后先 POST connectivity_probe_url；若返回 200，就直接用 curl_example 从 sandbox 上传。若 Claude Web 探测失败，提示用户在 Settings -> Capabilities -> Code execution and file creation -> Additional allowed domains 添加 Agent Hub 域名，然后重试；否则回退到 browser_upload_url 或让用户手动执行 curl_example", claudeWebInlineArchiveMaxZipLabel),
			InputSchema: jsonSchema(map[string]interface{}{
				"purpose":     prop("string", "用途说明"),
				"platform":    prop("string", "来源平台 (默认 claude-web)"),
				"ttl_minutes": prop("integer", "有效期分钟数，范围 5-120，默认 30"),
			}, "purpose"),
		},
		{
			Name:        "import_skills_archive",
			Description: fmt.Sprintf("导入已知足够小的 skills zip archive；仅适合已经确认能安全放进一次 MCP tool call 的 archive。Claude Web zip 大于 %s 或大小未知时，不要读取或 base64 化，改用 prepare_skills_upload", claudeWebInlineArchiveMaxZipLabel),
			InputSchema: jsonSchema(map[string]interface{}{
				"archive_base64": prop("string", fmt.Sprintf("仅用于已经在内存中的小型 zip archive 的 base64 内容。Claude Web zip 大于 %s 或大小未知时，不要为了填这个字段而去读取、cat、或 base64 化大文件，也不要把超长 archive 字符串放进对话", claudeWebInlineArchiveMaxZipLabel)),
				"archive_name":   prop("string", "归档文件名 (默认 skills.zip)"),
				"platform":       prop("string", "来源平台 (默认 claude-web)"),
			}, "archive_base64"),
		},
		{
			Name:        "create_sync_token",
			Description: "生成短命 scoped token，用于批量同步 bundle 到 /agent/import/bundle 或从 /agent/export/bundle 导出",
			InputSchema: jsonSchema(map[string]interface{}{
				"purpose":     prop("string", "用途说明"),
				"access":      prop("string", "权限: push, pull, both (默认 push)"),
				"ttl_minutes": prop("integer", "有效期分钟数，范围 5-120，默认 30"),
			}, "purpose"),
		},
		// import_claude_memory removed from MCP tools — available via admin HTTP API only
	}

	filteredByCapability := make([]MCPTool, 0, len(tools))
	for _, t := range tools {
		if s.supportsTool(t.Name) {
			filteredByCapability = append(filteredByCapability, t)
		}
	}

	// Filter by scopes if token has limited scopes
	if len(s.Scopes) > 0 && !models.HasScope(s.Scopes, models.ScopeAdmin) {
		var filtered []MCPTool
		for _, t := range filteredByCapability {
			if s.toolAllowed(t.Name) {
				filtered = append(filtered, t)
			}
		}
		return filtered
	}
	return filteredByCapability
}

func (s *MCPServer) supportsTool(name string) bool {
	switch name {
	case "read_profile", "update_profile", "search_memory", "list_projects", "create_project",
		"get_project", "log_action", "list_directory", "read_file", "write_file",
		"list_secrets", "read_secret", "list_skills", "read_skill", "get_stats", "save_memory",
		"import_skill", "import_skills_archive", "create_sync_token", "prepare_skills_upload", "import_claude_memory":
	default:
		return false
	}
	switch name {
	case "read_profile", "update_profile", "save_memory":
		return s.Memory != nil
	case "search_memory", "list_directory", "read_file", "write_file", "list_skills", "read_skill":
		return s.FileTree != nil
	case "list_projects", "create_project", "get_project", "log_action":
		return s.Project != nil
	case "list_secrets", "read_secret":
		return s.Vault != nil
	case "get_stats":
		return s.Dashboard != nil
	case "import_skill", "import_skills_archive", "import_claude_memory":
		return s.Import != nil
	case "create_sync_token", "prepare_skills_upload":
		return s.Token != nil
	default:
		return false
	}
}

func (s *MCPServer) toolAllowed(name string) bool {
	scopeMap := map[string]string{
		"read_profile":          models.ScopeReadProfile,
		"update_profile":        models.ScopeWriteProfile,
		"search_memory":         models.ScopeReadMemory,
		"list_projects":         models.ScopeReadProjects,
		"create_project":        models.ScopeWriteProjects,
		"get_project":           models.ScopeReadProjects,
		"log_action":            models.ScopeWriteProjects,
		"list_directory":        models.ScopeReadTree,
		"read_file":             models.ScopeReadTree,
		"write_file":            models.ScopeWriteTree,
		"list_secrets":          models.ScopeReadVault,
		"read_secret":           models.ScopeReadVault,
		"list_skills":           models.ScopeReadSkills,
		"read_skill":            models.ScopeReadSkills,
		"get_stats":             models.ScopeReadProfile,
		"save_memory":           models.ScopeWriteMemory,
		"import_skill":          models.ScopeWriteSkills,
		"import_skills_archive": models.ScopeWriteSkills,
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

	if len(s.Scopes) > 0 && !models.HasScope(s.Scopes, models.ScopeAdmin) && !s.toolAllowed(params.Name) {
		return fmt.Sprintf("error: tool %q not allowed by current scopes", params.Name), true
	}
	if !s.isKnownTool(params.Name) {
		return fmt.Sprintf("unknown tool: %s", params.Name), true
	}
	if !s.supportsTool(params.Name) {
		return fmt.Sprintf("error: tool %q not configured", params.Name), true
	}

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
		prefixes := []string{}
		switch scope {
		case "memory":
			prefixes = []string{"/memory", "/identity"}
		case "projects":
			prefixes = []string{"/projects"}
		case "skills":
			prefixes = []string{"/skills"}
		default:
			prefixes = []string{"/memory", "/identity", "/projects", "/skills"}
		}

		seen := make(map[string]bool)
		for _, prefix := range prefixes {
			entries, err := s.FileTree.Search(ctx, s.UserID, query, s.TrustLevel, prefix)
			if err != nil {
				return fmt.Sprintf("error searching %s: %v", prefix, err), true
			}
			for _, e := range entries {
				publicPath := hubpath.StorageToPublic(e.Path)
				if isHiddenMCPPath(publicPath) {
					continue
				}
				if seen[publicPath] {
					continue
				}
				seen[publicPath] = true
				results = append(results, map[string]interface{}{
					"type":    e.Kind,
					"path":    publicPath,
					"snippet": mcpSnippetText(e.Content),
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
		if isHiddenMCPPath(path) {
			return fmt.Sprintf("error: path %q is not available on the public MCP surface", path), true
		}
		entries, err := s.FileTree.List(ctx, s.UserID, path, s.TrustLevel)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		// Normalize any legacy storage aliases to canonical public paths.
		filtered := entries[:0]
		for i := range entries {
			rendered := s.renderSystemSkillEntry(ctx, &entries[i])
			entries[i] = *rendered
			entries[i].Path = hubpath.StorageToPublic(entries[i].Path)
			if isHiddenMCPPath(entries[i].Path) {
				continue
			}
			filtered = append(filtered, entries[i])
		}
		result, _ := json.MarshalIndent(filtered, "", "  ")
		return string(result), false

	case "read_file":
		path, _ := args["path"].(string)
		if isHiddenMCPPath(path) {
			return fmt.Sprintf("error: path %q is not available on the public MCP surface", path), true
		}
		entry, err := s.FileTree.Read(ctx, s.UserID, path, s.TrustLevel)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		entry = s.renderSystemSkillEntry(ctx, entry)
		return entry.Content, false

	case "write_file":
		path, _ := args["path"].(string)
		if isHiddenMCPPath(path) {
			return fmt.Sprintf("error: path %q is not available on the public MCP surface", path), true
		}
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
		entries, err := s.FileTree.ListSkillSummaries(ctx, s.UserID, s.TrustLevel)
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
		entry = s.renderSystemSkillEntry(ctx, entry)
		return entry.Content, false

	case "get_stats":
		stats, err := s.Dashboard.GetStats(ctx, s.UserID)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		result, _ := json.MarshalIndent(stats, "", "  ")
		return string(result), false

	case "save_memory":
		memoriesRaw, ok := args["memories"].([]interface{})
		if !ok || len(memoriesRaw) == 0 {
			return "error: memories array is required (at least one {content, title?} object)", true
		}
		var saved []string

		for _, item := range memoriesRaw {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			content, _ := m["content"].(string)
			if content == "" {
				continue
			}
			title, _ := m["title"].(string)
			entry, err := s.Memory.WriteScratchWithTitle(ctx, s.UserID, content, "mcp", title)
			if err != nil {
				return fmt.Sprintf("error saving memory: %v", err), true
			}
			if entry != nil {
				saved = append(saved, hubpath.StorageToPublic(entry.Path))
			}
		}

		if len(saved) == 0 {
			return "error: no valid memory items to save", true
		}
		return fmt.Sprintf("saved %d memories: %s", len(saved), strings.Join(saved, ", ")), false

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

	case "import_skills_archive":
		archiveBase64, _ := args["archive_base64"].(string)
		archiveBase64 = strings.TrimSpace(archiveBase64)
		if archiveBase64 == "" {
			return "error: archive_base64 is required", true
		}
		if strings.HasPrefix(archiveBase64, "data:") {
			if idx := strings.Index(archiveBase64, ","); idx >= 0 {
				archiveBase64 = archiveBase64[idx+1:]
			}
		}

		archiveData, err := base64.StdEncoding.DecodeString(archiveBase64)
		if err != nil {
			archiveData, err = base64.RawStdEncoding.DecodeString(archiveBase64)
			if err != nil {
				return fmt.Sprintf("error: decode archive_base64: %v", err), true
			}
		}

		archiveName, _ := args["archive_name"].(string)
		if strings.TrimSpace(archiveName) == "" {
			archiveName = "skills.zip"
		}
		platform, _ := args["platform"].(string)
		if strings.TrimSpace(platform) == "" {
			platform = "claude-web"
		}

		result, err := s.Import.ImportSkillsArchive(ctx, s.UserID, archiveData, platform, archiveName)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		payload, _ := json.MarshalIndent(result, "", "  ")
		return string(payload), false

	case "create_sync_token":
		if len(s.Scopes) > 0 && !models.HasScope(s.Scopes, models.ScopeAdmin) {
			return "error: create_sync_token requires admin scope", true
		}
		if s.Token == nil {
			return "error: token service not configured", true
		}

		purpose, _ := args["purpose"].(string)
		if strings.TrimSpace(purpose) == "" {
			return "error: purpose is required", true
		}

		access, _ := args["access"].(string)
		access = strings.TrimSpace(strings.ToLower(access))
		if access == "" {
			access = "push"
		}

		rawTTL, hasTTL := args["ttl_minutes"]
		ttlMinutes := 30
		if hasTTL {
			switch typed := rawTTL.(type) {
			case float64:
				ttlMinutes = int(typed)
			case int:
				ttlMinutes = typed
			}
		}
		if ttlMinutes < 5 || ttlMinutes > 120 {
			return "error: ttl_minutes must be between 5 and 120", true
		}

		var scopes []string
		switch access {
		case "push":
			scopes = []string{models.ScopeWriteBundle}
		case "pull":
			scopes = []string{models.ScopeReadBundle}
		case "both":
			scopes = []string{models.ScopeReadBundle, models.ScopeWriteBundle}
		default:
			return "error: access must be one of push, pull, both", true
		}

		tokenName := fmt.Sprintf("sync:%s", purpose)
		resp, err := s.Token.CreateEphemeralToken(ctx, s.UserID, tokenName, scopes, models.TrustLevelWork, time.Duration(ttlMinutes)*time.Minute)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		payload, _ := json.MarshalIndent(map[string]interface{}{
			"token":      resp.Token,
			"expires_at": resp.ScopedToken.ExpiresAt.Format(time.RFC3339),
			"api_base":   s.BaseURL,
			"scopes":     resp.ScopedToken.Scopes,
			"usage":      fmt.Sprintf("agenthub sync login --api-base %s --token %s && agenthub sync push --bundle backup.ahub", s.BaseURL, resp.Token),
		}, "", "  ")
		return string(payload), false

	case "prepare_skills_upload":
		if len(s.Scopes) > 0 && !models.HasScope(s.Scopes, models.ScopeAdmin) {
			return "error: prepare_skills_upload requires admin scope", true
		}
		if s.Token == nil {
			return "error: token service not configured", true
		}

		purpose, _ := args["purpose"].(string)
		if strings.TrimSpace(purpose) == "" {
			return "error: purpose is required", true
		}
		platform, _ := args["platform"].(string)
		platform = strings.TrimSpace(strings.ToLower(platform))
		if platform == "" {
			platform = "claude-web"
		}

		rawTTL, hasTTL := args["ttl_minutes"]
		ttlMinutes := 30
		if hasTTL {
			switch typed := rawTTL.(type) {
			case float64:
				ttlMinutes = int(typed)
			case int:
				ttlMinutes = typed
			}
		}
		if ttlMinutes < 5 || ttlMinutes > 120 {
			return "error: ttl_minutes must be between 5 and 120", true
		}

		tokenName := fmt.Sprintf("skills-import:%s", purpose)
		resp, err := s.Token.CreateEphemeralToken(ctx, s.UserID, tokenName, []string{models.ScopeWriteSkills}, models.TrustLevelWork, time.Duration(ttlMinutes)*time.Minute)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		uploadURL := strings.TrimRight(s.BaseURL, "/") + "/agent/import/skills?platform=" + platform
		probeURL := prepareSkillsUploadProbeURL(s.BaseURL)
		browserUploadURL := strings.TrimRight(s.BaseURL, "/") + "/import/skills?token=" + url.QueryEscape(resp.Token) + "&platform=" + url.QueryEscape(platform)
		payload, _ := json.MarshalIndent(map[string]interface{}{
			"token":                        resp.Token,
			"expires_at":                   resp.ScopedToken.ExpiresAt.Format(time.RFC3339),
			"api_base":                     s.BaseURL,
			"upload_url":                   uploadURL,
			"connectivity_probe_url":       probeURL,
			"connectivity_probe_method":    http.MethodPost,
			"browser_upload_url":           browserUploadURL,
			"scopes":                       resp.ScopedToken.Scopes,
			"recommended_flow":             "probe_then_agent_curl_upload",
			"inline_archive_max_zip_bytes": claudeWebInlineArchiveMaxZipBytes,
			"warning":                      prepareSkillsUploadWarning(),
			"connectivity_failure_help":    prepareSkillsUploadConnectivityHelp(s.BaseURL),
			"connectivity_probe_curl":      fmt.Sprintf(`curl -f -sS -o /dev/null -w "%%{http_code}" -X POST "%s"`, probeURL),
			"curl_example":                 fmt.Sprintf(`curl -f -X POST -H "Authorization: Bearer %s" -F "platform=%s" -F "file=@/mnt/user-data/outputs/agenthub-skills.zip" "%s"`, resp.Token, platform, uploadURL),
		}, "", "  ")
		return string(payload), false

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

func (s *MCPServer) isKnownTool(name string) bool {
	switch name {
	case "read_profile", "update_profile", "search_memory", "list_projects", "create_project",
		"get_project", "log_action", "list_directory", "read_file", "write_file",
		"list_secrets", "read_secret", "list_skills", "read_skill", "get_stats", "save_memory",
		"import_skill", "import_skills_archive", "create_sync_token", "prepare_skills_upload", "import_claude_memory":
		return true
	default:
		return false
	}
}

func (s *MCPServer) getResources() []MCPResource {
	if s.FileTree == nil {
		return nil
	}
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

	resources = append(resources, wellKnownResources()...)
	return resources
}

func wellKnownResources() []MCPResource {
	return []MCPResource{
		{
			URI:         "agenthub://skills/agenthub/SKILL.md",
			Name:        "/skills/agenthub/SKILL.md",
			Description: "Agent Hub umbrella skill entrypoint",
			MimeType:    "text/markdown",
		},
		{
			URI:         "agenthub://skills/portability/general/SKILL.md",
			Name:        "/skills/portability/general/SKILL.md",
			Description: "General portability manual",
			MimeType:    "text/markdown",
		},
		{
			URI:         "agenthub://skills/portability/claude/SKILL.md",
			Name:        "/skills/portability/claude/SKILL.md",
			Description: "Claude portability manual",
			MimeType:    "text/markdown",
		},
		{
			URI:         "agenthub://skills/portability/chatgpt/SKILL.md",
			Name:        "/skills/portability/chatgpt/SKILL.md",
			Description: "ChatGPT portability manual",
			MimeType:    "text/markdown",
		},
		{
			URI:         "agenthub://skills/portability/codex/SKILL.md",
			Name:        "/skills/portability/codex/SKILL.md",
			Description: "Codex portability manual",
			MimeType:    "text/markdown",
		},
		{
			URI:      "agenthub://identity/profile.json",
			Name:     "用户身份信息",
			MimeType: "application/json",
		},
		{
			URI:      "agenthub://memory/SKILL.md",
			Name:     "记忆系统说明",
			MimeType: "text/markdown",
		},
		{
			URI:      "agenthub://vault/SKILL.md",
			Name:     "保险柜说明",
			MimeType: "text/markdown",
		},
	}
}

func (s *MCPServer) readResource(uri string) (string, error) {
	if s.FileTree == nil {
		return "", fmt.Errorf("file tree service not configured")
	}
	path := strings.TrimPrefix(uri, "agenthub://")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	ctx := context.Background()
	entry, err := s.FileTree.Read(ctx, s.UserID, path, s.TrustLevel)
	if err != nil {
		return "", err
	}
	entry = s.renderSystemSkillEntry(ctx, entry)
	return entry.Content, nil
}

func mcpSnippetText(raw string) string {
	raw = strings.Join(strings.Fields(raw), " ")
	if len(raw) <= 180 {
		return raw
	}
	return strings.TrimSpace(raw[:177]) + "..."
}

// RunStdio runs the MCP server on stdin/stdout (for Claude Code stdio transport)
func (s *MCPServer) RunStdio(stdin io.Reader, stdout io.Writer) error {
	return RunStdioHandler(s, stdin, stdout)
}

// ServeHTTP handles MCP over HTTP (for http transport)
func (s *MCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ServeHTTPHandler(s, w, r)
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
