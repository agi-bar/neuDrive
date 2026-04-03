package services

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

	baseDir := ".skills/" + skillName

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

	// Use a transaction for atomicity.
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("import.ImportBulkFiles: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	imported := 0

	// Collect all parent directories that need to be created.
	dirs := map[string]bool{}
	for path := range files {
		dir := filepath.Dir(path)
		for dir != "." && dir != "" && dir != "/" {
			if !dirs[dir] {
				dirs[dir] = true
			}
			dir = filepath.Dir(dir)
		}
	}

	// Create directories.
	for dir := range dirs {
		dirPath := dir
		if !strings.HasSuffix(dirPath, "/") {
			dirPath = dirPath + "/"
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO file_tree (id, user_id, path, is_directory, content, content_type, metadata, min_trust_level, created_at, updated_at)
			 VALUES ($1, $2, $3, true, '', 'directory', '{}', 1, $4, $4)
			 ON CONFLICT (user_id, path) DO NOTHING`,
			uuid.New(), userID, dirPath, now)
		if err != nil {
			return 0, fmt.Errorf("import.ImportBulkFiles: ensure dir %s: %w", dir, err)
		}
	}

	// Insert files.
	for path, content := range files {
		if path == "" || content == "" {
			continue
		}

		ct := contentTypeFromExt(path)
		id := uuid.New()

		_, err := tx.Exec(ctx,
			`INSERT INTO file_tree (id, user_id, path, is_directory, content, content_type, metadata, min_trust_level, created_at, updated_at)
			 VALUES ($1, $2, $3, false, $4, $5, '{}', $6, $7, $7)
			 ON CONFLICT (user_id, path) DO UPDATE SET
			   content = EXCLUDED.content,
			   content_type = EXCLUDED.content_type,
			   min_trust_level = EXCLUDED.min_trust_level,
			   updated_at = EXCLUDED.updated_at`,
			id, userID, path, content, ct, minTrustLevel, now)
		if err != nil {
			return 0, fmt.Errorf("import.ImportBulkFiles: write %s: %w", path, err)
		}
		imported++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("import.ImportBulkFiles: commit: %w", err)
	}

	return imported, nil
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
