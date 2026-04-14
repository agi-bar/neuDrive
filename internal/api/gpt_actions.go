package api

import (
	"encoding/json"
	"net/http"
)

// handleGPTOpenAPISchema serves the GPT Actions OpenAPI schema as JSON,
// dynamically setting the server URL to match the incoming request host.
// All actions point to /agent/* endpoints — no separate /gpt/* handlers needed.
func (s *Server) handleGPTOpenAPISchema(w http.ResponseWriter, r *http.Request) {
	baseURL := s.baseURL(r)

	schema := map[string]interface{}{
		"openapi": "3.1.0",
		"info": map[string]string{
			"title":       "neuDrive",
			"description": "连接你的 AI 身份和记忆到 ChatGPT",
			"version":     "1.0.0",
		},
		"servers": []map[string]string{{"url": baseURL}},
		"paths":   agentOpenAPIPaths(),
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{},
			"securitySchemes": map[string]interface{}{
				"BearerAuth": map[string]string{
					"type":        "http",
					"scheme":      "bearer",
					"description": "neuDrive Token (ndt_...)",
				},
			},
		},
		"security": []map[string][]string{{"BearerAuth": {}}},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(schema)
}

// agentOpenAPIPaths returns the OpenAPI paths object pointing to /agent/* endpoints.
func agentOpenAPIPaths() map[string]interface{} {
	type M = map[string]interface{}
	str := M{"type": "string"}
	num := M{"type": "number"}
	obj := func(props M) M { return M{"type": "object", "properties": props} }
	arr := func(items M) M { return M{"type": "array", "items": items} }
	rsp := func(schema M) M {
		return M{"200": M{"description": "ok", "content": M{"application/json": M{"schema": schema}}}}
	}
	get := func(id, summary string, schema M) M {
		return M{"get": M{"operationId": id, "summary": summary, "responses": rsp(schema)}}
	}
	pp := func(name string) M { return M{"name": name, "in": "path", "required": true, "schema": str} }
	body := func(required []string, props M) M {
		return M{"required": true, "content": M{"application/json": M{"schema": M{"type": "object", "required": required, "properties": props}}}}
	}

	return M{
		// Profile & Memory
		"/agent/memory/profile": M{
			"get": M{
				"operationId": "getProfile", "summary": "获取用户身份信息和偏好",
				"responses": rsp(obj(M{
					"slug": str, "display_name": str, "timezone": str, "language": str,
					"profiles": arr(obj(M{"category": str, "content": str, "source": str})),
				})),
			},
			"put": M{
				"operationId": "updateProfile", "summary": "更新用户偏好",
				"requestBody": body([]string{"category", "content"}, M{"category": str, "content": str}),
				"responses":   rsp(obj(M{"slug": str, "display_name": str})),
			},
		},

		// Search
		"/agent/search": M{"post": M{
			"operationId": "searchMemory", "summary": "搜索记忆和知识库",
			"requestBody": body([]string{"query"}, M{"query": str, "scope": str}),
			"responses":   rsp(obj(M{"query": str, "results": arr(obj(M{"path": str, "type": str, "snippet": str, "score": num}))})),
		}},

		// Projects
		"/agent/projects": get("listProjects", "列出所有项目", obj(M{
			"projects": arr(obj(M{"name": str, "status": str, "updated_at": str})),
		})),
		"/agent/projects/{name}": M{"get": M{
			"operationId": "getProject", "summary": "获取项目详情和日志",
			"parameters": []M{pp("name")},
			"responses":  rsp(obj(M{"project": obj(M{"name": str, "status": str}), "logs": arr(obj(M{"action": str, "summary": str, "created_at": str}))})),
		}},
		"/agent/projects/{name}/log": M{"post": M{
			"operationId": "logAction", "summary": "记录项目操作日志",
			"parameters":  []M{pp("name")},
			"requestBody": body([]string{"summary"}, M{"source": str, "action": str, "summary": str}),
			"responses":   rsp(obj(M{"status": str, "project": str})),
		}},

		// Skills
		"/agent/skills": get("listSkills", "列出所有技能", obj(M{
			"skills": arr(obj(M{"name": str, "path": str, "source": str, "description": str, "when_to_use": str})),
		})),

		// Tree sync
		"/agent/tree/snapshot": get("snapshotTree", "获取虚拟树快照", obj(M{
			"path": str, "cursor": num, "root_checksum": str, "entries": arr(obj(M{"path": str, "kind": str, "checksum": str, "version": num})),
		})),
		"/agent/tree/changes": get("treeChanges", "获取虚拟树增量变更", obj(M{
			"path": str, "from_cursor": num, "next_cursor": num,
			"changes": arr(obj(M{"cursor": num, "change_type": str, "entry": obj(M{"path": str, "kind": str, "checksum": str, "version": num})})),
		})),

		// Vault
		"/agent/vault/scopes": get("listSecrets", "列出保险库范围", obj(M{
			"scopes": arr(obj(M{"scope": str, "description": str})),
		})),
		"/agent/vault/{scope}": M{"get": M{
			"operationId": "getSecret", "summary": "读取保险库条目",
			"parameters": []M{pp("scope")},
			"responses":  rsp(obj(M{"scope": str, "data": str})),
		}},

		// Dashboard
		"/agent/dashboard/stats": get("getStats", "获取 Hub 统计概览", obj(M{
			"connections": num, "skills": num, "projects": num,
		})),
	}
}
