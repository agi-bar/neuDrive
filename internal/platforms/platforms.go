package platforms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/runtimecfg"
	"github.com/agi-bar/agenthub/internal/storage/sqlite"
	"github.com/agi-bar/agenthub/internal/systemskills"
)

const LocalServerName = "agenthub-local"

var lookPath = exec.LookPath

const (
	agenthubSkillName       = "agenthub"
	managedMarkerFile       = ".agenthub-managed.json"
	managedCommandHeader    = "<!-- agenthub-managed:command -->"
	codexEntrypointDir      = "~/.agents/skills/agenthub"
	claudeEntrypointDir     = "~/.claude/skills/agenthub"
	claudeCommandEntrypoint = "~/.claude/commands/agenthub.md"
)

type Source = sqlite.Source

type Status struct {
	ID                  string
	DisplayName         string
	Installed           bool
	Connected           bool
	MCPInstalled        bool
	BinaryPath          string
	ConfigPath          string
	DaemonTarget        string
	EntrypointInstalled bool
	EntrypointType      string
	EntrypointPath      string
	ChatUsage           []string
	AgentMediated       string
	SupportedDomains    []string
	Sources             []Source
}

type Adapter interface {
	ID() string
	DisplayName() string
	Aliases() []string
	SupportedDomains() []string
	EntrypointType() string
	EntrypointPath() string
	ChatUsage() []string
	AgentMediatedSupport() string
	Detect(cfg *runtimecfg.CLIConfig, daemonURL string) Status
	Connect(ctx context.Context, cfg *runtimecfg.CLIConfig, executable, daemonURL string, connection runtimecfg.LocalConnection) (runtimecfg.LocalConnection, error)
	Disconnect(ctx context.Context, cfg *runtimecfg.CLIConfig) error
	DiscoverSources() []Source
}

func Registry() []Adapter {
	return []Adapter{
		&claudeAdapter{},
		&codexAdapter{},
		&geminiAdapter{},
		&cursorAdapter{},
	}
}

func Resolve(name string) (Adapter, error) {
	requested := strings.TrimSpace(strings.ToLower(name))
	for _, adapter := range Registry() {
		if adapter.ID() == requested {
			return adapter, nil
		}
		for _, alias := range adapter.Aliases() {
			if alias == requested {
				return adapter, nil
			}
		}
	}
	return nil, fmt.Errorf("unknown platform %q", name)
}

func AllStatuses(cfg *runtimecfg.CLIConfig, daemonURL string) []Status {
	out := make([]Status, 0, len(Registry()))
	for _, adapter := range Registry() {
		out = append(out, adapter.Detect(cfg, daemonURL))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func ImportIntoLocalHub(ctx context.Context, cfg *runtimecfg.CLIConfig, platform string) (*sqlite.ImportResult, error) {
	adapter, err := Resolve(platform)
	if err != nil {
		return nil, err
	}
	result, _, _, err := importPlatformData(ctx, cfg, adapter.ID(), adapter.DiscoverSources(), nil)
	return result, err
}

func ExportFromLocalHub(ctx context.Context, cfg *runtimecfg.CLIConfig, platform, outputRoot string) (*sqlite.ExportResult, error) {
	adapter, err := Resolve(platform)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(outputRoot) == "" {
		outputRoot, err = defaultExportRoot(adapter.ID())
		if err != nil {
			return nil, err
		}
	} else {
		outputRoot, err = resolveLocalPath(outputRoot)
		if err != nil {
			return nil, err
		}
	}
	var result sqlite.ExportResult
	_, err = localPlatformAPIPostJSON(ctx, cfg.Local.PublicBaseURL, cfg.Local.OwnerToken, "/agent/local/platform/export", map[string]string{
		"platform":    adapter.ID(),
		"output_root": outputRoot,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func EnsureConnection(ctx context.Context, cfg *runtimecfg.CLIConfig, platform, executable, daemonURL string) (runtimecfg.LocalConnection, error) {
	adapter, err := Resolve(platform)
	if err != nil {
		return runtimecfg.LocalConnection{}, err
	}
	if cfg.Local.Connections == nil {
		cfg.Local.Connections = map[string]runtimecfg.LocalConnection{}
	}

	connection := cfg.Local.Connections[adapter.ID()]
	if strings.TrimSpace(connection.Token) == "" {
		var tokenResp models.CreateTokenResponse
		_, err := localPlatformAPIPostJSON(ctx, daemonURL, cfg.Local.OwnerToken, "/agent/local/platform-token", map[string]interface{}{
			"platform":    adapter.ID(),
			"trust_level": models.TrustLevelWork,
		}, &tokenResp)
		if err != nil {
			return runtimecfg.LocalConnection{}, err
		}
		connection.Token = tokenResp.Token
		connection.TokenID = tokenResp.ScopedToken.ID.String()
		connection.TokenPrefix = tokenResp.TokenPrefix
		connection.Scopes = tokenResp.ScopedToken.Scopes
		connection.MaxTrustLevel = tokenResp.ScopedToken.MaxTrustLevel
		connection.ConnectedAt = time.Now().UTC().Format(time.RFC3339)
	}
	connection.LastPlatformURL = strings.TrimRight(daemonURL, "/") + "/mcp"
	updated, err := adapter.Connect(ctx, cfg, executable, daemonURL, connection)
	if err != nil {
		return runtimecfg.LocalConnection{}, err
	}
	cfg.Local.Connections[adapter.ID()] = updated
	return updated, nil
}

func Disconnect(ctx context.Context, cfg *runtimecfg.CLIConfig, platform string) error {
	adapter, err := Resolve(platform)
	if err != nil {
		return err
	}
	connection, ok := cfg.Local.Connections[adapter.ID()]
	if ok && strings.TrimSpace(connection.TokenID) != "" {
		_, _ = localPlatformAPIDelete(ctx, cfg.Local.PublicBaseURL, cfg.Local.OwnerToken, "/agent/local/platform-token/"+url.PathEscape(connection.TokenID), nil)
	}
	if err := adapter.Disconnect(ctx, cfg); err != nil {
		return err
	}
	delete(cfg.Local.Connections, adapter.ID())
	return nil
}

type baseAdapter struct {
	id               string
	displayName      string
	aliases          []string
	command          string
	configPath       string
	entrypointType   string
	entrypointPath   string
	chatUsage        []string
	agentMediated    string
	supportedDomains []string
	sources          []Source
}

func (b *baseAdapter) ID() string                 { return b.id }
func (b *baseAdapter) DisplayName() string        { return b.displayName }
func (b *baseAdapter) Aliases() []string          { return b.aliases }
func (b *baseAdapter) SupportedDomains() []string { return b.supportedDomains }
func (b *baseAdapter) EntrypointType() string     { return b.entrypointType }
func (b *baseAdapter) EntrypointPath() string     { return expandUser(b.entrypointPath) }
func (b *baseAdapter) ChatUsage() []string        { return append([]string{}, b.chatUsage...) }
func (b *baseAdapter) AgentMediatedSupport() string {
	if strings.TrimSpace(b.agentMediated) == "" {
		return "planned"
	}
	return b.agentMediated
}
func (b *baseAdapter) DiscoverSources() []Source { return existingSources(b.sources) }
func (b *baseAdapter) Detect(cfg *runtimecfg.CLIConfig, daemonURL string) Status {
	binPath, _ := lookPath(b.command)
	connection, connected := cfg.Local.Connections[b.id]
	entrypointType := b.entrypointType
	if strings.TrimSpace(connection.EntrypointType) != "" {
		entrypointType = connection.EntrypointType
	}
	entrypointPath := strings.TrimSpace(connection.EntrypointPath)
	if entrypointPath == "" {
		entrypointPath = expandUser(b.entrypointPath)
	}
	entrypointInstalled := false
	if entrypointPath != "" {
		if _, err := os.Stat(entrypointPath); err == nil {
			entrypointInstalled = true
		}
	}
	chatUsage := append([]string{}, b.chatUsage...)
	if len(connection.ChatUsage) > 0 {
		chatUsage = append([]string{}, connection.ChatUsage...)
	}
	return Status{
		ID:                  b.id,
		DisplayName:         b.displayName,
		Installed:           binPath != "",
		Connected:           connected,
		MCPInstalled:        connected,
		BinaryPath:          binPath,
		ConfigPath:          expandUser(b.configPath),
		DaemonTarget:        strings.TrimRight(daemonURL, "/") + "/mcp",
		EntrypointInstalled: entrypointInstalled,
		EntrypointType:      entrypointType,
		EntrypointPath:      entrypointPath,
		ChatUsage:           chatUsage,
		AgentMediated:       b.AgentMediatedSupport(),
		SupportedDomains:    append([]string{}, b.supportedDomains...),
		Sources:             existingSources(b.sources),
	}
}

type claudeAdapter struct{ baseAdapter }
type codexAdapter struct{ baseAdapter }
type geminiAdapter struct{ baseAdapter }
type cursorAdapter struct{ baseAdapter }

func newBaseAdapter(id, displayName, command, configPath, entrypointType, entrypointPath string, aliases []string, chatUsage, domains []string, sources []Source, agentMediated string) baseAdapter {
	return baseAdapter{
		id:               id,
		displayName:      displayName,
		aliases:          aliases,
		command:          command,
		configPath:       configPath,
		entrypointType:   entrypointType,
		entrypointPath:   entrypointPath,
		chatUsage:        append([]string{}, chatUsage...),
		agentMediated:    agentMediated,
		supportedDomains: domains,
		sources:          sources,
	}
}

func (a *claudeAdapter) init() *claudeAdapter {
	if a.id == "" {
		*a = claudeAdapter{newBaseAdapter(
			"claude-code",
			"Claude Code",
			"claude",
			"~/.claude.json",
			"command",
			claudeCommandEntrypoint,
			[]string{"claude"},
			[]string{"/agenthub ls", "/agenthub read profile/preferences", "/agenthub write memory \"Remember this\"", "/agenthub create project demo", "/agenthub import platform claude", "/agenthub token create --kind sync --purpose backup", "/agenthub git init", "/agenthub git pull"},
			[]string{"connections", "skills", "projects", "prompts", "tools", "archives"},
			[]Source{
				{Domain: "connections", Label: "claude.json", Path: expandUser("~/.claude.json")},
				{Domain: "projects", Label: "projects", Path: expandUser("~/.claude/projects"), IsDir: true},
				{Domain: "prompts", Label: "plans", Path: expandUser("~/.claude/plans"), IsDir: true},
				{Domain: "tools", Label: "plugins", Path: expandUser("~/.claude/plugins"), IsDir: true},
			},
			"supported",
		)}
	}
	return a
}

func (a *claudeAdapter) ID() string                 { return a.init().baseAdapter.ID() }
func (a *claudeAdapter) DisplayName() string        { return a.init().baseAdapter.DisplayName() }
func (a *claudeAdapter) Aliases() []string          { return a.init().baseAdapter.Aliases() }
func (a *claudeAdapter) SupportedDomains() []string { return a.init().baseAdapter.SupportedDomains() }
func (a *claudeAdapter) DiscoverSources() []Source  { return a.init().baseAdapter.DiscoverSources() }
func (a *claudeAdapter) Detect(cfg *runtimecfg.CLIConfig, daemonURL string) Status {
	return a.init().baseAdapter.Detect(cfg, daemonURL)
}
func (a *claudeAdapter) Connect(ctx context.Context, cfg *runtimecfg.CLIConfig, executable, daemonURL string, connection runtimecfg.LocalConnection) (runtimecfg.LocalConnection, error) {
	if _, err := lookPath("claude"); err != nil {
		return connection, err
	}
	_ = run(ctx, "claude", "mcp", "remove", "--scope", "user", LocalServerName)
	if err := run(ctx, "claude", "mcp", "add", "--scope", "user", "--transport", "http", LocalServerName, strings.TrimRight(daemonURL, "/")+"/mcp", "--header", "Authorization: Bearer "+connection.Token); err != nil {
		return connection, err
	}
	skillPath, managedPaths, err := installManagedSkill(expandUser(claudeEntrypointDir), "claude-code")
	if err != nil {
		return connection, err
	}
	commandPath, err := installManagedClaudeCommand(expandUser(claudeCommandEntrypoint))
	if err != nil {
		return connection, err
	}
	connection.Transport = "http"
	connection.ConfigPath = expandUser("~/.claude.json")
	connection.EntrypointType = "command"
	connection.EntrypointPath = commandPath
	connection.ManagedPaths = append(managedPaths, commandPath)
	connection.ChatUsage = []string{"/agenthub ls", "/agenthub read profile/preferences", "/agenthub write memory \"Remember this\"", "/agenthub create project demo", "/agenthub import platform claude", "/agenthub token create --kind sync --purpose backup", "/agenthub git init", "/agenthub git pull"}
	_ = skillPath
	return connection, nil
}
func (a *claudeAdapter) Disconnect(ctx context.Context, cfg *runtimecfg.CLIConfig) error {
	for _, target := range managedPathsForPlatform(cfg, a.ID(), expandUser(claudeEntrypointDir), expandUser(claudeCommandEntrypoint)) {
		if err := removeManagedPath(target); err != nil {
			return err
		}
	}
	return run(ctx, "claude", "mcp", "remove", "--scope", "user", LocalServerName)
}

func (a *codexAdapter) init() *codexAdapter {
	if a.id == "" {
		*a = codexAdapter{newBaseAdapter(
			"codex",
			"Codex CLI",
			"codex",
			"~/.codex/config.toml",
			"skill",
			codexEntrypointDir,
			nil,
			[]string{"$agenthub ls", "$agenthub read profile/preferences", "$agenthub write memory \"Remember this\"", "$agenthub create project demo", "$agenthub import platform codex", "$agenthub token create --kind sync --purpose backup", "$agenthub git init", "$agenthub git pull"},
			[]string{"connections", "skills", "profile", "memory", "projects", "automations", "archives"},
			[]Source{
				{Domain: "profile", Label: "config.toml", Path: expandUser("~/.codex/config.toml")},
				{Domain: "profile", Label: "AGENTS.md", Path: expandUser("~/.codex/AGENTS.md")},
				{Domain: "skills", Label: "skills", Path: expandUser("~/.agents/skills"), IsDir: true},
				{Domain: "automations", Label: "automations", Path: expandUser("~/.codex/automations"), IsDir: true},
				{Domain: "projects", Label: "sessions", Path: expandUser("~/.codex/sessions"), IsDir: true},
				{Domain: "archives", Label: "archived_sessions", Path: expandUser("~/.codex/archived_sessions"), IsDir: true},
			},
			"supported",
		)}
	}
	return a
}

func (a *codexAdapter) ID() string                 { return a.init().baseAdapter.ID() }
func (a *codexAdapter) DisplayName() string        { return a.init().baseAdapter.DisplayName() }
func (a *codexAdapter) Aliases() []string          { return a.init().baseAdapter.Aliases() }
func (a *codexAdapter) SupportedDomains() []string { return a.init().baseAdapter.SupportedDomains() }
func (a *codexAdapter) DiscoverSources() []Source  { return a.init().baseAdapter.DiscoverSources() }
func (a *codexAdapter) Detect(cfg *runtimecfg.CLIConfig, daemonURL string) Status {
	return a.init().baseAdapter.Detect(cfg, daemonURL)
}
func (a *codexAdapter) Connect(ctx context.Context, cfg *runtimecfg.CLIConfig, executable, daemonURL string, connection runtimecfg.LocalConnection) (runtimecfg.LocalConnection, error) {
	if _, err := lookPath("codex"); err != nil {
		return connection, err
	}
	_ = run(ctx, "codex", "mcp", "remove", LocalServerName)
	if err := run(ctx,
		"codex", "mcp", "add", LocalServerName,
		"--env", "AGENTHUB_TOKEN="+connection.Token,
		"--env", "AGENTHUB_STORAGE="+cfg.Local.Storage,
		"--env", "AGENTHUB_SQLITE_PATH="+cfg.Local.SQLitePath,
		"--env", "DATABASE_URL="+cfg.Local.DatabaseURL,
		"--env", "JWT_SECRET="+cfg.Local.JWTSecret,
		"--env", "VAULT_MASTER_KEY="+cfg.Local.VaultMasterKey,
		"--env", "PUBLIC_BASE_URL="+cfg.Local.PublicBaseURL,
		"--",
		executable, "mcp", "stdio", "--storage", cfg.Local.Storage, "--sqlite-path", cfg.Local.SQLitePath, "--token-env", "AGENTHUB_TOKEN",
	); err != nil {
		return connection, err
	}
	skillPath, managedPaths, err := installManagedSkill(expandUser(codexEntrypointDir), "codex")
	if err != nil {
		return connection, err
	}
	connection.Transport = "stdio"
	connection.ConfigPath = expandUser("~/.codex/config.toml")
	connection.EntrypointType = "skill"
	connection.EntrypointPath = skillPath
	connection.ManagedPaths = managedPaths
	connection.ChatUsage = []string{"$agenthub ls", "$agenthub read profile/preferences", "$agenthub write memory \"Remember this\"", "$agenthub create project demo", "$agenthub import platform codex", "$agenthub token create --kind sync --purpose backup", "$agenthub git init", "$agenthub git pull"}
	return connection, nil
}
func (a *codexAdapter) Disconnect(ctx context.Context, cfg *runtimecfg.CLIConfig) error {
	for _, target := range managedPathsForPlatform(cfg, a.ID(), expandUser(codexEntrypointDir)) {
		if err := removeManagedPath(target); err != nil {
			return err
		}
	}
	return run(ctx, "codex", "mcp", "remove", LocalServerName)
}

func (a *geminiAdapter) init() *geminiAdapter {
	if a.id == "" {
		*a = geminiAdapter{newBaseAdapter(
			"gemini-cli",
			"Gemini CLI",
			"gemini",
			"~/.gemini/settings.json",
			"",
			"",
			[]string{"gemini"},
			nil,
			[]string{"connections", "profile", "projects", "prompts", "archives"},
			[]Source{
				{Domain: "profile", Label: "settings.json", Path: expandUser("~/.gemini/settings.json")},
				{Domain: "connections", Label: "mcp-oauth-tokens.json", Path: expandUser("~/.gemini/mcp-oauth-tokens.json")},
				{Domain: "projects", Label: "history", Path: expandUser("~/.gemini/history"), IsDir: true},
				{Domain: "projects", Label: "projects.json", Path: expandUser("~/.gemini/projects.json")},
				{Domain: "archives", Label: "tmp", Path: expandUser("~/.gemini/tmp"), IsDir: true},
			},
			"planned",
		)}
	}
	return a
}

func (a *geminiAdapter) ID() string                 { return a.init().baseAdapter.ID() }
func (a *geminiAdapter) DisplayName() string        { return a.init().baseAdapter.DisplayName() }
func (a *geminiAdapter) Aliases() []string          { return a.init().baseAdapter.Aliases() }
func (a *geminiAdapter) SupportedDomains() []string { return a.init().baseAdapter.SupportedDomains() }
func (a *geminiAdapter) DiscoverSources() []Source  { return a.init().baseAdapter.DiscoverSources() }
func (a *geminiAdapter) Detect(cfg *runtimecfg.CLIConfig, daemonURL string) Status {
	return a.init().baseAdapter.Detect(cfg, daemonURL)
}
func (a *geminiAdapter) Connect(ctx context.Context, cfg *runtimecfg.CLIConfig, executable, daemonURL string, connection runtimecfg.LocalConnection) (runtimecfg.LocalConnection, error) {
	if _, err := lookPath("gemini"); err != nil {
		return connection, err
	}
	_ = run(ctx, "gemini", "mcp", "remove", "--scope", "user", LocalServerName)
	if err := run(ctx, "gemini", "mcp", "add", "--scope", "user", "--transport", "http", LocalServerName, strings.TrimRight(daemonURL, "/")+"/mcp", "--header", "Authorization: Bearer "+connection.Token); err != nil {
		return connection, err
	}
	connection.Transport = "http"
	connection.ConfigPath = expandUser("~/.gemini/settings.json")
	return connection, nil
}
func (a *geminiAdapter) Disconnect(ctx context.Context, cfg *runtimecfg.CLIConfig) error {
	return run(ctx, "gemini", "mcp", "remove", "--scope", "user", LocalServerName)
}

func (a *cursorAdapter) init() *cursorAdapter {
	if a.id == "" {
		*a = cursorAdapter{newBaseAdapter(
			"cursor-agent",
			"Cursor Agent",
			"cursor-agent",
			"~/.cursor/mcp.json",
			"",
			"",
			[]string{"cursor"},
			nil,
			[]string{"connections", "skills", "projects", "prompts", "archives"},
			[]Source{
				{Domain: "connections", Label: "mcp.json", Path: expandUser("~/.cursor/mcp.json")},
				{Domain: "skills", Label: "skills-cursor", Path: expandUser("~/.cursor/skills-cursor"), IsDir: true},
				{Domain: "projects", Label: "projects", Path: expandUser("~/.cursor/projects"), IsDir: true},
				{Domain: "prompts", Label: "prompt_history.json", Path: expandUser("~/.cursor/prompt_history.json")},
				{Domain: "archives", Label: "chats", Path: expandUser("~/.cursor/chats"), IsDir: true},
			},
			"planned",
		)}
	}
	return a
}

func (a *cursorAdapter) ID() string                 { return a.init().baseAdapter.ID() }
func (a *cursorAdapter) DisplayName() string        { return a.init().baseAdapter.DisplayName() }
func (a *cursorAdapter) Aliases() []string          { return a.init().baseAdapter.Aliases() }
func (a *cursorAdapter) SupportedDomains() []string { return a.init().baseAdapter.SupportedDomains() }
func (a *cursorAdapter) DiscoverSources() []Source  { return a.init().baseAdapter.DiscoverSources() }
func (a *cursorAdapter) Detect(cfg *runtimecfg.CLIConfig, daemonURL string) Status {
	return a.init().baseAdapter.Detect(cfg, daemonURL)
}
func (a *cursorAdapter) Connect(ctx context.Context, cfg *runtimecfg.CLIConfig, executable, daemonURL string, connection runtimecfg.LocalConnection) (runtimecfg.LocalConnection, error) {
	configPath := expandUser("~/.cursor/mcp.json")
	current := map[string]any{"mcpServers": map[string]any{}}
	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &current)
	}
	servers, _ := current["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers[LocalServerName] = map[string]any{
		"url": strings.TrimRight(daemonURL, "/") + "/mcp",
		"headers": map[string]string{
			"Authorization": "Bearer " + connection.Token,
		},
	}
	current["mcpServers"] = servers
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return connection, err
	}
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return connection, err
	}
	if err := os.WriteFile(configPath, append(data, '\n'), 0o644); err != nil {
		return connection, err
	}
	connection.Transport = "http"
	connection.ConfigPath = configPath
	return connection, nil
}
func (a *cursorAdapter) Disconnect(ctx context.Context, cfg *runtimecfg.CLIConfig) error {
	configPath := expandUser("~/.cursor/mcp.json")
	current := map[string]any{"mcpServers": map[string]any{}}
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &current); err != nil {
			return err
		}
	}
	servers, _ := current["mcpServers"].(map[string]any)
	if servers == nil {
		return nil
	}
	delete(servers, LocalServerName)
	current["mcpServers"] = servers
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, append(data, '\n'), 0o644)
}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, trimmed)
		}
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func existingSources(sources []Source) []Source {
	out := make([]Source, 0, len(sources))
	for _, source := range sources {
		if _, err := os.Stat(source.Path); err == nil {
			out = append(out, source)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Domain == out[j].Domain {
			return out[i].Path < out[j].Path
		}
		return out[i].Domain < out[j].Domain
	})
	return out
}

func expandUser(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

type managedInstallMarker struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Platform    string `json:"platform"`
	GeneratedAt string `json:"generated_at"`
}

func managedPathsForPlatform(cfg *runtimecfg.CLIConfig, platform string, defaults ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(defaults))
	if cfg != nil {
		if connection, ok := cfg.Local.Connections[platform]; ok {
			for _, candidate := range connection.ManagedPaths {
				candidate = strings.TrimSpace(candidate)
				if candidate == "" {
					continue
				}
				if _, ok := seen[candidate]; ok {
					continue
				}
				seen[candidate] = struct{}{}
				out = append(out, candidate)
			}
		}
	}
	for _, candidate := range defaults {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func installManagedSkill(targetDir, platform string) (string, []string, error) {
	files, err := systemskills.ExportSkillFiles(agenthubSkillName)
	if err != nil {
		return "", nil, err
	}
	if info, err := os.Stat(targetDir); err == nil {
		if !info.IsDir() {
			return "", nil, fmt.Errorf("%s exists and is not a directory", targetDir)
		}
		if !isManagedSkillDir(targetDir) {
			return "", nil, fmt.Errorf("%s already exists and is not managed by Agent Hub", targetDir)
		}
		if err := os.RemoveAll(targetDir); err != nil {
			return "", nil, err
		}
	} else if !os.IsNotExist(err) {
		return "", nil, err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", nil, err
	}
	for relPath, content := range files {
		target := filepath.Join(targetDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", nil, err
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return "", nil, err
		}
	}
	markerData, _ := json.MarshalIndent(managedInstallMarker{
		Name:        agenthubSkillName,
		Kind:        "skill",
		Platform:    platform,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, "", "  ")
	if err := os.WriteFile(filepath.Join(targetDir, managedMarkerFile), append(markerData, '\n'), 0o644); err != nil {
		return "", nil, err
	}
	return targetDir, []string{targetDir}, nil
}

func installManagedClaudeCommand(targetPath string) (string, error) {
	if data, err := os.ReadFile(targetPath); err == nil {
		if !strings.HasPrefix(string(data), managedCommandHeader) {
			return "", fmt.Errorf("%s already exists and is not managed by Agent Hub", targetPath)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", err
	}
	content := strings.Join([]string{
		managedCommandHeader,
		"---",
		"description: Route `/agenthub <subcommand>` through the installed Agent Hub skill and MCP surface.",
		"---",
		"",
		"Use the installed `agenthub` skill at `~/.claude/skills/agenthub/SKILL.md`.",
		"",
		"Treat the first argument after `/agenthub` as the subcommand.",
		"Supported subcommands: `ls`, `read`, `write`, `search`, `create`, `log`, `import`, `token`, `stats`, `git`, `export`, `status`, `help`.",
		"Examples: `/agenthub ls`, `/agenthub read profile/preferences`, `/agenthub git init`, `/agenthub git pull`.",
		"Use `/agenthub help` or `/agenthub help import` when the user needs guidance on the command surface.",
		"Use `/agenthub git init` to create the local Git mirror and `/agenthub git pull` to refresh it.",
		"",
		"1. Read `~/.claude/skills/agenthub/SKILL.md`.",
		"2. Read the matching command document under `~/.claude/skills/agenthub/commands/`.",
		"3. Use Agent Hub MCP tools for all Hub reads and writes.",
		"4. Use `~/.claude/skills/agenthub/references/platforms/claude.md` for Claude-specific routing.",
		"",
		"If no subcommand is provided, treat it as `help`.",
	}, "\n")
	if err := os.WriteFile(targetPath, []byte(content+"\n"), 0o644); err != nil {
		return "", err
	}
	return targetPath, nil
}

func isManagedSkillDir(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, managedMarkerFile))
	if err != nil {
		return false
	}
	var marker managedInstallMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return false
	}
	return marker.Name == agenthubSkillName && marker.Kind == "skill"
}

func removeManagedPath(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		if !isManagedSkillDir(target) {
			return fmt.Errorf("%s is not managed by Agent Hub", target)
		}
		return os.RemoveAll(target)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(data), managedCommandHeader) {
		return fmt.Errorf("%s is not managed by Agent Hub", target)
	}
	return os.Remove(target)
}
