package services

import (
	"context"
	"fmt"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProjectService struct {
	db   *pgxpool.Pool
	role *RoleService
}

func NewProjectService(db *pgxpool.Pool, role *RoleService) *ProjectService {
	return &ProjectService{db: db, role: role}
}

func (s *ProjectService) List(ctx context.Context, userID uuid.UUID) ([]models.Project, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, name, status, context_md, metadata, created_at, updated_at
		 FROM projects WHERE user_id = $1 AND status = 'active'
		 ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("project.List: %w", err)
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var p models.Project
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Status, &p.ContextMD,
			&p.Metadata, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("project.List: scan: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (s *ProjectService) Get(ctx context.Context, userID uuid.UUID, name string) (*models.Project, error) {
	var p models.Project
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, status, context_md, metadata, created_at, updated_at
		 FROM projects WHERE user_id = $1 AND name = $2`, userID, name).
		Scan(&p.ID, &p.UserID, &p.Name, &p.Status, &p.ContextMD,
			&p.Metadata, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("project.Get: %w", err)
	}
	return &p, nil
}

// Create creates a new project and a corresponding worker role scoped to it.
func (s *ProjectService) Create(ctx context.Context, userID uuid.UUID, name string) (*models.Project, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("project.Create: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	id := uuid.New()
	now := time.Now().UTC()

	_, err = tx.Exec(ctx,
		`INSERT INTO projects (id, user_id, name, status, context_md, metadata, created_at, updated_at)
		 VALUES ($1, $2, $3, 'active', '', '{}', $4, $4)`,
		id, userID, name, now)
	if err != nil {
		return nil, fmt.Errorf("project.Create: insert: %w", err)
	}

	// Create a worker role scoped to this project's path.
	roleName := "worker-" + name
	projectPath := "/projects/" + name + "/"
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, user_id, name, role_type, config, allowed_paths, allowed_vault_scopes, lifecycle, created_at)
		 VALUES ($1, $2, $3, 'worker', '{}', $4, '{}', 'project', $5)`,
		uuid.New(), userID, roleName, []string{projectPath}, now)
	if err != nil {
		return nil, fmt.Errorf("project.Create: create worker role: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("project.Create: commit: %w", err)
	}

	p := &models.Project{
		ID:        id,
		UserID:    userID,
		Name:      name,
		Status:    "active",
		ContextMD: "",
		Metadata:  map[string]interface{}{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	return p, nil
}

func (s *ProjectService) Archive(ctx context.Context, userID uuid.UUID, name string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE projects SET status = 'archived', updated_at = $1 WHERE user_id = $2 AND name = $3`,
		time.Now().UTC(), userID, name)
	if err != nil {
		return fmt.Errorf("project.Archive: %w", err)
	}
	return nil
}

func (s *ProjectService) UpdateContext(ctx context.Context, userID uuid.UUID, name, contextMD string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE projects SET context_md = $1, updated_at = $2 WHERE user_id = $3 AND name = $4`,
		contextMD, time.Now().UTC(), userID, name)
	if err != nil {
		return fmt.Errorf("project.UpdateContext: %w", err)
	}
	return nil
}

func (s *ProjectService) AppendLog(ctx context.Context, projectID uuid.UUID, log models.ProjectLog) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO project_logs (id, project_id, source, role, action, summary, artifacts, tags, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		uuid.New(), projectID, log.Source, log.Role, log.Action, log.Summary, log.Artifacts, log.Tags, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("project.AppendLog: %w", err)
	}
	return nil
}

func (s *ProjectService) GetLogs(ctx context.Context, projectID uuid.UUID, limit int) ([]models.ProjectLog, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, project_id, source, role, action, summary, artifacts, tags, created_at
		 FROM project_logs WHERE project_id = $1
		 ORDER BY created_at DESC LIMIT $2`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("project.GetLogs: %w", err)
	}
	defer rows.Close()

	var logs []models.ProjectLog
	for rows.Next() {
		var l models.ProjectLog
		if err := rows.Scan(&l.ID, &l.ProjectID, &l.Source, &l.Role, &l.Action,
			&l.Summary, &l.Artifacts, &l.Tags, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("project.GetLogs: scan: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
