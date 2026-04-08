package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/agi-bar/agenthub/internal/services"
)

// JobConfig controls whether a job is enabled and how often it runs.
type JobConfig struct {
	Enabled  bool
	Interval time.Duration
}

// SchedulerConfig holds the configuration for all background jobs.
type SchedulerConfig struct {
	CleanExpiredScratch    JobConfig
	CleanExpiredTokens     JobConfig
	CleanExpiredSync       JobConfig
	ArchiveExpiredMessages JobConfig
	GenerateDailyScratch   JobConfig
}

// DefaultSchedulerConfig returns the default configuration for all jobs.
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		CleanExpiredScratch: JobConfig{
			Enabled:  true,
			Interval: 6 * time.Hour,
		},
		CleanExpiredTokens: JobConfig{
			Enabled:  true,
			Interval: 1 * time.Hour,
		},
		CleanExpiredSync: JobConfig{
			Enabled:  true,
			Interval: 1 * time.Hour,
		},
		ArchiveExpiredMessages: JobConfig{
			Enabled:  true,
			Interval: 1 * time.Hour,
		},
		GenerateDailyScratch: JobConfig{
			Enabled:  true,
			Interval: 24 * time.Hour,
		},
	}
}

// Scheduler manages periodic background jobs.
type Scheduler struct {
	memory *services.MemoryService
	token  *services.TokenService
	inbox  *services.InboxService
	sync   *services.SyncService
	logger *slog.Logger
	config SchedulerConfig
	stop   chan struct{}
	wg     sync.WaitGroup
}

// NewScheduler creates a new Scheduler with default configuration.
func NewScheduler(memory *services.MemoryService, token *services.TokenService, inbox *services.InboxService, syncSvc *services.SyncService, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		memory: memory,
		token:  token,
		inbox:  inbox,
		sync:   syncSvc,
		logger: logger,
		config: DefaultSchedulerConfig(),
		stop:   make(chan struct{}),
	}
}

// NewSchedulerWithConfig creates a new Scheduler with the given configuration.
func NewSchedulerWithConfig(memory *services.MemoryService, token *services.TokenService, inbox *services.InboxService, syncSvc *services.SyncService, logger *slog.Logger, config SchedulerConfig) *Scheduler {
	return &Scheduler{
		memory: memory,
		token:  token,
		inbox:  inbox,
		sync:   syncSvc,
		logger: logger,
		config: config,
		stop:   make(chan struct{}),
	}
}

// Start begins running all enabled periodic jobs in the background.
func (s *Scheduler) Start(ctx context.Context) {
	s.logger.Info("starting background job scheduler")

	if s.config.CleanExpiredScratch.Enabled && s.memory != nil {
		s.startJob(ctx, "CleanExpiredScratch", s.config.CleanExpiredScratch.Interval, s.cleanExpiredScratch)
	}
	if s.config.CleanExpiredTokens.Enabled && s.token != nil {
		s.startJob(ctx, "CleanExpiredTokens", s.config.CleanExpiredTokens.Interval, s.cleanExpiredTokens)
	}
	if s.config.CleanExpiredSync.Enabled && s.sync != nil {
		s.startJob(ctx, "CleanExpiredSyncSessions", s.config.CleanExpiredSync.Interval, s.cleanExpiredSyncSessions)
	}
	if s.config.ArchiveExpiredMessages.Enabled && s.inbox != nil {
		s.startJob(ctx, "ArchiveExpiredMessages", s.config.ArchiveExpiredMessages.Interval, s.archiveExpiredMessages)
	}
	if s.config.GenerateDailyScratch.Enabled && s.memory != nil {
		s.startJob(ctx, "GenerateDailyScratch", s.config.GenerateDailyScratch.Interval, s.generateDailyScratch)
	}

	s.logger.Info("background job scheduler started")
}

// Stop gracefully stops all jobs and waits for them to finish.
func (s *Scheduler) Stop() {
	s.logger.Info("stopping background job scheduler")
	close(s.stop)
	s.wg.Wait()
	s.logger.Info("background job scheduler stopped")
}

// startJob launches a goroutine that runs the given function at the specified interval.
// It runs the job once immediately on startup, then on each tick.
func (s *Scheduler) startJob(ctx context.Context, name string, interval time.Duration, fn func(context.Context)) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info("job registered", "job", name, "interval", interval.String())

		// Run once immediately at startup.
		fn(ctx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				fn(ctx)
			case <-s.stop:
				s.logger.Info("job stopping", "job", name)
				return
			case <-ctx.Done():
				s.logger.Info("job stopping (context cancelled)", "job", name)
				return
			}
		}
	}()
}

func (s *Scheduler) cleanExpiredScratch(ctx context.Context) {
	name := "CleanExpiredScratch"
	start := time.Now()
	s.logger.Info("job started", "job", name)

	count, err := s.memory.CleanExpiredScratch(ctx)
	duration := time.Since(start)

	if err != nil {
		s.logger.Error("job failed", "job", name, "error", err, "duration", duration.String())
		return
	}

	s.logger.Info("job completed", "job", name, "affected", count, "duration", duration.String())
}

func (s *Scheduler) cleanExpiredTokens(ctx context.Context) {
	name := "CleanExpiredTokens"
	start := time.Now()
	s.logger.Info("job started", "job", name)

	count, err := s.token.DeactivateExpiredTokens(ctx)
	duration := time.Since(start)

	if err != nil {
		s.logger.Error("job failed", "job", name, "error", err, "duration", duration.String())
		return
	}

	s.logger.Info("job completed", "job", name, "affected", count, "duration", duration.String())
}

func (s *Scheduler) cleanExpiredSyncSessions(ctx context.Context) {
	name := "CleanExpiredSyncSessions"
	start := time.Now()
	s.logger.Info("job started", "job", name)

	if s.sync == nil {
		s.logger.Info("job skipped", "job", name, "reason", "sync service not configured")
		return
	}

	result, err := s.sync.CleanExpiredSessions(ctx)
	duration := time.Since(start)

	if err != nil {
		s.logger.Error("job failed", "job", name, "error", err, "duration", duration.String())
		return
	}

	s.logger.Info(
		"job completed",
		"job", name,
		"expired_sessions", result.ExpiredSessions,
		"deleted_parts", result.DeletedParts,
		"deleted_bytes", result.DeletedBytes,
		"duration", duration.String(),
	)
}

func (s *Scheduler) archiveExpiredMessages(ctx context.Context) {
	name := "ArchiveExpiredMessages"
	start := time.Now()
	s.logger.Info("job started", "job", name)

	count, err := s.inbox.ArchiveExpiredMessages(ctx)
	duration := time.Since(start)

	if err != nil {
		s.logger.Error("job failed", "job", name, "error", err, "duration", duration.String())
		return
	}

	s.logger.Info("job completed", "job", name, "affected", count, "duration", duration.String())
}

func (s *Scheduler) generateDailyScratch(ctx context.Context) {
	name := "GenerateDailyScratch"
	start := time.Now()
	s.logger.Info("job started", "job", name)

	count, err := s.memory.GenerateDailyScratchPlaceholders(ctx)
	duration := time.Since(start)

	if err != nil {
		s.logger.Error("job failed", "job", name, "error", err, "duration", duration.String())
		return
	}

	s.logger.Info("job completed", "job", name, "affected", count, "duration", duration.String())
}
