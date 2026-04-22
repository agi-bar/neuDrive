package platforms

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/runtimecfg"
	"github.com/agi-bar/neudrive/internal/skillsarchive"
	"github.com/agi-bar/neudrive/internal/storage/sqlite"
)

const claudeBinaryInlineMaxBytes = 64 << 10

type ImportPreview struct {
	Platform          string                         `json:"platform"`
	DisplayName       string                         `json:"display_name"`
	Mode              ImportMode                     `json:"mode"`
	StartedAt         string                         `json:"started_at,omitempty"`
	CompletedAt       string                         `json:"completed_at,omitempty"`
	DurationMs        int64                          `json:"duration_ms,omitempty"`
	Categories        []ImportPreviewCategory        `json:"categories"`
	SensitiveFindings []sqlite.AgentSensitiveFinding `json:"sensitive_findings"`
	VaultCandidates   []sqlite.AgentVaultCandidate   `json:"vault_candidates"`
	Notes             []string                       `json:"notes"`
	NextCommand       string                         `json:"next_command"`
}

type ImportPreviewCategory struct {
	Name       string `json:"name"`
	Discovered int    `json:"discovered"`
	Importable int    `json:"importable"`
	Archived   int    `json:"archived"`
	Blocked    int    `json:"blocked"`
}

type claudeLocalScanResult struct {
	ProfileRules []sqlite.AgentProfileRule
	MemoryItems  []sqlite.AgentMemoryItem
	Automations  []sqlite.AgentRecord
	Tools        []sqlite.AgentRecord
	Connections  []sqlite.AgentRecord
	Inventory    sqlite.ClaudeInventory
	Notes        []string
}

func PreviewImport(ctx context.Context, cfg *runtimecfg.CLIConfig, platform, rawMode string) (*ImportPreview, error) {
	_ = ctx
	startedAt := time.Now().UTC()
	adapter, err := Resolve(platform)
	if err != nil {
		return nil, err
	}
	mode, err := ParseImportMode(adapter.ID(), rawMode)
	if err != nil {
		return nil, err
	}

	preview := &ImportPreview{
		Platform:    adapter.ID(),
		DisplayName: adapter.DisplayName(),
		Mode:        mode,
		NextCommand: suggestedImportCommand(adapter.ID(), mode),
	}

	sources := adapter.DiscoverSources()
	var payload sqlite.AgentExportPayload
	if adapter.ID() == "claude-code" {
		scan, err := scanLocalClaudeMigration()
		if err != nil {
			return nil, err
		}
		payload = mergeClaudeScanIntoPayload(payload, scan)
		preview.Notes = append(preview.Notes, scan.Notes...)
	} else if adapter.ID() == "codex" {
		scan, err := scanLocalCodexMigration()
		if err != nil {
			return nil, err
		}
		payload = mergeCodexScanIntoPayload(payload, scan)
		preview.Notes = append(preview.Notes, scan.Notes...)
		preview.Notes = append(preview.Notes, "Codex preview uses neuDrive's deterministic local inventory mapping; live agent semantic scan is skipped by default.")
	}
	preview.Categories = buildImportPreviewCategories(mode, sources, payload)
	preview.SensitiveFindings = append(preview.SensitiveFindings, payload.SensitiveFindings...)
	preview.VaultCandidates = append(preview.VaultCandidates, payload.VaultCandidates...)
	if payload.Claude != nil {
		preview.SensitiveFindings = append(preview.SensitiveFindings, payload.Claude.SensitiveFindings...)
		preview.VaultCandidates = append(preview.VaultCandidates, payload.Claude.VaultCandidates...)
	}
	completedAt := time.Now().UTC()
	preview.StartedAt = startedAt.Format(time.RFC3339)
	preview.CompletedAt = completedAt.Format(time.RFC3339)
	preview.DurationMs = completedAt.Sub(startedAt).Milliseconds()
	return preview, nil
}

func enrichClaudePayload(payload sqlite.AgentExportPayload) (sqlite.AgentExportPayload, []string, error) {
	scan, err := scanLocalClaudeMigration()
	if err != nil {
		return payload, nil, err
	}
	return mergeClaudeScanIntoPayload(payload, scan), scan.Notes, nil
}

func mergeClaudeScanIntoPayload(payload sqlite.AgentExportPayload, scan *claudeLocalScanResult) sqlite.AgentExportPayload {
	if scan == nil {
		return payload
	}
	payload.ProfileRules = appendUniqueProfileRules(payload.ProfileRules, scan.ProfileRules)
	payload.MemoryItems = appendUniqueMemoryItems(payload.MemoryItems, scan.MemoryItems)
	payload.Automations = appendUniqueAgentRecords(payload.Automations, scan.Automations)
	payload.Tools = appendUniqueAgentRecords(payload.Tools, scan.Tools)
	payload.Connections = appendUniqueAgentRecords(payload.Connections, scan.Connections)
	if payload.Claude == nil {
		payload.Claude = &sqlite.ClaudeInventory{}
	}
	payload.Claude = mergeClaudeInventory(payload.Claude, &scan.Inventory)
	return payload
}

func mergeAgentPayload(base, extra sqlite.AgentExportPayload) sqlite.AgentExportPayload {
	base.ProfileRules = appendUniqueProfileRules(base.ProfileRules, extra.ProfileRules)
	base.MemoryItems = appendUniqueMemoryItems(base.MemoryItems, extra.MemoryItems)
	base.Projects = appendUniqueProjects(base.Projects, extra.Projects)
	base.Automations = appendUniqueAgentRecords(base.Automations, extra.Automations)
	base.Tools = appendUniqueAgentRecords(base.Tools, extra.Tools)
	base.Connections = appendUniqueAgentRecords(base.Connections, extra.Connections)
	base.Archives = appendUniqueAgentRecords(base.Archives, extra.Archives)
	base.Unsupported = appendUniqueAgentRecords(base.Unsupported, extra.Unsupported)
	base.SensitiveFindings = appendUniqueSensitiveFindings(base.SensitiveFindings, extra.SensitiveFindings)
	base.VaultCandidates = appendUniqueVaultCandidates(base.VaultCandidates, extra.VaultCandidates)
	base.Notes = appendUniqueStrings(base.Notes, extra.Notes)
	if extra.Claude != nil {
		if base.Claude == nil {
			base.Claude = &sqlite.ClaudeInventory{}
		}
		base.Claude = mergeClaudeInventory(base.Claude, extra.Claude)
	}
	if extra.Codex != nil {
		if base.Codex == nil {
			base.Codex = &sqlite.CodexInventory{}
		}
		base.Codex = mergeCodexInventory(base.Codex, extra.Codex)
	}
	if strings.TrimSpace(base.Platform) == "" {
		base.Platform = extra.Platform
	}
	if strings.TrimSpace(base.Command) == "" {
		base.Command = extra.Command
	}
	return base
}

func mergeClaudeInventory(base, extra *sqlite.ClaudeInventory) *sqlite.ClaudeInventory {
	if base == nil && extra == nil {
		return nil
	}
	if base == nil {
		copyValue := *extra
		return &copyValue
	}
	if extra == nil {
		return base
	}
	base.Projects = append(base.Projects, extra.Projects...)
	base.Bundles = append(base.Bundles, extra.Bundles...)
	base.Conversations = append(base.Conversations, extra.Conversations...)
	base.Files = append(base.Files, extra.Files...)
	base.SensitiveFindings = append(base.SensitiveFindings, extra.SensitiveFindings...)
	base.VaultCandidates = append(base.VaultCandidates, extra.VaultCandidates...)
	return base
}

func mergeCodexInventory(base, extra *sqlite.CodexInventory) *sqlite.CodexInventory {
	if base == nil && extra == nil {
		return nil
	}
	if base == nil {
		copyValue := *extra
		return &copyValue
	}
	if extra == nil {
		return base
	}
	base.Bundles = append(base.Bundles, extra.Bundles...)
	base.Conversations = append(base.Conversations, extra.Conversations...)
	return base
}

func buildImportPreviewCategories(mode ImportMode, sources []Source, payload sqlite.AgentExportPayload) []ImportPreviewCategory {
	categories := []ImportPreviewCategory{}
	if mode == ImportModeFiles || mode == ImportModeAll {
		categories = append(categories, ImportPreviewCategory{
			Name:       "raw_platform_snapshot",
			Discovered: countSourceFiles(sources),
			Archived:   countSourceFiles(sources),
		})
	}
	if len(payload.ProfileRules) > 0 {
		categories = append(categories, ImportPreviewCategory{Name: "profile_rules", Importable: len(payload.ProfileRules), Discovered: len(payload.ProfileRules)})
	}
	if len(payload.MemoryItems) > 0 {
		categories = append(categories, ImportPreviewCategory{Name: "memory_items", Importable: len(payload.MemoryItems), Discovered: len(payload.MemoryItems)})
	}
	if len(payload.Projects) > 0 {
		categories = append(categories, ImportPreviewCategory{Name: "projects", Importable: len(payload.Projects), Discovered: len(payload.Projects)})
	}
	if payload.Claude != nil {
		if len(payload.Claude.Projects) > 0 {
			categories = append(categories, ImportPreviewCategory{Name: "claude_projects", Importable: len(payload.Claude.Projects), Discovered: len(payload.Claude.Projects)})
		}
		if len(payload.Claude.Bundles) > 0 {
			categories = append(categories, ImportPreviewCategory{Name: "bundles", Importable: len(payload.Claude.Bundles), Discovered: len(payload.Claude.Bundles)})
		}
		if len(payload.Claude.Conversations) > 0 {
			categories = append(categories, ImportPreviewCategory{Name: "conversations", Importable: len(payload.Claude.Conversations), Discovered: len(payload.Claude.Conversations)})
		}
		if len(payload.Claude.Files) > 0 {
			categories = append(categories, ImportPreviewCategory{Name: "structured_archives", Archived: len(payload.Claude.Files), Discovered: len(payload.Claude.Files)})
		}
	}
	if payload.Codex != nil {
		if len(payload.Codex.Bundles) > 0 {
			categories = append(categories, ImportPreviewCategory{Name: "bundles", Importable: len(payload.Codex.Bundles), Discovered: len(payload.Codex.Bundles)})
		}
		if len(payload.Codex.Conversations) > 0 {
			categories = append(categories, ImportPreviewCategory{Name: "conversations", Importable: len(payload.Codex.Conversations), Discovered: len(payload.Codex.Conversations)})
		}
	}
	archived := len(payload.Automations) + len(payload.Tools) + len(payload.Connections) + len(payload.Archives)
	blocked := len(payload.Unsupported)
	if archived > 0 || blocked > 0 {
		categories = append(categories, ImportPreviewCategory{
			Name:       "agent_artifacts",
			Discovered: archived + blocked,
			Archived:   archived,
			Blocked:    blocked,
		})
	}
	return categories
}

func countSourceFiles(sources []Source) int {
	total := 0
	for _, source := range sources {
		info, err := os.Stat(source.Path)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			total++
			continue
		}
		_ = filepath.Walk(source.Path, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if info.IsDir() {
				if path != source.Path && isManagedNeuDriveDir(path) {
					return filepath.SkipDir
				}
				return nil
			}
			total++
			return nil
		})
	}
	return total
}

func suggestedImportCommand(platform string, mode ImportMode) string {
	name := platform
	if platform == "claude-code" {
		name = "claude"
	}
	switch mode {
	case ImportModeAll:
		return fmt.Sprintf("neudrive import %s --raw", name)
	default:
		return fmt.Sprintf("neudrive import %s", name)
	}
}

func scanLocalClaudeMigration() (*claudeLocalScanResult, error) {
	result := &claudeLocalScanResult{Inventory: sqlite.ClaudeInventory{}}

	for _, rule := range []struct {
		path  string
		title string
	}{
		{path: expandUser("~/.claude/CLAUDE.md"), title: "Global CLAUDE.md"},
		{path: expandUser("~/.claude/CLAUDE.local.md"), title: "Global CLAUDE.local.md"},
	} {
		if content, ok, err := readTextFile(rule.path); err != nil {
			return nil, err
		} else if ok && strings.TrimSpace(content) != "" {
			result.ProfileRules = append(result.ProfileRules, sqlite.AgentProfileRule{
				Title:       rule.title,
				Content:     strings.TrimSpace(content),
				Exactness:   "exact",
				SourcePaths: []string{rule.path},
				Confidence:  1,
			})
		}
	}

	if err := scanClaudeMemoryTree(result, expandUser("~/.claude/agent-memory"), "agent-memory"); err != nil {
		return nil, err
	}
	if err := scanClaudeMemoryTree(result, expandUser("~/.claude/memory"), "memory"); err != nil {
		return nil, err
	}
	if err := scanClaudeProjectMemory(result, expandUser("~/.claude/projects")); err != nil {
		return nil, err
	}
	if err := scanClaudeConversations(&result.Inventory, expandUser("~/.claude/projects")); err != nil {
		return nil, err
	}
	if err := scanClaudeBundleDirectory(&result.Inventory, expandUser("~/.claude/skills"), "skill", nil, &result.Notes); err != nil {
		return nil, err
	}
	if err := scanClaudeMarkdownBundles(&result.Inventory, expandUser("~/.claude/agents"), "agent"); err != nil {
		return nil, err
	}
	if err := scanClaudeMarkdownBundles(&result.Inventory, expandUser("~/.claude/commands"), "command"); err != nil {
		return nil, err
	}
	if err := scanClaudeMarkdownBundles(&result.Inventory, expandUser("~/.claude/rules"), "rule"); err != nil {
		return nil, err
	}

	projectRoots := discoverClaudeProjectRoots(expandUser("~/.claude.json"))
	for _, root := range projectRoots {
		if err := scanClaudeProjectRoot(&result.Inventory, root, &result.Notes); err != nil {
			return nil, err
		}
		if err := scanClaudeProjectConnections(result, root); err != nil {
			return nil, err
		}
	}
	if err := scanClaudeConnectionManifests(result, expandUser("~/.claude.json"), expandUser("~/.claude/settings.json"), expandUser("~/.claude/settings.local.json")); err != nil {
		return nil, err
	}
	if err := scanClaudeSensitiveFileFindings(&result.Inventory, expandUser("~/.claude/settings.local.json")); err != nil {
		return nil, err
	}
	if err := scanClaudeSensitiveFileFindings(&result.Inventory, expandUser("~/.claude/.credentials.json")); err != nil {
		return nil, err
	}
	if err := scanClaudeScheduledTasks(result, expandUser("~/.claude/scheduled-tasks")); err != nil {
		return nil, err
	}
	if err := scanClaudePlugins(result, expandUser("~/.claude/plugins")); err != nil {
		return nil, err
	}
	if err := scanClaudeOutputStyles(result, expandUser("~/.claude/output-styles")); err != nil {
		return nil, err
	}

	if err := archiveClaudeRuntimeFile(&result.Inventory, expandUser("~/.claude/history.jsonl"), "agent/runtime/history.jsonl"); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeTree(&result.Inventory, expandUser("~/.claude/agent-memory"), "agent/runtime/agent-memory", &result.Notes); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeTree(&result.Inventory, expandUser("~/.claude/scheduled-tasks"), "agent/runtime/scheduled-tasks", &result.Notes); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeTree(&result.Inventory, expandUser("~/.claude/output-styles"), "agent/runtime/output-styles", &result.Notes); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeTree(&result.Inventory, expandUser("~/.claude/hooks"), "agent/runtime/hooks", &result.Notes); err != nil {
		return nil, err
	}

	return result, nil
}

func scanClaudeMemoryTree(result *claudeLocalScanResult, dir, prefix string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if ext != ".md" && ext != ".txt" {
			return nil
		}
		content, ok, err := readTextFile(path)
		if err != nil || !ok || strings.TrimSpace(content) == "" {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		result.MemoryItems = append(result.MemoryItems, sqlite.AgentMemoryItem{
			Title:       fmt.Sprintf("%s/%s", prefix, filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel)))),
			Content:     strings.TrimSpace(content),
			Exactness:   "exact",
			SourcePaths: []string{path},
			Confidence:  1,
		})
		return nil
	})
}

func scanClaudeProjectMemory(result *claudeLocalScanResult, projectsRoot string) error {
	info, err := os.Stat(projectsRoot)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(projectsRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(filepath.Dir(path)) != "memory" || !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}
		if strings.EqualFold(info.Name(), "MEMORY.md") {
			return nil
		}
		content, ok, err := readTextFile(path)
		if err != nil || !ok || strings.TrimSpace(content) == "" {
			return err
		}
		title := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
		project := filepath.Base(filepath.Dir(filepath.Dir(path)))
		result.MemoryItems = append(result.MemoryItems, sqlite.AgentMemoryItem{
			Title:       fmt.Sprintf("%s/%s", project, title),
			Content:     strings.TrimSpace(content),
			Exactness:   "exact",
			SourcePaths: []string{path},
			Confidence:  1,
		})
		return nil
	})
}

func scanClaudeConversations(inventory *sqlite.ClaudeInventory, projectsRoot string) error {
	info, err := os.Stat(projectsRoot)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(projectsRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".jsonl") {
			return nil
		}
		convo, ok, err := parseClaudeConversationFile(path)
		if err != nil {
			return err
		}
		if ok {
			inventory.Conversations = append(inventory.Conversations, convo)
		}
		return nil
	})
}

func parseClaudeConversationFile(path string) (sqlite.ClaudeConversation, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return sqlite.ClaudeConversation{}, false, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	messages := []sqlite.ClaudeConversationMessage{}
	firstTimestamp := ""
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return sqlite.ClaudeConversation{}, false, readErr
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			if errors.Is(readErr, io.EOF) {
				break
			}
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal(line, &entry); err != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			continue
		}
		message := extractClaudeConversationMessage(entry)
		if strings.TrimSpace(message.Content) == "" && len(message.Parts) == 0 {
			if errors.Is(readErr, io.EOF) {
				break
			}
			continue
		}
		if firstTimestamp == "" && strings.TrimSpace(message.Timestamp) != "" {
			firstTimestamp = strings.TrimSpace(message.Timestamp)
		}
		if title == strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) && message.Role == "user" {
			title = firstNonEmptyLine(preferredClaudeMessageText(message), title)
		}
		messages = append(messages, message)
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	if len(messages) == 0 {
		return sqlite.ClaudeConversation{}, false, nil
	}
	projectName := filepath.Base(filepath.Dir(path))
	if projectName == "subagents" {
		projectName = filepath.Base(filepath.Dir(filepath.Dir(path)))
	}
	summary := ""
	for _, message := range messages {
		if strings.EqualFold(message.Role, "assistant") && strings.TrimSpace(message.Content) != "" {
			summary = firstNonEmptyLine(preferredClaudeMessageText(message), "")
			break
		}
	}
	return sqlite.ClaudeConversation{
		Name:        title,
		SessionID:   strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		ProjectName: projectName,
		Summary:     summary,
		StartedAt:   firstTimestamp,
		Exactness:   "exact",
		SourcePaths: []string{path},
		Messages:    messages,
	}, true, nil
}

func extractClaudeConversationMessage(entry map[string]interface{}) sqlite.ClaudeConversationMessage {
	role := strings.TrimSpace(fmt.Sprint(entry["type"]))
	timestamp := strings.TrimSpace(fmt.Sprint(entry["timestamp"]))
	kind := strings.TrimSpace(fmt.Sprint(entry["type"]))
	id := firstNonEmptyString(entry["uuid"], entry["id"], entry["message_uuid"])
	parentID := firstNonEmptyString(entry["parent_uuid"], entry["parent_id"], entry["parent_message_uuid"])
	var parts []sqlite.NormalizedPart

	if message, ok := entry["message"].(map[string]interface{}); ok {
		if msgRole := strings.TrimSpace(fmt.Sprint(message["role"])); msgRole != "" {
			role = msgRole
		}
		if msgTimestamp := firstNonEmptyString(message["timestamp"], message["created_at"]); msgTimestamp != "" {
			timestamp = msgTimestamp
		}
		if msgKind := strings.TrimSpace(fmt.Sprint(message["type"])); msgKind != "" {
			kind = msgKind
		}
		if msgID := firstNonEmptyString(message["uuid"], message["id"], message["message_uuid"]); msgID != "" {
			id = msgID
		}
		if msgParentID := firstNonEmptyString(message["parent_uuid"], message["parent_id"], message["parent_message_uuid"]); msgParentID != "" {
			parentID = msgParentID
		}
		parts = extractClaudeContentParts(message["content"])
	}
	if len(parts) == 0 {
		parts = extractClaudeContentParts(entry["content"])
	}
	content := flattenClaudeParts(parts)
	if strings.TrimSpace(content) == "" {
		content = flattenClaudeContent(entry["content"])
	}
	if strings.TrimSpace(content) == "" {
		serialized, err := json.Marshal(entry)
		if err == nil {
			content = string(serialized)
		}
	}
	return sqlite.ClaudeConversationMessage{
		ID:        id,
		ParentID:  parentID,
		Role:      strings.TrimSpace(role),
		Content:   strings.TrimSpace(content),
		Timestamp: strings.TrimSpace(timestamp),
		Kind:      strings.TrimSpace(kind),
		Parts:     parts,
	}
}

func flattenClaudeContent(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			part := flattenClaudeContent(item)
			if strings.TrimSpace(part) != "" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, "\n\n")
	case map[string]interface{}:
		if text := strings.TrimSpace(fmt.Sprint(typed["text"])); text != "" && text != "<nil>" {
			return text
		}
		if content := flattenClaudeContent(typed["content"]); strings.TrimSpace(content) != "" {
			return content
		}
		serialized, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(serialized)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func extractClaudeContentParts(value interface{}) []sqlite.NormalizedPart {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		return []sqlite.NormalizedPart{{Type: "text", Text: text}}
	case []interface{}:
		parts := make([]sqlite.NormalizedPart, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, extractClaudeContentParts(item)...)
		}
		return parts
	case map[string]interface{}:
		part, ok := extractClaudeContentPart(typed)
		if !ok {
			return nil
		}
		return []sqlite.NormalizedPart{part}
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			return nil
		}
		return []sqlite.NormalizedPart{{Type: "text", Text: text}}
	}
}

func extractClaudeContentPart(block map[string]interface{}) (sqlite.NormalizedPart, bool) {
	rawType := strings.TrimSpace(strings.ToLower(fmt.Sprint(block["type"])))
	if thinking := strings.TrimSpace(fmt.Sprint(block["thinking"])); thinking != "" && thinking != "<nil>" {
		return sqlite.NormalizedPart{Type: "thinking", Text: thinking}, true
	}
	if rawType == "thinking" {
		text := strings.TrimSpace(flattenClaudeContent(block["content"]))
		if text == "" {
			text = strings.TrimSpace(fmt.Sprint(block["text"]))
		}
		if text != "" && text != "<nil>" {
			return sqlite.NormalizedPart{Type: "thinking", Text: text}, true
		}
	}
	if rawType == "tool_use" || strings.HasSuffix(rawType, "tool_use") {
		argsText, argsTruncated := previewClaudeStructuredData(block["input"], 1600)
		return sqlite.NormalizedPart{
			Type:          "tool_call",
			Name:          strings.TrimSpace(fmt.Sprint(block["name"])),
			ArgsText:      argsText,
			ArgsTruncated: argsTruncated,
		}, true
	}
	if rawType == "tool_result" || strings.HasSuffix(rawType, "tool_result") {
		text := strings.TrimSpace(flattenClaudeContent(block["content"]))
		truncated := false
		if text == "" {
			var preview string
			preview, truncated = previewClaudeStructuredData(block["content"], 2400)
			text = preview
		} else {
			text, truncated = truncateClaudeText(text, 2400)
		}
		return sqlite.NormalizedPart{
			Type:      "tool_result",
			Text:      text,
			Truncated: truncated,
		}, true
	}
	fileName := firstNonEmptyString(block["file_name"], block["filename"], block["name"])
	mimeType := firstNonEmptyString(block["mime_type"], block["mimeType"], block["content_type"])
	if fileName != "" || mimeType != "" || rawType == "attachment" || rawType == "file" || rawType == "image" {
		return sqlite.NormalizedPart{
			Type:     "attachment",
			FileName: fileName,
			MimeType: mimeType,
		}, true
	}
	if text := strings.TrimSpace(fmt.Sprint(block["text"])); text != "" && text != "<nil>" {
		text, truncated := truncateClaudeText(text, 32000)
		return sqlite.NormalizedPart{Type: "text", Text: text, Truncated: truncated}, true
	}
	if content := strings.TrimSpace(flattenClaudeContent(block["content"])); content != "" {
		content, truncated := truncateClaudeText(content, 32000)
		partType := rawType
		if partType == "" || partType == "text" || partType == "content" {
			partType = "text"
		}
		return sqlite.NormalizedPart{Type: partType, Text: content, Truncated: truncated}, true
	}
	preview, truncated := previewClaudeStructuredData(block, 1200)
	if preview == "" {
		return sqlite.NormalizedPart{}, false
	}
	partType := rawType
	if partType == "" {
		partType = "content"
	}
	return sqlite.NormalizedPart{Type: partType, Text: preview, Truncated: truncated}, true
}

func flattenClaudeParts(parts []sqlite.NormalizedPart) string {
	rendered := make([]string, 0, len(parts))
	for _, part := range parts {
		text := renderClaudePartText(part)
		if strings.TrimSpace(text) != "" {
			rendered = append(rendered, text)
		}
	}
	return strings.TrimSpace(strings.Join(rendered, "\n\n"))
}

func renderClaudePartText(part sqlite.NormalizedPart) string {
	switch strings.TrimSpace(part.Type) {
	case "", "text":
		return strings.TrimSpace(part.Text)
	case "thinking":
		return "[thinking]\n" + strings.TrimSpace(part.Text)
	case "tool_call":
		lines := []string{"[tool_call]"}
		if strings.TrimSpace(part.Name) != "" {
			lines = append(lines, "name: "+strings.TrimSpace(part.Name))
		}
		if strings.TrimSpace(part.ArgsText) != "" {
			lines = append(lines, strings.TrimSpace(part.ArgsText))
		}
		return strings.Join(lines, "\n")
	case "tool_result":
		lines := []string{"[tool_result]"}
		if strings.TrimSpace(part.Text) != "" {
			lines = append(lines, strings.TrimSpace(part.Text))
		}
		return strings.Join(lines, "\n")
	case "attachment":
		lines := []string{"[attachment]"}
		if strings.TrimSpace(part.FileName) != "" {
			lines = append(lines, "name: "+strings.TrimSpace(part.FileName))
		}
		if strings.TrimSpace(part.MimeType) != "" {
			lines = append(lines, "mime: "+strings.TrimSpace(part.MimeType))
		}
		return strings.Join(lines, "\n")
	default:
		if strings.TrimSpace(part.Text) == "" {
			return "[" + strings.TrimSpace(part.Type) + "]"
		}
		return "[" + strings.TrimSpace(part.Type) + "]\n" + strings.TrimSpace(part.Text)
	}
}

func previewClaudeStructuredData(value interface{}, limit int) (string, bool) {
	if value == nil {
		return "", false
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return truncateClaudeText(strings.TrimSpace(fmt.Sprint(value)), limit)
	}
	return truncateClaudeText(string(data), limit)
}

func truncateClaudeText(value string, limit int) (string, bool) {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value, false
	}
	suffix := fmt.Sprintf("\n[truncated %d chars]", len(value)-limit)
	if limit <= len(suffix)+8 {
		return value[:limit], true
	}
	return strings.TrimSpace(value[:limit-len(suffix)]) + suffix, true
}

func firstNonEmptyString(values ...interface{}) string {
	for _, value := range values {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func preferredClaudeMessageText(message sqlite.ClaudeConversationMessage) string {
	for _, part := range message.Parts {
		switch strings.TrimSpace(part.Type) {
		case "", "text", "tool_result":
			if strings.TrimSpace(part.Text) != "" {
				return strings.TrimSpace(part.Text)
			}
		}
	}
	return strings.TrimSpace(message.Content)
}

func scanClaudeConnectionManifests(result *claudeLocalScanResult, paths ...string) error {
	for _, candidate := range paths {
		if err := appendClaudeConnectionRecord(result, candidate, "", "global"); err != nil {
			return err
		}
	}
	return nil
}

func scanClaudeProjectConnections(result *claudeLocalScanResult, root string) error {
	projectName := normalizeClaudeName(filepath.Base(root), "claude-project")
	pairs := []struct {
		name string
		path string
	}{
		{name: projectName + "-mcp", path: filepath.Join(root, ".mcp.json")},
		{name: projectName + "-settings", path: filepath.Join(root, ".claude", "settings.json")},
		{name: projectName + "-settings-local", path: filepath.Join(root, ".claude", "settings.local.json")},
	}
	for _, pair := range pairs {
		if err := appendClaudeConnectionRecord(result, pair.path, pair.name, "project"); err != nil {
			return err
		}
	}
	if err := scanClaudeSensitiveFileFindings(&result.Inventory, filepath.Join(root, ".claude", "settings.local.json")); err != nil {
		return err
	}
	return nil
}

func appendClaudeConnectionRecord(result *claudeLocalScanResult, sourcePath, name, scope string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	recordName := strings.TrimSpace(name)
	if recordName == "" {
		recordName = strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	}
	mcpServers := extractCodexObjectKeys(payload["mcpServers"])
	topKeys := extractCodexObjectKeys(payload)
	lines := []string{"Observed Claude connection manifest."}
	if scope != "" {
		lines = append(lines, "- Scope: "+scope)
	}
	if len(mcpServers) > 0 {
		lines = append(lines, "- MCP servers: "+strings.Join(mcpServers, ", "))
	}
	if len(topKeys) > 0 {
		lines = append(lines, "- Top-level keys: "+strings.Join(topKeys, ", "))
	}
	result.Connections = append(result.Connections, sqlite.AgentRecord{
		Name:        recordName,
		Content:     strings.Join(lines, "\n"),
		Exactness:   "exact",
		SourcePaths: []string{sourcePath},
		Confidence:  1,
		Metadata: map[string]interface{}{
			"scope":          scope,
			"mcp_servers":    mcpServers,
			"top_level_keys": topKeys,
		},
	})
	return nil
}

func scanClaudeScheduledTasks(result *claudeLocalScanResult, dir string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		metadata, ok, err := readLooseStructuredMetadata(path)
		if err != nil || !ok {
			return err
		}
		lines := []string{"Observed Claude scheduled task metadata."}
		if metadata["name"] != "" {
			lines = append(lines, "- Name: "+metadata["name"])
		}
		if metadata["schedule"] != "" {
			lines = append(lines, "- Schedule: "+metadata["schedule"])
		}
		if metadata["status"] != "" {
			lines = append(lines, "- Status: "+metadata["status"])
		}
		result.Automations = append(result.Automations, sqlite.AgentRecord{
			Name:        firstNonEmptyString(metadata["name"], strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))),
			Content:     strings.Join(lines, "\n"),
			Exactness:   "exact",
			SourcePaths: []string{path},
			Confidence:  1,
			Metadata: map[string]interface{}{
				"kind":     firstNonEmptyString(metadata["kind"], "scheduled-task"),
				"name":     metadata["name"],
				"schedule": metadata["schedule"],
				"status":   metadata["status"],
			},
		})
		return nil
	})
}

func scanClaudePlugins(result *claudeLocalScanResult, dir string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	seen := map[string]struct{}{}
	return filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || strings.ToLower(filepath.Ext(info.Name())) != ".json" {
			return nil
		}
		records, err := readClaudePluginRecords(path)
		if err != nil {
			return err
		}
		for _, record := range records {
			key := record.Name + "|" + strings.Join(record.SourcePaths, ",")
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result.Tools = append(result.Tools, record)
		}
		return nil
	})
}

func scanClaudeOutputStyles(result *claudeLocalScanResult, dir string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if ext := strings.ToLower(filepath.Ext(info.Name())); ext != ".md" && ext != ".txt" {
			return nil
		}
		content, ok, err := readTextFile(path)
		if err != nil || !ok || strings.TrimSpace(content) == "" {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		result.ProfileRules = append(result.ProfileRules, sqlite.AgentProfileRule{
			Title:       "Output style: " + filepath.ToSlash(rel),
			Content:     strings.TrimSpace(content),
			Exactness:   "exact",
			SourcePaths: []string{path},
			Confidence:  1,
		})
		return nil
	})
}

func scanClaudeSensitiveFileFindings(inventory *sqlite.ClaudeInventory, path string) error {
	content, ok, err := readTextFile(path)
	if err != nil || !ok || strings.TrimSpace(content) == "" {
		return err
	}
	_, findings, candidates := redactSensitiveText(path, content)
	inventory.SensitiveFindings = append(inventory.SensitiveFindings, findings...)
	inventory.VaultCandidates = append(inventory.VaultCandidates, candidates...)
	return nil
}

func readLooseStructuredMetadata(path string) (map[string]string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	metadata := map[string]string{}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err == nil {
		metadata["name"] = firstNonEmptyString(payload["name"], payload["title"])
		metadata["kind"] = firstNonEmptyString(payload["kind"], payload["type"])
		metadata["status"] = firstNonEmptyString(payload["status"])
		metadata["schedule"] = firstNonEmptyString(payload["schedule"], payload["rrule"], payload["cron"])
		return metadata, true, nil
	}
	content := string(data)
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		sep := strings.Index(line, "=")
		if sep <= 0 {
			sep = strings.Index(line, ":")
		}
		if sep <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:sep])
		value := strings.TrimSpace(line[sep+1:])
		switch strings.ToLower(key) {
		case "name", "title":
			metadata["name"] = parseCodexStringValue(value)
		case "kind", "type":
			metadata["kind"] = parseCodexStringValue(value)
		case "status":
			metadata["status"] = parseCodexStringValue(value)
		case "schedule", "rrule", "cron":
			metadata["schedule"] = parseCodexStringValue(value)
		}
	}
	return metadata, len(metadata) > 0, scanner.Err()
}

func readClaudePluginRecords(path string) ([]sqlite.AgentRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var payload interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, nil
	}
	records := []sqlite.AgentRecord{}
	appendRecord := func(name string, value map[string]interface{}) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		version := firstNonEmptyString(value["version"])
		description := firstNonEmptyString(value["description"], value["summary"])
		lines := []string{"Observed Claude plugin metadata."}
		if version != "" {
			lines = append(lines, "- Version: "+version)
		}
		if description != "" {
			lines = append(lines, "- Description: "+description)
		}
		records = append(records, sqlite.AgentRecord{
			Name:        name,
			Content:     strings.Join(lines, "\n"),
			Exactness:   "exact",
			SourcePaths: []string{path},
			Confidence:  1,
			Metadata: map[string]interface{}{
				"version":     version,
				"description": description,
			},
		})
	}
	switch typed := payload.(type) {
	case []interface{}:
		for _, item := range typed {
			if entry, ok := item.(map[string]interface{}); ok {
				appendRecord(firstNonEmptyString(entry["name"], entry["display_name"]), entry)
			}
		}
	case map[string]interface{}:
		if name := firstNonEmptyString(typed["name"], typed["display_name"]); name != "" {
			appendRecord(name, typed)
			break
		}
		for key, value := range typed {
			entry, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			appendRecord(firstNonEmptyString(entry["name"], entry["display_name"], key), entry)
		}
	}
	return records, nil
}

func scanClaudeProjectRoot(inventory *sqlite.ClaudeInventory, root string, notes *[]string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	projectName := normalizeClaudeName(filepath.Base(root), "claude-project")
	project := sqlite.ClaudeProjectSnapshot{
		Name:        projectName,
		Exactness:   "exact",
		SourcePaths: []string{root},
	}
	contextParts := []string{}
	for _, candidate := range []string{filepath.Join(root, "CLAUDE.md"), filepath.Join(root, "CLAUDE.local.md")} {
		content, ok, err := readTextFile(candidate)
		if err != nil {
			return err
		}
		if !ok || strings.TrimSpace(content) == "" {
			continue
		}
		contextParts = append(contextParts, fmt.Sprintf("## %s\n\n%s", filepath.Base(candidate), strings.TrimSpace(content)))
	}
	project.Context = strings.TrimSpace(strings.Join(contextParts, "\n\n"))
	if err := scanClaudeProjectKnowledgeFiles(&project, root, notes); err != nil {
		return err
	}
	if strings.TrimSpace(project.Context) != "" || len(project.Files) > 0 {
		inventory.Projects = append(inventory.Projects, project)
	}

	if err := scanClaudeBundleDirectory(inventory, filepath.Join(root, ".claude", "skills"), "skill", []string{root}, notes); err != nil {
		return err
	}
	if err := scanClaudeMarkdownBundles(inventory, filepath.Join(root, ".claude", "agents"), "agent"); err != nil {
		return err
	}
	if err := scanClaudeMarkdownBundles(inventory, filepath.Join(root, ".claude", "commands"), "command"); err != nil {
		return err
	}
	if err := scanClaudeMarkdownBundles(inventory, filepath.Join(root, ".claude", "rules"), "rule"); err != nil {
		return err
	}
	for _, pair := range []struct {
		source string
		target string
	}{
		{filepath.Join(root, ".claude", "output-styles", "default.md"), filepath.Join("agent/projects", projectName, "output-style-default.md")},
	} {
		if err := archiveClaudeRuntimeFile(inventory, pair.source, pair.target); err != nil {
			return err
		}
	}
	return nil
}

func scanClaudeProjectKnowledgeFiles(project *sqlite.ClaudeProjectSnapshot, root string, notes *[]string) error {
	candidates := []string{
		filepath.Join(root, "README.md"),
		filepath.Join(root, "docs"),
		filepath.Join(root, "notes"),
		filepath.Join(root, "knowledge"),
		filepath.Join(root, "prompts"),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if info.IsDir() {
			if err := filepath.Walk(candidate, func(path string, info os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if info.IsDir() {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(info.Name()))
				switch ext {
				case ".md", ".txt", ".json", ".yaml", ".yml":
				default:
					return nil
				}
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				record, ok, note, _, _, err := readClaudeFileRecord(path, filepath.ToSlash(rel), false)
				if err != nil {
					return err
				}
				if note != "" && notes != nil {
					*notes = append(*notes, note)
				}
				if ok {
					project.Files = append(project.Files, record)
				}
				return nil
			}); err != nil {
				return err
			}
			continue
		}
		rel, err := filepath.Rel(root, candidate)
		if err != nil {
			return err
		}
		record, ok, note, _, _, err := readClaudeFileRecord(candidate, filepath.ToSlash(rel), false)
		if err != nil {
			return err
		}
		if note != "" && notes != nil {
			*notes = append(*notes, note)
		}
		if ok {
			project.Files = append(project.Files, record)
		}
	}
	return nil
}

func scanClaudeBundleDirectory(inventory *sqlite.ClaudeInventory, dir, kind string, sourcePaths []string, notes *[]string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == neudriveSkillName {
			continue
		}
		bundleRoot := filepath.Join(dir, entry.Name())
		bundle := sqlite.ClaudeBundle{
			Name:        entry.Name(),
			Kind:        kind,
			Exactness:   "exact",
			SourcePaths: append([]string{bundleRoot}, sourcePaths...),
		}
		err := filepath.Walk(bundleRoot, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				if path != bundleRoot && isManagedNeuDriveDir(path) {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(bundleRoot, path)
			if err != nil {
				return err
			}
			record, ok, note, _, _, err := readClaudeFileRecord(path, rel, false)
			if err != nil {
				return err
			}
			if note != "" && notes != nil {
				*notes = append(*notes, note)
			}
			if ok {
				bundle.Files = append(bundle.Files, record)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if len(bundle.Files) > 0 {
			inventory.Bundles = append(inventory.Bundles, bundle)
		}
	}
	return nil
}

func scanClaudeMarkdownBundles(inventory *sqlite.ClaudeInventory, dir, kind string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Base(path), "neudrive.md") {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}
		content, ok, err := readTextFile(path)
		if err != nil || !ok || strings.TrimSpace(content) == "" {
			return err
		}
		name := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
		inventory.Bundles = append(inventory.Bundles, sqlite.ClaudeBundle{
			Name:        name,
			Kind:        kind,
			Exactness:   "exact",
			SourcePaths: []string{path},
			Files: []sqlite.ClaudeFileRecord{
				{
					Path:        info.Name(),
					Content:     content,
					ContentType: "text/markdown",
					Exactness:   "exact",
					SourcePath:  path,
				},
			},
		})
		return nil
	})
}

func archiveClaudeRuntimeTree(inventory *sqlite.ClaudeInventory, dir, targetPrefix string, notes *[]string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		record, ok, note, findings, candidates, err := readClaudeFileRecord(path, filepath.ToSlash(filepath.Join(targetPrefix, rel)), true)
		if err != nil {
			return err
		}
		if note != "" && notes != nil {
			*notes = append(*notes, note)
		}
		if ok {
			inventory.Files = append(inventory.Files, record)
		}
		inventory.SensitiveFindings = append(inventory.SensitiveFindings, findings...)
		inventory.VaultCandidates = append(inventory.VaultCandidates, candidates...)
		return nil
	})
}

func archiveClaudeRuntimeFile(inventory *sqlite.ClaudeInventory, sourcePath, targetPath string) error {
	record, ok, _, findings, candidates, err := readClaudeFileRecord(sourcePath, targetPath, true)
	if err != nil || !ok {
		return err
	}
	inventory.Files = append(inventory.Files, record)
	inventory.SensitiveFindings = append(inventory.SensitiveFindings, findings...)
	inventory.VaultCandidates = append(inventory.VaultCandidates, candidates...)
	return nil
}

func readClaudeFileRecord(sourcePath, targetPath string, redact bool) (sqlite.ClaudeFileRecord, bool, string, []sqlite.AgentSensitiveFinding, []sqlite.AgentVaultCandidate, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return sqlite.ClaudeFileRecord{}, false, "", nil, nil, nil
		}
		return sqlite.ClaudeFileRecord{}, false, "", nil, nil, err
	}
	contentType := skillsarchive.DetectContentType(sourcePath, data)
	record := sqlite.ClaudeFileRecord{
		Path:        filepath.ToSlash(targetPath),
		ContentType: contentType,
		Exactness:   "exact",
		SourcePath:  sourcePath,
	}
	if skillsarchive.LooksBinary(sourcePath, data) {
		if len(data) > claudeBinaryInlineMaxBytes {
			return sqlite.ClaudeFileRecord{}, false, fmt.Sprintf("Skipped large binary asset %s during Claude scan.", sourcePath), nil, nil, nil
		}
		record.ContentBase64 = base64.StdEncoding.EncodeToString(data)
		return record, true, "", nil, nil, nil
	}
	content := string(data)
	if redact {
		redacted, findings, candidates := redactSensitiveText(sourcePath, content)
		record.Content = redacted
		record.SourcePaths = []string{sourcePath}
		return record, true, "", findings, candidates, nil
	} else {
		record.Content = content
	}
	return record, true, "", nil, nil, nil
}

func redactSensitiveText(sourcePath, content string) (string, []sqlite.AgentSensitiveFinding, []sqlite.AgentVaultCandidate) {
	lines := strings.Split(content, "\n")
	findings := []sqlite.AgentSensitiveFinding{}
	candidates := []sqlite.AgentVaultCandidate{}
	seen := map[string]struct{}{}
	for i, line := range lines {
		sep := strings.IndexAny(line, ":=")
		if sep <= 0 {
			continue
		}
		key := strings.Trim(strings.TrimSpace(line[:sep]), "\"'")
		value := strings.TrimSpace(line[sep+1:])
		if !looksSensitiveKey(key) || value == "" || value == "{}" || value == "[]" {
			continue
		}
		lines[i] = line[:sep+1] + " [REDACTED]"
		id := sourcePath + ":" + strings.ToLower(key)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		findings = append(findings, sqlite.AgentSensitiveFinding{
			Title:           fmt.Sprintf("%s in %s", key, filepath.Base(sourcePath)),
			Detail:          "Potential plaintext secret discovered during Claude Code migration scan.",
			Severity:        "high",
			SourcePaths:     []string{sourcePath},
			RedactedExample: strings.TrimSpace(lines[i]),
		})
		candidates = append(candidates, sqlite.AgentVaultCandidate{
			Scope:       fmt.Sprintf("claude.%s.%s", normalizeClaudeName(filepath.Base(sourcePath), "file"), normalizeClaudeName(key, "secret")),
			Description: fmt.Sprintf("Candidate vault scope for %s discovered in %s.", key, sourcePath),
			SourcePaths: []string{sourcePath},
		})
	}
	return strings.Join(lines, "\n"), findings, candidates
}

func looksSensitiveKey(raw string) bool {
	key := strings.ToLower(strings.TrimSpace(raw))
	for _, needle := range []string{"token", "secret", "password", "api_key", "apikey", "authorization", "bearer", "appkey", "appsecret", "client_secret"} {
		if strings.Contains(key, needle) {
			return true
		}
	}
	return false
}

func discoverClaudeProjectRoots(configPath string) []string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	roots := map[string]struct{}{}
	for _, key := range []string{"projects", "githubRepoPaths"} {
		collectClaudeProjectRoots(payload[key], roots)
	}
	out := make([]string, 0, len(roots))
	for root := range roots {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			out = append(out, root)
		}
	}
	sort.Strings(out)
	return out
}

func collectClaudeProjectRoots(value interface{}, roots map[string]struct{}) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, nested := range typed {
			if root, ok := normalizeClaudeProjectRoot(key); ok {
				roots[root] = struct{}{}
			}
			collectClaudeProjectRoots(nested, roots)
		}
	case []interface{}:
		for _, item := range typed {
			collectClaudeProjectRoots(item, roots)
		}
	case string:
		if root, ok := normalizeClaudeProjectRoot(typed); ok {
			roots[root] = struct{}{}
		}
	}
}

func normalizeClaudeProjectRoot(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if strings.HasPrefix(raw, "~/") {
		raw = expandUser(raw)
	}
	if filepath.IsAbs(raw) {
		return raw, true
	}
	return "", false
}

func appendUniqueProfileRules(base, extra []sqlite.AgentProfileRule) []sqlite.AgentProfileRule {
	seen := map[string]struct{}{}
	for _, item := range base {
		seen[item.Title+"|"+strings.Join(item.SourcePaths, ",")] = struct{}{}
	}
	for _, item := range extra {
		key := item.Title + "|" + strings.Join(item.SourcePaths, ",")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		base = append(base, item)
	}
	return base
}

func appendUniqueMemoryItems(base, extra []sqlite.AgentMemoryItem) []sqlite.AgentMemoryItem {
	seen := map[string]struct{}{}
	for _, item := range base {
		seen[item.Title+"|"+strings.Join(item.SourcePaths, ",")] = struct{}{}
	}
	for _, item := range extra {
		key := item.Title + "|" + strings.Join(item.SourcePaths, ",")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		base = append(base, item)
	}
	return base
}

func appendUniqueProjects(base, extra []sqlite.AgentProjectRecord) []sqlite.AgentProjectRecord {
	seen := map[string]struct{}{}
	for _, item := range base {
		seen[item.Name] = struct{}{}
	}
	for _, item := range extra {
		if _, ok := seen[item.Name]; ok {
			continue
		}
		seen[item.Name] = struct{}{}
		base = append(base, item)
	}
	return base
}

func appendUniqueAgentRecords(base, extra []sqlite.AgentRecord) []sqlite.AgentRecord {
	seen := map[string]struct{}{}
	for _, item := range base {
		seen[item.Name+"|"+strings.Join(item.SourcePaths, ",")] = struct{}{}
	}
	for _, item := range extra {
		key := item.Name + "|" + strings.Join(item.SourcePaths, ",")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		base = append(base, item)
	}
	return base
}

func appendUniqueSensitiveFindings(base, extra []sqlite.AgentSensitiveFinding) []sqlite.AgentSensitiveFinding {
	seen := map[string]struct{}{}
	for _, item := range base {
		seen[item.Title+"|"+strings.Join(item.SourcePaths, ",")] = struct{}{}
	}
	for _, item := range extra {
		key := item.Title + "|" + strings.Join(item.SourcePaths, ",")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		base = append(base, item)
	}
	return base
}

func appendUniqueVaultCandidates(base, extra []sqlite.AgentVaultCandidate) []sqlite.AgentVaultCandidate {
	seen := map[string]struct{}{}
	for _, item := range base {
		seen[item.Scope+"|"+strings.Join(item.SourcePaths, ",")] = struct{}{}
	}
	for _, item := range extra {
		key := item.Scope + "|" + strings.Join(item.SourcePaths, ",")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		base = append(base, item)
	}
	return base
}

func appendUniqueStrings(base, extra []string) []string {
	seen := map[string]struct{}{}
	for _, item := range base {
		seen[item] = struct{}{}
	}
	for _, item := range extra {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		base = append(base, item)
	}
	return base
}

func readTextFile(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(data), true, nil
}

func firstNonEmptyLine(content, fallback string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 80 {
				return line[:80]
			}
			return line
		}
	}
	return fallback
}

func isManagedNeuDriveDir(pathValue string) bool {
	_, err := os.Stat(filepath.Join(pathValue, managedMarkerFile))
	return err == nil
}

func normalizeClaudeName(raw, fallback string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		raw = fallback
	}
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
}
