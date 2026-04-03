package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CollaborationService struct {
	db *pgxpool.Pool
}

func NewCollaborationService(db *pgxpool.Pool) *CollaborationService {
	return &CollaborationService{db: db}
}

// Create creates a new collaboration link between an owner and a guest user.
func (s *CollaborationService) Create(ctx context.Context, ownerUserID, guestUserID uuid.UUID, sharedPaths []string, permissions string, expiresInDays *int) (*models.Collaboration, error) {
	if ownerUserID == guestUserID {
		return nil, fmt.Errorf("collaboration.Create: cannot collaborate with yourself")
	}
	if len(sharedPaths) == 0 {
		return nil, fmt.Errorf("collaboration.Create: shared_paths must not be empty")
	}
	if permissions == "" {
		permissions = "read"
	}
	if permissions != "read" && permissions != "readwrite" {
		return nil, fmt.Errorf("collaboration.Create: permissions must be 'read' or 'readwrite'")
	}
	for i, sharedPath := range sharedPaths {
		sharedPaths[i] = hubpath.NormalizePublic(sharedPath)
	}

	id := uuid.New()
	now := time.Now().UTC()

	var expiresAt *time.Time
	if expiresInDays != nil && *expiresInDays > 0 {
		t := now.AddDate(0, 0, *expiresInDays)
		expiresAt = &t
	}

	_, err := s.db.Exec(ctx,
		`INSERT INTO collaborations (id, owner_user_id, guest_user_id, shared_paths, permissions, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, ownerUserID, guestUserID, sharedPaths, permissions, expiresAt, now)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique") {
			return nil, fmt.Errorf("collaboration.Create: collaboration already exists between these users")
		}
		return nil, fmt.Errorf("collaboration.Create: %w", err)
	}

	return s.getByID(ctx, id)
}

// ListOwned returns collaborations where the given user is the owner.
func (s *CollaborationService) ListOwned(ctx context.Context, userID uuid.UUID) ([]models.Collaboration, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, owner_user_id, guest_user_id, shared_paths, permissions, expires_at, created_at
		 FROM collaborations WHERE owner_user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("collaboration.ListOwned: %w", err)
	}
	defer rows.Close()

	var collabs []models.Collaboration
	for rows.Next() {
		var c models.Collaboration
		if err := rows.Scan(&c.ID, &c.OwnerUserID, &c.GuestUserID, &c.SharedPaths, &c.Permissions, &c.ExpiresAt, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("collaboration.ListOwned: scan: %w", err)
		}
		collabs = append(collabs, c)
	}
	return collabs, rows.Err()
}

// ListShared returns collaborations where the given user is the guest.
func (s *CollaborationService) ListShared(ctx context.Context, userID uuid.UUID) ([]models.Collaboration, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, owner_user_id, guest_user_id, shared_paths, permissions, expires_at, created_at
		 FROM collaborations WHERE guest_user_id = $1
		   AND (expires_at IS NULL OR expires_at > NOW())
		 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("collaboration.ListShared: %w", err)
	}
	defer rows.Close()

	var collabs []models.Collaboration
	for rows.Next() {
		var c models.Collaboration
		if err := rows.Scan(&c.ID, &c.OwnerUserID, &c.GuestUserID, &c.SharedPaths, &c.Permissions, &c.ExpiresAt, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("collaboration.ListShared: scan: %w", err)
		}
		collabs = append(collabs, c)
	}
	return collabs, rows.Err()
}

// Revoke deletes a collaboration. Only the owner can revoke.
func (s *CollaborationService) Revoke(ctx context.Context, collabID, ownerUserID uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM collaborations WHERE id = $1 AND owner_user_id = $2`,
		collabID, ownerUserID)
	if err != nil {
		return fmt.Errorf("collaboration.Revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("collaboration.Revoke: not found or not owned by user")
	}
	return nil
}

// GetSharedPaths returns the paths that a guest user can access from an owner.
func (s *CollaborationService) GetSharedPaths(ctx context.Context, guestUserID, ownerUserID uuid.UUID) ([]string, error) {
	var paths []string
	err := s.db.QueryRow(ctx,
		`SELECT shared_paths FROM collaborations
		 WHERE owner_user_id = $1 AND guest_user_id = $2
		   AND (expires_at IS NULL OR expires_at > NOW())`,
		ownerUserID, guestUserID).Scan(&paths)
	if err != nil {
		return nil, fmt.Errorf("collaboration.GetSharedPaths: %w", err)
	}
	return paths, nil
}

// CanAccess checks if a guest user can access a specific path on an owner's hub.
func (s *CollaborationService) CanAccess(ctx context.Context, guestUserID, ownerUserID uuid.UUID, path string) (bool, error) {
	paths, err := s.GetSharedPaths(ctx, guestUserID, ownerUserID)
	if err != nil {
		return false, nil // no collaboration found = no access
	}

	path = hubpath.NormalizePublic(path)

	for _, shared := range paths {
		shared = hubpath.NormalizePublic(shared)
		// Exact match or the requested path is under the shared path.
		if path == shared || strings.HasPrefix(path, shared+"/") {
			return true, nil
		}
	}
	return false, nil
}

func (s *CollaborationService) getByID(ctx context.Context, id uuid.UUID) (*models.Collaboration, error) {
	var c models.Collaboration
	err := s.db.QueryRow(ctx,
		`SELECT id, owner_user_id, guest_user_id, shared_paths, permissions, expires_at, created_at
		 FROM collaborations WHERE id = $1`, id).
		Scan(&c.ID, &c.OwnerUserID, &c.GuestUserID, &c.SharedPaths, &c.Permissions, &c.ExpiresAt, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("collaboration.getByID: %w", err)
	}
	return &c, nil
}
