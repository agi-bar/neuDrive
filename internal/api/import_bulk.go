package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Request types for bulk import endpoints
// ---------------------------------------------------------------------------

// ImportSkillRequest represents a JSON payload for a single skill import.
type ImportSkillRequest struct {
	Name  string            `json:"name"`
	Files map[string]string `json:"files"` // relativePath -> content
}

// ImportClaudeMemoryV2Request represents a Claude memory export with typed entries.
type ImportClaudeMemoryV2Request struct {
	Memories []ClaudeMemoryV2Item `json:"memories"`
}

// ClaudeMemoryV2Item represents a single typed memory entry from Claude.
type ClaudeMemoryV2Item struct {
	Content   string `json:"content"`
	Type      string `json:"type"`                // preference, relationship, project
	CreatedAt string `json:"created_at,omitempty"`
}

// ImportProfileV2Request represents a profile import with arbitrary categories.
type ImportProfileV2Request struct {
	Preferences   string `json:"preferences,omitempty"`
	Relationships string `json:"relationships,omitempty"`
	Principles    string `json:"principles,omitempty"`
}

// ImportBulkRequest represents a bulk file import request.
type ImportBulkRequest struct {
	Files         map[string]string `json:"files"`           // path -> content
	MinTrustLevel int               `json:"min_trust_level"` // default: 1
}

// ImportResponse is the standard response for all new import endpoints.
type ImportResponse struct {
	OK   bool              `json:"ok"`
	Data ImportResponseData `json:"data"`
}

// ImportResponseData carries the result of an import operation.
type ImportResponseData struct {
	ImportedCount int      `json:"imported_count"`
	Paths         []string `json:"paths,omitempty"`
	Errors        []string `json:"errors,omitempty"`
}

// ---------------------------------------------------------------------------
// POST /api/import/skill
// ---------------------------------------------------------------------------

func (s *Server) handleImportSkill(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if s.ImportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "import service not configured")
		return
	}

	var req ImportSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid JSON body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, "name", "skill name is required")
		return
	}
	if len(req.Files) == 0 {
		respondValidationError(w, "files", "at least one file is required")
		return
	}

	count, err := s.ImportService.ImportSkill(r.Context(), userID, req.Name, req.Files)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	// Collect imported paths.
	paths := make([]string, 0, len(req.Files))
	for relPath := range req.Files {
		paths = append(paths, ".skills/"+req.Name+"/"+relPath)
	}

	respondOK(w, ImportResponse{
		OK: true,
		Data: ImportResponseData{
			ImportedCount: count,
			Paths:         paths,
		},
	})
}

// ---------------------------------------------------------------------------
// POST /api/import/claude-memory (v2 with typed entries)
// ---------------------------------------------------------------------------

func (s *Server) handleImportClaudeMemoryV2(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if s.ImportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "import service not configured")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB limit
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "failed to read request body")
		return
	}

	// Validate JSON structure.
	var check ImportClaudeMemoryV2Request
	if err := json.Unmarshal(body, &check); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid JSON body")
		return
	}
	if len(check.Memories) == 0 {
		respondValidationError(w, "memories", "no memories provided")
		return
	}

	count, err := s.ImportService.ImportClaudeMemory(r.Context(), userID, body)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	// Build paths to describe where things were stored.
	paths := []string{}
	for _, mem := range check.Memories {
		switch mem.Type {
		case "preference":
			paths = appendUnique(paths, "memory/profile/preferences")
		case "relationship":
			paths = appendUnique(paths, "memory/profile/relationships")
		case "project":
			paths = appendUnique(paths, "memory/scratch")
		default:
			paths = appendUnique(paths, "memory/profile/claude-misc")
		}
	}

	respondOK(w, ImportResponse{
		OK: true,
		Data: ImportResponseData{
			ImportedCount: count,
			Paths:         paths,
		},
	})
}

// ---------------------------------------------------------------------------
// POST /api/import/profile (v2)
// ---------------------------------------------------------------------------

func (s *Server) handleImportProfileV2(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if s.ImportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "import service not configured")
		return
	}

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

// ---------------------------------------------------------------------------
// POST /api/import/bulk
// ---------------------------------------------------------------------------

func (s *Server) handleImportBulk(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if s.ImportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "import service not configured")
		return
	}

	var req ImportBulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid JSON body")
		return
	}

	if len(req.Files) == 0 {
		respondValidationError(w, "files", "no files provided")
		return
	}

	minTrust := req.MinTrustLevel
	if minTrust <= 0 {
		minTrust = models.TrustLevelGuest
	}

	count, err := s.ImportService.ImportBulkFiles(r.Context(), userID, req.Files, minTrust)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	paths := make([]string, 0, len(req.Files))
	for p := range req.Files {
		paths = append(paths, p)
	}

	respondOK(w, ImportResponse{
		OK: true,
		Data: ImportResponseData{
			ImportedCount: count,
			Paths:         paths,
		},
	})
}

// ---------------------------------------------------------------------------
// GET /api/export/all
// ---------------------------------------------------------------------------

func (s *Server) handleExportAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if s.ImportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "import service not configured")
		return
	}

	data, err := s.ImportService.ExportAll(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, data)
}

// ---------------------------------------------------------------------------
// Agent API handlers for import
// ---------------------------------------------------------------------------

func (s *Server) handleAgentImportSkill(w http.ResponseWriter, r *http.Request) {
	userID, trustLevel := agentAuth(r)
	if userID == nil {
		respondUnauthorized(w)
		return
	}
	if trustLevel < models.TrustLevelWork {
		respondForbidden(w, "insufficient trust level")
		return
	}

	if s.ImportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "import service not configured")
		return
	}

	var req ImportSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid JSON body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, "name", "skill name is required")
		return
	}
	if len(req.Files) == 0 {
		respondValidationError(w, "files", "at least one file is required")
		return
	}

	count, err := s.ImportService.ImportSkill(r.Context(), *userID, req.Name, req.Files)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	paths := make([]string, 0, len(req.Files))
	for relPath := range req.Files {
		paths = append(paths, ".skills/"+req.Name+"/"+relPath)
	}

	respondOK(w, ImportResponse{
		OK: true,
		Data: ImportResponseData{
			ImportedCount: count,
			Paths:         paths,
		},
	})
}

func (s *Server) handleAgentImportClaudeMemory(w http.ResponseWriter, r *http.Request) {
	userID, trustLevel := agentAuth(r)
	if userID == nil {
		respondUnauthorized(w)
		return
	}
	if trustLevel < models.TrustLevelWork {
		respondForbidden(w, "insufficient trust level")
		return
	}

	if s.ImportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "import service not configured")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "failed to read request body")
		return
	}

	count, err := s.ImportService.ImportClaudeMemory(r.Context(), *userID, body)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, ImportResponse{
		OK: true,
		Data: ImportResponseData{
			ImportedCount: count,
		},
	})
}

func (s *Server) handleAgentImportBulk(w http.ResponseWriter, r *http.Request) {
	userID, trustLevel := agentAuth(r)
	if userID == nil {
		respondUnauthorized(w)
		return
	}
	if trustLevel < models.TrustLevelWork {
		respondForbidden(w, "insufficient trust level")
		return
	}

	if s.ImportService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "import service not configured")
		return
	}

	var req ImportBulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid JSON body")
		return
	}

	if len(req.Files) == 0 {
		respondValidationError(w, "files", "no files provided")
		return
	}

	// Agent trust level caps the min trust level of imported files.
	minTrust := req.MinTrustLevel
	if minTrust <= 0 {
		minTrust = models.TrustLevelGuest
	}
	// Do not allow agent to set trust level higher than its own.
	if minTrust > trustLevel {
		minTrust = trustLevel
	}

	count, err := s.ImportService.ImportBulkFiles(r.Context(), *userID, req.Files, minTrust)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	paths := make([]string, 0, len(req.Files))
	for p := range req.Files {
		paths = append(paths, p)
	}

	respondOK(w, ImportResponse{
		OK: true,
		Data: ImportResponseData{
			ImportedCount: count,
			Paths:         paths,
		},
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// agentAuth extracts userID and trustLevel from the request context,
// supporting both connection-based and token-based agent auth.
func agentAuth(r *http.Request) (userID *uuid.UUID, trustLevel int) {
	conn := connectionFromCtx(r.Context())
	if conn != nil {
		return &conn.UserID, conn.TrustLevel
	}

	// Fall back to token-based auth.
	token := scopedTokenFromCtx(r.Context())
	if token != nil {
		return &token.UserID, token.MaxTrustLevel
	}

	// Last resort: check if userID is in context (JWT fallback in apiKeyMiddleware).
	uid, ok := userIDFromCtx(r.Context())
	if ok {
		tl := trustLevelFromCtx(r.Context())
		return &uid, tl
	}

	return nil, 0
}

// appendUnique appends s to the slice only if it is not already present.
func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}
