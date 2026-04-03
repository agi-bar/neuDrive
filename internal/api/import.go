package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
)

// ---------------------------------------------------------------------------
// Request / Response types
// ---------------------------------------------------------------------------

// ImportSkillsRequest represents a JSON payload for skill imports.
type ImportSkillsRequest struct {
	Skills []SkillFile `json:"skills"`
}

// SkillFile represents a single skill file to import.
type SkillFile struct {
	Path        string `json:"path"` // e.g. "cyberzen-write/SKILL.md"
	Content     string `json:"content"`
	ContentType string `json:"content_type,omitempty"` // default: text/markdown
}

// ImportClaudeMemoryRequest represents a Claude memory export import.
type ImportClaudeMemoryRequest struct {
	Memories []ClaudeMemoryItem `json:"memories"`
}

// ClaudeMemoryItem represents a single memory entry from Claude.
type ClaudeMemoryItem struct {
	Content   string `json:"content"`
	Source    string `json:"source"` // "claude"
	CreatedAt string `json:"created_at,omitempty"`
}

// ImportProfileRequest represents a bulk profile update.
type ImportProfileRequest struct {
	Preferences   string `json:"preferences,omitempty"`
	Relationships string `json:"relationships,omitempty"`
	Principles    string `json:"principles,omitempty"`
}

// ImportVaultRequest represents a bulk vault secrets import.
type ImportVaultRequest struct {
	Secrets []VaultSecretImport `json:"secrets"`
}

// VaultSecretImport represents a single vault secret to import.
type VaultSecretImport struct {
	Scope         string `json:"scope"`
	Value         string `json:"value"`
	Description   string `json:"description"`
	MinTrustLevel int    `json:"min_trust_level,omitempty"` // default: 4
}

// ImportDevicesRequest represents a bulk device registration.
type ImportDevicesRequest struct {
	Devices []DeviceImport `json:"devices"`
}

// DeviceImport represents a single device to register.
type DeviceImport struct {
	Name       string                 `json:"name"`
	DeviceType string                 `json:"device_type"`
	Brand      string                 `json:"brand,omitempty"`
	Protocol   string                 `json:"protocol"`
	Endpoint   string                 `json:"endpoint"`
	SkillMD    string                 `json:"skill_md,omitempty"`
	Config     map[string]interface{} `json:"config,omitempty"`
}

// FullHubExport represents a complete Hub data export for backup/restore.
type FullHubExport struct {
	Version     string            `json:"version"`
	ExportedAt  string            `json:"exported_at"`
	User        models.User       `json:"user"`
	Profile     map[string]string `json:"profile"` // category -> content
	Skills      []SkillFile       `json:"skills"`
	Devices     []DeviceImport    `json:"devices"`
	Projects    []ProjectExport   `json:"projects"`
	VaultScopes []string          `json:"vault_scopes"` // scope names only, not values
}

// ProjectExport represents a project in an export.
type ProjectExport struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	ContextMD string `json:"context_md"`
}

// ImportResult is the standard response for all import endpoints.
type ImportResult struct {
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}

// ---------------------------------------------------------------------------
// POST /api/import/skills
// ---------------------------------------------------------------------------

// HandleImportSkills handles skill file imports via JSON or multipart zip upload.
func (s *Server) HandleImportSkills(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	contentType := r.Header.Get("Content-Type")

	var skills []SkillFile

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle zip file upload.
		extracted, err := extractSkillsFromZip(r)
		if err != nil {
			respondError(w, http.StatusBadRequest, ErrCodeBadRequest, fmt.Sprintf("failed to process zip: %v", err))
			return
		}
		skills = extracted
	} else {
		// Handle JSON payload.
		var req ImportSkillsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid JSON body")
			return
		}
		skills = req.Skills
	}

	if s.FileTreeService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "file tree service not configured")
		return
	}

	result := ImportResult{}
	for _, skill := range skills {
		if skill.Path == "" || skill.Content == "" {
			result.Skipped++
			continue
		}

		ct := skill.ContentType
		if ct == "" {
			ct = "text/markdown"
		}

		// Ensure path is under .skills/ prefix.
		path := skill.Path
		if !strings.HasPrefix(path, ".skills/") {
			path = ".skills/" + path
		}

		// Ensure parent directory exists.
		dir := filepath.Dir(path)
		if dir != "." && dir != "" {
			if err := s.FileTreeService.EnsureDirectory(r.Context(), userID, dir); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("dir %s: %v", dir, err))
				continue
			}
		}

		_, err := s.FileTreeService.Write(r.Context(), userID, path, skill.Content, ct, models.TrustLevelGuest)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("skill %s: %v", skill.Path, err))
			continue
		}
		result.Imported++
	}

	respondOK(w, result)
}

// extractSkillsFromZip reads a zip file from the multipart form field "file"
// and extracts text-based skill files from it.
func extractSkillsFromZip(r *http.Request) ([]SkillFile, error) {
	// Limit upload to 50MB.
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		return nil, fmt.Errorf("parse multipart: %w", err)
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("read form file: %w", err)
	}
	defer file.Close()

	buf, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read file data: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	var skills []SkillFile
	for _, f := range reader.File {
		// Skip directories and hidden/system files.
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.HasPrefix(filepath.Base(f.Name), ".") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		ct := "text/markdown"
		ext := strings.ToLower(filepath.Ext(f.Name))
		switch ext {
		case ".json":
			ct = "application/json"
		case ".yaml", ".yml":
			ct = "text/yaml"
		case ".txt":
			ct = "text/plain"
		}

		skills = append(skills, SkillFile{
			Path:        f.Name,
			Content:     string(content),
			ContentType: ct,
		})
	}
	return skills, nil
}

// ---------------------------------------------------------------------------
// POST /api/import/claude-memory
// ---------------------------------------------------------------------------

// HandleImportClaudeMemory imports memory entries from a Claude memory export.
func (s *Server) HandleImportClaudeMemory(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req ImportClaudeMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid JSON body")
		return
	}

	if s.MemoryService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "memory service not configured")
		return
	}

	result := ImportResult{}
	for i, mem := range req.Memories {
		if mem.Content == "" {
			result.Skipped++
			continue
		}

		source := mem.Source
		if source == "" {
			source = "claude"
		}

		// Store each memory as a profile entry under "claude-import-N" category,
		// or aggregate them under a single "claude-import" category.
		category := fmt.Sprintf("claude-import-%d", i)
		if err := s.MemoryService.UpsertProfile(r.Context(), userID, category, mem.Content, source); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("memory %d: %v", i, err))
			continue
		}
		result.Imported++
	}

	respondOK(w, result)
}

// ---------------------------------------------------------------------------
// POST /api/import/profile
// ---------------------------------------------------------------------------

// HandleImportProfile performs a bulk update of profile memory categories.
func (s *Server) HandleImportProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req ImportProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid JSON body")
		return
	}

	if s.MemoryService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "memory service not configured")
		return
	}

	result := ImportResult{}

	categories := map[string]string{
		"preferences":   req.Preferences,
		"relationships": req.Relationships,
		"principles":    req.Principles,
	}

	for category, content := range categories {
		if content == "" {
			result.Skipped++
			continue
		}
		if err := s.MemoryService.UpsertProfile(r.Context(), userID, category, content, "import"); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("profile %s: %v", category, err))
			continue
		}
		result.Imported++
	}

	respondOK(w, result)
}

// ---------------------------------------------------------------------------
// POST /api/import/vault
// ---------------------------------------------------------------------------

// HandleImportVault performs a bulk import of vault secrets.
func (s *Server) HandleImportVault(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req ImportVaultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid JSON body")
		return
	}

	if s.VaultService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "vault service not configured")
		return
	}

	result := ImportResult{}
	for _, secret := range req.Secrets {
		if secret.Scope == "" || secret.Value == "" {
			result.Skipped++
			continue
		}

		minTrust := secret.MinTrustLevel
		if minTrust <= 0 {
			minTrust = models.TrustLevelFull
		}

		if err := s.VaultService.Write(r.Context(), userID, secret.Scope, secret.Value, secret.Description, minTrust); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("vault %s: %v", secret.Scope, err))
			continue
		}
		result.Imported++
	}

	respondOK(w, result)
}

// ---------------------------------------------------------------------------
// POST /api/import/devices
// ---------------------------------------------------------------------------

// HandleImportDevices performs a bulk registration of devices.
func (s *Server) HandleImportDevices(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req ImportDevicesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid JSON body")
		return
	}

	if s.DeviceService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "device service not configured")
		return
	}

	result := ImportResult{}
	for _, dev := range req.Devices {
		if dev.Name == "" || dev.Protocol == "" || dev.Endpoint == "" {
			result.Skipped++
			continue
		}

		device := models.Device{
			Name:       dev.Name,
			DeviceType: dev.DeviceType,
			Brand:      dev.Brand,
			Protocol:   dev.Protocol,
			Endpoint:   dev.Endpoint,
			SkillMD:    dev.SkillMD,
			Config:     dev.Config,
		}

		_, err := s.DeviceService.Register(r.Context(), userID, device)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("device %s: %v", dev.Name, err))
			continue
		}
		result.Imported++
	}

	respondOK(w, result)
}

// ---------------------------------------------------------------------------
// POST /api/import/full
// ---------------------------------------------------------------------------

// HandleImportFull performs a full Hub restore from an exported JSON backup.
func (s *Server) HandleImportFull(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var export FullHubExport
	if err := json.NewDecoder(r.Body).Decode(&export); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid JSON body")
		return
	}

	result := ImportResult{}

	// Import profile entries.
	if s.MemoryService != nil {
		for category, content := range export.Profile {
			if content == "" {
				result.Skipped++
				continue
			}
			if err := s.MemoryService.UpsertProfile(r.Context(), userID, category, content, "full-import"); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("profile %s: %v", category, err))
				continue
			}
			result.Imported++
		}
	}

	// Import skills into file tree.
	if s.FileTreeService != nil {
		for _, skill := range export.Skills {
			if skill.Path == "" || skill.Content == "" {
				result.Skipped++
				continue
			}
			ct := skill.ContentType
			if ct == "" {
				ct = "text/markdown"
			}
			path := skill.Path
			if !strings.HasPrefix(path, ".skills/") {
				path = ".skills/" + path
			}
			dir := filepath.Dir(path)
			if dir != "." && dir != "" {
				_ = s.FileTreeService.EnsureDirectory(r.Context(), userID, dir)
			}
			_, err := s.FileTreeService.Write(r.Context(), userID, path, skill.Content, ct, models.TrustLevelGuest)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("skill %s: %v", skill.Path, err))
				continue
			}
			result.Imported++
		}
	}

	// Import devices.
	if s.DeviceService != nil {
		for _, dev := range export.Devices {
			if dev.Name == "" {
				result.Skipped++
				continue
			}
			device := models.Device{
				Name:       dev.Name,
				DeviceType: dev.DeviceType,
				Brand:      dev.Brand,
				Protocol:   dev.Protocol,
				Endpoint:   dev.Endpoint,
				SkillMD:    dev.SkillMD,
				Config:     dev.Config,
			}
			_, err := s.DeviceService.Register(r.Context(), userID, device)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("device %s: %v", dev.Name, err))
				continue
			}
			result.Imported++
		}
	}

	// Import projects.
	if s.ProjectService != nil {
		for _, proj := range export.Projects {
			if proj.Name == "" {
				result.Skipped++
				continue
			}
			created, err := s.ProjectService.Create(r.Context(), userID, proj.Name)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("project %s: %v", proj.Name, err))
				continue
			}
			if proj.ContextMD != "" {
				_ = s.ProjectService.UpdateContext(r.Context(), userID, created.Name, proj.ContextMD)
			}
			if proj.Status == "archived" {
				_ = s.ProjectService.Archive(r.Context(), userID, created.Name)
			}
			result.Imported++
		}
	}

	respondOK(w, result)
}

// ---------------------------------------------------------------------------
// GET /api/export/full
// ---------------------------------------------------------------------------

// HandleExportFull exports the entire Hub as JSON for data portability.
func (s *Server) HandleExportFull(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	export := FullHubExport{
		Version:    "1.0",
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Profile:    make(map[string]string),
	}

	// Export user info.
	if s.UserService != nil {
		user, err := s.UserService.GetByID(r.Context(), userID)
		if err == nil {
			export.User = *user
		}
	}

	// Export profile.
	if s.MemoryService != nil {
		profiles, err := s.MemoryService.GetProfile(r.Context(), userID)
		if err == nil {
			for _, p := range profiles {
				export.Profile[p.Category] = p.Content
			}
		}
	}

	// Export skills from file tree (everything under .skills/).
	if s.FileTreeService != nil {
		entries, err := s.FileTreeService.List(r.Context(), userID, ".skills/", models.TrustLevelFull)
		if err == nil {
			for _, e := range entries {
				if e.IsDirectory {
					continue
				}
				// Read full content for each skill file.
				full, err := s.FileTreeService.Read(r.Context(), userID, e.Path, models.TrustLevelFull)
				if err != nil {
					continue
				}
				export.Skills = append(export.Skills, SkillFile{
					Path:        strings.TrimPrefix(full.Path, ".skills/"),
					Content:     full.Content,
					ContentType: full.ContentType,
				})
			}
		}
	}

	// Export devices.
	if s.DeviceService != nil {
		devices, err := s.DeviceService.List(r.Context(), userID)
		if err == nil {
			for _, d := range devices {
				export.Devices = append(export.Devices, DeviceImport{
					Name:       d.Name,
					DeviceType: d.DeviceType,
					Brand:      d.Brand,
					Protocol:   d.Protocol,
					Endpoint:   d.Endpoint,
					SkillMD:    d.SkillMD,
					Config:     d.Config,
				})
			}
		}
	}

	// Export projects.
	if s.ProjectService != nil {
		projects, err := s.ProjectService.List(r.Context(), userID)
		if err == nil {
			for _, p := range projects {
				export.Projects = append(export.Projects, ProjectExport{
					Name:      p.Name,
					Status:    p.Status,
					ContextMD: p.ContextMD,
				})
			}
		}
	}

	// Export vault scope names (not values).
	if s.VaultService != nil {
		scopes, err := s.VaultService.ListScopes(r.Context(), userID, models.TrustLevelFull)
		if err == nil {
			for _, vs := range scopes {
				export.VaultScopes = append(export.VaultScopes, vs.Scope)
			}
		}
	}

	respondOK(w, export)
}

// ---------------------------------------------------------------------------
// POST /api/import/claude-data
// ---------------------------------------------------------------------------

// HandleImportClaudeData imports a full Claude data export zip file.
// Accepts multipart/form-data with a "file" field containing the zip.
func (s *Server) HandleImportClaudeData(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	// Parse multipart form (max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "failed to parse multipart form: "+err.Error())
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "missing 'file' field in form data")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "failed to read file: "+err.Error())
		return
	}

	result, err := s.ImportService.ImportClaudeData(r.Context(), userID, data)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "import failed: "+err.Error())
		return
	}

	respondOK(w, result)
}
