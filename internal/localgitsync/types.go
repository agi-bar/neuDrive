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
	DefaultRemoteName        = "origin"
	DefaultRemoteBranch      = "main"

	gitMirrorGitHubTokenScope = "auth.github.git_mirror"
	defaultGitHubAPIBaseURL   = "https://api.github.com"
	commitAuthorName          = "NeuDrive Mirror"
	commitAuthorEmail         = "neudrive-mirror@local"
)

type Option func(*Service)

func WithGitHubAPIBaseURL(baseURL string) Option {
	return func(s *Service) {
		if trimmed := strings.TrimSpace(baseURL); trimmed != "" {
			s.githubAPIBaseURL = strings.TrimRight(trimmed, "/")
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

type SyncInfo struct {
	Enabled           bool   `json:"enabled"`
	Path              string `json:"path,omitempty"`
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
	Enabled               bool   `json:"enabled"`
	Path                  string `json:"path,omitempty"`
	AutoCommitEnabled     bool   `json:"auto_commit_enabled"`
	AutoPushEnabled       bool   `json:"auto_push_enabled"`
	AuthMode              string `json:"auth_mode"`
	RemoteName            string `json:"remote_name"`
	RemoteURL             string `json:"remote_url,omitempty"`
	RemoteBranch          string `json:"remote_branch"`
	LastSyncedAt          string `json:"last_synced_at,omitempty"`
	LastError             string `json:"last_error,omitempty"`
	LastCommitAt          string `json:"last_commit_at,omitempty"`
	LastCommitHash        string `json:"last_commit_hash,omitempty"`
	LastPushAt            string `json:"last_push_at,omitempty"`
	LastPushError         string `json:"last_push_error,omitempty"`
	GitHubTokenConfigured bool   `json:"github_token_configured"`
	GitHubTokenVerifiedAt string `json:"github_token_verified_at,omitempty"`
	GitHubTokenLogin      string `json:"github_token_login,omitempty"`
	GitHubRepoPermission  string `json:"github_repo_permission,omitempty"`
	Message               string `json:"message,omitempty"`
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

func normalizeMirror(mirror *models.LocalGitMirror) models.LocalGitMirror {
	if mirror == nil {
		return models.LocalGitMirror{
			AuthMode:     AuthModeLocalCredentials,
			RemoteName:   DefaultRemoteName,
			RemoteBranch: DefaultRemoteBranch,
		}
	}
	normalized := *mirror
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
