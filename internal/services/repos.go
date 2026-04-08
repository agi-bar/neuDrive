package services

import (
	"context"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
)

type TokenRepo interface {
	CreateToken(ctx context.Context, userID uuid.UUID, req models.CreateTokenRequest) (*models.CreateTokenResponse, error)
	CreateEphemeralToken(ctx context.Context, userID uuid.UUID, name string, scopes []string, maxTrustLevel int, ttl time.Duration) (*models.CreateTokenResponse, error)
	ValidateToken(ctx context.Context, rawToken string) (*models.ScopedToken, error)
	ListTokens(ctx context.Context, userID uuid.UUID) ([]models.ScopedToken, error)
	RevokeToken(ctx context.Context, userID, tokenID uuid.UUID) error
	UpdateTokenName(ctx context.Context, userID, tokenID uuid.UUID, name string) error
	GetTokenByID(ctx context.Context, tokenID, userID uuid.UUID) (*models.ScopedToken, error)
	CheckRateLimit(ctx context.Context, token *models.ScopedToken) error
	DeactivateExpiredTokens(ctx context.Context) (int64, error)
}

type FileTreeRepo interface {
	List(ctx context.Context, userID uuid.UUID, path string, trustLevel int) ([]models.FileTreeEntry, error)
	Read(ctx context.Context, userID uuid.UUID, path string, trustLevel int) (*models.FileTreeEntry, error)
	WriteEntry(ctx context.Context, userID uuid.UUID, path string, content string, contentType string, opts models.FileTreeWriteOptions) (*models.FileTreeEntry, error)
	WriteBinaryEntry(ctx context.Context, userID uuid.UUID, path string, data []byte, contentType string, opts models.FileTreeWriteOptions) (*models.FileTreeEntry, error)
	Delete(ctx context.Context, userID uuid.UUID, path string) error
	Search(ctx context.Context, userID uuid.UUID, query string, trustLevel int, pathPrefix string) ([]models.FileTreeEntry, error)
	EnsureDirectory(ctx context.Context, userID uuid.UUID, path string) error
	Snapshot(ctx context.Context, userID uuid.UUID, pathPrefix string, trustLevel int) (*models.EntrySnapshot, error)
	ListSkillSummaries(ctx context.Context, userID uuid.UUID, trustLevel int) ([]models.SkillSummary, error)
	ReadBinary(ctx context.Context, userID uuid.UUID, path string, trustLevel int) ([]byte, *models.FileTreeEntry, error)
}

type MemoryRepo interface {
	GetProfiles(ctx context.Context, userID uuid.UUID) ([]models.MemoryProfile, error)
	UpsertProfile(ctx context.Context, userID uuid.UUID, category, content, source string) error
	GetScratch(ctx context.Context, userID uuid.UUID, days int) ([]models.MemoryScratch, error)
	GetScratchActive(ctx context.Context, userID uuid.UUID) ([]models.MemoryScratch, error)
	WriteScratchWithTitle(ctx context.Context, userID uuid.UUID, content, source, title string) (*models.FileTreeEntry, error)
	ImportScratch(ctx context.Context, userID uuid.UUID, content, source, title string, createdAt time.Time, expiresAt *time.Time) (*models.FileTreeEntry, error)
}

type ProjectRepo interface {
	ListProjects(ctx context.Context, userID uuid.UUID) ([]models.Project, error)
	GetProject(ctx context.Context, userID uuid.UUID, name string) (*models.Project, error)
	GetProjectIdentity(ctx context.Context, projectID uuid.UUID) (string, uuid.UUID, error)
	CreateProject(ctx context.Context, userID uuid.UUID, name string) (*models.Project, error)
	ArchiveProject(ctx context.Context, userID uuid.UUID, name string) error
	UpdateProjectContext(ctx context.Context, userID uuid.UUID, name, contextMD string) error
	AppendProjectLog(ctx context.Context, userID uuid.UUID, name string, log models.ProjectLog) error
	GetProjectLogs(ctx context.Context, userID uuid.UUID, name string, limit int) ([]models.ProjectLog, error)
}

type DashboardRepo interface {
	GetStats(ctx context.Context, userID uuid.UUID) (*models.DashboardStats, error)
}

type SyncRepo interface {
	ExportBundleJSON(ctx context.Context, userID uuid.UUID, filters models.BundleFilters) (*models.Bundle, error)
	ExportArchive(ctx context.Context, userID uuid.UUID, filters models.BundleFilters) ([]byte, *models.BundleArchiveManifest, error)
	StartSession(ctx context.Context, userID uuid.UUID, req models.SyncStartSessionRequest) (*models.SyncSessionResponse, error)
	UploadPart(ctx context.Context, userID, sessionID uuid.UUID, index int, data []byte) (*models.SyncSessionResponse, error)
	GetSession(ctx context.Context, userID, sessionID uuid.UUID) (*models.SyncSessionResponse, error)
	AbortSession(ctx context.Context, userID, sessionID uuid.UUID) error
	CommitSession(ctx context.Context, userID, sessionID uuid.UUID, req models.SyncCommitRequest) (*models.BundleImportResult, error)
	ListJobs(ctx context.Context, userID uuid.UUID) ([]models.SyncJob, error)
	GetJob(ctx context.Context, userID, jobID uuid.UUID) (*models.SyncJob, error)
	CleanExpiredSessions(ctx context.Context) (*SyncCleanupResult, error)
}
