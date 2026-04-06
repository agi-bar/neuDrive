package models

const (
	BundleVersionV1 = "ahub.bundle/v1"
	BundleVersionV2 = "ahub.bundle/v2"
)

type Bundle struct {
	Version   string                 `json:"version"`
	CreatedAt string                 `json:"created_at"`
	Source    string                 `json:"source,omitempty"`
	Mode      string                 `json:"mode,omitempty"`
	Profile   map[string]string      `json:"profile,omitempty"`
	Skills    map[string]BundleSkill `json:"skills,omitempty"`
	Memory    []BundleMemoryItem     `json:"memory,omitempty"`
	Stats     BundleStats            `json:"stats,omitempty"`
}

type BundleSkill struct {
	Files       map[string]string         `json:"files,omitempty"`
	BinaryFiles map[string]BundleBlobFile `json:"binary_files,omitempty"`
}

type BundleBlobFile struct {
	ContentBase64 string `json:"content_base64"`
	ContentType   string `json:"content_type,omitempty"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
}

type BundleMemoryItem struct {
	Content   string `json:"content"`
	Title     string `json:"title,omitempty"`
	Source    string `json:"source,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type BundleStats struct {
	TotalSkills  int   `json:"total_skills,omitempty"`
	TotalFiles   int   `json:"total_files,omitempty"`
	TotalBytes   int64 `json:"total_bytes,omitempty"`
	BinaryFiles  int   `json:"binary_files,omitempty"`
	ProfileItems int   `json:"profile_items,omitempty"`
	MemoryItems  int   `json:"memory_items,omitempty"`
}

type BundleImportResult struct {
	Version           string `json:"version"`
	Mode              string `json:"mode"`
	SkillsWritten     int    `json:"skills_written"`
	FilesWritten      int    `json:"files_written"`
	FilesDeleted      int    `json:"files_deleted"`
	ProfileCategories int    `json:"profile_categories"`
	MemoryImported    int    `json:"memory_imported"`
}

type BundlePreviewSummary struct {
	Create   int `json:"create,omitempty"`
	Update   int `json:"update,omitempty"`
	Delete   int `json:"delete,omitempty"`
	Skip     int `json:"skip,omitempty"`
	Conflict int `json:"conflict,omitempty"`
}

type BundlePreviewEntry struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Kind   string `json:"kind,omitempty"`
}

type BundleSkillPreview struct {
	Summary BundlePreviewSummary `json:"summary"`
	Files   []BundlePreviewEntry `json:"files,omitempty"`
}

type BundlePreviewResult struct {
	Version     string                        `json:"version"`
	Mode        string                        `json:"mode"`
	Fingerprint string                        `json:"fingerprint,omitempty"`
	Summary     BundlePreviewSummary          `json:"summary"`
	Profile     []BundlePreviewEntry          `json:"profile,omitempty"`
	Memory      []BundlePreviewEntry          `json:"memory,omitempty"`
	Skills      map[string]BundleSkillPreview `json:"skills,omitempty"`
}
