package platforms

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/agi-bar/agenthub/internal/localhub"
	"github.com/agi-bar/agenthub/internal/localruntime"
	"github.com/agi-bar/agenthub/internal/systemskills"
)

type ImportMode string

const (
	ImportModeAgent ImportMode = "agent"
	ImportModeFiles ImportMode = "files"
	ImportModeAll   ImportMode = "all"
)

type ImportSummary struct {
	Platform string                      `json:"platform"`
	Mode     ImportMode                  `json:"mode"`
	Files    *localhub.ImportResult      `json:"files,omitempty"`
	Agent    *localhub.AgentImportResult `json:"agent,omitempty"`
}

func ParseImportMode(platform, raw string) (ImportMode, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		if strings.EqualFold(platform, "codex") {
			return ImportModeAgent, nil
		}
		return ImportModeFiles, nil
	}
	switch ImportMode(mode) {
	case ImportModeAgent, ImportModeFiles, ImportModeAll:
		if platform != "codex" && ImportMode(mode) != ImportModeFiles {
			return "", fmt.Errorf("agent-mediated import is currently supported only for codex; use --mode files for %s", platform)
		}
		return ImportMode(mode), nil
	default:
		return "", fmt.Errorf("mode must be one of: agent, files, all")
	}
}

func Import(ctx context.Context, cfg *localruntime.CLIConfig, platform, rawMode string) (*ImportSummary, error) {
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
		result, err := ImportIntoLocalHub(ctx, cfg, adapter.ID())
		if err != nil {
			return nil, err
		}
		summary.Files = result
	case ImportModeAgent:
		result, err := importViaAgent(ctx, cfg, adapter.ID())
		if err != nil {
			return nil, err
		}
		summary.Agent = result
	case ImportModeAll:
		agentResult, err := importViaAgent(ctx, cfg, adapter.ID())
		if err != nil {
			return nil, err
		}
		fileResult, err := ImportIntoLocalHub(ctx, cfg, adapter.ID())
		if err != nil {
			return nil, err
		}
		summary.Agent = agentResult
		summary.Files = fileResult
	}
	return summary, nil
}

func importViaAgent(ctx context.Context, cfg *localruntime.CLIConfig, platform string) (*localhub.AgentImportResult, error) {
	if platform != "codex" {
		return nil, fmt.Errorf("agent-mediated import is currently supported only for codex")
	}
	connection, ok := cfg.Local.Connections[platform]
	if !ok || strings.TrimSpace(connection.Token) == "" {
		return nil, fmt.Errorf("codex is not connected; run `agenthub connect codex` first")
	}
	payload, err := runCodexAgentExport(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := localhub.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer hub.Close()
	return hub.ImportAgentExport(ctx, platform, payload)
}

func runCodexAgentExport(ctx context.Context) (localhub.AgentExportPayload, error) {
	skillDoc, err := readSystemDoc("/skills/agenthub/SKILL.md")
	if err != nil {
		return localhub.AgentExportPayload{}, err
	}
	commandDoc, err := readSystemDoc("/skills/agenthub/commands/export.md")
	if err != nil {
		return localhub.AgentExportPayload{}, err
	}
	platformDoc, err := readSystemDoc("/skills/agenthub/references/platforms/codex.md")
	if err != nil {
		return localhub.AgentExportPayload{}, err
	}
	portabilityDoc, err := readSystemDoc("/skills/portability/codex/SKILL.md")
	if err != nil {
		return localhub.AgentExportPayload{}, err
	}

	tempDir, err := os.MkdirTemp("", "agenthub-codex-export-*")
	if err != nil {
		return localhub.AgentExportPayload{}, err
	}
	defer os.RemoveAll(tempDir)

	schemaPath := filepath.Join(tempDir, "schema.json")
	outputPath := filepath.Join(tempDir, "agenthub-export.json")
	schema, err := json.MarshalIndent(codexExportSchema(), "", "  ")
	if err != nil {
		return localhub.AgentExportPayload{}, err
	}
	if err := os.WriteFile(schemaPath, append(schema, '\n'), 0o644); err != nil {
		return localhub.AgentExportPayload{}, err
	}

	prompt := buildCodexExportPrompt(skillDoc, commandDoc, platformDoc, portabilityDoc)
	cmd := exec.CommandContext(ctx, "codex", "exec", "--skip-git-repo-check", "--output-schema", schemaPath, "--output-last-message", outputPath, prompt)
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			return localhub.AgentExportPayload{}, fmt.Errorf("codex exec failed: %w: %s", err, trimmed)
		}
		return localhub.AgentExportPayload{}, fmt.Errorf("codex exec failed: %w", err)
	}

	payloadBytes, err := os.ReadFile(outputPath)
	if err != nil || len(strings.TrimSpace(string(payloadBytes))) == 0 {
		payloadBytes = output
	}
	var payload localhub.AgentExportPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return localhub.AgentExportPayload{}, fmt.Errorf("decode codex export payload: %w", err)
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

func buildCodexExportPrompt(skillDoc, commandDoc, platformDoc, portabilityDoc string) string {
	return strings.TrimSpace(fmt.Sprintf(`You are executing the installed Agent Hub Codex entrypoint.

Follow the umbrella skill and platform portability instructions below. Return only JSON matching the provided schema. Do not wrap the JSON in markdown fences.

<agenthub-skill>
%s
</agenthub-skill>

<agenthub-export-command>
%s
</agenthub-export-command>

<codex-entry-reference>
%s
</codex-entry-reference>

<codex-portability-reference>
%s
</codex-portability-reference>

Capture exact assets when directly observable, and capture derived long-term rules, memory, and project context when you can infer them. Preserve unsupported or partial data under the unsupported/notes fields instead of dropping it.`, skillDoc, commandDoc, platformDoc, portabilityDoc))
}

func codexExportSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
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
		"required": []string{"platform", "profile_rules", "memory_items", "projects", "automations", "tools", "connections", "archives", "unsupported", "notes"},
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
			properties[field] = map[string]interface{}{"type": "object"}
		default:
			properties[field] = map[string]interface{}{"type": "string"}
		}
	}
	return map[string]interface{}{
		"type": "array",
		"items": map[string]interface{}{
			"type":       "object",
			"properties": properties,
		},
	}
}
