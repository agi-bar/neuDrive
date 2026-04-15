package localgitsync

import (
	"net/http"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
)

const (
	readmePath               = "README.md"
	AuthModeLocalCredentials = "local_credentials"
	AuthModeGitHubToken      = "github_token"
	AuthModeGitHubAppUser    = "github_app_user"
	ExecutionModeLocal       = "local"
	ExecutionModeHosted      = "hosted"
	SyncStateIdle            = "idle"
	SyncStateQueued          = "queued"
	SyncStateRunning         = "running"
	SyncStateError           = "error"
	DefaultRemoteName        = "origin"
	DefaultRemoteBranch      = "main"

	gitMirrorGitHubTokenScope           = "auth.github.git_mirror"
	gitMirrorGitHubAppRefreshTokenScope = "auth.github.git_mirror.app_user_refresh_token"
	defaultGitHubAPIBaseURL             = "https://api.github.com"
	defaultGitHubBaseURL                = "https://github.com"
	commitAuthorName                    = "NeuDrive Mirror"
	commitAuthorEmail                   = "neudrive-mirror@local"
)

type Option func(*Service)

func WithGitHubAPIBaseURL(baseURL string) Option {
	return func(s *Service) {
		if trimmed := strings.TrimSpace(baseURL); trimmed != "" {
			s.githubAPIBaseURL = strings.TrimRight(trimmed, "/")
		}
	}
}

func WithGitHubBaseURL(baseURL string) Option {
	return func(s *Service) {
		if trimmed := strings.TrimSpace(baseURL); trimmed != "" {
			s.githubBaseURL = strings.TrimRight(trimmed, "/")
		}
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(s *Service) {
		if client != nil {
			s.httpClient = client
		}
	}
}

func WithExecutionMode(mode string) Option {
	return func(s *Service) {
		switch strings.TrimSpace(mode) {
		case ExecutionModeHosted:
			s.executionMode = ExecutionModeHosted
		default:
			s.executionMode = ExecutionModeLocal
		}
	}
}

func WithHostedRoot(root string) Option {
	return func(s *Service) {
		s.hostedRoot = strings.TrimSpace(root)
	}
}

func WithGitMirrorPublicBaseURL(baseURL string) Option {
	return func(s *Service) {
		s.publicBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
}

func WithGitHubAppConfig(clientID, clientSecret, slug string) Option {
	return func(s *Service) {
		s.gitHubAppClientID = strings.TrimSpace(clientID)
		s.gitHubAppClientSecret = strings.TrimSpace(clientSecret)
		s.gitHubAppSlug = strings.TrimSpace(slug)
	}
}

func WithStateSigningSecret(secret string) Option {
	return func(s *Service) {
		s.stateSigningSecret = strings.TrimSpace(secret)
	}
}

type SyncInfo struct {
	Enabled           bool   `json:"enabled"`
	Path              string `json:"path,omitempty"`
	ExecutionMode     string `json:"execution_mode,omitempty"`
	SyncState         string `json:"sync_state,omitempty"`
	SyncRequestedAt   string `json:"sync_requested_at,omitempty"`
	SyncStartedAt     string `json:"sync_started_at,omitempty"`
	SyncNextAttemptAt string `json:"sync_next_attempt_at,omitempty"`
	SyncAttemptCount  int    `json:"sync_attempt_count,omitempty"`
	Synced            bool   `json:"synced"`
	LastSyncedAt      string `json:"last_synced_at,omitempty"`
	Message           string `json:"message,omitempty"`
	LastError         string `json:"last_error,omitempty"`
	AutoCommitEnabled bool   `json:"auto_commit_enabled,omitempty"`
	AutoPushEnabled   bool   `json:"auto_push_enabled,omitempty"`
	AuthMode          string `json:"auth_mode,omitempty"`
	RemoteName        string `json:"remote_name,omitempty"`
	RemoteBranch      string `json:"remote_branch,omitempty"`
	LastCommitAt      string `json:"last_commit_at,omitempty"`
	LastCommitHash    string `json:"last_commit_hash,omitempty"`
	LastPushAt        string `json:"last_push_at,omitempty"`
	LastPushError     string `json:"last_push_error,omitempty"`
	CommitCreated     bool   `json:"commit_created,omitempty"`
	PushAttempted     bool   `json:"push_attempted,omitempty"`
	PushSucceeded     bool   `json:"push_succeeded,omitempty"`
}

type MirrorSettings struct {
	Enabled                       bool   `json:"enabled"`
	Path                          string `json:"path,omitempty"`
	ExecutionMode                 string `json:"execution_mode,omitempty"`
	SyncState                     string `json:"sync_state,omitempty"`
	SyncRequestedAt               string `json:"sync_requested_at,omitempty"`
	SyncStartedAt                 string `json:"sync_started_at,omitempty"`
	SyncNextAttemptAt             string `json:"sync_next_attempt_at,omitempty"`
	SyncAttemptCount              int    `json:"sync_attempt_count,omitempty"`
	AutoCommitEnabled             bool   `json:"auto_commit_enabled"`
	AutoPushEnabled               bool   `json:"auto_push_enabled"`
	AuthMode                      string `json:"auth_mode"`
	RemoteName                    string `json:"remote_name"`
	RemoteURL                     string `json:"remote_url,omitempty"`
	RemoteBranch                  string `json:"remote_branch"`
	LastSyncedAt                  string `json:"last_synced_at,omitempty"`
	LastError                     string `json:"last_error,omitempty"`
	LastCommitAt                  string `json:"last_commit_at,omitempty"`
	LastCommitHash                string `json:"last_commit_hash,omitempty"`
	LastPushAt                    string `json:"last_push_at,omitempty"`
	LastPushError                 string `json:"last_push_error,omitempty"`
	GitHubTokenConfigured         bool   `json:"github_token_configured"`
	GitHubTokenVerifiedAt         string `json:"github_token_verified_at,omitempty"`
	GitHubTokenLogin              string `json:"github_token_login,omitempty"`
	GitHubRepoPermission          string `json:"github_repo_permission,omitempty"`
	GitHubAppUserConnected        bool   `json:"github_app_user_connected"`
	GitHubAppUserLogin            string `json:"github_app_user_login,omitempty"`
	GitHubAppUserAuthorizedAt     string `json:"github_app_user_authorized_at,omitempty"`
	GitHubAppUserRefreshExpiresAt string `json:"github_app_user_refresh_expires_at,omitempty"`
	Message                       string `json:"message,omitempty"`
}

type MirrorSettingsUpdate struct {
	AutoCommitEnabled bool   `json:"auto_commit_enabled"`
	AutoPushEnabled   bool   `json:"auto_push_enabled"`
	AuthMode          string `json:"auth_mode"`
	RemoteName        string `json:"remote_name,omitempty"`
	RemoteURL         string `json:"remote_url,omitempty"`
	RemoteBranch      string `json:"remote_branch,omitempty"`
	GitHubToken       string `json:"github_token,omitempty"`
	ClearGitHubToken  bool   `json:"clear_github_token,omitempty"`
}

type GitHubAppBrowserStartResult struct {
	AuthorizationURL string `json:"authorization_url"`
}

type GitHubAppDeviceStartResult struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	Interval        int    `json:"interval,omitempty"`
}

type GitHubAppDevicePollResult struct {
	Connected             bool   `json:"connected"`
	Pending               bool   `json:"pending,omitempty"`
	Message               string `json:"message,omitempty"`
	GitHubAppUserLogin    string `json:"github_app_user_login,omitempty"`
	GitHubAppAuthorizedAt string `json:"github_app_user_authorized_at,omitempty"`
}

type GitHubMirrorRepo struct {
	OwnerLogin       string `json:"owner_login"`
	OwnerType        string `json:"owner_type"`
	RepoName         string `json:"repo_name"`
	FullName         string `json:"full_name"`
	DefaultBranch    string `json:"default_branch,omitempty"`
	CloneURL         string `json:"clone_url"`
	ViewerPermission string `json:"viewer_permission,omitempty"`
}

type GitHubMirrorRepoCreateRequest struct {
	OwnerLogin   string `json:"owner_login"`
	RepoName     string `json:"repo_name"`
	Description  string `json:"description,omitempty"`
	Private      bool   `json:"private"`
	RemoteName   string `json:"remote_name,omitempty"`
	RemoteBranch string `json:"remote_branch,omitempty"`
}

type GitHubTokenTestResult struct {
	OK                  bool   `json:"ok"`
	Login               string `json:"login,omitempty"`
	Repo                string `json:"repo,omitempty"`
	NormalizedRemoteURL string `json:"normalized_remote_url,omitempty"`
	Permission          string `json:"permission,omitempty"`
	Message             string `json:"message,omitempty"`
}

type repoSyncResult struct {
	gitInitializedAt *time.Time
	commitCreated    bool
	pushAttempted    bool
	pushSucceeded    bool
}

type gitHubRepoAccessResult struct {
	OK                  bool
	Login               string
	Repo                string
	NormalizedRemoteURL string
	Permission          string
	Message             string
}

func normalizeMirror(mirror *models.LocalGitMirror) models.LocalGitMirror {
	if mirror == nil {
		return models.LocalGitMirror{
			ExecutionMode: ExecutionModeLocal,
			SyncState:     SyncStateIdle,
			AuthMode:      AuthModeLocalCredentials,
			RemoteName:    DefaultRemoteName,
			RemoteBranch:  DefaultRemoteBranch,
		}
	}
	normalized := *mirror
	if strings.TrimSpace(normalized.ExecutionMode) == "" {
		normalized.ExecutionMode = ExecutionModeLocal
	}
	if strings.TrimSpace(normalized.SyncState) == "" {
		normalized.SyncState = SyncStateIdle
	}
	if strings.TrimSpace(normalized.AuthMode) == "" {
		normalized.AuthMode = AuthModeLocalCredentials
	}
	if strings.TrimSpace(normalized.RemoteName) == "" {
		normalized.RemoteName = DefaultRemoteName
	}
	if strings.TrimSpace(normalized.RemoteBranch) == "" {
		normalized.RemoteBranch = DefaultRemoteBranch
	}
	normalized.RemoteURL = strings.TrimSpace(normalized.RemoteURL)
	return normalized
}

func canPushPermission(permission string) bool {
	switch strings.TrimSpace(permission) {
	case "admin", "write":
		return true
	default:
		return false
	}
}
