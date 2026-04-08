package services

import (
	"context"
	"fmt"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/systemskills"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DashboardService struct {
	db   *pgxpool.Pool
	repo DashboardRepo
}

func NewDashboardService(db *pgxpool.Pool) *DashboardService {
	return &DashboardService{db: db}
}

func NewDashboardServiceWithRepo(repo DashboardRepo) *DashboardService {
	return &DashboardService{repo: repo}
}

// GetStats aggregates dashboard statistics for a user.
func (s *DashboardService) GetStats(ctx context.Context, userID uuid.UUID) (*models.DashboardStats, error) {
	if s.repo != nil {
		return s.repo.GetStats(ctx, userID)
	}
	stats := &models.DashboardStats{}
	skillStoragePat := hubpath.NormalizeStorage("/skills/") + "%"
	skillAltPat := hubpath.AlternateSkillsPath(hubpath.NormalizeStorage("/skills/")) + "%"
	memoryPat := hubpath.NormalizeStorage("/memory/") + "%"
	profilePat := hubpath.NormalizeStorage("/memory/profile/") + "%"

	// Count connected entries across manual API-key connections and OAuth/MCP grants.
	err := s.db.QueryRow(ctx,
		`SELECT
		   (SELECT COUNT(*) FROM connections WHERE user_id = $1) +
		   (SELECT COUNT(*) FROM oauth_grants WHERE user_id = $1)`,
		userID).
		Scan(&stats.TotalConnections)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: connections count: %w", err)
	}

	// Count all visible files across the Hub tree.
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM file_tree WHERE user_id = $1 AND is_directory = false AND deleted_at IS NULL`,
		userID).
		Scan(&stats.TotalFiles)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: files count: %w", err)
	}

	// Count memory files outside the structured profile section.
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM file_tree WHERE user_id = $1 AND is_directory = false AND deleted_at IS NULL
		   AND path LIKE $2
		   AND path NOT LIKE $3`,
		userID, memoryPat, profilePat).
		Scan(&stats.TotalMemory)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: memory count: %w", err)
	}

	// Count profile entries.
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM file_tree WHERE user_id = $1 AND is_directory = false AND deleted_at IS NULL
		   AND path LIKE $2`,
		userID, profilePat).
		Scan(&stats.TotalProfile)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: profile count: %w", err)
	}

	// Count skills by SKILL.md documents, plus bundled system skills.
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM file_tree WHERE user_id = $1 AND is_directory = false AND deleted_at IS NULL
		   AND (path LIKE $2 OR path LIKE $3)
		   AND path LIKE '%/SKILL.md'`,
		userID, skillStoragePat, skillAltPat).
		Scan(&stats.TotalSkills)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: skills count: %w", err)
	}
	stats.TotalSkills += len(systemskills.SkillSummaries())

	// Count devices.
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM devices WHERE user_id = $1`, userID).
		Scan(&stats.TotalDevices)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: devices count: %w", err)
	}

	// Count projects.
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM projects WHERE user_id = $1`, userID).
		Scan(&stats.TotalProjects)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: projects count: %w", err)
	}

	// Count inbox messages.
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM inbox_messages WHERE user_id = $1`, userID).
		Scan(&stats.TotalInbox)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: inbox count: %w", err)
	}

	// Weekly activity: count activity logs per day for the last 7 days.
	rows, err := s.db.Query(ctx,
		`SELECT to_char(created_at, 'YYYY-MM-DD') AS day, COUNT(*)
		 FROM activity_log
		 WHERE user_id = $1 AND created_at >= NOW() - INTERVAL '7 days'
		 GROUP BY day ORDER BY day ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: weekly activity: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var activity models.DashboardActivity
		if err := rows.Scan(&activity.Platform, &activity.Count); err != nil {
			return nil, fmt.Errorf("dashboard.GetStats: weekly activity scan: %w", err)
		}
		stats.WeeklyActivity = append(stats.WeeklyActivity, activity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: weekly activity rows: %w", err)
	}

	var pendingInbox int
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM inbox_messages
		 WHERE user_id = $1 AND action_required = true AND status = 'incoming'`,
		userID).
		Scan(&pendingInbox)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: pending inbox: %w", err)
	}

	if pendingInbox > 0 {
		stats.Pending = append(stats.Pending, models.DashboardPending{
			Type:    "inbox",
			Count:   pendingInbox,
			Message: "待处理收件箱消息",
		})
	}

	var pendingConflicts int
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM memory_conflicts WHERE user_id = $1 AND status = 'pending'`,
		userID).
		Scan(&pendingConflicts)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: pending conflicts: %w", err)
	}

	if pendingConflicts > 0 {
		stats.Pending = append(stats.Pending, models.DashboardPending{
			Type:    "conflict",
			Count:   pendingConflicts,
			Message: "待解决记忆冲突",
		})
	}

	return stats, nil
}
