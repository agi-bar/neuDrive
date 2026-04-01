package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FileTreeService struct {
	db *pgxpool.Pool
}

func NewFileTreeService(db *pgxpool.Pool) *FileTreeService {
	return &FileTreeService{db: db}
}

// List returns file tree entries under the given path, filtered by trust level.
// It lists immediate children (one level deep) of the specified directory path.
func (s *FileTreeService) List(ctx context.Context, userID uuid.UUID, path string, trustLevel int) ([]models.FileTreeEntry, error) {
	// Normalize: ensure path ends with /
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, path, is_directory, COALESCE(content, ''), COALESCE(content_type, ''), COALESCE(metadata, '{}'), min_trust_level, created_at, updated_at
		 FROM file_tree
		 WHERE user_id = $1
		   AND path LIKE $2
		   AND path != $3
		   AND min_trust_level <= $4
		   AND path NOT LIKE $5
		 ORDER BY is_directory DESC, path ASC`,
		userID, path+"%", path, trustLevel, path+"%/%")
	if err != nil {
		return nil, fmt.Errorf("filetree.List: %w", err)
	}
	defer rows.Close()

	var entries []models.FileTreeEntry
	for rows.Next() {
		var e models.FileTreeEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Path, &e.IsDirectory, &e.Content,
			&e.ContentType, &e.Metadata, &e.MinTrustLevel, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("filetree.List: scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Read returns a single file entry, checking trust level access.
func (s *FileTreeService) Read(ctx context.Context, userID uuid.UUID, path string, trustLevel int) (*models.FileTreeEntry, error) {
	var e models.FileTreeEntry
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, path, is_directory, COALESCE(content, ''), COALESCE(content_type, ''), COALESCE(metadata, '{}'), min_trust_level, created_at, updated_at
		 FROM file_tree
		 WHERE user_id = $1 AND path = $2`,
		userID, path).
		Scan(&e.ID, &e.UserID, &e.Path, &e.IsDirectory, &e.Content,
			&e.ContentType, &e.Metadata, &e.MinTrustLevel, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("filetree.Read: %w", err)
	}
	if e.MinTrustLevel > trustLevel {
		return nil, fmt.Errorf("filetree.Read: insufficient trust level (need %d, have %d)", e.MinTrustLevel, trustLevel)
	}
	return &e, nil
}

// Write upserts a file entry at the given path.
func (s *FileTreeService) Write(ctx context.Context, userID uuid.UUID, path, content, contentType string, minTrustLevel int) (*models.FileTreeEntry, error) {
	now := time.Now().UTC()
	id := uuid.New()

	_, err := s.db.Exec(ctx,
		`INSERT INTO file_tree (id, user_id, path, is_directory, content, content_type, metadata, min_trust_level, created_at, updated_at)
		 VALUES ($1, $2, $3, false, $4, $5, '{}', $6, $7, $7)
		 ON CONFLICT (user_id, path) DO UPDATE SET
		   content = EXCLUDED.content,
		   content_type = EXCLUDED.content_type,
		   min_trust_level = EXCLUDED.min_trust_level,
		   updated_at = EXCLUDED.updated_at`,
		id, userID, path, content, contentType, minTrustLevel, now)
	if err != nil {
		return nil, fmt.Errorf("filetree.Write: %w", err)
	}

	var e models.FileTreeEntry
	err = s.db.QueryRow(ctx,
		`SELECT id, user_id, path, is_directory, COALESCE(content, ''), COALESCE(content_type, ''), COALESCE(metadata, '{}'), min_trust_level, created_at, updated_at
		 FROM file_tree WHERE user_id = $1 AND path = $2`, userID, path).
		Scan(&e.ID, &e.UserID, &e.Path, &e.IsDirectory, &e.Content,
			&e.ContentType, &e.Metadata, &e.MinTrustLevel, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("filetree.Write: fetch after upsert: %w", err)
	}
	return &e, nil
}

// Delete removes a file tree entry by path.
func (s *FileTreeService) Delete(ctx context.Context, userID uuid.UUID, path string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM file_tree WHERE user_id = $1 AND path = $2`, userID, path)
	if err != nil {
		return fmt.Errorf("filetree.Delete: %w", err)
	}
	return nil
}

// Search performs full-text search on file content using PostgreSQL tsvector.
func (s *FileTreeService) Search(ctx context.Context, userID uuid.UUID, query string, trustLevel int) ([]models.FileTreeEntry, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, path, is_directory, COALESCE(content, ''), COALESCE(content_type, ''), COALESCE(metadata, '{}'), min_trust_level, created_at, updated_at
		 FROM file_tree
		 WHERE user_id = $1
		   AND min_trust_level <= $2
		   AND is_directory = false
		   AND to_tsvector('english', content) @@ plainto_tsquery('english', $3)
		 ORDER BY ts_rank(to_tsvector('english', content), plainto_tsquery('english', $3)) DESC
		 LIMIT 50`,
		userID, trustLevel, query)
	if err != nil {
		return nil, fmt.Errorf("filetree.Search: %w", err)
	}
	defer rows.Close()

	var entries []models.FileTreeEntry
	for rows.Next() {
		var e models.FileTreeEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Path, &e.IsDirectory, &e.Content,
			&e.ContentType, &e.Metadata, &e.MinTrustLevel, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("filetree.Search: scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// EnsureDirectory creates a directory entry if it does not already exist.
func (s *FileTreeService) EnsureDirectory(ctx context.Context, userID uuid.UUID, path string) error {
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx,
		`INSERT INTO file_tree (id, user_id, path, is_directory, content, content_type, metadata, min_trust_level, created_at, updated_at)
		 VALUES ($1, $2, $3, true, '', 'directory', '{}', 1, $4, $4)
		 ON CONFLICT (user_id, path) DO NOTHING`,
		uuid.New(), userID, path, now)
	if err != nil {
		return fmt.Errorf("filetree.EnsureDirectory: %w", err)
	}
	return nil
}
