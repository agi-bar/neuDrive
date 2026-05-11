package localgitsync

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/agi-bar/neuDrive/internal/hubpath"
	"github.com/agi-bar/neuDrive/internal/models"
	"github.com/agi-bar/neuDrive/internal/runtimecfg"
	"github.com/agi-bar/neuDrive/internal/services"
	sqlitestorage "github.com/agi-bar/neuDrive/internal/storage/sqlite"
	"github.com/agi-bar/neuDrive/internal/vault"
	"github.com/google/uuid"
)

type mirrorRepo interface {
	GetActiveLocalGitMirror(ctx context.Context, userID uuid.UUID) (*models.LocalGitMirror, error)
	UpsertActiveLocalGitMirror(ctx context.Context, mirror models.LocalGitMirror) error
	ListQueuedLocalGitMirrors(ctx context.Context, executionMode string, now time.Time, limit int) ([]models.LocalGitMirror, error)
	ClaimQueuedLocalGitMirror(ctx context.Context, userID uuid.UUID, executionMode string, startedAt time.Time) (bool, error)
}

type Service struct {
	mirrors               mirrorRepo
	fileTree              *services.FileTreeService
	users                 *services.UserService
	connections           *services.ConnectionService
	projects              *services.ProjectService
	vault                 *services.VaultService
	httpClient            *http.Client
	githubAPIBaseURL      string
	githubBaseURL         string
	executionMode         string
	hostedRoot            string
	publicBaseURL         string
	gitHubAppClientID     string
	gitHubAppClientSecret string
	gitHubAppSlug         string
	stateSigningSecret    string
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
		githubBaseURL:    defaultGitHubBaseURL,
		executionMode:    ExecutionModeLocal,
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
	mirror.ExecutionMode = s.configuredExecutionMode()
	mirror.SyncState = SyncStateIdle
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

func (s *Service) SyncActiveMirror(ctx context.Context, userID uuid.UUID, forceRemoteOverwrite bool) (*SyncInfo, error) {
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
	mirror.ExecutionMode = s.configuredExecutionMode()
	if forceRemoteOverwrite {
		mirror.ForceRemoteOverwrite = true
		mirror.RemoteConflict = false
		mirror.LastPushError = ""
	}
	syncedAt := time.Now().UTC()
	gitInitializedAt, syncErr := s.syncIntoRoot(ctx, userID, mirror.RootPath, syncedAt, &mirror)
	mirror.GitInitializedAt = gitInitializedAt
	if syncErr != nil {
		mirror.SyncState = SyncStateError
		mirror.LastError = syncErr.Error()
		mirror.UpdatedAt = time.Now().UTC()
		if err := s.mirrors.UpsertActiveLocalGitMirror(ctx, mirror); err != nil {
			return buildFailureInfo(mirror, syncErr), syncErr
		}
		return buildFailureInfo(mirror, syncErr), syncErr
	}

	mirror.LastSyncedAt = &syncedAt
	mirror.LastError = ""
	mirror.SyncState = SyncStateIdle
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
	if err := writeTextFile(filepath.Join(rootPath, readmePath), buildREADME(rootPath)); err != nil {
		return nil, err
	}
	gitInitializedAt, err := ensureGitRepo(ctx, rootPath, existing)
	if err != nil {
		return nil, err
	}
	return gitInitializedAt, nil
}

func (s *Service) writeFileTree(ctx context.Context, userID uuid.UUID, rootPath string) error {
	return s.writeFileTreeDir(ctx, userID, rootPath, "/", map[string]bool{})
}

func (s *Service) writeFileTreeDir(ctx context.Context, userID uuid.UUID, rootPath, dirPath string, visited map[string]bool) error {
	dirPath = hubpath.NormalizePublic(dirPath)
	if visited[dirPath] {
		return nil
	}
	visited[dirPath] = true

	entries, err := s.fileTree.List(ctx, userID, dirPath, models.TrustLevelFull)
	if err != nil && err != services.ErrEntryNotFound {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	for _, entry := range entries {
		entry.Path = hubpath.NormalizePublic(entry.Path)
		if entry.Path == "" || entry.Path == "/" {
			continue
		}
		if entry.IsDirectory {
			if err := s.writeFileTreeDir(ctx, userID, rootPath, entry.Path, visited); err != nil {
				return err
			}
			continue
		}
		if err := s.writeFileTreeEntry(ctx, userID, rootPath, entry); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) writeFileTreeEntry(ctx context.Context, userID uuid.UUID, rootPath string, entry models.FileTreeEntry) error {
	relativePath := strings.TrimPrefix(hubpath.NormalizePublic(entry.Path), "/")
	if relativePath == "" {
		return nil
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
		return nil
	}
	if err := os.WriteFile(target, []byte(entry.Content), 0o644); err != nil {
		return err
	}
	return nil
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
		"This repository mirrors the user-visible NeuDrive file tree for this account.",
		"",
		"- Files are written with the same paths shown in NeuDrive, such as `skills/...` and `memory/...`.",
		"- Internal account metadata, connection records, and vault scopes are not exported here.",
		"- Secrets are not exported.",
		"",
		"Current mirror root: " + rootPath,
		"",
	}
	return strings.Join(lines, "\n")
}

func buildFailureInfo(mirror models.LocalGitMirror, err error) *SyncInfo {
	path := mirror.RootPath
	if mirror.ExecutionMode == ExecutionModeHosted {
		path = ""
	}
	info := &SyncInfo{
		Enabled:              true,
		Path:                 path,
		ExecutionMode:        mirror.ExecutionMode,
		SyncState:            mirror.SyncState,
		SyncRequestedAt:      formatOptionalTime(mirror.SyncRequestedAt),
		SyncStartedAt:        formatOptionalTime(mirror.SyncStartedAt),
		SyncNextAttemptAt:    formatOptionalTime(mirror.SyncNextAttemptAt),
		SyncAttemptCount:     mirror.SyncAttemptCount,
		Synced:               false,
		LastSyncedAt:         formatOptionalTime(mirror.LastSyncedAt),
		LastError:            err.Error(),
		AutoCommitEnabled:    mirror.AutoCommitEnabled,
		AutoPushEnabled:      mirror.AutoPushEnabled,
		AuthMode:             mirror.AuthMode,
		RemoteName:           mirror.RemoteName,
		RemoteBranch:         mirror.RemoteBranch,
		LastCommitAt:         formatOptionalTime(mirror.LastCommitAt),
		LastCommitHash:       strings.TrimSpace(mirror.LastCommitHash),
		LastPushAt:           formatOptionalTime(mirror.LastPushAt),
		LastPushError:        strings.TrimSpace(mirror.LastPushError),
		RemoteConflict:       mirror.RemoteConflict,
		ForceRemoteOverwrite: mirror.ForceRemoteOverwrite,
	}
	if path != "" {
		info.Message = fmt.Sprintf("Git Mirror 同步失败: %s。目录: %s。", err.Error(), path)
	} else {
		info.Message = fmt.Sprintf("Git Mirror 同步失败: %s。", err.Error())
	}
	return info
}

func (s *Service) buildSyncInfo(_ context.Context, _ uuid.UUID, mirror models.LocalGitMirror, synced, commitCreated, pushAttempted, pushSucceeded bool) *SyncInfo {
	path := mirror.RootPath
	if mirror.ExecutionMode == ExecutionModeHosted {
		path = ""
	}
	info := &SyncInfo{
		Enabled:              true,
		Path:                 path,
		ExecutionMode:        mirror.ExecutionMode,
		SyncState:            mirror.SyncState,
		SyncRequestedAt:      formatOptionalTime(mirror.SyncRequestedAt),
		SyncStartedAt:        formatOptionalTime(mirror.SyncStartedAt),
		SyncNextAttemptAt:    formatOptionalTime(mirror.SyncNextAttemptAt),
		SyncAttemptCount:     mirror.SyncAttemptCount,
		Synced:               synced,
		LastSyncedAt:         formatOptionalTime(mirror.LastSyncedAt),
		LastError:            strings.TrimSpace(mirror.LastError),
		AutoCommitEnabled:    mirror.AutoCommitEnabled,
		AutoPushEnabled:      mirror.AutoPushEnabled,
		AuthMode:             mirror.AuthMode,
		RemoteName:           mirror.RemoteName,
		RemoteBranch:         mirror.RemoteBranch,
		LastCommitAt:         formatOptionalTime(mirror.LastCommitAt),
		LastCommitHash:       strings.TrimSpace(mirror.LastCommitHash),
		LastPushAt:           formatOptionalTime(mirror.LastPushAt),
		LastPushError:        strings.TrimSpace(mirror.LastPushError),
		RemoteConflict:       mirror.RemoteConflict,
		ForceRemoteOverwrite: mirror.ForceRemoteOverwrite,
		CommitCreated:        commitCreated,
		PushAttempted:        pushAttempted,
		PushSucceeded:        pushSucceeded,
		Message:              mirrorSummaryMessage(mirror, commitCreated, pushAttempted, pushSucceeded),
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
