package platforms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/agi-bar/agenthub/internal/localgitsync"
	"github.com/agi-bar/agenthub/internal/runtimecfg"
	"github.com/agi-bar/agenthub/internal/storage/sqlite"
	"github.com/agi-bar/agenthub/internal/systemskills"
)

type ImportMode string

const (
	ImportModeAgent ImportMode = "agent"
	ImportModeFiles ImportMode = "files"
	ImportModeAll   ImportMode = "all"
)

type ImportSummary struct {
	Platform string                    `json:"platform"`
	Mode     ImportMode                `json:"mode"`
	Files    *sqlite.ImportResult      `json:"files,omitempty"`
	Agent    *sqlite.AgentImportResult `json:"agent,omitempty"`
	LocalGit *localgitsync.SyncInfo    `json:"local_git_sync,omitempty"`
}

func ParseImportMode(platform, raw string) (ImportMode, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		if supportsAgentMediatedImport(platform) {
			return ImportModeAgent, nil
		}
		return ImportModeFiles, nil
	}
	switch ImportMode(mode) {
	case ImportModeAgent, ImportModeFiles, ImportModeAll:
		if !supportsAgentMediatedImport(platform) && ImportMode(mode) != ImportModeFiles {
			return "", fmt.Errorf("agent-mediated import is currently supported only for codex and claude; use --mode files for %s", platform)
		}
		return ImportMode(mode), nil
	default:
		return "", fmt.Errorf("mode must be one of: agent, files, all")
	}
}

func Import(ctx context.Context, cfg *runtimecfg.CLIConfig, platform, rawMode string) (*ImportSummary, error) {
	adapter, err := Resolve(platform)
	if err != nil {
		return nil, err
	}
	mode, err := ParseImportMode(adapter.ID(), rawMode)
	if err != nil {
		return nil, err
	}
	summary := &ImportSummary{Platform: adapter.ID(), Mode: mode}
	switch mode {
	case ImportModeFiles:
		result, _, syncInfo, err := importPlatformData(ctx, cfg, adapter.ID(), adapter.DiscoverSources(), nil)
		if err != nil {
			return nil, err
		}
		summary.Files = result
		summary.LocalGit = syncInfo
	case ImportModeAgent:
		if err := ensureAgentImportReady(cfg, adapter.ID()); err != nil {
			return nil, err
		}
		payload, err := runAgentExport(ctx, adapter.ID())
		if err != nil {
			return nil, err
		}
		_, result, syncInfo, err := importPlatformData(ctx, cfg, adapter.ID(), nil, &payload)
		if err != nil {
			return nil, err
		}
		summary.Agent = result
		summary.LocalGit = syncInfo
	case ImportModeAll:
		if err := ensureAgentImportReady(cfg, adapter.ID()); err != nil {
			return nil, err
		}
		payload, err := runAgentExport(ctx, adapter.ID())
		if err != nil {
			return nil, err
		}
		fileResult, agentResult, syncInfo, err := importPlatformData(ctx, cfg, adapter.ID(), adapter.DiscoverSources(), &payload)
		if err != nil {
			return nil, err
		}
		summary.Agent = agentResult
		summary.Files = fileResult
		summary.LocalGit = syncInfo
	}
	return summary, nil
}

func ensureAgentImportReady(cfg *runtimecfg.CLIConfig, platform string) error {
	connection, ok := cfg.Local.Connections[platform]
	if !ok || strings.TrimSpace(connection.Token) == "" {
		return fmt.Errorf("%s is not connected; run `agenthub connect %s` first", platformDisplayName(platform), preferredConnectName(platform))
	}
	return nil
}

func ImportSkillsZip(ctx context.Context, cfg *runtimecfg.CLIConfig, platform, archivePath string) (*ImportSummary, error) {
	adapter, err := Resolve(platform)
	if err != nil {
		return nil, err
	}
	if adapter.ID() != "claude-code" {
		return nil, fmt.Errorf("--zip is currently supported only for claude")
	}
	archivePath, err = resolveLocalPath(archivePath)
	if err != nil {
		return nil, err
	}
	var result sqlite.ImportResult
	syncInfo, err := localPlatformAPIPostJSON(ctx, cfg.Local.PublicBaseURL, cfg.Local.OwnerToken, "/agent/local/platform/import-skills-zip", map[string]string{
		"platform":     "claude-web",
		"archive_path": archivePath,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &ImportSummary{
		Platform: adapter.ID(),
		Mode:     ImportModeFiles,
		Files:    &result,
		LocalGit: syncInfo,
	}, nil
}

func importPlatformData(ctx context.Context, cfg *runtimecfg.CLIConfig, platform string, sources []Source, payload *sqlite.AgentExportPayload) (*sqlite.ImportResult, *sqlite.AgentImportResult, *localgitsync.SyncInfo, error) {
	var response struct {
		Files *sqlite.ImportResult      `json:"files,omitempty"`
		Agent *sqlite.AgentImportResult `json:"agent,omitempty"`
	}
	syncInfo, err := localPlatformAPIPostJSON(ctx, cfg.Local.PublicBaseURL, cfg.Local.OwnerToken, "/agent/local/platform/import", map[string]interface{}{
		"platform":      platform,
		"sources":       sources,
		"agent_payload": payload,
	}, &response)
	if err != nil {
		return nil, nil, syncInfo, err
	}
	return response.Files, response.Agent, syncInfo, nil
}

func runAgentExport(ctx context.Context, platform string) (sqlite.AgentExportPayload, error) {
	switch platform {
	case "codex":
		return runCodexAgentExport(ctx)
	case "claude-code":
		return runClaudeAgentExport(ctx)
	default:
		return sqlite.AgentExportPayload{}, fmt.Errorf("agent-mediated import is not supported for %s", platform)
	}
}

func runCodexAgentExport(ctx context.Context) (sqlite.AgentExportPayload, error) {
	skillDoc, err := readSystemDoc("/skills/agenthub/SKILL.md")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}
	commandDoc, err := readSystemDoc("/skills/agenthub/commands/export.md")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}
	platformDoc, err := readSystemDoc("/skills/agenthub/references/platforms/codex.md")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}
	portabilityDoc, err := readSystemDoc("/skills/portability/codex/SKILL.md")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}

	tempDir, err := os.MkdirTemp("", "agenthub-codex-export-*")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}
	defer os.RemoveAll(tempDir)

	schemaPath := filepath.Join(tempDir, "schema.json")
	outputPath := filepath.Join(tempDir, "agenthub-export.json")
	schema, err := json.MarshalIndent(agentExportSchema(), "", "  ")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}
	if err := os.WriteFile(schemaPath, append(schema, '\n'), 0o644); err != nil {
		return sqlite.AgentExportPayload{}, err
	}

	prompt := buildAgentExportPrompt("Codex", "codex-entry-reference", "codex-portability-reference", skillDoc, commandDoc, platformDoc, portabilityDoc)
	cmd := exec.CommandContext(ctx, "codex", "exec", "--skip-git-repo-check", "--output-schema", schemaPath, "--output-last-message", outputPath, prompt)
	cmd.Dir = tempDir
	output, stderr, err := runCommandJSON(cmd)
	if err != nil {
		trimmed := strings.TrimSpace(stderr)
		if trimmed != "" {
			return sqlite.AgentExportPayload{}, fmt.Errorf("codex exec failed: %w: %s", err, trimmed)
		}
		return sqlite.AgentExportPayload{}, fmt.Errorf("codex exec failed: %w", err)
	}

	payloadBytes, err := os.ReadFile(outputPath)
	if err != nil || len(strings.TrimSpace(string(payloadBytes))) == 0 {
		payloadBytes = output
	}
	payload, err := decodeAgentExportPayload(payloadBytes)
	if err != nil {
		return sqlite.AgentExportPayload{}, fmt.Errorf("decode codex export payload: %w", err)
	}
	if strings.TrimSpace(payload.Platform) == "" {
		payload.Platform = "codex"
	}
	if strings.TrimSpace(payload.Command) == "" {
		payload.Command = "export"
	}
	return payload, nil
}

func runClaudeAgentExport(ctx context.Context) (sqlite.AgentExportPayload, error) {
	skillDoc, err := readSystemDoc("/skills/agenthub/SKILL.md")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}
	commandDoc, err := readSystemDoc("/skills/agenthub/commands/export.md")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}
	platformDoc, err := readSystemDoc("/skills/agenthub/references/platforms/claude.md")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}
	portabilityDoc, err := readSystemDoc("/skills/portability/claude/SKILL.md")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}

	tempDir, err := os.MkdirTemp("", "agenthub-claude-export-*")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}
	defer os.RemoveAll(tempDir)

	prompt := buildAgentExportPrompt("Claude Code", "claude-entry-reference", "claude-portability-reference", skillDoc, commandDoc, platformDoc, portabilityDoc)
	cmd := exec.CommandContext(ctx, "claude", "-p", prompt)
	cmd.Dir = tempDir
	output, stderr, err := runCommandJSON(cmd)
	if err != nil {
		trimmed := strings.TrimSpace(stderr)
		if trimmed != "" {
			return sqlite.AgentExportPayload{}, fmt.Errorf("claude -p failed: %w: %s", err, trimmed)
		}
		return sqlite.AgentExportPayload{}, fmt.Errorf("claude -p failed: %w", err)
	}

	payload, err := decodeAgentExportPayload(output)
	if err != nil {
		return sqlite.AgentExportPayload{}, fmt.Errorf("decode claude export payload: %w", err)
	}
	if strings.TrimSpace(payload.Platform) == "" {
		payload.Platform = "claude-code"
	}
	if strings.TrimSpace(payload.Command) == "" {
		payload.Command = "export"
	}
	return payload, nil
}

func readSystemDoc(publicPath string) (string, error) {
	entry, ok, err := systemskills.ReadEntry(publicPath)
	if err != nil {
		return "", err
	}
	if !ok || entry == nil {
		return "", fmt.Errorf("system skill entry not found: %s", publicPath)
	}
	return entry.Content, nil
}

func buildAgentExportPrompt(platformDisplayName, referenceTag, portabilityTag, skillDoc, commandDoc, platformDoc, portabilityDoc string) string {
	return strings.TrimSpace(fmt.Sprintf(`You are executing the installed Agent Hub %s entrypoint.

Follow the umbrella skill and platform portability instructions below. Return only JSON matching the provided schema. Do not wrap the JSON in markdown fences.

<agenthub-skill>
%s
</agenthub-skill>

<agenthub-export-command>
%s
</agenthub-export-command>

<%s>
%s
</%s>

<%s>
%s
</%s>

Capture exact assets when directly observable, and capture derived long-term rules, memory, and project context when you can infer them. Preserve unsupported or partial data under the unsupported/notes fields instead of dropping it.`, platformDisplayName, skillDoc, commandDoc, referenceTag, platformDoc, referenceTag, portabilityTag, portabilityDoc, portabilityTag))
}

func agentExportSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"platform":      map[string]interface{}{"type": "string"},
			"command":       map[string]interface{}{"type": "string"},
			"profile_rules": exportArraySchema([]string{"title", "content", "exactness", "source_paths", "confidence"}),
			"memory_items":  exportArraySchema([]string{"title", "content", "exactness", "source_paths", "confidence"}),
			"projects":      exportArraySchema([]string{"name", "context", "exactness", "source_paths"}),
			"automations":   exportArraySchema([]string{"name", "content", "exactness", "source_paths", "confidence", "metadata"}),
			"tools":         exportArraySchema([]string{"name", "content", "exactness", "source_paths", "confidence", "metadata"}),
			"connections":   exportArraySchema([]string{"name", "content", "exactness", "source_paths", "confidence", "metadata"}),
			"archives":      exportArraySchema([]string{"name", "content", "exactness", "source_paths", "confidence", "metadata"}),
			"unsupported":   exportArraySchema([]string{"name", "content", "exactness", "source_paths", "confidence", "metadata"}),
			"notes": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
		},
		"required": []string{"platform", "command", "profile_rules", "memory_items", "projects", "automations", "tools", "connections", "archives", "unsupported", "notes"},
	}
}

func exportArraySchema(fields []string) map[string]interface{} {
	properties := map[string]interface{}{}
	for _, field := range fields {
		switch field {
		case "source_paths":
			properties[field] = map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			}
		case "confidence":
			properties[field] = map[string]interface{}{"type": "number"}
		case "metadata":
			properties[field] = map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           map[string]interface{}{},
			}
		default:
			properties[field] = map[string]interface{}{"type": "string"}
		}
	}
	return map[string]interface{}{
		"type": "array",
		"items": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           properties,
			"required":             fields,
		},
	}
}

func runCommandJSON(cmd *exec.Cmd) ([]byte, string, error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	return output, stderr.String(), err
}

func decodeAgentExportPayload(payloadBytes []byte) (sqlite.AgentExportPayload, error) {
	raw := strings.TrimSpace(string(payloadBytes))
	var payload sqlite.AgentExportPayload
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		return payload, nil
	}
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			return payload, nil
		}
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(raw[start:end+1]), &payload); err == nil {
			return payload, nil
		}
	}
	return payload, fmt.Errorf("invalid json payload")
}

func supportsAgentMediatedImport(platform string) bool {
	switch platform {
	case "codex", "claude-code":
		return true
	default:
		return false
	}
}

func preferredConnectName(platform string) string {
	switch platform {
	case "claude-code":
		return "claude"
	default:
		return platform
	}
}

func platformDisplayName(platform string) string {
	switch platform {
	case "claude-code":
		return "claude"
	default:
		return platform
	}
}
