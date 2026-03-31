package services

import (
	"context"
	"fmt"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RoleService struct {
	db *pgxpool.Pool
}

func NewRoleService(db *pgxpool.Pool) *RoleService {
	return &RoleService{db: db}
}

func (s *RoleService) List(ctx context.Context, userID uuid.UUID) ([]models.Role, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, name, role_type, config, allowed_paths, allowed_vault_scopes, lifecycle, created_at
		 FROM roles WHERE user_id = $1 ORDER BY name ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("role.List: %w", err)
	}
	defer rows.Close()

	var roles []models.Role
	for rows.Next() {
		var r models.Role
		if err := rows.Scan(&r.ID, &r.UserID, &r.Name, &r.RoleType, &r.Config,
			&r.AllowedPaths, &r.AllowedVaultScopes, &r.Lifecycle, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("role.List: scan: %w", err)
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

func (s *RoleService) Create(ctx context.Context, userID uuid.UUID, name, roleType string, allowedPaths, allowedVaultScopes []string, lifecycle string) (*models.Role, error) {
	id := uuid.New()
	now := time.Now().UTC()

	_, err := s.db.Exec(ctx,
		`INSERT INTO roles (id, user_id, name, role_type, config, allowed_paths, allowed_vault_scopes, lifecycle, created_at)
		 VALUES ($1, $2, $3, $4, '{}', $5, $6, $7, $8)`,
		id, userID, name, roleType, allowedPaths, allowedVaultScopes, lifecycle, now)
	if err != nil {
		return nil, fmt.Errorf("role.Create: %w", err)
	}

	r := &models.Role{
		ID:                 id,
		UserID:             userID,
		Name:               name,
		RoleType:           roleType,
		Config:             map[string]interface{}{},
		AllowedPaths:       allowedPaths,
		AllowedVaultScopes: allowedVaultScopes,
		Lifecycle:          lifecycle,
		CreatedAt:          now,
	}
	return r, nil
}

func (s *RoleService) Delete(ctx context.Context, userID uuid.UUID, name string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM roles WHERE user_id = $1 AND name = $2`, userID, name)
	if err != nil {
		return fmt.Errorf("role.Delete: %w", err)
	}
	return nil
}

// EnsureDefaultRoles creates the default 'assistant' role if it does not exist.
func (s *RoleService) EnsureDefaultRoles(ctx context.Context, userID uuid.UUID) error {
	var exists bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM roles WHERE user_id = $1 AND name = 'assistant')`, userID).
		Scan(&exists)
	if err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("role.EnsureDefaultRoles: check: %w", err)
	}
	if exists {
		return nil
	}

	_, err = s.Create(ctx, userID, "assistant", "assistant",
		[]string{"/"}, []string{}, "permanent")
	if err != nil {
		return fmt.Errorf("role.EnsureDefaultRoles: create assistant: %w", err)
	}
	return nil
}
