package services

import (
	"context"
	"fmt"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DeviceService struct {
	db *pgxpool.Pool
}

func NewDeviceService(db *pgxpool.Pool) *DeviceService {
	return &DeviceService{db: db}
}

func (s *DeviceService) List(ctx context.Context, userID uuid.UUID) ([]models.Device, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, name, device_type, brand, protocol, endpoint, skill_md, config, status, created_at, updated_at
		 FROM devices WHERE user_id = $1 ORDER BY name ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("device.List: %w", err)
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		var d models.Device
		if err := rows.Scan(&d.ID, &d.UserID, &d.Name, &d.DeviceType, &d.Brand,
			&d.Protocol, &d.Endpoint, &d.SkillMD, &d.Config, &d.Status, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("device.List: scan: %w", err)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (s *DeviceService) Register(ctx context.Context, userID uuid.UUID, device models.Device) (*models.Device, error) {
	device.ID = uuid.New()
	device.UserID = userID
	device.Status = "online"
	now := time.Now().UTC()
	device.CreatedAt = now
	device.UpdatedAt = now

	_, err := s.db.Exec(ctx,
		`INSERT INTO devices (id, user_id, name, device_type, brand, protocol, endpoint, skill_md, config, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		device.ID, device.UserID, device.Name, device.DeviceType, device.Brand,
		device.Protocol, device.Endpoint, device.SkillMD, device.Config, device.Status,
		device.CreatedAt, device.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("device.Register: %w", err)
	}
	return &device, nil
}

// Call sends an action to a device. For now, returns a mock response.
// Real device protocol dispatching will be implemented later.
func (s *DeviceService) Call(ctx context.Context, userID uuid.UUID, deviceName, action string, params map[string]interface{}) (map[string]interface{}, error) {
	// Verify the device exists and belongs to the user.
	var deviceID uuid.UUID
	var protocol, endpoint string
	err := s.db.QueryRow(ctx,
		`SELECT id, protocol, endpoint FROM devices WHERE user_id = $1 AND name = $2`,
		userID, deviceName).
		Scan(&deviceID, &protocol, &endpoint)
	if err != nil {
		return nil, fmt.Errorf("device.Call: device not found: %w", err)
	}

	// Mock response until real device protocol dispatching is implemented.
	result := map[string]interface{}{
		"device_id": deviceID.String(),
		"device":    deviceName,
		"action":    action,
		"params":    params,
		"status":    "ok",
		"message":   "mock response - real device calls not yet implemented",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	return result, nil
}
