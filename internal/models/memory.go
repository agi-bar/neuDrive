package models

import (
	"time"

	"github.com/google/uuid"
)

type MemoryProfile struct {
	ID        uuid.UUID `json:"id" db:"id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	Category  string    `json:"category" db:"category"`
	Content   string    `json:"content" db:"content"`
	Source    string    `json:"source" db:"source"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type MemoryScratch struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	UserID    uuid.UUID  `json:"user_id" db:"user_id"`
	Date      string     `json:"date" db:"date"`
	Content   string     `json:"content" db:"content"`
	Title     string     `json:"title,omitempty"`
	Source    string     `json:"source" db:"source"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}
