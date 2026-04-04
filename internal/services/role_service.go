package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RoleService struct {
	db       *pgxpool.Pool
	fileTree *FileTreeService
}

func NewRoleService(db *pgxpool.Pool, fileTree *FileTreeService) *RoleService {
	return &RoleService{db: db, fileTree: fileTree}
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
	if err := s.syncRoleTree(ctx, *r); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *RoleService) Delete(ctx context.Context, userID uuid.UUID, name string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM roles WHERE user_id = $1 AND name = $2`, userID, name)
	if err != nil {
		return fmt.Errorf("role.Delete: %w", err)
	}
	if s.fileTree != nil {
		_ = s.fileTree.Delete(ctx, userID, hubpath.RoleSkillPath(name))
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

	_, err = s.Create(ctx, userID, "assistant", "assistant", []string{"/"}, []string{}, "permanent")
	if err != nil {
		return fmt.Errorf("role.EnsureDefaultRoles: create assistant: %w", err)
	}
	return nil
}

func (s *RoleService) syncRoleTree(ctx context.Context, role models.Role) error {
	if s.fileTree == nil {
		return nil
	}
	content := renderRoleSkill(role)
	_, err := s.fileTree.WriteEntry(ctx, role.UserID, hubpath.RoleSkillPath(role.Name), content, "text/markdown", models.FileTreeWriteOptions{
		Kind:          "role_skill",
		MinTrustLevel: models.TrustLevelCollaborate,
		Metadata: map[string]interface{}{
			"name":                 role.Name,
			"description":          strings.TrimSpace(role.RoleType + " role"),
			"role_type":            role.RoleType,
			"lifecycle":            role.Lifecycle,
			"allowed_paths":        role.AllowedPaths,
			"allowed_vault_scopes": role.AllowedVaultScopes,
			"source":               "roles",
		},
	})
	if err != nil {
		return fmt.Errorf("role.syncRoleTree: %w", err)
	}
	return nil
}

func renderRoleSkill(role models.Role) string {
	return strings.TrimSpace(fmt.Sprintf(
		"# Role: %s\n\nThis role represents the `%s` persona inside AgentHub.\n\n## Lifecycle\n%s\n\n## Allowed Paths\n%s\n\n## Allowed Vault Scopes\n%s\n",
		role.Name,
		role.RoleType,
		role.Lifecycle,
		strings.Join(role.AllowedPaths, ", "),
		strings.Join(role.AllowedVaultScopes, ", "),
	)) + "\n"
}
