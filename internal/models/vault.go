package models

import (
	"time"

	"github.com/google/uuid"
)

type VaultEntry struct {
	ID            uuid.UUID `json:"id" db:"id"`
	UserID        uuid.UUID `json:"user_id" db:"user_id"`
	Scope         string    `json:"scope" db:"scope"`
	EncryptedData []byte    `json:"-" db:"encrypted_data"`
	Nonce         []byte    `json:"-" db:"nonce"`
	Description   string    `json:"description" db:"description"`
	MinTrustLevel int       `json:"min_trust_level" db:"min_trust_level"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// VaultScope is the API response representation without raw encrypted data.
type VaultScope struct {
	ID            uuid.UUID `json:"id"`
	Scope         string    `json:"scope"`
	Description   string    `json:"description"`
	MinTrustLevel int       `json:"min_trust_level"`
	CreatedAt     time.Time `json:"created_at"`
}
