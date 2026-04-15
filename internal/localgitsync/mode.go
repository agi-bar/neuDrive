package localgitsync

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
)

func (s *Service) configuredExecutionMode() string {
	if s == nil {
		return ExecutionModeLocal
	}
	switch strings.TrimSpace(s.executionMode) {
	case ExecutionModeHosted:
		return ExecutionModeHosted
	default:
		return ExecutionModeLocal
	}
}

func (s *Service) isHostedExecution() bool {
	return s.configuredExecutionMode() == ExecutionModeHosted
}

func (s *Service) rootPathForUser(userID uuid.UUID) (string, error) {
	if !s.isHostedExecution() {
		return "", nil
	}
	root := strings.TrimSpace(s.hostedRoot)
	if root == "" {
		return "", fmt.Errorf("GIT_MIRROR_HOSTED_ROOT is not configured")
	}
	return filepath.Join(root, userID.String()), nil
}

func (s *Service) ensureMirror(ctx context.Context, userID uuid.UUID) (models.LocalGitMirror, *models.LocalGitMirror, error) {
	active, err := s.mirrors.GetActiveLocalGitMirror(ctx, userID)
	if err != nil {
		return models.LocalGitMirror{}, nil, err
	}
	mirror := normalizeMirror(active)
	mirror.UserID = userID
	mirror.IsActive = true
	mirror.ExecutionMode = s.configuredExecutionMode()
	if mirror.SyncState == "" {
		mirror.SyncState = SyncStateIdle
	}
	if s.isHostedExecution() {
		rootPath, err := s.rootPathForUser(userID)
		if err != nil {
			return models.LocalGitMirror{}, active, err
		}
		mirror.RootPath = rootPath
	}
	return mirror, active, nil
}
