package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/go-chi/chi/v5"
)

// ---------------------------------------------------------------------------
// GPT Actions handlers — optimized for OpenAI Custom GPT action calling.
// All responses are flat JSON objects so they match the published schema.
// ---------------------------------------------------------------------------

func (s *Server) handleGPTGetProfile(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelGuest, models.ScopeReadProfile) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	user, err := s.UserService.GetByID(r.Context(), userID)
	if err != nil {
		writeGPTError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"slug":         user.Slug,
		"display_name": user.DisplayName,
		"timezone":     user.Timezone,
		"language":     user.Language,
	})
}

func (s *Server) handleGPTGetPreferences(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelGuest, models.ScopeReadProfile) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	user, err := s.UserService.GetByID(r.Context(), userID)
	if err != nil {
		writeGPTError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"timezone": user.Timezone,
		"language": user.Language,
	})
}

func (s *Server) handleGPTSearch(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeSearch) {
		return
	}

	var body struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Query) == "" {
		writeGPTError(w, http.StatusBadRequest, "missing query")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	trustLevel := trustLevelFromCtx(r.Context())
	results, err := s.searchHub(r.Context(), userID, trustLevel, body.Query, "all")
	if err != nil {
		writeGPTError(w, http.StatusInternalServerError, "search failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   body.Query,
		"results": results,
	})
}

func (s *Server) handleGPTListProjects(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadProjects) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	projects, err := s.ProjectService.List(r.Context(), userID)
	if err != nil {
		writeGPTError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	items := make([]map[string]interface{}, 0, len(projects))
	for _, project := range projects {
		items = append(items, map[string]interface{}{
			"name":       project.Name,
			"status":     project.Status,
			"updated_at": project.UpdatedAt.Format(timeLayoutRFC3339),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"projects": items})
}

func (s *Server) handleGPTGetProject(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadProjects) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	name := chi.URLParam(r, "name")
	project, err := s.ProjectService.Get(r.Context(), userID, name)
	if err != nil {
		writeGPTError(w, http.StatusNotFound, "project not found")
		return
	}
	logs, err := s.ProjectService.GetLogs(r.Context(), project.ID, 50)
	if err != nil {
		writeGPTError(w, http.StatusInternalServerError, "failed to load project logs")
		return
	}

	items := make([]map[string]interface{}, 0, len(logs))
	for _, logEntry := range logs {
		items = append(items, map[string]interface{}{
			"action":    logEntry.Action,
			"summary":   logEntry.Summary,
			"timestamp": logEntry.CreatedAt.Format(timeLayoutRFC3339),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name": project.Name,
		"logs": items,
	})
}

func (s *Server) handleGPTLog(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeWriteProjects) {
		return
	}

	var body struct {
		Project string `json:"project"`
		Action  string `json:"action"`
		Summary string `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Project) == "" {
		writeGPTError(w, http.StatusBadRequest, "missing project")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	project, err := s.ProjectService.Get(r.Context(), userID, body.Project)
	if err != nil {
		project, err = s.ProjectService.Create(r.Context(), userID, body.Project)
		if err != nil {
			writeGPTError(w, http.StatusInternalServerError, "failed to create project")
			return
		}
	}

	logEntry := models.ProjectLog{
		ProjectID: project.ID,
		Source:    "gpt",
		Action:    body.Action,
		Summary:   body.Summary,
	}
	if err := s.ProjectService.AppendLog(r.Context(), project.ID, logEntry); err != nil {
		writeGPTError(w, http.StatusInternalServerError, "failed to write log")
		return
	}
	if s.WebhookService != nil {
		go s.WebhookService.Trigger(context.Background(), userID, models.EventProjectUpdate, map[string]interface{}{
			"project": project.Name,
			"action":  body.Action,
			"summary": body.Summary,
		})
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged"})
}

func (s *Server) handleGPTListSkills(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadSkills) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	trustLevel := trustLevelFromCtx(r.Context())
	skills, err := s.listSkills(r.Context(), userID, trustLevel)
	if err != nil {
		writeGPTError(w, http.StatusInternalServerError, "failed to list skills")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"skills": skills})
}

func (s *Server) handleGPTGetSkill(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadSkills) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	trustLevel := trustLevelFromCtx(r.Context())
	name := chi.URLParam(r, "name")
	entry, err := s.FileTreeService.Read(r.Context(), userID, "/skills/"+name+"/SKILL.md", trustLevel)
	if err != nil {
		writeGPTError(w, http.StatusNotFound, "skill not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":    name,
		"content": entry.Content,
	})
}

func (s *Server) handleGPTListDevices(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadDevices) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	devices, err := s.DeviceService.List(r.Context(), userID)
	if err != nil {
		writeGPTError(w, http.StatusInternalServerError, "failed to list devices")
		return
	}

	items := make([]map[string]interface{}, 0, len(devices))
	for _, device := range devices {
		items = append(items, map[string]interface{}{
			"name":   device.Name,
			"type":   device.DeviceType,
			"status": device.Status,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": items})
}

func (s *Server) handleGPTCallDevice(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeCallDevices) {
		return
	}

	name := chi.URLParam(r, "name")
	var body struct {
		Action string                 `json:"action"`
		Params map[string]interface{} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Action) == "" {
		writeGPTError(w, http.StatusBadRequest, "missing action")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	result, err := s.DeviceService.Call(r.Context(), userID, name, body.Action, body.Params)
	if err != nil {
		writeGPTDeviceError(w, err)
		return
	}

	status, _ := result["status"].(string)
	if status == "" {
		status = "ok"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device": name,
		"action": body.Action,
		"status": status,
	})
}

func (s *Server) handleGPTSendMessage(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeWriteInbox) {
		return
	}

	var body struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.To) == "" {
		writeGPTError(w, http.StatusBadRequest, "missing to")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	msg := models.InboxMessage{
		FromAddress: "assistant@" + userID.String(),
		ToAddress:   body.To,
		Subject:     body.Subject,
		Body:        body.Body,
		Priority:    "normal",
	}
	if _, err := s.InboxService.Send(r.Context(), userID, msg); err != nil {
		writeGPTError(w, http.StatusInternalServerError, "failed to send message")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *Server) handleGPTGetInbox(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadInbox) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	messages, err := s.InboxService.GetMessages(r.Context(), userID, "", "incoming")
	if err != nil {
		writeGPTError(w, http.StatusInternalServerError, "failed to load inbox")
		return
	}

	items := make([]map[string]interface{}, 0, len(messages))
	for _, message := range messages {
		items = append(items, map[string]interface{}{
			"from":      message.FromAddress,
			"subject":   message.Subject,
			"body":      message.Body,
			"timestamp": message.CreatedAt.Format(timeLayoutRFC3339),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"messages": items})
}

func (s *Server) handleGPTListSecrets(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeReadVault) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	trustLevel := trustLevelFromCtx(r.Context())
	scopes, err := s.VaultService.ListScopes(r.Context(), userID, trustLevel)
	if err != nil {
		writeGPTError(w, http.StatusInternalServerError, "failed to list secrets")
		return
	}

	items := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		items = append(items, scope.Scope)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"scopes": items})
}

func (s *Server) handleGPTGetSecret(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeReadVault) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	trustLevel := trustLevelFromCtx(r.Context())
	scope := chi.URLParam(r, "scope")
	data, err := s.VaultService.Read(r.Context(), userID, scope, trustLevel)
	if err != nil {
		writeGPTError(w, http.StatusNotFound, "secret not found")
		return
	}
	if s.WebhookService != nil {
		go s.WebhookService.Trigger(context.Background(), userID, models.EventVaultAccess, map[string]interface{}{
			"scope":       scope,
			"trust_level": trustLevel,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"scope": scope,
		"data":  data,
	})
}

func writeGPTError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeGPTDeviceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrDeviceInvalidRequest):
		writeGPTError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, services.ErrDeviceNotFound):
		writeGPTError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, services.ErrDeviceUnsupported):
		writeGPTError(w, http.StatusNotImplemented, err.Error())
	case errors.Is(err, services.ErrDeviceUpstreamFailed):
		writeGPTError(w, http.StatusBadGateway, err.Error())
	default:
		writeGPTError(w, http.StatusInternalServerError, "device call failed")
	}
}

const timeLayoutRFC3339 = time.RFC3339

// handleGPTOpenAPISchema serves the GPT Actions OpenAPI schema as JSON,
// dynamically setting the server URL to match the incoming request host.
func (s *Server) handleGPTOpenAPISchema(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if r.TLS == nil && !strings.HasPrefix(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "http"
	}
	baseURL := scheme + "://" + r.Host

	schema := map[string]interface{}{
		"openapi": "3.1.0",
		"info": map[string]string{
			"title":       "Agent Hub",
			"description": "连接你的 AI 身份和记忆到 ChatGPT",
			"version":     "1.0.0",
		},
		"servers": []map[string]string{{"url": baseURL}},
		"paths":   gptOpenAPIPaths(),
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{},
			"securitySchemes": map[string]interface{}{
				"BearerAuth": map[string]string{
					"type":        "http",
					"scheme":      "bearer",
					"description": "Agent Hub Token (aht_...)",
				},
			},
		},
		"security": []map[string][]string{{"BearerAuth": {}}},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(schema)
}

func gptOpenAPIPaths() map[string]interface{} {
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
		"/gpt/profile":     get("getProfile", "获取用户身份信息", obj(M{"slug": str, "display_name": str, "timezone": str, "language": str})),
		"/gpt/preferences": get("getPreferences", "获取用户偏好", obj(M{"timezone": str, "language": str})),
		"/gpt/projects":    get("listProjects", "列出所有项目", obj(M{"projects": arr(obj(M{"name": str, "status": str, "updated_at": str}))})),
		"/gpt/skills":      get("listSkills", "列出所有技能", obj(M{"skills": arr(obj(M{"name": str, "description": str}))})),
		"/gpt/devices":     get("listDevices", "列出设备", obj(M{"devices": arr(obj(M{"name": str, "type": str, "status": str}))})),
		"/gpt/inbox":       get("getInbox", "读取收件箱", obj(M{"messages": arr(obj(M{"from": str, "subject": str, "body": str, "timestamp": str}))})),
		"/gpt/secrets":     get("listSecrets", "列出保险库范围", obj(M{"scopes": arr(str)})),
		"/gpt/project/{name}": M{"get": M{
			"operationId": "getProject", "summary": "获取项目详情",
			"parameters": []M{pp("name")},
			"responses":  rsp(obj(M{"name": str, "logs": arr(obj(M{"action": str, "summary": str, "timestamp": str}))})),
		}},
		"/gpt/skill/{name}": M{"get": M{
			"operationId": "getSkill", "summary": "读取技能内容",
			"parameters": []M{pp("name")},
			"responses":  rsp(obj(M{"name": str, "content": str})),
		}},
		"/gpt/secret/{scope}": M{"get": M{
			"operationId": "getSecret", "summary": "读取保险库条目",
			"parameters": []M{pp("scope")},
			"responses":  rsp(obj(M{"scope": str, "data": str})),
		}},
		"/gpt/device/{name}": M{"post": M{
			"operationId": "callDevice", "summary": "调用设备",
			"parameters":  []M{pp("name")},
			"requestBody": body([]string{"action"}, M{"action": str, "params": M{"type": "object"}}),
			"responses":   rsp(obj(M{"device": str, "action": str, "status": str})),
		}},
		"/gpt/search": M{"post": M{
			"operationId": "searchMemory", "summary": "搜索记忆和知识库",
			"requestBody": body([]string{"query"}, M{"query": str}),
			"responses":   rsp(obj(M{"query": str, "results": arr(obj(M{"path": str, "snippet": str, "score": num}))})),
		}},
		"/gpt/log": M{"post": M{
			"operationId": "logAction", "summary": "记录项目日志",
			"requestBody": body([]string{"project"}, M{"project": str, "action": str, "summary": str}),
			"responses":   rsp(obj(M{"status": str})),
		}},
		"/gpt/message": M{"post": M{
			"operationId": "sendMessage", "summary": "发送消息",
			"requestBody": body([]string{"to"}, M{"to": str, "subject": str, "body": str}),
			"responses":   rsp(obj(M{"status": str})),
		}},
	}
}
