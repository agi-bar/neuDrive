package api

import (
	"encoding/json"
	"net/http"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/go-chi/chi/v5"
)

// ---------------------------------------------------------------------------
// GPT Actions handlers — optimized for OpenAI Custom GPT action calling.
// All responses are flat JSON (no wrapping envelope) for simpler schemas.
// Auth: Bearer aht_xxxxx scoped token via apiKeyMiddleware.
// ---------------------------------------------------------------------------

// GET /gpt/profile
func (s *Server) handleGPTGetProfile(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelGuest, models.ScopeReadProfile) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	user, err := s.UserService.GetByID(r.Context(), userID)
	if err != nil {
		respondNotFound(w, "user")
		return
	}

	respondOK(w, map[string]interface{}{
		"slug":         user.Slug,
		"display_name": user.DisplayName,
		"timezone":     user.Timezone,
		"language":     user.Language,
	})
}

// GET /gpt/preferences
func (s *Server) handleGPTGetPreferences(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelGuest, models.ScopeReadProfile) {
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	user, err := s.UserService.GetByID(r.Context(), userID)
	if err != nil {
		respondNotFound(w, "user")
		return
	}

	respondOK(w, map[string]interface{}{
		"timezone": user.Timezone,
		"language": user.Language,
	})
}

// POST /gpt/search — { "query": "..." }
func (s *Server) handleGPTSearch(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeSearch) {
		return
	}

	var body struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Query == "" {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "missing query")
		return
	}

	respondOK(w, map[string]interface{}{
		"query":   body.Query,
		"results": []interface{}{},
	})
}

// GET /gpt/projects
func (s *Server) handleGPTListProjects(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadProjects) {
		return
	}

	respondOK(w, map[string]interface{}{
		"projects": []interface{}{},
	})
}

// GET /gpt/project/{name}
func (s *Server) handleGPTGetProject(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadProjects) {
		return
	}

	name := chi.URLParam(r, "name")
	respondOK(w, map[string]interface{}{
		"name": name,
		"logs": []interface{}{},
	})
}

// POST /gpt/log — { "project": "...", "action": "...", "summary": "..." }
func (s *Server) handleGPTLog(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeWriteProjects) {
		return
	}

	var body struct {
		Project string `json:"project"`
		Action  string `json:"action"`
		Summary string `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Project == "" {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "missing project")
		return
	}

	respondOK(w, map[string]string{"status": "logged"})
}

// GET /gpt/skills
func (s *Server) handleGPTListSkills(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadSkills) {
		return
	}

	respondOK(w, map[string]interface{}{
		"skills": []interface{}{},
	})
}

// GET /gpt/skill/{name}
func (s *Server) handleGPTGetSkill(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadSkills) {
		return
	}

	name := chi.URLParam(r, "name")
	respondOK(w, map[string]interface{}{
		"name":    name,
		"content": "",
	})
}

// GET /gpt/devices
func (s *Server) handleGPTListDevices(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadDevices) {
		return
	}

	respondOK(w, map[string]interface{}{
		"devices": []interface{}{},
	})
}

// POST /gpt/device/{name} — { "action": "...", "params": {...} }
func (s *Server) handleGPTCallDevice(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeCallDevices) {
		return
	}

	name := chi.URLParam(r, "name")

	var body struct {
		Action string                 `json:"action"`
		Params map[string]interface{} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Action == "" {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "missing action")
		return
	}

	respondOK(w, map[string]interface{}{
		"device": name,
		"action": body.Action,
		"status": "dispatched",
	})
}

// POST /gpt/message — { "to": "...", "subject": "...", "body": "..." }
func (s *Server) handleGPTSendMessage(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeWriteInbox) {
		return
	}

	var body struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.To == "" {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "missing to")
		return
	}

	respondOK(w, map[string]string{"status": "sent"})
}

// GET /gpt/inbox
func (s *Server) handleGPTGetInbox(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeReadInbox) {
		return
	}

	respondOK(w, map[string]interface{}{
		"messages": []interface{}{},
	})
}

// GET /gpt/secrets
func (s *Server) handleGPTListSecrets(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeReadVault) {
		return
	}

	respondOK(w, map[string]interface{}{
		"scopes": []interface{}{},
	})
}

// GET /gpt/secret/{scope}
func (s *Server) handleGPTGetSecret(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelWork, models.ScopeReadVault) {
		return
	}

	scope := chi.URLParam(r, "scope")
	respondOK(w, map[string]interface{}{
		"scope": scope,
		"data":  "",
	})
}
