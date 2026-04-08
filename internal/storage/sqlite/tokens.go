package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
)

func (s *Store) CreateToken(ctx context.Context, userID uuid.UUID, name string, scopes []string, maxTrustLevel int, ttl time.Duration) (*models.CreateTokenResponse, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("sqlite.CreateToken: name is required")
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("sqlite.CreateToken: at least one scope is required")
	}
	if maxTrustLevel < models.TrustLevelGuest || maxTrustLevel > models.TrustLevelFull {
		return nil, fmt.Errorf("sqlite.CreateToken: invalid max trust level %d", maxTrustLevel)
	}
	if ttl < time.Minute {
		return nil, fmt.Errorf("sqlite.CreateToken: ttl must be at least 1 minute")
	}
	for _, scope := range scopes {
		if !validScope(scope) {
			return nil, fmt.Errorf("sqlite.CreateToken: invalid scope %q", scope)
		}
	}
	rawToken, tokenHash, tokenPrefix, err := generateToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	id := uuid.New()
	expiresAt := now.Add(ttl)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO scoped_tokens (
			id, user_id, name, token_hash, token_prefix, scopes_json, max_trust_level,
			expires_at, rate_limit, request_count, rate_limit_reset_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		id.String(),
		userID.String(),
		name,
		tokenHash,
		tokenPrefix,
		encodeStringSlice(scopes),
		maxTrustLevel,
		timeText(expiresAt),
		1000,
		timeText(now),
		timeText(now),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.CreateToken: %w", err)
	}
	token, err := s.tokenByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &models.CreateTokenResponse{
		Token:       rawToken,
		TokenPrefix: tokenPrefix,
		ScopedToken: token.ToResponse(),
	}, nil
}

func (s *Store) RevokeToken(ctx context.Context, userID, tokenID uuid.UUID) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE scoped_tokens SET revoked_at = ?, last_used_at = COALESCE(last_used_at, ?)
		 WHERE id = ? AND user_id = ? AND revoked_at IS NULL`,
		timeText(now),
		timeText(now),
		tokenID.String(),
		userID.String(),
	)
	if err != nil {
		return fmt.Errorf("sqlite.RevokeToken: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("sqlite.RevokeToken: token not found or already revoked")
	}
	return nil
}

func (s *Store) ValidateToken(ctx context.Context, rawToken string) (*models.ScopedToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, scopes_json, max_trust_level,
		        expires_at, rate_limit, request_count, rate_limit_reset_at,
		        last_used_at, last_used_ip, created_at, revoked_at
		   FROM scoped_tokens
		  WHERE token_hash = ?`,
		hashToken(rawToken),
	)
	token, err := scanScopedToken(row)
	if err != nil {
		return nil, fmt.Errorf("sqlite.ValidateToken: invalid or expired token")
	}
	if token.RevokedAt != nil {
		return nil, fmt.Errorf("sqlite.ValidateToken: token has been revoked")
	}
	if token.IsExpired() {
		return nil, fmt.Errorf("sqlite.ValidateToken: token has expired")
	}
	now := time.Now().UTC()
	_, _ = s.db.ExecContext(ctx, `UPDATE scoped_tokens SET last_used_at = ? WHERE id = ?`, timeText(now), token.ID.String())
	return token, nil
}

func (s *Store) CheckRateLimit(ctx context.Context, token *models.ScopedToken) error {
	if token == nil {
		return fmt.Errorf("sqlite.CheckRateLimit: token is required")
	}
	now := time.Now().UTC()
	if token.RateLimitResetAt.IsZero() || now.After(token.RateLimitResetAt.Add(time.Hour)) {
		_, err := s.db.ExecContext(ctx,
			`UPDATE scoped_tokens SET request_count = 1, rate_limit_reset_at = ? WHERE id = ?`,
			timeText(now),
			token.ID.String(),
		)
		return err
	}
	if token.RequestCount >= token.RateLimit {
		return fmt.Errorf("sqlite.CheckRateLimit: rate limit exceeded (%d/%d per hour)", token.RequestCount, token.RateLimit)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE scoped_tokens SET request_count = request_count + 1 WHERE id = ?`, token.ID.String())
	return err
}

func (s *Store) tokenByID(ctx context.Context, id uuid.UUID) (*models.ScopedToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, scopes_json, max_trust_level,
		        expires_at, rate_limit, request_count, rate_limit_reset_at,
		        last_used_at, last_used_ip, created_at, revoked_at
		   FROM scoped_tokens
		  WHERE id = ?`,
		id.String(),
	)
	return scanScopedToken(row)
}

type scopedTokenScanner interface {
	Scan(dest ...any) error
}

func scanScopedToken(row scopedTokenScanner) (*models.ScopedToken, error) {
	var (
		id             string
		userID         string
		name           string
		tokenHash      string
		tokenPrefix    string
		scopesJSON     string
		maxTrust       int
		expiresAt      string
		rateLimit      int
		requestCount   int
		rateLimitReset string
		lastUsedAt     *string
		lastUsedIP     *string
		createdAt      string
		revokedAt      *string
	)
	if err := row.Scan(
		&id,
		&userID,
		&name,
		&tokenHash,
		&tokenPrefix,
		&scopesJSON,
		&maxTrust,
		&expiresAt,
		&rateLimit,
		&requestCount,
		&rateLimitReset,
		&lastUsedAt,
		&lastUsedIP,
		&createdAt,
		&revokedAt,
	); err != nil {
		return nil, err
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	token := &models.ScopedToken{
		ID:               parsedID,
		UserID:           parsedUserID,
		Name:             name,
		TokenHash:        tokenHash,
		TokenPrefix:      tokenPrefix,
		Scopes:           decodeJSONStringSlice(scopesJSON),
		MaxTrustLevel:    maxTrust,
		ExpiresAt:        mustParseTime(expiresAt),
		RateLimit:        rateLimit,
		RequestCount:     requestCount,
		RateLimitResetAt: mustParseTime(rateLimitReset),
		CreatedAt:        mustParseTime(createdAt),
		LastUsedIP:       lastUsedIP,
	}
	if lastUsedAt != nil {
		ts := mustParseTime(*lastUsedAt)
		token.LastUsedAt = &ts
	}
	if revokedAt != nil {
		ts := mustParseTime(*revokedAt)
		token.RevokedAt = &ts
	}
	return token, nil
}

func validScope(scope string) bool {
	for _, candidate := range models.AllScopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

func generateToken() (rawToken, tokenHash, tokenPrefix string, err error) {
	buf := make([]byte, 20)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("sqlite.generateToken: %w", err)
	}
	tokenBody := hex.EncodeToString(buf)
	rawToken = "aht_" + tokenBody
	tokenHash = hashToken(rawToken)
	tokenPrefix = rawToken[:minInt(len(rawToken), 10)]
	return rawToken, tokenHash, tokenPrefix, nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
