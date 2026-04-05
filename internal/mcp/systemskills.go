package mcp

import (
	"context"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/systemskills"
)

func (s *MCPServer) systemSkillSnapshotDeps() systemskills.SnapshotDeps {
	var deps systemskills.SnapshotDeps

	if s.Connection != nil {
		deps.Connections = s.Connection
	}
	if s.OAuth != nil {
		deps.Grants = s.OAuth
	}
	if s.Memory != nil {
		deps.Profiles = s.Memory
	}
	if s.Project != nil {
		deps.Projects = s.Project
	}
	if s.FileTree != nil {
		deps.Skills = s.FileTree
	}

	return deps
}

func (s *MCPServer) renderSystemSkillEntry(ctx context.Context, entry *models.FileTreeEntry) *models.FileTreeEntry {
	return systemskills.MaybeRenderEntry(ctx, s.UserID, s.TrustLevel, entry, s.systemSkillSnapshotDeps())
}
