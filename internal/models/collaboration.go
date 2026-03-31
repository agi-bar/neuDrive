package models

import (
	"time"

	"github.com/google/uuid"
)

type Collaboration struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	OwnerUserID uuid.UUID  `json:"owner_user_id" db:"owner_user_id"`
	GuestUserID uuid.UUID  `json:"guest_user_id" db:"guest_user_id"`
	SharedPaths []string   `json:"shared_paths" db:"shared_paths"`
	Permissions string     `json:"permissions" db:"permissions"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}
