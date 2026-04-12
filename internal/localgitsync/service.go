package localgitsync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	sqlitestorage "github.com/agi-bar/agenthub/internal/storage/sqlite"
	"github.com/agi-bar/agenthub/internal/systemskills"
	"github.com/agi-bar/agenthub/internal/vault"
	"github.com/google/uuid"
)

const (
	defaultMirrorRoot = "./agenthub-export/git-mirror"
	readmePath        = "README.md"
)

type mirrorRepo interface {
	GetActiveLocalGitMirror(ctx context.Context, userID uuid.UUID) (*models.LocalGitMirror, error)
	UpsertActiveLocalGitMirror(ctx context.Context, mirror models.LocalGitMirror) error
	UpdateLocalGitMirrorState(ctx context.Context, userID uuid.UUID, lastSyncedAt *time.Time, lastError string, gitInitializedAt *time.Time) error
}

type SyncInfo struct {
	Enabled      bool   `json:"enabled"`
	Path         string `json:"path,omitempty"`
	Synced       bool   `json:"synced"`
	LastSyncedAt string `json:"last_synced_at,omitempty"`
	Message      string `json:"message,omitempty"`
	LastError    string `json:"last_error,omitempty"`
}

type Service struct {
	mirrors     mirrorRepo
	fileTree    *services.FileTreeService
	users       *services.UserService
	connections *services.ConnectionService
	projects    *services.ProjectService
	vault       *services.VaultService
}

func New(store *sqlitestorage.Store, vaultCrypto *vault.Vault) *Service {
	if store == nil {
		return nil
	}
	fileTree := services.NewFileTreeServiceWithRepo(sqlitestorage.NewFileTreeRepo(store))
	roleSvc := services.NewRoleServiceWithRepo(sqlitestorage.NewRoleRepo(store), fileTree)
	return NewWithDeps(
		store,
		fileTree,
		services.NewUserServiceWithRepo(sqlitestorage.NewUserRepo(store)),
		services.NewConnectionServiceWithRepo(sqlitestorage.NewConnectionRepo(store)),
		services.NewProjectServiceWithRepo(sqlitestorage.NewProjectRepo(store), roleSvc, fileTree),
		services.NewVaultServiceWithRepo(sqlitestorage.NewVaultRepo(store), vaultCrypto),
	)
}

func NewWithDeps(
	mirrors mirrorRepo,
	fileTree *services.FileTreeService,
	users *services.UserService,
	connections *services.ConnectionService,
	projects *services.ProjectService,
	vaultSvc *services.VaultService,
) *Service {
	if mirrors == nil || fileTree == nil {
		return nil
	}
	return &Service{
		mirrors:     mirrors,
		fileTree:    fileTree,
		users:       users,
		connections: connections,
		projects:    projects,
		vault:       vaultSvc,
	}
}

func DefaultMirrorRoot() string {
	return defaultMirrorRoot
}

func (s *Service) GetActiveMirror(ctx context.Context, userID uuid.UUID) (*models.LocalGitMirror, error) {
	if s == nil || s.mirrors == nil {
		return nil, fmt.Errorf("local git sync not configured")
	}
	return s.mirrors.GetActiveLocalGitMirror(ctx, userID)
}

func (s *Service) RegisterMirrorAndSync(ctx context.Context, userID uuid.UUID, outputRoot string) (*SyncInfo, error) {
	if s == nil || s.mirrors == nil {
		return nil, fmt.Errorf("local git sync not configured")
	}
	rootPath, err := resolveMirrorRoot(outputRoot)
	if err != nil {
		return nil, err
	}
	active, err := s.mirrors.GetActiveLocalGitMirror(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := validateMirrorRoot(rootPath, active); err != nil {
		return nil, err
	}

	syncedAt := time.Now().UTC()
	gitInitializedAt, err := s.syncIntoRoot(ctx, userID, rootPath, syncedAt, active)
	if err != nil {
		return nil, err
	}

	if err := s.mirrors.UpsertActiveLocalGitMirror(ctx, models.LocalGitMirror{
		UserID:           userID,
		RootPath:         rootPath,
		IsActive:         true,
		GitInitializedAt: gitInitializedAt,
		LastSyncedAt:     &syncedAt,
		LastError:        "",
		CreatedAt:        mirrorCreatedAt(active, syncedAt),
		UpdatedAt:        syncedAt,
	}); err != nil {
		return nil, err
	}

	return buildSuccessInfo(rootPath, syncedAt), nil
}

func (s *Service) SyncActiveMirror(ctx context.Context, userID uuid.UUID) (*SyncInfo, error) {
	if s == nil || s.mirrors == nil {
		return nil, nil
	}
	active, err := s.mirrors.GetActiveLocalGitMirror(ctx, userID)
	if err != nil {
		return nil, err
	}
	if active == nil || strings.TrimSpace(active.RootPath) == "" {
		return &SyncInfo{Enabled: false, Synced: false}, nil
	}

	syncedAt := time.Now().UTC()
	gitInitializedAt, syncErr := s.syncIntoRoot(ctx, userID, active.RootPath, syncedAt, active)
	if syncErr != nil {
		info := buildFailureInfo(active.RootPath, syncErr)
		_ = s.mirrors.UpdateLocalGitMirrorState(ctx, userID, active.LastSyncedAt, syncErr.Error(), gitInitializedAt)
		return info, syncErr
	}

	if err := s.mirrors.UpdateLocalGitMirrorState(ctx, userID, &syncedAt, "", gitInitializedAt); err != nil {
		info := buildFailureInfo(active.RootPath, err)
		return info, err
	}

	return buildSuccessInfo(active.RootPath, syncedAt), nil
}

func (s *Service) syncIntoRoot(
	ctx context.Context,
	userID uuid.UUID,
	rootPath string,
	syncedAt time.Time,
	existing *models.LocalGitMirror,
) (*time.Time, error) {
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		return nil, err
	}
	if err := clearMirrorRoot(rootPath); err != nil {
		return nil, err
	}
	if err := s.writeFileTree(ctx, userID, rootPath); err != nil {
		return nil, err
	}
	if err := s.writeSidecars(ctx, userID, rootPath, syncedAt); err != nil {
		return nil, err
	}
	gitInitializedAt, err := ensureGitRepo(ctx, rootPath, existing)
	if err != nil {
		return nil, err
	}
	return gitInitializedAt, nil
}

func (s *Service) writeFileTree(ctx context.Context, userID uuid.UUID, rootPath string) error {
	snapshot, err := s.fileTree.Snapshot(ctx, userID, "/", models.TrustLevelFull)
	if err != nil && err != services.ErrEntryNotFound {
		return err
	}
	if snapshot == nil {
		return nil
	}
	for _, entry := range snapshot.Entries {
		if entry.IsDirectory || systemskills.IsProtectedPath(entry.Path) {
			continue
		}
		relativePath := strings.TrimPrefix(hubpath.NormalizePublic(entry.Path), "/")
		if relativePath == "" {
			continue
		}
		target := filepath.Join(rootPath, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if isBinaryEntry(entry.Metadata) {
			data, _, err := s.fileTree.ReadBinary(ctx, userID, entry.Path, models.TrustLevelFull)
			if err != nil {
				return err
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				return err
			}
			continue
		}
		if err := os.WriteFile(target, []byte(entry.Content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) writeSidecars(ctx context.Context, userID uuid.UUID, rootPath string, syncedAt time.Time) error {
	user, err := s.users.GetByID(ctx, userID)
	if err == nil && user != nil {
		if err := writeJSONFile(filepath.Join(rootPath, "identity", "profile.json"), map[string]any{
			"id":           user.ID.String(),
			"slug":         user.Slug,
			"display_name": user.DisplayName,
			"email":        user.Email,
			"timezone":     user.Timezone,
			"language":     user.Language,
			"created_at":   user.CreatedAt.UTC().Format(time.RFC3339),
			"updated_at":   user.UpdatedAt.UTC().Format(time.RFC3339),
		}); err != nil {
			return err
		}
	}
	if s.connections != nil {
		connections, err := s.connections.ListByUser(ctx, userID)
		if err == nil {
			sanitized := make([]map[string]any, 0, len(connections))
			for _, connection := range connections {
				item := map[string]any{
					"id":             connection.ID.String(),
					"name":           connection.Name,
					"platform":       connection.Platform,
					"trust_level":    connection.TrustLevel,
					"api_key_prefix": connection.APIKeyPrefix,
					"config":         connection.Config,
					"created_at":     connection.CreatedAt.UTC().Format(time.RFC3339),
					"updated_at":     connection.UpdatedAt.UTC().Format(time.RFC3339),
				}
				if connection.LastUsedAt != nil {
					item["last_used_at"] = connection.LastUsedAt.UTC().Format(time.RFC3339)
				}
				sanitized = append(sanitized, item)
			}
			if err := writeJSONFile(filepath.Join(rootPath, "connections", "connections.json"), sanitized); err != nil {
				return err
			}
		}
	}
	if s.vault != nil {
		scopes, err := s.vault.ListScopes(ctx, userID, models.TrustLevelFull)
		if err == nil {
			exported := make([]map[string]any, 0, len(scopes))
			for _, scope := range scopes {
				exported = append(exported, map[string]any{
					"scope":           scope.Scope,
					"description":     scope.Description,
					"min_trust_level": scope.MinTrustLevel,
					"created_at":      scope.CreatedAt.UTC().Format(time.RFC3339),
				})
			}
			if err := writeJSONFile(filepath.Join(rootPath, "vault", "scopes.json"), exported); err != nil {
				return err
			}
		}
	}
	if s.projects != nil {
		projects, err := s.projects.List(ctx, userID)
		if err == nil {
			exported := make([]map[string]any, 0, len(projects))
			for _, project := range projects {
				exported = append(exported, map[string]any{
					"id":         project.ID.String(),
					"name":       project.Name,
					"status":     project.Status,
					"metadata":   project.Metadata,
					"created_at": project.CreatedAt.UTC().Format(time.RFC3339),
					"updated_at": project.UpdatedAt.UTC().Format(time.RFC3339),
				})
			}
			sort.Slice(exported, func(i, j int) bool {
				return fmt.Sprint(exported[i]["name"]) < fmt.Sprint(exported[j]["name"])
			})
			if err := writeJSONFile(filepath.Join(rootPath, "_agenthub", "projects.json"), exported); err != nil {
				return err
			}
		}
	}
	if err := writeJSONFile(filepath.Join(rootPath, "_agenthub", "metadata.json"), map[string]any{
		"version":        "1.0",
		"exported_at":    syncedAt.UTC().Format(time.RFC3339),
		"last_synced_at": syncedAt.UTC().Format(time.RFC3339),
		"root_path":      rootPath,
		"mode":           "local_git_mirror",
	}); err != nil {
		return err
	}
	return writeTextFile(filepath.Join(rootPath, readmePath), buildREADME(rootPath))
}

func resolveMirrorRoot(outputRoot string) (string, error) {
	target := strings.TrimSpace(outputRoot)
	if target == "" {
		target = defaultMirrorRoot
	}
	return filepath.Abs(target)
}

func validateMirrorRoot(rootPath string, active *models.LocalGitMirror) error {
	info, err := os.Stat(rootPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists and is not a directory", rootPath)
	}
	if active != nil && samePath(active.RootPath, rootPath) {
		return nil
	}
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			return nil
		}
	}
	return fmt.Errorf("%s is not empty and is not an existing git mirror", rootPath)
}

func clearMirrorRoot(rootPath string) error {
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(rootPath, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func ensureGitRepo(ctx context.Context, rootPath string, existing *models.LocalGitMirror) (*time.Time, error) {
	gitDir := filepath.Join(rootPath, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		if existing != nil && existing.GitInitializedAt != nil {
			return existing.GitInitializedAt, nil
		}
		now := time.Now().UTC()
		return &now, nil
	}
	cmd := exec.CommandContext(ctx, "git", "-C", rootPath, "init")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git init failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	now := time.Now().UTC()
	return &now, nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeBytes(path, data)
}

func writeTextFile(path, content string) error {
	return writeBytes(path, []byte(content))
}

func writeBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func buildREADME(rootPath string) string {
	lines := []string{
		"# AgentHub Local Git Mirror",
		"",
		"This directory is a local Git mirror of your AgentHub data.",
		"",
		"- Secrets are not exported.",
		"- Vault scope metadata is available in `vault/scopes.json`.",
		"- Mirror metadata is available in `_agenthub/metadata.json`.",
		"",
		"To sync this mirror to GitHub, run these commands in this directory:",
		"",
		"```bash",
		"git add .",
		"git commit -m \"Update AgentHub mirror\"",
		"git remote add origin <your-repo-url>",
		"git push -u origin main",
		"```",
		"",
		"Current mirror root: " + rootPath,
		"",
	}
	return strings.Join(lines, "\n")
}

func buildSuccessInfo(rootPath string, syncedAt time.Time) *SyncInfo {
	return &SyncInfo{
		Enabled:      true,
		Path:         rootPath,
		Synced:       true,
		LastSyncedAt: syncedAt.UTC().Format(time.RFC3339),
		Message:      successMessage(rootPath),
	}
}

func buildFailureInfo(rootPath string, err error) *SyncInfo {
	info := &SyncInfo{
		Enabled:   true,
		Path:      rootPath,
		Synced:    false,
		LastError: err.Error(),
	}
	if rootPath != "" {
		info.Message = fmt.Sprintf("本地 Git 目录同步失败: %s。目录: %s。", err.Error(), rootPath)
	} else {
		info.Message = fmt.Sprintf("本地 Git 目录同步失败: %s。", err.Error())
	}
	return info
}

func successMessage(rootPath string) string {
	return fmt.Sprintf("已同步到本地 Git 目录: %s。如需同步到 GitHub，请在该目录执行 git add / git commit / git remote add origin / git push。", rootPath)
}

func mirrorCreatedAt(active *models.LocalGitMirror, fallback time.Time) time.Time {
	if active != nil && !active.CreatedAt.IsZero() {
		return active.CreatedAt
	}
	return fallback
}

func samePath(left, right string) bool {
	return filepath.Clean(strings.TrimSpace(left)) == filepath.Clean(strings.TrimSpace(right))
}

func isBinaryEntry(metadata map[string]interface{}) bool {
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
