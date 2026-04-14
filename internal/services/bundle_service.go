package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
)

const (
	bundleModeMerge  = "merge"
	bundleModeMirror = "mirror"
)

type validatedBundleBlob struct {
	data        []byte
	contentType string
}

type validatedBundleSkill struct {
	textFiles   map[string]string
	binaryFiles map[string]validatedBundleBlob
}

type validatedBundleMemoryItem struct {
	content   string
	title     string
	source    string
	createdAt time.Time
	expiresAt *time.Time
}

func normalizeBundleMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", bundleModeMerge:
		return bundleModeMerge
	case bundleModeMirror:
		return bundleModeMirror
	default:
		return ""
	}
}

func (s *ImportService) ImportBundle(ctx context.Context, userID uuid.UUID, bundle models.Bundle) (*models.BundleImportResult, error) {
	if s.fileTree == nil {
		return nil, fmt.Errorf("import.ImportBundle: file tree service not configured")
	}
	if s.memory == nil {
		return nil, fmt.Errorf("import.ImportBundle: memory service not configured")
	}
	if bundle.Version != models.BundleVersionV1 {
		return nil, fmt.Errorf("import.ImportBundle: unsupported bundle version %q", bundle.Version)
	}

	mode := normalizeBundleMode(bundle.Mode)
	if mode == "" {
		return nil, fmt.Errorf("import.ImportBundle: invalid mode %q", bundle.Mode)
	}

	result := &models.BundleImportResult{
		Version: bundle.Version,
		Mode:    mode,
	}

	normalizedProfile := make(map[string]string, len(bundle.Profile))
	for category, content := range bundle.Profile {
		category = strings.TrimSpace(category)
		if category == "" {
			return nil, fmt.Errorf("import.ImportBundle: profile category is required")
		}
		normalizedProfile[category] = content
	}

	validatedMemory := make([]validatedBundleMemoryItem, 0, len(bundle.Memory))
	for idx, item := range bundle.Memory {
		if strings.TrimSpace(item.Content) == "" {
			return nil, fmt.Errorf("import.ImportBundle: memory[%d] content is required", idx)
		}
		createdAt, expiresAt, err := parseBundleMemoryTimes(item)
		if err != nil {
			return nil, fmt.Errorf("import.ImportBundle: memory[%d]: %w", idx, err)
		}
		validatedMemory = append(validatedMemory, validatedBundleMemoryItem{
			content:   item.Content,
			title:     item.Title,
			source:    item.Source,
			createdAt: createdAt,
			expiresAt: expiresAt,
		})
	}

	validatedSkills := make(map[string]validatedBundleSkill, len(bundle.Skills))
	skillNames := make([]string, 0, len(bundle.Skills))
	for skillName, skill := range bundle.Skills {
		if err := validateSlug(skillName, 128); err != nil {
			return nil, fmt.Errorf("import.ImportBundle: invalid skill name %q: %w", skillName, err)
		}

		normalized := validatedBundleSkill{
			textFiles:   make(map[string]string, len(skill.Files)),
			binaryFiles: make(map[string]validatedBundleBlob, len(skill.BinaryFiles)),
		}
		hasSkillDoc := false

		for relPath, content := range skill.Files {
			cleanPath, err := cleanBundleRelativePath(relPath)
			if err != nil {
				return nil, fmt.Errorf("import.ImportBundle: skill %q: %w", skillName, err)
			}
			if _, exists := normalized.textFiles[cleanPath]; exists {
				return nil, fmt.Errorf("import.ImportBundle: skill %q: duplicate file %q", skillName, cleanPath)
			}
			if _, exists := normalized.binaryFiles[cleanPath]; exists {
				return nil, fmt.Errorf("import.ImportBundle: skill %q: file %q declared as both text and binary", skillName, cleanPath)
			}
			normalized.textFiles[cleanPath] = content
			if cleanPath == "SKILL.md" {
				hasSkillDoc = true
			}
		}

		for relPath, blob := range skill.BinaryFiles {
			cleanPath, err := cleanBundleRelativePath(relPath)
			if err != nil {
				return nil, fmt.Errorf("import.ImportBundle: skill %q: %w", skillName, err)
			}
			if cleanPath == "SKILL.md" {
				return nil, fmt.Errorf("import.ImportBundle: skill %q: SKILL.md must be a text file", skillName)
			}
			if _, exists := normalized.textFiles[cleanPath]; exists {
				return nil, fmt.Errorf("import.ImportBundle: skill %q: file %q declared as both text and binary", skillName, cleanPath)
			}
			if _, exists := normalized.binaryFiles[cleanPath]; exists {
				return nil, fmt.Errorf("import.ImportBundle: skill %q: duplicate file %q", skillName, cleanPath)
			}
			data, contentType, err := decodeBundleBlob(cleanPath, blob)
			if err != nil {
				return nil, fmt.Errorf("import.ImportBundle: skill %q: %w", skillName, err)
			}
			normalized.binaryFiles[cleanPath] = validatedBundleBlob{
				data:        data,
				contentType: contentType,
			}
		}

		if !hasSkillDoc {
			return nil, fmt.Errorf("import.ImportBundle: skill %q missing SKILL.md", skillName)
		}

		validatedSkills[skillName] = normalized
		skillNames = append(skillNames, skillName)
	}
	sort.Strings(skillNames)

	for category, content := range normalizedProfile {
		if err := s.memory.UpsertProfile(ctx, userID, category, content, "bundle-import"); err != nil {
			return nil, err
		}
		result.ProfileCategories++
	}

	for _, item := range validatedMemory {
		if _, err := s.memory.ImportScratch(ctx, userID, item.content, item.source, item.title, item.createdAt, item.expiresAt); err != nil {
			return nil, err
		}
		result.MemoryImported++
	}

	for _, skillName := range skillNames {
		skill := validatedSkills[skillName]
		skillMetadata := WithSourceMetadata(nil, bundle.Source)
		if EntrySourceFromMetadata(skillMetadata) == "" {
			skillMetadata = WithSourceMetadata(skillMetadata, "import")
		}
		declared := make(map[string]struct{}, len(skill.textFiles)+len(skill.binaryFiles))

		for relPath, content := range skill.textFiles {
			fullPath := path.Join("/skills", skillName, relPath)
			if _, err := s.fileTree.WriteEntry(ctx, userID, fullPath, content, contentTypeFromExt(relPath), models.FileTreeWriteOptions{
				Metadata:      skillMetadata,
				MinTrustLevel: models.TrustLevelGuest,
			}); err != nil {
				return nil, err
			}
			declared[relPath] = struct{}{}
			result.FilesWritten++
		}

		for relPath, blob := range skill.binaryFiles {
			fullPath := path.Join("/skills", skillName, relPath)
			if _, err := s.fileTree.WriteBinaryEntry(ctx, userID, fullPath, blob.data, blob.contentType, models.FileTreeWriteOptions{
				Metadata:      skillMetadata,
				MinTrustLevel: models.TrustLevelGuest,
			}); err != nil {
				return nil, err
			}
			declared[relPath] = struct{}{}
			result.FilesWritten++
		}

		if mode == bundleModeMirror {
			skillRoot := path.Join("/skills", skillName)
			snapshot, err := s.fileTree.Snapshot(ctx, userID, skillRoot, models.TrustLevelFull)
			if err != nil {
				return nil, err
			}
			for _, entry := range snapshot.Entries {
				if entry.IsDirectory {
					continue
				}
				publicPath := hubpath.NormalizePublic(entry.Path)
				relPath := strings.TrimPrefix(publicPath, strings.TrimSuffix(skillRoot, "/")+"/")
				if _, ok := declared[relPath]; ok {
					continue
				}
				if err := s.fileTree.Delete(ctx, userID, entry.Path); err != nil {
					return nil, err
				}
				result.FilesDeleted++
			}
		}

		result.SkillsWritten++
	}

	return result, nil
}

func (s *ImportService) PreviewBundle(ctx context.Context, userID uuid.UUID, bundle models.Bundle) (*models.BundlePreviewResult, error) {
	if s.fileTree == nil {
		return nil, fmt.Errorf("import.PreviewBundle: file tree service not configured")
	}
	if s.memory == nil {
		return nil, fmt.Errorf("import.PreviewBundle: memory service not configured")
	}

	mode, normalizedProfile, validatedMemory, validatedSkills, skillNames, err := prepareBundlePreview(bundle)
	if err != nil {
		return nil, err
	}

	preview := &models.BundlePreviewResult{
		Version: bundle.Version,
		Mode:    mode,
		Skills:  make(map[string]models.BundleSkillPreview, len(validatedSkills)),
	}

	profileCategories := make([]string, 0, len(normalizedProfile))
	for category := range normalizedProfile {
		profileCategories = append(profileCategories, category)
	}
	sort.Strings(profileCategories)
	for _, category := range profileCategories {
		action, err := s.previewTextPath(ctx, userID, hubpath.ProfilePath(category), normalizedProfile[category], "text/markdown")
		if err != nil {
			return nil, err
		}
		entry := models.BundlePreviewEntry{
			Path:   hubpath.ProfilePath(category),
			Action: action,
			Kind:   "profile",
		}
		preview.Profile = append(preview.Profile, entry)
		applyBundlePreviewAction(&preview.Summary, action)
	}

	for _, item := range validatedMemory {
		scratchPath := importedScratchPath(item)
		existing, _, blobExists, err := s.loadPreviewEntry(ctx, userID, scratchPath)
		if err != nil {
			return nil, err
		}
		action := previewTextAction(existing, blobExists, item.content, "text/markdown")
		entry := models.BundlePreviewEntry{
			Path:   scratchPath,
			Action: action,
			Kind:   "memory",
		}
		preview.Memory = append(preview.Memory, entry)
		applyBundlePreviewAction(&preview.Summary, action)
	}

	for _, skillName := range skillNames {
		skill := validatedSkills[skillName]
		skillRoot := path.Join("/skills", skillName)
		snapshot, err := s.fileTree.Snapshot(ctx, userID, skillRoot, models.TrustLevelFull)
		if err != nil {
			if errors.Is(err, ErrEntryNotFound) {
				snapshot = &models.EntrySnapshot{Path: skillRoot}
			} else {
				return nil, err
			}
		}

		existing := make(map[string]models.FileTreeEntry, len(snapshot.Entries))
		for _, entry := range snapshot.Entries {
			if entry.IsDirectory {
				continue
			}
			publicPath := hubpath.NormalizePublic(entry.Path)
			relPath := strings.TrimPrefix(publicPath, strings.TrimSuffix(skillRoot, "/")+"/")
			existing[relPath] = entry
		}

		skillPreview := models.BundleSkillPreview{}
		declared := make(map[string]struct{}, len(skill.textFiles)+len(skill.binaryFiles))

		textPaths := sortedStringKeys(skill.textFiles)
		for _, relPath := range textPaths {
			declared[relPath] = struct{}{}
			entry, hasEntry := existing[relPath]
			var current *models.FileTreeEntry
			var blobExists bool
			if hasEntry {
				current = &entry
				_, blobExists, err = s.fileTree.ReadBlobByEntryID(ctx, entry.ID)
				if err != nil {
					return nil, err
				}
			}
			action := previewTextAction(current, blobExists, skill.textFiles[relPath], contentTypeFromExt(relPath))
			skillPreview.Files = append(skillPreview.Files, models.BundlePreviewEntry{
				Path:   path.Join(skillRoot, relPath),
				Action: action,
				Kind:   "text",
			})
			applyBundlePreviewAction(&skillPreview.Summary, action)
			applyBundlePreviewAction(&preview.Summary, action)
		}

		binaryPaths := sortedBlobKeys(skill.binaryFiles)
		for _, relPath := range binaryPaths {
			declared[relPath] = struct{}{}
			entry, hasEntry := existing[relPath]
			var current *models.FileTreeEntry
			var currentBlob []byte
			var blobExists bool
			if hasEntry {
				current = &entry
				currentBlob, blobExists, err = s.fileTree.ReadBlobByEntryID(ctx, entry.ID)
				if err != nil {
					return nil, err
				}
			}
			action := previewBinaryAction(current, currentBlob, blobExists, skill.binaryFiles[relPath])
			skillPreview.Files = append(skillPreview.Files, models.BundlePreviewEntry{
				Path:   path.Join(skillRoot, relPath),
				Action: action,
				Kind:   "binary",
			})
			applyBundlePreviewAction(&skillPreview.Summary, action)
			applyBundlePreviewAction(&preview.Summary, action)
		}

		if mode == bundleModeMirror {
			existingPaths := sortedEntryKeys(existing)
			for _, relPath := range existingPaths {
				if _, ok := declared[relPath]; ok {
					continue
				}
				skillPreview.Files = append(skillPreview.Files, models.BundlePreviewEntry{
					Path:   path.Join(skillRoot, relPath),
					Action: "delete",
					Kind:   "file",
				})
				applyBundlePreviewAction(&skillPreview.Summary, "delete")
				applyBundlePreviewAction(&preview.Summary, "delete")
			}
		}

		preview.Skills[skillName] = skillPreview
	}

	return preview, nil
}

func (s *ExportService) ExportBundle(ctx context.Context, userID uuid.UUID) (*models.Bundle, error) {
	bundle := &models.Bundle{
		Version:   models.BundleVersionV1,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Source:    "neudrive",
		Mode:      bundleModeMerge,
		Profile:   map[string]string{},
		Skills:    map[string]models.BundleSkill{},
		Memory:    []models.BundleMemoryItem{},
	}

	if s.Memory != nil {
		profiles, err := s.Memory.GetProfile(ctx, userID)
		if err != nil {
			return nil, err
		}
		for _, profile := range profiles {
			bundle.Profile[profile.Category] = profile.Content
			bundle.Stats.ProfileItems++
			bundle.Stats.TotalBytes += int64(len(profile.Content))
		}

		scratch, err := s.Memory.GetScratchActive(ctx, userID)
		if err != nil {
			return nil, err
		}
		for _, entry := range scratch {
			item := models.BundleMemoryItem{
				Content:   entry.Content,
				Title:     entry.Title,
				Source:    entry.Source,
				CreatedAt: entry.CreatedAt.UTC().Format(time.RFC3339),
			}
			if entry.ExpiresAt != nil {
				item.ExpiresAt = entry.ExpiresAt.UTC().Format(time.RFC3339)
			}
			bundle.Memory = append(bundle.Memory, item)
			bundle.Stats.MemoryItems++
			bundle.Stats.TotalBytes += int64(len(entry.Content))
		}
	}

	if s.FileTree != nil {
		snapshot, err := s.FileTree.Snapshot(ctx, userID, "/skills", models.TrustLevelFull)
		if err != nil {
			return nil, err
		}
		for _, entry := range snapshot.Entries {
			if entry.IsDirectory {
				continue
			}
			publicPath := hubpath.NormalizePublic(entry.Path)
			parts := strings.SplitN(strings.TrimPrefix(publicPath, "/skills/"), "/", 2)
			if len(parts) != 2 {
				continue
			}
			skillName := parts[0]
			relPath := parts[1]

			skill := bundle.Skills[skillName]
			if skill.Files == nil {
				skill.Files = map[string]string{}
			}
			if skill.BinaryFiles == nil {
				skill.BinaryFiles = map[string]models.BundleBlobFile{}
			}

			if data, ok, err := s.FileTree.ReadBlobByEntryID(ctx, entry.ID); err != nil {
				return nil, err
			} else if ok {
				hash := sha256.Sum256(data)
				skill.BinaryFiles[relPath] = models.BundleBlobFile{
					ContentBase64: base64.StdEncoding.EncodeToString(data),
					ContentType:   entry.ContentType,
					SizeBytes:     int64(len(data)),
					SHA256:        hex.EncodeToString(hash[:]),
				}
				bundle.Stats.BinaryFiles++
				bundle.Stats.TotalBytes += int64(len(data))
			} else {
				skill.Files[relPath] = entry.Content
				bundle.Stats.TotalBytes += int64(len(entry.Content))
			}

			bundle.Skills[skillName] = skill
			bundle.Stats.TotalFiles++
		}
	}

	bundle.Stats.TotalSkills = len(bundle.Skills)
	return bundle, nil
}

func prepareBundlePreview(bundle models.Bundle) (string, map[string]string, []validatedBundleMemoryItem, map[string]validatedBundleSkill, []string, error) {
	if bundle.Version != models.BundleVersionV1 {
		return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: unsupported bundle version %q", bundle.Version)
	}

	mode := normalizeBundleMode(bundle.Mode)
	if mode == "" {
		return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: invalid mode %q", bundle.Mode)
	}

	normalizedProfile := make(map[string]string, len(bundle.Profile))
	for category, content := range bundle.Profile {
		category = strings.TrimSpace(category)
		if category == "" {
			return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: profile category is required")
		}
		normalizedProfile[category] = content
	}

	validatedMemory := make([]validatedBundleMemoryItem, 0, len(bundle.Memory))
	for idx, item := range bundle.Memory {
		if strings.TrimSpace(item.Content) == "" {
			return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: memory[%d] content is required", idx)
		}
		createdAt, expiresAt, err := parseBundleMemoryTimes(item)
		if err != nil {
			return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: memory[%d]: %w", idx, err)
		}
		validatedMemory = append(validatedMemory, validatedBundleMemoryItem{
			content:   item.Content,
			title:     item.Title,
			source:    item.Source,
			createdAt: createdAt,
			expiresAt: expiresAt,
		})
	}

	validatedSkills := make(map[string]validatedBundleSkill, len(bundle.Skills))
	skillNames := make([]string, 0, len(bundle.Skills))
	for skillName, skill := range bundle.Skills {
		if err := validateSlug(skillName, 128); err != nil {
			return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: invalid skill name %q: %w", skillName, err)
		}

		normalized := validatedBundleSkill{
			textFiles:   make(map[string]string, len(skill.Files)),
			binaryFiles: make(map[string]validatedBundleBlob, len(skill.BinaryFiles)),
		}
		hasSkillDoc := false

		for relPath, content := range skill.Files {
			cleanPath, err := cleanBundleRelativePath(relPath)
			if err != nil {
				return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: skill %q: %w", skillName, err)
			}
			if _, exists := normalized.textFiles[cleanPath]; exists {
				return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: skill %q: duplicate file %q", skillName, cleanPath)
			}
			if _, exists := normalized.binaryFiles[cleanPath]; exists {
				return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: skill %q: file %q declared as both text and binary", skillName, cleanPath)
			}
			normalized.textFiles[cleanPath] = content
			if cleanPath == "SKILL.md" {
				hasSkillDoc = true
			}
		}

		for relPath, blob := range skill.BinaryFiles {
			cleanPath, err := cleanBundleRelativePath(relPath)
			if err != nil {
				return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: skill %q: %w", skillName, err)
			}
			if cleanPath == "SKILL.md" {
				return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: skill %q: SKILL.md must be a text file", skillName)
			}
			if _, exists := normalized.textFiles[cleanPath]; exists {
				return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: skill %q: file %q declared as both text and binary", skillName, cleanPath)
			}
			if _, exists := normalized.binaryFiles[cleanPath]; exists {
				return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: skill %q: duplicate file %q", skillName, cleanPath)
			}
			data, contentType, err := decodeBundleBlob(cleanPath, blob)
			if err != nil {
				return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: skill %q: %w", skillName, err)
			}
			normalized.binaryFiles[cleanPath] = validatedBundleBlob{
				data:        data,
				contentType: contentType,
			}
		}

		if !hasSkillDoc {
			return "", nil, nil, nil, nil, fmt.Errorf("import.PreviewBundle: skill %q missing SKILL.md", skillName)
		}

		validatedSkills[skillName] = normalized
		skillNames = append(skillNames, skillName)
	}
	sort.Strings(skillNames)
	return mode, normalizedProfile, validatedMemory, validatedSkills, skillNames, nil
}

func (s *ImportService) loadPreviewEntry(ctx context.Context, userID uuid.UUID, fullPath string) (*models.FileTreeEntry, []byte, bool, error) {
	entry, err := s.fileTree.Read(ctx, userID, fullPath, models.TrustLevelFull)
	if err != nil {
		if errors.Is(err, ErrEntryNotFound) {
			return nil, nil, false, nil
		}
		return nil, nil, false, err
	}
	blob, ok, err := s.fileTree.ReadBlobByEntryID(ctx, entry.ID)
	if err != nil {
		return nil, nil, false, err
	}
	return entry, blob, ok, nil
}

func (s *ImportService) previewTextPath(ctx context.Context, userID uuid.UUID, fullPath, desiredContent, desiredContentType string) (string, error) {
	entry, _, blobExists, err := s.loadPreviewEntry(ctx, userID, fullPath)
	if err != nil {
		return "", err
	}
	return previewTextAction(entry, blobExists, desiredContent, desiredContentType), nil
}

func previewTextAction(entry *models.FileTreeEntry, blobExists bool, desiredContent, desiredContentType string) string {
	if entry == nil {
		return "create"
	}
	if blobExists {
		return "update"
	}
	if entry.Content == desiredContent && entry.ContentType == desiredContentType {
		return "skip"
	}
	return "update"
}

func previewBinaryAction(entry *models.FileTreeEntry, blob []byte, blobExists bool, desired validatedBundleBlob) string {
	if entry == nil {
		return "create"
	}
	if !blobExists {
		return "update"
	}
	if bytes.Equal(blob, desired.data) && entry.ContentType == desired.contentType {
		return "skip"
	}
	return "update"
}

func importedScratchPath(item validatedBundleMemoryItem) string {
	legacyID := importedScratchLegacyID(item.source, item.title, item.createdAt)
	slugBase := item.title
	if strings.TrimSpace(slugBase) == "" {
		slugBase = item.source
	}
	slug := fmt.Sprintf("%s-%s", slugBase, legacyID.String()[:8])
	return hubpath.ScratchPath(item.createdAt, slug)
}

func applyBundlePreviewAction(summary *models.BundlePreviewSummary, action string) {
	switch action {
	case "create":
		summary.Create++
	case "update":
		summary.Update++
	case "delete":
		summary.Delete++
	case "skip":
		summary.Skip++
	case "conflict":
		summary.Conflict++
	}
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedBlobKeys(values map[string]validatedBundleBlob) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedEntryKeys(values map[string]models.FileTreeEntry) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cleanBundleRelativePath(relPath string) (string, error) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(relPath, "\\", "/"))
	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" {
		return "", fmt.Errorf("relative path is required")
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("invalid relative path %q", relPath)
	}
	return cleaned, nil
}

func decodeBundleBlob(relPath string, blob models.BundleBlobFile) ([]byte, string, error) {
	data, err := base64.StdEncoding.DecodeString(blob.ContentBase64)
	if err != nil {
		return nil, "", fmt.Errorf("invalid base64 for %s: %w", relPath, err)
	}
	if blob.SizeBytes > 0 && int64(len(data)) != blob.SizeBytes {
		return nil, "", fmt.Errorf("size mismatch for %s: got %d want %d", relPath, len(data), blob.SizeBytes)
	}
	if blob.SHA256 != "" {
		sum := sha256.Sum256(data)
		if !strings.EqualFold(blob.SHA256, hex.EncodeToString(sum[:])) {
			return nil, "", fmt.Errorf("sha256 mismatch for %s", relPath)
		}
	}

	contentType := strings.TrimSpace(blob.ContentType)
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(relPath))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return data, contentType, nil
}

func parseBundleMemoryTimes(item models.BundleMemoryItem) (time.Time, *time.Time, error) {
	createdAt := time.Now().UTC()
	if strings.TrimSpace(item.CreatedAt) != "" {
		parsed, err := time.Parse(time.RFC3339, item.CreatedAt)
		if err != nil {
			return time.Time{}, nil, fmt.Errorf("invalid created_at %q", item.CreatedAt)
		}
		createdAt = parsed.UTC()
	}

	var expiresAt *time.Time
	if strings.TrimSpace(item.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, item.ExpiresAt)
		if err != nil {
			return time.Time{}, nil, fmt.Errorf("invalid expires_at %q", item.ExpiresAt)
		}
		ts := parsed.UTC()
		expiresAt = &ts
	}
	return createdAt, expiresAt, nil
}
