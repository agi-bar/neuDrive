package localgitsync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
)

type gitHubUserResponse struct {
	Login string `json:"login"`
}

type gitHubRepoResponse struct {
	FullName    string `json:"full_name"`
	Permissions struct {
		Admin bool `json:"admin"`
		Push  bool `json:"push"`
		Pull  bool `json:"pull"`
	} `json:"permissions"`
}

func (s *Service) GetMirrorSettings(ctx context.Context, userID uuid.UUID) (*MirrorSettings, error) {
	if s == nil || s.mirrors == nil {
		return nil, fmt.Errorf("local git sync not configured")
	}
	active, err := s.mirrors.GetActiveLocalGitMirror(ctx, userID)
	if err != nil {
		return nil, err
	}
	tokenConfigured, err := s.hasStoredGitHubToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	appConnected, err := s.hasStoredGitHubAppRefreshToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	if active == nil || strings.TrimSpace(active.RootPath) == "" {
		return &MirrorSettings{
			Enabled:                false,
			ExecutionMode:          s.configuredExecutionMode(),
			SyncState:              SyncStateIdle,
			AuthMode:               defaultAuthModeForExecution(s.configuredExecutionMode()),
			RemoteName:             DefaultRemoteName,
			RemoteBranch:           DefaultRemoteBranch,
			GitHubTokenConfigured:  tokenConfigured,
			GitHubAppUserConnected: appConnected,
		}, nil
	}
	return buildMirrorSettings(*active, tokenConfigured, appConnected, s.configuredExecutionMode()), nil
}

func (s *Service) UpdateMirrorSettings(ctx context.Context, userID uuid.UUID, update MirrorSettingsUpdate) (*MirrorSettings, error) {
	if s == nil || s.mirrors == nil {
		return nil, fmt.Errorf("local git sync not configured")
	}
	mirror, active, err := s.ensureMirror(ctx, userID)
	if err != nil {
		return nil, err
	}
	if s.configuredExecutionMode() == ExecutionModeLocal && (active == nil || strings.TrimSpace(active.RootPath) == "") {
		return nil, fmt.Errorf("no local Git mirror is configured; run `neudrive git init` first")
	}
	if err := applySettingsUpdate(&mirror, update); err != nil {
		return nil, err
	}

	tokenChanged := strings.TrimSpace(update.GitHubToken) != ""
	tokenValue, tokenConfigured, err := s.resolveGitHubTokenForSettings(ctx, userID, update)
	if err != nil {
		return nil, err
	}
	if mirror.AutoPushEnabled && !mirror.AutoCommitEnabled {
		return nil, fmt.Errorf("auto push requires auto commit to be enabled")
	}
	if mirror.AuthMode == AuthModeGitHubToken && mirror.AutoPushEnabled {
		if strings.TrimSpace(mirror.RemoteURL) == "" {
			return nil, fmt.Errorf("a GitHub repo URL is required before enabling auto push")
		}
		if !tokenConfigured {
			return nil, fmt.Errorf("a verified GitHub token is required before enabling auto push")
		}
	}
	if mirror.AuthMode == AuthModeGitHubAppUser && mirror.AutoPushEnabled {
		if strings.TrimSpace(mirror.RemoteURL) == "" {
			return nil, fmt.Errorf("a GitHub repo URL is required before enabling auto push")
		}
		appConnected, err := s.hasStoredGitHubAppRefreshToken(ctx, userID)
		if err != nil {
			return nil, err
		}
		if !appConnected {
			return nil, fmt.Errorf("connect the GitHub App account before enabling auto push")
		}
	}

	verificationNeeded := mirror.AuthMode == AuthModeGitHubToken &&
		strings.TrimSpace(mirror.RemoteURL) != "" &&
		tokenConfigured &&
		(tokenChanged || settingsRemoteChanged(active, mirror) || mirror.AutoPushEnabled)

	if mirror.AuthMode != AuthModeGitHubToken || !tokenConfigured || strings.TrimSpace(mirror.RemoteURL) == "" {
		clearGitHubVerification(&mirror)
	}
	if verificationNeeded {
		result, err := s.testGitHubToken(ctx, mirror.RemoteURL, tokenValue)
		if err != nil {
			return nil, err
		}
		if !result.OK {
			clearGitHubVerification(&mirror)
			if mirror.AutoPushEnabled {
				return nil, fmt.Errorf("%s", result.Message)
			}
		} else {
			now := time.Now().UTC()
			mirror.RemoteURL = result.NormalizedRemoteURL
			mirror.GitHubTokenVerifiedAt = &now
			mirror.GitHubTokenLogin = result.Login
			mirror.GitHubRepoPermission = result.Permission
		}
	}
	if mirror.AuthMode == AuthModeGitHubToken && mirror.AutoPushEnabled && !canPushPermission(mirror.GitHubRepoPermission) {
		return nil, fmt.Errorf("the configured GitHub token does not have push access to this repository")
	}

	if tokenChanged && mirror.AuthMode != AuthModeGitHubToken {
		return nil, fmt.Errorf("GitHub token mode must be selected before saving a GitHub token")
	}
	if update.ClearGitHubToken {
		if err := s.deleteStoredGitHubToken(ctx, userID); err != nil {
			return nil, err
		}
		tokenConfigured = false
		clearGitHubVerification(&mirror)
	} else if tokenChanged {
		if err := s.writeStoredGitHubToken(ctx, userID, tokenValue); err != nil {
			return nil, err
		}
		tokenConfigured = true
	}

	now := time.Now().UTC()
	mirror.UserID = userID
	mirror.IsActive = true
	mirror.ExecutionMode = s.configuredExecutionMode()
	mirror.CreatedAt = mirrorCreatedAt(active, now)
	mirror.UpdatedAt = now
	if err := s.mirrors.UpsertActiveLocalGitMirror(ctx, mirror); err != nil {
		return nil, err
	}

	appConnected, err := s.hasStoredGitHubAppRefreshToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	return buildMirrorSettings(mirror, tokenConfigured, appConnected, s.configuredExecutionMode()), nil
}

func (s *Service) TestGitHubToken(ctx context.Context, userID uuid.UUID, remoteURL, token string) (*GitHubTokenTestResult, error) {
	if s == nil || s.mirrors == nil {
		return nil, fmt.Errorf("local git sync not configured")
	}
	targetRemote := strings.TrimSpace(remoteURL)
	if targetRemote == "" {
		active, err := s.mirrors.GetActiveLocalGitMirror(ctx, userID)
		if err != nil {
			return nil, err
		}
		if active != nil {
			targetRemote = strings.TrimSpace(active.RemoteURL)
		}
	}
	if targetRemote == "" {
		return nil, fmt.Errorf("a GitHub repo URL is required")
	}
	return s.testGitHubToken(ctx, targetRemote, token)
}

func (s *Service) testGitHubToken(ctx context.Context, remoteURL, token string) (*GitHubTokenTestResult, error) {
	normalizedRemote, owner, repo, err := normalizeGitHubRemoteURL(remoteURL)
	if err != nil {
		return nil, err
	}
	trimmedToken := strings.TrimSpace(token)
	if trimmedToken == "" {
		return &GitHubTokenTestResult{
			OK:                  false,
			Repo:                owner + "/" + repo,
			NormalizedRemoteURL: normalizedRemote,
			Message:             "GitHub token is required.",
		}, nil
	}

	var user gitHubUserResponse
	if err := s.githubRequestJSON(ctx, trimmedToken, "/user", &user); err != nil {
		return &GitHubTokenTestResult{
			OK:                  false,
			Repo:                owner + "/" + repo,
			NormalizedRemoteURL: normalizedRemote,
			Message:             "GitHub token validation failed. Please check the token and try again.",
		}, nil
	}

	var repoResp gitHubRepoResponse
	if err := s.githubRequestJSON(ctx, trimmedToken, "/repos/"+owner+"/"+repo, &repoResp); err != nil {
		return &GitHubTokenTestResult{
			OK:                  false,
			Login:               user.Login,
			Repo:                owner + "/" + repo,
			NormalizedRemoteURL: normalizedRemote,
			Message:             "GitHub token cannot access this repository.",
		}, nil
	}

	permission := githubPermissionFromRepo(repoResp)
	result := &GitHubTokenTestResult{
		OK:                  canPushPermission(permission),
		Login:               user.Login,
		Repo:                repoResp.FullName,
		NormalizedRemoteURL: normalizedRemote,
		Permission:          permission,
	}
	if result.OK {
		result.Message = "GitHub token is valid and has push access to this repository."
		return result, nil
	}
	result.Message = "GitHub token is valid, but it does not have push access to this repository."
	return result, nil
}

func (s *Service) githubRequestJSON(ctx context.Context, token, apiPath string, out any) error {
	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	baseURL := strings.TrimRight(s.githubAPIBaseURL, "/")
	if baseURL == "" {
		baseURL = defaultGitHubAPIBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+apiPath, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github api returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func buildMirrorSettings(mirror models.LocalGitMirror, tokenConfigured, appConnected bool, executionMode string) *MirrorSettings {
	normalized := normalizeMirror(&mirror)
	path := normalized.RootPath
	if executionMode == ExecutionModeHosted {
		path = ""
	}
	return &MirrorSettings{
		Enabled:                       true,
		Path:                          path,
		ExecutionMode:                 executionMode,
		SyncState:                     normalized.SyncState,
		AutoCommitEnabled:             normalized.AutoCommitEnabled,
		AutoPushEnabled:               normalized.AutoPushEnabled,
		AuthMode:                      normalized.AuthMode,
		RemoteName:                    normalized.RemoteName,
		RemoteURL:                     normalized.RemoteURL,
		RemoteBranch:                  normalized.RemoteBranch,
		LastSyncedAt:                  formatOptionalTime(normalized.LastSyncedAt),
		LastError:                     strings.TrimSpace(normalized.LastError),
		LastCommitAt:                  formatOptionalTime(normalized.LastCommitAt),
		LastCommitHash:                strings.TrimSpace(normalized.LastCommitHash),
		LastPushAt:                    formatOptionalTime(normalized.LastPushAt),
		LastPushError:                 strings.TrimSpace(normalized.LastPushError),
		GitHubTokenConfigured:         tokenConfigured,
		GitHubTokenVerifiedAt:         formatOptionalTime(normalized.GitHubTokenVerifiedAt),
		GitHubTokenLogin:              strings.TrimSpace(normalized.GitHubTokenLogin),
		GitHubRepoPermission:          strings.TrimSpace(normalized.GitHubRepoPermission),
		GitHubAppUserConnected:        appConnected,
		GitHubAppUserLogin:            strings.TrimSpace(normalized.GitHubAppUserLogin),
		GitHubAppUserAuthorizedAt:     formatOptionalTime(normalized.GitHubAppAuthorizedAt),
		GitHubAppUserRefreshExpiresAt: formatOptionalTime(normalized.GitHubAppRefreshExpiresAt),
		Message:                       mirrorSummaryMessage(normalized, false, false, false),
	}
}

func applySettingsUpdate(mirror *models.LocalGitMirror, update MirrorSettingsUpdate) error {
	if mirror == nil {
		return fmt.Errorf("missing local git mirror")
	}
	nextAuthMode := strings.TrimSpace(update.AuthMode)
	if nextAuthMode == "" {
		nextAuthMode = AuthModeLocalCredentials
	}
	switch nextAuthMode {
	case AuthModeLocalCredentials, AuthModeGitHubToken, AuthModeGitHubAppUser:
	default:
		return fmt.Errorf("unsupported auth mode %q", nextAuthMode)
	}
	if mirror.ExecutionMode == ExecutionModeHosted && nextAuthMode == AuthModeLocalCredentials {
		return fmt.Errorf("local credentials auth is only available in local mode")
	}
	mirror.AutoCommitEnabled = update.AutoCommitEnabled
	mirror.AutoPushEnabled = update.AutoPushEnabled
	mirror.AuthMode = nextAuthMode
	mirror.RemoteName = strings.TrimSpace(update.RemoteName)
	mirror.RemoteURL = strings.TrimSpace(update.RemoteURL)
	mirror.RemoteBranch = strings.TrimSpace(update.RemoteBranch)
	normalized := normalizeMirror(mirror)
	*mirror = normalized
	return nil
}

func (s *Service) resolveGitHubTokenForSettings(ctx context.Context, userID uuid.UUID, update MirrorSettingsUpdate) (string, bool, error) {
	if update.ClearGitHubToken {
		return "", false, nil
	}
	if trimmed := strings.TrimSpace(update.GitHubToken); trimmed != "" {
		return trimmed, true, nil
	}
	return s.readStoredGitHubToken(ctx, userID)
}

func (s *Service) hasStoredGitHubToken(ctx context.Context, userID uuid.UUID) (bool, error) {
	if s == nil || s.vault == nil {
		return false, nil
	}
	scopes, err := s.vault.ListScopes(ctx, userID, models.TrustLevelFull)
	if err != nil {
		return false, err
	}
	for _, scope := range scopes {
		if scope.Scope == gitMirrorGitHubTokenScope {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) readStoredGitHubToken(ctx context.Context, userID uuid.UUID) (string, bool, error) {
	configured, err := s.hasStoredGitHubToken(ctx, userID)
	if err != nil || !configured {
		return "", configured, err
	}
	if s == nil || s.vault == nil {
		return "", false, nil
	}
	token, err := s.vault.Read(ctx, userID, gitMirrorGitHubTokenScope, models.TrustLevelFull)
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(token), true, nil
}

func (s *Service) writeStoredGitHubToken(ctx context.Context, userID uuid.UUID, token string) error {
	if s == nil || s.vault == nil {
		return fmt.Errorf("vault service not configured")
	}
	return s.vault.Write(ctx, userID, gitMirrorGitHubTokenScope, strings.TrimSpace(token), "GitHub token for local Git mirror auto-push", models.TrustLevelFull)
}

func (s *Service) deleteStoredGitHubToken(ctx context.Context, userID uuid.UUID) error {
	if s == nil || s.vault == nil {
		return nil
	}
	configured, err := s.hasStoredGitHubToken(ctx, userID)
	if err != nil || !configured {
		return err
	}
	return s.vault.Delete(ctx, userID, gitMirrorGitHubTokenScope)
}

func clearGitHubVerification(mirror *models.LocalGitMirror) {
	if mirror == nil {
		return
	}
	mirror.GitHubTokenVerifiedAt = nil
	mirror.GitHubTokenLogin = ""
	mirror.GitHubRepoPermission = ""
}

func settingsRemoteChanged(existing *models.LocalGitMirror, next models.LocalGitMirror) bool {
	if existing == nil {
		return strings.TrimSpace(next.RemoteURL) != ""
	}
	return strings.TrimSpace(existing.RemoteURL) != strings.TrimSpace(next.RemoteURL)
}

func formatOptionalTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func githubPermissionFromRepo(repo gitHubRepoResponse) string {
	switch {
	case repo.Permissions.Admin:
		return "admin"
	case repo.Permissions.Push:
		return "write"
	case repo.Permissions.Pull:
		return "read"
	default:
		return "none"
	}
}

func normalizeGitHubRemoteURL(remoteURL string) (normalizedURL, owner, repo string, err error) {
	trimmed := strings.TrimSpace(remoteURL)
	if trimmed == "" {
		return "", "", "", fmt.Errorf("a GitHub repo URL is required")
	}
	switch {
	case strings.HasPrefix(trimmed, "git@github.com:"):
		trimmed = "https://github.com/" + strings.TrimPrefix(trimmed, "git@github.com:")
	case strings.HasPrefix(trimmed, "ssh://git@github.com/"):
		trimmed = "https://github.com/" + strings.TrimPrefix(trimmed, "ssh://git@github.com/")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", "", "", err
	}
	if host := strings.ToLower(strings.TrimSpace(parsed.Host)); host != "github.com" {
		return "", "", "", fmt.Errorf("GitHub token mode only supports github.com repositories")
	}
	path := strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
	segments := strings.Split(path, "/")
	if len(segments) < 2 || segments[0] == "" || segments[1] == "" {
		return "", "", "", fmt.Errorf("invalid GitHub repository URL")
	}
	owner = segments[0]
	repo = segments[1]
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo), owner, repo, nil
}

func mirrorSummaryMessage(mirror models.LocalGitMirror, commitCreated, pushAttempted, pushSucceeded bool) string {
	base := fmt.Sprintf("已同步到本地 Git 目录: %s。", mirror.RootPath)
	parts := []string{base}
	if commitCreated && strings.TrimSpace(mirror.LastCommitHash) != "" {
		parts = append(parts, fmt.Sprintf("已自动提交 %s。", shortCommitHash(mirror.LastCommitHash)))
	} else if mirror.AutoCommitEnabled {
		parts = append(parts, "已启用自动 commit。")
	}
	if pushAttempted {
		if pushSucceeded {
			parts = append(parts, fmt.Sprintf("已自动推送到 %s/%s。", mirror.RemoteName, mirror.RemoteBranch))
		} else if strings.TrimSpace(mirror.LastPushError) != "" {
			parts = append(parts, fmt.Sprintf("自动推送失败: %s。", mirror.LastPushError))
		}
	} else if mirror.AutoPushEnabled && strings.TrimSpace(mirror.LastPushError) != "" {
		parts = append(parts, fmt.Sprintf("最近一次自动推送失败: %s。", mirror.LastPushError))
	}
	return strings.Join(parts, "")
}

func shortCommitHash(hash string) string {
	trimmed := strings.TrimSpace(hash)
	if len(trimmed) <= 8 {
		return trimmed
	}
	return trimmed[:8]
}
