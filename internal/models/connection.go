package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	TrustLevelGuest       = 1 // L1: Only public profile
	TrustLevelCollaborate = 2 // L2: Shared projects only
	TrustLevelWork        = 3 // L3: Skills, memory, non-personal vault
	TrustLevelFull        = 4 // L4: Everything including all vault
)

type Connection struct {
	ID           uuid.UUID              `json:"id" db:"id"`
	UserID       uuid.UUID              `json:"user_id" db:"user_id"`
	Name         string                 `json:"name" db:"name"`
	Platform     string                 `json:"platform" db:"platform"`
	TrustLevel   int                    `json:"trust_level" db:"trust_level"`
	APIKeyHash   string                 `json:"-" db:"api_key_hash"`
	APIKeyPrefix string                 `json:"api_key_prefix" db:"api_key_prefix"`
	Config       map[string]interface{} `json:"config" db:"config"`
	LastUsedAt   *time.Time             `json:"last_used_at,omitempty" db:"last_used_at"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at" db:"updated_at"`
}
