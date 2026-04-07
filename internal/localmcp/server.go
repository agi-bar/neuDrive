package localmcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/localstore"
	"github.com/agi-bar/agenthub/internal/mcp"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
)

type Server struct {
	Store      *localstore.Store
	UserID     uuid.UUID
	TrustLevel int
	Scopes     []string
	BaseURL    string
}

func (s *Server) HandleJSONRPC(req mcp.JSONRPCRequest) mcp.JSONRPCResponse {
	resp := mcp.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{},
				"resources": map[string]interface{}{"listChanged": false},
			},
			"serverInfo": map[string]interface{}{
				"name":    "agenthub-local",
				"version": "1.0.0",
			},
		}
	case "notifications/initialized":
		resp.Result = map[string]interface{}{}
	case "tools/list":
		resp.Result = map[string]interface{}{"tools": s.tools()}
	case "tools/call":
		var params mcp.ToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &mcp.RPCError{Code: -32602, Message: "invalid params"}
			return resp
		}
		result, isErr := s.callTool(params)
		resp.Result = map[string]interface{}{
			"content": []mcp.ContentBlock{{Type: "text", Text: result}},
			"isError": isErr,
		}
	case "resources/list":
		resp.Result = map[string]interface{}{"resources": []mcp.MCPResource{}}
	case "resources/read":
		var params mcp.ResourceReadParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &mcp.RPCError{Code: -32602, Message: "invalid params"}
			return resp
		}
		content, err := s.readResource(params.URI)
		if err != nil {
			resp.Error = &mcp.RPCError{Code: -32000, Message: err.Error()}
			return resp
		}
		resp.Result = map[string]interface{}{
			"contents": []map[string]interface{}{{"uri": params.URI, "mimeType": "text/plain", "text": content}},
		}
	case "ping":
		resp.Result = map[string]interface{}{}
	default:
		resp.Error = &mcp.RPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)}
	}
	return resp
}

func (s *Server) RunStdio(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	encoder := json.NewEncoder(out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req mcp.JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := mcp.JSONRPCResponse{JSONRPC: "2.0", Error: &mcp.RPCError{Code: -32700, Message: "parse error"}}
			if err := encoder.Encode(resp); err != nil {
				return err
			}
			continue
		}
		resp := s.HandleJSONRPC(req)
		if strings.HasPrefix(req.Method, "notifications/") {
			continue
		}
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Server) tools() []mcp.MCPTool {
	tools := []mcp.MCPTool{
		{Name: "read_profile", Description: "读取用户偏好和个人信息", InputSchema: schema(map[string]interface{}{"category": prop("string", "分类")})},
		{Name: "update_profile", Description: "更新用户长期偏好", InputSchema: schema(map[string]interface{}{"category": prop("string", "分类"), "content": prop("string", "内容")}, "category", "content")},
		{Name: "search_memory", Description: "全文搜索记忆和技能", InputSchema: schema(map[string]interface{}{"query": prop("string", "搜索关键词"), "scope": prop("string", "搜索范围")}, "query")},
		{Name: "list_projects", Description: "列出所有项目", InputSchema: schema(map[string]interface{}{})},
		{Name: "create_project", Description: "创建新项目", InputSchema: schema(map[string]interface{}{"name": prop("string", "项目名称")}, "name")},
		{Name: "get_project", Description: "获取项目上下文和最近日志", InputSchema: schema(map[string]interface{}{"name": prop("string", "项目名称")}, "name")},
		{Name: "log_action", Description: "向项目日志追加一条记录", InputSchema: schema(map[string]interface{}{"project": prop("string", "项目名称"), "action": prop("string", "动作"), "summary": prop("string", "摘要"), "source": prop("string", "来源"), "tags": propArray("string", "标签")}, "project", "action", "summary")},
		{Name: "list_directory", Description: "列出目录内容", InputSchema: schema(map[string]interface{}{"path": prop("string", "目录路径")}, "path")},
		{Name: "read_file", Description: "读取文件", InputSchema: schema(map[string]interface{}{"path": prop("string", "文件路径")}, "path")},
		{Name: "write_file", Description: "写入文件", InputSchema: schema(map[string]interface{}{"path": prop("string", "文件路径"), "content": prop("string", "文件内容")}, "path", "content")},
		{Name: "list_skills", Description: "列出技能", InputSchema: schema(map[string]interface{}{})},
		{Name: "read_skill", Description: "读取技能", InputSchema: schema(map[string]interface{}{"name": prop("string", "技能名")}, "name")},
		{Name: "save_memory", Description: "保存短期记忆", InputSchema: schema(map[string]interface{}{"memories": propArray("object", "记忆数组")}, "memories")},
		{Name: "import_skill", Description: "导入 .skill 目录", InputSchema: schema(map[string]interface{}{"name": prop("string", "技能名"), "files": propObject("文件 map")}, "name", "files")},
		{Name: "create_sync_token", Description: "生成短命 sync token", InputSchema: schema(map[string]interface{}{"purpose": prop("string", "用途说明"), "access": prop("string", "push/pull/both"), "ttl_minutes": prop("integer", "TTL 分钟")}, "purpose")},
		{Name: "create_skills_import_token", Description: "生成短命 skills zip 上传 token", InputSchema: schema(map[string]interface{}{"purpose": prop("string", "用途说明"), "platform": prop("string", "来源平台"), "ttl_minutes": prop("integer", "TTL 分钟")}, "purpose")},
	}
	if len(s.Scopes) > 0 && !models.HasScope(s.Scopes, models.ScopeAdmin) {
		filtered := make([]mcp.MCPTool, 0, len(tools))
		for _, tool := range tools {
			if s.toolAllowed(tool.Name) {
				filtered = append(filtered, tool)
			}
		}
		return filtered
	}
	return tools
}

func (s *Server) toolAllowed(name string) bool {
	scopeMap := map[string]string{
		"read_profile":      models.ScopeReadProfile,
		"update_profile":    models.ScopeWriteProfile,
		"search_memory":     models.ScopeReadMemory,
		"list_projects":     models.ScopeReadProjects,
		"create_project":    models.ScopeWriteProjects,
		"get_project":       models.ScopeReadProjects,
		"log_action":        models.ScopeWriteProjects,
		"list_directory":    models.ScopeReadTree,
		"read_file":         models.ScopeReadTree,
		"write_file":        models.ScopeWriteTree,
		"list_skills":       models.ScopeReadSkills,
		"read_skill":        models.ScopeReadSkills,
		"save_memory":       models.ScopeWriteMemory,
		"import_skill":      models.ScopeWriteSkills,
		"create_sync_token": models.ScopeWriteBundle,
	}
	required, ok := scopeMap[name]
	if !ok {
		return false
	}
	return models.HasScope(s.Scopes, required)
}

func (s *Server) callTool(params mcp.ToolCallParams) (string, bool) {
	ctx := context.Background()
	if len(s.Scopes) > 0 && !models.HasScope(s.Scopes, models.ScopeAdmin) && !s.toolAllowed(params.Name) {
		return fmt.Sprintf("error: tool %q not allowed by current scopes", params.Name), true
	}
	switch params.Name {
	case "read_profile":
		category, _ := params.Arguments["category"].(string)
		profiles, err := s.Store.GetProfiles(ctx, s.UserID)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		if category != "" {
			for _, profile := range profiles {
				if profile.Category == category {
					return profile.Content, false
				}
			}
			return fmt.Sprintf("category %q not found", category), true
		}
		var builder strings.Builder
		for _, profile := range profiles {
			builder.WriteString("## " + profile.Category + "\n" + profile.Content + "\n\n")
		}
		return strings.TrimSpace(builder.String()), false
	case "update_profile":
		category, _ := params.Arguments["category"].(string)
		content, _ := params.Arguments["content"].(string)
		if err := s.Store.UpsertProfile(ctx, s.UserID, category, content, "mcp"); err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return "profile updated", false
	case "search_memory":
		query, _ := params.Arguments["query"].(string)
		results, err := s.Store.Search(ctx, s.UserID, query, s.TrustLevel, "/")
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		payload := make([]map[string]interface{}, 0, len(results))
		for _, entry := range results {
			payload = append(payload, map[string]interface{}{
				"type":    entry.Kind,
				"path":    hubpath.NormalizePublic(entry.Path),
				"snippet": snippetText(entry.Content),
			})
		}
		data, _ := json.MarshalIndent(payload, "", "  ")
		return string(data), false
	case "list_projects":
		projects, err := s.Store.ListProjects(ctx, s.UserID)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		data, _ := json.MarshalIndent(projects, "", "  ")
		return string(data), false
	case "create_project":
		name, _ := params.Arguments["name"].(string)
		project, err := s.Store.CreateProject(ctx, s.UserID, name)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		data, _ := json.MarshalIndent(project, "", "  ")
		return string(data), false
	case "get_project":
		name, _ := params.Arguments["name"].(string)
		project, err := s.Store.GetProject(ctx, s.UserID, name)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		logs, _ := s.Store.GetProjectLogs(ctx, s.UserID, name, 20)
		data, _ := json.MarshalIndent(map[string]interface{}{"project": project, "recent_logs": logs}, "", "  ")
		return string(data), false
	case "log_action":
		projectName, _ := params.Arguments["project"].(string)
		action, _ := params.Arguments["action"].(string)
		summary, _ := params.Arguments["summary"].(string)
		source, _ := params.Arguments["source"].(string)
		if source == "" {
			source = "mcp"
		}
		if err := s.Store.AppendProjectLog(ctx, s.UserID, projectName, models.ProjectLog{
			ProjectID: uuid.NewSHA1(uuid.NameSpaceURL, []byte("local-project:"+projectName)),
			Source:    source,
			Role:      "assistant",
			Action:    action,
			Summary:   summary,
			Tags:      toStringSlice(params.Arguments["tags"]),
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return "log entry added", false
	case "list_directory":
		rawPath, _ := params.Arguments["path"].(string)
		entries, err := s.Store.List(ctx, s.UserID, rawPath, s.TrustLevel)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		for i := range entries {
			entries[i].Path = hubpath.NormalizePublic(entries[i].Path)
		}
		data, _ := json.MarshalIndent(entries, "", "  ")
		return string(data), false
	case "read_file":
		rawPath, _ := params.Arguments["path"].(string)
		entry, err := s.Store.Read(ctx, s.UserID, rawPath, s.TrustLevel)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return entry.Content, false
	case "write_file":
		rawPath, _ := params.Arguments["path"].(string)
		content, _ := params.Arguments["content"].(string)
		if _, err := s.Store.WriteEntry(ctx, s.UserID, rawPath, content, "text/markdown", models.FileTreeWriteOptions{MinTrustLevel: s.TrustLevel}); err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return "file written", false
	case "list_skills":
		skills, err := s.Store.ListSkillSummaries(ctx, s.UserID, s.TrustLevel)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		data, _ := json.MarshalIndent(skills, "", "  ")
		return string(data), false
	case "read_skill":
		name, _ := params.Arguments["name"].(string)
		entry, err := s.Store.Read(ctx, s.UserID, fmt.Sprintf("/skills/%s/SKILL.md", name), s.TrustLevel)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		return entry.Content, false
	case "save_memory":
		memories, ok := params.Arguments["memories"].([]interface{})
		if !ok || len(memories) == 0 {
			return "error: memories array is required", true
		}
		saved := []string{}
		for _, item := range memories {
			memory, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			content, _ := memory["content"].(string)
			title, _ := memory["title"].(string)
			entry, err := s.Store.WriteScratchWithTitle(ctx, s.UserID, content, "mcp", title)
			if err == nil && entry != nil {
				saved = append(saved, hubpath.NormalizePublic(entry.Path))
			}
		}
		if len(saved) == 0 {
			return "error: no valid memory items to save", true
		}
		return fmt.Sprintf("saved %d memories: %s", len(saved), strings.Join(saved, ", ")), false
	case "import_skill":
		name, _ := params.Arguments["name"].(string)
		filesRaw, _ := params.Arguments["files"].(map[string]interface{})
		count := 0
		for relPath, raw := range filesRaw {
			content, _ := raw.(string)
			if _, err := s.Store.WriteEntry(ctx, s.UserID, path.Join("/skills", name, relPath), content, "text/markdown", models.FileTreeWriteOptions{MinTrustLevel: models.TrustLevelGuest}); err != nil {
				return fmt.Sprintf("error: %v", err), true
			}
			count++
		}
		return fmt.Sprintf("imported %d files for skill %q", count, name), false
	case "create_sync_token":
		purpose, _ := params.Arguments["purpose"].(string)
		if strings.TrimSpace(purpose) == "" {
			return "error: purpose is required", true
		}
		access, _ := params.Arguments["access"].(string)
		access = strings.ToLower(strings.TrimSpace(access))
		if access == "" {
			access = "push"
		}
		ttlMinutes := 30
		if raw, ok := params.Arguments["ttl_minutes"].(float64); ok {
			ttlMinutes = int(raw)
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
			return "error: access must be push, pull, both", true
		}
		resp, err := s.Store.CreateToken(ctx, s.UserID, "sync:"+purpose, scopes, models.TrustLevelWork, time.Duration(ttlMinutes)*time.Minute)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		payload, _ := json.MarshalIndent(map[string]interface{}{
			"token":      resp.Token,
			"expires_at": resp.ScopedToken.ExpiresAt.Format(time.RFC3339),
			"api_base":   s.BaseURL,
			"scopes":     resp.ScopedToken.Scopes,
			"usage":      fmt.Sprintf("agenthub sync push --bundle backup.ahubz --token %s --api-base %s", resp.Token, s.BaseURL),
		}, "", "  ")
		return string(payload), false
	case "create_skills_import_token":
		purpose, _ := params.Arguments["purpose"].(string)
		if strings.TrimSpace(purpose) == "" {
			return "error: purpose is required", true
		}
		platform, _ := params.Arguments["platform"].(string)
		platform = strings.ToLower(strings.TrimSpace(platform))
		if platform == "" {
			platform = "claude-web"
		}
		ttlMinutes := 30
		if raw, ok := params.Arguments["ttl_minutes"].(float64); ok {
			ttlMinutes = int(raw)
		}
		if ttlMinutes < 5 || ttlMinutes > 120 {
			return "error: ttl_minutes must be between 5 and 120", true
		}
		resp, err := s.Store.CreateToken(ctx, s.UserID, "skills-import:"+purpose, []string{models.ScopeWriteSkills}, models.TrustLevelWork, time.Duration(ttlMinutes)*time.Minute)
		if err != nil {
			return fmt.Sprintf("error: %v", err), true
		}
		uploadURL := strings.TrimRight(s.BaseURL, "/") + "/agent/import/skills?platform=" + platform
		payload, _ := json.MarshalIndent(map[string]interface{}{
			"token":        resp.Token,
			"expires_at":   resp.ScopedToken.ExpiresAt.Format(time.RFC3339),
			"api_base":     s.BaseURL,
			"upload_url":   uploadURL,
			"scopes":       resp.ScopedToken.Scopes,
			"curl_example": fmt.Sprintf(`curl -f -X POST -H "Authorization: Bearer %s" -F "platform=%s" -F "file=@/mnt/user-data/outputs/agenthub-skills.zip" "%s"`, resp.Token, platform, uploadURL),
		}, "", "  ")
		return string(payload), false
	default:
		return fmt.Sprintf("unknown tool: %s", params.Name), true
	}
}

func (s *Server) readResource(uri string) (string, error) {
	pathValue := strings.TrimPrefix(uri, "agenthub://")
	if !strings.HasPrefix(pathValue, "/") {
		pathValue = "/" + pathValue
	}
	entry, err := s.Store.Read(context.Background(), s.UserID, pathValue, s.TrustLevel)
	if err != nil {
		return "", err
	}
	return entry.Content, nil
}

func prop(typeName, description string) map[string]interface{} {
	return map[string]interface{}{"type": typeName, "description": description}
}

func propArray(itemType, description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": description,
		"items": map[string]interface{}{
			"type": itemType,
		},
	}
}

func propObject(description string) map[string]interface{} {
	return map[string]interface{}{"type": "object", "description": description}
}

func schema(properties map[string]interface{}, required ...string) map[string]interface{} {
	payload := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		payload["required"] = required
	}
	return payload
}

func toStringSlice(raw interface{}) []string {
	items, _ := raw.([]interface{})
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, _ := item.(string)
		if strings.TrimSpace(text) != "" {
			result = append(result, text)
		}
	}
	return result
}

func snippetText(content string) string {
	text := strings.TrimSpace(strings.ReplaceAll(content, "\n", " "))
	if len(text) > 180 {
		return text[:180] + "..."
	}
	return text
}
