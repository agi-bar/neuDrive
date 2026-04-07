package localstore

import (
	"context"
	"fmt"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/systemskills"
	"github.com/google/uuid"
)

func (s *Store) DashboardStats(ctx context.Context, userID uuid.UUID) (*models.DashboardStats, error) {
	stats := &models.DashboardStats{
		WeeklyActivity: []models.DashboardActivity{},
		Pending:        []models.DashboardPending{},
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scoped_tokens
		  WHERE user_id = ? AND revoked_at IS NULL AND expires_at > ? AND name LIKE 'local platform %'`,
		userID.String(),
		now,
	).Scan(&stats.TotalConnections); err != nil {
		return nil, fmt.Errorf("localstore.DashboardStats: connections count: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_tree WHERE user_id = ? AND is_directory = 0 AND deleted_at IS NULL`,
		userID.String(),
	).Scan(&stats.TotalFiles); err != nil {
		return nil, fmt.Errorf("localstore.DashboardStats: files count: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_tree
		  WHERE user_id = ? AND is_directory = 0 AND deleted_at IS NULL
		    AND path LIKE '/memory/%'
		    AND path NOT LIKE '/memory/profile/%'`,
		userID.String(),
	).Scan(&stats.TotalMemory); err != nil {
		return nil, fmt.Errorf("localstore.DashboardStats: memory count: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_tree
		  WHERE user_id = ? AND is_directory = 0 AND deleted_at IS NULL
		    AND path LIKE '/memory/profile/%'`,
		userID.String(),
	).Scan(&stats.TotalProfile); err != nil {
		return nil, fmt.Errorf("localstore.DashboardStats: profile count: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_tree
		  WHERE user_id = ? AND is_directory = 0 AND deleted_at IS NULL
		    AND (path LIKE '/skills/%' OR path LIKE '/.skills/%')
		    AND path LIKE '%/SKILL.md'`,
		userID.String(),
	).Scan(&stats.TotalSkills); err != nil {
		return nil, fmt.Errorf("localstore.DashboardStats: skills count: %w", err)
	}
	stats.TotalSkills += len(systemskills.SkillSummaries())

	projects, err := s.ListProjects(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("localstore.DashboardStats: projects count: %w", err)
	}
	stats.TotalProjects = len(projects)

	return stats, nil
}
