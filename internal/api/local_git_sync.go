package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/agi-bar/agenthub/internal/localgitsync"
	"github.com/google/uuid"
)

func (s *Server) syncLocalGitMirror(ctx context.Context, userID uuid.UUID) *localgitsync.SyncInfo {
	if s == nil || s.LocalGitSync == nil {
		return nil
	}
	info, err := s.LocalGitSync.SyncActiveMirror(ctx, userID)
	if err != nil {
		slog.Warn("local git mirror sync failed", "user_id", userID.String(), "error", err)
	}
	return info
}

func respondOKWithLocalGitSync(w http.ResponseWriter, data interface{}, info *localgitsync.SyncInfo) {
	writeJSON(w, http.StatusOK, APISuccess{OK: true, Data: data, LocalGitSync: info})
}

func respondCreatedWithLocalGitSync(w http.ResponseWriter, data interface{}, info *localgitsync.SyncInfo) {
	writeJSON(w, http.StatusCreated, APISuccess{OK: true, Data: data, LocalGitSync: info})
}
