package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
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
		declared := make(map[string]struct{}, len(skill.textFiles)+len(skill.binaryFiles))

		for relPath, content := range skill.textFiles {
			fullPath := path.Join("/skills", skillName, relPath)
			if _, err := s.fileTree.WriteEntry(ctx, userID, fullPath, content, contentTypeFromExt(relPath), models.FileTreeWriteOptions{
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

func (s *ExportService) ExportBundle(ctx context.Context, userID uuid.UUID) (*models.Bundle, error) {
	bundle := &models.Bundle{
		Version:   models.BundleVersionV1,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Source:    "agenthub",
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
