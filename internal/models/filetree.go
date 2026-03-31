package models

import (
	"time"

	"github.com/google/uuid"
)

type FileTreeEntry struct {
	ID            uuid.UUID              `json:"id" db:"id"`
	UserID        uuid.UUID              `json:"user_id" db:"user_id"`
	Path          string                 `json:"path" db:"path"`
	IsDirectory   bool                   `json:"is_directory" db:"is_directory"`
	Content       string                 `json:"content,omitempty" db:"content"`
	ContentType   string                 `json:"content_type" db:"content_type"`
	Metadata      map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
	MinTrustLevel int                    `json:"min_trust_level" db:"min_trust_level"`
	CreatedAt     time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at" db:"updated_at"`
}

type FileTreeListResult struct {
	Path    string           `json:"path"`
	Entries []FileTreeEntry  `json:"entries"`
	Total   int              `json:"total"`
}
