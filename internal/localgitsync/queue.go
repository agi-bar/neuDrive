package localgitsync

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

var hostedRetryBackoff = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}

func (s *Service) QueueOrSyncActiveMirror(ctx context.Context, userID uuid.UUID, reason string) (*SyncInfo, error) {
	if s == nil || s.mirrors == nil {
		return nil, nil
	}
	if s.isHostedExecution() {
		return s.MarkMirrorQueued(ctx, userID, reason)
	}
	return s.SyncActiveMirror(ctx, userID)
}

func (s *Service) MarkMirrorQueued(ctx context.Context, userID uuid.UUID, _ string) (*SyncInfo, error) {
	active, err := s.mirrors.GetActiveLocalGitMirror(ctx, userID)
	if err != nil {
		return nil, err
	}
	if active == nil {
		return &SyncInfo{Enabled: false, ExecutionMode: s.configuredExecutionMode(), SyncState: SyncStateIdle}, nil
	}
	mirror := normalizeMirror(active)
	if strings.TrimSpace(mirror.RemoteURL) == "" {
		return s.buildSyncInfo(ctx, userID, mirror, false, false, false, false), nil
	}
	now := time.Now().UTC()
	mirror.ExecutionMode = s.configuredExecutionMode()
	mirror.SyncRequestedAt = &now
	mirror.SyncNextAttemptAt = nil
	if mirror.SyncState != SyncStateRunning {
		mirror.SyncState = SyncStateQueued
		mirror.SyncAttemptCount = 0
	}
	mirror.UpdatedAt = now
	if err := s.mirrors.UpsertActiveLocalGitMirror(ctx, mirror); err != nil {
		return nil, err
	}
	return s.buildSyncInfo(ctx, userID, mirror, false, false, false, false), nil
}

func (s *Service) RunQueuedGitMirrorSyncs(ctx context.Context, limit int) error {
	if s == nil || s.mirrors == nil || !s.isHostedExecution() {
		return nil
	}
	queued, err := s.mirrors.ListQueuedLocalGitMirrors(ctx, s.configuredExecutionMode(), time.Now().UTC(), limit)
	if err != nil {
		return err
	}
	for _, mirror := range queued {
		if !s.tryAcquireUserRun(mirror.UserID) {
			continue
		}
		if err := s.runQueuedMirror(ctx, mirror.UserID); err != nil {
			slog.Warn("git mirror queued sync failed", "user_id", mirror.UserID.String(), "error", err)
		}
		s.releaseUserRun(mirror.UserID)
	}
	return nil
}

func (s *Service) runQueuedMirror(ctx context.Context, userID uuid.UUID) error {
	active, err := s.mirrors.GetActiveLocalGitMirror(ctx, userID)
	if err != nil || active == nil {
		return err
	}
	mirror := normalizeMirror(active)
	now := time.Now().UTC()
	mirror.ExecutionMode = s.configuredExecutionMode()
	mirror.SyncState = SyncStateRunning
	mirror.SyncStartedAt = &now
	mirror.SyncNextAttemptAt = nil
	mirror.UpdatedAt = now
	if err := s.mirrors.UpsertActiveLocalGitMirror(ctx, mirror); err != nil {
		return err
	}

	_, syncErr := s.SyncActiveMirror(ctx, userID)
	current, err := s.mirrors.GetActiveLocalGitMirror(ctx, userID)
	if err != nil || current == nil {
		return err
	}
	mirror = normalizeMirror(current)
	requestedDuringRun := mirror.SyncRequestedAt != nil && mirror.SyncStartedAt != nil && mirror.SyncRequestedAt.After(*mirror.SyncStartedAt)
	mirror.UpdatedAt = time.Now().UTC()
	if syncErr != nil {
		mirror.SyncAttemptCount++
		if mirror.SyncAttemptCount <= len(hostedRetryBackoff) {
			next := time.Now().UTC().Add(hostedRetryBackoff[mirror.SyncAttemptCount-1])
			mirror.SyncState = SyncStateQueued
			mirror.SyncNextAttemptAt = &next
		} else {
			mirror.SyncState = SyncStateError
			mirror.SyncNextAttemptAt = nil
		}
		if requestedDuringRun && mirror.SyncAttemptCount <= len(hostedRetryBackoff) {
			mirror.SyncState = SyncStateQueued
		}
		return s.mirrors.UpsertActiveLocalGitMirror(ctx, mirror)
	}

	mirror.SyncAttemptCount = 0
	mirror.SyncNextAttemptAt = nil
	if requestedDuringRun {
		mirror.SyncState = SyncStateQueued
	} else {
		mirror.SyncState = SyncStateIdle
	}
	return s.mirrors.UpsertActiveLocalGitMirror(ctx, mirror)
}

func (s *Service) tryAcquireUserRun(userID uuid.UUID) bool {
	_, loaded := s.runningUsers.LoadOrStore(userID.String(), struct{}{})
	return !loaded
}

func (s *Service) releaseUserRun(userID uuid.UUID) {
	s.runningUsers.Delete(userID.String())
}
