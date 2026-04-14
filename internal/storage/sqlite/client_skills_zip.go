package sqlite

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/skillsarchive"
)

func (c *Client) ImportSkillsArchive(ctx context.Context, platform, archivePath string) (*ImportResult, error) {
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open skills archive: %w", err)
	}
	files, err := skillsarchive.ParseZipBytes(data, filepath.Base(archivePath))
	if err != nil {
		return nil, err
	}

	result := &ImportResult{Platform: platform}
	for _, file := range files {
		hubPath := filepath.ToSlash(path.Join("/skills", file.SkillName, file.RelPath))
		metadata := map[string]interface{}{
			"source_platform": platform,
			"source_archive":  filepath.Base(archivePath),
			"capture_mode":    "archive",
		}
		contentType := skillsarchive.DetectContentType(file.RelPath, file.Data)
		if skillsarchive.LooksBinary(file.RelPath, file.Data) {
			if _, err := c.store.WriteBinaryEntry(ctx, c.userID, hubPath, file.Data, contentType, models.FileTreeWriteOptions{
				Kind:          "skill_asset",
				Metadata:      metadata,
				MinTrustLevel: models.TrustLevelWork,
			}); err != nil {
				return nil, fmt.Errorf("write %s: %w", hubPath, err)
			}
		} else {
			if _, err := c.store.WriteEntry(ctx, c.userID, hubPath, string(file.Data), contentType, models.FileTreeWriteOptions{
				Kind:          "skill_file",
				Metadata:      metadata,
				MinTrustLevel: models.TrustLevelWork,
			}); err != nil {
				return nil, fmt.Errorf("write %s: %w", hubPath, err)
			}
		}
		result.Files++
		result.Bytes += int64(len(file.Data))
		result.Paths = append(result.Paths, hubPath)
	}
	sort.Strings(result.Paths)
	return result, nil
}
