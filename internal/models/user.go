package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID          uuid.UUID `json:"id" db:"id"`
	Slug        string    `json:"slug" db:"slug"`
	DisplayName string    `json:"display_name" db:"display_name"`
	Timezone    string    `json:"timezone" db:"timezone"`
	Language    string    `json:"language" db:"language"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type AuthBinding struct {
	ID           uuid.UUID              `json:"id" db:"id"`
	UserID       uuid.UUID              `json:"user_id" db:"user_id"`
	Provider     string                 `json:"provider" db:"provider"`
	ProviderID   string                 `json:"provider_id" db:"provider_id"`
	ProviderData map[string]interface{} `json:"provider_data" db:"provider_data"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
}
