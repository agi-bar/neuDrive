package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	BundleFormatJSON    = "json"
	BundleFormatArchive = "archive"

	SyncTransportVersionV1 = "ahub.sync/v1"
	SyncTransportAuto      = "auto"
	SyncTransportJSON      = "json"
	SyncTransportArchive   = "archive"

	SyncSessionStatusUploading = "uploading"
	SyncSessionStatusReady     = "ready"
	SyncSessionStatusCommitted = "committed"
	SyncSessionStatusAborted   = "aborted"
	SyncSessionStatusExpired   = "expired"

	SyncJobDirectionImport = "import"
	SyncJobDirectionExport = "export"

	SyncJobTransportJSON    = "json"
	SyncJobTransportArchive = "archive"

	SyncJobStatusPending   = "pending"
	SyncJobStatusRunning   = "running"
	SyncJobStatusSucceeded = "succeeded"
	SyncJobStatusFailed    = "failed"
	SyncJobStatusAborted   = "aborted"

	DefaultArchiveAutoThreshold = 8 << 20
	DefaultSyncChunkSize        = 4 << 20
)

type BundleFilters struct {
	IncludeDomains []string `json:"include_domains,omitempty"`
	IncludeSkills  []string `json:"include_skills,omitempty"`
	ExcludeSkills  []string `json:"exclude_skills,omitempty"`
}

type BundleArchiveEntry struct {
	ArchivePath string `json:"archive_path"`
	Binary      bool   `json:"binary,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

type BundleArchiveMemoryItem struct {
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	Source      string `json:"source,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	ArchivePath string `json:"archive_path"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

type BundleArchiveManifest struct {
	Version       string                                   `json:"version"`
	CreatedAt     string                                   `json:"created_at"`
	Source        string                                   `json:"source,omitempty"`
	Mode          string                                   `json:"mode,omitempty"`
	Domains       []string                                 `json:"domains,omitempty"`
	Filters       BundleFilters                            `json:"filters,omitempty"`
	ProfileFiles  map[string]BundleArchiveEntry            `json:"profile_files,omitempty"`
	MemoryItems   []BundleArchiveMemoryItem                `json:"memory_items,omitempty"`
	SkillFiles    map[string]map[string]BundleArchiveEntry `json:"skill_files,omitempty"`
	Stats         BundleStats                              `json:"stats,omitempty"`
	ArchiveSHA256 string                                   `json:"archive_sha256,omitempty"`
}

type BundlePreviewRequest struct {
	Bundle   *Bundle                `json:"bundle,omitempty"`
	Manifest *BundleArchiveManifest `json:"manifest,omitempty"`
}

type SyncStartSessionRequest struct {
	TransportVersion string                `json:"transport_version"`
	Format           string                `json:"format"`
	Mode             string                `json:"mode,omitempty"`
	Manifest         BundleArchiveManifest `json:"manifest"`
	ArchiveSizeBytes int64                 `json:"archive_size_bytes"`
	ArchiveSHA256    string                `json:"archive_sha256"`
}

type SyncCommitRequest struct {
	PreviewFingerprint string `json:"preview_fingerprint,omitempty"`
}

type SyncSessionResponse struct {
	SessionID      uuid.UUID      `json:"session_id"`
	JobID          uuid.UUID      `json:"job_id"`
	Status         string         `json:"status"`
	ChunkSizeBytes int64          `json:"chunk_size_bytes"`
	TotalParts     int            `json:"total_parts"`
	ExpiresAt      time.Time      `json:"expires_at"`
	Mode           string         `json:"mode,omitempty"`
	Summary        SyncJobSummary `json:"summary"`
	ReceivedParts  []int          `json:"received_parts,omitempty"`
	MissingParts   []int          `json:"missing_parts,omitempty"`
}

type SyncJobSummary struct {
	TotalSkills       int   `json:"total_skills,omitempty"`
	TotalFiles        int   `json:"total_files,omitempty"`
	TotalBytes        int64 `json:"total_bytes,omitempty"`
	BinaryFiles       int   `json:"binary_files,omitempty"`
	ProfileItems      int   `json:"profile_items,omitempty"`
	MemoryItems       int   `json:"memory_items,omitempty"`
	SkillsWritten     int   `json:"skills_written,omitempty"`
	FilesWritten      int   `json:"files_written,omitempty"`
	FilesDeleted      int   `json:"files_deleted,omitempty"`
	ProfileCategories int   `json:"profile_categories,omitempty"`
	MemoryImported    int   `json:"memory_imported,omitempty"`
}

type SyncJob struct {
	ID          uuid.UUID      `json:"id"`
	UserID      uuid.UUID      `json:"user_id"`
	SessionID   *uuid.UUID     `json:"session_id,omitempty"`
	Direction   string         `json:"direction"`
	Transport   string         `json:"transport"`
	Status      string         `json:"status"`
	Source      string         `json:"source,omitempty"`
	Mode        string         `json:"mode,omitempty"`
	Filters     BundleFilters  `json:"filters,omitempty"`
	Summary     SyncJobSummary `json:"summary"`
	Error       string         `json:"error,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
}

type SyncSession struct {
	ID               uuid.UUID             `json:"id"`
	UserID           uuid.UUID             `json:"user_id"`
	JobID            uuid.UUID             `json:"job_id"`
	Status           string                `json:"status"`
	Format           string                `json:"format"`
	Mode             string                `json:"mode"`
	Manifest         BundleArchiveManifest `json:"manifest"`
	ArchiveSizeBytes int64                 `json:"archive_size_bytes"`
	ArchiveSHA256    string                `json:"archive_sha256"`
	ChunkSizeBytes   int64                 `json:"chunk_size_bytes"`
	TotalParts       int                   `json:"total_parts"`
	ExpiresAt        time.Time             `json:"expires_at"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
	CommittedAt      *time.Time            `json:"committed_at,omitempty"`
}

type SyncTokenRequest struct {
	Access     string `json:"access"`
	TTLMinutes int    `json:"ttl_minutes"`
}

type SyncTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	APIBase   string    `json:"api_base"`
	Scopes    []string  `json:"scopes"`
	Usage     string    `json:"usage"`
}

func (s SyncSession) ManifestJSON() ([]byte, error) {
	return json.Marshal(s.Manifest)
}
