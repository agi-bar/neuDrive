package services

import (
	"context"
	"fmt"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DashboardService struct {
	db *pgxpool.Pool
}

func NewDashboardService(db *pgxpool.Pool) *DashboardService {
	return &DashboardService{db: db}
}

// GetStats aggregates dashboard statistics for a user.
func (s *DashboardService) GetStats(ctx context.Context, userID uuid.UUID) (*models.DashboardStats, error) {
	stats := &models.DashboardStats{
		WeeklyActivity: make(map[string]int),
	}

	// Count connections.
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM connections WHERE user_id = $1`, userID).
		Scan(&stats.TotalConnections)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: connections count: %w", err)
	}

	// Count skills (non-directory file_tree entries under /skills/).
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM file_tree WHERE user_id = $1 AND is_directory = false AND path LIKE '/skills/%'`, userID).
		Scan(&stats.TotalSkills)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: skills count: %w", err)
	}

	// Count devices.
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM devices WHERE user_id = $1`, userID).
		Scan(&stats.TotalDevices)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: devices count: %w", err)
	}

	// Count active projects.
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM projects WHERE user_id = $1 AND status = 'active'`, userID).
		Scan(&stats.TotalProjects)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: projects count: %w", err)
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
		var day string
		var count int
		if err := rows.Scan(&day, &count); err != nil {
			return nil, fmt.Errorf("dashboard.GetStats: weekly activity scan: %w", err)
		}
		stats.WeeklyActivity[day] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: weekly activity rows: %w", err)
	}

	// Pending conflicts: count inbox messages with action_required=true and status='incoming'.
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM inbox_messages
		 WHERE to_address LIKE $1 || '%' AND action_required = true AND status = 'incoming'`,
		userID.String()).
		Scan(&stats.PendingConflicts)
	if err != nil {
		return nil, fmt.Errorf("dashboard.GetStats: pending conflicts: %w", err)
	}

	return stats, nil
}
