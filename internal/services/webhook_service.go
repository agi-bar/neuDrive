package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// maxFailures is the number of consecutive delivery failures before a
	// webhook is automatically deactivated.
	maxFailures = 10

	// webhookTimeout is the HTTP timeout for individual webhook deliveries.
	webhookTimeout = 10 * time.Second
)

// WebhookService manages webhook registrations and event delivery.
type WebhookService struct {
	DB     *pgxpool.Pool
	client *http.Client
}

// NewWebhookService creates a new WebhookService.
func NewWebhookService(db *pgxpool.Pool) *WebhookService {
	return &WebhookService{
		DB: db,
		client: &http.Client{
			Timeout: webhookTimeout,
		},
	}
}

// Register creates a new webhook for the given user and returns the webhook
// together with the generated secret (the secret is only returned once).
func (s *WebhookService) Register(ctx context.Context, userID uuid.UUID, url string, events []string) (*models.Webhook, string, error) {
	secret, err := generateSecret(32)
	if err != nil {
		return nil, "", fmt.Errorf("webhook.Register: generate secret: %w", err)
	}

	wh := models.Webhook{
		ID:        uuid.New(),
		UserID:    userID,
		URL:       url,
		Secret:    secret,
		Events:    events,
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}

	_, err = s.DB.Exec(ctx,
		`INSERT INTO webhooks (id, user_id, url, secret, events, is_active, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		wh.ID, wh.UserID, wh.URL, wh.Secret, wh.Events, wh.IsActive, wh.CreatedAt,
	)
	if err != nil {
		return nil, "", fmt.Errorf("webhook.Register: %w", err)
	}

	return &wh, secret, nil
}

// List returns all webhooks belonging to the given user.
func (s *WebhookService) List(ctx context.Context, userID uuid.UUID) ([]models.Webhook, error) {
	rows, err := s.DB.Query(ctx,
		`SELECT id, user_id, url, events, is_active, last_triggered_at, failure_count, created_at
		 FROM webhooks
		 WHERE user_id = $1
		 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("webhook.List: %w", err)
	}
	defer rows.Close()

	var webhooks []models.Webhook
	for rows.Next() {
		var wh models.Webhook
		if err := rows.Scan(
			&wh.ID, &wh.UserID, &wh.URL, &wh.Events,
			&wh.IsActive, &wh.LastTriggeredAt, &wh.FailureCount, &wh.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("webhook.List scan: %w", err)
		}
		webhooks = append(webhooks, wh)
	}
	return webhooks, rows.Err()
}

// Delete removes a webhook owned by the given user.
func (s *WebhookService) Delete(ctx context.Context, webhookID, userID uuid.UUID) error {
	tag, err := s.DB.Exec(ctx,
		`DELETE FROM webhooks WHERE id = $1 AND user_id = $2`, webhookID, userID)
	if err != nil {
		return fmt.Errorf("webhook.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("webhook.Delete: not found")
	}
	return nil
}

// Trigger sends the event payload to all active webhooks that subscribe to the
// given event for the specified user. Deliveries are dispatched asynchronously
// so the caller is never blocked.
func (s *WebhookService) Trigger(ctx context.Context, userID uuid.UUID, event string, payload interface{}) error {
	rows, err := s.DB.Query(ctx,
		`SELECT id, url, secret
		 FROM webhooks
		 WHERE user_id = $1 AND is_active = true AND $2 = ANY(events)`,
		userID, event)
	if err != nil {
		return fmt.Errorf("webhook.Trigger: %w", err)
	}
	defer rows.Close()

	envelope := models.WebhookPayload{
		Event:     event,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data:      payload,
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("webhook.Trigger: marshal: %w", err)
	}

	type target struct {
		id     uuid.UUID
		url    string
		secret string
	}

	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.url, &t.secret); err != nil {
			return fmt.Errorf("webhook.Trigger scan: %w", err)
		}
		targets = append(targets, t)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("webhook.Trigger rows: %w", err)
	}

	// Fire each delivery in its own goroutine.
	for _, t := range targets {
		go s.deliver(t.id, t.url, t.secret, event, body)
	}

	return nil
}

// Test sends a synthetic test event to the specified webhook.
func (s *WebhookService) Test(ctx context.Context, webhookID, userID uuid.UUID) error {
	var url, secret string
	err := s.DB.QueryRow(ctx,
		`SELECT url, secret FROM webhooks WHERE id = $1 AND user_id = $2`,
		webhookID, userID,
	).Scan(&url, &secret)
	if err != nil {
		return fmt.Errorf("webhook.Test: %w", err)
	}

	envelope := models.WebhookPayload{
		Event:     "test",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data:      map[string]string{"message": "This is a test webhook from Agent Hub."},
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("webhook.Test: marshal: %w", err)
	}

	go s.deliver(webhookID, url, secret, "test", body)
	return nil
}

// deliver performs the actual HTTP POST and updates webhook metadata.
func (s *WebhookService) deliver(webhookID uuid.UUID, url, secret, event string, body []byte) {
	sig := computeHMAC(secret, body)

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		slog.Error("webhook deliver: failed to build request", "webhook_id", webhookID, "error", err)
		s.recordFailure(webhookID)
		return
	}

	req.Body = http.NoBody // replaced below
	req.Body = newBytesReadCloser(body)
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AgentHub-Event", event)
	req.Header.Set("X-AgentHub-Signature", "sha256="+sig)

	resp, err := s.client.Do(req)
	if err != nil {
		slog.Error("webhook deliver: POST failed", "url", url, "error", err)
		s.recordFailure(webhookID)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Success: reset failure count, update last_triggered_at.
		_, _ = s.DB.Exec(context.Background(),
			`UPDATE webhooks SET last_triggered_at = NOW(), failure_count = 0 WHERE id = $1`,
			webhookID)
	} else {
		slog.Warn("webhook deliver: non-success status", "url", url, "status", resp.StatusCode)
		s.recordFailure(webhookID)
	}
}

// recordFailure increments the failure count and deactivates the webhook if
// the threshold is exceeded.
func (s *WebhookService) recordFailure(webhookID uuid.UUID) {
	_, _ = s.DB.Exec(context.Background(),
		`UPDATE webhooks
		 SET failure_count = failure_count + 1,
		     is_active = CASE WHEN failure_count + 1 > $1 THEN false ELSE is_active END
		 WHERE id = $2`,
		maxFailures, webhookID)
}

// computeHMAC returns the hex-encoded HMAC-SHA256 of the body using the given secret.
func computeHMAC(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// generateSecret returns a cryptographically random hex string of the given byte length.
func generateSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// bytesReadCloser wraps a byte slice as an io.ReadCloser.
type bytesReadCloser struct {
	data   []byte
	offset int
}

func newBytesReadCloser(data []byte) *bytesReadCloser {
	return &bytesReadCloser{data: data}
}

func (b *bytesReadCloser) Read(p []byte) (int, error) {
	if b.offset >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.offset:])
	b.offset += n
	return n, nil
}

func (b *bytesReadCloser) Close() error {
	return nil
}
