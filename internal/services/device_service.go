package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDeviceNotFound       = errors.New("device not found")
	ErrDeviceUnsupported    = errors.New("device protocol not supported")
	ErrDeviceInvalidRequest = errors.New("invalid device request")
	ErrDeviceUpstreamFailed = errors.New("device upstream failed")
)

type DeviceService struct {
	db       *pgxpool.Pool
	repo     DeviceRepo
	fileTree *FileTreeService
}

func NewDeviceService(db *pgxpool.Pool, fileTree *FileTreeService) *DeviceService {
	return &DeviceService{db: db, fileTree: fileTree}
}

func NewDeviceServiceWithRepo(repo DeviceRepo, fileTree *FileTreeService) *DeviceService {
	return &DeviceService{repo: repo, fileTree: fileTree}
}

func (s *DeviceService) List(ctx context.Context, userID uuid.UUID) ([]models.Device, error) {
	if s.repo != nil {
		devices, err := s.repo.List(ctx, userID)
		if err != nil {
			return nil, err
		}
		for i := range devices {
			if s.fileTree != nil {
				if skill, err := s.fileTree.Read(ctx, userID, hubpath.DeviceSkillPath(devices[i].Name), models.TrustLevelFull); err == nil {
					devices[i].SkillMD = skill.Content
				}
				if cfg, err := s.fileTree.Read(ctx, userID, hubpath.DeviceConfigPath(devices[i].Name), models.TrustLevelFull); err == nil {
					_ = json.Unmarshal([]byte(cfg.Content), &devices[i].Config)
				}
			}
		}
		return devices, nil
	}
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
		if s.fileTree != nil {
			if skill, err := s.fileTree.Read(ctx, userID, hubpath.DeviceSkillPath(d.Name), models.TrustLevelFull); err == nil {
				d.SkillMD = skill.Content
			}
			if cfg, err := s.fileTree.Read(ctx, userID, hubpath.DeviceConfigPath(d.Name), models.TrustLevelFull); err == nil {
				_ = json.Unmarshal([]byte(cfg.Content), &d.Config)
			}
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

	if s.repo != nil {
		if err := s.repo.Create(ctx, device); err != nil {
			return nil, fmt.Errorf("device.Register: %w", err)
		}
	} else {
		_, err := s.db.Exec(ctx,
			`INSERT INTO devices (id, user_id, name, device_type, brand, protocol, endpoint, skill_md, config, status, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			device.ID, device.UserID, device.Name, device.DeviceType, device.Brand,
			device.Protocol, device.Endpoint, device.SkillMD, device.Config, device.Status,
			device.CreatedAt, device.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("device.Register: %w", err)
		}
	}
	if err := s.syncDeviceTree(ctx, device); err != nil {
		return nil, err
	}
	return &device, nil
}

// Call sends an action to a device. Dispatches via protocol-specific handler.
func (s *DeviceService) Call(ctx context.Context, userID uuid.UUID, deviceName, action string, params map[string]interface{}) (map[string]interface{}, error) {
	if strings.TrimSpace(action) == "" {
		return nil, fmt.Errorf("%w: action is required", ErrDeviceInvalidRequest)
	}

	var (
		deviceID uuid.UUID
		protocol string
		endpoint string
	)
	if s.repo != nil {
		device, err := s.repo.GetByName(ctx, userID, deviceName)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrDeviceNotFound, deviceName)
		}
		deviceID = device.ID
		protocol = device.Protocol
		endpoint = device.Endpoint
	} else {
		err := s.db.QueryRow(ctx,
			`SELECT id, protocol, endpoint FROM devices WHERE user_id = $1 AND name = $2`,
			userID, deviceName).
			Scan(&deviceID, &protocol, &endpoint)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("%w: %s", ErrDeviceNotFound, deviceName)
			}
			return nil, fmt.Errorf("device.Call: lookup: %w", err)
		}
	}

	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "http", "homeassistant":
		if endpoint == "" {
			return nil, fmt.Errorf("%w: device %q has no endpoint configured", ErrDeviceUpstreamFailed, deviceName)
		}
		return s.callHTTP(ctx, deviceID, deviceName, endpoint, action, params)
	default:
		return nil, fmt.Errorf("%w: %s", ErrDeviceUnsupported, protocol)
	}
}

func (s *DeviceService) callHTTP(ctx context.Context, deviceID uuid.UUID, deviceName, endpoint, action string, params map[string]interface{}) (map[string]interface{}, error) {
	body, err := json.Marshal(map[string]interface{}{
		"action": action,
		"params": params,
	})
	if err != nil {
		return nil, fmt.Errorf("device.callHTTP: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("device.callHTTP: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: request failed: %v", ErrDeviceUpstreamFailed, err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		result = map[string]interface{}{
			"status_code": resp.StatusCode,
			"status":      resp.Status,
			"body":        strings.TrimSpace(string(bodyBytes)),
		}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: status=%d body=%s", ErrDeviceUpstreamFailed, resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	result["device_id"] = deviceID.String()
	result["device"] = deviceName
	result["action"] = action
	result["timestamp"] = time.Now().UTC().Format(time.RFC3339)
	return result, nil
}

func (s *DeviceService) syncDeviceTree(ctx context.Context, device models.Device) error {
	if s.fileTree == nil {
		return nil
	}
	if _, err := s.fileTree.WriteEntry(ctx, device.UserID, hubpath.DeviceSkillPath(device.Name), device.SkillMD, "text/markdown", models.FileTreeWriteOptions{
		Kind:          "device_skill",
		MinTrustLevel: models.TrustLevelCollaborate,
		Metadata: map[string]interface{}{
			"name":        device.Name,
			"description": strings.TrimSpace(device.DeviceType + " device"),
			"device_type": device.DeviceType,
			"protocol":    device.Protocol,
			"status":      device.Status,
			"brand":       device.Brand,
			"endpoint":    device.Endpoint,
			"source":      "devices",
		},
	}); err != nil {
		return fmt.Errorf("device.syncDeviceTree: skill: %w", err)
	}
	configJSON, err := json.MarshalIndent(device.Config, "", "  ")
	if err != nil {
		return fmt.Errorf("device.syncDeviceTree: marshal config: %w", err)
	}
	if _, err := s.fileTree.WriteEntry(ctx, device.UserID, hubpath.DeviceConfigPath(device.Name), string(configJSON), "application/json", models.FileTreeWriteOptions{
		Kind:          "device_config",
		MinTrustLevel: models.TrustLevelCollaborate,
		Metadata: map[string]interface{}{
			"device": device.Name,
			"source": "devices",
		},
	}); err != nil {
		return fmt.Errorf("device.syncDeviceTree: config: %w", err)
	}
	return nil
}
