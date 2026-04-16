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

	"github.com/agi-bar/neudrive/internal/runtimecfg"
	"github.com/agi-bar/neudrive/internal/skillsarchive"
	"github.com/agi-bar/neudrive/internal/storage/sqlite"
)

const claudeBinaryInlineMaxBytes = 64 << 10

type ImportPreview struct {
	Platform          string                         `json:"platform"`
	DisplayName       string                         `json:"display_name"`
	Mode              ImportMode                     `json:"mode"`
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
	Inventory    sqlite.ClaudeInventory
	Notes        []string
}

func PreviewImport(ctx context.Context, cfg *runtimecfg.CLIConfig, platform, rawMode string) (*ImportPreview, error) {
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
	}
	if mode != ImportModeFiles && supportsAgentMediatedImport(adapter.ID()) {
		agentPayload, err := runAgentExport(ctx, adapter.ID())
		if err != nil {
			preview.Notes = append(preview.Notes, fmt.Sprintf("Agent semantic scan unavailable: %v", err))
		} else {
			payload = mergeAgentPayload(payload, agentPayload)
		}
	}

	preview.Categories = buildImportPreviewCategories(mode, sources, payload)
	if payload.Claude != nil {
		preview.SensitiveFindings = append(preview.SensitiveFindings, payload.Claude.SensitiveFindings...)
		preview.VaultCandidates = append(preview.VaultCandidates, payload.Claude.VaultCandidates...)
	}
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
	base.Automations = append(base.Automations, extra.Automations...)
	base.Tools = append(base.Tools, extra.Tools...)
	base.Connections = append(base.Connections, extra.Connections...)
	base.Archives = append(base.Archives, extra.Archives...)
	base.Unsupported = append(base.Unsupported, extra.Unsupported...)
	base.Notes = append(base.Notes, extra.Notes...)
	if extra.Claude != nil {
		if base.Claude == nil {
			base.Claude = &sqlite.ClaudeInventory{}
		}
		base.Claude = mergeClaudeInventory(base.Claude, extra.Claude)
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
	return fmt.Sprintf("neudrive import platform %s --mode %s", name, mode)
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
	}

	if err := archiveClaudeRuntimeFile(&result.Inventory, expandUser("~/.claude.json"), "agent/settings/claude.json"); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeFile(&result.Inventory, expandUser("~/.claude/settings.json"), "agent/settings/settings.json"); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeFile(&result.Inventory, expandUser("~/.claude/settings.local.json"), "agent/settings/settings.local.json"); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeFile(&result.Inventory, expandUser("~/.claude/.credentials.json"), "agent/runtime/credentials.json"); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeFile(&result.Inventory, expandUser("~/.claude/history.jsonl"), "agent/runtime/history.jsonl"); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeTree(&result.Inventory, expandUser("~/.claude/todos"), "agent/runtime/todos", &result.Notes); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeTree(&result.Inventory, expandUser("~/.claude/plans"), "agent/runtime/plans", &result.Notes); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeTree(&result.Inventory, expandUser("~/.claude/agent-memory"), "agent/runtime/agent-memory", &result.Notes); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeTree(&result.Inventory, expandUser("~/.claude/scheduled-tasks"), "agent/runtime/scheduled-tasks", &result.Notes); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeTree(&result.Inventory, expandUser("~/.claude/channels"), "agent/runtime/channels", &result.Notes); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeTree(&result.Inventory, expandUser("~/.claude/output-styles"), "agent/runtime/output-styles", &result.Notes); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeTree(&result.Inventory, expandUser("~/.claude/hooks"), "agent/runtime/hooks", &result.Notes); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeTree(&result.Inventory, expandUser("~/.claude/plugins"), "agent/runtime/plugins", &result.Notes); err != nil {
		return nil, err
	}
	if err := archiveClaudeRuntimeFile(&result.Inventory, expandUser("~/.claude/plugins/installed_plugins.json"), "agent/runtime/plugins/installed_plugins.json"); err != nil {
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
		role, content, ts := extractClaudeConversationMessage(entry)
		if strings.TrimSpace(content) == "" {
			if errors.Is(readErr, io.EOF) {
				break
			}
			continue
		}
		if firstTimestamp == "" && strings.TrimSpace(ts) != "" {
			firstTimestamp = strings.TrimSpace(ts)
		}
		if title == strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) && role == "user" {
			title = firstNonEmptyLine(content, title)
		}
		messages = append(messages, sqlite.ClaudeConversationMessage{
			Role:      role,
			Content:   content,
			Timestamp: strings.TrimSpace(ts),
			Kind:      strings.TrimSpace(fmt.Sprint(entry["type"])),
		})
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
			summary = firstNonEmptyLine(message.Content, "")
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

func extractClaudeConversationMessage(entry map[string]interface{}) (string, string, string) {
	role := strings.TrimSpace(fmt.Sprint(entry["type"]))
	timestamp := strings.TrimSpace(fmt.Sprint(entry["timestamp"]))
	if message, ok := entry["message"].(map[string]interface{}); ok {
		if msgRole := strings.TrimSpace(fmt.Sprint(message["role"])); msgRole != "" {
			role = msgRole
		}
		if content := flattenClaudeContent(message["content"]); strings.TrimSpace(content) != "" {
			return role, content, timestamp
		}
	}
	if content := flattenClaudeContent(entry["content"]); strings.TrimSpace(content) != "" {
		return role, content, timestamp
	}
	serialized, err := json.Marshal(entry)
	if err != nil {
		return role, "", timestamp
	}
	return role, string(serialized), timestamp
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
		{filepath.Join(root, ".mcp.json"), filepath.Join("agent/projects", projectName, "mcp.json")},
		{filepath.Join(root, ".claude", "settings.json"), filepath.Join("agent/projects", projectName, "settings.json")},
		{filepath.Join(root, ".claude", "settings.local.json"), filepath.Join("agent/projects", projectName, "settings.local.json")},
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
