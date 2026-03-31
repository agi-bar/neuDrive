package services

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
)

// ExportService handles full data export for data portability.
type ExportService struct {
	FileTree *FileTreeService
	Memory   *MemoryService
	Project  *ProjectService
	Vault    *VaultService
	Device   *DeviceService
	Inbox    *InboxService
	Role     *RoleService
	User     *UserService
}

// NewExportService creates a new ExportService.
func NewExportService(
	fileTree *FileTreeService,
	memory *MemoryService,
	project *ProjectService,
	vault *VaultService,
	device *DeviceService,
	inbox *InboxService,
	role *RoleService,
	user *UserService,
) *ExportService {
	return &ExportService{
		FileTree: fileTree,
		Memory:   memory,
		Project:  project,
		Vault:    vault,
		Device:   device,
		Inbox:    inbox,
		Role:     role,
		User:     user,
	}
}

// ExportToZip creates a zip archive of the user's entire hub data and writes it
// to the provided writer. It streams directly without buffering the full zip.
func (s *ExportService) ExportToZip(ctx context.Context, userID uuid.UUID, w io.Writer) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	data, err := s.gatherExportData(ctx, userID)
	if err != nil {
		return fmt.Errorf("export.ExportToZip: gather data: %w", err)
	}

	// identity/profile.json
	if data.Identity != nil {
		if err := writeZipJSON(zw, "export/identity/profile.json", data.Identity); err != nil {
			return err
		}
	}

	// vault/scopes.json
	if len(data.VaultScopes) > 0 {
		if err := writeZipJSON(zw, "export/vault/scopes.json", data.VaultScopes); err != nil {
			return err
		}
	}

	// skills/ — files from the file tree under .skills/
	for path, content := range data.SkillFiles {
		// Convert .skills/cyberzen-write/SKILL.md -> export/skills/cyberzen-write/SKILL.md
		zipPath := "export/skills/" + strings.TrimPrefix(path, ".skills/")
		if err := writeZipString(zw, zipPath, content); err != nil {
			return err
		}
	}

	// memory/profile/
	for category, content := range data.ProfileMemory {
		if err := writeZipString(zw, "export/memory/profile/"+category+".md", content); err != nil {
			return err
		}
	}

	// memory/projects/
	for _, proj := range data.Projects {
		dir := "export/memory/projects/" + proj.Name + "/"
		if proj.ContextMD != "" {
			if err := writeZipString(zw, dir+"context.md", proj.ContextMD); err != nil {
				return err
			}
		}
		// Write project metadata as JSON
		meta := map[string]string{"name": proj.Name, "status": proj.Status}
		if err := writeZipJSON(zw, dir+"project.json", meta); err != nil {
			return err
		}
	}

	// memory/scratch/
	for _, scratch := range data.ScratchEntries {
		filename := scratch.Date
		if filename == "" {
			filename = scratch.CreatedAt.Format("2006-01-02")
		}
		if err := writeZipString(zw, "export/memory/scratch/"+filename+".md", scratch.Content); err != nil {
			return err
		}
	}

	// devices/
	for _, dev := range data.Devices {
		if err := writeZipJSON(zw, "export/devices/"+dev.Name+".json", dev); err != nil {
			return err
		}
	}

	// roles/roles.json
	if len(data.Roles) > 0 {
		if err := writeZipJSON(zw, "export/roles/roles.json", data.Roles); err != nil {
			return err
		}
	}

	// inbox/messages.jsonl
	if len(data.InboxMessages) > 0 {
		fw, err := zw.Create("export/inbox/messages.jsonl")
		if err != nil {
			return fmt.Errorf("export: create inbox file: %w", err)
		}
		for _, msg := range data.InboxMessages {
			line, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			if _, err := fw.Write(append(line, '\n')); err != nil {
				return fmt.Errorf("export: write inbox line: %w", err)
			}
		}
	}

	// metadata.json
	metadata := map[string]interface{}{
		"version":     "1.0",
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"user_id":     userID.String(),
		"stats": map[string]int{
			"skills":    len(data.SkillFiles),
			"devices":   len(data.Devices),
			"projects":  len(data.Projects),
			"roles":     len(data.Roles),
			"messages":  len(data.InboxMessages),
			"vault_scopes": len(data.VaultScopes),
		},
	}
	if err := writeZipJSON(zw, "export/metadata.json", metadata); err != nil {
		return err
	}

	return nil
}

// ExportToJSON returns the same data as a single JSON-serializable map.
func (s *ExportService) ExportToJSON(ctx context.Context, userID uuid.UUID) (map[string]interface{}, error) {
	data, err := s.gatherExportData(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("export.ExportToJSON: %w", err)
	}

	result := map[string]interface{}{
		"version":     "1.0",
		"exported_at": time.Now().UTC().Format(time.RFC3339),
	}

	if data.Identity != nil {
		result["identity"] = data.Identity
	}

	if len(data.VaultScopes) > 0 {
		result["vault_scopes"] = data.VaultScopes
	}

	if len(data.SkillFiles) > 0 {
		result["skills"] = data.SkillFiles
	}

	if len(data.ProfileMemory) > 0 {
		result["profile"] = data.ProfileMemory
	}

	if len(data.Projects) > 0 {
		projects := make([]map[string]string, 0, len(data.Projects))
		for _, p := range data.Projects {
			projects = append(projects, map[string]string{
				"name":       p.Name,
				"status":     p.Status,
				"context_md": p.ContextMD,
			})
		}
		result["projects"] = projects
	}

	if len(data.ScratchEntries) > 0 {
		scratch := make([]map[string]string, 0, len(data.ScratchEntries))
		for _, sc := range data.ScratchEntries {
			scratch = append(scratch, map[string]string{
				"date":    sc.Date,
				"content": sc.Content,
				"source":  sc.Source,
			})
		}
		result["scratch"] = scratch
	}

	if len(data.Devices) > 0 {
		result["devices"] = data.Devices
	}

	if len(data.Roles) > 0 {
		result["roles"] = data.Roles
	}

	if len(data.InboxMessages) > 0 {
		result["inbox"] = data.InboxMessages
	}

	return result, nil
}

// exportData is an internal aggregate of all user data for export.
type exportData struct {
	Identity       map[string]interface{}
	VaultScopes    []vaultScopeExport
	SkillFiles     map[string]string // path -> content
	ProfileMemory  map[string]string // category -> content
	Projects       []models.Project
	ScratchEntries []models.MemoryScratch
	Devices        []models.Device
	Roles          []models.Role
	InboxMessages  []models.InboxMessage
}

type vaultScopeExport struct {
	Scope         string `json:"scope"`
	Description   string `json:"description"`
	MinTrustLevel int    `json:"min_trust_level"`
}

func (s *ExportService) gatherExportData(ctx context.Context, userID uuid.UUID) (*exportData, error) {
	data := &exportData{
		SkillFiles:    make(map[string]string),
		ProfileMemory: make(map[string]string),
	}

	// Identity (user profile).
	if s.User != nil {
		user, err := s.User.GetByID(ctx, userID)
		if err == nil {
			data.Identity = map[string]interface{}{
				"slug":         user.Slug,
				"display_name": user.DisplayName,
				"email":        user.Email,
				"timezone":     user.Timezone,
				"language":     user.Language,
				"created_at":   user.CreatedAt.Format(time.RFC3339),
			}
		}
	}

	// Vault scopes (names + descriptions, NOT decrypted values).
	if s.Vault != nil {
		scopes, err := s.Vault.ListScopes(ctx, userID, models.TrustLevelFull)
		if err == nil {
			for _, vs := range scopes {
				data.VaultScopes = append(data.VaultScopes, vaultScopeExport{
					Scope:         vs.Scope,
					Description:   vs.Description,
					MinTrustLevel: vs.MinTrustLevel,
				})
			}
		}
	}

	// Skills from file tree (everything under .skills/).
	if s.FileTree != nil {
		entries, err := s.FileTree.List(ctx, userID, ".skills/", models.TrustLevelFull)
		if err == nil {
			for _, e := range entries {
				if e.IsDirectory {
					continue
				}
				full, err := s.FileTree.Read(ctx, userID, e.Path, models.TrustLevelFull)
				if err != nil {
					continue
				}
				data.SkillFiles[full.Path] = full.Content
			}
		}
	}

	// Memory profile.
	if s.Memory != nil {
		profiles, err := s.Memory.GetProfile(ctx, userID)
		if err == nil {
			for _, p := range profiles {
				data.ProfileMemory[p.Category] = p.Content
			}
		}

		// Memory scratch (last 365 days for full export).
		scratch, err := s.Memory.GetScratch(ctx, userID, 365)
		if err == nil {
			data.ScratchEntries = scratch
		}
	}

	// Projects.
	if s.Project != nil {
		projects, err := s.Project.List(ctx, userID)
		if err == nil {
			data.Projects = projects
		}
	}

	// Devices.
	if s.Device != nil {
		devices, err := s.Device.List(ctx, userID)
		if err == nil {
			data.Devices = devices
		}
	}

	// Roles.
	if s.Role != nil {
		roles, err := s.Role.List(ctx, userID)
		if err == nil {
			data.Roles = roles
		}
	}

	// Inbox messages.
	if s.Inbox != nil {
		messages, err := s.Inbox.GetMessages(ctx, userID, "", "")
		if err == nil {
			data.InboxMessages = messages
		}
	}

	return data, nil
}

// writeZipJSON marshals v as indented JSON and writes it to the zip as a file.
func writeZipJSON(zw *zip.Writer, path string, v interface{}) error {
	fw, err := zw.Create(path)
	if err != nil {
		return fmt.Errorf("export: create %s: %w", path, err)
	}
	enc := json.NewEncoder(fw)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("export: encode %s: %w", path, err)
	}
	return nil
}

// writeZipString writes a string as a file in the zip.
func writeZipString(zw *zip.Writer, path, content string) error {
	fw, err := zw.Create(path)
	if err != nil {
		return fmt.Errorf("export: create %s: %w", path, err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		return fmt.Errorf("export: write %s: %w", path, err)
	}
	return nil
}
