package models

import (
	"time"

	"github.com/google/uuid"
)

type InboxMessage struct {
	ID uuid.UUID `json:"id" db:"id"`

	// Envelope
	FromAddress    string     `json:"from_address" db:"from_address"`
	ToAddress      string     `json:"to_address" db:"to_address"`
	ThreadID       string     `json:"thread_id,omitempty" db:"thread_id"`
	Priority       string     `json:"priority" db:"priority"`
	ActionRequired bool       `json:"action_required" db:"action_required"`
	TTL            *string    `json:"ttl,omitempty" db:"ttl"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty" db:"expires_at"`

	// Metadata
	Domain      string   `json:"domain" db:"domain"`
	ActionType  string   `json:"action_type,omitempty" db:"action_type"`
	Tags        []string `json:"tags,omitempty" db:"tags"`
	ContextHash string   `json:"context_hash,omitempty" db:"context_hash"`

	// Content
	Subject           string                 `json:"subject" db:"subject"`
	Body              string                 `json:"body" db:"body"`
	StructuredPayload map[string]interface{} `json:"structured_payload,omitempty" db:"structured_payload"`
	Attachments       []string               `json:"attachments,omitempty" db:"attachments"`

	// Status
	Status     string     `json:"status" db:"status"` // incoming, read, archived
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	ArchivedAt *time.Time `json:"archived_at,omitempty" db:"archived_at"`
}
