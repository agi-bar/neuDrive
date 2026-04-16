package systemskills

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
)

//go:embed resources
var resourcesFS embed.FS

const currentUserSnapshotPlaceholder = "{{CURRENT_USER_SNAPSHOT}}"

var (
	systemSkillTimestamp = time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	skillsRoot           = "/skills"
	agentHubRoot         = "/skills/neudrive"
	portabilityRoot      = "/skills/portability"
	portabilityPlatforms = []string{"general", "claude", "chatgpt", "codex"}
	systemSkillOrder     = []string{"neudrive", "portability/general", "portability/claude", "portability/chatgpt", "portability/codex"}
	neudriveManifest     = skillManifest{
		DisplayName: "neuDrive",
		SkillName:   "neudrive",
		Path:        agentHubRoot,
		ResourceDir: "resources/neudrive",
		Description: "Umbrella skill for using the full neuDrive MCP surface from inside supported agent platforms.",
		WhenToUse:   "Use when the user wants to export into neuDrive, import neuDrive data back into a platform, list syncable data, or check neuDrive platform connectivity.",
		Tags:        []string{"neudrive", "mcp", "platforms", "sync", "portability"},
	}
	platformManifests = map[string]skillManifest{
		"general": {
			DisplayName: "General",
			SkillName:   "portability/general",
			Path:        portabilityRoot + "/general",
			ResourceDir: "resources/portability/general",
			Description: "Fallback guide for migrating data from platforms that do not yet have a dedicated NeuDrive portability manual.",
			WhenToUse:   "Use when the user asks to migrate, back up, restore, import, or export platform data and no dedicated portability/<platform> manual exists, or the dedicated manual does not cover the needed surface.",
			Tags:        []string{"portability", "migration", "backup", "general", "neudrive"},
			Platform:    "general",
		},
		"claude": {
			DisplayName: "Claude",
			SkillName:   "portability/claude",
			Path:        portabilityRoot + "/claude",
			ResourceDir: "resources/portability/claude",
			Description: "Guide for importing Claude data into NeuDrive or restoring NeuDrive data into Claude-compatible structures.",
			WhenToUse:   "Use when the user asks to migrate, back up, restore, import, or export Claude data and skills.",
			Tags:        []string{"portability", "migration", "backup", "claude", "neudrive"},
			Platform:    "claude",
		},
		"chatgpt": {
			DisplayName: "ChatGPT",
			SkillName:   "portability/chatgpt",
			Path:        portabilityRoot + "/chatgpt",
			ResourceDir: "resources/portability/chatgpt",
			Description: "Guide for importing ChatGPT data into NeuDrive or restoring NeuDrive data into ChatGPT-compatible structures.",
			WhenToUse:   "Use when the user asks to migrate, back up, restore, import, or export ChatGPT data and platform features.",
			Tags:        []string{"portability", "migration", "backup", "chatgpt", "neudrive"},
			Platform:    "chatgpt",
		},
		"codex": {
			DisplayName: "Codex",
			SkillName:   "portability/codex",
			Path:        portabilityRoot + "/codex",
			ResourceDir: "resources/portability/codex",
			Description: "Guide for importing Codex workspace conventions into NeuDrive or exporting NeuDrive context back into Codex workflows.",
			WhenToUse:   "Use when the user asks to migrate, back up, restore, import, or export Codex projects, prompts, tools, or automations.",
			Tags:        []string{"portability", "migration", "backup", "codex", "neudrive"},
			Platform:    "codex",
		},
	}
)

type skillManifest struct {
	DisplayName string
	SkillName   string
	Path        string
	ResourceDir string
	Description string
	WhenToUse   string
	Tags        []string
	Platform    string
}

type Snapshot struct {
	Connected           string
	ProfileDataPresent  bool
	ProjectsCount       int
	CustomSkillsCount   int
	RecommendedNextStep string
}

type ConnectionLister interface {
	ListByUser(context.Context, uuid.UUID) ([]models.Connection, error)
}

type GrantLister interface {
	ListGrants(context.Context, uuid.UUID) ([]models.OAuthGrantResponse, error)
}

type ProfileLister interface {
	GetProfile(context.Context, uuid.UUID) ([]models.MemoryProfile, error)
}

type ProjectLister interface {
	List(context.Context, uuid.UUID) ([]models.Project, error)
}

type SkillSummaryLister interface {
	ListSkillSummaries(context.Context, uuid.UUID, int) ([]models.SkillSummary, error)
}

type SnapshotDeps struct {
	Connections ConnectionLister
	Grants      GrantLister
	Profiles    ProfileLister
	Projects    ProjectLister
	Skills      SkillSummaryLister
}

func IsProtectedPath(rawPath string) bool {
	publicPath := strings.TrimSuffix(hubpath.NormalizePublic(rawPath), "/")
	return publicPath == agentHubRoot ||
		strings.HasPrefix(publicPath, agentHubRoot+"/") ||
		publicPath == portabilityRoot ||
		strings.HasPrefix(publicPath, portabilityRoot+"/")
}

func IsSkillDocumentPath(rawPath string) bool {
	publicPath := hubpath.NormalizePublic(rawPath)
	return strings.HasSuffix(publicPath, "/SKILL.md") && strings.HasPrefix(publicPath, skillsRoot+"/")
}

func PlatformFromPath(rawPath string) (string, bool) {
	publicPath := hubpath.NormalizePublic(rawPath)
	for _, platform := range portabilityPlatforms {
		prefix := portabilityRoot + "/" + platform + "/"
		if strings.HasPrefix(publicPath, prefix) {
			return platform, true
		}
	}
	return "", false
}

func SkillSummaries() []models.SkillSummary {
	summaries := make([]models.SkillSummary, 0, len(systemSkillOrder))
	for _, key := range systemSkillOrder {
		manifest, ok := systemManifestByKey(key)
		if !ok {
			continue
		}
		summaries = append(summaries, models.SkillSummary{
			Name:          manifest.SkillName,
			Path:          manifest.Path + "/SKILL.md",
			BundlePath:    manifest.Path,
			PrimaryPath:   manifest.Path + "/SKILL.md",
			Source:        "system",
			ReadOnly:      true,
			Description:   manifest.Description,
			WhenToUse:     manifest.WhenToUse,
			Capabilities:  []string{"instructions"},
			Tags:          append([]string{}, manifest.Tags...),
			MinTrustLevel: models.TrustLevelGuest,
		})
	}
	return summaries
}

func ListEntries(rawPath string) ([]models.FileTreeEntry, bool) {
	publicPath := hubpath.NormalizePublic(rawPath)
	if publicPath == "" {
		publicPath = "/"
	}
	if publicPath == "/" {
		return []models.FileTreeEntry{directoryEntry(skillsRoot + "/")}, true
	}

	switch strings.TrimSuffix(publicPath, "/") {
	case "":
		return nil, false
	case skillsRoot:
		return []models.FileTreeEntry{
			directoryEntry(agentHubRoot + "/"),
			directoryEntry(portabilityRoot + "/"),
		}, true
	case portabilityRoot:
		entries := make([]models.FileTreeEntry, 0, len(portabilityPlatforms))
		for _, platform := range portabilityPlatforms {
			entries = append(entries, directoryEntry(portabilityRoot+"/"+platform+"/"))
		}
		return entries, true
	}

	resourceRoot, publicRoot, ok := resourceRootForPath(publicPath)
	if !ok {
		return nil, false
	}
	resourcePath := resourceRoot
	if rel := strings.Trim(strings.TrimPrefix(publicPath, publicRoot), "/"); rel != "" {
		resourcePath = path.Join(resourceRoot, rel)
	}
	items, err := fs.ReadDir(resourcesFS, resourcePath)
	if err != nil {
		return nil, false
	}
	entries := make([]models.FileTreeEntry, 0, len(items))
	for _, item := range items {
		childPath := path.Join(publicPath, item.Name())
		if item.IsDir() {
			entries = append(entries, directoryEntry(strings.TrimSuffix(childPath, "/")+"/"))
			continue
		}
		entry, ok, err := ReadEntry(childPath)
		if err != nil || !ok {
			continue
		}
		entries = append(entries, *entry)
	}
	return entries, true
}

func ReadEntry(rawPath string) (*models.FileTreeEntry, bool, error) {
	publicPath := hubpath.NormalizePublic(rawPath)
	if entry, ok := systemDirectoryEntry(publicPath); ok {
		return &entry, true, nil
	}
	manifest, resourcePathValue, ok := resourceForFile(publicPath)
	if !ok {
		return nil, false, nil
	}

	filename := path.Base(publicPath)
	if filename == "." || filename == "/" {
		return nil, false, nil
	}

	content, err := fs.ReadFile(resourcesFS, resourcePathValue)
	if err != nil {
		return nil, false, err
	}

	metadata := map[string]interface{}{
		"source":    "system",
		"read_only": true,
	}

	kind := "file"
	mimeType := contentType(filename)
	if filename == "SKILL.md" && strings.TrimSuffix(publicPath, "/") == manifest.Path+"/SKILL.md" {
		kind = "skill"
		metadata["name"] = manifest.SkillName
		metadata["description"] = manifest.Description
		metadata["when_to_use"] = manifest.WhenToUse
		metadata["tags"] = append([]string{}, manifest.Tags...)
	}

	entry := &models.FileTreeEntry{
		ID:            uuid.Nil,
		UserID:        uuid.Nil,
		Path:          publicPath,
		Kind:          kind,
		IsDirectory:   false,
		Content:       string(content),
		ContentType:   mimeType,
		Metadata:      metadata,
		Checksum:      checksum(publicPath, string(content), mimeType, metadata),
		Version:       1,
		MinTrustLevel: models.TrustLevelGuest,
		CreatedAt:     systemSkillTimestamp,
		UpdatedAt:     systemSkillTimestamp,
	}
	return entry, true, nil
}

func systemDirectoryEntry(rawPath string) (models.FileTreeEntry, bool) {
	publicPath := strings.TrimSuffix(hubpath.NormalizePublic(rawPath), "/")
	switch publicPath {
	case "", "/":
		return models.FileTreeEntry{}, false
	case skillsRoot, agentHubRoot, portabilityRoot:
		return directoryEntry(publicPath + "/"), true
	}
	for _, platform := range portabilityPlatforms {
		if publicPath == portabilityRoot+"/"+platform {
			return directoryEntry(publicPath + "/"), true
		}
	}

	resourceRoot, publicRoot, ok := resourceRootForPath(publicPath)
	if !ok {
		return models.FileTreeEntry{}, false
	}
	resourcePath := resourceRoot
	if rel := strings.Trim(strings.TrimPrefix(publicPath, publicRoot), "/"); rel != "" {
		resourcePath = path.Join(resourceRoot, rel)
	}
	info, err := fs.Stat(resourcesFS, resourcePath)
	if err != nil || !info.IsDir() {
		return models.FileTreeEntry{}, false
	}
	return directoryEntry(publicPath + "/"), true
}

func BuildSnapshot(ctx context.Context, userID uuid.UUID, trustLevel int, platform string, deps SnapshotDeps) Snapshot {
	snapshot := Snapshot{
		Connected:           "unknown",
		RecommendedNextStep: recommendedNextStep(platform, "unknown", false, 0),
	}

	connectionsAvailable := false
	grantsAvailable := false

	var connections []models.Connection
	if deps.Connections != nil {
		connectionsAvailable = true
		if listed, err := deps.Connections.ListByUser(ctx, userID); err == nil {
			connections = listed
		}
	}

	var grants []models.OAuthGrantResponse
	if deps.Grants != nil {
		grantsAvailable = true
		if listed, err := deps.Grants.ListGrants(ctx, userID); err == nil {
			grants = listed
		}
	}

	if connectionsAvailable || grantsAvailable {
		snapshot.Connected = connectionState(platform, connections, grants)
	}

	if deps.Profiles != nil {
		if profiles, err := deps.Profiles.GetProfile(ctx, userID); err == nil {
			snapshot.ProfileDataPresent = hasMeaningfulProfile(profiles)
		}
	}

	if deps.Projects != nil {
		if projects, err := deps.Projects.List(ctx, userID); err == nil {
			snapshot.ProjectsCount = len(projects)
		}
	}

	if deps.Skills != nil {
		if skills, err := deps.Skills.ListSkillSummaries(ctx, userID, trustLevel); err == nil {
			count := 0
			for _, skill := range skills {
				if skill.Source == "system" {
					continue
				}
				if !strings.HasPrefix(skill.Path, "/skills/") {
					continue
				}
				if strings.HasPrefix(skill.Path, portabilityRoot+"/") {
					continue
				}
				count++
			}
			snapshot.CustomSkillsCount = count
		}
	}

	snapshot.RecommendedNextStep = recommendedNextStep(platform, snapshot.Connected, snapshot.ProfileDataPresent, snapshot.ProjectsCount)
	return snapshot
}

func MaybeRenderEntry(ctx context.Context, userID uuid.UUID, trustLevel int, entry *models.FileTreeEntry, deps SnapshotDeps) *models.FileTreeEntry {
	if entry == nil || !IsSkillDocumentPath(entry.Path) {
		return entry
	}

	platform, ok := PlatformFromPath(entry.Path)
	if !ok {
		return entry
	}

	rendered := RenderSkillDocument(entry.Content, platform, BuildSnapshot(ctx, userID, trustLevel, platform, deps))
	clone := *entry
	clone.Content = rendered
	clone.Checksum = checksum(clone.Path, clone.Content, clone.ContentType, clone.Metadata)
	return &clone
}

func RenderSkillDocument(baseContent string, platform string, snapshot Snapshot) string {
	display := displayName(platform)
	block := []string{
		"## Current User Snapshot",
		"",
		fmt.Sprintf("- Connected to %s: %s", display, snapshot.Connected),
		fmt.Sprintf("- Profile data present: %t", snapshot.ProfileDataPresent),
		fmt.Sprintf("- Projects count: %d", snapshot.ProjectsCount),
		fmt.Sprintf("- Custom skills count: %d", snapshot.CustomSkillsCount),
		fmt.Sprintf("- Recommended next step: %s", snapshot.RecommendedNextStep),
	}
	snapshotSection := strings.Join(block, "\n")

	if strings.Contains(baseContent, currentUserSnapshotPlaceholder) {
		return strings.ReplaceAll(baseContent, currentUserSnapshotPlaceholder, snapshotSection)
	}

	trimmed := strings.TrimRight(baseContent, "\n")
	return trimmed + "\n\n" + snapshotSection + "\n"
}

func directoryEntry(publicPath string) models.FileTreeEntry {
	metadata := map[string]interface{}{
		"source": "system",
	}
	kind := "directory"
	if IsProtectedPath(publicPath) {
		metadata["read_only"] = true
	}
	if manifest, ok := bundleManifestForPath(publicPath); ok {
		kind = "skill_bundle"
		metadata["bundle_kind"] = "skill"
		metadata["bundle_name"] = manifest.SkillName
		metadata["name"] = manifest.SkillName
		metadata["description"] = manifest.Description
		metadata["when_to_use"] = manifest.WhenToUse
		metadata["bundle_primary_path"] = manifest.Path + "/SKILL.md"
		metadata["bundle_capabilities"] = []string{"instructions"}
		metadata["tags"] = append([]string{}, manifest.Tags...)
	}
	return models.FileTreeEntry{
		ID:            uuid.Nil,
		UserID:        uuid.Nil,
		Path:          publicPath,
		Kind:          kind,
		IsDirectory:   true,
		ContentType:   "directory",
		Metadata:      metadata,
		Checksum:      checksum(publicPath, "", "directory", metadata),
		Version:       1,
		MinTrustLevel: models.TrustLevelGuest,
		CreatedAt:     systemSkillTimestamp,
		UpdatedAt:     systemSkillTimestamp,
	}
}

func bundleManifestForPath(rawPath string) (skillManifest, bool) {
	publicPath := strings.TrimSuffix(hubpath.NormalizePublic(rawPath), "/")
	if publicPath == neudriveManifest.Path {
		return neudriveManifest, true
	}
	for _, platform := range portabilityPlatforms {
		manifest, ok := platformManifests[platform]
		if ok && publicPath == manifest.Path {
			return manifest, true
		}
	}
	return skillManifest{}, false
}

func contentType(filename string) string {
	switch path.Ext(filename) {
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	default:
		return "text/plain"
	}
}

func checksum(pathValue, content, contentType string, metadata map[string]interface{}) string {
	payload, _ := json.Marshal(map[string]interface{}{
		"path":         pathValue,
		"content":      content,
		"content_type": contentType,
		"metadata":     metadata,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func connectionState(platform string, connections []models.Connection, grants []models.OAuthGrantResponse) string {
	explicitUnknown := false

	for _, connection := range connections {
		if connectionMatchesPlatform(connection, platform) {
			return "yes"
		}
	}

	for _, grant := range grants {
		match, unknown := grantMatchesPlatform(grant, platform)
		if match {
			return "yes"
		}
		if unknown {
			explicitUnknown = true
		}
	}

	if explicitUnknown {
		return "unknown"
	}
	return "no"
}

func connectionMatchesPlatform(connection models.Connection, platform string) bool {
	name := strings.ToLower(strings.TrimSpace(connection.Name))
	switch platform {
	case "claude":
		return connection.Platform == "claude" || strings.Contains(name, "claude")
	case "chatgpt":
		return connection.Platform == "gpt" || strings.Contains(name, "chatgpt")
	case "codex":
		return strings.Contains(name, "codex")
	default:
		return false
	}
}

func grantMatchesPlatform(grant models.OAuthGrantResponse, platform string) (bool, bool) {
	values := []string{
		strings.ToLower(grant.App.Name),
		strings.ToLower(grant.App.ClientID),
	}
	for _, uri := range grant.App.RedirectURIs {
		values = append(values, strings.ToLower(uri))
	}
	joined := strings.Join(values, " ")
	hostSignals := grantHosts(grant)

	switch platform {
	case "claude":
		if strings.Contains(joined, "claude.ai") || strings.Contains(joined, "claude.com") {
			return true, false
		}
	case "chatgpt":
		for _, host := range hostSignals {
			if strings.Contains(host, "chatgpt.com") || strings.Contains(host, "openai.com") {
				return true, false
			}
		}
	case "codex":
		if strings.Contains(joined, "codex") {
			return true, false
		}
		for _, host := range hostSignals {
			if strings.Contains(host, "openai.com") || strings.Contains(host, "chatgpt.com") {
				return false, true
			}
		}
	}

	return false, false
}

func grantHosts(grant models.OAuthGrantResponse) []string {
	hosts := []string{}
	values := append([]string{grant.App.ClientID}, grant.App.RedirectURIs...)
	for _, value := range values {
		if value == "" {
			continue
		}
		if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
			hosts = append(hosts, strings.ToLower(parsed.Host))
		}
	}
	sort.Strings(hosts)
	return hosts
}

func hasMeaningfulProfile(profiles []models.MemoryProfile) bool {
	for _, profile := range profiles {
		if strings.TrimSpace(profile.Content) != "" {
			return true
		}
	}
	return false
}

func recommendedNextStep(platform, connected string, profilePresent bool, projectCount int) string {
	display := displayName(platform)

	switch connected {
	case "unknown":
		return fmt.Sprintf("Verify the %s connection state or prepare exported materials from %s before migrating more data.", display, display)
	case "no":
		return fmt.Sprintf("Connect %s first or prepare an exported data package from %s.", display, display)
	}

	if !profilePresent {
		return "Migrate profile and memory first so stable preferences land in NeuDrive before project data."
	}
	if projectCount == 0 {
		return "Migrate project context next so workspaces and ongoing tasks have a canonical home in NeuDrive."
	}
	return "Migrate knowledge files, tools, and automations next, then review platform-specific portability gaps."
}

func displayName(platform string) string {
	if manifest, ok := platformManifests[platform]; ok {
		return manifest.DisplayName
	}
	return strings.Title(platform)
}

func systemManifestByKey(key string) (skillManifest, bool) {
	if key == "neudrive" {
		return neudriveManifest, true
	}
	if strings.HasPrefix(key, "portability/") {
		platform := strings.TrimPrefix(key, "portability/")
		manifest, ok := platformManifests[platform]
		return manifest, ok
	}
	return skillManifest{}, false
}

func resourceRootForPath(publicPath string) (string, string, bool) {
	publicPath = strings.TrimSuffix(hubpath.NormalizePublic(publicPath), "/")
	if publicPath == agentHubRoot || strings.HasPrefix(publicPath, agentHubRoot+"/") {
		return neudriveManifest.ResourceDir, neudriveManifest.Path, true
	}
	for _, platform := range portabilityPlatforms {
		manifest := platformManifests[platform]
		if publicPath == manifest.Path || strings.HasPrefix(publicPath, manifest.Path+"/") {
			return manifest.ResourceDir, manifest.Path, true
		}
	}
	return "", "", false
}

func resourceForFile(publicPath string) (skillManifest, string, bool) {
	resourceRoot, publicRoot, ok := resourceRootForPath(publicPath)
	if !ok {
		return skillManifest{}, "", false
	}
	manifest, ok := manifestForRoot(publicRoot)
	if !ok {
		return skillManifest{}, "", false
	}
	rel := strings.Trim(strings.TrimPrefix(hubpath.NormalizePublic(publicPath), publicRoot), "/")
	if rel == "" {
		return skillManifest{}, "", false
	}
	return manifest, path.Join(resourceRoot, rel), true
}

func manifestForRoot(publicRoot string) (skillManifest, bool) {
	if publicRoot == agentHubRoot {
		return neudriveManifest, true
	}
	for _, platform := range portabilityPlatforms {
		manifest := platformManifests[platform]
		if manifest.Path == publicRoot {
			return manifest, true
		}
	}
	return skillManifest{}, false
}

func ExportSkillFiles(skillName string) (map[string]string, error) {
	var manifest skillManifest
	switch strings.TrimSpace(skillName) {
	case neudriveManifest.SkillName:
		manifest = neudriveManifest
	default:
		platform, ok := strings.CutPrefix(strings.TrimSpace(skillName), "portability/")
		if !ok {
			return nil, fmt.Errorf("unknown system skill %q", skillName)
		}
		var exists bool
		manifest, exists = platformManifests[platform]
		if !exists {
			return nil, fmt.Errorf("unknown system skill %q", skillName)
		}
	}

	files := map[string]string{}
	err := fs.WalkDir(resourcesFS, manifest.ResourceDir, func(resourcePath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(resourcesFS, resourcePath)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(resourcePath, manifest.ResourceDir+"/")
		files[rel] = string(data)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
