package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleAgentVaultListScopes(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeReadVault) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	trustLevel := trustLevelFromCtx(r.Context())
	scopes, err := s.VaultService.ListScopes(r.Context(), userID, trustLevel)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{"scopes": scopes})
}

func (s *Server) handleAgentVaultWrite(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeWriteVault) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	scope := chi.URLParam(r, "scope")
	var req VaultWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	minTrust := trustLevelFromCtx(r.Context())
	if req.MinTrustLevel != nil && *req.MinTrustLevel < minTrust {
		minTrust = *req.MinTrustLevel
	}
	if err := s.VaultService.Write(r.Context(), userID, scope, req.Data, req.Description, minTrust); err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{"scope": scope, "data": req.Data})
}

func (s *Server) handleAgentUpdateProfile(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeWriteProfile) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	var req struct {
		Category    string            `json:"category"`
		Content     string            `json:"content"`
		Source      string            `json:"source"`
		DisplayName string            `json:"display_name"`
		Preferences map[string]string `json:"preferences"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "agent"
	}

	if req.Category != "" {
		if err := s.MemoryService.UpsertProfile(r.Context(), userID, req.Category, req.Content, source); err != nil {
			respondInternalError(w, err)
			return
		}
	}
	for category, content := range req.Preferences {
		if err := s.MemoryService.UpsertProfile(r.Context(), userID, category, content, source); err != nil {
			respondInternalError(w, err)
			return
		}
	}
	if strings.TrimSpace(req.DisplayName) != "" {
		if err := s.MemoryService.UpsertProfile(r.Context(), userID, "display_name", req.DisplayName, source); err != nil {
			respondInternalError(w, err)
			return
		}
	}

	profile, err := s.buildAgentProfile(r.Context(), userID, "")
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, profile)
}

func (s *Server) handleAgentListProjects(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadProjects) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	projects, err := s.ProjectService.List(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, map[string]interface{}{"projects": projects})
}

func (s *Server) handleAgentCreateProject(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeWriteProjects) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		respondValidationError(w, "name", "project name is required")
		return
	}

	project, err := s.ProjectService.Create(r.Context(), userID, req.Name)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if s.WebhookService != nil {
		go s.WebhookService.Trigger(r.Context(), userID, models.EventProjectUpdate, map[string]interface{}{
			"project": project.Name,
			"action":  "created",
		})
	}
	respondCreated(w, project)
}

func (s *Server) handleAgentListSkills(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadSkills) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	trustLevel := trustLevelFromCtx(r.Context())
	skills, err := s.listSkills(r.Context(), userID, trustLevel)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, map[string]interface{}{"skills": skills})
}

func (s *Server) handleAgentGetProject(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadProjects) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	name := chi.URLParam(r, "name")
	project, err := s.ProjectService.Get(r.Context(), userID, name)
	if err != nil {
		respondNotFound(w, "project")
		return
	}
	logs, err := s.ProjectService.GetLogs(r.Context(), project.ID, 50)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, map[string]interface{}{"project": project, "logs": logs})
}

func (s *Server) handleAgentAppendProjectLog(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeWriteProjects) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	name := chi.URLParam(r, "name")
	project, err := s.ProjectService.Get(r.Context(), userID, name)
	if err != nil {
		respondNotFound(w, "project")
		return
	}

	var req struct {
		Source  string   `json:"source"`
		Action  string   `json:"action"`
		Summary string   `json:"summary"`
		Tags    []string `json:"tags,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Summary) == "" {
		respondValidationError(w, "summary", "summary is required")
		return
	}

	logEntry := models.ProjectLog{
		ProjectID: project.ID,
		Source:    req.Source,
		Action:    req.Action,
		Summary:   req.Summary,
		Tags:      req.Tags,
	}
	if err := s.ProjectService.AppendLog(r.Context(), project.ID, logEntry); err != nil {
		respondInternalError(w, err)
		return
	}
	if s.WebhookService != nil {
		go s.WebhookService.Trigger(r.Context(), userID, models.EventProjectUpdate, map[string]interface{}{
			"project": project.Name,
			"action":  req.Action,
			"summary": req.Summary,
		})
	}

	respondCreated(w, map[string]string{"status": "appended", "project": name})
}

func (s *Server) handleAgentDevicesList(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadDevices) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	devices, err := s.DeviceService.List(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, map[string]interface{}{"devices": devices})
}

func (s *Server) handleAgentArchiveInbox(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeWriteInbox) {
		return
	}

	msgID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid message ID")
		return
	}
	if err := s.InboxService.Archive(r.Context(), msgID); err != nil {
		respondNotFound(w, "message")
		return
	}
	respondOK(w, map[string]string{"status": "archived", "id": msgID.String()})
}

func (s *Server) handleAgentImportProfile(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeWriteProfile) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	var req ImportProfileV2Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid JSON body")
		return
	}

	profile := map[string]string{}
	paths := []string{}
	if req.Preferences != "" {
		profile["preferences"] = req.Preferences
		paths = append(paths, "memory/profile/preferences")
	}
	if req.Relationships != "" {
		profile["relationships"] = req.Relationships
		paths = append(paths, "memory/profile/relationships")
	}
	if req.Principles != "" {
		profile["principles"] = req.Principles
		paths = append(paths, "memory/profile/principles")
	}
	if len(profile) == 0 {
		respondValidationError(w, "preferences,relationships,principles", "at least one profile field is required")
		return
	}

	if err := s.ImportService.ImportProfile(r.Context(), userID, profile); err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, ImportResponse{
		OK: true,
		Data: ImportResponseData{
			ImportedCount: len(profile),
			Paths:         paths,
		},
	})
}

func (s *Server) handleAgentExportAll(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelFull, models.ScopeAdmin) {
		return
	}
	if s.ExportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "export service not configured")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	data, err := s.ExportService.ExportToJSON(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, data)
}
