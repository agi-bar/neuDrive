package models

import (
	"time"

	"github.com/google/uuid"
)

type Device struct {
	ID         uuid.UUID              `json:"id" db:"id"`
	UserID     uuid.UUID              `json:"user_id" db:"user_id"`
	Name       string                 `json:"name" db:"name"`
	DeviceType string                 `json:"device_type" db:"device_type"`
	Brand      string                 `json:"brand" db:"brand"`
	Protocol   string                 `json:"protocol" db:"protocol"`
	Endpoint   string                 `json:"endpoint" db:"endpoint"`
	SkillMD    string                 `json:"skill_md" db:"skill_md"`
	Config     map[string]interface{} `json:"config,omitempty" db:"config"`
	Status     string                 `json:"status" db:"status"`
	CreatedAt  time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at" db:"updated_at"`
}
