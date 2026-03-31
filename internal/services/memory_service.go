package services

import (
	"context"
	"fmt"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MemoryService struct {
	db *pgxpool.Pool
}

func NewMemoryService(db *pgxpool.Pool) *MemoryService {
	return &MemoryService{db: db}
}

func (s *MemoryService) GetProfile(ctx context.Context, userID uuid.UUID) ([]models.MemoryProfile, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, category, content, source, created_at, updated_at
		 FROM memory_profile WHERE user_id = $1
		 ORDER BY category ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("memory.GetProfile: %w", err)
	}
	defer rows.Close()

	var profiles []models.MemoryProfile
	for rows.Next() {
		var p models.MemoryProfile
		if err := rows.Scan(&p.ID, &p.UserID, &p.Category, &p.Content, &p.Source,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("memory.GetProfile: scan: %w", err)
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

func (s *MemoryService) UpsertProfile(ctx context.Context, userID uuid.UUID, category, content, source string) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx,
		`INSERT INTO memory_profile (id, user_id, category, content, source, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)
		 ON CONFLICT (user_id, category) DO UPDATE SET
		   content = EXCLUDED.content,
		   source = EXCLUDED.source,
		   updated_at = EXCLUDED.updated_at`,
		uuid.New(), userID, category, content, source, now)
	if err != nil {
		return fmt.Errorf("memory.UpsertProfile: %w", err)
	}
	return nil
}

func (s *MemoryService) GetScratch(ctx context.Context, userID uuid.UUID, days int) ([]models.MemoryScratch, error) {
	if days <= 0 {
		days = 7
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, date, content, source, expires_at, created_at
		 FROM memory_scratch
		 WHERE user_id = $1
		   AND (expires_at IS NULL OR expires_at > NOW())
		   AND created_at >= NOW() - make_interval(days => $2)
		 ORDER BY created_at DESC`, userID, days)
	if err != nil {
		return nil, fmt.Errorf("memory.GetScratch: %w", err)
	}
	defer rows.Close()

	var entries []models.MemoryScratch
	for rows.Next() {
		var m models.MemoryScratch
		if err := rows.Scan(&m.ID, &m.UserID, &m.Date, &m.Content, &m.Source,
			&m.ExpiresAt, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("memory.GetScratch: scan: %w", err)
		}
		entries = append(entries, m)
	}
	return entries, rows.Err()
}

func (s *MemoryService) WriteScratch(ctx context.Context, userID uuid.UUID, content, source string) error {
	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	// Default TTL: 7 days from now.
	expiresAt := now.AddDate(0, 0, 7)

	_, err := s.db.Exec(ctx,
		`INSERT INTO memory_scratch (id, user_id, date, content, source, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		uuid.New(), userID, date, content, source, expiresAt, now)
	if err != nil {
		return fmt.Errorf("memory.WriteScratch: %w", err)
	}
	return nil
}

// CleanExpiredScratch removes expired scratch entries and returns how many were deleted.
func (s *MemoryService) CleanExpiredScratch(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM memory_scratch WHERE expires_at IS NOT NULL AND expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("memory.CleanExpiredScratch: %w", err)
	}
	return tag.RowsAffected(), nil
}
