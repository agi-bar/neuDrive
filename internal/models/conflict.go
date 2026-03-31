package models

import (
	"time"

	"github.com/google/uuid"
)

type MemoryConflict struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	Category   string     `json:"category"`
	SourceA    string     `json:"source_a"`
	ContentA   string     `json:"content_a"`
	SourceB    string     `json:"source_b"`
	ContentB   string     `json:"content_b"`
	Status     string     `json:"status"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}
