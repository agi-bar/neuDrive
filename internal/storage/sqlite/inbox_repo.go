package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
	"github.com/google/uuid"
)

type InboxRepo struct {
	Store *Store
}

func NewInboxRepo(store *Store) services.InboxRepo {
	return &InboxRepo{Store: store}
}

func (r *InboxRepo) ListMessages(ctx context.Context, userID uuid.UUID, role, status string) ([]models.InboxMessage, error) {
	query := `SELECT id, from_address, to_address, thread_id, priority, action_required, ttl, expires_at,
	                 domain, action_type, tags_json, context_hash,
	                 subject, body, structured_payload_json, attachments_json,
	                 status, created_at, archived_at
	          FROM inbox_messages
	          WHERE user_id = ?`
	args := []any{userID.String()}
	if role != "" {
		query += ` AND to_address = ?`
		args = append(args, role)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC LIMIT 100`

	rows, err := r.Store.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite.InboxRepo.ListMessages: %w", err)
	}
	defer rows.Close()
	return scanInboxMessages(rows)
}

func (r *InboxRepo) CreateMessage(ctx context.Context, userID uuid.UUID, msg models.InboxMessage) error {
	_, err := r.Store.DB().ExecContext(ctx,
		`INSERT INTO inbox_messages (
			id, user_id, from_address, to_address, thread_id, priority, action_required, ttl, expires_at,
			domain, action_type, tags_json, context_hash,
			subject, body, structured_payload_json, attachments_json,
			status, created_at, archived_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID.String(), userID.String(), msg.FromAddress, msg.ToAddress, msg.ThreadID, msg.Priority, boolInt(msg.ActionRequired),
		nullString(msg.TTL), nullableTimeText(msg.ExpiresAt),
		msg.Domain, msg.ActionType, encodeStringSlice(msg.Tags), msg.ContextHash,
		msg.Subject, msg.Body, encodeJSON(msg.StructuredPayload), encodeStringSlice(msg.Attachments),
		msg.Status, timeText(msg.CreatedAt), nullableTimeText(msg.ArchivedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlite.InboxRepo.CreateMessage: %w", err)
	}
	return nil
}

func (r *InboxRepo) GetMessage(ctx context.Context, msgID uuid.UUID) (models.InboxMessage, uuid.UUID, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT user_id, id, from_address, to_address, thread_id, priority, action_required, ttl, expires_at,
		        domain, action_type, tags_json, context_hash,
		        subject, body, structured_payload_json, attachments_json,
		        status, created_at, archived_at
		   FROM inbox_messages
		  WHERE id = ?`,
		msgID.String(),
	)
	var (
		userID string
		msg    models.InboxMessage
	)
	if err := scanInboxMessageRow(row, &userID, &msg); err != nil {
		return models.InboxMessage{}, uuid.Nil, fmt.Errorf("sqlite.InboxRepo.GetMessage: %w", err)
	}
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return models.InboxMessage{}, uuid.Nil, err
	}
	return msg, parsedUserID, nil
}

func (r *InboxRepo) UpdateMessageStatus(ctx context.Context, msgID uuid.UUID, status string, archivedAt *time.Time) error {
	_, err := r.Store.DB().ExecContext(ctx,
		`UPDATE inbox_messages SET status = ?, archived_at = ? WHERE id = ?`,
		status, nullableTimeText(archivedAt), msgID.String(),
	)
	if err != nil {
		return fmt.Errorf("sqlite.InboxRepo.UpdateMessageStatus: %w", err)
	}
	return nil
}

func (r *InboxRepo) ArchiveExpiredMessages(ctx context.Context, now time.Time) (int64, error) {
	result, err := r.Store.DB().ExecContext(ctx,
		`UPDATE inbox_messages
		    SET status = 'archived', archived_at = ?
		  WHERE expires_at IS NOT NULL
		    AND expires_at <= ?
		    AND status != 'archived'`,
		timeText(now), timeText(now),
	)
	if err != nil {
		return 0, fmt.Errorf("sqlite.InboxRepo.ArchiveExpiredMessages: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}

func (r *InboxRepo) SearchMessages(ctx context.Context, userID uuid.UUID, query, scope string) ([]models.InboxMessage, error) {
	query = strings.TrimSpace(strings.ToLower(query))
	sqlQuery := `SELECT id, from_address, to_address, thread_id, priority, action_required, ttl, expires_at,
	                    domain, action_type, tags_json, context_hash,
	                    subject, body, structured_payload_json, attachments_json,
	                    status, created_at, archived_at
	               FROM inbox_messages
	              WHERE user_id = ?
	                AND (LOWER(subject) LIKE ? OR LOWER(body) LIKE ?)`
	args := []any{userID.String(), "%" + query + "%", "%" + query + "%"}
	if scope != "" {
		sqlQuery += ` AND domain = ?`
		args = append(args, scope)
	}
	sqlQuery += ` ORDER BY created_at DESC LIMIT 50`
	rows, err := r.Store.DB().QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite.InboxRepo.SearchMessages: %w", err)
	}
	defer rows.Close()
	return scanInboxMessages(rows)
}

func scanInboxMessages(rows *sql.Rows) ([]models.InboxMessage, error) {
	var messages []models.InboxMessage
	for rows.Next() {
		var msg models.InboxMessage
		if err := scanInboxMessageRow(rows, nil, &msg); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if messages == nil {
		messages = []models.InboxMessage{}
	}
	return messages, rows.Err()
}

func scanInboxMessageRow(scanner interface{ Scan(dest ...any) error }, userIDDest *string, msg *models.InboxMessage) error {
	var (
		id, ttl, expiresAt, createdAt, archivedAt             sql.NullString
		fromAddress, toAddress, threadID, priority            string
		domain, actionType, tagsJSON, contextHash             string
		subject, body, structuredPayloadJSON, attachmentsJSON string
		status                                                string
		actionRequired                                        int
	)
	if userIDDest != nil {
		if err := scanner.Scan(
			userIDDest,
			&id, &fromAddress, &toAddress, &threadID, &priority, &actionRequired, &ttl, &expiresAt,
			&domain, &actionType, &tagsJSON, &contextHash,
			&subject, &body, &structuredPayloadJSON, &attachmentsJSON,
			&status, &createdAt, &archivedAt,
		); err != nil {
			return err
		}
	} else {
		if err := scanner.Scan(
			&id, &fromAddress, &toAddress, &threadID, &priority, &actionRequired, &ttl, &expiresAt,
			&domain, &actionType, &tagsJSON, &contextHash,
			&subject, &body, &structuredPayloadJSON, &attachmentsJSON,
			&status, &createdAt, &archivedAt,
		); err != nil {
			return err
		}
	}

	parsedID, err := uuid.Parse(id.String)
	if err != nil {
		return err
	}
	msg.ID = parsedID
	msg.FromAddress = fromAddress
	msg.ToAddress = toAddress
	msg.ThreadID = threadID
	msg.Priority = priority
	msg.ActionRequired = actionRequired != 0
	msg.TTL = nullStringPtr(ttl)
	msg.ExpiresAt = nullableParsedTime(expiresAt)
	msg.Domain = domain
	msg.ActionType = actionType
	msg.Tags = decodeJSONStringSlice(tagsJSON)
	msg.ContextHash = contextHash
	msg.Subject = subject
	msg.Body = body
	msg.StructuredPayload = decodeJSONMap(structuredPayloadJSON)
	msg.Attachments = decodeJSONStringSlice(attachmentsJSON)
	msg.Status = status
	msg.CreatedAt = mustParseTime(createdAt.String)
	msg.ArchivedAt = nullableParsedTime(archivedAt)
	return nil
}

func nullString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	v := value.String
	return &v
}

func nullableParsedTime(value sql.NullString) *time.Time {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	ts := mustParseTime(value.String)
	return &ts
}
