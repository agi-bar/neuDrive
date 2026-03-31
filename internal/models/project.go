package models

import (
	"time"

	"github.com/google/uuid"
)

type Project struct {
	ID        uuid.UUID              `json:"id" db:"id"`
	UserID    uuid.UUID              `json:"user_id" db:"user_id"`
	Name      string                 `json:"name" db:"name"`
	Status    string                 `json:"status" db:"status"` // active, archived
	ContextMD string                 `json:"context_md" db:"context_md"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
	CreatedAt time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt time.Time              `json:"updated_at" db:"updated_at"`
}

type ProjectLog struct {
	ID        uuid.UUID `json:"id" db:"id"`
	ProjectID uuid.UUID `json:"project_id" db:"project_id"`
	Source    string    `json:"source" db:"source"`
	Role      string    `json:"role" db:"role"`
	Action    string    `json:"action" db:"action"`
	Summary   string    `json:"summary" db:"summary"`
	Artifacts []string  `json:"artifacts" db:"artifacts"`
	Tags      []string  `json:"tags" db:"tags"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
