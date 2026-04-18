package platforms

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agi-bar/neudrive/internal/storage/sqlite"
)

type codexLocalScanResult struct {
	ProfileRules      []sqlite.AgentProfileRule
	MemoryItems       []sqlite.AgentMemoryItem
	Projects          []sqlite.AgentProjectRecord
	Automations       []sqlite.AgentRecord
	Tools             []sqlite.AgentRecord
	Connections       []sqlite.AgentRecord
	Archives          []sqlite.AgentRecord
	Unsupported       []sqlite.AgentRecord
	SensitiveFindings []sqlite.AgentSensitiveFinding
	VaultCandidates   []sqlite.AgentVaultCandidate
	Notes             []string
}

type codexConfigSummary struct {
	Model                string
	ModelReasoningEffort string
	ApprovalPolicy       string
	SandboxMode          string
	Projects             []codexTrustedProject
	MCPServers           []codexMCPServer
}

type codexTrustedProject struct {
	Path       string
	TrustLevel string
}

type codexMCPServer struct {
	Name    string
	Command string
	URL     string
	Args    []string
	EnvKeys []string
}

type codexSessionIndexEntry struct {
	ID         string `json:"id"`
	ThreadName string `json:"thread_name"`
	UpdatedAt  string `json:"updated_at"`
}

type codexSessionSummary struct {
	ID            string
	ThreadName    string
	CWD           string
	StartedAt     string
	UpdatedAt     string
	Originator    string
	Source        string
	CLI           string
	ModelProvider string
	FilePath      string
	Archived      bool
}

type codexProjectAggregate struct {
	CWD         string
	Active      int
	Archived    int
	LastAt      string
	Originators map[string]struct{}
	Sources     map[string]struct{}
	Titles      []string
	SourcePaths []string
}

func enrichCodexPayload(payload sqlite.AgentExportPayload) (sqlite.AgentExportPayload, []string, error) {
	scan, err := scanLocalCodexMigration()
	if err != nil {
		return payload, nil, err
	}
	return mergeCodexScanIntoPayload(payload, scan), scan.Notes, nil
}

func mergeCodexScanIntoPayload(payload sqlite.AgentExportPayload, scan *codexLocalScanResult) sqlite.AgentExportPayload {
	if scan == nil {
		return payload
	}
	payload.ProfileRules = appendUniqueProfileRules(payload.ProfileRules, scan.ProfileRules)
	payload.MemoryItems = appendUniqueMemoryItems(payload.MemoryItems, scan.MemoryItems)
	payload.Projects = appendUniqueProjects(payload.Projects, scan.Projects)
	payload.Automations = appendUniqueAgentRecords(payload.Automations, scan.Automations)
	payload.Tools = appendUniqueAgentRecords(payload.Tools, scan.Tools)
	payload.Connections = appendUniqueAgentRecords(payload.Connections, scan.Connections)
	payload.Archives = appendUniqueAgentRecords(payload.Archives, scan.Archives)
	payload.Unsupported = appendUniqueAgentRecords(payload.Unsupported, scan.Unsupported)
	payload.SensitiveFindings = appendUniqueSensitiveFindings(payload.SensitiveFindings, scan.SensitiveFindings)
	payload.VaultCandidates = appendUniqueVaultCandidates(payload.VaultCandidates, scan.VaultCandidates)
	payload.Notes = appendUniqueStrings(payload.Notes, scan.Notes)
	if strings.TrimSpace(payload.Platform) == "" {
		payload.Platform = "codex"
	}
	if strings.TrimSpace(payload.Command) == "" {
		payload.Command = "local-scan"
	}
	return payload
}

func scanLocalCodexMigration() (*codexLocalScanResult, error) {
	result := &codexLocalScanResult{}

	if content, ok, err := readTextFile(expandUser("~/.codex/AGENTS.md")); err != nil {
		return nil, err
	} else if ok && strings.TrimSpace(content) != "" {
		result.ProfileRules = append(result.ProfileRules, sqlite.AgentProfileRule{
			Title:       "Global AGENTS.md",
			Content:     strings.TrimSpace(content),
			Exactness:   "exact",
			SourcePaths: []string{expandUser("~/.codex/AGENTS.md")},
			Confidence:  1,
		})
	}

	if err := scanCodexRuleTree(result, expandUser("~/.codex/rules")); err != nil {
		return nil, err
	}
	if err := scanCodexMemoryTree(result, expandUser("~/.codex/memories")); err != nil {
		return nil, err
	}
	if err := scanCodexConfig(result, expandUser("~/.codex/config.toml")); err != nil {
		return nil, err
	}
	if err := scanCodexAuth(result, expandUser("~/.codex/auth.json")); err != nil {
		return nil, err
	}
	if err := scanCodexSessions(result, expandUser("~/.codex/session_index.jsonl"), expandUser("~/.codex/sessions"), expandUser("~/.codex/archived_sessions")); err != nil {
		return nil, err
	}
	if err := scanCodexAutomations(result, expandUser("~/.codex/automations")); err != nil {
		return nil, err
	}
	if err := appendCodexSkillInventory(result, expandUser("~/.agents/skills"), "custom_skills", "Observed custom Codex skills installed under ~/.agents/skills."); err != nil {
		return nil, err
	}
	if err := appendCodexSkillInventory(result, expandUser("~/.codex/skills"), "bundled_skills", "Observed bundled Codex skill packs under ~/.codex/skills."); err != nil {
		return nil, err
	}
	if err := appendCodexRuntimeArchives(result); err != nil {
		return nil, err
	}
	return result, nil
}

func scanCodexRuleTree(result *codexLocalScanResult, dir string) error {
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
		if ext != ".rules" && ext != ".md" && ext != ".txt" {
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
			Title:       "Rule set: " + filepath.ToSlash(rel),
			Content:     strings.TrimSpace(content),
			Exactness:   "exact",
			SourcePaths: []string{path},
			Confidence:  1,
		})
		return nil
	})
}

func scanCodexMemoryTree(result *codexLocalScanResult, dir string) error {
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
			Title:       "memories/" + filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel))),
			Content:     strings.TrimSpace(content),
			Exactness:   "exact",
			SourcePaths: []string{path},
			Confidence:  1,
		})
		return nil
	})
}

func scanCodexConfig(result *codexLocalScanResult, path string) error {
	content, ok, err := readTextFile(path)
	if err != nil || !ok || strings.TrimSpace(content) == "" {
		return err
	}

	summary := parseCodexConfigSummary(content)
	lines := []string{}
	if summary.Model != "" {
		lines = append(lines, "- Model: "+summary.Model)
	}
	if summary.ModelReasoningEffort != "" {
		lines = append(lines, "- Reasoning effort: "+summary.ModelReasoningEffort)
	}
	if summary.ApprovalPolicy != "" {
		lines = append(lines, "- Approval policy: "+summary.ApprovalPolicy)
	}
	if summary.SandboxMode != "" {
		lines = append(lines, "- Sandbox mode: "+summary.SandboxMode)
	}
	if len(summary.Projects) > 0 {
		lines = append(lines, fmt.Sprintf("- Trusted projects configured: %d", len(summary.Projects)))
	}
	if len(lines) > 0 {
		result.ProfileRules = append(result.ProfileRules, sqlite.AgentProfileRule{
			Title:       "Codex runtime preferences",
			Content:     strings.Join(lines, "\n"),
			Exactness:   "derived",
			SourcePaths: []string{path},
			Confidence:  0.98,
		})
	}

	for _, server := range summary.MCPServers {
		record := sqlite.AgentRecord{
			Name:        server.Name,
			Exactness:   "exact",
			SourcePaths: []string{path},
			Confidence:  1,
			Metadata: map[string]interface{}{
				"command":  strings.TrimSpace(server.Command),
				"url":      strings.TrimSpace(server.URL),
				"args":     append([]string{}, server.Args...),
				"env_keys": append([]string{}, server.EnvKeys...),
			},
		}
		serverLines := []string{}
		if server.Command != "" {
			serverLines = append(serverLines, "- Command: "+server.Command)
		}
		if len(server.Args) > 0 {
			serverLines = append(serverLines, "- Args: "+strings.Join(server.Args, " "))
		}
		if server.URL != "" {
			serverLines = append(serverLines, "- URL: "+server.URL)
		}
		if len(server.EnvKeys) > 0 {
			serverLines = append(serverLines, "- Env keys: "+strings.Join(server.EnvKeys, ", "))
		}
		record.Content = strings.Join(serverLines, "\n")
		result.Connections = append(result.Connections, record)

		for _, key := range server.EnvKeys {
			if !codexSensitiveKey(key) {
				continue
			}
			result.SensitiveFindings = append(result.SensitiveFindings, sqlite.AgentSensitiveFinding{
				Title:           fmt.Sprintf("Sensitive env key in mcp_servers.%s.env", server.Name),
				Detail:          fmt.Sprintf("Codex config stores the env key `%s` for MCP server `%s`. The value was not imported.", key, server.Name),
				Severity:        "high",
				SourcePaths:     []string{path},
				RedactedExample: fmt.Sprintf("%s=[REDACTED]", key),
			})
			result.VaultCandidates = append(result.VaultCandidates, sqlite.AgentVaultCandidate{
				Scope:       fmt.Sprintf("codex.mcp.%s.%s", normalizeClaudeName(server.Name, "server"), normalizeClaudeName(key, "secret")),
				Description: fmt.Sprintf("Store `%s` for Codex MCP server `%s` in vault instead of importing plaintext values.", key, server.Name),
				SourcePaths: []string{path},
			})
		}
	}
	return nil
}

func parseCodexConfigSummary(content string) codexConfigSummary {
	summary := codexConfigSummary{}
	section := ""
	projectTrust := map[string]string{}
	projectOrder := []string{}
	serverIndex := map[string]int{}

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		value := strings.TrimSpace(line[eq+1:])
		switch {
		case section == "":
			switch key {
			case "model":
				summary.Model = parseCodexStringValue(value)
			case "model_reasoning_effort":
				summary.ModelReasoningEffort = parseCodexStringValue(value)
			case "approval_policy":
				summary.ApprovalPolicy = parseCodexStringValue(value)
			case "sandbox_mode":
				summary.SandboxMode = parseCodexStringValue(value)
			}
		case strings.HasPrefix(section, `projects."`) && strings.HasSuffix(section, `"`) && key == "trust_level":
			projectPath := strings.TrimSuffix(strings.TrimPrefix(section, `projects."`), `"`)
			if _, ok := projectTrust[projectPath]; !ok {
				projectOrder = append(projectOrder, projectPath)
			}
			projectTrust[projectPath] = parseCodexStringValue(value)
		case strings.HasPrefix(section, "mcp_servers.") && !strings.HasSuffix(section, ".env"):
			serverName := strings.TrimPrefix(section, "mcp_servers.")
			index, ok := serverIndex[serverName]
			if !ok {
				index = len(summary.MCPServers)
				serverIndex[serverName] = index
				summary.MCPServers = append(summary.MCPServers, codexMCPServer{Name: serverName})
			}
			switch key {
			case "command":
				summary.MCPServers[index].Command = parseCodexStringValue(value)
			case "url":
				summary.MCPServers[index].URL = parseCodexStringValue(value)
			case "args":
				summary.MCPServers[index].Args = parseCodexStringArray(value)
			}
		case strings.HasPrefix(section, "mcp_servers.") && strings.HasSuffix(section, ".env"):
			serverName := strings.TrimSuffix(strings.TrimPrefix(section, "mcp_servers."), ".env")
			index, ok := serverIndex[serverName]
			if !ok {
				index = len(summary.MCPServers)
				serverIndex[serverName] = index
				summary.MCPServers = append(summary.MCPServers, codexMCPServer{Name: serverName})
			}
			summary.MCPServers[index].EnvKeys = append(summary.MCPServers[index].EnvKeys, key)
		}
	}

	for _, path := range projectOrder {
		summary.Projects = append(summary.Projects, codexTrustedProject{
			Path:       path,
			TrustLevel: projectTrust[path],
		})
	}
	for index := range summary.MCPServers {
		sort.Strings(summary.MCPServers[index].EnvKeys)
	}
	return summary
}

func parseCodexStringValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) {
		unquoted, err := strconvUnquote(raw)
		if err == nil {
			return unquoted
		}
		return strings.Trim(raw, `"`)
	}
	return strings.Trim(raw, `"`)
}

func parseCodexStringArray(raw string) []string {
	raw = strings.TrimSpace(raw)
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err == nil {
		return out
	}
	return nil
}

func scanCodexAuth(result *codexLocalScanResult, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		result.Unsupported = append(result.Unsupported, sqlite.AgentRecord{
			Name:        "auth-json",
			Content:     "Could not parse ~/.codex/auth.json; preserve the raw file snapshot instead.",
			Exactness:   "reference",
			SourcePaths: []string{path},
			Confidence:  0.5,
		})
		return nil
	}

	tokenKeys := []string{}
	if tokens, ok := payload["tokens"].(map[string]interface{}); ok {
		for key := range tokens {
			tokenKeys = append(tokenKeys, key)
		}
	}
	sort.Strings(tokenKeys)

	lines := []string{}
	if mode := strings.TrimSpace(fmt.Sprint(payload["auth_mode"])); mode != "" && mode != "<nil>" {
		lines = append(lines, "- Auth mode: "+mode)
	}
	if len(tokenKeys) > 0 {
		lines = append(lines, "- Token keys present: "+strings.Join(tokenKeys, ", "))
	}
	if lastRefresh := strings.TrimSpace(fmt.Sprint(payload["last_refresh"])); lastRefresh != "" && lastRefresh != "<nil>" {
		lines = append(lines, "- Last refresh: "+lastRefresh)
	}
	if len(lines) > 0 {
		result.Connections = append(result.Connections, sqlite.AgentRecord{
			Name:        "openai-auth-session",
			Content:     strings.Join(lines, "\n"),
			Exactness:   "exact",
			SourcePaths: []string{path},
			Confidence:  1,
		})
	}
	if len(tokenKeys) > 0 {
		result.SensitiveFindings = append(result.SensitiveFindings, sqlite.AgentSensitiveFinding{
			Title:           "Stored authentication tokens in auth.json",
			Detail:          "Codex local auth.json contains refresh/access session material. Token values were not imported.",
			Severity:        "high",
			SourcePaths:     []string{path},
			RedactedExample: "\"refresh_token\": \"[REDACTED]\"",
		})
		result.VaultCandidates = append(result.VaultCandidates, sqlite.AgentVaultCandidate{
			Scope:       "codex.auth.openai-session",
			Description: "Vault-backed replacement for the OpenAI/ChatGPT session tokens stored in ~/.codex/auth.json.",
			SourcePaths: []string{path},
		})
	}
	return nil
}

func scanCodexSessions(result *codexLocalScanResult, indexPath, activeRoot, archivedRoot string) error {
	indexEntries, err := readCodexSessionIndex(indexPath)
	if err != nil {
		return err
	}

	summaries := []codexSessionSummary{}
	activeCount, err := scanCodexSessionDirectory(&summaries, activeRoot, false, indexEntries)
	if err != nil {
		return err
	}
	archivedCount, err := scanCodexSessionDirectory(&summaries, archivedRoot, true, indexEntries)
	if err != nil {
		return err
	}

	if len(summaries) == 0 {
		return nil
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].UpdatedAt == summaries[j].UpdatedAt {
			return summaries[i].FilePath < summaries[j].FilePath
		}
		return summaries[i].UpdatedAt > summaries[j].UpdatedAt
	})

	groups := map[string]*codexProjectAggregate{}
	groupOrder := []string{}
	for _, session := range summaries {
		cwd := strings.TrimSpace(session.CWD)
		if cwd == "" {
			continue
		}
		group := groups[cwd]
		if group == nil {
			group = &codexProjectAggregate{
				CWD:         cwd,
				Originators: map[string]struct{}{},
				Sources:     map[string]struct{}{},
			}
			groups[cwd] = group
			groupOrder = append(groupOrder, cwd)
		}
		if session.Archived {
			group.Archived++
		} else {
			group.Active++
		}
		if session.UpdatedAt > group.LastAt {
			group.LastAt = session.UpdatedAt
		}
		if session.Originator != "" {
			group.Originators[session.Originator] = struct{}{}
		}
		if session.Source != "" {
			group.Sources[session.Source] = struct{}{}
		}
		if len(group.Titles) < 5 && strings.TrimSpace(session.ThreadName) != "" {
			group.Titles = append(group.Titles, strings.TrimSpace(session.ThreadName))
		}
		if len(group.SourcePaths) < 6 {
			group.SourcePaths = append(group.SourcePaths, session.FilePath)
		}
	}

	sort.Strings(groupOrder)
	usedNames := map[string]int{}
	for _, cwd := range groupOrder {
		group := groups[cwd]
		projectName := codexProjectName(cwd, usedNames)
		sourcePaths := append([]string{cwd}, group.SourcePaths...)
		lines := []string{
			"Imported from Codex local session inventory.",
			"",
			"- Workspace: " + cwd,
			fmt.Sprintf("- Active sessions: %d", group.Active),
			fmt.Sprintf("- Archived sessions: %d", group.Archived),
		}
		if group.LastAt != "" {
			lines = append(lines, "- Last activity: "+group.LastAt)
		}
		if names := sortedCodexKeys(group.Originators); len(names) > 0 {
			lines = append(lines, "- Originators: "+strings.Join(names, ", "))
		}
		if names := sortedCodexKeys(group.Sources); len(names) > 0 {
			lines = append(lines, "- Sources: "+strings.Join(names, ", "))
		}
		if len(group.Titles) > 0 {
			lines = append(lines, "- Recent threads:")
			for _, title := range group.Titles {
				lines = append(lines, "  - "+title)
			}
		}
		result.Projects = append(result.Projects, sqlite.AgentProjectRecord{
			Name:        projectName,
			Context:     strings.Join(lines, "\n"),
			Exactness:   "derived",
			SourcePaths: sourcePaths,
		})
	}

	indexCount := len(indexEntries)
	result.Archives = append(result.Archives, sqlite.AgentRecord{
		Name:      "session-inventory",
		Exactness: "reference",
		Content: strings.Join([]string{
			"Observed Codex local session inventory.",
			fmt.Sprintf("- Indexed sessions: %d", indexCount),
			fmt.Sprintf("- Active session files: %d", activeCount),
			fmt.Sprintf("- Archived session files: %d", archivedCount),
			fmt.Sprintf("- Workspaces discovered: %d", len(groupOrder)),
		}, "\n"),
		SourcePaths: compactStringList([]string{indexPath, activeRoot, archivedRoot}),
		Confidence:  1,
	})
	return nil
}

func readCodexSessionIndex(path string) (map[string]codexSessionIndexEntry, error) {
	entries := map[string]codexSessionIndexEntry{}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry codexSessionIndexEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.ID != "" {
			entries[entry.ID] = entry
		}
	}
	return entries, scanner.Err()
}

func scanCodexSessionDirectory(out *[]codexSessionSummary, root string, archived bool, titles map[string]codexSessionIndexEntry) (int, error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return 0, nil
	}
	count := 0
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || strings.ToLower(filepath.Ext(info.Name())) != ".jsonl" {
			return nil
		}
		count++
		summary, ok, err := parseCodexSessionMeta(path, archived, titles)
		if err != nil {
			return err
		}
		if ok {
			*out = append(*out, summary)
		}
		return nil
	})
	return count, err
}

func parseCodexSessionMeta(path string, archived bool, titles map[string]codexSessionIndexEntry) (codexSessionSummary, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return codexSessionSummary{}, false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var envelope struct {
			Timestamp string `json:"timestamp"`
			Type      string `json:"type"`
			Payload   struct {
				ID            string `json:"id"`
				Timestamp     string `json:"timestamp"`
				CWD           string `json:"cwd"`
				Originator    string `json:"originator"`
				CLIVersion    string `json:"cli_version"`
				Source        string `json:"source"`
				ModelProvider string `json:"model_provider"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		if envelope.Type != "session_meta" {
			continue
		}
		updatedAt := strings.TrimSpace(envelope.Payload.Timestamp)
		if updatedAt == "" {
			updatedAt = strings.TrimSpace(envelope.Timestamp)
		}
		threadName := ""
		if indexEntry, ok := titles[envelope.Payload.ID]; ok {
			threadName = strings.TrimSpace(indexEntry.ThreadName)
			if strings.TrimSpace(indexEntry.UpdatedAt) != "" {
				updatedAt = strings.TrimSpace(indexEntry.UpdatedAt)
			}
		}
		if threadName == "" {
			threadName = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		return codexSessionSummary{
			ID:            strings.TrimSpace(envelope.Payload.ID),
			ThreadName:    threadName,
			CWD:           strings.TrimSpace(envelope.Payload.CWD),
			StartedAt:     strings.TrimSpace(envelope.Payload.Timestamp),
			UpdatedAt:     updatedAt,
			Originator:    strings.TrimSpace(envelope.Payload.Originator),
			Source:        strings.TrimSpace(envelope.Payload.Source),
			CLI:           strings.TrimSpace(envelope.Payload.CLIVersion),
			ModelProvider: strings.TrimSpace(envelope.Payload.ModelProvider),
			FilePath:      path,
			Archived:      archived,
		}, true, nil
	}
	if err := scanner.Err(); err != nil {
		return codexSessionSummary{}, false, err
	}
	return codexSessionSummary{}, false, nil
}

func scanCodexAutomations(result *codexLocalScanResult, dir string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	paths := []string{}
	err = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || strings.ToLower(filepath.Ext(info.Name())) != ".toml" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(paths)
	for _, path := range paths {
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = filepath.Base(path)
		}
		result.Automations = append(result.Automations, sqlite.AgentRecord{
			Name:        filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel))),
			Content:     "Observed Codex automation manifest at " + path,
			Exactness:   "reference",
			SourcePaths: []string{path},
			Confidence:  1,
		})
	}
	return nil
}

func appendCodexSkillInventory(result *codexLocalScanResult, dir, name, intro string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	skills := []string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, entry.Name())
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err == nil {
			skills = append(skills, entry.Name())
		}
	}
	sort.Strings(skills)
	lines := []string{intro, fmt.Sprintf("- Skills discovered: %d", len(skills))}
	for _, skill := range firstCodexItems(skills, 8) {
		lines = append(lines, "- "+skill)
	}
	result.Archives = append(result.Archives, sqlite.AgentRecord{
		Name:        name,
		Content:     strings.Join(lines, "\n"),
		Exactness:   "reference",
		SourcePaths: []string{dir},
		Confidence:  1,
	})
	return nil
}

func appendCodexRuntimeArchives(result *codexLocalScanResult) error {
	runtimePaths := []string{
		expandUser("~/.codex/history.jsonl"),
		expandUser("~/.codex/session_index.jsonl"),
		expandUser("~/.codex/.codex-global-state.json"),
		expandUser("~/.codex/logs_2.sqlite"),
		expandUser("~/.codex/state_5.sqlite"),
	}
	lines := []string{"Observed Codex runtime/state files."}
	sourcePaths := []string{}
	for _, target := range runtimePaths {
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			lines = append(lines, fmt.Sprintf("- %s (%d bytes)", filepath.Base(target), info.Size()))
			sourcePaths = append(sourcePaths, target)
		}
	}
	for _, dir := range []string{expandUser("~/.codex/shell_snapshots"), expandUser("~/.codex/worktrees")} {
		if count, ok, err := countCodexFiles(dir); err != nil {
			return err
		} else if ok {
			lines = append(lines, fmt.Sprintf("- %s (%d files)", filepath.Base(dir), count))
			sourcePaths = append(sourcePaths, dir)
		}
	}
	if len(sourcePaths) == 0 {
		return nil
	}
	result.Archives = append(result.Archives, sqlite.AgentRecord{
		Name:        "runtime-state",
		Content:     strings.Join(lines, "\n"),
		Exactness:   "reference",
		SourcePaths: compactStringList(sourcePaths),
		Confidence:  1,
	})
	return nil
}

func countCodexFiles(dir string) (int, bool, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	if !info.IsDir() {
		return 0, false, nil
	}
	count := 0
	err = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count, true, err
}

func codexProjectName(cwd string, used map[string]int) string {
	base := normalizeClaudeName(filepath.Base(strings.TrimRight(cwd, string(os.PathSeparator))), "codex-project")
	count := used[base]
	used[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, count+1)
}

func sortedCodexKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func firstCodexItems(values []string, max int) []string {
	if len(values) <= max {
		return values
	}
	return values[:max]
}

func compactStringList(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func codexSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch {
	case strings.Contains(normalized, "token"),
		strings.Contains(normalized, "secret"),
		strings.Contains(normalized, "password"),
		strings.Contains(normalized, "api_key"),
		strings.Contains(normalized, "access_key"),
		strings.Contains(normalized, "refresh"),
		strings.Contains(normalized, "bearer"),
		strings.Contains(normalized, "jwt"),
		strings.Contains(normalized, "vault"):
		return true
	default:
		return false
	}
}

func strconvUnquote(value string) (string, error) {
	var out string
	err := json.Unmarshal([]byte(value), &out)
	return out, err
}
