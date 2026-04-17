package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
	"github.com/agi-bar/neudrive/internal/systemskills"
	"github.com/google/uuid"
)

type DashboardRepo struct {
	Store *Store
}

func NewDashboardRepo(store *Store) services.DashboardRepo {
	return &DashboardRepo{Store: store}
}

func (r *DashboardRepo) GetStats(ctx context.Context, userID uuid.UUID) (*models.DashboardStats, error) {
	stats := &models.DashboardStats{
		WeeklyActivity: []models.DashboardActivity{},
		Pending:        []models.DashboardPending{},
	}
	db := r.Store.DB()
	if db == nil {
		return nil, fmt.Errorf("sqlite.DashboardRepo.GetStats: database not configured")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scoped_tokens
		  WHERE user_id = ? AND revoked_at IS NULL AND expires_at > ? AND name LIKE 'local platform %'`,
		userID.String(),
		now,
	).Scan(&stats.TotalConnections); err != nil {
		return nil, fmt.Errorf("sqlite.DashboardRepo.GetStats: connections count: %w", err)
	}

	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_tree WHERE user_id = ? AND is_directory = 0 AND deleted_at IS NULL`,
		userID.String(),
	).Scan(&stats.TotalFiles); err != nil {
		return nil, fmt.Errorf("sqlite.DashboardRepo.GetStats: files count: %w", err)
	}

	memoryPat := hubpath.NormalizeStorage("/memory/") + "%"
	profilePat := hubpath.NormalizeStorage("/memory/profile/") + "%"
	conversationPat := hubpath.NormalizeStorage("/conversations/") + "%"
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_tree
		  WHERE user_id = ? AND is_directory = 0 AND deleted_at IS NULL
		    AND path LIKE ?
		    AND path NOT LIKE ?`,
		userID.String(),
		memoryPat,
		profilePat,
	).Scan(&stats.TotalMemory); err != nil {
		return nil, fmt.Errorf("sqlite.DashboardRepo.GetStats: memory count: %w", err)
	}

	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_tree
		  WHERE user_id = ? AND is_directory = 0 AND deleted_at IS NULL
		    AND path LIKE ?`,
		userID.String(),
		profilePat,
	).Scan(&stats.TotalProfile); err != nil {
		return nil, fmt.Errorf("sqlite.DashboardRepo.GetStats: profile count: %w", err)
	}

	skillStoragePat := hubpath.NormalizeStorage("/skills/") + "%"
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_tree
		  WHERE user_id = ? AND is_directory = 0 AND deleted_at IS NULL
		    AND path LIKE ?
		    AND path LIKE '%/SKILL.md'`,
		userID.String(),
		skillStoragePat,
	).Scan(&stats.TotalSkills); err != nil {
		return nil, fmt.Errorf("sqlite.DashboardRepo.GetStats: skills count: %w", err)
	}
	stats.TotalSkills += len(systemskills.SkillSummaries())

	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_tree
		  WHERE user_id = ? AND is_directory = 1 AND deleted_at IS NULL
		    AND path LIKE ?
		    AND kind = ?`,
		userID.String(),
		conversationPat,
		services.EntryKindConversationBundle,
	).Scan(&stats.TotalConversations); err != nil {
		return nil, fmt.Errorf("sqlite.DashboardRepo.GetStats: conversations count: %w", err)
	}

	projects, err := NewProjectRepo(r.Store).ListProjects(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("sqlite.DashboardRepo.GetStats: projects count: %w", err)
	}
	stats.TotalProjects = len(projects)

	return stats, nil
}
