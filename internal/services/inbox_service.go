package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InboxService struct {
	db       *pgxpool.Pool
	fileTree *FileTreeService
	Webhook  *WebhookService
}

func NewInboxService(db *pgxpool.Pool, fileTree *FileTreeService) *InboxService {
	return &InboxService{db: db, fileTree: fileTree}
}

// GetMessages retrieves inbox messages for a user, optionally filtered by role address and status.
func (s *InboxService) GetMessages(ctx context.Context, userID uuid.UUID, role, status string) ([]models.InboxMessage, error) {
	if s.fileTree != nil {
		messages, err := s.loadMessagesFromTree(ctx, userID, role, status)
		if err == nil && len(messages) > 0 {
			return messages, nil
		}
		if err != nil && err != ErrEntryNotFound {
			return nil, err
		}
	}

	query := `SELECT id, from_address, to_address, thread_id, priority, action_required, ttl, expires_at,
	                 domain, action_type, tags, context_hash,
	                 subject, body, structured_payload, attachments,
	                 status, created_at, archived_at
	          FROM inbox_messages
	          WHERE user_id = $1`
	args := []interface{}{userID}
	argIdx := 2

	if role != "" {
		query += fmt.Sprintf(` AND to_address = $%d`, argIdx)
		args = append(args, role)
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

// Send inserts a new inbox message and mirrors it into the canonical tree.
func (s *InboxService) Send(ctx context.Context, userID uuid.UUID, msg models.InboxMessage) (*models.InboxMessage, error) {
	msg.ID = uuid.New()
	msg.Status = "incoming"
	msg.CreatedAt = time.Now().UTC()

	if s.fileTree != nil {
		if err := s.writeCanonicalMessage(ctx, userID, msg); err != nil {
			return nil, err
		}
	}

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

	if s.Webhook != nil {
		go s.Webhook.Trigger(context.Background(), userID, "inbox.new", map[string]interface{}{
			"message_id": msg.ID.String(), "subject": msg.Subject, "from": msg.FromAddress, "to": msg.ToAddress,
		})
	}

	return &msg, nil
}

func (s *InboxService) MarkRead(ctx context.Context, msgID uuid.UUID) error {
	return s.moveStatus(ctx, msgID, "read")
}

func (s *InboxService) Archive(ctx context.Context, msgID uuid.UUID) error {
	return s.moveStatus(ctx, msgID, "archived")
}

// ArchiveExpiredMessages moves messages with a past expires_at to archived status.
func (s *InboxService) ArchiveExpiredMessages(ctx context.Context) (int64, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id FROM inbox_messages
		 WHERE expires_at IS NOT NULL AND expires_at <= NOW() AND status != 'archived'`)
	if err != nil {
		return 0, fmt.Errorf("inbox.ArchiveExpiredMessages: %w", err)
	}
	defer rows.Close()

	var count int64
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return count, fmt.Errorf("inbox.ArchiveExpiredMessages: scan: %w", err)
		}
		if err := s.Archive(ctx, id); err == nil {
			count++
		}
	}
	return count, rows.Err()
}

// Search performs text search on subject and body fields.
func (s *InboxService) Search(ctx context.Context, userID uuid.UUID, query, scope string) ([]models.InboxMessage, error) {
	if s.fileTree != nil {
		entries, err := s.fileTree.Search(ctx, userID, query, models.TrustLevelFull, "/inbox")
		if err == nil && len(entries) > 0 {
			messages := make([]models.InboxMessage, 0, len(entries))
			for _, entry := range entries {
				msg, ok := decodeInboxMessage(entry.Content)
				if !ok {
					continue
				}
				if scope != "" && msg.Domain != scope {
					continue
				}
				messages = append(messages, msg)
			}
			sort.Slice(messages, func(i, j int) bool {
				return messages[i].CreatedAt.After(messages[j].CreatedAt)
			})
			return messages, nil
		}
		if err != nil && err != ErrEntryNotFound {
			return nil, err
		}
	}

	sqlQuery := `SELECT id, from_address, to_address, thread_id, priority, action_required, ttl, expires_at,
	                    domain, action_type, tags, context_hash,
	                    subject, body, structured_payload, attachments,
	                    status, created_at, archived_at
	             FROM inbox_messages
	             WHERE user_id = $1
	               AND (to_tsvector('simple', subject || ' ' || body) @@ plainto_tsquery('simple', $2)
	                    OR subject ILIKE '%' || $2 || '%'
	                    OR body ILIKE '%' || $2 || '%')`
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

func (s *InboxService) loadMessagesFromTree(ctx context.Context, userID uuid.UUID, role, status string) ([]models.InboxMessage, error) {
	snapshot, err := s.fileTree.Snapshot(ctx, userID, "/inbox", models.TrustLevelFull)
	if err != nil {
		return nil, err
	}

	messages := make([]models.InboxMessage, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		if entry.IsDirectory || !strings.HasSuffix(entry.Path, ".json") {
			continue
		}
		msg, ok := decodeInboxMessage(entry.Content)
		if !ok {
			continue
		}
		if role != "" && msg.ToAddress != role {
			continue
		}
		if status != "" && msg.Status != status {
			continue
		}
		messages = append(messages, msg)
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].CreatedAt.After(messages[j].CreatedAt)
	})
	return messages, nil
}

func (s *InboxService) moveStatus(ctx context.Context, msgID uuid.UUID, nextStatus string) error {
	msg, userID, err := s.lookupMessage(ctx, msgID)
	if err != nil {
		return err
	}

	oldPath := hubpath.InboxMessagePath(msg.ToAddress, msg.Status, msg.ID.String())
	msg.Status = nextStatus
	if nextStatus == "archived" {
		now := time.Now().UTC()
		msg.ArchivedAt = &now
	}

	if s.fileTree != nil {
		_ = s.fileTree.Delete(ctx, userID, oldPath)
		if err := s.writeCanonicalMessage(ctx, userID, msg); err != nil {
			return err
		}
	}

	if nextStatus == "archived" {
		_, err = s.db.Exec(ctx,
			`UPDATE inbox_messages SET status = 'archived', archived_at = $1 WHERE id = $2`,
			time.Now().UTC(), msgID)
	} else {
		_, err = s.db.Exec(ctx,
			`UPDATE inbox_messages SET status = $1 WHERE id = $2`,
			nextStatus, msgID)
	}
	if err != nil {
		return fmt.Errorf("inbox.moveStatus: %w", err)
	}
	return nil
}

func (s *InboxService) lookupMessage(ctx context.Context, msgID uuid.UUID) (models.InboxMessage, uuid.UUID, error) {
	var userID uuid.UUID
	var m models.InboxMessage
	err := s.db.QueryRow(ctx,
		`SELECT user_id, id, from_address, to_address, thread_id, priority, action_required, ttl, expires_at,
		        domain, action_type, tags, context_hash,
		        subject, body, structured_payload, attachments,
		        status, created_at, archived_at
		 FROM inbox_messages
		 WHERE id = $1`,
		msgID,
	).Scan(
		&userID,
		&m.ID, &m.FromAddress, &m.ToAddress, &m.ThreadID, &m.Priority, &m.ActionRequired, &m.TTL, &m.ExpiresAt,
		&m.Domain, &m.ActionType, &m.Tags, &m.ContextHash,
		&m.Subject, &m.Body, &m.StructuredPayload, &m.Attachments,
		&m.Status, &m.CreatedAt, &m.ArchivedAt,
	)
	if err != nil {
		return models.InboxMessage{}, uuid.Nil, fmt.Errorf("inbox.lookupMessage: %w", err)
	}
	return m, userID, nil
}

func (s *InboxService) writeCanonicalMessage(ctx context.Context, userID uuid.UUID, msg models.InboxMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("inbox.writeCanonicalMessage: marshal: %w", err)
	}
	_, err = s.fileTree.WriteEntry(ctx, userID, hubpath.InboxMessagePath(msg.ToAddress, msg.Status, msg.ID.String()), string(body), "application/json", models.FileTreeWriteOptions{
		Kind:          "inbox_message",
		MinTrustLevel: models.TrustLevelCollaborate,
		Metadata: map[string]interface{}{
			"message_id": msg.ID.String(),
			"role":       msg.ToAddress,
			"status":     msg.Status,
			"domain":     msg.Domain,
			"tags":       msg.Tags,
		},
	})
	if err != nil {
		return fmt.Errorf("inbox.writeCanonicalMessage: %w", err)
	}
	return nil
}

func decodeInboxMessage(content string) (models.InboxMessage, bool) {
	var msg models.InboxMessage
	if err := json.Unmarshal([]byte(content), &msg); err != nil {
		return models.InboxMessage{}, false
	}
	return msg, true
}
