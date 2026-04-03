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

type DashboardActivity struct {
	Platform string `json:"platform"`
	Count    int    `json:"count"`
}

type DashboardPending struct {
	Type    string `json:"type"`
	Count   int    `json:"count"`
	Message string `json:"message"`
}

type DashboardStats struct {
	TotalConnections int                 `json:"connections"`
	TotalSkills      int                 `json:"skills"`
	TotalDevices     int                 `json:"devices"`
	TotalProjects    int                 `json:"projects"`
	WeeklyActivity   []DashboardActivity `json:"weekly_activity"`
	Pending          []DashboardPending  `json:"pending"`
}
