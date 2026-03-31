package models

import (
	"time"

	"github.com/google/uuid"
)

type ActivityLog struct {
	ID           uuid.UUID              `json:"id" db:"id"`
	UserID       uuid.UUID              `json:"user_id" db:"user_id"`
	ConnectionID uuid.UUID              `json:"connection_id" db:"connection_id"`
	Action       string                 `json:"action" db:"action"`
	Path         string                 `json:"path" db:"path"`
	Metadata     map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
}

type DashboardStats struct {
	TotalConnections int            `json:"connections"`
	TotalSkills      int            `json:"skills"`
	TotalDevices     int            `json:"devices"`
	TotalProjects    int            `json:"projects"`
	WeeklyActivity   map[string]int `json:"weekly_activity"`
	PendingConflicts int            `json:"pending_conflicts"`
}
