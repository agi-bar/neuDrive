package localstore

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/google/uuid"
)

func (s *Store) GetProfiles(ctx context.Context, userID uuid.UUID) ([]models.MemoryProfile, error) {
	snapshot, err := s.Snapshot(ctx, userID, "/memory/profile", models.TrustLevelFull)
	if err != nil {
		if err == services.ErrEntryNotFound {
			return nil, nil
		}
		return nil, err
	}
	profiles := make([]models.MemoryProfile, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		if entry.IsDirectory || !strings.HasSuffix(entry.Path, ".md") {
			continue
		}
		category := strings.TrimSuffix(path.Base(entry.Path), ".md")
		source, _ := entry.Metadata["source"].(string)
		profiles = append(profiles, models.MemoryProfile{
			ID:        entry.ID,
			UserID:    entry.UserID,
			Category:  category,
			Content:   entry.Content,
			Source:    source,
			CreatedAt: entry.CreatedAt,
			UpdatedAt: entry.UpdatedAt,
		})
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Category < profiles[j].Category })
	return profiles, nil
}

func (s *Store) UpsertProfile(ctx context.Context, userID uuid.UUID, category, content, source string) error {
	if strings.TrimSpace(category) == "" {
		return fmt.Errorf("profile category is required")
	}
	if strings.TrimSpace(source) == "" {
		source = "agenthub"
	}
	_, err := s.WriteEntry(ctx, userID, hubpath.ProfilePath(category), content, "text/markdown", models.FileTreeWriteOptions{
		Kind:          "memory_profile",
		MinTrustLevel: models.TrustLevelFull,
		Metadata: map[string]interface{}{
			"category": category,
			"source":   source,
		},
	})
	return err
}

func (s *Store) WriteScratchWithTitle(ctx context.Context, userID uuid.UUID, content, source, title string) (*models.FileTreeEntry, error) {
	now := time.Now().UTC()
	expiresAt := now.AddDate(0, 0, 7)
	return s.writeScratchEntry(ctx, userID, content, source, title, now, &expiresAt, uuid.New())
}

func (s *Store) ImportScratch(ctx context.Context, userID uuid.UUID, content, source, title string, createdAt time.Time, expiresAt *time.Time) (*models.FileTreeEntry, error) {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return s.writeScratchEntry(ctx, userID, content, source, title, createdAt.UTC(), expiresAt, uuid.NewSHA1(uuid.NameSpaceURL, []byte(content+title+source+createdAt.UTC().Format(time.RFC3339Nano))))
}

func (s *Store) writeScratchEntry(ctx context.Context, userID uuid.UUID, content, source, title string, createdAt time.Time, expiresAt *time.Time, id uuid.UUID) (*models.FileTreeEntry, error) {
	if strings.TrimSpace(source) == "" {
		source = "agenthub"
	}
	if expiresAt == nil {
		defaultExpiry := createdAt.AddDate(0, 0, 7)
		expiresAt = &defaultExpiry
	}
	slugBase := title
	if strings.TrimSpace(slugBase) == "" {
		slugBase = source
	}
	slug := fmt.Sprintf("%s-%s", slugBase, id.String()[:8])
	entry, err := s.WriteEntry(ctx, userID, hubpath.ScratchPath(createdAt, slug), content, "text/markdown", models.FileTreeWriteOptions{
		Kind:          "memory_scratch",
		MinTrustLevel: models.TrustLevelFull,
		Metadata: map[string]interface{}{
			"source":     source,
			"title":      title,
			"date":       createdAt.Format("2006-01-02"),
			"expires_at": expiresAt.UTC().Format(time.RFC3339),
			"legacy_id":  id.String(),
		},
	})
	if err != nil {
		return nil, err
	}
	entry.CreatedAt = createdAt
	entry.UpdatedAt = createdAt
	_, _ = s.db.ExecContext(ctx, `UPDATE file_tree SET created_at = ?, updated_at = ? WHERE id = ?`, timeText(createdAt), timeText(createdAt), entry.ID.String())
	return entry, nil
}

func (s *Store) GetScratchActive(ctx context.Context, userID uuid.UUID) ([]models.MemoryScratch, error) {
	return s.getScratch(ctx, userID, nil)
}

func (s *Store) GetScratch(ctx context.Context, userID uuid.UUID, days int) ([]models.MemoryScratch, error) {
	var cutoff *time.Time
	if days > 0 {
		ts := time.Now().UTC().AddDate(0, 0, -days)
		cutoff = &ts
	}
	return s.getScratch(ctx, userID, cutoff)
}

func (s *Store) getScratch(ctx context.Context, userID uuid.UUID, cutoff *time.Time) ([]models.MemoryScratch, error) {
	snapshot, err := s.Snapshot(ctx, userID, "/memory/scratch", models.TrustLevelFull)
	if err != nil {
		if err == services.ErrEntryNotFound {
			return nil, nil
		}
		return nil, err
	}
	items := make([]models.MemoryScratch, 0, len(snapshot.Entries))
	now := time.Now().UTC()
	for _, entry := range snapshot.Entries {
		if entry.IsDirectory || !strings.HasSuffix(entry.Path, ".md") {
			continue
		}
		expiresAt := metadataTime(entry.Metadata, "expires_at")
		if expiresAt != nil && !expiresAt.After(now) {
			continue
		}
		if cutoff != nil && entry.CreatedAt.Before(*cutoff) {
			continue
		}
		date, _ := entry.Metadata["date"].(string)
		source, _ := entry.Metadata["source"].(string)
		title, _ := entry.Metadata["title"].(string)
		items = append(items, models.MemoryScratch{
			ID:        entry.ID,
			UserID:    entry.UserID,
			Date:      date,
			Content:   entry.Content,
			Title:     title,
			Source:    source,
			ExpiresAt: expiresAt,
			CreatedAt: entry.CreatedAt,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (s *Store) ListProjects(ctx context.Context, userID uuid.UUID) ([]models.Project, error) {
	snapshot, err := s.Snapshot(ctx, userID, "/projects", models.TrustLevelFull)
	if err != nil {
		if err == services.ErrEntryNotFound {
			return nil, nil
		}
		return nil, err
	}
	projects := map[string]models.Project{}
	for _, entry := range snapshot.Entries {
		publicPath := hubpath.NormalizePublic(entry.Path)
		if entry.IsDirectory || !strings.HasSuffix(publicPath, "/context.md") {
			continue
		}
		name := path.Base(path.Dir(publicPath))
		project := projects[name]
		if project.ID == uuid.Nil {
			project.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("local-project:"+name))
			project.UserID = userID
			project.Name = name
			project.Status = "active"
			project.CreatedAt = entry.CreatedAt
		}
		project.ContextMD = entry.Content
		project.UpdatedAt = entry.UpdatedAt
		projects[name] = project
	}
	names := make([]string, 0, len(projects))
	for name := range projects {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]models.Project, 0, len(names))
	for _, name := range names {
		out = append(out, projects[name])
	}
	return out, nil
}

func (s *Store) CreateProject(ctx context.Context, userID uuid.UUID, name string) (*models.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("project name is required")
	}
	now := time.Now().UTC()
	entry, err := s.WriteEntry(ctx, userID, hubpath.ProjectContextPath(name), "", "text/markdown", models.FileTreeWriteOptions{
		Kind:          "project_context",
		MinTrustLevel: models.TrustLevelCollaborate,
	})
	if err != nil {
		return nil, err
	}
	_, err = s.WriteEntry(ctx, userID, hubpath.ProjectLogPath(name), "", "application/x-ndjson", models.FileTreeWriteOptions{
		Kind:          "project_log",
		MinTrustLevel: models.TrustLevelCollaborate,
	})
	if err != nil {
		return nil, err
	}
	return &models.Project{
		ID:        uuid.NewSHA1(uuid.NameSpaceURL, []byte("local-project:"+name)),
		UserID:    userID,
		Name:      name,
		Status:    "active",
		ContextMD: "",
		CreatedAt: entry.CreatedAt,
		UpdatedAt: now,
	}, nil
}

func (s *Store) GetProject(ctx context.Context, userID uuid.UUID, name string) (*models.Project, error) {
	entry, err := s.Read(ctx, userID, hubpath.ProjectContextPath(name), models.TrustLevelFull)
	if err != nil {
		return nil, err
	}
	return &models.Project{
		ID:        uuid.NewSHA1(uuid.NameSpaceURL, []byte("local-project:"+name)),
		UserID:    userID,
		Name:      name,
		Status:    "active",
		ContextMD: entry.Content,
		CreatedAt: entry.CreatedAt,
		UpdatedAt: entry.UpdatedAt,
	}, nil
}

func (s *Store) GetProjectLogs(ctx context.Context, userID uuid.UUID, name string, limit int) ([]models.ProjectLog, error) {
	entry, err := s.Read(ctx, userID, hubpath.ProjectLogPath(name), models.TrustLevelFull)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(entry.Content), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	logs := make([]models.ProjectLog, 0, len(lines))
	for _, line := range lines {
		var logEntry models.ProjectLog
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			continue
		}
		logs = append(logs, logEntry)
	}
	return logs, nil
}

func (s *Store) AppendProjectLog(ctx context.Context, userID uuid.UUID, name string, logEntry models.ProjectLog) error {
	entry, err := s.Read(ctx, userID, hubpath.ProjectLogPath(name), models.TrustLevelFull)
	if err != nil {
		if err == services.ErrEntryNotFound {
			if _, err := s.CreateProject(ctx, userID, name); err != nil {
				return err
			}
			entry, err = s.Read(ctx, userID, hubpath.ProjectLogPath(name), models.TrustLevelFull)
		}
		if err != nil {
			return err
		}
	}
	if logEntry.ID == uuid.Nil {
		logEntry.ID = uuid.New()
	}
	if logEntry.ProjectID == uuid.Nil {
		logEntry.ProjectID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("local-project:"+name))
	}
	if logEntry.CreatedAt.IsZero() {
		logEntry.CreatedAt = time.Now().UTC()
	}
	line, err := json.Marshal(logEntry)
	if err != nil {
		return err
	}
	content := strings.TrimRight(entry.Content, "\n")
	if content != "" {
		content += "\n"
	}
	content += string(line) + "\n"
	_, err = s.WriteEntry(ctx, userID, hubpath.ProjectLogPath(name), content, "application/x-ndjson", models.FileTreeWriteOptions{
		Kind:          "project_log",
		MinTrustLevel: models.TrustLevelCollaborate,
	})
	return err
}

func metadataTime(metadata map[string]interface{}, key string) *time.Time {
	raw, ok := metadata[key]
	if !ok {
		return nil
	}
	text, ok := raw.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil
	}
	ts, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return nil
	}
	utc := ts.UTC()
	return &utc
}
