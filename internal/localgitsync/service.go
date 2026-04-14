package localgitsync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/runtimecfg"
	"github.com/agi-bar/neudrive/internal/services"
	sqlitestorage "github.com/agi-bar/neudrive/internal/storage/sqlite"
	"github.com/agi-bar/neudrive/internal/systemskills"
	"github.com/agi-bar/neudrive/internal/vault"
	"github.com/google/uuid"
)

type mirrorRepo interface {
	GetActiveLocalGitMirror(ctx context.Context, userID uuid.UUID) (*models.LocalGitMirror, error)
	UpsertActiveLocalGitMirror(ctx context.Context, mirror models.LocalGitMirror) error
}

type Service struct {
	mirrors          mirrorRepo
	fileTree         *services.FileTreeService
	users            *services.UserService
	connections      *services.ConnectionService
	projects         *services.ProjectService
	vault            *services.VaultService
	httpClient       *http.Client
	githubAPIBaseURL string
}

func New(store *sqlitestorage.Store, vaultCrypto *vault.Vault, opts ...Option) *Service {
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
		opts...,
	)
}

func NewWithDeps(
	mirrors mirrorRepo,
	fileTree *services.FileTreeService,
	users *services.UserService,
	connections *services.ConnectionService,
	projects *services.ProjectService,
	vaultSvc *services.VaultService,
	opts ...Option,
) *Service {
	if mirrors == nil || fileTree == nil {
		return nil
	}
	svc := &Service{
		mirrors:          mirrors,
		fileTree:         fileTree,
		users:            users,
		connections:      connections,
		projects:         projects,
		vault:            vaultSvc,
		httpClient:       http.DefaultClient,
		githubAPIBaseURL: defaultGitHubAPIBaseURL,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

func DefaultMirrorRoot() string {
	return runtimecfg.DefaultGitMirrorPath
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

	mirror := normalizeMirror(active)
	mirror.UserID = userID
	mirror.RootPath = rootPath
	mirror.IsActive = true
	mirror.GitInitializedAt = gitInitializedAt
	mirror.LastSyncedAt = &syncedAt
	mirror.LastError = ""
	mirror.CreatedAt = mirrorCreatedAt(active, syncedAt)
	mirror.UpdatedAt = syncedAt

	if err := s.mirrors.UpsertActiveLocalGitMirror(ctx, mirror); err != nil {
		return nil, err
	}

	return s.buildSyncInfo(ctx, userID, mirror, true, false, false, false), nil
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

	mirror := normalizeMirror(active)
	syncedAt := time.Now().UTC()
	gitInitializedAt, syncErr := s.syncIntoRoot(ctx, userID, mirror.RootPath, syncedAt, &mirror)
	mirror.GitInitializedAt = gitInitializedAt
	if syncErr != nil {
		mirror.LastError = syncErr.Error()
		mirror.UpdatedAt = time.Now().UTC()
		if err := s.mirrors.UpsertActiveLocalGitMirror(ctx, mirror); err != nil {
			return buildFailureInfo(mirror, syncErr), syncErr
		}
		return buildFailureInfo(mirror, syncErr), syncErr
	}

	mirror.LastSyncedAt = &syncedAt
	mirror.LastError = ""
	result, err := s.finalizeMirrorRepo(ctx, userID, &mirror)
	mirror.UpdatedAt = time.Now().UTC()
	if err != nil {
		mirror.LastError = err.Error()
		if persistErr := s.mirrors.UpsertActiveLocalGitMirror(ctx, mirror); persistErr != nil {
			return buildFailureInfo(mirror, err), err
		}
		return buildFailureInfo(mirror, err), err
	}
	if persistErr := s.mirrors.UpsertActiveLocalGitMirror(ctx, mirror); persistErr != nil {
		return buildFailureInfo(mirror, persistErr), persistErr
	}

	return s.buildSyncInfo(ctx, userID, mirror, true, result.commitCreated, result.pushAttempted, result.pushSucceeded), nil
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
			if err := writeJSONFile(filepath.Join(rootPath, "_neudrive", "projects.json"), exported); err != nil {
				return err
			}
		}
	}
	if err := writeJSONFile(filepath.Join(rootPath, "_neudrive", "metadata.json"), map[string]any{
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
		target = DefaultMirrorRoot()
	}
	return filepath.Abs(expandUser(target))
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
	cmd.Env = gitCommandEnv(nil)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git init failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	now := time.Now().UTC()
	return &now, nil
}

func scrubGitEnv(env []string) []string {
	clean := make([]string, 0, len(env))
	for _, entry := range env {
		key := entry
		if idx := strings.IndexByte(entry, '='); idx >= 0 {
			key = entry[:idx]
		}
		if strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			continue
		}
		clean = append(clean, entry)
	}
	return clean
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
		"# NeuDrive Local Git Mirror",
		"",
		"This directory is a local Git mirror of your NeuDrive data.",
		"",
		"- Secrets are not exported.",
		"- Vault scope metadata is available in `vault/scopes.json`.",
		"- Mirror metadata is available in `_neudrive/metadata.json`.",
		"",
		"To sync this mirror to GitHub, run these commands in this directory:",
		"",
		"```bash",
		"git add .",
		"git commit -m \"Update NeuDrive mirror\"",
		"git remote add origin <your-repo-url>",
		"git push -u origin main",
		"```",
		"",
		"Current mirror root: " + rootPath,
		"",
	}
	return strings.Join(lines, "\n")
}

func buildFailureInfo(mirror models.LocalGitMirror, err error) *SyncInfo {
	info := &SyncInfo{
		Enabled:           true,
		Path:              mirror.RootPath,
		Synced:            false,
		LastSyncedAt:      formatOptionalTime(mirror.LastSyncedAt),
		LastError:         err.Error(),
		AutoCommitEnabled: mirror.AutoCommitEnabled,
		AutoPushEnabled:   mirror.AutoPushEnabled,
		AuthMode:          mirror.AuthMode,
		RemoteName:        mirror.RemoteName,
		RemoteBranch:      mirror.RemoteBranch,
		LastCommitAt:      formatOptionalTime(mirror.LastCommitAt),
		LastCommitHash:    strings.TrimSpace(mirror.LastCommitHash),
		LastPushAt:        formatOptionalTime(mirror.LastPushAt),
		LastPushError:     strings.TrimSpace(mirror.LastPushError),
	}
	if mirror.RootPath != "" {
		info.Message = fmt.Sprintf("本地 Git 目录同步失败: %s。目录: %s。", err.Error(), mirror.RootPath)
	} else {
		info.Message = fmt.Sprintf("本地 Git 目录同步失败: %s。", err.Error())
	}
	return info
}

func (s *Service) buildSyncInfo(_ context.Context, _ uuid.UUID, mirror models.LocalGitMirror, synced, commitCreated, pushAttempted, pushSucceeded bool) *SyncInfo {
	info := &SyncInfo{
		Enabled:           true,
		Path:              mirror.RootPath,
		Synced:            synced,
		LastSyncedAt:      formatOptionalTime(mirror.LastSyncedAt),
		LastError:         strings.TrimSpace(mirror.LastError),
		AutoCommitEnabled: mirror.AutoCommitEnabled,
		AutoPushEnabled:   mirror.AutoPushEnabled,
		AuthMode:          mirror.AuthMode,
		RemoteName:        mirror.RemoteName,
		RemoteBranch:      mirror.RemoteBranch,
		LastCommitAt:      formatOptionalTime(mirror.LastCommitAt),
		LastCommitHash:    strings.TrimSpace(mirror.LastCommitHash),
		LastPushAt:        formatOptionalTime(mirror.LastPushAt),
		LastPushError:     strings.TrimSpace(mirror.LastPushError),
		CommitCreated:     commitCreated,
		PushAttempted:     pushAttempted,
		PushSucceeded:     pushSucceeded,
		Message:           mirrorSummaryMessage(mirror, commitCreated, pushAttempted, pushSucceeded),
	}
	return info
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

func expandUser(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
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
