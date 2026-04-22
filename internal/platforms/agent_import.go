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

	"github.com/agi-bar/neudrive/internal/localgitsync"
	"github.com/agi-bar/neudrive/internal/runtimecfg"
	"github.com/agi-bar/neudrive/internal/storage/sqlite"
	"github.com/agi-bar/neudrive/internal/systemskills"
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
			return "", fmt.Errorf("semantic import is currently supported only for codex and claude; use a raw snapshot import for %s", platform)
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
		payload, err := PrepareAgentImportPayload(ctx, cfg, adapter.ID())
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
		payload, err := PrepareAgentImportPayload(ctx, cfg, adapter.ID())
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

func PrepareAgentImportPayload(ctx context.Context, cfg *runtimecfg.CLIConfig, platform string) (sqlite.AgentExportPayload, error) {
	if platform == "codex" {
		payload := sqlite.AgentExportPayload{
			Platform: platform,
			Command:  "local-scan",
			Notes: []string{
				"Codex import uses neuDrive's deterministic local inventory mapping; live agent semantic scan is skipped by default.",
			},
		}
		enriched, _, err := enrichCodexPayload(payload)
		if err != nil {
			return sqlite.AgentExportPayload{}, err
		}
		return enriched, nil
	}

	if platform == "claude-code" {
		payload := sqlite.AgentExportPayload{
			Platform: platform,
			Command:  "local-scan",
		}
		enriched, _, err := enrichClaudePayload(payload)
		if err != nil {
			return sqlite.AgentExportPayload{}, err
		}
		return enriched, nil
	}

	if platform != "claude-code" {
		if err := ensureAgentImportReady(cfg, platform); err != nil {
			return sqlite.AgentExportPayload{}, err
		}
		return runAgentExport(ctx, platform)
	}
	return sqlite.AgentExportPayload{}, fmt.Errorf("agent-mediated import is not supported for %s", platform)
}

func ensureAgentImportReady(cfg *runtimecfg.CLIConfig, platform string) error {
	connection, ok := cfg.Local.Connections[platform]
	if !ok || strings.TrimSpace(connection.Token) == "" {
		return fmt.Errorf("%s is not connected; run `neudrive connect %s` first", platformDisplayName(platform), preferredConnectName(platform))
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
	default:
		return sqlite.AgentExportPayload{}, fmt.Errorf("agent-mediated import is not supported for %s", platform)
	}
}

func runCodexAgentExport(ctx context.Context) (sqlite.AgentExportPayload, error) {
	skillDoc, err := readSystemDoc("/skills/neudrive/SKILL.md")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}
	commandDoc, err := readSystemDoc("/skills/neudrive/commands/export.md")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}
	platformDoc, err := readSystemDoc("/skills/neudrive/references/platforms/codex.md")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}
	portabilityDoc, err := readSystemDoc("/skills/portability/codex/SKILL.md")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}

	tempDir, err := os.MkdirTemp("", "neudrive-codex-export-*")
	if err != nil {
		return sqlite.AgentExportPayload{}, err
	}
	defer os.RemoveAll(tempDir)

	schemaPath := filepath.Join(tempDir, "schema.json")
	outputPath := filepath.Join(tempDir, "neudrive-export.json")
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
	return strings.TrimSpace(fmt.Sprintf(`You are executing the installed neuDrive %s entrypoint.

Follow the umbrella skill and platform portability instructions below. Return only JSON matching the provided schema. Do not wrap the JSON in markdown fences.

<neudrive-skill>
%s
</neudrive-skill>

<neudrive-export-command>
%s
</neudrive-export-command>

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
			"platform":           map[string]interface{}{"type": "string"},
			"command":            map[string]interface{}{"type": "string"},
			"profile_rules":      exportArraySchema([]string{"title", "content", "exactness", "source_paths", "confidence"}),
			"memory_items":       exportArraySchema([]string{"title", "content", "exactness", "source_paths", "confidence"}),
			"projects":           exportArraySchema([]string{"name", "context", "exactness", "source_paths"}),
			"automations":        exportArraySchema([]string{"name", "content", "exactness", "source_paths", "confidence", "metadata"}),
			"tools":              exportArraySchema([]string{"name", "content", "exactness", "source_paths", "confidence", "metadata"}),
			"connections":        exportArraySchema([]string{"name", "content", "exactness", "source_paths", "confidence", "metadata"}),
			"archives":           exportArraySchema([]string{"name", "content", "exactness", "source_paths", "confidence", "metadata"}),
			"unsupported":        exportArraySchema([]string{"name", "content", "exactness", "source_paths", "confidence", "metadata"}),
			"sensitive_findings": claudeSensitiveFindingSchemaArray(),
			"vault_candidates":   claudeVaultCandidateSchemaArray(),
			"notes": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
		},
		"required": []string{"platform", "command", "profile_rules", "memory_items", "projects", "automations", "tools", "connections", "archives", "unsupported", "sensitive_findings", "vault_candidates", "notes"},
	}
}

func claudeInventorySchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"projects":           claudeProjectSchemaArray(),
			"bundles":            claudeBundleSchemaArray(),
			"conversations":      claudeConversationSchemaArray(),
			"files":              claudeFileSchemaArray(),
			"sensitive_findings": claudeSensitiveFindingSchemaArray(),
			"vault_candidates":   claudeVaultCandidateSchemaArray(),
		},
		"required": []string{"projects", "bundles", "conversations", "files", "sensitive_findings", "vault_candidates"},
	}
}

func claudeProjectSchemaArray() map[string]interface{} {
	return map[string]interface{}{
		"type": "array",
		"items": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"name":         map[string]interface{}{"type": "string"},
				"context":      map[string]interface{}{"type": "string"},
				"exactness":    map[string]interface{}{"type": "string"},
				"source_paths": stringArraySchema(),
				"files":        claudeFileSchemaArray(),
			},
			"required": []string{"name", "context", "exactness", "source_paths", "files"},
		},
	}
}

func claudeBundleSchemaArray() map[string]interface{} {
	return map[string]interface{}{
		"type": "array",
		"items": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"name":         map[string]interface{}{"type": "string"},
				"kind":         map[string]interface{}{"type": "string"},
				"description":  map[string]interface{}{"type": "string"},
				"exactness":    map[string]interface{}{"type": "string"},
				"source_paths": stringArraySchema(),
				"files":        claudeFileSchemaArray(),
			},
			"required": []string{"name", "kind", "description", "exactness", "source_paths", "files"},
		},
	}
}

func claudeConversationSchemaArray() map[string]interface{} {
	return map[string]interface{}{
		"type": "array",
		"items": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"name":         map[string]interface{}{"type": "string"},
				"session_id":   map[string]interface{}{"type": "string"},
				"project_name": map[string]interface{}{"type": "string"},
				"summary":      map[string]interface{}{"type": "string"},
				"started_at":   map[string]interface{}{"type": "string"},
				"exactness":    map[string]interface{}{"type": "string"},
				"source_paths": stringArraySchema(),
				"messages": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]interface{}{
							"role":      map[string]interface{}{"type": "string"},
							"content":   map[string]interface{}{"type": "string"},
							"timestamp": map[string]interface{}{"type": "string"},
							"kind":      map[string]interface{}{"type": "string"},
						},
						"required": []string{"role", "content", "timestamp", "kind"},
					},
				},
			},
			"required": []string{"name", "session_id", "project_name", "summary", "started_at", "exactness", "source_paths", "messages"},
		},
	}
}

func claudeFileSchemaArray() map[string]interface{} {
	return map[string]interface{}{
		"type": "array",
		"items": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"path":           map[string]interface{}{"type": "string"},
				"content":        map[string]interface{}{"type": "string"},
				"content_base64": map[string]interface{}{"type": "string"},
				"content_type":   map[string]interface{}{"type": "string"},
				"exactness":      map[string]interface{}{"type": "string"},
				"source_path":    map[string]interface{}{"type": "string"},
				"source_paths":   stringArraySchema(),
			},
			"required": []string{"path", "content", "content_base64", "content_type", "exactness", "source_path", "source_paths"},
		},
	}
}

func claudeSensitiveFindingSchemaArray() map[string]interface{} {
	return map[string]interface{}{
		"type": "array",
		"items": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"title":            map[string]interface{}{"type": "string"},
				"detail":           map[string]interface{}{"type": "string"},
				"severity":         map[string]interface{}{"type": "string"},
				"source_paths":     stringArraySchema(),
				"redacted_example": map[string]interface{}{"type": "string"},
			},
			"required": []string{"title", "detail", "severity", "source_paths", "redacted_example"},
		},
	}
}

func claudeVaultCandidateSchemaArray() map[string]interface{} {
	return map[string]interface{}{
		"type": "array",
		"items": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"scope":        map[string]interface{}{"type": "string"},
				"description":  map[string]interface{}{"type": "string"},
				"source_paths": stringArraySchema(),
			},
			"required": []string{"scope", "description", "source_paths"},
		},
	}
}

func stringArraySchema() map[string]interface{} {
	return map[string]interface{}{
		"type":  "array",
		"items": map[string]interface{}{"type": "string"},
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
				"required":             []string{},
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
	return decodeAgentExportPayloadString(raw, 0)
}

func decodeAgentExportPayloadString(raw string, depth int) (sqlite.AgentExportPayload, error) {
	const maxDecodeDepth = 4
	var payload sqlite.AgentExportPayload
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return payload, fmt.Errorf("invalid json payload")
	}
	if depth > maxDecodeDepth {
		return payload, fmt.Errorf("invalid json payload")
	}
	if err := json.Unmarshal([]byte(raw), &payload); err == nil && isMeaningfulAgentExportPayload(payload) {
		return payload, nil
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &wrapper); err == nil {
		if structured := bytes.TrimSpace(wrapper["structured_output"]); len(structured) > 0 && !bytes.Equal(structured, []byte("null")) {
			return decodeAgentExportPayloadString(string(structured), depth+1)
		}
		if result := bytes.TrimSpace(wrapper["result"]); len(result) > 0 && !bytes.Equal(result, []byte("null")) {
			var resultString string
			if err := json.Unmarshal(result, &resultString); err == nil {
				return decodeAgentExportPayloadString(resultString, depth+1)
			}
			return decodeAgentExportPayloadString(string(result), depth+1)
		}
	}
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
		return decodeAgentExportPayloadString(raw, depth+1)
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return decodeAgentExportPayloadString(raw[start:end+1], depth+1)
	}
	return payload, fmt.Errorf("invalid json payload")
}

func isMeaningfulAgentExportPayload(payload sqlite.AgentExportPayload) bool {
	return strings.TrimSpace(payload.Platform) != "" ||
		strings.TrimSpace(payload.Command) != "" ||
		len(payload.ProfileRules) > 0 ||
		len(payload.MemoryItems) > 0 ||
		len(payload.Projects) > 0 ||
		len(payload.Automations) > 0 ||
		len(payload.Tools) > 0 ||
		len(payload.Connections) > 0 ||
		len(payload.Archives) > 0 ||
		len(payload.Unsupported) > 0 ||
		len(payload.SensitiveFindings) > 0 ||
		len(payload.VaultCandidates) > 0 ||
		payload.Claude != nil ||
		payload.Codex != nil ||
		len(payload.Notes) > 0
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
