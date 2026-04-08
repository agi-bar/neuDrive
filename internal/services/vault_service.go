package services

import (
	"context"
	"fmt"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/vault"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type VaultService struct {
	db    *pgxpool.Pool
	repo  VaultRepo
	vault *vault.Vault
}

func NewVaultService(db *pgxpool.Pool, v *vault.Vault) *VaultService {
	return &VaultService{db: db, vault: v}
}

func NewVaultServiceWithRepo(repo VaultRepo, v *vault.Vault) *VaultService {
	return &VaultService{repo: repo, vault: v}
}

// ListScopes returns vault scope metadata (without decrypted data) filtered by trust level.
func (s *VaultService) ListScopes(ctx context.Context, userID uuid.UUID, trustLevel int) ([]models.VaultScope, error) {
	if s.repo != nil {
		return s.repo.ListScopes(ctx, userID, trustLevel)
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, scope, description, min_trust_level, created_at
		 FROM vault_entries
		 WHERE user_id = $1 AND min_trust_level <= $2
		 ORDER BY scope ASC`,
		userID, trustLevel)
	if err != nil {
		return nil, fmt.Errorf("vault.ListScopes: %w", err)
	}
	defer rows.Close()

	var scopes []models.VaultScope
	for rows.Next() {
		var vs models.VaultScope
		if err := rows.Scan(&vs.ID, &vs.Scope, &vs.Description, &vs.MinTrustLevel, &vs.CreatedAt); err != nil {
			return nil, fmt.Errorf("vault.ListScopes: scan: %w", err)
		}
		scopes = append(scopes, vs)
	}
	return scopes, rows.Err()
}

// Read retrieves and decrypts a vault entry by scope, checking trust level.
func (s *VaultService) Read(ctx context.Context, userID uuid.UUID, scope string, trustLevel int) (string, error) {
	var (
		entry *models.VaultEntry
		err   error
	)
	if s.repo != nil {
		entry, err = s.repo.GetEntry(ctx, userID, scope)
		if err != nil {
			return "", fmt.Errorf("vault.Read: %w", err)
		}
	} else {
		var current models.VaultEntry
		err = s.db.QueryRow(ctx,
			`SELECT id, user_id, scope, encrypted_data, nonce, description, min_trust_level, created_at, updated_at
			 FROM vault_entries
			 WHERE user_id = $1 AND scope = $2`,
			userID, scope).
			Scan(&current.ID, &current.UserID, &current.Scope, &current.EncryptedData, &current.Nonce,
				&current.Description, &current.MinTrustLevel, &current.CreatedAt, &current.UpdatedAt)
		if err != nil {
			return "", fmt.Errorf("vault.Read: %w", err)
		}
		entry = &current
	}

	if entry.MinTrustLevel > trustLevel {
		return "", fmt.Errorf("vault.Read: insufficient trust level (need %d, have %d)", entry.MinTrustLevel, trustLevel)
	}

	plaintext, err := s.vault.Decrypt(entry.EncryptedData, entry.Nonce)
	if err != nil {
		return "", fmt.Errorf("vault.Read: decrypt: %w", err)
	}
	return string(plaintext), nil
}

// Write encrypts and upserts a vault entry.
func (s *VaultService) Write(ctx context.Context, userID uuid.UUID, scope, plaintext, description string, minTrustLevel int) error {
	if err := validateSlug(scope, 128); err != nil {
		return fmt.Errorf("vault.Write: invalid scope: %w", err)
	}
	ciphertext, nonce, err := s.vault.Encrypt([]byte(plaintext))
	if err != nil {
		return fmt.Errorf("vault.Write: encrypt: %w", err)
	}

	now := time.Now().UTC()
	if s.repo != nil {
		return s.repo.UpsertEntry(ctx, models.VaultEntry{
			ID:            uuid.New(),
			UserID:        userID,
			Scope:         scope,
			EncryptedData: ciphertext,
			Nonce:         nonce,
			Description:   description,
			MinTrustLevel: minTrustLevel,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}
	_, err = s.db.Exec(ctx,
		`INSERT INTO vault_entries (id, user_id, scope, encrypted_data, nonce, description, min_trust_level, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		 ON CONFLICT (user_id, scope) DO UPDATE SET
		   encrypted_data = EXCLUDED.encrypted_data,
		   nonce = EXCLUDED.nonce,
		   description = EXCLUDED.description,
		   min_trust_level = EXCLUDED.min_trust_level,
		   updated_at = EXCLUDED.updated_at`,
		uuid.New(), userID, scope, ciphertext, nonce, description, minTrustLevel, now)
	if err != nil {
		return fmt.Errorf("vault.Write: %w", err)
	}
	return nil
}

// Delete removes a vault entry by scope.
func (s *VaultService) Delete(ctx context.Context, userID uuid.UUID, scope string) error {
	if s.repo != nil {
		return s.repo.DeleteEntry(ctx, userID, scope)
	}
	_, err := s.db.Exec(ctx,
		`DELETE FROM vault_entries WHERE user_id = $1 AND scope = $2`, userID, scope)
	if err != nil {
		return fmt.Errorf("vault.Delete: %w", err)
	}
	return nil
}
