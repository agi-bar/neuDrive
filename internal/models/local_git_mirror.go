package models

import (
	"time"

	"github.com/google/uuid"
)

type LocalGitMirror struct {
	UserID           uuid.UUID  `json:"user_id"`
	RootPath         string     `json:"root_path"`
	IsActive         bool       `json:"is_active"`
	GitInitializedAt *time.Time `json:"git_initialized_at,omitempty"`
	LastSyncedAt     *time.Time `json:"last_synced_at,omitempty"`
	LastError        string     `json:"last_error,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
