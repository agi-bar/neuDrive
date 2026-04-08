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
	InsertJob(ctx context.Context, job models.SyncJob) error
	FinishJob(ctx context.Context, jobID, userID uuid.UUID, status string, summary models.SyncJobSummary, errorMessage string) error
	StartSession(ctx context.Context, userID uuid.UUID, req models.SyncStartSessionRequest) (*models.SyncSessionResponse, error)
	UploadPart(ctx context.Context, userID, sessionID uuid.UUID, index int, data []byte) (*models.SyncSessionResponse, error)
	GetSession(ctx context.Context, userID, sessionID uuid.UUID) (*models.SyncSessionResponse, error)
	AbortSession(ctx context.Context, userID, sessionID uuid.UUID) error
	CommitSession(ctx context.Context, userID, sessionID uuid.UUID, req models.SyncCommitRequest) (*models.BundleImportResult, error)
	ListJobs(ctx context.Context, userID uuid.UUID) ([]models.SyncJob, error)
	GetJob(ctx context.Context, userID, jobID uuid.UUID) (*models.SyncJob, error)
	CleanExpiredSessions(ctx context.Context) (*SyncCleanupResult, error)
}

type UserRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetBySlug(ctx context.Context, slug string) (*models.User, error)
	GetAuthBinding(ctx context.Context, provider string, providerID string) (*models.AuthBinding, error)
}

type AuthRepo interface {
	RegisterUser(ctx context.Context, email, slug, displayName, passwordHash string, now time.Time) (*models.User, error)
	LookupLogin(ctx context.Context, email string) (*models.Credentials, *models.User, error)
	UpdateLoginStats(ctx context.Context, credentialID uuid.UUID, now time.Time) error
	CreateSession(ctx context.Context, userID uuid.UUID, refreshTokenHash, userAgent, ipAddress string, expiresAt, createdAt time.Time) error
	GetSession(ctx context.Context, refreshTokenHash string) (*models.Session, error)
	DeleteSessionByID(ctx context.Context, sessionID uuid.UUID) error
	DeleteSessionByRefreshHash(ctx context.Context, refreshTokenHash string) error
	CreateOrUpdateGitHubUser(ctx context.Context, githubID, login, displayName, email, avatarURL string, now time.Time) (*models.User, error)
	ListSessions(ctx context.Context, userID uuid.UUID) ([]models.Session, error)
	RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) error
	GetCredentialsByUserID(ctx context.Context, userID uuid.UUID) (*models.Credentials, error)
	UpdatePasswordHash(ctx context.Context, credentialID uuid.UUID, passwordHash string, now time.Time) error
	GetProfile(ctx context.Context, userID uuid.UUID) (*models.User, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, displayName, bio, timezone, language string, now time.Time) (*models.User, error)
}

type ConnectionRepo interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]models.Connection, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Connection, error)
	GetByAPIKey(ctx context.Context, apiKeyHash string) (*models.Connection, error)
	Create(ctx context.Context, conn models.Connection) error
	Update(ctx context.Context, id uuid.UUID, name string, trustLevel int, updatedAt time.Time) error
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateLastUsed(ctx context.Context, id uuid.UUID, lastUsedAt time.Time) error
}

type VaultRepo interface {
	ListScopes(ctx context.Context, userID uuid.UUID, trustLevel int) ([]models.VaultScope, error)
	GetEntry(ctx context.Context, userID uuid.UUID, scope string) (*models.VaultEntry, error)
	UpsertEntry(ctx context.Context, entry models.VaultEntry) error
	DeleteEntry(ctx context.Context, userID uuid.UUID, scope string) error
}

type RoleRepo interface {
	List(ctx context.Context, userID uuid.UUID) ([]models.Role, error)
	Create(ctx context.Context, role models.Role) error
	Delete(ctx context.Context, userID uuid.UUID, name string) error
	HasRole(ctx context.Context, userID uuid.UUID, name string) (bool, error)
}

type InboxRepo interface {
	ListMessages(ctx context.Context, userID uuid.UUID, role, status string) ([]models.InboxMessage, error)
	CreateMessage(ctx context.Context, userID uuid.UUID, msg models.InboxMessage) error
	GetMessage(ctx context.Context, msgID uuid.UUID) (models.InboxMessage, uuid.UUID, error)
	UpdateMessageStatus(ctx context.Context, msgID uuid.UUID, status string, archivedAt *time.Time) error
	ArchiveExpiredMessages(ctx context.Context, now time.Time) (int64, error)
	SearchMessages(ctx context.Context, userID uuid.UUID, query, scope string) ([]models.InboxMessage, error)
}

type DeviceRepo interface {
	List(ctx context.Context, userID uuid.UUID) ([]models.Device, error)
	Create(ctx context.Context, device models.Device) error
	GetByName(ctx context.Context, userID uuid.UUID, name string) (*models.Device, error)
}

type OAuthRepo interface {
	CreateApp(ctx context.Context, app models.OAuthApp) error
	GetAppByID(ctx context.Context, id uuid.UUID) (*models.OAuthApp, error)
	GetAppByClientID(ctx context.Context, clientID string) (*models.OAuthApp, error)
	DeleteApp(ctx context.Context, userID, appID uuid.UUID) error
	ListApps(ctx context.Context, userID uuid.UUID) ([]models.OAuthApp, error)
	CreateCode(ctx context.Context, code models.OAuthCode) error
	GetCodeByHash(ctx context.Context, codeHash string) (*models.OAuthCode, error)
	MarkCodeUsed(ctx context.Context, codeID uuid.UUID) error
	UpsertGrant(ctx context.Context, grant models.OAuthGrant) error
	GetGrant(ctx context.Context, userID, appID uuid.UUID) (*models.OAuthGrant, error)
	ListGrants(ctx context.Context, userID uuid.UUID) ([]models.OAuthGrantResponse, error)
	RevokeGrant(ctx context.Context, userID, grantID uuid.UUID) error
	GetUserSlug(ctx context.Context, userID uuid.UUID) (string, error)
}
