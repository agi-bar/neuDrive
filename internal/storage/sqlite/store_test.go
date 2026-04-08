package sqlite_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/agi-bar/agenthub/internal/storage/sqlite"
	"github.com/google/uuid"
)

type testServiceFixture struct {
	importSvc *services.ImportService
	exportSvc *services.ExportService
	syncSvc   *services.SyncService
}

func openTestStore(t *testing.T) (context.Context, *sqlite.Store, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	user, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	return ctx, store, user.ID
}

func newTestServiceFixture(store *sqlite.Store) *testServiceFixture {
	fileTree := services.NewFileTreeServiceWithRepo(sqlite.NewFileTreeRepo(store))
	memory := services.NewMemoryServiceWithRepo(sqlite.NewMemoryRepo(store), nil)
	project := services.NewProjectServiceWithRepo(sqlite.NewProjectRepo(store), nil, nil)
	importSvc := services.NewImportService(nil, fileTree, memory, nil)
	exportSvc := services.NewExportService(fileTree, memory, project, nil, nil, nil, nil, nil)
	syncSvc := services.NewSyncServiceWithRepo(sqlite.NewSyncRepo(store), importSvc, exportSvc, fileTree, memory)
	return &testServiceFixture{
		importSvc: importSvc,
		exportSvc: exportSvc,
		syncSvc:   syncSvc,
	}
}

func TestTokenLifecycle(t *testing.T) {
	ctx, store, userID := openTestStore(t)

	firstUser, err := store.EnsureOwner(ctx)
	if err != nil {
		t.Fatalf("EnsureOwner (again): %v", err)
	}
	if firstUser.ID != userID {
		t.Fatalf("owner changed: got %s want %s", firstUser.ID, userID)
	}

	created, err := store.CreateToken(ctx, userID, "test token", []string{models.ScopeReadBundle}, models.TrustLevelWork, time.Hour)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	validated, err := store.ValidateToken(ctx, created.Token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if validated.UserID != userID {
		t.Fatalf("ValidateToken user mismatch: got %s want %s", validated.UserID, userID)
	}
	if !models.HasScope(validated.Scopes, models.ScopeReadBundle) {
		t.Fatalf("ValidateToken scopes mismatch: got %v", validated.Scopes)
	}
	if err := store.RevokeToken(ctx, userID, validated.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if _, err := store.ValidateToken(ctx, created.Token); err == nil {
		t.Fatal("ValidateToken succeeded after revoke")
	}
}

func TestFileAndBlobRoundTrip(t *testing.T) {
	ctx, store, userID := openTestStore(t)

	entry, err := store.WriteEntry(ctx, userID, "/skills/demo/SKILL.md", "# Demo\n", "text/markdown", models.FileTreeWriteOptions{
		MinTrustLevel: models.TrustLevelGuest,
	})
	if err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	readBack, err := store.Read(ctx, userID, "/skills/demo/SKILL.md", models.TrustLevelGuest)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if readBack.Content != "# Demo\n" {
		t.Fatalf("Read content mismatch: got %q", readBack.Content)
	}

	binaryData := []byte{0x89, 'P', 'N', 'G', 0x00, 0x01, 0x02, 0x03}
	binaryEntry, err := store.WriteBinaryEntry(ctx, userID, "/skills/demo/assets/logo.png", binaryData, "image/png", models.FileTreeWriteOptions{
		MinTrustLevel: models.TrustLevelGuest,
	})
	if err != nil {
		t.Fatalf("WriteBinaryEntry: %v", err)
	}
	blob, ok, err := store.ReadBlobByEntryID(ctx, binaryEntry.ID)
	if err != nil {
		t.Fatalf("ReadBlobByEntryID: %v", err)
	}
	if !ok || string(blob) != string(binaryData) {
		t.Fatalf("blob mismatch: ok=%t len=%d", ok, len(blob))
	}
	readBinary, binaryMeta, err := store.ReadBinary(ctx, userID, "/skills/demo/assets/logo.png", models.TrustLevelGuest)
	if err != nil {
		t.Fatalf("ReadBinary: %v", err)
	}
	if string(readBinary) != string(binaryData) {
		t.Fatalf("ReadBinary mismatch: got %v want %v", readBinary, binaryData)
	}
	if binaryMeta.ContentType != "image/png" {
		t.Fatalf("binary content type mismatch: got %q", binaryMeta.ContentType)
	}

	overwritten, err := store.WriteEntry(ctx, userID, "/skills/demo/assets/logo.png", "plain text", "text/plain", models.FileTreeWriteOptions{
		MinTrustLevel: models.TrustLevelGuest,
	})
	if err != nil {
		t.Fatalf("WriteEntry overwrite: %v", err)
	}
	if overwritten.ID != binaryEntry.ID {
		t.Fatalf("expected overwrite to reuse entry id: got %s want %s", overwritten.ID, binaryEntry.ID)
	}
	if _, ok, err := store.ReadBlobByEntryID(ctx, binaryEntry.ID); err != nil {
		t.Fatalf("ReadBlobByEntryID after overwrite: %v", err)
	} else if ok {
		t.Fatal("blob still present after text overwrite")
	}
	if err := store.Delete(ctx, userID, entry.Path); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Read(ctx, userID, entry.Path, models.TrustLevelGuest); err == nil {
		t.Fatal("deleted text entry still readable")
	}
}

func TestBundleImportExportRoundTrip(t *testing.T) {
	ctx, sourceStore, sourceUserID := openTestStore(t)
	sourceServices := newTestServiceFixture(sourceStore)
	if err := sourceStore.UpsertProfile(ctx, sourceUserID, "preferences", "prefers local-first", "test"); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}
	createdAt := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(7 * 24 * time.Hour)
	if _, err := sourceStore.ImportScratch(ctx, sourceUserID, "remember this", "test", "note", createdAt, &expiresAt); err != nil {
		t.Fatalf("ImportScratch: %v", err)
	}
	if _, err := sourceStore.WriteEntry(ctx, sourceUserID, "/skills/demo/SKILL.md", "# Demo\n", "text/markdown", models.FileTreeWriteOptions{MinTrustLevel: models.TrustLevelGuest}); err != nil {
		t.Fatalf("WriteEntry skill: %v", err)
	}
	if _, err := sourceStore.WriteEntry(ctx, sourceUserID, "/skills/demo/references/guide.md", "guide", "text/markdown", models.FileTreeWriteOptions{MinTrustLevel: models.TrustLevelGuest}); err != nil {
		t.Fatalf("WriteEntry ref: %v", err)
	}
	logo := []byte{0x89, 'P', 'N', 'G', 0x00, 0x01, 0x02, 0x03}
	if _, err := sourceStore.WriteBinaryEntry(ctx, sourceUserID, "/skills/demo/assets/logo.png", logo, "image/png", models.FileTreeWriteOptions{MinTrustLevel: models.TrustLevelGuest}); err != nil {
		t.Fatalf("WriteBinaryEntry: %v", err)
	}

	exported, err := sourceServices.exportSvc.ExportBundle(ctx, sourceUserID)
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	if exported.Stats.TotalSkills != 1 || exported.Stats.TotalFiles != 3 || exported.Stats.BinaryFiles != 1 {
		t.Fatalf("unexpected export stats: %+v", exported.Stats)
	}

	_, targetStore, targetUserID := openTestStore(t)
	targetServices := newTestServiceFixture(targetStore)
	preview, err := targetServices.syncSvc.PreviewBundle(ctx, targetUserID, *exported)
	if err != nil {
		t.Fatalf("PreviewBundle: %v", err)
	}
	if preview.Summary.Create == 0 {
		t.Fatalf("expected preview create actions, got %+v", preview.Summary)
	}
	result, err := targetServices.importSvc.ImportBundle(ctx, targetUserID, *exported)
	if err != nil {
		t.Fatalf("ImportBundle: %v", err)
	}
	if result.SkillsWritten != 1 || result.FilesWritten != 3 || result.ProfileCategories != 1 || result.MemoryImported != 1 {
		t.Fatalf("unexpected import result: %+v", result)
	}

	reExported, err := targetServices.exportSvc.ExportBundle(ctx, targetUserID)
	if err != nil {
		t.Fatalf("ExportBundle reexport: %v", err)
	}
	if !bundlesEquivalent(*exported, *reExported) {
		left, _ := json.MarshalIndent(exported, "", "  ")
		right, _ := json.MarshalIndent(reExported, "", "  ")
		t.Fatalf("bundle mismatch after round trip\nleft=%s\nright=%s", left, right)
	}
}

func TestArchiveSessionCommitAndCleanup(t *testing.T) {
	ctx, sourceStore, sourceUserID := openTestStore(t)
	sourceServices := newTestServiceFixture(sourceStore)
	if _, err := sourceStore.WriteEntry(ctx, sourceUserID, "/skills/large/SKILL.md", "# Large\n", "text/markdown", models.FileTreeWriteOptions{MinTrustLevel: models.TrustLevelGuest}); err != nil {
		t.Fatalf("WriteEntry SKILL: %v", err)
	}
	largeBinary := make([]byte, (5<<20)+(512<<10))
	if _, err := rand.Read(largeBinary); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	if _, err := sourceStore.WriteBinaryEntry(ctx, sourceUserID, "/skills/large/assets/blob.bin", largeBinary, "application/octet-stream", models.FileTreeWriteOptions{MinTrustLevel: models.TrustLevelGuest}); err != nil {
		t.Fatalf("WriteBinaryEntry: %v", err)
	}
	archive, manifest, err := sourceServices.syncSvc.ExportArchive(ctx, sourceUserID, models.BundleFilters{})
	if err != nil {
		t.Fatalf("ExportArchive: %v", err)
	}

	_, targetStore, targetUserID := openTestStore(t)
	targetServices := newTestServiceFixture(targetStore)
	session, err := targetServices.syncSvc.StartSession(ctx, targetUserID, models.SyncStartSessionRequest{
		TransportVersion: models.SyncTransportVersionV1,
		Format:           models.BundleFormatArchive,
		Mode:             manifest.Mode,
		Manifest:         *manifest,
		ArchiveSizeBytes: int64(len(archive)),
		ArchiveSHA256:    manifest.ArchiveSHA256,
	})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if session.TotalParts < 2 {
		t.Fatalf("expected multi-part archive, got %d parts for %d bytes", session.TotalParts, len(archive))
	}
	chunkSize := int(session.ChunkSizeBytes)
	first, err := targetServices.syncSvc.UploadPart(ctx, targetUserID, session.SessionID, 0, archive[:chunkSize])
	if err != nil {
		t.Fatalf("UploadPart first: %v", err)
	}
	if first.Status != models.SyncSessionStatusUploading || len(first.MissingParts) == 0 {
		t.Fatalf("unexpected first part state: %+v", first)
	}
	second, err := targetServices.syncSvc.UploadPart(ctx, targetUserID, session.SessionID, 1, archive[chunkSize:])
	if err != nil {
		t.Fatalf("UploadPart second: %v", err)
	}
	if second.Status != models.SyncSessionStatusReady {
		t.Fatalf("expected ready session, got %+v", second)
	}
	importResult, err := targetServices.syncSvc.CommitSession(ctx, targetUserID, session.SessionID, models.SyncCommitRequest{})
	if err != nil {
		t.Fatalf("CommitSession: %v", err)
	}
	if importResult.SkillsWritten != 1 || importResult.FilesWritten != 2 {
		t.Fatalf("unexpected commit import result: %+v", importResult)
	}

	var remainingParts int
	if err := targetStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_session_parts WHERE session_id = ?`, session.SessionID.String()).Scan(&remainingParts); err != nil {
		t.Fatalf("count remaining parts: %v", err)
	}
	if remainingParts != 0 {
		t.Fatalf("expected session parts cleanup, got %d remaining", remainingParts)
	}
	job, err := targetServices.syncSvc.GetJob(ctx, targetUserID, session.JobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if job.Status != models.SyncJobStatusSucceeded || job.Transport != models.SyncJobTransportArchive {
		t.Fatalf("unexpected sync job after commit: %+v", job)
	}
	jobs, err := targetServices.syncSvc.ListJobs(ctx, targetUserID)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected single archive job, got %d", len(jobs))
	}
	exported, err := targetServices.exportSvc.ExportBundle(ctx, targetUserID)
	if err != nil {
		t.Fatalf("ExportBundle after commit: %v", err)
	}
	if _, ok := exported.Skills["large"]; !ok {
		t.Fatalf("expected imported large skill, got %v", exported.Skills)
	}
}

func TestCleanExpiredSyncSessions(t *testing.T) {
	ctx, store, userID := openTestStore(t)
	svcs := newTestServiceFixture(store)
	manifest := models.BundleArchiveManifest{
		Version:       models.BundleVersionV2,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Source:        "test",
		Mode:          "merge",
		ArchiveSHA256: strings.Repeat("a", 64),
	}
	session, err := svcs.syncSvc.StartSession(ctx, userID, models.SyncStartSessionRequest{
		TransportVersion: models.SyncTransportVersionV1,
		Format:           models.BundleFormatArchive,
		Mode:             "merge",
		Manifest:         manifest,
		ArchiveSizeBytes: 16,
		ArchiveSHA256:    manifest.ArchiveSHA256,
	})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if _, err := svcs.syncSvc.UploadPart(ctx, userID, session.SessionID, 0, []byte("partial")); err != nil {
		t.Fatalf("UploadPart: %v", err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	if _, err := store.DB().ExecContext(ctx, `UPDATE sync_sessions SET expires_at = ? WHERE id = ?`, past.UTC().Format(time.RFC3339Nano), session.SessionID.String()); err != nil {
		t.Fatalf("expire session: %v", err)
	}
	cleanup, err := svcs.syncSvc.CleanExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("CleanExpiredSyncSessions: %v", err)
	}
	if cleanup.ExpiredSessions != 1 || cleanup.DeletedBytes == 0 {
		t.Fatalf("unexpected cleanup result: %+v", cleanup)
	}
	if _, err := svcs.syncSvc.UploadPart(ctx, userID, session.SessionID, 0, []byte("retry")); err == nil || err != services.ErrSyncSessionExpired {
		t.Fatalf("expected expired session error, got %v", err)
	}
	job, err := svcs.syncSvc.GetJob(ctx, userID, session.JobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if job.Status != models.SyncJobStatusAborted {
		t.Fatalf("expected aborted cleanup job, got %+v", job)
	}
}

func bundlesEquivalent(left, right models.Bundle) bool {
	left.CreatedAt = ""
	right.CreatedAt = ""
	return reflect.DeepEqual(left, right)
}

func TestBundlePreviewIncludesBinarySHA(t *testing.T) {
	ctx, store, userID := openTestStore(t)
	svcs := newTestServiceFixture(store)
	data := []byte("binary-data")
	if _, err := store.WriteBinaryEntry(ctx, userID, "/skills/demo/assets/file.bin", data, "application/octet-stream", models.FileTreeWriteOptions{MinTrustLevel: models.TrustLevelGuest}); err != nil {
		t.Fatalf("WriteBinaryEntry: %v", err)
	}
	exported, err := svcs.exportSvc.ExportBundle(ctx, userID)
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	file := exported.Skills["demo"].BinaryFiles["assets/file.bin"]
	if decoded, err := base64.StdEncoding.DecodeString(file.ContentBase64); err != nil || string(decoded) != string(data) {
		t.Fatalf("binary export mismatch: err=%v len=%d", err, len(decoded))
	}
}
