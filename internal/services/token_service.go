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

// CreateToken generates a new scoped access token.
// It returns the raw token string exactly ONCE in the response.
func (s *TokenService) CreateToken(ctx context.Context, userID uuid.UUID, req models.CreateTokenRequest) (*models.CreateTokenResponse, error) {
	// Validate name
	if req.Name == "" {
		return nil, fmt.Errorf("token.CreateToken: name is required")
	}

	// Validate scopes
	if len(req.Scopes) == 0 {
		return nil, fmt.Errorf("token.CreateToken: at least one scope is required")
	}
	validScopes := make(map[string]bool, len(models.AllScopes))
	for _, sc := range models.AllScopes {
		validScopes[sc] = true
	}
	for _, sc := range req.Scopes {
		if !validScopes[sc] {
			return nil, fmt.Errorf("token.CreateToken: invalid scope %q", sc)
		}
	}

	// Validate trust level
	if req.MaxTrustLevel < 1 || req.MaxTrustLevel > 4 {
		return nil, fmt.Errorf("token.CreateToken: max_trust_level must be between 1 and 4")
	}

	// Validate expiration
	if req.ExpiresInDays < 1 {
		return nil, fmt.Errorf("token.CreateToken: expires_in_days must be at least 1")
	}

	// Generate random token: aht_ + 40 hex chars (20 bytes)
	rawToken, tokenHash, tokenPrefix := generateToken()

	expiresAt := time.Now().UTC().Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour)
	now := time.Now().UTC()
	id := uuid.New()

	_, err := s.db.Exec(ctx,
		`INSERT INTO scoped_tokens (id, user_id, name, token_hash, token_prefix, scopes, max_trust_level, expires_at, rate_limit_reset_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
		id, userID, req.Name, tokenHash, tokenPrefix, req.Scopes, req.MaxTrustLevel, expiresAt, now)
	if err != nil {
		return nil, fmt.Errorf("token.CreateToken: %w", err)
	}

	token, err := s.getByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &models.CreateTokenResponse{
		Token:       rawToken,
		TokenPrefix: tokenPrefix,
		ScopedToken: token.ToResponse(),
	}, nil
}

// ValidateToken hashes the raw token, looks it up in DB (not revoked, not expired),
// updates last_used_at, and returns the ScopedToken record.
func (s *TokenService) ValidateToken(ctx context.Context, rawToken string) (*models.ScopedToken, error) {
	hash := hashToken(rawToken)

	var t models.ScopedToken
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, scopes, max_trust_level,
		        expires_at, rate_limit, request_count, rate_limit_reset_at,
		        last_used_at, last_used_ip, created_at, revoked_at
		 FROM scoped_tokens
		 WHERE token_hash = $1 AND revoked_at IS NULL`, hash).
		Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.TokenPrefix, &t.Scopes, &t.MaxTrustLevel,
			&t.ExpiresAt, &t.RateLimit, &t.RequestCount, &t.RateLimitResetAt,
			&t.LastUsedAt, &t.LastUsedIP, &t.CreatedAt, &t.RevokedAt)
	if err != nil {
		return nil, fmt.Errorf("token.ValidateToken: invalid token")
	}

	// Check expiration
	if t.IsExpired() {
		return nil, fmt.Errorf("token.ValidateToken: token has expired")
	}

	// Update last_used_at (fire-and-forget)
	now := time.Now().UTC()
	_, _ = s.db.Exec(ctx,
		`UPDATE scoped_tokens SET last_used_at = $1 WHERE id = $2`,
		now, t.ID)

	return &t, nil
}

// ListTokens returns all tokens for a user (both active and revoked).
func (s *TokenService) ListTokens(ctx context.Context, userID uuid.UUID) ([]models.ScopedToken, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, scopes, max_trust_level,
		        expires_at, rate_limit, request_count, rate_limit_reset_at,
		        last_used_at, last_used_ip, created_at, revoked_at
		 FROM scoped_tokens
		 WHERE user_id = $1
		 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("token.ListTokens: %w", err)
	}
	defer rows.Close()

	var tokens []models.ScopedToken
	for rows.Next() {
		var t models.ScopedToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.TokenPrefix, &t.Scopes, &t.MaxTrustLevel,
			&t.ExpiresAt, &t.RateLimit, &t.RequestCount, &t.RateLimitResetAt,
			&t.LastUsedAt, &t.LastUsedIP, &t.CreatedAt, &t.RevokedAt); err != nil {
			return nil, fmt.Errorf("token.ListTokens: scan: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// RevokeToken sets revoked_at on a token, ensuring it belongs to the given user.
func (s *TokenService) RevokeToken(ctx context.Context, userID, tokenID uuid.UUID) error {
	now := time.Now().UTC()
	tag, err := s.db.Exec(ctx,
		`UPDATE scoped_tokens SET revoked_at = $1
		 WHERE id = $2 AND user_id = $3 AND revoked_at IS NULL`,
		now, tokenID, userID)
	if err != nil {
		return fmt.Errorf("token.RevokeToken: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("token.RevokeToken: token not found or already revoked")
	}
	return nil
}

// GetByID returns a single token by ID (public, for the detail endpoint).
func (s *TokenService) GetByID(ctx context.Context, tokenID, userID uuid.UUID) (*models.ScopedToken, error) {
	var t models.ScopedToken
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, scopes, max_trust_level,
		        expires_at, rate_limit, request_count, rate_limit_reset_at,
		        last_used_at, last_used_ip, created_at, revoked_at
		 FROM scoped_tokens
		 WHERE id = $1 AND user_id = $2`, tokenID, userID).
		Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.TokenPrefix, &t.Scopes, &t.MaxTrustLevel,
			&t.ExpiresAt, &t.RateLimit, &t.RequestCount, &t.RateLimitResetAt,
			&t.LastUsedAt, &t.LastUsedIP, &t.CreatedAt, &t.RevokedAt)
	if err != nil {
		return nil, fmt.Errorf("token.GetByID: %w", err)
	}
	return &t, nil
}

// CheckScope validates that a token has the required scope.
func (s *TokenService) CheckScope(token *models.ScopedToken, requiredScope string) bool {
	return models.HasScope(token.Scopes, requiredScope)
}

// CheckRateLimit checks and increments the request count for a token.
// Returns an error if the rate limit has been exceeded.
// Resets the counter hourly.
func (s *TokenService) CheckRateLimit(ctx context.Context, token *models.ScopedToken) error {
	now := time.Now().UTC()

	// If the reset window has passed, reset the counter.
	if now.After(token.RateLimitResetAt.Add(time.Hour)) {
		_, err := s.db.Exec(ctx,
			`UPDATE scoped_tokens SET request_count = 1, rate_limit_reset_at = $1 WHERE id = $2`,
			now, token.ID)
		if err != nil {
			return fmt.Errorf("token.CheckRateLimit: reset: %w", err)
		}
		return nil
	}

	// Check if we are over the limit.
	if token.RequestCount >= token.RateLimit {
		return fmt.Errorf("token.CheckRateLimit: rate limit exceeded (%d/%d per hour)", token.RequestCount, token.RateLimit)
	}

	// Increment counter.
	_, err := s.db.Exec(ctx,
		`UPDATE scoped_tokens SET request_count = request_count + 1 WHERE id = $1`,
		token.ID)
	if err != nil {
		return fmt.Errorf("token.CheckRateLimit: increment: %w", err)
	}
	return nil
}

// getByID fetches a single token by primary key (internal, no user check).
func (s *TokenService) getByID(ctx context.Context, id uuid.UUID) (*models.ScopedToken, error) {
	var t models.ScopedToken
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, scopes, max_trust_level,
		        expires_at, rate_limit, request_count, rate_limit_reset_at,
		        last_used_at, last_used_ip, created_at, revoked_at
		 FROM scoped_tokens WHERE id = $1`, id).
		Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.TokenPrefix, &t.Scopes, &t.MaxTrustLevel,
			&t.ExpiresAt, &t.RateLimit, &t.RequestCount, &t.RateLimitResetAt,
			&t.LastUsedAt, &t.LastUsedIP, &t.CreatedAt, &t.RevokedAt)
	if err != nil {
		return nil, fmt.Errorf("token.getByID: %w", err)
	}
	return &t, nil
}

// generateToken produces a random token and returns (rawToken, sha256Hash, prefix).
// Token format: "aht_" + 40 hex chars (20 random bytes).
func generateToken() (rawToken, hashedToken, prefix string) {
	b := make([]byte, 20)
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
