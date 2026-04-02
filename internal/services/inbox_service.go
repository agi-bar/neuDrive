package services

import (
	"context"
	"fmt"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InboxService struct {
	db      *pgxpool.Pool
	Webhook *WebhookService // optional — triggers inbox.new events
}

func NewInboxService(db *pgxpool.Pool) *InboxService {
	return &InboxService{db: db}
}

// GetMessages retrieves inbox messages for a user, optionally filtered by role address and status.
func (s *InboxService) GetMessages(ctx context.Context, userID uuid.UUID, role, status string) ([]models.InboxMessage, error) {
	query := `SELECT id, from_address, to_address, thread_id, priority, action_required, ttl, expires_at,
	                 domain, action_type, tags, context_hash,
	                 subject, body, structured_payload, attachments,
	                 status, created_at, archived_at
	          FROM inbox_messages
	          WHERE to_address LIKE $1 || '%'`
	args := []interface{}{userID.String()}
	argIdx := 2

	if role != "" {
		query += fmt.Sprintf(` AND to_address = $%d`, argIdx)
		args = append(args, role+"@"+userID.String())
		argIdx++
	}
	if status != "" {
		query += fmt.Sprintf(` AND status = $%d`, argIdx)
		args = append(args, status)
		argIdx++
	}

	query += ` ORDER BY created_at DESC LIMIT 100`

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("inbox.GetMessages: %w", err)
	}
	defer rows.Close()

	return s.scanMessages(rows)
}

// Send inserts a new inbox message.
func (s *InboxService) Send(ctx context.Context, userID uuid.UUID, msg models.InboxMessage) (*models.InboxMessage, error) {
	msg.ID = uuid.New()
	msg.Status = "incoming"
	msg.CreatedAt = time.Now().UTC()

	_, err := s.db.Exec(ctx,
		`INSERT INTO inbox_messages (id, user_id, from_address, to_address, thread_id, priority, action_required, ttl, expires_at,
		                             domain, action_type, tags, context_hash,
		                             subject, body, structured_payload, attachments,
		                             status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
		msg.ID, userID, msg.FromAddress, msg.ToAddress, msg.ThreadID, msg.Priority, msg.ActionRequired, msg.TTL, msg.ExpiresAt,
		msg.Domain, msg.ActionType, msg.Tags, msg.ContextHash,
		msg.Subject, msg.Body, msg.StructuredPayload, msg.Attachments,
		msg.Status, msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("inbox.Send: %w", err)
	}

	// Trigger webhook (async, non-blocking)
	if s.Webhook != nil {
		go s.Webhook.Trigger(context.Background(), userID, "inbox.new", map[string]interface{}{
			"message_id": msg.ID.String(), "subject": msg.Subject, "from": msg.FromAddress, "to": msg.ToAddress,
		})
	}

	return &msg, nil
}

func (s *InboxService) MarkRead(ctx context.Context, msgID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE inbox_messages SET status = 'read' WHERE id = $1`, msgID)
	if err != nil {
		return fmt.Errorf("inbox.MarkRead: %w", err)
	}
	return nil
}

func (s *InboxService) Archive(ctx context.Context, msgID uuid.UUID) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx,
		`UPDATE inbox_messages SET status = 'archived', archived_at = $1 WHERE id = $2`, now, msgID)
	if err != nil {
		return fmt.Errorf("inbox.Archive: %w", err)
	}
	return nil
}

// ArchiveExpiredMessages moves messages with a past expires_at to archived status.
// Returns the number of messages archived.
func (s *InboxService) ArchiveExpiredMessages(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	tag, err := s.db.Exec(ctx,
		`UPDATE inbox_messages SET status = 'archived', archived_at = $1
		 WHERE expires_at IS NOT NULL AND expires_at <= $1 AND status != 'archived'`, now)
	if err != nil {
		return 0, fmt.Errorf("inbox.ArchiveExpiredMessages: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Search performs text search on subject and body fields.
func (s *InboxService) Search(ctx context.Context, userID uuid.UUID, query, scope string) ([]models.InboxMessage, error) {
	sqlQuery := `SELECT id, from_address, to_address, thread_id, priority, action_required, ttl, expires_at,
	                    domain, action_type, tags, context_hash,
	                    subject, body, structured_payload, attachments,
	                    status, created_at, archived_at
	             FROM inbox_messages
	             WHERE user_id = $1
	               AND (to_tsvector('english', subject || ' ' || body) @@ plainto_tsquery('english', $2))`
	args := []interface{}{userID, query}
	argIdx := 3

	if scope != "" {
		sqlQuery += fmt.Sprintf(` AND domain = $%d`, argIdx)
		args = append(args, scope)
	}

	sqlQuery += ` ORDER BY created_at DESC LIMIT 50`

	rows, err := s.db.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("inbox.Search: %w", err)
	}
	defer rows.Close()

	return s.scanMessages(rows)
}

type rowScanner interface {
	Next() bool
	Scan(dest ...interface{}) error
	Err() error
}

func (s *InboxService) scanMessages(rows rowScanner) ([]models.InboxMessage, error) {
	var messages []models.InboxMessage
	for rows.Next() {
		var m models.InboxMessage
		if err := rows.Scan(
			&m.ID, &m.FromAddress, &m.ToAddress, &m.ThreadID, &m.Priority, &m.ActionRequired, &m.TTL, &m.ExpiresAt,
			&m.Domain, &m.ActionType, &m.Tags, &m.ContextHash,
			&m.Subject, &m.Body, &m.StructuredPayload, &m.Attachments,
			&m.Status, &m.CreatedAt, &m.ArchivedAt,
		); err != nil {
			return nil, fmt.Errorf("inbox.scanMessages: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
