package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TokenService struct {
	repo TokenRepo
}

func NewTokenService(db *pgxpool.Pool) *TokenService {
	return &TokenService{repo: &postgresTokenRepo{db: db}}
}

func NewTokenServiceWithRepo(repo TokenRepo) *TokenService {
	return &TokenService{repo: repo}
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

	// Generate random token: ndt_ + 40 hex chars (20 bytes)
	return s.repo.CreateToken(ctx, userID, req)
}

// CreateEphemeralToken generates a short-lived scoped token with minute-level TTL.
func (s *TokenService) CreateEphemeralToken(ctx context.Context, userID uuid.UUID, name string, scopes []string, maxTrustLevel int, ttl time.Duration) (*models.CreateTokenResponse, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("token.CreateEphemeralToken: name is required")
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("token.CreateEphemeralToken: at least one scope is required")
	}
	if maxTrustLevel < 1 || maxTrustLevel > 4 {
		return nil, fmt.Errorf("token.CreateEphemeralToken: max_trust_level must be between 1 and 4")
	}
	if ttl < time.Minute {
		return nil, fmt.Errorf("token.CreateEphemeralToken: ttl must be at least 1 minute")
	}

	validScopes := make(map[string]bool, len(models.AllScopes))
	for _, sc := range models.AllScopes {
		validScopes[sc] = true
	}
	for _, sc := range scopes {
		if !validScopes[sc] {
			return nil, fmt.Errorf("token.CreateEphemeralToken: invalid scope %q", sc)
		}
	}

	return s.repo.CreateEphemeralToken(ctx, userID, name, scopes, maxTrustLevel, ttl)
}

// ValidateToken hashes the raw token, looks it up in DB (not revoked, not expired),
// updates last_used_at, and returns the ScopedToken record.
func (s *TokenService) ValidateToken(ctx context.Context, rawToken string) (*models.ScopedToken, error) {
	return s.repo.ValidateToken(ctx, rawToken)
}

// ListTokens returns all tokens for a user (both active and revoked).
func (s *TokenService) ListTokens(ctx context.Context, userID uuid.UUID) ([]models.ScopedToken, error) {
	return s.repo.ListTokens(ctx, userID)
}

// RevokeToken sets revoked_at on a token, ensuring it belongs to the given user.
func (s *TokenService) RevokeToken(ctx context.Context, userID, tokenID uuid.UUID) error {
	return s.repo.RevokeToken(ctx, userID, tokenID)
}

// UpdateTokenName changes a token's display name.
func (s *TokenService) UpdateTokenName(ctx context.Context, userID, tokenID uuid.UUID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("token.UpdateTokenName: name is required")
	}

	return s.repo.UpdateTokenName(ctx, userID, tokenID, name)
}

// GetByID returns a single token by ID (public, for the detail endpoint).
func (s *TokenService) GetByID(ctx context.Context, tokenID, userID uuid.UUID) (*models.ScopedToken, error) {
	return s.repo.GetTokenByID(ctx, tokenID, userID)
}

// CheckScope validates that a token has the required scope.
func (s *TokenService) CheckScope(token *models.ScopedToken, requiredScope string) bool {
	return models.HasScope(token.Scopes, requiredScope)
}

// CheckRateLimit checks and increments the request count for a token.
// Returns an error if the rate limit has been exceeded.
// Resets the counter hourly.
func (s *TokenService) CheckRateLimit(ctx context.Context, token *models.ScopedToken) error {
	return s.repo.CheckRateLimit(ctx, token)
}

// DeactivateExpiredTokens revokes all tokens that have passed their expiration time
// and have not already been revoked. Returns the number of tokens affected.
func (s *TokenService) DeactivateExpiredTokens(ctx context.Context) (int64, error) {
	return s.repo.DeactivateExpiredTokens(ctx)
}

type postgresTokenRepo struct {
	db *pgxpool.Pool
}

func (r *postgresTokenRepo) CreateToken(ctx context.Context, userID uuid.UUID, req models.CreateTokenRequest) (*models.CreateTokenResponse, error) {
	rawToken, tokenHash, tokenPrefix, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("token.CreateToken: %w", err)
	}

	expiresAt := time.Now().UTC().Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour)
	now := time.Now().UTC()
	id := uuid.New()

	_, err = r.db.Exec(ctx,
		`INSERT INTO scoped_tokens (id, user_id, name, token_hash, token_prefix, scopes, max_trust_level, expires_at, rate_limit_reset_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
		id, userID, req.Name, tokenHash, tokenPrefix, req.Scopes, req.MaxTrustLevel, expiresAt, now)
	if err != nil {
		return nil, fmt.Errorf("token.CreateToken: %w", err)
	}

	token, err := r.getByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &models.CreateTokenResponse{
		Token:       rawToken,
		TokenPrefix: tokenPrefix,
		ScopedToken: token.ToResponse(),
	}, nil
}

func (r *postgresTokenRepo) CreateEphemeralToken(ctx context.Context, userID uuid.UUID, name string, scopes []string, maxTrustLevel int, ttl time.Duration) (*models.CreateTokenResponse, error) {
	rawToken, tokenHash, tokenPrefix, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("token.CreateEphemeralToken: %w", err)
	}

	now := time.Now().UTC()
	id := uuid.New()
	expiresAt := now.Add(ttl)

	_, err = r.db.Exec(ctx,
		`INSERT INTO scoped_tokens (id, user_id, name, token_hash, token_prefix, scopes, max_trust_level, expires_at, rate_limit_reset_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
		id, userID, name, tokenHash, tokenPrefix, scopes, maxTrustLevel, expiresAt, now)
	if err != nil {
		return nil, fmt.Errorf("token.CreateEphemeralToken: %w", err)
	}

	token, err := r.getByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &models.CreateTokenResponse{
		Token:       rawToken,
		TokenPrefix: tokenPrefix,
		ScopedToken: token.ToResponse(),
	}, nil
}

func (r *postgresTokenRepo) ValidateToken(ctx context.Context, rawToken string) (*models.ScopedToken, error) {
	hash := hashToken(rawToken)

	var t models.ScopedToken
	err := r.db.QueryRow(ctx,
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
	if t.IsExpired() {
		return nil, fmt.Errorf("token.ValidateToken: token has expired")
	}

	now := time.Now().UTC()
	_, _ = r.db.Exec(ctx, `UPDATE scoped_tokens SET last_used_at = $1 WHERE id = $2`, now, t.ID)
	return &t, nil
}

func (r *postgresTokenRepo) ListTokens(ctx context.Context, userID uuid.UUID) ([]models.ScopedToken, error) {
	rows, err := r.db.Query(ctx,
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

func (r *postgresTokenRepo) RevokeToken(ctx context.Context, userID, tokenID uuid.UUID) error {
	now := time.Now().UTC()
	tag, err := r.db.Exec(ctx,
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

func (r *postgresTokenRepo) UpdateTokenName(ctx context.Context, userID, tokenID uuid.UUID, name string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE scoped_tokens SET name = $1
		 WHERE id = $2 AND user_id = $3`,
		name, tokenID, userID)
	if err != nil {
		return fmt.Errorf("token.UpdateTokenName: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("token.UpdateTokenName: token not found")
	}
	return nil
}

func (r *postgresTokenRepo) GetTokenByID(ctx context.Context, tokenID, userID uuid.UUID) (*models.ScopedToken, error) {
	var t models.ScopedToken
	err := r.db.QueryRow(ctx,
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

func (r *postgresTokenRepo) CheckRateLimit(ctx context.Context, token *models.ScopedToken) error {
	now := time.Now().UTC()
	if now.After(token.RateLimitResetAt.Add(time.Hour)) {
		_, err := r.db.Exec(ctx, `UPDATE scoped_tokens SET request_count = 1, rate_limit_reset_at = $1 WHERE id = $2`, now, token.ID)
		if err != nil {
			return fmt.Errorf("token.CheckRateLimit: reset: %w", err)
		}
		return nil
	}
	if token.RequestCount >= token.RateLimit {
		return fmt.Errorf("token.CheckRateLimit: rate limit exceeded (%d/%d per hour)", token.RequestCount, token.RateLimit)
	}
	_, err := r.db.Exec(ctx, `UPDATE scoped_tokens SET request_count = request_count + 1 WHERE id = $1`, token.ID)
	if err != nil {
		return fmt.Errorf("token.CheckRateLimit: increment: %w", err)
	}
	return nil
}

func (r *postgresTokenRepo) DeactivateExpiredTokens(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	tag, err := r.db.Exec(ctx,
		`UPDATE scoped_tokens SET revoked_at = $1
		 WHERE expires_at < $1 AND revoked_at IS NULL`, now)
	if err != nil {
		return 0, fmt.Errorf("token.DeactivateExpiredTokens: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *postgresTokenRepo) getByID(ctx context.Context, id uuid.UUID) (*models.ScopedToken, error) {
	var t models.ScopedToken
	err := r.db.QueryRow(ctx,
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
// Token format: "ndt_" + 40 hex chars (20 random bytes).
func generateToken() (rawToken, hashedToken, prefix string, err error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("token: failed to generate random bytes: %w", err)
	}
	rawToken = "ndt_" + hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(rawToken))
	hashedToken = hex.EncodeToString(hash[:])
	prefix = rawToken[:12]
	return rawToken, hashedToken, prefix, nil
}

// hashToken hashes a raw token with SHA-256 for lookup.
func hashToken(rawToken string) string {
	hash := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(hash[:])
}
