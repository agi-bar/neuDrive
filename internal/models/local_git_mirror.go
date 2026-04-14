package models

import (
	"time"

	"github.com/google/uuid"
)

type LocalGitMirror struct {
	UserID                uuid.UUID  `json:"user_id"`
	RootPath              string     `json:"root_path"`
	IsActive              bool       `json:"is_active"`
	AutoCommitEnabled     bool       `json:"auto_commit_enabled"`
	AutoPushEnabled       bool       `json:"auto_push_enabled"`
	AuthMode              string     `json:"auth_mode,omitempty"`
	RemoteName            string     `json:"remote_name,omitempty"`
	RemoteURL             string     `json:"remote_url,omitempty"`
	RemoteBranch          string     `json:"remote_branch,omitempty"`
	GitInitializedAt      *time.Time `json:"git_initialized_at,omitempty"`
	LastSyncedAt          *time.Time `json:"last_synced_at,omitempty"`
	LastError             string     `json:"last_error,omitempty"`
	LastCommitAt          *time.Time `json:"last_commit_at,omitempty"`
	LastCommitHash        string     `json:"last_commit_hash,omitempty"`
	LastPushAt            *time.Time `json:"last_push_at,omitempty"`
	LastPushError         string     `json:"last_push_error,omitempty"`
	GitHubTokenVerifiedAt *time.Time `json:"github_token_verified_at,omitempty"`
	GitHubTokenLogin      string     `json:"github_token_login,omitempty"`
	GitHubRepoPermission  string     `json:"github_repo_permission,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}
