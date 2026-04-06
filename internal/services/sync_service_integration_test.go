package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agi-bar/agenthub/internal/database"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupSyncIntegration(t *testing.T) (context.Context, uuid.UUID, *SyncService, *FileTreeService, *MemoryService) {
	t.Helper()

	dbURL := os.Getenv("AGENTHUB_TEST_DB")
	if dbURL == "" {
		t.Skip("AGENTHUB_TEST_DB not set; skipping sync integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	bundleTestMigrationsOnce.Do(func() {
		bundleTestMigrationsErr = database.RunMigrations(pool, filepath.Join("..", "..", "migrations"))
	})
	if bundleTestMigrationsErr != nil {
		t.Fatalf("run migrations: %v", bundleTestMigrationsErr)
	}

	userID := uuid.New()
	now := time.Now().UTC()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, slug, display_name, timezone, language, created_at, updated_at)
		 VALUES ($1, $2, $3, 'UTC', 'en', $4, $4)`,
		userID, "sync-test-"+userID.String()[:8], "Sync Test User", now,
	); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	fileTree := NewFileTreeService(pool)
	memory := NewMemoryService(pool, fileTree)
	importSvc := NewImportService(pool, fileTree, memory, nil)
	exportSvc := NewExportService(fileTree, memory, nil, nil, nil, nil, nil, nil)
	syncSvc := NewSyncService(pool, importSvc, exportSvc, fileTree, memory)
	return ctx, userID, syncSvc, fileTree, memory
}

func buildArchiveFixtureAtLeast(t *testing.T, minSize int) (models.Bundle, []byte, *models.BundleArchiveManifest) {
	t.Helper()
	for multiplier := 1; multiplier <= 24; multiplier++ {
		bundle := buildLargeFixtureBundle(t, multiplier)
		archive, manifest, err := BuildBundleArchive(bundle, models.BundleFilters{})
		if err != nil {
			t.Fatalf("build archive fixture: %v", err)
		}
		if len(archive) > minSize {
			return bundle, archive, manifest
		}
	}
	t.Fatalf("could not build archive fixture larger than %d bytes", minSize)
	return models.Bundle{}, nil, nil
}

func TestBundleArchive_RoundTripRealisticFixture(t *testing.T) {
	bundle := buildLargeFixtureBundle(t, 2)
	archive, manifest, err := BuildBundleArchive(bundle, models.BundleFilters{})
	if err != nil {
		t.Fatalf("build archive: %v", err)
	}
	if len(archive) == 0 {
		t.Fatal("archive is empty")
	}
	if manifest.Version != models.BundleVersionV2 {
		t.Fatalf("manifest version = %q", manifest.Version)
	}
	if manifest.ArchiveSHA256 == "" {
		t.Fatal("manifest archive sha is empty")
	}

	decoded, decodedManifest, err := ParseBundleArchive(archive)
	if err != nil {
		t.Fatalf("parse archive: %v", err)
	}
	if decodedManifest.ArchiveSHA256 != manifest.ArchiveSHA256 {
		t.Fatalf("manifest sha mismatch after parse: got %q want %q", decodedManifest.ArchiveSHA256, manifest.ArchiveSHA256)
	}
	if len(decoded.Skills) != len(bundle.Skills) {
		t.Fatalf("skill count mismatch after archive round-trip: got %d want %d", len(decoded.Skills), len(bundle.Skills))
	}
	if len(decoded.Memory) != len(bundle.Memory) {
		t.Fatalf("memory count mismatch after archive round-trip: got %d want %d", len(decoded.Memory), len(bundle.Memory))
	}
	if len(decoded.Profile) != len(bundle.Profile) {
		t.Fatalf("profile count mismatch after archive round-trip: got %d want %d", len(decoded.Profile), len(bundle.Profile))
	}
}

func TestSyncService_SessionCommitResumeAndHistory(t *testing.T) {
	ctx, userID, syncSvc, fileTree, _ := setupSyncIntegration(t)
	_, archive, manifest := buildArchiveFixtureAtLeast(t, int(models.DefaultSyncChunkSize))

	started, err := syncSvc.StartSession(ctx, userID, models.SyncStartSessionRequest{
		TransportVersion: models.SyncTransportVersionV1,
		Format:           models.BundleFormatArchive,
		Mode:             bundleModeMirror,
		Manifest:         *manifest,
		ArchiveSizeBytes: int64(len(archive)),
		ArchiveSHA256:    manifest.ArchiveSHA256,
	})
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	if started.TotalParts < 2 {
		t.Fatalf("expected multi-part session, got %d parts", started.TotalParts)
	}

	firstPart := archive[:int(syncSvc.chunkSize)]
	state, err := syncSvc.UploadPart(ctx, userID, started.SessionID, 0, firstPart)
	if err != nil {
		t.Fatalf("upload first part: %v", err)
	}
	if state.Status != models.SyncSessionStatusUploading {
		t.Fatalf("state after first part = %q", state.Status)
	}
	if len(state.MissingParts) != started.TotalParts-1 {
		t.Fatalf("missing parts after first upload = %d, want %d", len(state.MissingParts), started.TotalParts-1)
	}

	for idx := 1; idx < started.TotalParts; idx++ {
		start := idx * int(syncSvc.chunkSize)
		end := start + int(syncSvc.chunkSize)
		if end > len(archive) {
			end = len(archive)
		}
		if _, err := syncSvc.UploadPart(ctx, userID, started.SessionID, idx, archive[start:end]); err != nil {
			t.Fatalf("upload part %d: %v", idx, err)
		}
	}

	sessionState, err := syncSvc.GetSession(ctx, userID, started.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sessionState.Status != models.SyncSessionStatusReady {
		t.Fatalf("ready state = %q", sessionState.Status)
	}

	preview, err := syncSvc.PreviewManifest(ctx, userID, *manifest)
	if err != nil {
		t.Fatalf("preview manifest: %v", err)
	}
	result, err := syncSvc.CommitSession(ctx, userID, started.SessionID, models.SyncCommitRequest{
		PreviewFingerprint: preview.Fingerprint,
	})
	if err != nil {
		t.Fatalf("commit session: %v", err)
	}
	if result.FilesWritten == 0 {
		t.Fatal("expected files to be written on commit")
	}

	committedState, err := syncSvc.GetSession(ctx, userID, started.SessionID)
	if err != nil {
		t.Fatalf("get committed session: %v", err)
	}
	if committedState.Status != models.SyncSessionStatusCommitted {
		t.Fatalf("committed state = %q", committedState.Status)
	}
	if len(committedState.ReceivedParts) != 0 {
		t.Fatalf("expected parts cleanup after commit, got %d received parts", len(committedState.ReceivedParts))
	}

	entry, err := fileTree.Read(ctx, userID, "/skills/atlas-brief/SKILL.md", models.TrustLevelFull)
	if err != nil {
		t.Fatalf("read imported skill: %v", err)
	}
	if entry.Content == "" {
		t.Fatal("imported skill content is empty")
	}

	jobs, err := syncSvc.ListJobs(ctx, userID)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("expected sync jobs after commit")
	}
	if jobs[0].Status != models.SyncJobStatusSucceeded {
		t.Fatalf("latest job status = %q, want succeeded", jobs[0].Status)
	}
	if jobs[0].Transport != models.SyncJobTransportArchive {
		t.Fatalf("latest job transport = %q", jobs[0].Transport)
	}
}

func TestSyncService_SessionConflictAbortAndSelectiveExport(t *testing.T) {
	ctx, userID, syncSvc, _, _ := setupSyncIntegration(t)
	bundle, archive, manifest := buildArchiveFixtureAtLeast(t, int(models.DefaultSyncChunkSize))

	started, err := syncSvc.StartSession(ctx, userID, models.SyncStartSessionRequest{
		TransportVersion: models.SyncTransportVersionV1,
		Format:           models.BundleFormatArchive,
		Mode:             bundleModeMerge,
		Manifest:         *manifest,
		ArchiveSizeBytes: int64(len(archive)),
		ArchiveSHA256:    manifest.ArchiveSHA256,
	})
	if err != nil {
		t.Fatalf("start session: %v", err)
	}

	part0 := archive[:int(syncSvc.chunkSize)]
	if _, err := syncSvc.UploadPart(ctx, userID, started.SessionID, 0, part0); err != nil {
		t.Fatalf("upload first part: %v", err)
	}
	badPart := append([]byte(nil), part0...)
	badPart[0] ^= 0xFF
	if _, err := syncSvc.UploadPart(ctx, userID, started.SessionID, 0, badPart); !errors.Is(err, ErrSyncPartConflict) {
		t.Fatalf("expected part conflict, got %v", err)
	}

	if err := syncSvc.AbortSession(ctx, userID, started.SessionID); err != nil {
		t.Fatalf("abort session: %v", err)
	}
	state, err := syncSvc.GetSession(ctx, userID, started.SessionID)
	if err != nil {
		t.Fatalf("get aborted session: %v", err)
	}
	if state.Status != models.SyncSessionStatusAborted {
		t.Fatalf("aborted state = %q", state.Status)
	}

	if _, err := syncSvc.ImportBundleJSON(ctx, userID, bundle); err != nil {
		t.Fatalf("import bundle json: %v", err)
	}
	preview, err := syncSvc.PreviewBundle(ctx, userID, bundle)
	if err != nil {
		t.Fatalf("preview bundle: %v", err)
	}
	if preview.Fingerprint == "" {
		t.Fatal("preview fingerprint is empty")
	}

	filtered, err := syncSvc.ExportBundleJSON(ctx, userID, models.BundleFilters{
		IncludeDomains: []string{"skills"},
		IncludeSkills:  []string{"atlas-brief", "atlas-layout"},
		ExcludeSkills:  []string{"atlas-layout"},
	})
	if err != nil {
		t.Fatalf("export filtered bundle: %v", err)
	}
	if len(filtered.Profile) != 0 || len(filtered.Memory) != 0 {
		t.Fatal("filtered export should exclude profile and memory domains")
	}
	if len(filtered.Skills) != 1 {
		t.Fatalf("filtered skills count = %d, want 1", len(filtered.Skills))
	}
	if _, ok := filtered.Skills["atlas-brief"]; !ok {
		t.Fatal("filtered export missing atlas-brief")
	}

	jobs, err := syncSvc.ListJobs(ctx, userID)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) < 3 {
		t.Fatalf("expected at least 3 jobs, got %d", len(jobs))
	}
	for _, job := range jobs {
		if job.Transport == models.SyncJobTransportJSON && job.Direction == models.SyncJobDirectionImport {
			return
		}
	}
	t.Fatal("missing json import job in history")
}
