package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
)

type AgentExportPayload struct {
	Platform     string               `json:"platform,omitempty"`
	Command      string               `json:"command,omitempty"`
	ProfileRules []AgentProfileRule   `json:"profile_rules,omitempty"`
	MemoryItems  []AgentMemoryItem    `json:"memory_items,omitempty"`
	Projects     []AgentProjectRecord `json:"projects,omitempty"`
	Automations  []AgentRecord        `json:"automations,omitempty"`
	Tools        []AgentRecord        `json:"tools,omitempty"`
	Connections  []AgentRecord        `json:"connections,omitempty"`
	Archives     []AgentRecord        `json:"archives,omitempty"`
	Unsupported  []AgentRecord        `json:"unsupported,omitempty"`
	Claude       *ClaudeInventory     `json:"claude,omitempty"`
	Notes        []string             `json:"notes,omitempty"`
}

type AgentProfileRule struct {
	Title       string   `json:"title,omitempty"`
	Content     string   `json:"content,omitempty"`
	Exactness   string   `json:"exactness,omitempty"`
	SourcePaths []string `json:"source_paths,omitempty"`
	Confidence  float64  `json:"confidence,omitempty"`
}

type AgentMemoryItem struct {
	Title       string   `json:"title,omitempty"`
	Content     string   `json:"content,omitempty"`
	Exactness   string   `json:"exactness,omitempty"`
	SourcePaths []string `json:"source_paths,omitempty"`
	Confidence  float64  `json:"confidence,omitempty"`
}

type AgentProjectRecord struct {
	Name        string   `json:"name,omitempty"`
	Context     string   `json:"context,omitempty"`
	Exactness   string   `json:"exactness,omitempty"`
	SourcePaths []string `json:"source_paths,omitempty"`
}

type AgentRecord struct {
	Name        string                 `json:"name,omitempty"`
	Content     string                 `json:"content,omitempty"`
	Exactness   string                 `json:"exactness,omitempty"`
	SourcePaths []string               `json:"source_paths,omitempty"`
	Confidence  float64                `json:"confidence,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type ClaudeInventory struct {
	Projects          []ClaudeProjectSnapshot `json:"projects,omitempty"`
	Bundles           []ClaudeBundle          `json:"bundles,omitempty"`
	Conversations     []ClaudeConversation    `json:"conversations,omitempty"`
	Files             []ClaudeFileRecord      `json:"files,omitempty"`
	SensitiveFindings []AgentSensitiveFinding `json:"sensitive_findings,omitempty"`
	VaultCandidates   []AgentVaultCandidate   `json:"vault_candidates,omitempty"`
}

type ClaudeProjectSnapshot struct {
	Name        string             `json:"name,omitempty"`
	Context     string             `json:"context,omitempty"`
	Exactness   string             `json:"exactness,omitempty"`
	SourcePaths []string           `json:"source_paths,omitempty"`
	Files       []ClaudeFileRecord `json:"files,omitempty"`
}

type ClaudeBundle struct {
	Name        string             `json:"name,omitempty"`
	Kind        string             `json:"kind,omitempty"`
	Description string             `json:"description,omitempty"`
	Exactness   string             `json:"exactness,omitempty"`
	SourcePaths []string           `json:"source_paths,omitempty"`
	Files       []ClaudeFileRecord `json:"files,omitempty"`
}

type ClaudeConversation struct {
	Name        string                      `json:"name,omitempty"`
	SessionID   string                      `json:"session_id,omitempty"`
	ProjectName string                      `json:"project_name,omitempty"`
	Summary     string                      `json:"summary,omitempty"`
	StartedAt   string                      `json:"started_at,omitempty"`
	Exactness   string                      `json:"exactness,omitempty"`
	SourcePaths []string                    `json:"source_paths,omitempty"`
	Messages    []ClaudeConversationMessage `json:"messages,omitempty"`
}

type ClaudeConversationMessage struct {
	ID        string           `json:"id,omitempty"`
	ParentID  string           `json:"parent_id,omitempty"`
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	Timestamp string           `json:"timestamp,omitempty"`
	Kind      string           `json:"kind,omitempty"`
	Parts     []NormalizedPart `json:"parts,omitempty"`
}

type ClaudeFileRecord struct {
	Path          string   `json:"path,omitempty"`
	Content       string   `json:"content,omitempty"`
	ContentBase64 string   `json:"content_base64,omitempty"`
	ContentType   string   `json:"content_type,omitempty"`
	Exactness     string   `json:"exactness,omitempty"`
	SourcePath    string   `json:"source_path,omitempty"`
	SourcePaths   []string `json:"source_paths,omitempty"`
}

type AgentSensitiveFinding struct {
	Title           string   `json:"title,omitempty"`
	Detail          string   `json:"detail,omitempty"`
	Severity        string   `json:"severity,omitempty"`
	SourcePaths     []string `json:"source_paths,omitempty"`
	RedactedExample string   `json:"redacted_example,omitempty"`
}

type AgentVaultCandidate struct {
	Scope       string   `json:"scope,omitempty"`
	Description string   `json:"description,omitempty"`
	SourcePaths []string `json:"source_paths,omitempty"`
}

type NormalizedConversation struct {
	Version              string                 `json:"version,omitempty"`
	SourcePlatform       string                 `json:"source_platform,omitempty"`
	SourceURL            string                 `json:"source_url,omitempty"`
	SourceConversationID string                 `json:"source_conversation_id,omitempty"`
	Title                string                 `json:"title,omitempty"`
	ImportedAt           string                 `json:"imported_at,omitempty"`
	ImportStrategy       string                 `json:"import_strategy,omitempty"`
	Model                string                 `json:"model,omitempty"`
	CreatedAt            string                 `json:"created_at,omitempty"`
	UpdatedAt            string                 `json:"updated_at,omitempty"`
	ProjectName          string                 `json:"project_name,omitempty"`
	Exactness            string                 `json:"exactness,omitempty"`
	SourcePaths          []string               `json:"source_paths,omitempty"`
	Provenance           map[string]interface{} `json:"provenance,omitempty"`
	Turns                []NormalizedTurn       `json:"turns,omitempty"`
	TurnCount            int                    `json:"turn_count,omitempty"`
}

type NormalizedTurn struct {
	ID                    string           `json:"id,omitempty"`
	Role                  string           `json:"role,omitempty"`
	At                    string           `json:"at,omitempty"`
	SourceMessageID       string           `json:"source_message_id,omitempty"`
	ParentSourceMessageID string           `json:"parent_source_message_id,omitempty"`
	SourceMessageKind     string           `json:"source_message_kind,omitempty"`
	Parts                 []NormalizedPart `json:"parts,omitempty"`
}

type NormalizedPart struct {
	Type          string `json:"type,omitempty"`
	Text          string `json:"text,omitempty"`
	Name          string `json:"name,omitempty"`
	ArgsText      string `json:"args_text,omitempty"`
	ArgsTruncated bool   `json:"args_truncated,omitempty"`
	Truncated     bool   `json:"truncated,omitempty"`
	FileName      string `json:"file_name,omitempty"`
	MimeType      string `json:"mime_type,omitempty"`
}

type AgentImportResult struct {
	Platform          string   `json:"platform"`
	ProfileCategories int      `json:"profile_categories"`
	MemoryItems       int      `json:"memory_items"`
	Projects          int      `json:"projects"`
	ProjectFiles      int      `json:"project_files"`
	Bundles           int      `json:"bundles"`
	Conversations     int      `json:"conversations"`
	Artifacts         int      `json:"artifacts"`
	Imported          int      `json:"imported"`
	Archived          int      `json:"archived"`
	Blocked           int      `json:"blocked"`
	SensitiveFindings int      `json:"sensitive_findings"`
	VaultCandidates   int      `json:"vault_candidates"`
	Paths             []string `json:"paths"`
}

func (c *Client) ImportAgentExport(ctx context.Context, platform string, payload AgentExportPayload) (*AgentImportResult, error) {
	result := &AgentImportResult{Platform: platform}
	source := "agent:" + platform

	if content := renderProfileRules(platform, payload.ProfileRules); strings.TrimSpace(content) != "" {
		category := platform + "-agent"
		if err := c.store.UpsertProfile(ctx, c.userID, category, content, source); err != nil {
			return nil, err
		}
		result.ProfileCategories++
		result.Imported++
		result.Paths = append(result.Paths, hubpath.ProfilePath(category))
	}

	for _, item := range payload.MemoryItems {
		if strings.TrimSpace(item.Content) == "" {
			continue
		}
		expiresAt := time.Now().UTC().AddDate(1, 0, 0)
		entry, err := c.store.ImportScratch(ctx, c.userID, renderMemoryItem(item), source, item.Title, time.Now().UTC(), &expiresAt)
		if err != nil {
			return nil, err
		}
		result.MemoryItems++
		result.Imported++
		result.Paths = append(result.Paths, entry.Path)
	}

	for _, project := range payload.Projects {
		name := strings.TrimSpace(project.Name)
		if name == "" || strings.TrimSpace(project.Context) == "" {
			continue
		}
		if _, err := c.store.GetProject(ctx, c.userID, name); err != nil {
			if _, err := c.store.CreateProject(ctx, c.userID, name); err != nil {
				return nil, err
			}
		}
		if _, err := c.store.WriteEntry(ctx, c.userID, hubpath.ProjectContextPath(name), renderProjectContext(project), "text/markdown", models.FileTreeWriteOptions{
			Kind:          "project_context",
			MinTrustLevel: models.TrustLevelCollaborate,
			Metadata: map[string]interface{}{
				"source_platform": platform,
				"capture_mode":    "agent",
				"exactness":       project.Exactness,
				"source_paths":    project.SourcePaths,
			},
		}); err != nil {
			return nil, err
		}
		result.Projects++
		result.Imported++
		result.Paths = append(result.Paths, hubpath.ProjectContextPath(name))
	}

	if payload.Claude != nil {
		if err := c.importClaudeInventory(ctx, platform, *payload.Claude, result); err != nil {
			return nil, err
		}
	}

	if written, err := c.writeAgentArtifact(ctx, platform, "automations.json", payload.Automations); err != nil {
		return nil, err
	} else if written != "" {
		result.Artifacts++
		result.Archived += len(payload.Automations)
		result.Paths = append(result.Paths, written)
	}
	if written, err := c.writeAgentArtifact(ctx, platform, "tools.json", payload.Tools); err != nil {
		return nil, err
	} else if written != "" {
		result.Artifacts++
		result.Archived += len(payload.Tools)
		result.Paths = append(result.Paths, written)
	}
	if written, err := c.writeAgentArtifact(ctx, platform, "connections.json", payload.Connections); err != nil {
		return nil, err
	} else if written != "" {
		result.Artifacts++
		result.Archived += len(payload.Connections)
		result.Paths = append(result.Paths, written)
	}
	if written, err := c.writeAgentArtifact(ctx, platform, "archives.json", payload.Archives); err != nil {
		return nil, err
	} else if written != "" {
		result.Artifacts++
		result.Archived += len(payload.Archives)
		result.Paths = append(result.Paths, written)
	}
	if written, err := c.writeAgentArtifact(ctx, platform, "unsupported.json", payload.Unsupported); err != nil {
		return nil, err
	} else if written != "" {
		result.Artifacts++
		result.Archived += len(payload.Unsupported)
		result.Paths = append(result.Paths, written)
	}
	if content := renderNotes(payload.Notes); strings.TrimSpace(content) != "" {
		target := filepath.ToSlash(filepath.Join("/platforms", platform, "agent", "notes.md"))
		if _, err := c.store.WriteEntry(ctx, c.userID, target, content, "text/markdown", models.FileTreeWriteOptions{
			Kind:          "file",
			MinTrustLevel: models.TrustLevelWork,
			Metadata: map[string]interface{}{
				"source_platform": platform,
				"capture_mode":    "agent",
				"exactness":       "reference",
			},
		}); err != nil {
			return nil, err
		}
		result.Artifacts++
		result.Archived++
		result.Paths = append(result.Paths, target)
	}

	sort.Strings(result.Paths)
	return result, nil
}

func (c *Client) writeAgentArtifact(ctx context.Context, platform, filename string, payload any) (string, error) {
	if isEmptyPayload(payload) {
		return "", nil
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	target := filepath.ToSlash(filepath.Join("/platforms", platform, "agent", filename))
	if _, err := c.store.WriteEntry(ctx, c.userID, target, string(data)+"\n", "application/json", models.FileTreeWriteOptions{
		Kind:          "file",
		MinTrustLevel: models.TrustLevelWork,
		Metadata: map[string]interface{}{
			"source_platform": platform,
			"capture_mode":    "agent",
			"exactness":       "reference",
		},
	}); err != nil {
		return "", err
	}
	return target, nil
}

func isEmptyPayload(payload any) bool {
	switch typed := payload.(type) {
	case []AgentRecord:
		return len(typed) == 0
	default:
		return payload == nil
	}
}

func renderProfileRules(platform string, rules []AgentProfileRule) string {
	if len(rules) == 0 {
		return ""
	}
	lines := []string{
		fmt.Sprintf("# %s agent-derived profile rules", strings.Title(platform)),
		"",
	}
	for _, rule := range rules {
		content := strings.TrimSpace(rule.Content)
		if content == "" {
			continue
		}
		title := strings.TrimSpace(rule.Title)
		if title == "" {
			title = "Rule"
		}
		lines = append(lines, "## "+title, "")
		lines = append(lines, content, "")
		lines = append(lines, fmt.Sprintf("- Exactness: %s", fallbackExactness(rule.Exactness)))
		if len(rule.SourcePaths) > 0 {
			lines = append(lines, "- Source paths:")
			for _, source := range rule.SourcePaths {
				lines = append(lines, "  - "+source)
			}
		}
		if rule.Confidence > 0 {
			lines = append(lines, fmt.Sprintf("- Confidence: %.2f", rule.Confidence))
		}
		lines = append(lines, "")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderMemoryItem(item AgentMemoryItem) string {
	lines := []string{}
	if title := strings.TrimSpace(item.Title); title != "" {
		lines = append(lines, "# "+title, "")
	}
	lines = append(lines, strings.TrimSpace(item.Content), "")
	lines = append(lines, fmt.Sprintf("- Exactness: %s", fallbackExactness(item.Exactness)))
	if len(item.SourcePaths) > 0 {
		lines = append(lines, "- Source paths:")
		for _, source := range item.SourcePaths {
			lines = append(lines, "  - "+source)
		}
	}
	if item.Confidence > 0 {
		lines = append(lines, fmt.Sprintf("- Confidence: %.2f", item.Confidence))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderProjectContext(project AgentProjectRecord) string {
	lines := []string{strings.TrimSpace(project.Context), ""}
	lines = append(lines, fmt.Sprintf("- Exactness: %s", fallbackExactness(project.Exactness)))
	if len(project.SourcePaths) > 0 {
		lines = append(lines, "- Source paths:")
		for _, source := range project.SourcePaths {
			lines = append(lines, "  - "+source)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderNotes(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	lines := []string{"# Agent-derived notes", ""}
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if note == "" {
			continue
		}
		lines = append(lines, "- "+note)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func fallbackExactness(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "derived"
	}
	return value
}
