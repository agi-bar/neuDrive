package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"gopkg.in/yaml.v3"
)

var (
	ErrEntryNotFound          = errors.New("entry not found")
	ErrOptimisticLockConflict = errors.New("entry version conflict")
	ErrSkillMetadataMalformed = errors.New("skill metadata malformed")
	ErrReadOnlyPath           = errors.New("path is read-only")
)

func entryChecksum(path, content, contentType string, metadata map[string]interface{}) string {
	payload, _ := json.Marshal(map[string]interface{}{
		"path":         path,
		"content":      content,
		"content_type": contentType,
		"metadata":     metadata,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func deletedEntryChecksum(path string, version int64) string {
	payload := []byte(path + "|deleted|" + strconvFormatInt(version))
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func rootChecksum(entries []models.FileTreeEntry) string {
	if len(entries) == 0 {
		return entryChecksum("/", "", "directory", map[string]interface{}{})
	}

	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, entry.Path+":"+entry.Checksum+":"+strconvFormatInt(entry.Version))
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func mergeMetadata(base map[string]interface{}, overlay map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func metadataString(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

type sourceContextKey struct{}

func NormalizeSource(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	if strings.HasPrefix(value, "agent:") {
		value = strings.TrimPrefix(value, "agent:")
	}
	if strings.HasPrefix(value, "platform:") {
		value = strings.TrimPrefix(value, "platform:")
	}

	switch value {
	case "":
		return ""
	case "browser", "dashboard", "manual", "manually", "user", "web":
		return "manual"
	case "upload", "uploaded", "file-upload", "browser-upload":
		return "upload"
	case "gpt", "chatgpt", "chatgpt-apps", "chatgpt-actions", "chatgpt.com", "chat-openai-com", "openai-chatgpt":
		return "chatgpt"
	case "codex", "codex-cli", "codex-mcp-client", "openai-codex":
		return "codex"
	case "cursor-vscode", "cursor-desktop", "cursor-agent":
		return "cursor"
	case "gemini-cli", "gemini-cli-mcp-client":
		return "gemini-cli"
	case "github-copilot", "copilot-chat":
		return "copilot"
	case "tongyi":
		return "qwen"
	case "chatglm", "glm", "bigmodel":
		return "zhipu"
	case "lark":
		return "feishu"
	case "openwebui":
		return "open-webui"
	case "bundle-import", "full-import":
		return "import"
	case "claude-import":
		return "claude"
	case "claude-code", "claudecode":
		return "claude-code"
	case "claude-connectors", "claude-connector":
		return "claude-web"
	case "claude-web", "claudeweb", "claude.ai":
		return "claude-web"
	case "claude":
		return "claude"
	default:
		return value
	}
}

func InferSourceFromTokenName(name string) string {
	value := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(value, "local platform "):
		return NormalizeSource(strings.TrimSpace(strings.TrimPrefix(value, "local platform ")))
	case strings.HasPrefix(value, "platform "):
		return NormalizeSource(strings.TrimSpace(strings.TrimPrefix(value, "platform ")))
	default:
		return ""
	}
}

func IsGenericSource(source string) bool {
	switch NormalizeSource(source) {
	case "", "manual", "upload", "import", "mcp", "agent", "summary", "scheduler", "system", "neudrive", "devices", "roles", "inbox":
		return true
	default:
		return false
	}
}

func ContextWithSource(ctx context.Context, source string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	normalized := NormalizeSource(source)
	if normalized == "" {
		return ctx
	}
	return context.WithValue(ctx, sourceContextKey{}, normalized)
}

func SourceFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(sourceContextKey{}).(string)
	return NormalizeSource(value)
}

func SourceOrDefault(ctx context.Context, fallback string) string {
	if source := SourceFromContext(ctx); source != "" {
		return source
	}
	return NormalizeSource(fallback)
}

func EntrySourceFromMetadata(metadata map[string]interface{}) string {
	if platform := NormalizeSource(metadataString(metadata, "source_platform")); platform != "" {
		return platform
	}
	if source := NormalizeSource(metadataString(metadata, "source")); source != "" {
		return source
	}
	if metadataString(metadata, "capture_mode") == "archive" {
		return "upload"
	}
	return ""
}

func EntrySource(entry *models.FileTreeEntry) string {
	if entry == nil {
		return ""
	}
	return EntrySourceFromMetadata(entry.Metadata)
}

func WithSourceMetadata(metadata map[string]interface{}, source string) map[string]interface{} {
	merged := mergeMetadata(nil, metadata)
	if EntrySourceFromMetadata(merged) != "" {
		return merged
	}
	if normalized := NormalizeSource(source); normalized != "" {
		merged["source"] = normalized
	}
	return merged
}

func WithSourceContextMetadata(metadata map[string]interface{}, ctx context.Context) map[string]interface{} {
	merged := mergeMetadata(nil, metadata)
	source := SourceFromContext(ctx)
	if source == "" {
		return merged
	}
	if metadataString(merged, "source_platform") == "" {
		existingSource := NormalizeSource(metadataString(merged, "source"))
		if existingSource != "" && existingSource != source {
			merged["source_platform"] = source
			return merged
		}
	}
	if EntrySourceFromMetadata(merged) != "" {
		return merged
	}
	merged["source"] = source
	return merged
}

func WithSourcePlatformMetadata(metadata map[string]interface{}, platform string) map[string]interface{} {
	merged := mergeMetadata(nil, metadata)
	if metadataString(merged, "source_platform") != "" {
		return merged
	}
	if normalized := NormalizeSource(platform); normalized != "" {
		merged["source_platform"] = normalized
	}
	return merged
}

func classifyEntryKind(rawPath string, isDirectory bool) string {
	if isDirectory {
		return "directory"
	}

	publicPath := hubpath.NormalizePublic(rawPath)
	switch {
	case publicPath == "/identity/profile.json":
		return "identity"
	case strings.HasPrefix(publicPath, "/memory/profile/"):
		return "memory_profile"
	case strings.HasPrefix(publicPath, "/memory/scratch/"):
		return "memory_scratch"
	case strings.HasPrefix(publicPath, "/projects/") && strings.HasSuffix(publicPath, "/context.md"):
		return "project_context"
	case strings.HasPrefix(publicPath, "/projects/") && strings.HasSuffix(publicPath, "/log.jsonl"):
		return "project_log"
	case strings.HasPrefix(publicPath, "/inbox/") && strings.HasSuffix(publicPath, ".json"):
		return "inbox_message"
	case strings.HasPrefix(publicPath, "/devices/") && strings.HasSuffix(publicPath, "/SKILL.md"):
		return "device_skill"
	case strings.HasPrefix(publicPath, "/devices/") && strings.HasSuffix(publicPath, "/config.json"):
		return "device_config"
	case strings.HasPrefix(publicPath, "/roles/") && strings.HasSuffix(publicPath, "/SKILL.md"):
		return "role_skill"
	case strings.HasSuffix(publicPath, "/SKILL.md"):
		return "skill"
	default:
		return "file"
	}
}

func skillMetadataForPath(rawPath, content string, metadata map[string]interface{}) map[string]interface{} {
	if !strings.HasSuffix(hubpath.NormalizePublic(rawPath), "/SKILL.md") {
		return metadata
	}

	frontmatter, description, err := parseSkillFrontmatter(content)
	if err != nil {
		return mergeMetadata(metadata, map[string]interface{}{
			"description": description,
			"indexed":     false,
			"parse_error": err.Error(),
		})
	}

	summary := map[string]interface{}{
		"description": description,
		"indexed":     true,
	}
	for key, value := range frontmatter {
		summary[key] = value
	}
	if name, ok := summary["name"].(string); !ok || strings.TrimSpace(name) == "" {
		summary["name"] = path.Base(path.Dir(hubpath.NormalizePublic(rawPath)))
	}
	return mergeMetadata(metadata, summary)
}

func SkillMetadataForPath(rawPath, content string, metadata map[string]interface{}) map[string]interface{} {
	return skillMetadataForPath(rawPath, content, metadata)
}

func parseSkillFrontmatter(content string) (map[string]interface{}, string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return map[string]interface{}{}, "", nil
	}

	description := firstMarkdownParagraph(content)
	if !strings.HasPrefix(trimmed, "---\n") && !strings.HasPrefix(trimmed, "---\r\n") {
		return map[string]interface{}{}, description, nil
	}

	lines := strings.Split(content, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return nil, description, ErrSkillMetadataMalformed
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil, description, ErrSkillMetadataMalformed
	}

	fm := strings.Join(lines[1:end], "\n")
	if strings.TrimSpace(fm) == "" {
		return map[string]interface{}{}, description, nil
	}

	var decoded map[string]interface{}
	if err := yaml.Unmarshal([]byte(fm), &decoded); err != nil {
		return nil, description, err
	}

	return normalizeYAMLMap(decoded), description, nil
}

func normalizeYAMLMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		out[key] = normalizeYAMLValue(value)
	}
	return out
}

func normalizeYAMLValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return normalizeYAMLMap(typed)
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, normalizeYAMLValue(item))
		}
		return out
	default:
		return typed
	}
}

func firstMarkdownParagraph(markdown string) string {
	lines := strings.Split(markdown, "\n")
	paragraph := make([]string, 0, 4)
	inFrontmatter := false
	frontmatterClosed := false

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if idx == 0 && trimmed == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			if trimmed == "---" {
				inFrontmatter = false
				frontmatterClosed = true
			}
			continue
		}
		if trimmed == "" {
			if len(paragraph) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") && len(paragraph) == 0 && !frontmatterClosed {
			continue
		}
		if strings.HasPrefix(trimmed, "#") && len(paragraph) == 0 {
			continue
		}
		paragraph = append(paragraph, trimmed)
	}

	return strings.TrimSpace(strings.Join(paragraph, " "))
}

func toStringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func toMap(value interface{}) map[string]interface{} {
	if typed, ok := value.(map[string]interface{}); ok {
		return typed
	}
	return nil
}

func strconvFormatInt(v int64) string {
	return strconv.FormatInt(v, 10)
}
