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

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
)

//go:embed resources
var resourcesFS embed.FS

const currentUserSnapshotPlaceholder = "{{CURRENT_USER_SNAPSHOT}}"

var (
	systemSkillTimestamp = time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	portabilityRoot      = "/skills/portability"
	portabilityPlatforms = []string{"claude", "chatgpt", "codex"}
	platformManifests    = map[string]platformManifest{
		"claude": {
			DisplayName: "Claude",
			SkillName:   "portability/claude",
			Description: "Guide for importing Claude data into AgentHub or restoring AgentHub data into Claude-compatible structures.",
			WhenToUse:   "Use when the user asks to migrate, back up, restore, import, or export Claude data and skills.",
			Tags:        []string{"portability", "migration", "backup", "claude", "agenthub"},
		},
		"chatgpt": {
			DisplayName: "ChatGPT",
			SkillName:   "portability/chatgpt",
			Description: "Guide for importing ChatGPT data into AgentHub or restoring AgentHub data into ChatGPT-compatible structures.",
			WhenToUse:   "Use when the user asks to migrate, back up, restore, import, or export ChatGPT data and platform features.",
			Tags:        []string{"portability", "migration", "backup", "chatgpt", "agenthub"},
		},
		"codex": {
			DisplayName: "Codex",
			SkillName:   "portability/codex",
			Description: "Guide for importing Codex workspace conventions into AgentHub or exporting AgentHub context back into Codex workflows.",
			WhenToUse:   "Use when the user asks to migrate, back up, restore, import, or export Codex projects, prompts, tools, or automations.",
			Tags:        []string{"portability", "migration", "backup", "codex", "agenthub"},
		},
	}
)

type platformManifest struct {
	DisplayName string
	SkillName   string
	Description string
	WhenToUse   string
	Tags        []string
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
	return publicPath == portabilityRoot || strings.HasPrefix(publicPath, portabilityRoot+"/")
}

func IsSkillDocumentPath(rawPath string) bool {
	publicPath := hubpath.NormalizePublic(rawPath)
	return strings.HasSuffix(publicPath, "/SKILL.md") && strings.HasPrefix(publicPath, portabilityRoot+"/")
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
	summaries := make([]models.SkillSummary, 0, len(portabilityPlatforms))
	for _, platform := range portabilityPlatforms {
		manifest := platformManifests[platform]
		summaries = append(summaries, models.SkillSummary{
			Name:          manifest.SkillName,
			Path:          portabilityRoot + "/" + platform + "/SKILL.md",
			Source:        "system",
			ReadOnly:      true,
			Description:   manifest.Description,
			WhenToUse:     manifest.WhenToUse,
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

	switch strings.TrimSuffix(publicPath, "/") {
	case "":
		return nil, false
	case "/":
		return []models.FileTreeEntry{directoryEntry("/skills/")}, true
	case "/skills":
		return []models.FileTreeEntry{directoryEntry(portabilityRoot + "/")}, true
	case portabilityRoot:
		entries := make([]models.FileTreeEntry, 0, len(portabilityPlatforms))
		for _, platform := range portabilityPlatforms {
			entries = append(entries, directoryEntry(portabilityRoot+"/"+platform+"/"))
		}
		return entries, true
	}

	for _, platform := range portabilityPlatforms {
		if strings.TrimSuffix(publicPath, "/") == portabilityRoot+"/"+platform {
			entries := make([]models.FileTreeEntry, 0, 3)
			for _, name := range []string{"SKILL.md", "mapping.json", "examples.md"} {
				entry, ok, err := ReadEntry(path.Join(portabilityRoot, platform, name))
				if err != nil || !ok {
					continue
				}
				entries = append(entries, *entry)
			}
			return entries, true
		}
	}

	return nil, false
}

func ReadEntry(rawPath string) (*models.FileTreeEntry, bool, error) {
	publicPath := hubpath.NormalizePublic(rawPath)
	platform, ok := PlatformFromPath(publicPath)
	if !ok {
		return nil, false, nil
	}

	filename := path.Base(publicPath)
	if filename == "." || filename == "/" {
		return nil, false, nil
	}

	content, err := fs.ReadFile(resourcesFS, resourcePath(platform, filename))
	if err != nil {
		return nil, false, err
	}

	metadata := map[string]interface{}{
		"source":    "system",
		"read_only": true,
	}

	kind := "file"
	mimeType := contentType(filename)
	if filename == "SKILL.md" {
		manifest := platformManifests[platform]
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
	if IsProtectedPath(publicPath) {
		metadata["read_only"] = true
	}
	return models.FileTreeEntry{
		ID:            uuid.Nil,
		UserID:        uuid.Nil,
		Path:          publicPath,
		Kind:          "directory",
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

func resourcePath(platform, filename string) string {
	return path.Join("resources", "portability", platform, filename)
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
		return "Migrate profile and memory first so stable preferences land in AgentHub before project data."
	}
	if projectCount == 0 {
		return "Migrate project context next so workspaces and ongoing tasks have a canonical home in AgentHub."
	}
	return "Migrate knowledge files, tools, and automations next, then review platform-specific portability gaps."
}

func displayName(platform string) string {
	if manifest, ok := platformManifests[platform]; ok {
		return manifest.DisplayName
	}
	return strings.Title(platform)
}
