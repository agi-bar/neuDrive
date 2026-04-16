package models

import (
	"time"

	"github.com/google/uuid"
)

type FileTreeEntry struct {
	ID            uuid.UUID              `json:"id" db:"id"`
	UserID        uuid.UUID              `json:"user_id" db:"user_id"`
	Path          string                 `json:"path" db:"path"`
	Kind          string                 `json:"kind" db:"kind"`
	IsDirectory   bool                   `json:"is_directory" db:"is_directory"`
	Content       string                 `json:"content,omitempty" db:"content"`
	ContentType   string                 `json:"content_type" db:"content_type"`
	Metadata      map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
	Checksum      string                 `json:"checksum" db:"checksum"`
	Version       int64                  `json:"version" db:"version"`
	MinTrustLevel int                    `json:"min_trust_level" db:"min_trust_level"`
	CreatedAt     time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at" db:"updated_at"`
	DeletedAt     *time.Time             `json:"deleted_at,omitempty" db:"deleted_at"`
}

type FileTreeListResult struct {
	Path    string          `json:"path"`
	Entries []FileTreeEntry `json:"entries"`
	Total   int             `json:"total"`
}

type FileTreeWriteOptions struct {
	Kind             string                 `json:"kind,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	MinTrustLevel    int                    `json:"min_trust_level,omitempty"`
	ExpectedVersion  *int64                 `json:"expected_version,omitempty"`
	ExpectedChecksum string                 `json:"expected_checksum,omitempty"`
}

type EntryVersion struct {
	Cursor        int64                  `json:"cursor" db:"cursor"`
	ID            uuid.UUID              `json:"id" db:"id"`
	EntryID       uuid.UUID              `json:"entry_id" db:"entry_id"`
	UserID        uuid.UUID              `json:"user_id" db:"user_id"`
	Path          string                 `json:"path" db:"path"`
	Kind          string                 `json:"kind" db:"kind"`
	Version       int64                  `json:"version" db:"version"`
	ChangeType    string                 `json:"change_type" db:"change_type"`
	Content       string                 `json:"content,omitempty" db:"content"`
	ContentType   string                 `json:"content_type" db:"content_type"`
	Metadata      map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
	Checksum      string                 `json:"checksum" db:"checksum"`
	MinTrustLevel int                    `json:"min_trust_level" db:"min_trust_level"`
	CreatedAt     time.Time              `json:"created_at" db:"created_at"`
}

type EntryChange struct {
	Cursor     int64         `json:"cursor"`
	ChangeType string        `json:"change_type"`
	Entry      FileTreeEntry `json:"entry"`
}

type EntrySnapshot struct {
	Path         string          `json:"path"`
	Cursor       int64           `json:"cursor"`
	RootChecksum string          `json:"root_checksum"`
	Entries      []FileTreeEntry `json:"entries"`
}

type SkillSummary struct {
	Name          string                 `json:"name"`
	Path          string                 `json:"path"`
	BundlePath    string                 `json:"bundle_path,omitempty"`
	PrimaryPath   string                 `json:"primary_path,omitempty"`
	Source        string                 `json:"source"`
	ReadOnly      bool                   `json:"read_only,omitempty"`
	Description   string                 `json:"description,omitempty"`
	WhenToUse     string                 `json:"when_to_use,omitempty"`
	Capabilities  []string               `json:"capabilities,omitempty"`
	AllowedTools  []string               `json:"allowed_tools,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
	Arguments     map[string]interface{} `json:"arguments,omitempty"`
	Activation    map[string]interface{} `json:"activation,omitempty"`
	MinTrustLevel int                    `json:"min_trust_level,omitempty"`
}

type BundleSummary struct {
	Kind          string                 `json:"kind"`
	Name          string                 `json:"name"`
	Path          string                 `json:"path"`
	Source        string                 `json:"source,omitempty"`
	ReadOnly      bool                   `json:"read_only,omitempty"`
	Description   string                 `json:"description,omitempty"`
	WhenToUse     string                 `json:"when_to_use,omitempty"`
	Status        string                 `json:"status,omitempty"`
	PrimaryPath   string                 `json:"primary_path,omitempty"`
	LogPath       string                 `json:"log_path,omitempty"`
	Capabilities  []string               `json:"capabilities,omitempty"`
	AllowedTools  []string               `json:"allowed_tools,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
	Arguments     map[string]interface{} `json:"arguments,omitempty"`
	Activation    map[string]interface{} `json:"activation,omitempty"`
	MinTrustLevel int                    `json:"min_trust_level,omitempty"`
}
