package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
)

const (
	agentSkillsUploadMaxZipBytes = 64 * 1024
	agentSkillsUploadMaxZipLabel = "64 KB"
)

func (s *Server) handleAgentWriteScratch(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelCollaborate, models.ScopeWriteMemory) {
		return
	}
	if s.MemoryService == nil {
		respondNotConfigured(w, "memory service")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	var req struct {
		Content string `json:"content"`
		Source  string `json:"source"`
		Title   string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		respondValidationError(w, "content", "content is required")
		return
	}
	ctx := s.requestSourceContext(r, "agent")
	if strings.TrimSpace(req.Source) == "" {
		req.Source = services.SourceOrDefault(ctx, "agent")
	}

	entry, err := s.MemoryService.WriteScratchWithTitle(ctx, userID, req.Content, req.Source, req.Title)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondCreatedWithLocalGitSync(w, ImportResponse{
		OK: true,
		Data: ImportResponseData{
			ImportedCount: 1,
			Paths:         []string{strings.TrimPrefix(entry.Path, "/")},
		},
	}, s.syncLocalGitMirror(r.Context(), userID))
}

func (s *Server) handleAgentCreateEphemeralToken(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelFull, models.ScopeAdmin) {
		return
	}
	if s.TokenService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "token service not configured")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	var req struct {
		Kind       string `json:"kind"`
		Purpose    string `json:"purpose"`
		Access     string `json:"access"`
		Platform   string `json:"platform"`
		TTLMinutes int    `json:"ttl_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	kind := strings.TrimSpace(strings.ToLower(req.Kind))
	purpose := strings.TrimSpace(req.Purpose)
	if kind == "" {
		respondValidationError(w, "kind", "kind is required")
		return
	}
	if purpose == "" {
		respondValidationError(w, "purpose", "purpose is required")
		return
	}
	ttlMinutes := req.TTLMinutes
	if ttlMinutes <= 0 {
		ttlMinutes = 30
	}
	if ttlMinutes < 5 || ttlMinutes > 120 {
		respondValidationError(w, "ttl_minutes", "ttl_minutes must be between 5 and 120")
		return
	}

	baseURL := requestBaseURL(r)
	switch kind {
	case "sync":
		access := strings.TrimSpace(strings.ToLower(req.Access))
		if access == "" {
			access = "push"
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
			respondValidationError(w, "access", "access must be push, pull, or both")
			return
		}
		created, err := s.TokenService.CreateEphemeralToken(r.Context(), userID, "sync:"+purpose, scopes, models.TrustLevelWork, time.Duration(ttlMinutes)*time.Minute)
		if err != nil {
			respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
			return
		}
		respondCreated(w, map[string]any{
			"token":      created.Token,
			"expires_at": created.ScopedToken.ExpiresAt.Format(time.RFC3339),
			"api_base":   baseURL,
			"scopes":     created.ScopedToken.Scopes,
			"usage":      fmt.Sprintf("neudrive login --api-base %s --token %s && neudrive sync push --bundle backup.ndrv", baseURL, created.Token),
		})
		return

	case "skills-upload":
		platform := strings.TrimSpace(strings.ToLower(req.Platform))
		if platform == "" {
			platform = "claude-web"
		}
		created, err := s.TokenService.CreateEphemeralToken(r.Context(), userID, "skills-import:"+purpose, []string{models.ScopeWriteSkills}, models.TrustLevelWork, time.Duration(ttlMinutes)*time.Minute)
		if err != nil {
			respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
			return
		}
		uploadURL := strings.TrimRight(baseURL, "/") + "/agent/import/skills?platform=" + platform
		probeURL := strings.TrimRight(baseURL, "/") + "/test/post"
		browserUploadURL := strings.TrimRight(baseURL, "/") + "/import/skills?token=" + url.QueryEscape(created.Token) + "&platform=" + url.QueryEscape(platform)
		respondCreated(w, map[string]any{
			"token":                        created.Token,
			"expires_at":                   created.ScopedToken.ExpiresAt.Format(time.RFC3339),
			"api_base":                     baseURL,
			"upload_url":                   uploadURL,
			"browser_upload_url":           browserUploadURL,
			"connectivity_probe_url":       probeURL,
			"connectivity_probe_method":    http.MethodPost,
			"scopes":                       created.ScopedToken.Scopes,
			"usage":                        "Probe connectivity first, then upload the skills zip directly from the sandbox or browser.",
			"warning":                      agentSkillsUploadWarning(),
			"curl_example":                 fmt.Sprintf(`curl -f -X POST -H "Authorization: Bearer %s" -F "platform=%s" -F "file=@/mnt/user-data/outputs/neudrive-skills.zip" "%s"`, created.Token, platform, uploadURL),
			"inline_archive_max_zip_bytes": agentSkillsUploadMaxZipBytes,
		})
		return

	default:
		respondValidationError(w, "kind", "kind must be sync or skills-upload")
	}
}

func agentSkillsUploadWarning() string {
	return fmt.Sprintf(
		"For Claude Web, if a skills zip is over %s (%d bytes) or its size is unknown, do not inline it into one tool call. Use the prepared upload flow instead.",
		agentSkillsUploadMaxZipLabel,
		agentSkillsUploadMaxZipBytes,
	)
}
