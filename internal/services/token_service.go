package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TokenService struct {
	db *pgxpool.Pool
}

func NewTokenService(db *pgxpool.Pool) *TokenService {
	return &TokenService{db: db}
}

// Create generates a new scoped access token. The raw token is returned only once.
func (s *TokenService) Create(ctx context.Context, userID uuid.UUID, req models.CreateTokenRequest) (*models.CreateTokenResponse, error) {
	// Validate scopes
	validScopes := make(map[string]bool)
	for _, sc := range models.AllScopes() {
		validScopes[sc] = true
	}
	for _, sc := range req.Scopes {
		if !validScopes[sc] {
			return nil, fmt.Errorf("token.Create: invalid scope %q", sc)
		}
	}

	if len(req.Scopes) == 0 {
		return nil, fmt.Errorf("token.Create: at least one scope is required")
	}

	if req.MaxTrustLevel < 1 || req.MaxTrustLevel > 4 {
		return nil, fmt.Errorf("token.Create: max_trust_level must be between 1 and 4")
	}

	if req.Name == "" {
		return nil, fmt.Errorf("token.Create: name is required")
	}

	// Generate random token: aht_ + 64 hex chars (32 bytes)
	rawToken, tokenHash, tokenPrefix := generateToken()

	// Calculate expiration
	var expiresAt *time.Time
	if req.ExpiresInDays != nil && *req.ExpiresInDays > 0 {
		t := time.Now().UTC().Add(time.Duration(*req.ExpiresInDays) * 24 * time.Hour)
		expiresAt = &t
	}

	now := time.Now().UTC()
	id := uuid.New()

	_, err := s.db.Exec(ctx,
		`INSERT INTO access_tokens (id, user_id, name, token_hash, token_prefix, scopes, max_trust_level, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
		id, userID, req.Name, tokenHash, tokenPrefix, req.Scopes, req.MaxTrustLevel, expiresAt, now)
	if err != nil {
		return nil, fmt.Errorf("token.Create: %w", err)
	}

	token, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &models.CreateTokenResponse{
		Token:       rawToken,
		TokenPrefix: tokenPrefix,
		AccessToken: *token,
	}, nil
}

// ValidateToken hashes the input, looks up in DB, checks active + not expired,
// increments use_count and updates last_used_at.
func (s *TokenService) ValidateToken(ctx context.Context, rawToken string) (*models.AccessToken, error) {
	hash := hashToken(rawToken)

	var t models.AccessToken
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, scopes, max_trust_level,
		        expires_at, last_used_at, last_used_ip, use_count, is_active, revoked_at, created_at, updated_at
		 FROM access_tokens
		 WHERE token_hash = $1 AND is_active = true`, hash).
		Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.TokenPrefix, &t.Scopes, &t.MaxTrustLevel,
			&t.ExpiresAt, &t.LastUsedAt, &t.LastUsedIP, &t.UseCount, &t.IsActive, &t.RevokedAt, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("token.ValidateToken: %w", err)
	}

	// Check expiration
	if t.IsExpired() {
		// Deactivate the expired token
		_, _ = s.db.Exec(ctx,
			`UPDATE access_tokens SET is_active = false, updated_at = $1 WHERE id = $2`,
			time.Now().UTC(), t.ID)
		return nil, fmt.Errorf("token.ValidateToken: token has expired")
	}

	// Update usage stats (fire-and-forget style, but in the same context)
	_, _ = s.db.Exec(ctx,
		`UPDATE access_tokens SET use_count = use_count + 1, last_used_at = $1, updated_at = $1 WHERE id = $2`,
		time.Now().UTC(), t.ID)

	return &t, nil
}

// ListByUser returns all active tokens for a user.
func (s *TokenService) ListByUser(ctx context.Context, userID uuid.UUID) ([]models.AccessToken, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, scopes, max_trust_level,
		        expires_at, last_used_at, last_used_ip, use_count, is_active, revoked_at, created_at, updated_at
		 FROM access_tokens
		 WHERE user_id = $1 AND is_active = true
		 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("token.ListByUser: %w", err)
	}
	defer rows.Close()

	var tokens []models.AccessToken
	for rows.Next() {
		var t models.AccessToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.TokenPrefix, &t.Scopes, &t.MaxTrustLevel,
			&t.ExpiresAt, &t.LastUsedAt, &t.LastUsedIP, &t.UseCount, &t.IsActive, &t.RevokedAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("token.ListByUser: scan: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// Revoke deactivates a single token by ID, ensuring it belongs to the given user.
func (s *TokenService) Revoke(ctx context.Context, tokenID, userID uuid.UUID) error {
	now := time.Now().UTC()
	tag, err := s.db.Exec(ctx,
		`UPDATE access_tokens SET is_active = false, revoked_at = $1, updated_at = $1
		 WHERE id = $2 AND user_id = $3 AND is_active = true`,
		now, tokenID, userID)
	if err != nil {
		return fmt.Errorf("token.Revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("token.Revoke: token not found or already revoked")
	}
	return nil
}

// RevokeAll deactivates all active tokens for a user.
func (s *TokenService) RevokeAll(ctx context.Context, userID uuid.UUID) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx,
		`UPDATE access_tokens SET is_active = false, revoked_at = $1, updated_at = $1
		 WHERE user_id = $2 AND is_active = true`,
		now, userID)
	if err != nil {
		return fmt.Errorf("token.RevokeAll: %w", err)
	}
	return nil
}

// CleanExpired deactivates all expired tokens and returns the number affected.
func (s *TokenService) CleanExpired(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	tag, err := s.db.Exec(ctx,
		`UPDATE access_tokens SET is_active = false, updated_at = $1
		 WHERE is_active = true AND expires_at IS NOT NULL AND expires_at < $1`,
		now)
	if err != nil {
		return 0, fmt.Errorf("token.CleanExpired: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// getByID fetches a single token by its primary key.
func (s *TokenService) getByID(ctx context.Context, id uuid.UUID) (*models.AccessToken, error) {
	var t models.AccessToken
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, scopes, max_trust_level,
		        expires_at, last_used_at, last_used_ip, use_count, is_active, revoked_at, created_at, updated_at
		 FROM access_tokens WHERE id = $1`, id).
		Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.TokenPrefix, &t.Scopes, &t.MaxTrustLevel,
			&t.ExpiresAt, &t.LastUsedAt, &t.LastUsedIP, &t.UseCount, &t.IsActive, &t.RevokedAt, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("token.getByID: %w", err)
	}
	return &t, nil
}

// generateToken produces a random 32-byte token and returns (rawToken, sha256Hash, prefix).
// Token format: "aht_" + 64 hex chars.
func generateToken() (rawToken, hashedToken, prefix string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("token: failed to generate random bytes: " + err.Error())
	}
	rawToken = "aht_" + hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(rawToken))
	hashedToken = hex.EncodeToString(hash[:])
	prefix = rawToken[:12]
	return rawToken, hashedToken, prefix
}

// hashToken hashes a raw token with SHA-256 for lookup.
func hashToken(rawToken string) string {
	hash := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(hash[:])
}
