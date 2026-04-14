package services

import (
	"context"
	"fmt"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserService struct {
	db   *pgxpool.Pool
	repo UserRepo
}

func NewUserService(db *pgxpool.Pool) *UserService {
	return &UserService{db: db}
}

func NewUserServiceWithRepo(repo UserRepo) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	if s.repo != nil {
		user, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("user.GetByID: %w", err)
		}
		return user, nil
	}
	var u models.User
	err := s.db.QueryRow(ctx,
		`SELECT id, slug, display_name, COALESCE(email, ''), COALESCE(avatar_url, ''), timezone, language, created_at, updated_at
		 FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Slug, &u.DisplayName, &u.Email, &u.AvatarURL, &u.Timezone, &u.Language, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("user.GetByID: %w", err)
	}
	return &u, nil
}

func (s *UserService) GetBySlug(ctx context.Context, slug string) (*models.User, error) {
	if s.repo != nil {
		user, err := s.repo.GetBySlug(ctx, slug)
		if err != nil {
			return nil, fmt.Errorf("user.GetBySlug: %w", err)
		}
		return user, nil
	}
	var u models.User
	err := s.db.QueryRow(ctx,
		`SELECT id, slug, display_name, COALESCE(email, ''), COALESCE(avatar_url, ''), timezone, language, created_at, updated_at
		 FROM users WHERE slug = $1`, slug).
		Scan(&u.ID, &u.Slug, &u.DisplayName, &u.Email, &u.AvatarURL, &u.Timezone, &u.Language, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("user.GetBySlug: %w", err)
	}
	return &u, nil
}

func (s *UserService) CreateOrUpdateFromGitHub(ctx context.Context, githubID string, login string, name string) (*models.User, error) {
	if s.repo != nil {
		return nil, fmt.Errorf("user.CreateOrUpdateFromGitHub: not supported by configured repo")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("user.CreateOrUpdateFromGitHub: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Check if an auth binding already exists for this GitHub account.
	var userID uuid.UUID
	err = tx.QueryRow(ctx,
		`SELECT user_id FROM auth_bindings WHERE provider = 'github' AND provider_id = $1`, githubID).
		Scan(&userID)

	now := time.Now().UTC()

	if err == pgx.ErrNoRows {
		// New user: create user row then auth binding.
		userID = uuid.New()
		_, err = tx.Exec(ctx,
			`INSERT INTO users (id, slug, display_name, timezone, language, created_at, updated_at)
			 VALUES ($1, $2, $3, 'UTC', 'en', $4, $4)`,
			userID, login, name, now)
		if err != nil {
			return nil, fmt.Errorf("user.CreateOrUpdateFromGitHub: insert user: %w", err)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO auth_bindings (id, user_id, provider, provider_id, provider_data, created_at)
			 VALUES ($1, $2, 'github', $3, '{}', $4)`,
			uuid.New(), userID, githubID, now)
		if err != nil {
			return nil, fmt.Errorf("user.CreateOrUpdateFromGitHub: insert binding: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("user.CreateOrUpdateFromGitHub: lookup binding: %w", err)
	} else {
		// Existing user: update display name.
		_, err = tx.Exec(ctx,
			`UPDATE users SET display_name = $1, updated_at = $2 WHERE id = $3`,
			name, now, userID)
		if err != nil {
			return nil, fmt.Errorf("user.CreateOrUpdateFromGitHub: update user: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("user.CreateOrUpdateFromGitHub: commit: %w", err)
	}

	return s.GetByID(ctx, userID)
}

func (s *UserService) GetAuthBinding(ctx context.Context, provider string, providerID string) (*models.AuthBinding, error) {
	if s.repo != nil {
		binding, err := s.repo.GetAuthBinding(ctx, provider, providerID)
		if err != nil {
			return nil, fmt.Errorf("user.GetAuthBinding: %w", err)
		}
		return binding, nil
	}
	var ab models.AuthBinding
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, provider, provider_id, provider_data, created_at
		 FROM auth_bindings WHERE provider = $1 AND provider_id = $2`,
		provider, providerID).
		Scan(&ab.ID, &ab.UserID, &ab.Provider, &ab.ProviderID, &ab.ProviderData, &ab.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("user.GetAuthBinding: %w", err)
	}
	return &ab, nil
}
