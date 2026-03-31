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

type ConnectionService struct {
	db *pgxpool.Pool
}

func NewConnectionService(db *pgxpool.Pool) *ConnectionService {
	return &ConnectionService{db: db}
}

func (s *ConnectionService) ListByUser(ctx context.Context, userID uuid.UUID) ([]models.Connection, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, name, platform, trust_level, api_key_hash, api_key_prefix,
		        config, last_used_at, created_at, updated_at
		 FROM connections WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("connection.ListByUser: %w", err)
	}
	defer rows.Close()

	var conns []models.Connection
	for rows.Next() {
		var c models.Connection
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Platform, &c.TrustLevel,
			&c.APIKeyHash, &c.APIKeyPrefix, &c.Config, &c.LastUsedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("connection.ListByUser: scan: %w", err)
		}
		conns = append(conns, c)
	}
	return conns, rows.Err()
}

func (s *ConnectionService) GetByID(ctx context.Context, id uuid.UUID) (*models.Connection, error) {
	var c models.Connection
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, platform, trust_level, api_key_hash, api_key_prefix,
		        config, last_used_at, created_at, updated_at
		 FROM connections WHERE id = $1`, id).
		Scan(&c.ID, &c.UserID, &c.Name, &c.Platform, &c.TrustLevel,
			&c.APIKeyHash, &c.APIKeyPrefix, &c.Config, &c.LastUsedAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("connection.GetByID: %w", err)
	}
	return &c, nil
}

func (s *ConnectionService) GetByAPIKey(ctx context.Context, apiKeyHash string) (*models.Connection, error) {
	var c models.Connection
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, platform, trust_level, api_key_hash, api_key_prefix,
		        config, last_used_at, created_at, updated_at
		 FROM connections WHERE api_key_hash = $1`, apiKeyHash).
		Scan(&c.ID, &c.UserID, &c.Name, &c.Platform, &c.TrustLevel,
			&c.APIKeyHash, &c.APIKeyPrefix, &c.Config, &c.LastUsedAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("connection.GetByAPIKey: %w", err)
	}
	return &c, nil
}

// Create creates a new connection and returns it along with the raw API key (shown once).
func (s *ConnectionService) Create(ctx context.Context, userID uuid.UUID, name, platform string, trustLevel int) (*models.Connection, string, error) {
	rawKey, hashedKey, prefix := GenerateAPIKey()

	now := time.Now().UTC()
	id := uuid.New()

	_, err := s.db.Exec(ctx,
		`INSERT INTO connections (id, user_id, name, platform, trust_level, api_key_hash, api_key_prefix, config, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, '{}', $8, $8)`,
		id, userID, name, platform, trustLevel, hashedKey, prefix, now)
	if err != nil {
		return nil, "", fmt.Errorf("connection.Create: %w", err)
	}

	conn, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	return conn, rawKey, nil
}

func (s *ConnectionService) Update(ctx context.Context, id uuid.UUID, name string, trustLevel int) (*models.Connection, error) {
	_, err := s.db.Exec(ctx,
		`UPDATE connections SET name = $1, trust_level = $2, updated_at = $3 WHERE id = $4`,
		name, trustLevel, time.Now().UTC(), id)
	if err != nil {
		return nil, fmt.Errorf("connection.Update: %w", err)
	}
	return s.GetByID(ctx, id)
}

func (s *ConnectionService) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx, `DELETE FROM connections WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("connection.Delete: %w", err)
	}
	return nil
}

func (s *ConnectionService) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE connections SET last_used_at = $1 WHERE id = $2`, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("connection.UpdateLastUsed: %w", err)
	}
	return nil
}

// GenerateAPIKey produces a random 32-byte key and returns (rawKey, sha256Hash, prefix).
// The raw key is hex-encoded and shown to the user once.
// The prefix is the first 8 characters for display purposes.
func GenerateAPIKey() (rawKey, hashedKey, prefix string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("connection: failed to generate random bytes: " + err.Error())
	}
	rawKey = "ahk_" + hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(rawKey))
	hashedKey = hex.EncodeToString(hash[:])
	prefix = rawKey[:12]
	return rawKey, hashedKey, prefix
}

// HashAPIKey hashes a raw API key with SHA-256 for lookup.
func HashAPIKey(rawKey string) string {
	hash := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(hash[:])
}
