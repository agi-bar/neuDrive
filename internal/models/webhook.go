package models

import (
	"time"

	"github.com/google/uuid"
)

// Webhook event type constants.
const (
	EventInboxNew      = "inbox.new"
	EventProjectUpdate = "project.update"
	EventConflictNew   = "conflict.new"
	EventVaultAccess   = "vault.access"
	EventCollabNew     = "collaboration.new"
)

// ValidWebhookEvents is the set of recognised event types.
var ValidWebhookEvents = map[string]bool{
	EventInboxNew:      true,
	EventProjectUpdate: true,
	EventConflictNew:   true,
	EventVaultAccess:   true,
	EventCollabNew:     true,
}

// Webhook represents a registered webhook endpoint.
type Webhook struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	UserID          uuid.UUID  `json:"user_id" db:"user_id"`
	URL             string     `json:"url" db:"url"`
	Secret          string     `json:"-" db:"secret"` // never expose in JSON
	Events          []string   `json:"events" db:"events"`
	IsActive        bool       `json:"is_active" db:"is_active"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty" db:"last_triggered_at"`
	FailureCount    int        `json:"failure_count" db:"failure_count"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
}

// WebhookPayload is the envelope sent to webhook endpoints.
type WebhookPayload struct {
	Event     string      `json:"event"`
	Timestamp string      `json:"timestamp"`
	Data      interface{} `json:"data"`
}
