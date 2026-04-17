package services

import (
	pathpkg "path"
	"sort"
	"strings"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
)

const (
	BundleKindSkill        = "skill"
	BundleKindProject      = "project"
	BundleKindConversation = "conversation"

	EntryKindSkillBundle        = "skill_bundle"
	EntryKindProjectBundle      = "project_bundle"
	EntryKindConversationBundle = "conversation_bundle"
)

func IsDirectoryLikeKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "", "directory", EntryKindSkillBundle, EntryKindProjectBundle, EntryKindConversationBundle:
		return true
	default:
		return false
	}
}

func bundleEntryKind(bundleKind string) string {
	switch strings.TrimSpace(bundleKind) {
	case BundleKindSkill:
		return EntryKindSkillBundle
	case BundleKindProject:
		return EntryKindProjectBundle
	case BundleKindConversation:
		return EntryKindConversationBundle
	default:
		return "directory"
	}
}

func BundleEntryKindForMetadata(metadata map[string]interface{}) string {
	return bundleEntryKind(metadataString(metadata, "bundle_kind"))
}

func BundleMetadata(summary models.BundleSummary) map[string]interface{} {
	if strings.TrimSpace(summary.Kind) == "" {
		return nil
	}

	metadata := map[string]interface{}{
		"bundle_kind": summary.Kind,
		"bundle_name": summary.Name,
	}
	if summary.Name != "" {
		metadata["name"] = summary.Name
	}
	if summary.Source != "" {
		metadata["source"] = summary.Source
	}
	if summary.Description != "" {
		metadata["description"] = summary.Description
	}
	if summary.WhenToUse != "" {
		metadata["when_to_use"] = summary.WhenToUse
	}
	if summary.Status != "" {
		metadata["status"] = summary.Status
	}
	if summary.PrimaryPath != "" {
		metadata["bundle_primary_path"] = hubpath.NormalizePublic(summary.PrimaryPath)
	}
	if summary.LogPath != "" {
		metadata["bundle_log_path"] = hubpath.NormalizePublic(summary.LogPath)
	}
	if summary.ReadOnly {
		metadata["read_only"] = true
	}
	if capabilities := uniqueSortedStrings(summary.Capabilities); len(capabilities) > 0 {
		metadata["bundle_capabilities"] = capabilities
	}
	if tags := uniqueSortedStrings(summary.Tags); len(tags) > 0 {
		metadata["tags"] = tags
	}
	if tools := uniqueSortedStrings(summary.AllowedTools); len(tools) > 0 {
		metadata["allowed_tools"] = tools
	}
	if len(summary.Arguments) > 0 {
		metadata["arguments"] = summary.Arguments
	}
	if len(summary.Activation) > 0 {
		metadata["activation"] = summary.Activation
	}
	if summary.MinTrustLevel > 0 {
		metadata["min_trust_level"] = summary.MinTrustLevel
	}
	return metadata
}

func BundleSummaryFromMetadata(rawPath string, metadata map[string]interface{}, fallbackMinTrust int) *models.BundleSummary {
	bundleKind := strings.TrimSpace(metadataString(metadata, "bundle_kind"))
	if bundleKind == "" {
		switch strings.TrimSpace(metadataString(metadata, "bundle_type")) {
		case BundleKindSkill, BundleKindProject, BundleKindConversation:
			bundleKind = strings.TrimSpace(metadataString(metadata, "bundle_type"))
		default:
			return nil
		}
	}

	summary := &models.BundleSummary{
		Kind:          bundleKind,
		Name:          firstNonEmpty(metadataString(metadata, "bundle_name"), metadataString(metadata, "name"), hubpath.BaseName(hubpath.NormalizePublic(rawPath))),
		Path:          strings.TrimSuffix(hubpath.NormalizePublic(rawPath), "/"),
		Source:        EntrySourceFromMetadata(metadata),
		ReadOnly:      metadataBool(metadata, "read_only"),
		Description:   metadataString(metadata, "description"),
		WhenToUse:     metadataString(metadata, "when_to_use"),
		Status:        metadataString(metadata, "status"),
		PrimaryPath:   metadataString(metadata, "bundle_primary_path"),
		LogPath:       metadataString(metadata, "bundle_log_path"),
		Capabilities:  uniqueSortedStrings(toStringSlice(metadata["bundle_capabilities"])),
		AllowedTools:  uniqueSortedStrings(toStringSlice(metadata["allowed_tools"])),
		Tags:          uniqueSortedStrings(toStringSlice(metadata["tags"])),
		Arguments:     cloneStringMap(toMap(metadata["arguments"])),
		Activation:    cloneStringMap(toMap(metadata["activation"])),
		MinTrustLevel: fallbackMinTrust,
	}
	if value, ok := metadata["min_trust_level"].(int); ok && value > 0 {
		summary.MinTrustLevel = value
	}
	if value, ok := metadata["min_trust_level"].(float64); ok && int(value) > 0 {
		summary.MinTrustLevel = int(value)
	}
	return summary
}

func BundleContextFromSummary(summary models.BundleSummary, currentPath string) *models.BundleContext {
	bundlePath := strings.TrimSuffix(hubpath.NormalizePublic(summary.Path), "/")
	if bundlePath == "" {
		return nil
	}
	current := strings.TrimSuffix(hubpath.NormalizePublic(currentPath), "/")
	if current == "" {
		current = bundlePath
	}
	relativePath := ""
	if current != bundlePath {
		prefix := bundlePath + "/"
		if strings.HasPrefix(current, prefix) {
			relativePath = strings.TrimPrefix(current, prefix)
		}
	}

	return &models.BundleContext{
		Kind:          summary.Kind,
		Name:          summary.Name,
		Path:          bundlePath,
		Source:        summary.Source,
		ReadOnly:      summary.ReadOnly,
		Description:   summary.Description,
		WhenToUse:     summary.WhenToUse,
		Status:        summary.Status,
		PrimaryPath:   hubpath.NormalizePublic(summary.PrimaryPath),
		LogPath:       hubpath.NormalizePublic(summary.LogPath),
		Capabilities:  uniqueSortedStrings(summary.Capabilities),
		AllowedTools:  uniqueSortedStrings(summary.AllowedTools),
		Tags:          uniqueSortedStrings(summary.Tags),
		Arguments:     cloneStringMap(summary.Arguments),
		Activation:    cloneStringMap(summary.Activation),
		MinTrustLevel: summary.MinTrustLevel,
		RelativePath:  relativePath,
	}
}

func bundleSummaryForDirectoryPath(
	currentPath string,
	readDirectory func(path string) (*models.FileTreeEntry, error),
	listDirectory func(path string) ([]models.FileTreeEntry, error),
) *models.BundleSummary {
	var entry *models.FileTreeEntry
	if readDirectory != nil {
		if dirEntry, err := readDirectory(currentPath); err == nil && dirEntry != nil && dirEntry.IsDirectory {
			entry = dirEntry
			if summary := BundleSummaryFromMetadata(dirEntry.Path, dirEntry.Metadata, dirEntry.MinTrustLevel); summary != nil {
				return summary
			}
		}
	}

	if listDirectory == nil {
		return nil
	}

	descendants, err := listDirectory(currentPath)
	if err != nil {
		return nil
	}

	dirPath := currentPath
	var dirMetadata map[string]interface{}
	minTrust := 0
	if entry != nil {
		dirPath = entry.Path
		dirMetadata = entry.Metadata
		minTrust = entry.MinTrustLevel
	}

	summary := bundleSummaryFromDescendants(dirPath, dirMetadata, descendants)
	if summary == nil {
		return nil
	}
	if summary.MinTrustLevel <= 0 {
		summary.MinTrustLevel = minTrust
	}
	return summary
}

func BundleContextForPath(
	currentPath string,
	readDirectory func(path string) (*models.FileTreeEntry, error),
	listDirectory func(path string) ([]models.FileTreeEntry, error),
) *models.BundleContext {
	current := strings.TrimSuffix(hubpath.NormalizePublic(currentPath), "/")
	if current == "" || current == "/" {
		return nil
	}
	original := current

	for {
		if summary := bundleSummaryForDirectoryPath(current, readDirectory, listDirectory); summary != nil {
			return BundleContextFromSummary(*summary, original)
		}
		if current == "/" {
			break
		}
		next := pathpkg.Dir(current)
		if next == "." || next == "" || next == current {
			break
		}
		current = next
	}

	return nil
}

func EnrichBundleDirectoryEntry(entry models.FileTreeEntry, descendants []models.FileTreeEntry) models.FileTreeEntry {
	if !entry.IsDirectory {
		return entry
	}

	summary := bundleSummaryFromDescendants(entry.Path, entry.Metadata, descendants)
	if summary == nil {
		return entry
	}

	metadata := mergeMetadata(entry.Metadata, BundleMetadata(*summary))
	entry.Kind = bundleEntryKind(summary.Kind)
	entry.IsDirectory = true
	entry.Content = ""
	entry.ContentType = "directory"
	entry.Metadata = metadata
	entry.Checksum = entryChecksum(hubpath.NormalizePublic(entry.Path), "", "directory", metadata)
	if summary.MinTrustLevel > 0 {
		entry.MinTrustLevel = summary.MinTrustLevel
	}
	return entry
}

func bundleSummaryFromDescendants(dirPath string, dirMetadata map[string]interface{}, descendants []models.FileTreeEntry) *models.BundleSummary {
	if summary := skillBundleSummaryFromDescendants(dirPath, dirMetadata, descendants); summary != nil {
		return summary
	}
	if summary := projectBundleSummaryFromDescendants(dirPath, dirMetadata, descendants); summary != nil {
		return summary
	}
	if summary := conversationBundleSummaryFromDescendants(dirPath, dirMetadata, descendants); summary != nil {
		return summary
	}
	return BundleSummaryFromMetadata(dirPath, dirMetadata, 0)
}

func skillBundleSummaryFromDescendants(dirPath string, dirMetadata map[string]interface{}, descendants []models.FileTreeEntry) *models.BundleSummary {
	publicDir := strings.TrimSuffix(hubpath.NormalizePublic(dirPath), "/")
	if !strings.HasPrefix(publicDir, "/skills/") {
		return nil
	}

	skillPath := publicDir + "/SKILL.md"
	var skillEntry *models.FileTreeEntry
	for idx := range descendants {
		if hubpath.NormalizePublic(descendants[idx].Path) == skillPath {
			skillEntry = &descendants[idx]
			break
		}
	}
	if skillEntry == nil {
		return BundleSummaryFromMetadata(dirPath, dirMetadata, 0)
	}

	skill := entryToSkillSummary(*skillEntry, metadataString(dirMetadata, "source"))
	summary := &models.BundleSummary{
		Kind:          BundleKindSkill,
		Name:          firstNonEmpty(skill.Name, metadataString(dirMetadata, "bundle_name"), hubpath.BaseName(publicDir)),
		Path:          publicDir,
		Source:        firstNonEmpty(skill.Source, EntrySourceFromMetadata(dirMetadata)),
		ReadOnly:      skill.ReadOnly || metadataBool(dirMetadata, "read_only"),
		Description:   firstNonEmpty(skill.Description, metadataString(dirMetadata, "description")),
		WhenToUse:     firstNonEmpty(skill.WhenToUse, metadataString(dirMetadata, "when_to_use")),
		PrimaryPath:   firstNonEmpty(skill.PrimaryPath, skillPath),
		Capabilities:  skillBundleCapabilities(publicDir, descendants),
		AllowedTools:  uniqueSortedStrings(skill.AllowedTools),
		Tags:          uniqueSortedStrings(skill.Tags),
		Arguments:     cloneStringMap(skill.Arguments),
		Activation:    cloneStringMap(skill.Activation),
		MinTrustLevel: maxInt(skill.MinTrustLevel, skillEntry.MinTrustLevel),
	}
	return summary
}

func projectBundleSummaryFromDescendants(dirPath string, dirMetadata map[string]interface{}, descendants []models.FileTreeEntry) *models.BundleSummary {
	publicDir := strings.TrimSuffix(hubpath.NormalizePublic(dirPath), "/")
	if !strings.HasPrefix(publicDir, "/projects/") {
		return nil
	}

	contextPath := publicDir + "/context.md"
	logPath := publicDir + "/log.jsonl"
	var contextEntry *models.FileTreeEntry
	var logEntry *models.FileTreeEntry
	hasArtifacts := false

	for idx := range descendants {
		publicPath := hubpath.NormalizePublic(descendants[idx].Path)
		switch publicPath {
		case contextPath:
			contextEntry = &descendants[idx]
		case logPath:
			logEntry = &descendants[idx]
		default:
			if publicPath != publicDir {
				hasArtifacts = true
			}
		}
	}

	if contextEntry == nil && logEntry == nil {
		return BundleSummaryFromMetadata(dirPath, dirMetadata, 0)
	}

	source := EntrySourceFromMetadata(dirMetadata)
	if source == "" && contextEntry != nil {
		source = EntrySource(contextEntry)
	}
	if source == "" && logEntry != nil {
		source = EntrySource(logEntry)
	}

	status := metadataString(dirMetadata, "status")
	if status == "" && contextEntry != nil {
		status = metadataString(contextEntry.Metadata, "status")
	}
	if status == "" {
		status = "active"
	}

	description := metadataString(dirMetadata, "description")
	if description == "" && contextEntry != nil {
		description = metadataString(contextEntry.Metadata, "description")
	}
	if description == "" && contextEntry != nil {
		description = firstMarkdownParagraph(contextEntry.Content)
	}

	capabilities := []string{}
	if contextEntry != nil {
		capabilities = append(capabilities, "context")
	}
	if logEntry != nil {
		capabilities = append(capabilities, "logs")
	}
	if hasArtifacts {
		capabilities = append(capabilities, "artifacts")
	}

	minTrust := 0
	if contextEntry != nil {
		minTrust = maxInt(minTrust, contextEntry.MinTrustLevel)
	}
	if logEntry != nil {
		minTrust = maxInt(minTrust, logEntry.MinTrustLevel)
	}

	return &models.BundleSummary{
		Kind:          BundleKindProject,
		Name:          firstNonEmpty(metadataString(dirMetadata, "bundle_name"), metadataString(dirMetadata, "project"), pathpkg.Base(publicDir)),
		Path:          publicDir,
		Source:        source,
		Description:   description,
		Status:        status,
		PrimaryPath:   contextPath,
		LogPath:       logPath,
		Capabilities:  uniqueSortedStrings(capabilities),
		MinTrustLevel: minTrust,
	}
}

func conversationBundleSummaryFromDescendants(dirPath string, dirMetadata map[string]interface{}, descendants []models.FileTreeEntry) *models.BundleSummary {
	publicDir := strings.TrimSuffix(hubpath.NormalizePublic(dirPath), "/")
	if !strings.HasPrefix(publicDir, "/conversations/") {
		return nil
	}

	transcriptPath := publicDir + "/conversation.md"
	sidecarPath := publicDir + "/conversation.json"
	claudeResumePath := publicDir + "/resume-claude.md"
	chatGPTResumePath := publicDir + "/resume-chatgpt.md"

	var transcriptEntry *models.FileTreeEntry
	var sidecarEntry *models.FileTreeEntry
	var claudeResumeEntry *models.FileTreeEntry
	var chatGPTResumeEntry *models.FileTreeEntry

	for idx := range descendants {
		publicPath := hubpath.NormalizePublic(descendants[idx].Path)
		switch publicPath {
		case transcriptPath:
			transcriptEntry = &descendants[idx]
		case sidecarPath:
			sidecarEntry = &descendants[idx]
		case claudeResumePath:
			claudeResumeEntry = &descendants[idx]
		case chatGPTResumePath:
			chatGPTResumeEntry = &descendants[idx]
		}
	}

	if transcriptEntry == nil && sidecarEntry == nil && claudeResumeEntry == nil && chatGPTResumeEntry == nil {
		return BundleSummaryFromMetadata(dirPath, dirMetadata, 0)
	}

	source := firstNonEmpty(
		EntrySourceFromMetadata(dirMetadata),
		func() string {
			if transcriptEntry != nil {
				return EntrySource(transcriptEntry)
			}
			return ""
		}(),
		func() string {
			if sidecarEntry != nil {
				return EntrySource(sidecarEntry)
			}
			return ""
		}(),
		conversationSourceFromPath(publicDir),
	)

	name := firstNonEmpty(
		metadataString(dirMetadata, "conversation_title"),
		metadataString(dirMetadata, "bundle_name"),
		func() string {
			if transcriptEntry != nil {
				return firstMarkdownHeading(transcriptEntry.Content)
			}
			return ""
		}(),
		pathpkg.Base(publicDir),
	)

	capabilities := []string{}
	if transcriptEntry != nil {
		capabilities = append(capabilities, "transcript")
	}
	if sidecarEntry != nil {
		capabilities = append(capabilities, "normalized")
	}
	if claudeResumeEntry != nil {
		capabilities = append(capabilities, "resume-claude")
	}
	if chatGPTResumeEntry != nil {
		capabilities = append(capabilities, "resume-chatgpt")
	}

	minTrust := 0
	for _, entry := range []*models.FileTreeEntry{transcriptEntry, sidecarEntry, claudeResumeEntry, chatGPTResumeEntry} {
		if entry != nil {
			minTrust = maxInt(minTrust, entry.MinTrustLevel)
		}
	}

	primaryPath := metadataString(dirMetadata, "bundle_primary_path")
	if primaryPath == "" {
		switch {
		case transcriptEntry != nil:
			primaryPath = transcriptPath
		case sidecarEntry != nil:
			primaryPath = sidecarPath
		case claudeResumeEntry != nil:
			primaryPath = claudeResumePath
		case chatGPTResumeEntry != nil:
			primaryPath = chatGPTResumePath
		}
	}

	return &models.BundleSummary{
		Kind:          BundleKindConversation,
		Name:          name,
		Path:          publicDir,
		Source:        source,
		Description:   metadataString(dirMetadata, "description"),
		PrimaryPath:   primaryPath,
		Capabilities:  uniqueSortedStrings(capabilities),
		MinTrustLevel: minTrust,
	}
}

func conversationSourceFromPath(publicDir string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(publicDir, "/"), "conversations/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

func firstMarkdownHeading(markdown string) string {
	lines := strings.Split(markdown, "\n")
	inFrontmatter := false

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if idx == 0 && trimmed == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			if trimmed == "---" {
				inFrontmatter = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}

	return ""
}

func skillBundleCapabilities(dirPath string, descendants []models.FileTreeEntry) []string {
	publicDir := strings.TrimSuffix(hubpath.NormalizePublic(dirPath), "/")
	capabilities := []string{"instructions"}
	prefixes := map[string]string{
		publicDir + "/assets/":  "assets",
		publicDir + "/prompts/": "prompts",
		publicDir + "/scripts/": "scripts",
		publicDir + "/config/":  "config",
	}

	for _, entry := range descendants {
		publicPath := hubpath.NormalizePublic(entry.Path)
		for prefix, capability := range prefixes {
			if strings.HasPrefix(publicPath, prefix) {
				capabilities = append(capabilities, capability)
			}
		}
		if strings.HasPrefix(publicPath, publicDir+"/") {
			base := pathpkg.Base(publicPath)
			switch base {
			case "config.json", "config.yaml", "config.yml", "config.toml":
				capabilities = append(capabilities, "config")
			}
		}
	}

	return uniqueSortedStrings(capabilities)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func metadataBool(metadata map[string]interface{}, key string) bool {
	if metadata == nil {
		return false
	}
	switch typed := metadata[key].(type) {
	case bool:
		return typed
	case string:
		switch strings.TrimSpace(strings.ToLower(typed)) {
		case "1", "true", "yes":
			return true
		}
	}
	return false
}

func cloneStringMap(in map[string]interface{}) map[string]interface{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func maxInt(left, right int) int {
	if right > left {
		return right
	}
	return left
}
