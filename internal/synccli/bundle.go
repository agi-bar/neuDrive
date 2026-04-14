package synccli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
)

var binaryExtensions = map[string]struct{}{
	".png":   {},
	".jpg":   {},
	".jpeg":  {},
	".gif":   {},
	".pdf":   {},
	".zip":   {},
	".skill": {},
	".bin":   {},
	".ico":   {},
	".woff":  {},
	".woff2": {},
	".ttf":   {},
}

func buildBundle(sourceDir, mode string) (*models.Bundle, error) {
	source := filepath.Clean(sourceDir)
	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("source directory does not exist: %s", sourceDir)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source path is not a directory: %s", sourceDir)
	}
	if mode == "" {
		mode = "merge"
	}
	bundle := &models.Bundle{
		Version:   models.BundleVersionV1,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Source:    "manual",
		Mode:      mode,
		Profile:   map[string]string{},
		Skills:    map[string]models.BundleSkill{},
		Memory:    []models.BundleMemoryItem{},
	}

	entries, err := os.ReadDir(source)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillRoot := filepath.Join(source, entry.Name())
		skill := models.BundleSkill{
			Files:       map[string]string{},
			BinaryFiles: map[string]models.BundleBlobFile{},
		}
		err := filepath.WalkDir(skillRoot, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			relPath, err := filepath.Rel(skillRoot, path)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(relPath)
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if shouldTreatAsBinary(path, data) {
				hash := sha256.Sum256(data)
				skill.BinaryFiles[relPath] = models.BundleBlobFile{
					ContentBase64: base64.StdEncoding.EncodeToString(data),
					ContentType:   detectContentType(path, data),
					SizeBytes:     int64(len(data)),
					SHA256:        hex.EncodeToString(hash[:]),
				}
				return nil
			}
			skill.Files[relPath] = string(data)
			return nil
		})
		if err != nil {
			return nil, err
		}
		if _, ok := skill.Files["SKILL.md"]; !ok {
			return nil, fmt.Errorf("skill %s is missing SKILL.md", entry.Name())
		}
		bundle.Skills[entry.Name()] = skill
	}
	bundle.Stats = calculateBundleStats(*bundle)
	return bundle, nil
}

func applyFiltersToBundle(bundle models.Bundle, filters models.BundleFilters) models.Bundle {
	working := bundle
	includeDomains := stringSet(filters.IncludeDomains)
	if len(includeDomains) > 0 && !includeDomains["profile"] {
		working.Profile = map[string]string{}
	}
	if len(includeDomains) > 0 && !includeDomains["memory"] {
		working.Memory = []models.BundleMemoryItem{}
	}
	if len(includeDomains) > 0 && !includeDomains["skills"] {
		working.Skills = map[string]models.BundleSkill{}
	}
	if working.Skills != nil {
		filtered := map[string]models.BundleSkill{}
		includeSkills := stringSet(filters.IncludeSkills)
		excludeSkills := stringSet(filters.ExcludeSkills)
		for name, skill := range working.Skills {
			if len(includeSkills) > 0 && !includeSkills[name] {
				continue
			}
			if excludeSkills[name] {
				continue
			}
			filtered[name] = skill
		}
		working.Skills = filtered
	}
	working.Stats = calculateBundleStats(working)
	return working
}

func calculateBundleStats(bundle models.Bundle) models.BundleStats {
	stats := models.BundleStats{
		TotalSkills:  len(bundle.Skills),
		ProfileItems: len(bundle.Profile),
		MemoryItems:  len(bundle.Memory),
	}
	for _, content := range bundle.Profile {
		stats.TotalBytes += int64(len([]byte(content)))
	}
	for _, item := range bundle.Memory {
		stats.TotalBytes += int64(len([]byte(item.Content)))
	}
	for _, skill := range bundle.Skills {
		for _, content := range skill.Files {
			stats.TotalFiles++
			stats.TotalBytes += int64(len([]byte(content)))
		}
		for _, blob := range skill.BinaryFiles {
			stats.TotalFiles++
			stats.BinaryFiles++
			if blob.SizeBytes > 0 {
				stats.TotalBytes += blob.SizeBytes
				continue
			}
			decoded, _ := base64.StdEncoding.DecodeString(blob.ContentBase64)
			stats.TotalBytes += int64(len(decoded))
		}
	}
	return stats
}

func printBundleStats(bundle models.Bundle) {
	stats := bundle.Stats
	fmt.Printf(
		"Bundle: %d skills, %d files, %d binary, %d bytes\n",
		stats.TotalSkills,
		stats.TotalFiles,
		stats.BinaryFiles,
		stats.TotalBytes,
	)
}

func buildArchive(bundle models.Bundle, filters models.BundleFilters) ([]byte, *models.BundleArchiveManifest, error) {
	return services.BuildBundleArchive(bundle, filters)
}

func loadBundleFile(path string) (*models.Bundle, *models.BundleArchiveManifest, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, err
	}
	if strings.EqualFold(filepath.Ext(path), ".ndrvz") {
		bundle, manifest, err := services.ParseBundleArchive(data)
		return bundle, manifest, data, err
	}
	var bundle models.Bundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid bundle json %s: %w", path, err)
	}
	return &bundle, nil, nil, nil
}

type diffCounts struct {
	Added     int `json:"added"`
	Removed   int `json:"removed"`
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
}

type diffItem struct {
	Domain  string         `json:"domain"`
	Path    string         `json:"path"`
	Status  string         `json:"status"`
	Kind    string         `json:"kind,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

type bundleDiff struct {
	Equal   bool `json:"equal"`
	Summary struct {
		Skills  diffCounts `json:"skills"`
		Files   diffCounts `json:"files"`
		Profile diffCounts `json:"profile"`
		Memory  diffCounts `json:"memory"`
	} `json:"summary"`
	Differences []diffItem `json:"differences"`
}

type normalizedMemoryKey struct {
	Title     string
	Source    string
	CreatedAt string
	ExpiresAt string
}

type normalizedMemoryItem struct {
	Title     string
	Source    string
	CreatedAt string
	ExpiresAt string
	Content   string
}

type normalizedFile struct {
	Kind        string
	Content     string
	SHA256      string
	SizeBytes   int64
	ContentType string
}

type normalizedBundle struct {
	Profile map[string]string
	Memory  map[normalizedMemoryKey][]normalizedMemoryItem
	Skills  map[string]map[string]normalizedFile
}

func compareBundles(left, right models.Bundle, filters models.BundleFilters) bundleDiff {
	lb := normalizeBundleForDiff(left, filters)
	rb := normalizeBundleForDiff(right, filters)
	var result bundleDiff
	result.Equal = true

	profileKeys := unionStringKeys(lb.Profile, rb.Profile)
	for _, category := range profileKeys {
		leftValue, leftOK := lb.Profile[category]
		rightValue, rightOK := rb.Profile[category]
		path := "/memory/profile/" + category + ".md"
		switch {
		case !leftOK:
			result.Equal = false
			result.Summary.Profile.Added++
			result.Differences = append(result.Differences, diffItem{Domain: "profile", Path: path, Status: "added", Kind: "profile"})
		case !rightOK:
			result.Equal = false
			result.Summary.Profile.Removed++
			result.Differences = append(result.Differences, diffItem{Domain: "profile", Path: path, Status: "removed", Kind: "profile"})
		case leftValue == rightValue:
			result.Summary.Profile.Unchanged++
		default:
			result.Equal = false
			result.Summary.Profile.Changed++
			result.Differences = append(result.Differences, diffItem{
				Domain: "profile",
				Path:   path,
				Status: "changed",
				Kind:   "profile",
				Details: map[string]any{
					"left_bytes":  len([]byte(leftValue)),
					"right_bytes": len([]byte(rightValue)),
				},
			})
		}
	}

	memoryKeys := unionMemoryKeys(lb.Memory, rb.Memory)
	for _, key := range memoryKeys {
		leftItems := lb.Memory[key]
		rightItems := rb.Memory[key]
		groupPath := memoryGroupPath(key)
		switch {
		case len(leftItems) == 0:
			result.Equal = false
			result.Summary.Memory.Added += len(rightItems)
			for _, item := range rightItems {
				result.Differences = append(result.Differences, diffItem{Domain: "memory", Path: memoryPath(item), Status: "added", Kind: "memory"})
			}
		case len(rightItems) == 0:
			result.Equal = false
			result.Summary.Memory.Removed += len(leftItems)
			for _, item := range leftItems {
				result.Differences = append(result.Differences, diffItem{Domain: "memory", Path: memoryPath(item), Status: "removed", Kind: "memory"})
			}
		case equalMemoryItems(leftItems, rightItems):
			result.Summary.Memory.Unchanged += len(leftItems)
		default:
			result.Equal = false
			count := len(leftItems)
			if len(rightItems) > count {
				count = len(rightItems)
			}
			result.Summary.Memory.Changed += count
			result.Differences = append(result.Differences, diffItem{
				Domain: "memory",
				Path:   groupPath,
				Status: "changed",
				Kind:   "memory",
				Details: map[string]any{
					"left_items":  len(leftItems),
					"right_items": len(rightItems),
				},
			})
		}
	}

	skillNames := unionSkillKeys(lb.Skills, rb.Skills)
	for _, skillName := range skillNames {
		leftFiles, leftOK := lb.Skills[skillName]
		rightFiles, rightOK := rb.Skills[skillName]
		skillStatus := "unchanged"
		switch {
		case !leftOK:
			result.Equal = false
			result.Summary.Skills.Added++
			skillStatus = "added"
		case !rightOK:
			result.Equal = false
			result.Summary.Skills.Removed++
			skillStatus = "removed"
		}
		fileKeys := unionStringKeysFromMaps(leftFiles, rightFiles)
		for _, relPath := range fileKeys {
			path := "/skills/" + skillName + "/" + relPath
			leftEntry, leftExists := leftFiles[relPath]
			rightEntry, rightExists := rightFiles[relPath]
			switch {
			case !leftExists:
				result.Equal = false
				result.Summary.Files.Added++
				result.Differences = append(result.Differences, diffItem{Domain: "skills", Path: path, Status: "added", Kind: rightEntry.Kind})
				if skillStatus == "unchanged" {
					skillStatus = "changed"
				}
			case !rightExists:
				result.Equal = false
				result.Summary.Files.Removed++
				result.Differences = append(result.Differences, diffItem{Domain: "skills", Path: path, Status: "removed", Kind: leftEntry.Kind})
				if skillStatus == "unchanged" {
					skillStatus = "changed"
				}
			case equalNormalizedFile(leftEntry, rightEntry):
				result.Summary.Files.Unchanged++
			default:
				result.Equal = false
				result.Summary.Files.Changed++
				details := map[string]any{}
				if leftEntry.Kind == "binary" && rightEntry.Kind == "binary" {
					details["left_sha256"] = leftEntry.SHA256
					details["right_sha256"] = rightEntry.SHA256
					details["left_size_bytes"] = leftEntry.SizeBytes
					details["right_size_bytes"] = rightEntry.SizeBytes
					details["left_content_type"] = leftEntry.ContentType
					details["right_content_type"] = rightEntry.ContentType
				} else if leftEntry.Kind == "text" && rightEntry.Kind == "text" {
					details["left_bytes"] = leftEntry.SizeBytes
					details["right_bytes"] = rightEntry.SizeBytes
				} else {
					details["left_kind"] = leftEntry.Kind
					details["right_kind"] = rightEntry.Kind
				}
				result.Differences = append(result.Differences, diffItem{
					Domain:  "skills",
					Path:    path,
					Status:  "changed",
					Kind:    maxKind(leftEntry.Kind, rightEntry.Kind),
					Details: details,
				})
				if skillStatus == "unchanged" {
					skillStatus = "changed"
				}
			}
		}
		switch skillStatus {
		case "unchanged":
			result.Summary.Skills.Unchanged++
		case "changed":
			result.Summary.Skills.Changed++
		}
	}

	sort.Slice(result.Differences, func(i, j int) bool {
		if result.Differences[i].Domain != result.Differences[j].Domain {
			return result.Differences[i].Domain < result.Differences[j].Domain
		}
		if result.Differences[i].Status != result.Differences[j].Status {
			return result.Differences[i].Status < result.Differences[j].Status
		}
		return result.Differences[i].Path < result.Differences[j].Path
	})
	return result
}

func renderDiffText(diff bundleDiff, leftLabel, rightLabel string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Diff: %s -> %s\n", leftLabel, rightLabel)
	if diff.Equal {
		fmt.Fprintln(&b, "Equal: yes")
	} else {
		fmt.Fprintln(&b, "Equal: no")
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Summary:")
	fmt.Fprintf(&b, "  skills: added=%d removed=%d changed=%d unchanged=%d\n", diff.Summary.Skills.Added, diff.Summary.Skills.Removed, diff.Summary.Skills.Changed, diff.Summary.Skills.Unchanged)
	fmt.Fprintf(&b, "  files: added=%d removed=%d changed=%d unchanged=%d\n", diff.Summary.Files.Added, diff.Summary.Files.Removed, diff.Summary.Files.Changed, diff.Summary.Files.Unchanged)
	fmt.Fprintf(&b, "  profile: added=%d removed=%d changed=%d unchanged=%d\n", diff.Summary.Profile.Added, diff.Summary.Profile.Removed, diff.Summary.Profile.Changed, diff.Summary.Profile.Unchanged)
	fmt.Fprintf(&b, "  memory: added=%d removed=%d changed=%d unchanged=%d\n", diff.Summary.Memory.Added, diff.Summary.Memory.Removed, diff.Summary.Memory.Changed, diff.Summary.Memory.Unchanged)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Differences:")
	if len(diff.Differences) == 0 {
		fmt.Fprintln(&b, "  none")
		return b.String()
	}
	for _, item := range diff.Differences {
		fmt.Fprintf(&b, "  [%s] %s [%s]", item.Status, item.Path, item.Kind)
		if len(item.Details) > 0 {
			keys := make([]string, 0, len(item.Details))
			for key := range item.Details {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, key := range keys {
				parts = append(parts, fmt.Sprintf("%s=%v", key, item.Details[key]))
			}
			fmt.Fprintf(&b, " (%s)", strings.Join(parts, ", "))
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

func normalizeBundleForDiff(bundle models.Bundle, filters models.BundleFilters) normalizedBundle {
	working := applyFiltersToBundle(bundle, filters)
	normalized := normalizedBundle{
		Profile: map[string]string{},
		Memory:  map[normalizedMemoryKey][]normalizedMemoryItem{},
		Skills:  map[string]map[string]normalizedFile{},
	}
	for category, content := range working.Profile {
		normalized.Profile[category] = content
	}
	for _, item := range working.Memory {
		key := normalizedMemoryKey{
			Title:     item.Title,
			Source:    item.Source,
			CreatedAt: item.CreatedAt,
			ExpiresAt: item.ExpiresAt,
		}
		normalized.Memory[key] = append(normalized.Memory[key], normalizedMemoryItem{
			Title:     item.Title,
			Source:    item.Source,
			CreatedAt: item.CreatedAt,
			ExpiresAt: item.ExpiresAt,
			Content:   item.Content,
		})
	}
	for key, items := range normalized.Memory {
		sort.Slice(items, func(i, j int) bool { return memoryIdentity(items[i]) < memoryIdentity(items[j]) })
		normalized.Memory[key] = items
	}
	for skillName, skill := range working.Skills {
		files := map[string]normalizedFile{}
		for relPath, content := range skill.Files {
			contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(relPath)))
			if strings.HasSuffix(strings.ToLower(relPath), ".md") {
				contentType = "text/markdown"
			}
			if contentType == "" {
				contentType = "text/plain"
			}
			files[relPath] = normalizedFile{
				Kind:        "text",
				Content:     content,
				SizeBytes:   int64(len([]byte(content))),
				ContentType: contentType,
			}
		}
		for relPath, blob := range skill.BinaryFiles {
			data, _ := base64.StdEncoding.DecodeString(blob.ContentBase64)
			hash := blob.SHA256
			if hash == "" {
				sum := sha256.Sum256(data)
				hash = hex.EncodeToString(sum[:])
			}
			size := blob.SizeBytes
			if size == 0 {
				size = int64(len(data))
			}
			files[relPath] = normalizedFile{
				Kind:        "binary",
				SHA256:      hash,
				SizeBytes:   size,
				ContentType: defaultString(blob.ContentType, "application/octet-stream"),
			}
		}
		normalized.Skills[skillName] = files
	}
	return normalized
}

func shouldTreatAsBinary(path string, data []byte) bool {
	if _, ok := binaryExtensions[strings.ToLower(filepath.Ext(path))]; ok {
		return true
	}
	if bytesIndexByte(data, 0) >= 0 {
		return true
	}
	return !utf8.Valid(data)
}

func detectContentType(path string, data []byte) string {
	if byExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); byExt != "" {
		return byExt
	}
	return http.DetectContentType(data)
}

func unionStringKeys(left, right map[string]string) []string {
	keys := map[string]struct{}{}
	for key := range left {
		keys[key] = struct{}{}
	}
	for key := range right {
		keys[key] = struct{}{}
	}
	return sortedKeys(keys)
}

func unionStringKeysFromMaps(left, right map[string]normalizedFile) []string {
	keys := map[string]struct{}{}
	for key := range left {
		keys[key] = struct{}{}
	}
	for key := range right {
		keys[key] = struct{}{}
	}
	return sortedKeys(keys)
}

func unionSkillKeys(left, right map[string]map[string]normalizedFile) []string {
	keys := map[string]struct{}{}
	for key := range left {
		keys[key] = struct{}{}
	}
	for key := range right {
		keys[key] = struct{}{}
	}
	return sortedKeys(keys)
}

func unionMemoryKeys(left, right map[normalizedMemoryKey][]normalizedMemoryItem) []normalizedMemoryKey {
	keys := map[normalizedMemoryKey]struct{}{}
	for key := range left {
		keys[key] = struct{}{}
	}
	for key := range right {
		keys[key] = struct{}{}
	}
	out := make([]normalizedMemoryKey, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool { return memoryGroupPath(out[i]) < memoryGroupPath(out[j]) })
	return out
}

func equalMemoryItems(left, right []normalizedMemoryItem) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if memoryIdentity(left[i]) != memoryIdentity(right[i]) {
			return false
		}
	}
	return true
}

func equalNormalizedFile(left, right normalizedFile) bool {
	switch {
	case left.Kind == "text" && right.Kind == "text":
		return left.Content == right.Content
	case left.Kind == "binary" && right.Kind == "binary":
		return left.SHA256 == right.SHA256 && left.SizeBytes == right.SizeBytes && left.ContentType == right.ContentType
	default:
		return false
	}
}

func memoryIdentity(item normalizedMemoryItem) string {
	data, _ := json.Marshal(item)
	return string(data)
}

func memoryPath(item normalizedMemoryItem) string {
	digest := sha256.Sum256([]byte(memoryIdentity(item)))
	return fmt.Sprintf(
		"/memory/diff/%s/%s-%s-%s.md",
		safeLabel(item.Source, "source"),
		safeLabel(item.CreatedAt, "created"),
		safeLabel(item.Title, "memory"),
		hex.EncodeToString(digest[:])[:10],
	)
}

func memoryGroupPath(key normalizedMemoryKey) string {
	raw := strings.Join([]string{key.Title, key.Source, key.CreatedAt, key.ExpiresAt}, "|")
	digest := sha256.Sum256([]byte(raw))
	return fmt.Sprintf(
		"/memory/diff/%s/%s-%s-%s.md",
		safeLabel(key.Source, "source"),
		safeLabel(key.CreatedAt, "created"),
		safeLabel(key.Title, "memory"),
		hex.EncodeToString(digest[:])[:10],
	)
}

func safeLabel(value, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fallback
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	label := strings.Trim(b.String(), "-")
	if label == "" {
		return fallback
	}
	if len(label) > 48 {
		return label[:48]
	}
	return label
}

func stringSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = true
		}
	}
	return set
}

func sortedKeys(keys map[string]struct{}) []string {
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func maxKind(left, right string) string {
	if left != "" {
		return left
	}
	return right
}

func bytesIndexByte(data []byte, target byte) int {
	for i, b := range data {
		if b == target {
			return i
		}
	}
	return -1
}
