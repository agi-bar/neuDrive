package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/skillsarchive"
	sqlitestorage "github.com/agi-bar/neudrive/internal/storage/sqlite"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type localPlatformImportRequest struct {
	Platform     string                            `json:"platform"`
	Sources      []sqlitestorage.Source            `json:"sources,omitempty"`
	AgentPayload *sqlitestorage.AgentExportPayload `json:"agent_payload,omitempty"`
}

type localPlatformImportResponse struct {
	Files *sqlitestorage.ImportResult      `json:"files,omitempty"`
	Agent *sqlitestorage.AgentImportResult `json:"agent,omitempty"`
}

type localPlatformExportRequest struct {
	Platform   string `json:"platform"`
	OutputRoot string `json:"output_root"`
}

type localPlatformTokenRequest struct {
	Platform   string `json:"platform"`
	TrustLevel int    `json:"trust_level,omitempty"`
}

type localPlatformSkillsArchiveRequest struct {
	Platform    string `json:"platform"`
	ArchivePath string `json:"archive_path"`
}

func (s *Server) ensureLocalPlatformMode(w http.ResponseWriter) bool {
	if s.isLocalMode() {
		return true
	}
	respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "platform local filesystem operations are only available in local mode")
	return false
}

func (s *Server) handleAgentCreateLocalPlatformToken(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelFull, models.ScopeAdmin) {
		return
	}
	if !s.ensureLocalPlatformMode(w) {
		return
	}
	if s.TokenService == nil {
		respondNotConfigured(w, "token service")
		return
	}

	var req localPlatformTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	scopes := make([]string, 0, len(models.AllScopes)-1)
	for _, scope := range models.AllScopes {
		if scope == models.ScopeAdmin {
			continue
		}
		scopes = append(scopes, scope)
	}
	trustLevel := req.TrustLevel
	if trustLevel <= 0 {
		trustLevel = models.TrustLevelWork
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "platform"
	}
	userID, _ := userIDFromCtx(r.Context())
	tokenResp, err := s.TokenService.CreateToken(r.Context(), userID, models.CreateTokenRequest{
		Name:          "local platform " + platform,
		Scopes:        scopes,
		MaxTrustLevel: trustLevel,
		ExpiresInDays: 365,
	})
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondCreated(w, tokenResp)
}

func (s *Server) handleAgentRevokeLocalPlatformToken(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelFull, models.ScopeAdmin) {
		return
	}
	if !s.ensureLocalPlatformMode(w) {
		return
	}
	if s.TokenService == nil {
		respondNotConfigured(w, "token service")
		return
	}

	tokenID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "id")))
	if err != nil {
		respondValidationError(w, "id", "token id must be a valid UUID")
		return
	}
	userID, _ := userIDFromCtx(r.Context())
	if err := s.TokenService.RevokeToken(r.Context(), userID, tokenID); err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, map[string]string{"status": "revoked", "id": tokenID.String()})
}

func (s *Server) handleAgentImportLocalPlatformData(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelFull, models.ScopeAdmin) {
		return
	}
	if !s.ensureLocalPlatformMode(w) {
		return
	}
	if s.FileTreeService == nil || s.MemoryService == nil || s.ProjectService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "platform import services not configured")
		return
	}

	var req localPlatformImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	if req.Sources == nil && req.AgentPayload == nil {
		respondValidationError(w, "sources", "at least one import source or agent payload is required")
		return
	}

	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		respondValidationError(w, "platform", "platform is required")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	resp := &localPlatformImportResponse{}
	var err error
	if req.AgentPayload != nil {
		resp.Agent, err = s.importLocalPlatformAgentPayload(r.Context(), userID, platform, *req.AgentPayload)
		if err != nil {
			respondInternalError(w, err)
			return
		}
	}
	if req.Sources != nil {
		resp.Files, err = s.importLocalPlatformSources(r.Context(), userID, platform, req.Sources)
		if err != nil {
			respondInternalError(w, err)
			return
		}
	}

	respondOKWithLocalGitSync(w, resp, s.syncLocalGitMirror(r.Context(), userID))
}

func (s *Server) handleAgentExportLocalPlatformData(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelFull, models.ScopeAdmin) {
		return
	}
	if !s.ensureLocalPlatformMode(w) {
		return
	}
	if s.FileTreeService == nil {
		respondNotConfigured(w, "file tree service")
		return
	}

	var req localPlatformExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		respondValidationError(w, "platform", "platform is required")
		return
	}
	outputRoot := strings.TrimSpace(req.OutputRoot)
	if outputRoot == "" {
		respondValidationError(w, "output_root", "output_root is required")
		return
	}

	userID, _ := userIDFromCtx(r.Context())
	result, err := s.exportLocalPlatformSnapshot(r.Context(), userID, platform, outputRoot)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, result)
}

func (s *Server) handleAgentImportLocalSkillsArchive(w http.ResponseWriter, r *http.Request) {
	if !s.agentCheckAuth(w, r, models.TrustLevelFull, models.ScopeAdmin) {
		return
	}
	if !s.ensureLocalPlatformMode(w) {
		return
	}
	if s.FileTreeService == nil {
		respondNotConfigured(w, "file tree service")
		return
	}

	var req localPlatformSkillsArchiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	archivePath := strings.TrimSpace(req.ArchivePath)
	if archivePath == "" {
		respondValidationError(w, "archive_path", "archive_path is required")
		return
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "claude-web"
	}

	userID, _ := userIDFromCtx(r.Context())
	result, err := s.importLocalSkillsArchive(r.Context(), userID, platform, archivePath)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOKWithLocalGitSync(w, result, s.syncLocalGitMirror(r.Context(), userID))
}

func (s *Server) importLocalPlatformSources(ctx context.Context, userID uuid.UUID, platform string, sources []sqlitestorage.Source) (*sqlitestorage.ImportResult, error) {
	result := &sqlitestorage.ImportResult{Platform: platform}
	for _, source := range sources {
		if strings.TrimSpace(source.Path) == "" {
			continue
		}
		info, err := os.Stat(source.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if info.IsDir() {
			err = filepath.WalkDir(source.Path, func(pathValue string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					if pathValue != source.Path && isManagedNeuDriveDir(pathValue) {
						return filepath.SkipDir
					}
					return nil
				}
				rel, err := filepath.Rel(source.Path, pathValue)
				if err != nil {
					return err
				}
				hubPath := filepath.ToSlash(filepath.Join("/platforms", platform, source.Domain, source.Label, rel))
				bytesWritten, err := s.writeLocalPlatformFile(ctx, userID, hubPath, pathValue, map[string]interface{}{
					"platform":      platform,
					"domain":        source.Domain,
					"source_label":  source.Label,
					"original_path": pathValue,
				})
				if err != nil {
					return err
				}
				result.Files++
				result.Bytes += bytesWritten
				result.Paths = append(result.Paths, hubPath)
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}

		hubPath := filepath.ToSlash(filepath.Join("/platforms", platform, source.Domain, source.Label))
		bytesWritten, err := s.writeLocalPlatformFile(ctx, userID, hubPath, source.Path, map[string]interface{}{
			"platform":      platform,
			"domain":        source.Domain,
			"source_label":  source.Label,
			"original_path": source.Path,
		})
		if err != nil {
			return nil, err
		}
		result.Files++
		result.Bytes += bytesWritten
		result.Paths = append(result.Paths, hubPath)
	}
	sort.Strings(result.Paths)
	return result, nil
}

func (s *Server) writeLocalPlatformFile(ctx context.Context, userID uuid.UUID, hubPath, srcPath string, metadata map[string]interface{}) (int64, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return 0, err
	}
	contentType := skillsarchive.DetectContentType(srcPath, data)
	if skillsarchive.LooksBinary(srcPath, data) {
		_, err = s.FileTreeService.WriteBinaryEntry(ctx, userID, hubPath, data, contentType, models.FileTreeWriteOptions{
			Metadata:      metadata,
			MinTrustLevel: models.TrustLevelWork,
		})
		return int64(len(data)), err
	}
	_, err = s.FileTreeService.WriteEntry(ctx, userID, hubPath, string(data), contentType, models.FileTreeWriteOptions{
		Metadata:      metadata,
		MinTrustLevel: models.TrustLevelWork,
	})
	return int64(len(data)), err
}

func (s *Server) exportLocalPlatformSnapshot(ctx context.Context, userID uuid.UUID, platform, outputRoot string) (*sqlitestorage.ExportResult, error) {
	result := &sqlitestorage.ExportResult{Platform: platform, OutputRoot: outputRoot}
	root := filepath.ToSlash(filepath.Join("/platforms", platform))
	snapshot, err := s.FileTreeService.Snapshot(ctx, userID, root, models.TrustLevelFull)
	if err != nil {
		return nil, err
	}
	for _, entry := range snapshot.Entries {
		if entry.IsDirectory {
			continue
		}
		rel := strings.TrimPrefix(entry.Path, root)
		rel = strings.TrimPrefix(rel, "/")
		target := filepath.Join(outputRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		if isBinaryMetadata(entry.Metadata) {
			data, _, err := s.FileTreeService.ReadBinary(ctx, userID, entry.Path, models.TrustLevelFull)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				return nil, err
			}
			result.Files++
			result.Bytes += int64(len(data))
			result.Paths = append(result.Paths, target)
			continue
		}
		if err := os.WriteFile(target, []byte(entry.Content), 0o644); err != nil {
			return nil, err
		}
		result.Files++
		result.Bytes += int64(len(entry.Content))
		result.Paths = append(result.Paths, target)
	}
	sort.Strings(result.Paths)
	return result, nil
}

func (s *Server) importLocalPlatformAgentPayload(ctx context.Context, userID uuid.UUID, platform string, payload sqlitestorage.AgentExportPayload) (*sqlitestorage.AgentImportResult, error) {
	result := &sqlitestorage.AgentImportResult{Platform: platform}
	source := "agent:" + platform

	if content := renderAgentProfileRules(platform, payload.ProfileRules); strings.TrimSpace(content) != "" {
		category := platform + "-agent"
		if err := s.MemoryService.UpsertProfile(ctx, userID, category, content, source); err != nil {
			return nil, err
		}
		result.ProfileCategories++
		result.Paths = append(result.Paths, hubpath.ProfilePath(category))
	}

	for _, item := range payload.MemoryItems {
		if strings.TrimSpace(item.Content) == "" {
			continue
		}
		now := time.Now().UTC()
		expiresAt := now.AddDate(1, 0, 0)
		entry, err := s.MemoryService.ImportScratch(ctx, userID, renderAgentMemoryItem(item), source, item.Title, now, &expiresAt)
		if err != nil {
			return nil, err
		}
		result.MemoryItems++
		if entry != nil {
			result.Paths = append(result.Paths, entry.Path)
		} else {
			result.Paths = append(result.Paths, importedScratchPath(source, item.Title, now))
		}
	}

	for _, project := range payload.Projects {
		name := strings.TrimSpace(project.Name)
		if name == "" || strings.TrimSpace(project.Context) == "" {
			continue
		}
		if _, err := s.ProjectService.Get(ctx, userID, name); err != nil {
			if _, err := s.ProjectService.Create(ctx, userID, name); err != nil {
				return nil, err
			}
		}
		contextBody := renderAgentProjectContext(project)
		if err := s.ProjectService.UpdateContext(ctx, userID, name, contextBody); err != nil {
			return nil, err
		}
		if _, err := s.FileTreeService.WriteEntry(ctx, userID, hubpath.ProjectContextPath(name), contextBody, "text/markdown", models.FileTreeWriteOptions{
			Kind:          "project_context",
			MinTrustLevel: models.TrustLevelCollaborate,
			Metadata: map[string]interface{}{
				"source_platform": platform,
				"capture_mode":    "agent",
				"exactness":       project.Exactness,
				"source_paths":    project.SourcePaths,
			},
		}); err != nil {
			return nil, err
		}
		result.Projects++
		result.Paths = append(result.Paths, hubpath.ProjectContextPath(name))
	}

	if written, err := s.writeLocalAgentArtifact(ctx, userID, platform, "automations.json", payload.Automations); err != nil {
		return nil, err
	} else if written != "" {
		result.Artifacts++
		result.Paths = append(result.Paths, written)
	}
	if written, err := s.writeLocalAgentArtifact(ctx, userID, platform, "tools.json", payload.Tools); err != nil {
		return nil, err
	} else if written != "" {
		result.Artifacts++
		result.Paths = append(result.Paths, written)
	}
	if written, err := s.writeLocalAgentArtifact(ctx, userID, platform, "connections.json", payload.Connections); err != nil {
		return nil, err
	} else if written != "" {
		result.Artifacts++
		result.Paths = append(result.Paths, written)
	}
	if written, err := s.writeLocalAgentArtifact(ctx, userID, platform, "archives.json", payload.Archives); err != nil {
		return nil, err
	} else if written != "" {
		result.Artifacts++
		result.Paths = append(result.Paths, written)
	}
	if written, err := s.writeLocalAgentArtifact(ctx, userID, platform, "unsupported.json", payload.Unsupported); err != nil {
		return nil, err
	} else if written != "" {
		result.Artifacts++
		result.Paths = append(result.Paths, written)
	}
	if content := renderAgentNotes(payload.Notes); strings.TrimSpace(content) != "" {
		target := filepath.ToSlash(filepath.Join("/platforms", platform, "agent", "notes.md"))
		if _, err := s.FileTreeService.WriteEntry(ctx, userID, target, content, "text/markdown", models.FileTreeWriteOptions{
			Kind:          "file",
			MinTrustLevel: models.TrustLevelWork,
			Metadata: map[string]interface{}{
				"source_platform": platform,
				"capture_mode":    "agent",
				"exactness":       "reference",
			},
		}); err != nil {
			return nil, err
		}
		result.Artifacts++
		result.Paths = append(result.Paths, target)
	}

	sort.Strings(result.Paths)
	return result, nil
}

func (s *Server) writeLocalAgentArtifact(ctx context.Context, userID uuid.UUID, platform, filename string, payload any) (string, error) {
	if isEmptyAgentPayload(payload) {
		return "", nil
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	target := filepath.ToSlash(filepath.Join("/platforms", platform, "agent", filename))
	if _, err := s.FileTreeService.WriteEntry(ctx, userID, target, string(data)+"\n", "application/json", models.FileTreeWriteOptions{
		Kind:          "file",
		MinTrustLevel: models.TrustLevelWork,
		Metadata: map[string]interface{}{
			"source_platform": platform,
			"capture_mode":    "agent",
			"exactness":       "reference",
		},
	}); err != nil {
		return "", err
	}
	return target, nil
}

func (s *Server) importLocalSkillsArchive(ctx context.Context, userID uuid.UUID, platform, archivePath string) (*sqlitestorage.ImportResult, error) {
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open skills archive: %w", err)
	}
	files, err := skillsarchive.ParseZipBytes(data, filepath.Base(archivePath))
	if err != nil {
		return nil, err
	}

	result := &sqlitestorage.ImportResult{Platform: platform}
	for _, file := range files {
		hubPath := filepath.ToSlash(path.Join("/skills", file.SkillName, file.RelPath))
		metadata := map[string]interface{}{
			"source_platform": platform,
			"source_archive":  filepath.Base(archivePath),
			"capture_mode":    "archive",
		}
		contentType := skillsarchive.DetectContentType(file.RelPath, file.Data)
		if skillsarchive.LooksBinary(file.RelPath, file.Data) {
			if _, err := s.FileTreeService.WriteBinaryEntry(ctx, userID, hubPath, file.Data, contentType, models.FileTreeWriteOptions{
				Kind:          "skill_asset",
				Metadata:      metadata,
				MinTrustLevel: models.TrustLevelWork,
			}); err != nil {
				return nil, fmt.Errorf("write %s: %w", hubPath, err)
			}
		} else {
			if _, err := s.FileTreeService.WriteEntry(ctx, userID, hubPath, string(file.Data), contentType, models.FileTreeWriteOptions{
				Kind:          "skill_file",
				Metadata:      metadata,
				MinTrustLevel: models.TrustLevelWork,
			}); err != nil {
				return nil, fmt.Errorf("write %s: %w", hubPath, err)
			}
		}
		result.Files++
		result.Bytes += int64(len(file.Data))
		result.Paths = append(result.Paths, hubPath)
	}
	sort.Strings(result.Paths)
	return result, nil
}

func isManagedNeuDriveDir(pathValue string) bool {
	_, err := os.Stat(filepath.Join(pathValue, ".neudrive-managed.json"))
	return err == nil
}

func isBinaryMetadata(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	value, ok := metadata["binary"]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == "true"
	default:
		return false
	}
}

func renderAgentProfileRules(platform string, rules []sqlitestorage.AgentProfileRule) string {
	if len(rules) == 0 {
		return ""
	}
	lines := []string{
		fmt.Sprintf("# %s agent-derived profile rules", platform),
		"",
	}
	for _, rule := range rules {
		content := strings.TrimSpace(rule.Content)
		if content == "" {
			continue
		}
		title := strings.TrimSpace(rule.Title)
		if title == "" {
			title = "Rule"
		}
		lines = append(lines, "## "+title, "")
		lines = append(lines, content, "")
		lines = append(lines, fmt.Sprintf("- Exactness: %s", fallbackAgentExactness(rule.Exactness)))
		if len(rule.SourcePaths) > 0 {
			lines = append(lines, "- Source paths:")
			for _, source := range rule.SourcePaths {
				lines = append(lines, "  - "+source)
			}
		}
		if rule.Confidence > 0 {
			lines = append(lines, fmt.Sprintf("- Confidence: %.2f", rule.Confidence))
		}
		lines = append(lines, "")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderAgentMemoryItem(item sqlitestorage.AgentMemoryItem) string {
	lines := []string{}
	if title := strings.TrimSpace(item.Title); title != "" {
		lines = append(lines, "# "+title, "")
	}
	lines = append(lines, strings.TrimSpace(item.Content), "")
	lines = append(lines, fmt.Sprintf("- Exactness: %s", fallbackAgentExactness(item.Exactness)))
	if len(item.SourcePaths) > 0 {
		lines = append(lines, "- Source paths:")
		for _, source := range item.SourcePaths {
			lines = append(lines, "  - "+source)
		}
	}
	if item.Confidence > 0 {
		lines = append(lines, fmt.Sprintf("- Confidence: %.2f", item.Confidence))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderAgentProjectContext(project sqlitestorage.AgentProjectRecord) string {
	lines := []string{strings.TrimSpace(project.Context), ""}
	lines = append(lines, fmt.Sprintf("- Exactness: %s", fallbackAgentExactness(project.Exactness)))
	if len(project.SourcePaths) > 0 {
		lines = append(lines, "- Source paths:")
		for _, source := range project.SourcePaths {
			lines = append(lines, "  - "+source)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderAgentNotes(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	lines := []string{"# Notes", ""}
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if note == "" {
			continue
		}
		lines = append(lines, "- "+note)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func fallbackAgentExactness(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "exact", "derived", "reference":
		return strings.TrimSpace(strings.ToLower(raw))
	default:
		return "derived"
	}
}

func isEmptyAgentPayload(payload any) bool {
	switch typed := payload.(type) {
	case []sqlitestorage.AgentRecord:
		return len(typed) == 0
	default:
		return payload == nil
	}
}

func importedScratchPath(source, title string, createdAt time.Time) string {
	key := fmt.Sprintf("neudrive/imported-scratch/%s/%s/%s", source, title, createdAt.UTC().Format(time.RFC3339Nano))
	legacyID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(key))
	slugBase := title
	if strings.TrimSpace(slugBase) == "" {
		slugBase = source
	}
	slug := fmt.Sprintf("%s-%s", slugBase, legacyID.String()[:8])
	return hubpath.ScratchPath(createdAt.UTC(), slug)
}
