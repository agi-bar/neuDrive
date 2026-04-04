package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProjectService struct {
	db       *pgxpool.Pool
	role     *RoleService
	fileTree *FileTreeService
}

func NewProjectService(db *pgxpool.Pool, role *RoleService, fileTree *FileTreeService) *ProjectService {
	return &ProjectService{db: db, role: role, fileTree: fileTree}
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
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Status, &p.ContextMD, &p.Metadata, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("project.List: scan: %w", err)
		}
		if s.fileTree != nil {
			if entry, err := s.fileTree.Read(ctx, userID, hubpath.ProjectContextPath(p.Name), models.TrustLevelFull); err == nil {
				p.ContextMD = entry.Content
				p.UpdatedAt = entry.UpdatedAt
			}
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
		Scan(&p.ID, &p.UserID, &p.Name, &p.Status, &p.ContextMD, &p.Metadata, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("project.Get: %w", err)
	}
	if s.fileTree != nil {
		if entry, err := s.fileTree.Read(ctx, userID, hubpath.ProjectContextPath(name), models.TrustLevelFull); err == nil {
			p.ContextMD = entry.Content
			p.UpdatedAt = entry.UpdatedAt
		}
	}
	return &p, nil
}

// Create creates a new project and a corresponding worker role scoped to it.
func (s *ProjectService) Create(ctx context.Context, userID uuid.UUID, name string) (*models.Project, error) {
	if err := validateSlug(name, 128); err != nil {
		return nil, fmt.Errorf("project.Create: invalid name: %w", err)
	}
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

	roleName := "worker-" + name
	projectPath := hubpath.ProjectDir(name)
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

	if s.fileTree != nil {
		if _, err := s.fileTree.EnsureDirectoryWithMetadata(ctx, userID, projectPath, map[string]interface{}{
			"project": name,
			"status":  "active",
		}, models.TrustLevelWork); err != nil {
			return nil, err
		}
		if _, err := s.fileTree.WriteEntry(ctx, userID, hubpath.ProjectContextPath(name), "", "text/markdown", models.FileTreeWriteOptions{
			Kind:          "project_context",
			MinTrustLevel: models.TrustLevelWork,
			Metadata: map[string]interface{}{
				"project": name,
				"status":  "active",
			},
		}); err != nil {
			return nil, err
		}
		if _, err := s.fileTree.WriteEntry(ctx, userID, hubpath.ProjectLogPath(name), "", "application/x-ndjson", models.FileTreeWriteOptions{
			Kind:          "project_log",
			MinTrustLevel: models.TrustLevelWork,
			Metadata:      map[string]interface{}{"project": name},
		}); err != nil {
			return nil, err
		}
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
	if s.fileTree != nil {
		_, _ = s.fileTree.EnsureDirectoryWithMetadata(ctx, userID, hubpath.ProjectDir(name), map[string]interface{}{
			"project": name,
			"status":  "archived",
		}, models.TrustLevelWork)
	}
	return nil
}

func (s *ProjectService) UpdateContext(ctx context.Context, userID uuid.UUID, name, contextMD string) error {
	if s.fileTree != nil {
		if _, err := s.fileTree.WriteEntry(ctx, userID, hubpath.ProjectContextPath(name), contextMD, "text/markdown", models.FileTreeWriteOptions{
			Kind:          "project_context",
			MinTrustLevel: models.TrustLevelWork,
			Metadata: map[string]interface{}{
				"project": name,
			},
		}); err != nil {
			return fmt.Errorf("project.UpdateContext: write canonical entry: %w", err)
		}
	}
	_, err := s.db.Exec(ctx,
		`UPDATE projects SET context_md = $1, updated_at = $2 WHERE user_id = $3 AND name = $4`,
		contextMD, time.Now().UTC(), userID, name)
	if err != nil {
		return fmt.Errorf("project.UpdateContext: %w", err)
	}
	return nil
}

func (s *ProjectService) AppendLog(ctx context.Context, projectID uuid.UUID, log models.ProjectLog) error {
	projectName, userID, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}
	log.ProjectID = projectID
	log.CreatedAt = now

	if s.fileTree != nil {
		path := hubpath.ProjectLogPath(projectName)
		current := ""
		if existing, err := s.fileTree.Read(ctx, userID, path, models.TrustLevelFull); err == nil {
			current = strings.TrimRight(existing.Content, "\n")
		}
		line, err := json.Marshal(log)
		if err != nil {
			return fmt.Errorf("project.AppendLog: marshal: %w", err)
		}
		nextContent := string(line)
		if current != "" {
			nextContent = current + "\n" + nextContent
		}
		if _, err := s.fileTree.WriteEntry(ctx, userID, path, nextContent, "application/x-ndjson", models.FileTreeWriteOptions{
			Kind:          "project_log",
			MinTrustLevel: models.TrustLevelWork,
			Metadata: map[string]interface{}{
				"project": projectName,
			},
		}); err != nil {
			return fmt.Errorf("project.AppendLog: write canonical entry: %w", err)
		}
	}

	_, err = s.db.Exec(ctx,
		`INSERT INTO project_logs (id, project_id, source, role, action, summary, artifacts, tags, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		log.ID, log.ProjectID, log.Source, log.Role, log.Action, log.Summary, log.Artifacts, log.Tags, log.CreatedAt)
	if err != nil {
		return fmt.Errorf("project.AppendLog: %w", err)
	}
	return nil
}

func (s *ProjectService) GetLogs(ctx context.Context, projectID uuid.UUID, limit int) ([]models.ProjectLog, error) {
	if limit <= 0 {
		limit = 50
	}

	if s.fileTree != nil {
		projectName, userID, err := s.projectIdentity(ctx, projectID)
		if err == nil {
			if entry, readErr := s.fileTree.Read(ctx, userID, hubpath.ProjectLogPath(projectName), models.TrustLevelFull); readErr == nil {
				logs := parseProjectLogs(entry.Content)
				if len(logs) > limit {
					logs = logs[len(logs)-limit:]
				}
				reverseProjectLogs(logs)
				return logs, nil
			}
		}
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
		if err := rows.Scan(&l.ID, &l.ProjectID, &l.Source, &l.Role, &l.Action, &l.Summary, &l.Artifacts, &l.Tags, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("project.GetLogs: scan: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (s *ProjectService) projectIdentity(ctx context.Context, projectID uuid.UUID) (string, uuid.UUID, error) {
	var name string
	var userID uuid.UUID
	err := s.db.QueryRow(ctx,
		`SELECT name, user_id FROM projects WHERE id = $1`,
		projectID,
	).Scan(&name, &userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", uuid.Nil, fmt.Errorf("project.projectIdentity: project not found")
		}
		return "", uuid.Nil, fmt.Errorf("project.projectIdentity: %w", err)
	}
	return name, userID, nil
}

func parseProjectLogs(content string) []models.ProjectLog {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	logs := make([]models.ProjectLog, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var log models.ProjectLog
		if err := json.Unmarshal([]byte(line), &log); err == nil {
			logs = append(logs, log)
		}
	}
	return logs
}

func reverseProjectLogs(logs []models.ProjectLog) {
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
}
