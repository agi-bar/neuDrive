package services

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MemoryService struct {
	db       *pgxpool.Pool
	repo     MemoryRepo
	fileTree *FileTreeService
	Webhook  *WebhookService
}

func NewMemoryService(db *pgxpool.Pool, fileTree *FileTreeService) *MemoryService {
	return &MemoryService{db: db, fileTree: fileTree}
}

func NewMemoryServiceWithRepo(repo MemoryRepo, fileTree *FileTreeService) *MemoryService {
	return &MemoryService{repo: repo, fileTree: fileTree}
}

func (s *MemoryService) SupportsScratchMaintenance() bool {
	return s != nil && s.db != nil
}

func (s *MemoryService) GetProfile(ctx context.Context, userID uuid.UUID) ([]models.MemoryProfile, error) {
	if s.repo != nil {
		return s.repo.GetProfiles(ctx, userID)
	}
	if s.fileTree != nil {
		profiles, err := s.loadProfilesFromTree(ctx, userID)
		if err == nil && len(profiles) > 0 {
			return profiles, nil
		}
		if err != nil && err != ErrEntryNotFound {
			return nil, err
		}
	}

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
		if err := rows.Scan(&p.ID, &p.UserID, &p.Category, &p.Content, &p.Source, &p.CreatedAt, &p.UpdatedAt); err != nil {
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
	if err := validateSlug(category, 128); err != nil {
		return fmt.Errorf("memory.UpsertProfile: invalid category: %w", err)
	}
	if source == "" {
		source = SourceFromContext(ctx)
	}
	if source == "" {
		source = "neudrive"
	}

	if s.db != nil {
		if conflict, _ := s.DetectConflict(ctx, userID, category, content, source); conflict != nil {
			slog.Info("memory conflict detected",
				"category", category, "sourceA", conflict.SourceA, "sourceB", conflict.SourceB)
		}
	}

	if s.fileTree != nil {
		if _, err := s.fileTree.WriteEntry(ctx, userID, hubpath.ProfilePath(category), content, "text/markdown", models.FileTreeWriteOptions{
			Kind:          "memory_profile",
			MinTrustLevel: models.TrustLevelFull,
			Metadata: map[string]interface{}{
				"category": category,
				"source":   source,
			},
		}); err != nil {
			return fmt.Errorf("memory.UpsertProfile: write canonical entry: %w", err)
		}
	}

	if s.repo != nil {
		return s.repo.UpsertProfile(ctx, userID, category, content, source)
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
	cutoff := time.Now().UTC().AddDate(0, 0, -days)

	if s.repo != nil {
		return s.repo.GetScratch(ctx, userID, days)
	}

	if s.fileTree != nil {
		entries, err := s.loadScratchFromTree(ctx, userID, &cutoff)
		if err == nil && len(entries) > 0 {
			return entries, nil
		}
		if err != nil && err != ErrEntryNotFound {
			return nil, err
		}
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
		if err := rows.Scan(&m.ID, &m.UserID, &m.Date, &m.Content, &m.Source, &m.ExpiresAt, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("memory.GetScratch: scan: %w", err)
		}
		entries = append(entries, m)
	}
	return entries, rows.Err()
}

func (s *MemoryService) GetScratchActive(ctx context.Context, userID uuid.UUID) ([]models.MemoryScratch, error) {
	if s.repo != nil {
		return s.repo.GetScratchActive(ctx, userID)
	}
	if s.fileTree != nil {
		entries, err := s.loadScratchFromTree(ctx, userID, nil)
		if err == nil && len(entries) > 0 {
			return entries, nil
		}
		if err != nil && err != ErrEntryNotFound {
			return nil, err
		}
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, date, content, source, expires_at, created_at
		 FROM memory_scratch
		 WHERE user_id = $1
		   AND (expires_at IS NULL OR expires_at > NOW())
		 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("memory.GetScratchActive: %w", err)
	}
	defer rows.Close()

	var entries []models.MemoryScratch
	for rows.Next() {
		var m models.MemoryScratch
		if err := rows.Scan(&m.ID, &m.UserID, &m.Date, &m.Content, &m.Source, &m.ExpiresAt, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("memory.GetScratchActive: scan: %w", err)
		}
		entries = append(entries, m)
	}
	return entries, rows.Err()
}

func (s *MemoryService) WriteScratch(ctx context.Context, userID uuid.UUID, content, source string) error {
	_, err := s.WriteScratchWithTitle(ctx, userID, content, source, "")
	return err
}

func (s *MemoryService) WriteScratchWithTitle(ctx context.Context, userID uuid.UUID, content, source, title string) (*models.FileTreeEntry, error) {
	return s.writeScratchEntry(ctx, userID, content, source, title, time.Now().UTC(), nil, uuid.New(), false)
}

func (s *MemoryService) ImportScratch(ctx context.Context, userID uuid.UUID, content, source, title string, createdAt time.Time, expiresAt *time.Time) (*models.FileTreeEntry, error) {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	now := createdAt.UTC()
	return s.writeScratchEntry(ctx, userID, content, source, title, now, expiresAt, importedScratchLegacyID(source, title, now), true)
}

func importedScratchLegacyID(source, title string, createdAt time.Time) uuid.UUID {
	key := fmt.Sprintf("neudrive/imported-scratch/%s/%s/%s", source, title, createdAt.UTC().Format(time.RFC3339Nano))
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key))
}

func (s *MemoryService) writeScratchEntry(ctx context.Context, userID uuid.UUID, content, source, title string, createdAt time.Time, expiresAt *time.Time, legacyID uuid.UUID, upsert bool) (*models.FileTreeEntry, error) {
	if err := validateContentLength(content, maxContentBytes); err != nil {
		return nil, fmt.Errorf("memory.WriteScratchWithTitle: %w", err)
	}
	if source == "" {
		source = SourceFromContext(ctx)
	}
	if source == "" {
		source = "neudrive"
	}

	if legacyID == uuid.Nil {
		legacyID = uuid.New()
	}
	now := createdAt.UTC()
	if expiresAt == nil {
		defaultExpiry := now.AddDate(0, 0, 7)
		expiresAt = &defaultExpiry
	}
	slugBase := title
	if strings.TrimSpace(slugBase) == "" {
		slugBase = source
	}
	slug := fmt.Sprintf("%s-%s", slugBase, legacyID.String()[:8])
	path := hubpath.ScratchPath(now, slug)

	var entry *models.FileTreeEntry
	var err error
	if s.fileTree != nil {
		entry, err = s.fileTree.WriteEntry(ctx, userID, path, content, "text/markdown", models.FileTreeWriteOptions{
			Kind:          "memory_scratch",
			MinTrustLevel: models.TrustLevelFull,
			Metadata: map[string]interface{}{
				"source":     source,
				"title":      title,
				"date":       now.Format("2006-01-02"),
				"expires_at": expiresAt.Format(time.RFC3339),
				"legacy_id":  legacyID.String(),
			},
		})
		if err != nil {
			return nil, fmt.Errorf("memory.WriteScratchWithTitle: write canonical entry: %w", err)
		}
	}

	if s.repo != nil {
		if upsert {
			_, err = s.repo.ImportScratch(ctx, userID, content, source, title, now, expiresAt)
		} else {
			entry, err = s.repo.WriteScratchWithTitle(ctx, userID, content, source, title)
		}
		if err != nil {
			return nil, fmt.Errorf("memory.WriteScratchWithTitle: %w", err)
		}
		if upsert || entry != nil {
			return entry, nil
		}
	}

	query := `INSERT INTO memory_scratch (id, user_id, date, content, source, expires_at, created_at)
		 VALUES ($1, $2, $3::DATE, $4, $5, $6, $7)`
	if upsert {
		query += `
		 ON CONFLICT (id) DO UPDATE SET
		   date = EXCLUDED.date,
		   content = EXCLUDED.content,
		   source = EXCLUDED.source,
		   expires_at = EXCLUDED.expires_at,
		   created_at = EXCLUDED.created_at`
	}

	_, err = s.db.Exec(ctx, query, legacyID, userID, now.Format("2006-01-02"), content, source, expiresAt, now)
	if err != nil {
		return nil, fmt.Errorf("memory.WriteScratchWithTitle: %w", err)
	}
	return entry, nil
}

// GenerateDailyScratchPlaceholders creates a daily summary placeholder for users
// who have recent scratch activity and mirrors it to the canonical tree.
func (s *MemoryService) GenerateDailyScratchPlaceholders(ctx context.Context) (int64, error) {
	rows, err := s.db.Query(ctx,
		`SELECT DISTINCT user_id FROM memory_scratch
		 WHERE created_at >= NOW() - INTERVAL '7 days'`)
	if err != nil {
		return 0, fmt.Errorf("memory.GenerateDailyScratchPlaceholders: %w", err)
	}
	defer rows.Close()

	var count int64
	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	for rows.Next() {
		var userID uuid.UUID
		if err := rows.Scan(&userID); err != nil {
			return count, fmt.Errorf("memory.GenerateDailyScratchPlaceholders: scan: %w", err)
		}

		var exists bool
		err := s.db.QueryRow(ctx,
			`SELECT EXISTS(
				SELECT 1 FROM memory_scratch
				WHERE user_id = $1 AND date = $2::DATE AND source = 'scheduler'
			)`, userID, date).Scan(&exists)
		if err != nil {
			return count, fmt.Errorf("memory.GenerateDailyScratchPlaceholders: check exists: %w", err)
		}
		if exists {
			continue
		}

		if _, err := s.WriteScratchWithTitle(ctx, userID, "Daily summary placeholder for "+date, "scheduler", "daily-summary"); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

// CleanExpiredScratch removes expired scratch entries from both projections.
func (s *MemoryService) CleanExpiredScratch(ctx context.Context) (int64, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id FROM memory_scratch
		 WHERE expires_at IS NOT NULL AND expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("memory.CleanExpiredScratch: query: %w", err)
	}
	defer rows.Close()

	type expiredScratch struct {
		ID     uuid.UUID
		UserID uuid.UUID
	}
	expired := make([]expiredScratch, 0, 16)
	for rows.Next() {
		var item expiredScratch
		if err := rows.Scan(&item.ID, &item.UserID); err != nil {
			return 0, fmt.Errorf("memory.CleanExpiredScratch: scan: %w", err)
		}
		expired = append(expired, item)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("memory.CleanExpiredScratch: rows: %w", err)
	}

	if s.fileTree != nil {
		for _, item := range expired {
			var path string
			err := s.db.QueryRow(ctx,
				`SELECT path FROM file_tree
				 WHERE user_id = $1
				   AND deleted_at IS NULL
				   AND metadata->>'legacy_id' = $2
				 LIMIT 1`,
				item.UserID, item.ID.String(),
			).Scan(&path)
			if err == nil {
				_ = s.fileTree.Delete(ctx, item.UserID, path)
			}
		}
	}

	tag, err := s.db.Exec(ctx,
		`DELETE FROM memory_scratch WHERE expires_at IS NOT NULL AND expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("memory.CleanExpiredScratch: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *MemoryService) DetectConflict(ctx context.Context, userID uuid.UUID, category, newContent, source string) (*models.MemoryConflict, error) {
	var existing models.MemoryProfile
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, category, content, source, created_at, updated_at
		 FROM memory_profile
		 WHERE user_id = $1 AND category = $2`, userID, category).Scan(
		&existing.ID, &existing.UserID, &existing.Category,
		&existing.Content, &existing.Source, &existing.CreatedAt, &existing.UpdatedAt)
	if err != nil {
		return nil, nil
	}

	if existing.Source == source {
		return nil, nil
	}

	ratio := diffRatio(existing.Content, newContent)
	if ratio <= 0.20 {
		return nil, nil
	}

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

	if s.Webhook != nil {
		go s.Webhook.Trigger(context.Background(), userID, models.EventConflictNew, map[string]interface{}{
			"conflict_id": conflict.ID.String(),
			"category":    conflict.Category,
			"source_a":    conflict.SourceA,
			"source_b":    conflict.SourceB,
		})
	}

	return &conflict, nil
}

func (s *MemoryService) ListConflicts(ctx context.Context, userID uuid.UUID) ([]models.MemoryConflict, error) {
	if s.db == nil {
		return []models.MemoryConflict{}, nil
	}
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

func (s *MemoryService) ResolveConflict(ctx context.Context, conflictID uuid.UUID, resolution string) error {
	if s.db == nil {
		return fmt.Errorf("memory.ResolveConflict: conflict resolution not configured")
	}
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

func (s *MemoryService) loadProfilesFromTree(ctx context.Context, userID uuid.UUID) ([]models.MemoryProfile, error) {
	snapshot, err := s.fileTree.Snapshot(ctx, userID, "/memory/profile", models.TrustLevelFull)
	if err != nil {
		return nil, err
	}

	profiles := make([]models.MemoryProfile, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		if entry.IsDirectory || !strings.HasSuffix(entry.Path, ".md") {
			continue
		}
		category := strings.TrimSuffix(path.Base(entry.Path), ".md")
		source, _ := entry.Metadata["source"].(string)
		profiles = append(profiles, models.MemoryProfile{
			ID:        entry.ID,
			UserID:    entry.UserID,
			Category:  category,
			Content:   entry.Content,
			Source:    source,
			CreatedAt: entry.CreatedAt,
			UpdatedAt: entry.UpdatedAt,
		})
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Category < profiles[j].Category })
	return profiles, nil
}

func (s *MemoryService) loadScratchFromTree(ctx context.Context, userID uuid.UUID, cutoff *time.Time) ([]models.MemoryScratch, error) {
	snapshot, err := s.fileTree.Snapshot(ctx, userID, "/memory/scratch", models.TrustLevelFull)
	if err != nil {
		return nil, err
	}

	entries := make([]models.MemoryScratch, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		if entry.IsDirectory || !strings.HasSuffix(entry.Path, ".md") {
			continue
		}

		if expiresAt := metadataTime(entry.Metadata, "expires_at"); expiresAt != nil && !expiresAt.After(time.Now().UTC()) {
			continue
		}
		if cutoff != nil && entry.CreatedAt.Before(*cutoff) {
			continue
		}

		date, _ := entry.Metadata["date"].(string)
		if date == "" {
			date = inferScratchDate(entry.Path, entry.CreatedAt)
		}
		source, _ := entry.Metadata["source"].(string)
		title, _ := entry.Metadata["title"].(string)

		entries = append(entries, models.MemoryScratch{
			ID:        entry.ID,
			UserID:    entry.UserID,
			Date:      date,
			Content:   entry.Content,
			Title:     title,
			Source:    source,
			ExpiresAt: metadataTime(entry.Metadata, "expires_at"),
			CreatedAt: entry.CreatedAt,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})
	return entries, nil
}

func metadataTime(metadata map[string]interface{}, key string) *time.Time {
	raw, ok := metadata[key]
	if !ok {
		return nil
	}
	text, ok := raw.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil
	}
	ts, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return nil
	}
	return &ts
}

func inferScratchDate(path string, createdAt time.Time) string {
	parts := strings.Split(strings.Trim(hubpath.NormalizePublic(path), "/"), "/")
	if len(parts) >= 3 {
		return parts[2]
	}
	return createdAt.Format("2006-01-02")
}

// diffRatio returns the fraction of characters that differ between two strings.
func diffRatio(a, b string) float64 {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 || lb == 0 {
		return 1.0
	}

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
