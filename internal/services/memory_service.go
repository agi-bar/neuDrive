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
	if err := validateContentLength(content, maxContentBytes); err != nil {
		return fmt.Errorf("memory.UpsertProfile: %w", err)
	}
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
	if err := validateContentLength(content, maxContentBytes); err != nil {
		return fmt.Errorf("memory.WriteScratch: %w", err)
	}
	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	// Default TTL: 7 days from now.
	expiresAt := now.AddDate(0, 0, 7)

	_, err := s.db.Exec(ctx,
		`INSERT INTO memory_scratch (id, user_id, date, content, source, expires_at, created_at)
		 VALUES ($1, $2, $3::DATE, $4, $5, $6, $7)`,
		uuid.New(), userID, date, content, source, expiresAt, now)
	if err != nil {
		return fmt.Errorf("memory.WriteScratch: %w", err)
	}
	return nil
}

// GenerateDailyScratchPlaceholders creates a daily summary placeholder for users
// who have been active in the last 7 days (i.e., have recent scratch entries).
// Returns the number of placeholders created.
func (s *MemoryService) GenerateDailyScratchPlaceholders(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	expiresAt := now.AddDate(0, 0, 7)

	tag, err := s.db.Exec(ctx,
		`INSERT INTO memory_scratch (id, user_id, date, content, source, expires_at, created_at)
		 SELECT gen_random_uuid(), u.user_id, $1::DATE, 'Daily summary placeholder for ' || $1, 'scheduler', $2, $3
		 FROM (
		   SELECT DISTINCT user_id FROM memory_scratch
		   WHERE created_at >= NOW() - INTERVAL '7 days'
		 ) u
		 WHERE NOT EXISTS (
		   SELECT 1 FROM memory_scratch ms
		   WHERE ms.user_id = u.user_id AND ms.date = $1::DATE AND ms.source = 'scheduler'
		 )`, date, expiresAt, now)
	if err != nil {
		return 0, fmt.Errorf("memory.GenerateDailyScratchPlaceholders: %w", err)
	}
	return tag.RowsAffected(), nil
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

// DetectConflict checks whether the new content for a category from a given
// source conflicts with existing content from a different source. If the
// contents differ by more than 20% (measured by character-level difference
// ratio), a conflict record is created and returned.
func (s *MemoryService) DetectConflict(ctx context.Context, userID uuid.UUID, category, newContent, source string) (*models.MemoryConflict, error) {
	// Look up the existing profile entry for this category.
	var existing models.MemoryProfile
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, category, content, source, created_at, updated_at
		 FROM memory_profile
		 WHERE user_id = $1 AND category = $2`, userID, category).Scan(
		&existing.ID, &existing.UserID, &existing.Category,
		&existing.Content, &existing.Source, &existing.CreatedAt, &existing.UpdatedAt)
	if err != nil {
		// No existing entry — no conflict possible.
		return nil, nil
	}

	// Same source — not a cross-platform conflict.
	if existing.Source == source {
		return nil, nil
	}

	// Compute difference ratio. If contents differ by more than 20%, flag it.
	ratio := diffRatio(existing.Content, newContent)
	if ratio <= 0.20 {
		return nil, nil
	}

	// Create conflict record.
	conflict := models.MemoryConflict{
		ID:        uuid.New(),
		UserID:    userID,
		Category:  category,
		SourceA:   existing.Source,
		ContentA:  existing.Content,
		SourceB:   source,
		ContentB:  newContent,
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
	}

	_, err = s.db.Exec(ctx,
		`INSERT INTO memory_conflicts (id, user_id, category, source_a, content_a, source_b, content_b, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		conflict.ID, conflict.UserID, conflict.Category,
		conflict.SourceA, conflict.ContentA, conflict.SourceB, conflict.ContentB,
		conflict.Status, conflict.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("memory.DetectConflict: insert: %w", err)
	}

	return &conflict, nil
}

// ListConflicts returns all pending memory conflicts for a user.
func (s *MemoryService) ListConflicts(ctx context.Context, userID uuid.UUID) ([]models.MemoryConflict, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, category, source_a, content_a, source_b, content_b, status, resolved_at, created_at
		 FROM memory_conflicts
		 WHERE user_id = $1 AND status = 'pending'
		 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("memory.ListConflicts: %w", err)
	}
	defer rows.Close()

	var conflicts []models.MemoryConflict
	for rows.Next() {
		var c models.MemoryConflict
		if err := rows.Scan(&c.ID, &c.UserID, &c.Category, &c.SourceA, &c.ContentA,
			&c.SourceB, &c.ContentB, &c.Status, &c.ResolvedAt, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("memory.ListConflicts: scan: %w", err)
		}
		conflicts = append(conflicts, c)
	}
	return conflicts, rows.Err()
}

// ResolveConflict resolves a conflict with the given resolution. Valid
// resolutions: keep_a, keep_b, keep_both, dismiss.
func (s *MemoryService) ResolveConflict(ctx context.Context, conflictID uuid.UUID, resolution string) error {
	validResolutions := map[string]bool{
		"keep_a":    true,
		"keep_b":    true,
		"keep_both": true,
		"dismiss":   true,
	}
	if !validResolutions[resolution] {
		return fmt.Errorf("memory.ResolveConflict: invalid resolution %q", resolution)
	}

	status := "resolved_" + resolution
	if resolution == "dismiss" {
		status = "dismissed"
	}

	now := time.Now().UTC()
	tag, err := s.db.Exec(ctx,
		`UPDATE memory_conflicts SET status = $1, resolved_at = $2
		 WHERE id = $3 AND status = 'pending'`,
		status, now, conflictID)
	if err != nil {
		return fmt.Errorf("memory.ResolveConflict: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("memory.ResolveConflict: conflict not found or already resolved")
	}
	return nil
}

// diffRatio returns the fraction of characters that differ between two strings,
// using a simple positional comparison bounded to [0, 1].
func diffRatio(a, b string) float64 {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 || lb == 0 {
		return 1.0
	}

	// Use the longer string as the denominator.
	maxLen := la
	if lb > maxLen {
		maxLen = lb
	}

	minLen := la
	if lb < minLen {
		minLen = lb
	}

	diffs := 0
	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			diffs++
		}
	}
	diffs += maxLen - minLen

	return float64(diffs) / float64(maxLen)
}
