package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
	"github.com/google/uuid"
)

type TokenRepo struct {
	Store *Store
}

func NewTokenRepo(store *Store) services.TokenRepo {
	return &TokenRepo{Store: store}
}

func (r *TokenRepo) CreateToken(ctx context.Context, userID uuid.UUID, req models.CreateTokenRequest) (*models.CreateTokenResponse, error) {
	ttl := time.Duration(req.ExpiresInDays) * 24 * time.Hour
	return r.Store.CreateToken(ctx, userID, req.Name, req.Scopes, req.MaxTrustLevel, ttl)
}

func (r *TokenRepo) CreateEphemeralToken(ctx context.Context, userID uuid.UUID, name string, scopes []string, maxTrustLevel int, ttl time.Duration) (*models.CreateTokenResponse, error) {
	return r.Store.CreateToken(ctx, userID, name, scopes, maxTrustLevel, ttl)
}

func (r *TokenRepo) ValidateToken(ctx context.Context, rawToken string) (*models.ScopedToken, error) {
	return r.Store.ValidateToken(ctx, rawToken)
}

func (r *TokenRepo) ListTokens(ctx context.Context, userID uuid.UUID) ([]models.ScopedToken, error) {
	return r.listTokens(ctx, userID)
}

func (r *TokenRepo) RevokeToken(ctx context.Context, userID, tokenID uuid.UUID) error {
	return r.Store.RevokeToken(ctx, userID, tokenID)
}

func (r *TokenRepo) UpdateTokenName(ctx context.Context, userID, tokenID uuid.UUID, name string) error {
	result, err := r.Store.DB().ExecContext(ctx,
		`UPDATE scoped_tokens
		    SET name = ?
		  WHERE id = ? AND user_id = ?`,
		name,
		tokenID.String(),
		userID.String(),
	)
	if err != nil {
		return fmt.Errorf("sqlite.TokenRepo.UpdateTokenName: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return services.ErrEntryNotFound
	}
	return nil
}

func (r *TokenRepo) GetTokenByID(ctx context.Context, tokenID, userID uuid.UUID) (*models.ScopedToken, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, scopes_json, max_trust_level,
		        expires_at, rate_limit, request_count, rate_limit_reset_at,
		        last_used_at, last_used_ip, created_at, revoked_at
		   FROM scoped_tokens
		  WHERE id = ? AND user_id = ?`,
		tokenID.String(),
		userID.String(),
	)
	token, err := scanScopedToken(row)
	if err != nil {
		return nil, services.ErrEntryNotFound
	}
	return token, nil
}

func (r *TokenRepo) CheckRateLimit(ctx context.Context, token *models.ScopedToken) error {
	return r.Store.CheckRateLimit(ctx, token)
}

func (r *TokenRepo) DeactivateExpiredTokens(ctx context.Context) (int64, error) {
	result, err := r.Store.DB().ExecContext(ctx,
		`UPDATE scoped_tokens
		    SET revoked_at = expires_at
		  WHERE revoked_at IS NULL AND expires_at < ?`,
		timeText(time.Now().UTC()),
	)
	if err != nil {
		return 0, fmt.Errorf("sqlite.TokenRepo.DeactivateExpiredTokens: %w", err)
	}
	return result.RowsAffected()
}

func (r *TokenRepo) listTokens(ctx context.Context, userID uuid.UUID) ([]models.ScopedToken, error) {
	rows, err := r.Store.DB().QueryContext(ctx,
		`SELECT id, user_id, name, token_hash, token_prefix, scopes_json, max_trust_level,
		        expires_at, rate_limit, request_count, rate_limit_reset_at,
		        last_used_at, last_used_ip, created_at, revoked_at
		   FROM scoped_tokens
		  WHERE user_id = ?
		  ORDER BY created_at DESC`,
		userID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.TokenRepo.ListTokens: %w", err)
	}
	defer rows.Close()

	var tokens []models.ScopedToken
	for rows.Next() {
		token, scanErr := scanScopedToken(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("sqlite.TokenRepo.ListTokens: %w", scanErr)
		}
		tokens = append(tokens, *token)
	}
	if tokens == nil {
		tokens = []models.ScopedToken{}
	}
	return tokens, rows.Err()
}

type MemoryRepo struct {
	Store *Store
}

func NewMemoryRepo(store *Store) services.MemoryRepo {
	return &MemoryRepo{Store: store}
}

func (r *MemoryRepo) GetProfiles(ctx context.Context, userID uuid.UUID) ([]models.MemoryProfile, error) {
	return r.Store.GetProfiles(ctx, userID)
}

func (r *MemoryRepo) UpsertProfile(ctx context.Context, userID uuid.UUID, category, content, source string) error {
	return r.Store.UpsertProfile(ctx, userID, category, content, source)
}

func (r *MemoryRepo) GetScratch(ctx context.Context, userID uuid.UUID, days int) ([]models.MemoryScratch, error) {
	return r.Store.GetScratch(ctx, userID, days)
}

func (r *MemoryRepo) GetScratchActive(ctx context.Context, userID uuid.UUID) ([]models.MemoryScratch, error) {
	return r.Store.GetScratchActive(ctx, userID)
}

func (r *MemoryRepo) WriteScratchWithTitle(ctx context.Context, userID uuid.UUID, content, source, title string) (*models.FileTreeEntry, error) {
	return r.Store.WriteScratchWithTitle(ctx, userID, content, source, title)
}

func (r *MemoryRepo) ImportScratch(ctx context.Context, userID uuid.UUID, content, source, title string, createdAt time.Time, expiresAt *time.Time) (*models.FileTreeEntry, error) {
	return r.Store.ImportScratch(ctx, userID, content, source, title, createdAt, expiresAt)
}

type ProjectRepo struct {
	Store *Store
}

type FileTreeRepo struct {
	Store *Store
}

func NewFileTreeRepo(store *Store) services.FileTreeRepo {
	return &FileTreeRepo{Store: store}
}

func (r *FileTreeRepo) List(ctx context.Context, userID uuid.UUID, path string, trustLevel int) ([]models.FileTreeEntry, error) {
	return r.Store.List(ctx, userID, path, trustLevel)
}

func (r *FileTreeRepo) Read(ctx context.Context, userID uuid.UUID, path string, trustLevel int) (*models.FileTreeEntry, error) {
	return r.Store.Read(ctx, userID, path, trustLevel)
}

func (r *FileTreeRepo) WriteEntry(ctx context.Context, userID uuid.UUID, path string, content string, contentType string, opts models.FileTreeWriteOptions) (*models.FileTreeEntry, error) {
	return r.Store.WriteEntry(ctx, userID, path, content, contentType, opts)
}

func (r *FileTreeRepo) WriteBinaryEntry(ctx context.Context, userID uuid.UUID, path string, data []byte, contentType string, opts models.FileTreeWriteOptions) (*models.FileTreeEntry, error) {
	return r.Store.WriteBinaryEntry(ctx, userID, path, data, contentType, opts)
}

func (r *FileTreeRepo) Delete(ctx context.Context, userID uuid.UUID, path string) error {
	return r.Store.Delete(ctx, userID, path)
}

func (r *FileTreeRepo) Search(ctx context.Context, userID uuid.UUID, query string, trustLevel int, pathPrefix string) ([]models.FileTreeEntry, error) {
	return r.Store.Search(ctx, userID, query, trustLevel, pathPrefix)
}

func (r *FileTreeRepo) EnsureDirectory(ctx context.Context, userID uuid.UUID, path string) error {
	return r.Store.EnsureDirectory(ctx, userID, path)
}

func (r *FileTreeRepo) Snapshot(ctx context.Context, userID uuid.UUID, pathPrefix string, trustLevel int) (*models.EntrySnapshot, error) {
	return r.Store.Snapshot(ctx, userID, pathPrefix, trustLevel)
}

func (r *FileTreeRepo) ListSkillSummaries(ctx context.Context, userID uuid.UUID, trustLevel int) ([]models.SkillSummary, error) {
	return r.Store.ListSkillSummaries(ctx, userID, trustLevel)
}

func (r *FileTreeRepo) ReadBinary(ctx context.Context, userID uuid.UUID, path string, trustLevel int) ([]byte, *models.FileTreeEntry, error) {
	return r.Store.ReadBinary(ctx, userID, path, trustLevel)
}

func (r *FileTreeRepo) ReadBlobByEntryID(ctx context.Context, entryID uuid.UUID) ([]byte, bool, error) {
	return r.Store.ReadBlobByEntryID(ctx, entryID)
}

func NewProjectRepo(store *Store) services.ProjectRepo {
	return &ProjectRepo{Store: store}
}

func (r *ProjectRepo) ListProjects(ctx context.Context, userID uuid.UUID) ([]models.Project, error) {
	return r.Store.ListProjects(ctx, userID)
}

func (r *ProjectRepo) GetProject(ctx context.Context, userID uuid.UUID, name string) (*models.Project, error) {
	return r.Store.GetProject(ctx, userID, name)
}

func (r *ProjectRepo) GetProjectIdentity(ctx context.Context, projectID uuid.UUID) (string, uuid.UUID, error) {
	userIDs, err := r.Store.ListUserIDs(ctx)
	if err != nil {
		return "", uuid.Nil, err
	}
	for _, userID := range userIDs {
		projects, err := r.Store.ListProjects(ctx, userID)
		if err != nil {
			return "", uuid.Nil, err
		}
		for _, project := range projects {
			if project.ID == projectID {
				return project.Name, userID, nil
			}
		}
	}
	return "", uuid.Nil, services.ErrEntryNotFound
}

func (r *ProjectRepo) CreateProject(ctx context.Context, userID uuid.UUID, name string) (*models.Project, error) {
	return r.Store.CreateProject(ctx, userID, name)
}

func (r *ProjectRepo) ArchiveProject(ctx context.Context, userID uuid.UUID, name string) error {
	project, err := r.Store.GetProject(ctx, userID, name)
	if err != nil {
		return err
	}
	project.Status = "archived"
	_, err = r.Store.WriteEntry(ctx, userID, hubpath.ProjectContextPath(name), project.ContextMD, "text/markdown", models.FileTreeWriteOptions{
		Kind:          "project_context",
		MinTrustLevel: models.TrustLevelCollaborate,
		Metadata:      map[string]interface{}{"project": name, "status": "archived"},
	})
	return err
}

func (r *ProjectRepo) UpdateProjectContext(ctx context.Context, userID uuid.UUID, name, contextMD string) error {
	_, err := r.Store.WriteEntry(ctx, userID, hubpath.ProjectContextPath(name), contextMD, "text/markdown", models.FileTreeWriteOptions{
		Kind:          "project_context",
		MinTrustLevel: models.TrustLevelCollaborate,
		Metadata:      map[string]interface{}{"project": name},
	})
	return err
}

func (r *ProjectRepo) AppendProjectLog(ctx context.Context, userID uuid.UUID, name string, log models.ProjectLog) error {
	return r.Store.AppendProjectLog(ctx, userID, name, log)
}

func (r *ProjectRepo) GetProjectLogs(ctx context.Context, userID uuid.UUID, name string, limit int) ([]models.ProjectLog, error) {
	return r.Store.GetProjectLogs(ctx, userID, name, limit)
}
