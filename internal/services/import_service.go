package services

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/logger"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/skillsarchive"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const MaxSkillsArchiveBytes = 50 << 20

// ImportService handles bulk import and export operations.
type ImportService struct {
	db       *pgxpool.Pool
	fileTree *FileTreeService
	memory   *MemoryService
	vault    *VaultService
}

// NewImportService creates a new ImportService.
func NewImportService(db *pgxpool.Pool, fileTree *FileTreeService, memory *MemoryService, vault *VaultService) *ImportService {
	return &ImportService{
		db:       db,
		fileTree: fileTree,
		memory:   memory,
		vault:    vault,
	}
}

// ImportSkill imports a .skill directory structure into the file tree.
// It creates /skills/{skillName}/ with all files including SKILL.md.
// Returns the number of files imported.
func (s *ImportService) ImportSkill(ctx context.Context, userID uuid.UUID, skillName string, files map[string]string) (int, error) {
	if skillName == "" {
		return 0, fmt.Errorf("import.ImportSkill: skill name is required")
	}
	if len(files) == 0 {
		return 0, fmt.Errorf("import.ImportSkill: no files provided")
	}
	if s.fileTree == nil {
		return 0, fmt.Errorf("import.ImportSkill: file tree service not configured")
	}

	baseDir := "/skills/" + skillName

	// Ensure the skill directory exists.
	if err := s.fileTree.EnsureDirectory(ctx, userID, baseDir); err != nil {
		return 0, fmt.Errorf("import.ImportSkill: create skill dir: %w", err)
	}

	imported := 0
	for relPath, content := range files {
		if relPath == "" || content == "" {
			continue
		}

		// Normalize: remove leading slashes.
		relPath = strings.TrimPrefix(relPath, "/")
		fullPath := baseDir + "/" + relPath

		// Ensure parent directory exists.
		dir := filepath.Dir(fullPath)
		if dir != "." && dir != "" {
			if err := s.fileTree.EnsureDirectory(ctx, userID, dir); err != nil {
				return imported, fmt.Errorf("import.ImportSkill: ensure dir %s: %w", dir, err)
			}
		}

		// Determine content type from extension.
		ct := contentTypeFromExt(relPath)

		_, err := s.fileTree.Write(ctx, userID, fullPath, content, ct, models.TrustLevelGuest)
		if err != nil {
			return imported, fmt.Errorf("import.ImportSkill: write %s: %w", relPath, err)
		}
		imported++
	}

	return imported, nil
}

type SkillsArchiveImportResult struct {
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
	Skills   []string `json:"skills,omitempty"`
}

// ImportSkillsArchive imports a zip archive that contains one or more skills.
// It preserves both text files and binary assets under /skills/<name>/...
func (s *ImportService) ImportSkillsArchive(ctx context.Context, userID uuid.UUID, archiveData []byte, platform, archiveName string) (*SkillsArchiveImportResult, error) {
	return s.ImportSkillsArchiveReader(ctx, userID, bytes.NewReader(archiveData), int64(len(archiveData)), platform, archiveName)
}

func (s *ImportService) ImportSkillsArchiveReader(ctx context.Context, userID uuid.UUID, readerAt io.ReaderAt, archiveSize int64, platform, archiveName string) (*SkillsArchiveImportResult, error) {
	log := logger.FromContext(ctx).With(
		"archive_name", filepath.Base(strings.TrimSpace(archiveName)),
		"archive_size_bytes", archiveSize,
		"platform", strings.TrimSpace(platform),
	)
	if s.fileTree == nil {
		err := fmt.Errorf("import.ImportSkillsArchive: file tree service not configured")
		log.Error("skills archive import failed", "error", err)
		return nil, err
	}
	if readerAt == nil || archiveSize <= 0 {
		err := fmt.Errorf("import.ImportSkillsArchive: archive data is required")
		log.Error("skills archive import failed", "error", err)
		return nil, err
	}
	if archiveSize > MaxSkillsArchiveBytes {
		err := fmt.Errorf("import.ImportSkillsArchive: archive exceeds 50 MB limit")
		log.Error("skills archive import failed", "error", err)
		return nil, err
	}

	startedAt := time.Now()
	entries, err := skillsarchive.ParseZipReader(readerAt, archiveSize, archiveName)
	if err != nil {
		log.Error("skills archive import failed", "phase", "parse", "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)
		return nil, fmt.Errorf("import.ImportSkillsArchive: %w", err)
	}
	parseDoneAt := time.Now()

	result, optimized, err := s.importSkillsArchiveEntries(ctx, userID, entries, platform, archiveName)
	if err != nil {
		log.Error("skills archive import failed",
			"phase", "write",
			"entries", len(entries),
			"parse_ms", parseDoneAt.Sub(startedAt).Milliseconds(),
			"duration_ms", time.Since(parseDoneAt).Milliseconds(),
			"optimized", optimized,
			"error", err,
		)
		return nil, err
	}

	log.Info("skills archive import completed",
		slog.Int("entries", len(entries)),
		slog.Int("skills", len(result.Skills)),
		slog.Int("imported", result.Imported),
		slog.Int("errors", len(result.Errors)),
		slog.Bool("optimized", optimized),
		slog.Int64("parse_ms", parseDoneAt.Sub(startedAt).Milliseconds()),
		slog.Int64("write_ms", time.Since(parseDoneAt).Milliseconds()),
		slog.Int64("total_ms", time.Since(startedAt).Milliseconds()),
	)
	return result, nil
}

// ImportSkillsArchiveEntries writes parsed archive entries into /skills/<name>/...
func (s *ImportService) ImportSkillsArchiveEntries(ctx context.Context, userID uuid.UUID, entries []skillsarchive.Entry, platform, archiveName string) (*SkillsArchiveImportResult, error) {
	result, _, err := s.importSkillsArchiveEntries(ctx, userID, entries, platform, archiveName)
	return result, err
}

func (s *ImportService) importSkillsArchiveEntries(ctx context.Context, userID uuid.UUID, entries []skillsarchive.Entry, platform, archiveName string) (*SkillsArchiveImportResult, bool, error) {
	if s.fileTree == nil {
		return nil, false, fmt.Errorf("import.ImportSkillsArchiveEntries: file tree service not configured")
	}
	if len(entries) == 0 {
		return nil, false, fmt.Errorf("import.ImportSkillsArchiveEntries: no archive entries provided")
	}

	if s.fileTree.db != nil && s.fileTree.repo == nil {
		result, err := s.importSkillsArchiveEntriesOptimized(ctx, userID, entries, platform, archiveName)
		return result, true, err
	}

	if strings.TrimSpace(platform) == "" {
		platform = "skills-archive"
	}
	archiveName = filepath.Base(strings.TrimSpace(archiveName))
	if archiveName == "" {
		archiveName = "skills.zip"
	}

	result := &SkillsArchiveImportResult{}
	for _, entry := range entries {
		fullPath := filepath.ToSlash(filepath.Join("/skills", entry.SkillName, entry.RelPath))
		metadata := map[string]interface{}{
			"source_platform": platform,
			"source_archive":  archiveName,
			"capture_mode":    "archive",
		}
		contentType := skillsarchive.DetectContentType(entry.RelPath, entry.Data)
		if skillsarchive.LooksBinary(entry.RelPath, entry.Data) {
			if _, err := s.fileTree.WriteBinaryEntry(ctx, userID, fullPath, entry.Data, contentType, models.FileTreeWriteOptions{
				Kind:          "skill_asset",
				Metadata:      metadata,
				MinTrustLevel: models.TrustLevelGuest,
			}); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("skill %s/%s: %v", entry.SkillName, entry.RelPath, err))
				continue
			}
		} else {
			if _, err := s.fileTree.WriteEntry(ctx, userID, fullPath, string(entry.Data), contentType, models.FileTreeWriteOptions{
				Kind:          "skill_file",
				Metadata:      metadata,
				MinTrustLevel: models.TrustLevelGuest,
			}); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("skill %s/%s: %v", entry.SkillName, entry.RelPath, err))
				continue
			}
		}
		result.Imported++
		result.Skills = appendUniqueString(result.Skills, entry.SkillName)
	}

	return result, false, nil
}

func appendUniqueString(items []string, value string) []string {
	for _, existing := range items {
		if existing == value {
			return items
		}
	}
	return append(items, value)
}

// claudeMemoryExport is the expected JSON structure from Claude memory exports.
type claudeMemoryExport struct {
	Memories []claudeMemoryItem `json:"memories"`
}

type claudeMemoryItem struct {
	Content   string `json:"content"`
	Type      string `json:"type"`
	CreatedAt string `json:"created_at,omitempty"`
}

// ImportClaudeMemory imports Claude's memory export (JSON format).
// It parses memory items by type: preferences and relationships go to
// memory_profile under their respective categories; project items go
// to memory_scratch.
func (s *ImportService) ImportClaudeMemory(ctx context.Context, userID uuid.UUID, memoryJSON []byte) (int, error) {
	if s.memory == nil {
		return 0, fmt.Errorf("import.ImportClaudeMemory: memory service not configured")
	}

	// Accept both wrapped {memories: [...]} and bare array [...]
	var export claudeMemoryExport
	if err := json.Unmarshal(memoryJSON, &export); err != nil || len(export.Memories) == 0 {
		// Try bare array
		var items []claudeMemoryItem
		if err2 := json.Unmarshal(memoryJSON, &items); err2 != nil {
			return 0, fmt.Errorf("import.ImportClaudeMemory: parse JSON: expected {memories:[...]} or [...]: %w", err)
		}
		export.Memories = items
	}

	if len(export.Memories) == 0 {
		return 0, nil
	}

	// Group memories by type for aggregation into profile categories.
	preferences := []string{}
	relationships := []string{}
	projectItems := []string{}
	otherItems := []string{}

	for _, mem := range export.Memories {
		if mem.Content == "" {
			continue
		}
		switch strings.ToLower(mem.Type) {
		case "preference":
			preferences = append(preferences, mem.Content)
		case "relationship":
			relationships = append(relationships, mem.Content)
		case "project":
			projectItems = append(projectItems, mem.Content)
		default:
			otherItems = append(otherItems, mem.Content)
		}
	}

	imported := 0

	// Write preferences to memory_profile.
	if len(preferences) > 0 {
		content := strings.Join(preferences, "\n")
		if err := s.memory.UpsertProfile(ctx, userID, "preferences", content, "claude-import"); err != nil {
			return imported, fmt.Errorf("import.ImportClaudeMemory: upsert preferences: %w", err)
		}
		imported += len(preferences)
	}

	// Write relationships to memory_profile.
	if len(relationships) > 0 {
		content := strings.Join(relationships, "\n")
		if err := s.memory.UpsertProfile(ctx, userID, "relationships", content, "claude-import"); err != nil {
			return imported, fmt.Errorf("import.ImportClaudeMemory: upsert relationships: %w", err)
		}
		imported += len(relationships)
	}

	// Write project items to memory_scratch.
	for _, item := range projectItems {
		if err := s.memory.WriteScratch(ctx, userID, item, "claude-import"); err != nil {
			return imported, fmt.Errorf("import.ImportClaudeMemory: write scratch: %w", err)
		}
		imported++
	}

	// Write uncategorized items to memory_profile under "claude-misc".
	if len(otherItems) > 0 {
		content := strings.Join(otherItems, "\n")
		if err := s.memory.UpsertProfile(ctx, userID, "claude-misc", content, "claude-import"); err != nil {
			return imported, fmt.Errorf("import.ImportClaudeMemory: upsert misc: %w", err)
		}
		imported += len(otherItems)
	}

	return imported, nil
}

// ImportProfile imports user profile data (preferences, relationships, principles).
func (s *ImportService) ImportProfile(ctx context.Context, userID uuid.UUID, profile map[string]string) error {
	if s.memory == nil {
		return fmt.Errorf("import.ImportProfile: memory service not configured")
	}

	for category, content := range profile {
		if content == "" {
			continue
		}
		if err := s.memory.UpsertProfile(ctx, userID, category, content, "import"); err != nil {
			return fmt.Errorf("import.ImportProfile: upsert %s: %w", category, err)
		}
	}
	return nil
}

// ImportBulkFiles imports multiple files into the file tree in a single transaction.
// Returns the number of files imported.
func (s *ImportService) ImportBulkFiles(ctx context.Context, userID uuid.UUID, files map[string]string, minTrustLevel int) (int, error) {
	if s.fileTree == nil {
		return 0, fmt.Errorf("import.ImportBulkFiles: file tree service not configured")
	}
	if len(files) == 0 {
		return 0, nil
	}
	if minTrustLevel <= 0 {
		minTrustLevel = models.TrustLevelGuest
	}
	imported := 0

	for rawPath, content := range files {
		if rawPath == "" || content == "" {
			continue
		}

		normalizedPath := hubpath.NormalizeStorage(rawPath)
		ct := contentTypeFromExt(normalizedPath)
		if _, err := s.fileTree.WriteEntry(ctx, userID, normalizedPath, content, ct, models.FileTreeWriteOptions{
			MinTrustLevel: minTrustLevel,
		}); err != nil {
			return 0, fmt.Errorf("import.ImportBulkFiles: write %s: %w", normalizedPath, err)
		}
		imported++
	}

	return imported, nil
}

// ---------------------------------------------------------------------------
// Claude Data Import (full export zip)
// ---------------------------------------------------------------------------

// ClaudeDataImportResult holds the result of importing a Claude data export.
type ClaudeDataImportResult struct {
	MemoriesImported      int `json:"memories_imported"`
	ConversationsImported int `json:"conversations_imported"`
	ProjectsImported      int `json:"projects_imported"`
	FilesWritten          int `json:"files_written"`
}

// Claude export JSON structures
type claudeUser struct {
	UUID     string `json:"uuid"`
	FullName string `json:"full_name"`
	Email    string `json:"email_address"`
}

type claudeConversation struct {
	UUID         string              `json:"uuid"`
	Name         string              `json:"name"`
	Summary      string              `json:"summary"`
	CreatedAt    string              `json:"created_at"`
	ChatMessages []claudeChatMessage `json:"chat_messages"`
}

type claudeChatMessage struct {
	Text      string `json:"text"`
	Sender    string `json:"sender"`
	CreatedAt string `json:"created_at"`
}

type claudeProject struct {
	UUID        string      `json:"uuid"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	IsStarter   bool        `json:"is_starter_project"`
	Docs        []claudeDoc `json:"docs"`
	CreatedAt   string      `json:"created_at"`
}

type claudeDoc struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

type claudeMemoryFile struct {
	ConversationsMemory string `json:"conversations_memory"`
	AccountUUID         string `json:"account_uuid"`
}

// ImportClaudeData imports a full Claude data export zip file.
// The zip contains: users.json, memories.json, projects.json, conversations.json
func (s *ImportService) ImportClaudeData(ctx context.Context, userID uuid.UUID, zipData []byte) (*ClaudeDataImportResult, error) {
	result := &ClaudeDataImportResult{}

	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("import.ClaudeData: open zip: %w", err)
	}

	// Read all files from zip
	fileMap := map[string][]byte{}
	for _, f := range reader.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		fileMap[f.Name] = data
	}

	// 1. Import memories.json → /memory/claude/memory.md
	if data, ok := fileMap["memories.json"]; ok {
		var memories []claudeMemoryFile
		if err := json.Unmarshal(data, &memories); err == nil && len(memories) > 0 {
			content := memories[0].ConversationsMemory
			if content != "" {
				path := "/memory/claude/memory.md"
				if _, err := s.fileTree.Write(ctx, userID, path, content, "text/markdown", models.TrustLevelFull); err == nil {
					result.MemoriesImported = 1
					result.FilesWritten++
				}
			}
		}
	}

	// 2. Import conversations.json → /memory/conversations/{name}.md
	if data, ok := fileMap["conversations.json"]; ok {
		var convos []claudeConversation
		if err := json.Unmarshal(data, &convos); err == nil {
			for _, c := range convos {
				if len(c.ChatMessages) == 0 {
					continue
				}

				// Build conversation markdown
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("# %s\n\n", c.Name))
				sb.WriteString(fmt.Sprintf("Date: %s\n\n", c.CreatedAt[:10]))
				if c.Summary != "" {
					sb.WriteString(fmt.Sprintf("Summary: %s\n\n", c.Summary))
				}
				sb.WriteString("---\n\n")

				for _, msg := range c.ChatMessages {
					role := "User"
					if msg.Sender == "assistant" {
						role = "Claude"
					}
					text := msg.Text
					if len(text) > 500 {
						text = text[:500] + "..."
					}
					sb.WriteString(fmt.Sprintf("**%s**: %s\n\n", role, text))
				}

				// Sanitize filename
				safeName := strings.Map(func(r rune) rune {
					if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
						return r
					}
					if r >= 0x4e00 && r <= 0x9fff {
						return r
					}
					return '-'
				}, c.Name)
				if len(safeName) > 50 {
					safeName = safeName[:50]
				}

				date := "unknown"
				if len(c.CreatedAt) >= 10 {
					date = c.CreatedAt[:10]
				}
				path := fmt.Sprintf("/memory/conversations/%s-%s.md", date, safeName)

				if _, err := s.fileTree.Write(ctx, userID, path, sb.String(), "text/markdown", models.TrustLevelFull); err == nil {
					result.ConversationsImported++
					result.FilesWritten++
				}
			}
		}
	}

	// 3. Import projects.json → projects + /skills/{name}/
	if data, ok := fileMap["projects.json"]; ok {
		var projects []claudeProject
		if err := json.Unmarshal(data, &projects); err == nil {
			for _, p := range projects {
				if p.IsStarter {
					continue // Skip starter/template projects
				}

				// Create project in DB
				safeName := strings.Map(func(r rune) rune {
					if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
						return r
					}
					return '-'
				}, strings.ToLower(p.Name))
				if safeName == "" {
					safeName = "claude-project"
				}

				// Import docs as skills
				for _, doc := range p.Docs {
					path := fmt.Sprintf("/skills/claude-%s/%s", safeName, doc.Filename)
					if _, err := s.fileTree.Write(ctx, userID, path, doc.Content, contentTypeFromExt(doc.Filename), models.TrustLevelWork); err == nil {
						result.FilesWritten++
					}
				}
				result.ProjectsImported++
			}
		}
	}

	return result, nil
}

// ExportAll exports the entire user data as a structured map for data portability.
func (s *ImportService) ExportAll(ctx context.Context, userID uuid.UUID) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"version":     "1.0",
		"exported_at": time.Now().UTC().Format(time.RFC3339),
	}

	// Export profile.
	if s.memory != nil {
		profiles, err := s.memory.GetProfile(ctx, userID)
		if err == nil {
			profileMap := map[string]string{}
			for _, p := range profiles {
				profileMap[p.Category] = p.Content
			}
			result["profile"] = profileMap
		}

		// Export scratch.
		scratch, err := s.memory.GetScratch(ctx, userID, 90) // last 90 days
		if err == nil {
			scratchItems := make([]map[string]string, 0, len(scratch))
			for _, s := range scratch {
				scratchItems = append(scratchItems, map[string]string{
					"date":    s.Date,
					"content": s.Content,
					"source":  s.Source,
				})
			}
			result["scratch"] = scratchItems
		}
	}

	// Export file tree.
	if s.fileTree != nil {
		entries, err := s.fileTree.List(ctx, userID, "/", models.TrustLevelFull)
		if err == nil {
			files := map[string]string{}
			for _, e := range entries {
				if e.IsDirectory {
					continue
				}
				full, err := s.fileTree.Read(ctx, userID, e.Path, models.TrustLevelFull)
				if err != nil {
					continue
				}
				files[full.Path] = full.Content
			}
			result["files"] = files
		}
	}

	// Export vault scope names (not values for security).
	if s.vault != nil {
		scopes, err := s.vault.ListScopes(ctx, userID, models.TrustLevelFull)
		if err == nil {
			scopeNames := make([]string, 0, len(scopes))
			for _, vs := range scopes {
				scopeNames = append(scopeNames, vs.Scope)
			}
			result["vault_scopes"] = scopeNames
		}
	}

	return result, nil
}

// contentTypeFromExt returns a content type string based on the file extension.
func contentTypeFromExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "text/yaml"
	case ".txt":
		return "text/plain"
	case ".go":
		return "text/x-go"
	case ".py":
		return "text/x-python"
	case ".js":
		return "text/javascript"
	case ".ts":
		return "text/typescript"
	case ".sh":
		return "text/x-shellscript"
	case ".toml":
		return "text/toml"
	default:
		return "text/plain"
	}
}
