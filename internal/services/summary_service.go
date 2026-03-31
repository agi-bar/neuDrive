package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SummaryService generates human-readable markdown summaries from project logs.
type SummaryService struct {
	DB      *pgxpool.Pool
	Project *ProjectService
}

// NewSummaryService creates a new SummaryService.
func NewSummaryService(db *pgxpool.Pool, project *ProjectService) *SummaryService {
	return &SummaryService{DB: db, Project: project}
}

// GenerateProjectSummary generates a markdown summary from recent project logs.
// This is a rule-based summary (no LLM needed). It groups logs by date and action type.
func (s *SummaryService) GenerateProjectSummary(ctx context.Context, projectID uuid.UUID) (string, error) {
	// 1. Fetch project info.
	var p models.Project
	err := s.DB.QueryRow(ctx,
		`SELECT id, user_id, name, status, context_md, metadata, created_at, updated_at
		 FROM projects WHERE id = $1`, projectID).
		Scan(&p.ID, &p.UserID, &p.Name, &p.Status, &p.ContextMD,
			&p.Metadata, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return "", fmt.Errorf("summary.GenerateProjectSummary: fetch project: %w", err)
	}

	// 2. Fetch last 50 logs.
	logs, err := s.Project.GetLogs(ctx, projectID, 50)
	if err != nil {
		return "", fmt.Errorf("summary.GenerateProjectSummary: fetch logs: %w", err)
	}

	// 3. Build the markdown.
	var b strings.Builder

	b.WriteString("# ")
	b.WriteString(p.Name)
	b.WriteString("\n\n")

	b.WriteString("## 状态: ")
	b.WriteString(p.Status)
	b.WriteString("\n\n")

	// Group logs by week label.
	if len(logs) == 0 {
		b.WriteString("## 最近活动\n\n_暂无活动记录_\n")
		return b.String(), nil
	}

	b.WriteString("## 最近活动\n\n")

	// Sort logs oldest-first so we can group chronologically,
	// then reverse for display (newest week first).
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].CreatedAt.Before(logs[j].CreatedAt)
	})

	now := time.Now().UTC()
	weekGroups := groupLogsByWeek(logs, now)

	// Display newest weeks first.
	for i := len(weekGroups) - 1; i >= 0; i-- {
		g := weekGroups[i]
		b.WriteString("### ")
		b.WriteString(g.dateLabel)
		b.WriteString(" (")
		b.WriteString(g.weekLabel)
		b.WriteString(")\n")
		for _, l := range g.logs {
			b.WriteString("- [")
			b.WriteString(l.Source)
			b.WriteString("] ")
			b.WriteString(l.Action)
			b.WriteString(": ")
			b.WriteString(l.Summary)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// 4. Key tags section.
	tagCounts := make(map[string]int)
	for _, l := range logs {
		for _, t := range l.Tags {
			tagCounts[t]++
		}
	}
	if len(tagCounts) > 0 {
		b.WriteString("## 关键标签\n")
		b.WriteString(formatTagCounts(tagCounts))
		b.WriteString("\n\n")
	}

	// 5. Platform participation section.
	sourceCounts := make(map[string]int)
	for _, l := range logs {
		sourceCounts[l.Source]++
	}
	if len(sourceCounts) > 0 {
		b.WriteString("## 参与平台\n")
		b.WriteString(formatSourceCounts(sourceCounts))
		b.WriteString("\n")
	}

	return b.String(), nil
}

// AutoUpdateContext checks if a project's context is stale and regenerates it.
// A context is considered stale if there are more than 5 new logs since the last update.
func (s *SummaryService) AutoUpdateContext(ctx context.Context, projectID uuid.UUID) error {
	// Get project.
	var p models.Project
	err := s.DB.QueryRow(ctx,
		`SELECT id, user_id, name, status, context_md, metadata, created_at, updated_at
		 FROM projects WHERE id = $1`, projectID).
		Scan(&p.ID, &p.UserID, &p.Name, &p.Status, &p.ContextMD,
			&p.Metadata, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("summary.AutoUpdateContext: fetch project: %w", err)
	}

	// Count logs since last context update.
	var logCount int
	err = s.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM project_logs WHERE project_id = $1 AND created_at > $2`,
		projectID, p.UpdatedAt).Scan(&logCount)
	if err != nil {
		return fmt.Errorf("summary.AutoUpdateContext: count logs: %w", err)
	}

	if logCount <= 5 {
		return nil // not stale
	}

	// Regenerate.
	md, err := s.GenerateProjectSummary(ctx, projectID)
	if err != nil {
		return fmt.Errorf("summary.AutoUpdateContext: generate: %w", err)
	}

	return s.Project.UpdateContext(ctx, p.UserID, p.Name, md)
}

// SummarizeAllStaleProjects finds projects with stale contexts and updates them.
// Returns the number of projects updated.
func (s *SummaryService) SummarizeAllStaleProjects(ctx context.Context, userID uuid.UUID) (int, error) {
	projects, err := s.Project.List(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("summary.SummarizeAllStaleProjects: list: %w", err)
	}

	updated := 0
	for _, p := range projects {
		// Count logs since last update.
		var logCount int
		err := s.DB.QueryRow(ctx,
			`SELECT COUNT(*) FROM project_logs WHERE project_id = $1 AND created_at > $2`,
			p.ID, p.UpdatedAt).Scan(&logCount)
		if err != nil {
			continue
		}
		if logCount <= 5 {
			continue
		}

		md, err := s.GenerateProjectSummary(ctx, p.ID)
		if err != nil {
			continue
		}

		if err := s.Project.UpdateContext(ctx, p.UserID, p.Name, md); err != nil {
			continue
		}
		updated++
	}

	return updated, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type weekGroup struct {
	dateLabel string
	weekLabel string
	logs      []models.ProjectLog
}

// groupLogsByWeek groups logs into week-based buckets relative to now.
func groupLogsByWeek(logs []models.ProjectLog, now time.Time) []weekGroup {
	// Compute the start of the current week (Monday).
	weekday := now.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	thisWeekStart := now.AddDate(0, 0, -int(weekday-time.Monday))
	thisWeekStart = time.Date(thisWeekStart.Year(), thisWeekStart.Month(), thisWeekStart.Day(), 0, 0, 0, 0, time.UTC)
	lastWeekStart := thisWeekStart.AddDate(0, 0, -7)

	type bucketKey struct {
		year int
		week int
	}

	bucketOrder := []bucketKey{}
	buckets := map[bucketKey]*weekGroup{}

	for _, l := range logs {
		t := l.CreatedAt.UTC()
		dateStr := t.Format("2006-01-02")

		var label string
		if !t.Before(thisWeekStart) {
			label = "本周"
		} else if !t.Before(lastWeekStart) {
			label = "上周"
		} else {
			weeksAgo := int(thisWeekStart.Sub(t).Hours()/168) + 1
			label = fmt.Sprintf("%d周前", weeksAgo)
		}

		y, w := t.ISOWeek()
		key := bucketKey{y, w}

		if _, ok := buckets[key]; !ok {
			buckets[key] = &weekGroup{
				dateLabel: dateStr,
				weekLabel: label,
			}
			bucketOrder = append(bucketOrder, key)
		}
		buckets[key].logs = append(buckets[key].logs, l)
		// Update dateLabel to the latest date in the group.
		if dateStr > buckets[key].dateLabel {
			buckets[key].dateLabel = dateStr
		}
	}

	result := make([]weekGroup, 0, len(bucketOrder))
	for _, k := range bucketOrder {
		result = append(result, *buckets[k])
	}
	return result
}

// formatTagCounts formats tag counts as "tag1(N), tag2(M), ..." sorted by count descending.
func formatTagCounts(counts map[string]int) string {
	type tagCount struct {
		tag   string
		count int
	}
	pairs := make([]tagCount, 0, len(counts))
	for t, c := range counts {
		pairs = append(pairs, tagCount{t, c})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].tag < pairs[j].tag
	})
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = fmt.Sprintf("%s(%d)", p.tag, p.count)
	}
	return strings.Join(parts, ", ")
}

// formatSourceCounts formats source counts as "Source1: N次, Source2: M次" sorted by count descending.
func formatSourceCounts(counts map[string]int) string {
	type srcCount struct {
		source string
		count  int
	}
	pairs := make([]srcCount, 0, len(counts))
	for src, c := range counts {
		pairs = append(pairs, srcCount{src, c})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].source < pairs[j].source
	})
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = fmt.Sprintf("%s: %d次", p.source, p.count)
	}
	return strings.Join(parts, ", ")
}
