package api

import (
	"context"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/systemskills"
	"github.com/google/uuid"
)

func (s *Server) systemSkillSnapshotDeps() systemskills.SnapshotDeps {
	var deps systemskills.SnapshotDeps

	if s.ConnectionService != nil {
		deps.Connections = s.ConnectionService
	}
	if s.OAuthService != nil {
		deps.Grants = s.OAuthService
	}
	if s.MemoryService != nil {
		deps.Profiles = s.MemoryService
	}
	if s.ProjectService != nil {
		deps.Projects = s.ProjectService
	}
	if s.FileTreeService != nil {
		deps.Skills = s.FileTreeService
	}

	return deps
}

func (s *Server) renderSystemSkillEntry(ctx context.Context, userID uuid.UUID, trustLevel int, entry *models.FileTreeEntry) *models.FileTreeEntry {
	return systemskills.MaybeRenderEntry(ctx, userID, trustLevel, entry, s.systemSkillSnapshotDeps())
}
