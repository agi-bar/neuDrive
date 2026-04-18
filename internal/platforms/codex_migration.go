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
	Inventory         sqlite.CodexInventory
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

type codexScannedSession struct {
	Summary      codexSessionSummary
	Conversation *sqlite.ClaudeConversation
}

const codexJSONLScannerMaxToken = 16 << 20

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
	if payload.Codex == nil {
		payload.Codex = &sqlite.CodexInventory{}
	}
	payload.Codex = mergeCodexInventory(payload.Codex, &scan.Inventory)
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
	if err := scanCodexSkillBundles(result, expandUser("~/.agents/skills"), "skill"); err != nil {
		return nil, err
	}
	if err := scanCodexSkillBundles(result, expandUser("~/.codex/skills"), "bundled-skill"); err != nil {
		return nil, err
	}
	if err := scanCodexPluginTree(result, expandUser("~/.codex/.tmp/plugins/plugins")); err != nil {
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

	sessions := []codexScannedSession{}
	activeCount, err := scanCodexSessionDirectory(&sessions, activeRoot, false, indexEntries)
	if err != nil {
		return err
	}
	archivedCount, err := scanCodexSessionDirectory(&sessions, archivedRoot, true, indexEntries)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return nil
	}

	sort.Slice(sessions, func(i, j int) bool {
		left := sessions[i].Summary
		right := sessions[j].Summary
		if left.UpdatedAt == right.UpdatedAt {
			return left.FilePath < right.FilePath
		}
		return left.UpdatedAt > right.UpdatedAt
	})

	groups := map[string]*codexProjectAggregate{}
	groupOrder := []string{}
	for _, scanned := range sessions {
		session := scanned.Summary
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
	projectNames := map[string]string{}
	for _, cwd := range groupOrder {
		group := groups[cwd]
		projectName := codexProjectName(cwd, usedNames)
		projectNames[cwd] = projectName
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

	for _, scanned := range sessions {
		if scanned.Conversation == nil || len(scanned.Conversation.Messages) == 0 {
			continue
		}
		convo := *scanned.Conversation
		if projectName := projectNames[strings.TrimSpace(scanned.Summary.CWD)]; projectName != "" {
			convo.ProjectName = projectName
		}
		result.Inventory.Conversations = append(result.Inventory.Conversations, convo)
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
			fmt.Sprintf("- Conversation archives prepared: %d", len(result.Inventory.Conversations)),
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
	scanner.Buffer(make([]byte, 0, 64*1024), codexJSONLScannerMaxToken)
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

func scanCodexSessionDirectory(out *[]codexScannedSession, root string, archived bool, titles map[string]codexSessionIndexEntry) (int, error) {
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
		session, ok, err := parseCodexSessionFile(path, archived, titles)
		if err != nil {
			return err
		}
		if ok {
			*out = append(*out, session)
		}
		return nil
	})
	return count, err
}

func parseCodexSessionFile(path string, archived bool, titles map[string]codexSessionIndexEntry) (codexScannedSession, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return codexScannedSession{}, false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), codexJSONLScannerMaxToken)
	summary := codexSessionSummary{FilePath: path, Archived: archived}
	messages := []sqlite.ClaudeConversationMessage{}
	firstTimestamp := ""
	lastTimestamp := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var envelope map[string]interface{}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		envelopeType := strings.TrimSpace(fmt.Sprint(envelope["type"]))
		envelopeTimestamp := strings.TrimSpace(fmt.Sprint(envelope["timestamp"]))
		switch envelopeType {
		case "session_meta":
			if payload, _ := envelope["payload"].(map[string]interface{}); payload != nil {
				summary.ID = strings.TrimSpace(fmt.Sprint(payload["id"]))
				summary.CWD = strings.TrimSpace(fmt.Sprint(payload["cwd"]))
				summary.StartedAt = strings.TrimSpace(fmt.Sprint(payload["timestamp"]))
				summary.UpdatedAt = summary.StartedAt
				summary.Originator = strings.TrimSpace(fmt.Sprint(payload["originator"]))
				summary.Source = strings.TrimSpace(fmt.Sprint(payload["source"]))
				summary.CLI = strings.TrimSpace(fmt.Sprint(payload["cli_version"]))
				summary.ModelProvider = strings.TrimSpace(fmt.Sprint(payload["model_provider"]))
			}
			continue
		case "response_item":
			payload, _ := envelope["payload"].(map[string]interface{})
			message, ok := extractCodexConversationMessage(payload, envelopeTimestamp)
			if !ok {
				continue
			}
			if firstTimestamp == "" && strings.TrimSpace(message.Timestamp) != "" {
				firstTimestamp = strings.TrimSpace(message.Timestamp)
			}
			if strings.TrimSpace(message.Timestamp) != "" {
				lastTimestamp = strings.TrimSpace(message.Timestamp)
			}
			messages = append(messages, message)
		}
	}
	if err := scanner.Err(); err != nil {
		return codexScannedSession{}, false, err
	}
	if summary.UpdatedAt == "" {
		summary.UpdatedAt = lastTimestamp
	}
	if summary.StartedAt == "" {
		summary.StartedAt = firstTimestamp
	}
	if indexEntry, ok := titles[summary.ID]; ok {
		summary.ThreadName = strings.TrimSpace(indexEntry.ThreadName)
		if strings.TrimSpace(indexEntry.UpdatedAt) != "" {
			summary.UpdatedAt = strings.TrimSpace(indexEntry.UpdatedAt)
		}
	}
	if summary.ThreadName == "" {
		summary.ThreadName = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	scanned := codexScannedSession{Summary: summary}
	if len(messages) > 0 {
		title := summary.ThreadName
		summaryText := ""
		for _, message := range messages {
			if title == summary.ThreadName && strings.EqualFold(message.Role, "user") {
				title = firstNonEmptyLine(preferredClaudeMessageText(message), title)
			}
			if summaryText == "" && strings.EqualFold(message.Role, "assistant") && strings.EqualFold(message.Kind, "message") {
				summaryText = firstNonEmptyLine(preferredClaudeMessageText(message), "")
			}
		}
		scanned.Conversation = &sqlite.ClaudeConversation{
			Name:        title,
			SessionID:   strings.TrimSpace(summary.ID),
			Summary:     summaryText,
			StartedAt:   summary.StartedAt,
			Exactness:   "exact",
			SourcePaths: []string{path},
			Messages:    messages,
		}
	}
	if strings.TrimSpace(scanned.Summary.ID) == "" && scanned.Conversation == nil {
		return codexScannedSession{}, false, nil
	}
	return scanned, true, nil
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
		metadata := parseCodexAutomationMetadata(path)
		lines := []string{"Observed Codex automation manifest."}
		if metadata["name"] != "" {
			lines = append(lines, "- Name: "+metadata["name"])
		}
		if metadata["kind"] != "" {
			lines = append(lines, "- Kind: "+metadata["kind"])
		}
		if metadata["status"] != "" {
			lines = append(lines, "- Status: "+metadata["status"])
		}
		if metadata["schedule"] != "" {
			lines = append(lines, "- Schedule: "+metadata["schedule"])
		}
		result.Automations = append(result.Automations, sqlite.AgentRecord{
			Name:        filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel))),
			Content:     strings.Join(lines, "\n"),
			Exactness:   "exact",
			SourcePaths: []string{path},
			Confidence:  1,
			Metadata: map[string]interface{}{
				"id":       metadata["id"],
				"name":     metadata["name"],
				"kind":     metadata["kind"],
				"status":   metadata["status"],
				"schedule": metadata["schedule"],
			},
		})
	}
	return nil
}

func scanCodexSkillBundles(result *codexLocalScanResult, dir, kind string) error {
	inventory := sqlite.ClaudeInventory{}
	notes := []string{}
	if err := scanClaudeBundleDirectory(&inventory, dir, "skill", nil, &notes); err != nil {
		return err
	}
	for _, bundle := range inventory.Bundles {
		bundle.Kind = kind
		result.Inventory.Bundles = append(result.Inventory.Bundles, bundle)
	}
	result.Notes = appendUniqueStrings(result.Notes, notes)
	return nil
}

func scanCodexPluginTree(result *codexLocalScanResult, dir string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || filepath.Base(path) != "plugin.json" || filepath.Base(filepath.Dir(path)) != ".codex-plugin" {
			return nil
		}
		record, ok, err := readCodexPluginManifest(path)
		if err != nil {
			return err
		}
		if ok {
			result.Tools = append(result.Tools, record)
		}
		return nil
	})
}

func extractCodexConversationMessage(payload map[string]interface{}, timestamp string) (sqlite.ClaudeConversationMessage, bool) {
	if payload == nil {
		return sqlite.ClaudeConversationMessage{}, false
	}
	itemType := strings.TrimSpace(fmt.Sprint(payload["type"]))
	switch itemType {
	case "message":
		role := strings.TrimSpace(fmt.Sprint(payload["role"]))
		if role != "user" && role != "assistant" {
			return sqlite.ClaudeConversationMessage{}, false
		}
		parts := extractCodexContentParts(payload["content"])
		content := strings.TrimSpace(flattenClaudeParts(parts))
		if content == "" {
			return sqlite.ClaudeConversationMessage{}, false
		}
		return sqlite.ClaudeConversationMessage{
			ID:        firstNonEmptyString(payload["id"], payload["message_id"]),
			Role:      role,
			Content:   content,
			Timestamp: strings.TrimSpace(timestamp),
			Kind:      itemType,
			Parts:     parts,
		}, true
	case "reasoning":
		text := extractCodexSummaryText(payload["summary"])
		if text == "" {
			return sqlite.ClaudeConversationMessage{}, false
		}
		return sqlite.ClaudeConversationMessage{
			ID:        firstNonEmptyString(payload["id"]),
			Role:      "assistant",
			Content:   "[thinking]\n" + text,
			Timestamp: strings.TrimSpace(timestamp),
			Kind:      itemType,
			Parts: []sqlite.NormalizedPart{{
				Type: "thinking",
				Text: text,
			}},
		}, true
	case "function_call", "custom_tool_call", "web_search_call":
		name := strings.TrimSpace(fmt.Sprint(payload["name"]))
		if itemType == "web_search_call" && name == "" {
			name = "web_search"
		}
		argsValue := payload["arguments"]
		if itemType == "custom_tool_call" {
			argsValue = payload["input"]
		}
		if itemType == "web_search_call" {
			argsValue = payload["action"]
		}
		argsText, truncated := previewClaudeStructuredData(argsValue, 1600)
		part := sqlite.NormalizedPart{
			Type:          "tool_call",
			Name:          name,
			ArgsText:      argsText,
			ArgsTruncated: truncated,
		}
		return sqlite.ClaudeConversationMessage{
			ID:        firstNonEmptyString(payload["id"], payload["call_id"]),
			Role:      "assistant",
			Content:   strings.TrimSpace(renderClaudePartText(part)),
			Timestamp: strings.TrimSpace(timestamp),
			Kind:      itemType,
			Parts:     []sqlite.NormalizedPart{part},
		}, true
	case "function_call_output", "custom_tool_call_output":
		text, truncated := previewClaudeStructuredData(payload["output"], 2400)
		if text == "" {
			return sqlite.ClaudeConversationMessage{}, false
		}
		part := sqlite.NormalizedPart{
			Type:      "tool_result",
			Text:      text,
			Truncated: truncated,
		}
		return sqlite.ClaudeConversationMessage{
			ID:        firstNonEmptyString(payload["id"], payload["call_id"]),
			Role:      "tool",
			Content:   strings.TrimSpace(renderClaudePartText(part)),
			Timestamp: strings.TrimSpace(timestamp),
			Kind:      itemType,
			Parts:     []sqlite.NormalizedPart{part},
		}, true
	default:
		return sqlite.ClaudeConversationMessage{}, false
	}
}

func extractCodexSummaryText(value interface{}) string {
	parts := extractCodexContentParts(value)
	return strings.TrimSpace(flattenClaudeParts(parts))
}

func extractCodexContentParts(value interface{}) []sqlite.NormalizedPart {
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
			parts = append(parts, extractCodexContentParts(item)...)
		}
		return parts
	case map[string]interface{}:
		part, ok := extractCodexContentPart(typed)
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

func extractCodexContentPart(block map[string]interface{}) (sqlite.NormalizedPart, bool) {
	rawType := strings.TrimSpace(strings.ToLower(fmt.Sprint(block["type"])))
	switch rawType {
	case "", "input_text", "output_text", "summary_text", "text":
		text := strings.TrimSpace(firstNonEmptyString(block["text"], block["content"]))
		if text == "" {
			return sqlite.NormalizedPart{}, false
		}
		text, truncated := truncateClaudeText(text, 32000)
		return sqlite.NormalizedPart{Type: "text", Text: text, Truncated: truncated}, true
	case "image", "input_image", "output_image", "file", "attachment":
		return sqlite.NormalizedPart{
			Type:     "attachment",
			FileName: firstNonEmptyString(block["file_name"], block["filename"], block["name"]),
			MimeType: firstNonEmptyString(block["mime_type"], block["mimeType"], block["content_type"]),
		}, true
	default:
		preview, truncated := previewClaudeStructuredData(block, 1200)
		if preview == "" {
			return sqlite.NormalizedPart{}, false
		}
		return sqlite.NormalizedPart{Type: rawType, Text: preview, Truncated: truncated}, true
	}
}

func parseCodexAutomationMetadata(path string) map[string]string {
	metadata := map[string]string{
		"id": filepath.Base(filepath.Dir(path)),
	}
	content, ok, err := readTextFile(path)
	if err != nil || !ok {
		return metadata
	}
	section := ""
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
		value := parseCodexStringValue(line[eq+1:])
		switch {
		case section == "" && key == "name":
			metadata["name"] = value
		case section == "" && key == "kind":
			metadata["kind"] = value
		case section == "" && key == "status":
			metadata["status"] = value
		case section == "" && (key == "rrule" || key == "schedule"):
			metadata["schedule"] = value
		case section == "" && key == "prompt":
			metadata["prompt"] = value
		}
	}
	return metadata
}

func readCodexPluginManifest(path string) (sqlite.AgentRecord, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sqlite.AgentRecord{}, false, nil
		}
		return sqlite.AgentRecord{}, false, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return sqlite.AgentRecord{}, false, nil
	}
	name := firstNonEmptyString(payload["name"])
	displayName := name
	if iface, _ := payload["interface"].(map[string]interface{}); iface != nil {
		if candidate := firstNonEmptyString(iface["displayName"]); candidate != "" {
			displayName = candidate
		}
	}
	if displayName == "" {
		displayName = strings.TrimSuffix(filepath.Base(filepath.Dir(path)), ".codex-plugin")
	}
	skills := extractCodexStringList(payload["skills"])
	mcpServers := extractCodexObjectKeys(payload["mcpServers"])
	capabilities := extractCodexStringList(payload["capabilities"])
	lines := []string{"Observed Codex plugin manifest."}
	if version := firstNonEmptyString(payload["version"]); version != "" {
		lines = append(lines, "- Version: "+version)
	}
	if description := firstNonEmptyString(payload["description"]); description != "" {
		lines = append(lines, "- Description: "+description)
	}
	if category := firstNonEmptyString(payload["category"]); category != "" {
		lines = append(lines, "- Category: "+category)
	}
	if len(skills) > 0 {
		lines = append(lines, "- Skills: "+strings.Join(skills, ", "))
	}
	if len(mcpServers) > 0 {
		lines = append(lines, "- MCP servers: "+strings.Join(mcpServers, ", "))
	}
	if len(capabilities) > 0 {
		lines = append(lines, "- Capabilities: "+strings.Join(capabilities, ", "))
	}
	return sqlite.AgentRecord{
		Name:        displayName,
		Content:     strings.Join(lines, "\n"),
		Exactness:   "exact",
		SourcePaths: []string{path},
		Confidence:  1,
		Metadata: map[string]interface{}{
			"name":         name,
			"display_name": displayName,
			"version":      firstNonEmptyString(payload["version"]),
			"description":  firstNonEmptyString(payload["description"]),
			"category":     firstNonEmptyString(payload["category"]),
			"skills":       skills,
			"mcp_servers":  mcpServers,
			"capabilities": capabilities,
		},
	}, true, nil
}

func extractCodexStringList(value interface{}) []string {
	switch typed := value.(type) {
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	default:
		return nil
	}
}

func extractCodexObjectKeys(value interface{}) []string {
	typed, _ := value.(map[string]interface{})
	if len(typed) == 0 {
		return nil
	}
	out := make([]string, 0, len(typed))
	for key := range typed {
		key = strings.TrimSpace(key)
		if key != "" {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
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
