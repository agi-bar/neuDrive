package sqlite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/runtimecfg"
	"github.com/agi-bar/agenthub/internal/skillsarchive"
	"github.com/google/uuid"
)

type Source struct {
	Domain string
	Label  string
	Path   string
	IsDir  bool
}

type ImportResult struct {
	Platform string
	Files    int
	Bytes    int64
	Paths    []string
}

type ExportResult struct {
	Platform   string
	Files      int
	Bytes      int64
	OutputRoot string
	Paths      []string
}

type Client struct {
	store  *Store
	userID uuid.UUID
}

func OpenClient(ctx context.Context, cfg *runtimecfg.CLIConfig) (*Client, error) {
	store, err := Open(cfg.Local.SQLitePath)
	if err != nil {
		return nil, err
	}
	user, err := store.EnsureOwner(ctx)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return &Client{store: store, userID: user.ID}, nil
}

func (c *Client) Close() {
	if c.store != nil {
		_ = c.store.Close()
	}
}

func (c *Client) CreatePlatformToken(ctx context.Context, platform string, trustLevel int) (*models.CreateTokenResponse, error) {
	scopes := make([]string, 0, len(models.AllScopes)-1)
	for _, scope := range models.AllScopes {
		if scope == models.ScopeAdmin {
			continue
		}
		scopes = append(scopes, scope)
	}
	return c.store.CreateToken(ctx, c.userID, "local platform "+platform, scopes, trustLevel, 365*24*time.Hour)
}

func (c *Client) CreateOwnerToken(ctx context.Context) (*models.CreateTokenResponse, error) {
	return c.store.CreateToken(ctx, c.userID, "local owner", []string{models.ScopeAdmin}, models.TrustLevelFull, 365*24*time.Hour)
}

func (c *Client) RevokeToken(ctx context.Context, tokenID string) error {
	id, err := uuid.Parse(strings.TrimSpace(tokenID))
	if err != nil {
		return err
	}
	return c.store.RevokeToken(ctx, c.userID, id)
}

func (c *Client) ImportPlatformSources(ctx context.Context, platform string, sources []Source) (*ImportResult, error) {
	result := &ImportResult{Platform: platform}
	for _, source := range sources {
		info, err := os.Stat(source.Path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			err = filepath.WalkDir(source.Path, func(pathValue string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					if pathValue != source.Path && isManagedAgentHubDir(pathValue) {
						return filepath.SkipDir
					}
					return nil
				}
				rel, err := filepath.Rel(source.Path, pathValue)
				if err != nil {
					return err
				}
				hubPath := filepath.ToSlash(filepath.Join("/platforms", platform, source.Domain, source.Label, rel))
				bytesWritten, err := c.writeLocalFile(ctx, hubPath, pathValue, map[string]interface{}{
					"platform":      platform,
					"domain":        source.Domain,
					"source_label":  source.Label,
					"original_path": pathValue,
				})
				if err != nil {
					return err
				}
				result.Files++
				result.Bytes += bytesWritten
				result.Paths = append(result.Paths, hubPath)
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		hubPath := filepath.ToSlash(filepath.Join("/platforms", platform, source.Domain, source.Label))
		bytesWritten, err := c.writeLocalFile(ctx, hubPath, source.Path, map[string]interface{}{
			"platform":      platform,
			"domain":        source.Domain,
			"source_label":  source.Label,
			"original_path": source.Path,
		})
		if err != nil {
			return nil, err
		}
		result.Files++
		result.Bytes += bytesWritten
		result.Paths = append(result.Paths, hubPath)
	}
	sort.Strings(result.Paths)
	return result, nil
}

func (c *Client) ExportPlatformSnapshot(ctx context.Context, platform, outputRoot string) (*ExportResult, error) {
	if outputRoot == "" {
		outputRoot = filepath.Join(".", "agenthub-export", platform)
	}
	result := &ExportResult{Platform: platform, OutputRoot: outputRoot}
	snapshot, err := c.store.Snapshot(ctx, c.userID, filepath.ToSlash(filepath.Join("/platforms", platform)), models.TrustLevelFull)
	if err != nil {
		return nil, err
	}
	for _, entry := range snapshot.Entries {
		if entry.IsDirectory {
			continue
		}
		rel := strings.TrimPrefix(entry.Path, filepath.ToSlash(filepath.Join("/platforms", platform)))
		rel = strings.TrimPrefix(rel, "/")
		target := filepath.Join(outputRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		if isBinaryMetadata(entry.Metadata) {
			data, _, err := c.store.ReadBinary(ctx, c.userID, entry.Path, models.TrustLevelFull)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				return nil, err
			}
			result.Files++
			result.Bytes += int64(len(data))
			result.Paths = append(result.Paths, target)
			continue
		}
		if err := os.WriteFile(target, []byte(entry.Content), 0o644); err != nil {
			return nil, err
		}
		result.Files++
		result.Bytes += int64(len(entry.Content))
		result.Paths = append(result.Paths, target)
	}
	sort.Strings(result.Paths)
	return result, nil
}

func (c *Client) writeLocalFile(ctx context.Context, hubPath, srcPath string, metadata map[string]interface{}) (int64, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return 0, err
	}
	contentType := detectContentType(srcPath, data)
	if looksBinary(srcPath, data) {
		_, err = c.store.WriteBinaryEntry(ctx, c.userID, hubPath, data, contentType, models.FileTreeWriteOptions{
			Metadata:      metadata,
			MinTrustLevel: models.TrustLevelWork,
		})
		return int64(len(data)), err
	}
	_, err = c.store.WriteEntry(ctx, c.userID, hubPath, string(data), contentType, models.FileTreeWriteOptions{
		Metadata:      metadata,
		MinTrustLevel: models.TrustLevelWork,
	})
	return int64(len(data)), err
}

func detectContentType(path string, data []byte) string {
	return skillsarchive.DetectContentType(path, data)
}

func looksBinary(path string, data []byte) bool {
	return skillsarchive.LooksBinary(path, data)
}

func isManagedAgentHubDir(pathValue string) bool {
	_, err := os.Stat(filepath.Join(pathValue, ".agenthub-managed.json"))
	return err == nil
}

func (c *Client) ValidateToken(ctx context.Context, token string) (*models.ScopedToken, error) {
	return c.store.ValidateToken(ctx, token)
}

func (c *Client) Store() *Store {
	return c.store
}

func (c *Client) UserID() uuid.UUID {
	return c.userID
}

func (c *Client) EnsureOwner(ctx context.Context) error {
	user, err := c.store.EnsureOwner(ctx)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("owner bootstrap failed")
	}
	c.userID = user.ID
	return nil
}
