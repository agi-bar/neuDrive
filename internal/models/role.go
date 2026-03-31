package models

import (
	"time"

	"github.com/google/uuid"
)

type Role struct {
	ID               uuid.UUID              `json:"id" db:"id"`
	UserID           uuid.UUID              `json:"user_id" db:"user_id"`
	Name             string                 `json:"name" db:"name"`
	RoleType         string                 `json:"role_type" db:"role_type"` // assistant, worker, delegate
	Config           map[string]interface{} `json:"config" db:"config"`
	AllowedPaths     []string               `json:"allowed_paths" db:"allowed_paths"`
	AllowedVaultScopes []string             `json:"allowed_vault_scopes" db:"allowed_vault_scopes"`
	Lifecycle        string                 `json:"lifecycle" db:"lifecycle"` // session, project, permanent
	CreatedAt        time.Time              `json:"created_at" db:"created_at"`
}
