package services

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agi-bar/agenthub/internal/database"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	bundleTestMigrationsOnce sync.Once
	bundleTestMigrationsErr  error
)

func loadRealisticSkillFixture(t *testing.T) map[string]string {
	t.Helper()

	reader, err := zip.OpenReader(filepath.Join("testdata", "ahub-sync.skill"))
	if err != nil {
		t.Fatalf("open realistic skill fixture: %v", err)
	}
	defer reader.Close()

	files := make(map[string]string)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		relPath := strings.TrimPrefix(file.Name, "pkg-skill/")
		if relPath == file.Name || relPath == "" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open fixture entry %q: %v", file.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read fixture entry %q: %v", file.Name, err)
		}
		files[relPath] = string(data)
	}
	return files
}

func readRealisticBinaryFixture(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", "tiny.png"))
	if err != nil {
		t.Fatalf("read realistic binary fixture: %v", err)
	}
	return data
}

func setupBundleIntegration(t *testing.T) (context.Context, uuid.UUID, *FileTreeService, *MemoryService, *ImportService, *ExportService) {
	t.Helper()

	dbURL := os.Getenv("AGENTHUB_TEST_DB")
	if dbURL == "" {
		t.Skip("AGENTHUB_TEST_DB not set; skipping bundle integration test")
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
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, slug, display_name, timezone, language, created_at, updated_at)
		 VALUES ($1, $2, $3, 'UTC', 'en', $4, $4)`,
		userID, "bundle-test-"+userID.String()[:8], "Bundle Test User", now,
	)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	fileTree := NewFileTreeService(pool)
	memory := NewMemoryService(pool, fileTree)
	importSvc := NewImportService(pool, fileTree, memory, nil)
	exportSvc := NewExportService(fileTree, memory, nil, nil, nil, nil, nil, nil)
	return ctx, userID, fileTree, memory, importSvc, exportSvc
}

func TestRealisticBundleFixture_LoadsActualSkillPackage(t *testing.T) {
	files := loadRealisticSkillFixture(t)
	if len(files) != 5 {
		t.Fatalf("fixture file count = %d, want 5", len(files))
	}

	if len(files["SKILL.md"]) < 1000 {
		t.Fatalf("SKILL.md fixture too small: %d bytes", len(files["SKILL.md"]))
	}
	if len(files["scripts/ahub-sync.py"]) < 7000 {
		t.Fatalf("scripts/ahub-sync.py fixture too small: %d bytes", len(files["scripts/ahub-sync.py"]))
	}
	if _, ok := files["spec/api.md"]; !ok {
		t.Fatal("fixture missing spec/api.md")
	}

	binary := readRealisticBinaryFixture(t)
	if len(binary) == 0 {
		t.Fatal("binary fixture is empty")
	}
	if !bytes.HasPrefix(binary, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		t.Fatal("binary fixture is not a PNG")
	}
}

func TestBundleImportExport_RoundTripRealisticFixture(t *testing.T) {
	ctx, userID, fileTree, memory, importSvc, exportSvc := setupBundleIntegration(t)

	files := loadRealisticSkillFixture(t)
	binary := readRealisticBinaryFixture(t)
	sum := sha256.Sum256(binary)

	createdAt := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(48 * time.Hour)

	bundle := models.Bundle{
		Version: models.BundleVersionV1,
		Source:  "realistic-fixture",
		Mode:    bundleModeMerge,
		Profile: map[string]string{
			"preferences": "简洁输出\n保留证据",
			"sync-notes":  "来自真实 ahub-sync.skill fixture",
		},
		Skills: map[string]models.BundleSkill{
			"ahub-sync": {
				Files: files,
				BinaryFiles: map[string]models.BundleBlobFile{
					"assets/tiny.png": {
						ContentBase64: base64.StdEncoding.EncodeToString(binary),
						ContentType:   "image/png",
						SizeBytes:     int64(len(binary)),
						SHA256:        hex.EncodeToString(sum[:]),
					},
				},
			},
		},
		Memory: []models.BundleMemoryItem{
			{
				Title:     "fixture-entry",
				Content:   "bundle import/export should stay idempotent",
				Source:    "fixture",
				CreatedAt: createdAt.Format(time.RFC3339),
				ExpiresAt: expiresAt.Format(time.RFC3339),
			},
		},
	}

	result, err := importSvc.ImportBundle(ctx, userID, bundle)
	if err != nil {
		t.Fatalf("initial import bundle: %v", err)
	}
	if result.SkillsWritten != 1 || result.FilesWritten != 6 || result.ProfileCategories != 2 || result.MemoryImported != 1 {
		t.Fatalf("unexpected import result: %+v", result)
	}

	skillDoc, err := fileTree.Read(ctx, userID, "/skills/ahub-sync/SKILL.md", models.TrustLevelFull)
	if err != nil {
		t.Fatalf("read imported SKILL.md: %v", err)
	}
	if skillDoc.Content != files["SKILL.md"] {
		t.Fatal("SKILL.md content mismatch after import")
	}

	binaryData, _, err := fileTree.ReadBinary(ctx, userID, "/skills/ahub-sync/assets/tiny.png", models.TrustLevelFull)
	if err != nil {
		t.Fatalf("read imported binary: %v", err)
	}
	if !bytes.Equal(binaryData, binary) {
		t.Fatal("binary payload mismatch after import")
	}

	if _, err := importSvc.ImportBundle(ctx, userID, bundle); err != nil {
		t.Fatalf("repeat merge import bundle: %v", err)
	}
	scratch, err := memory.GetScratchActive(ctx, userID)
	if err != nil {
		t.Fatalf("load scratch after repeat import: %v", err)
	}
	if len(scratch) != 1 {
		t.Fatalf("repeat import duplicated scratch entries: got %d want 1", len(scratch))
	}

	if _, err := fileTree.WriteEntry(ctx, userID, "/skills/ahub-sync/extra.md", "delete me", "text/markdown", models.FileTreeWriteOptions{
		MinTrustLevel: models.TrustLevelGuest,
	}); err != nil {
		t.Fatalf("write extra file: %v", err)
	}
	if _, err := fileTree.WriteEntry(ctx, userID, "/skills/keep-me/SKILL.md", "# Keep Me\n", "text/markdown", models.FileTreeWriteOptions{
		MinTrustLevel: models.TrustLevelGuest,
	}); err != nil {
		t.Fatalf("write untouched skill: %v", err)
	}

	mirrorBundle := bundle
	mirrorBundle.Mode = bundleModeMirror
	mirrorResult, err := importSvc.ImportBundle(ctx, userID, mirrorBundle)
	if err != nil {
		t.Fatalf("mirror import bundle: %v", err)
	}
	if mirrorResult.FilesDeleted != 1 {
		t.Fatalf("mirror deleted %d files, want 1", mirrorResult.FilesDeleted)
	}

	if _, err := fileTree.Read(ctx, userID, "/skills/ahub-sync/extra.md", models.TrustLevelFull); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("extra file should be deleted, got err=%v", err)
	}
	if _, err := fileTree.Read(ctx, userID, "/skills/keep-me/SKILL.md", models.TrustLevelFull); err != nil {
		t.Fatalf("untouched skill should remain: %v", err)
	}

	exported, err := exportSvc.ExportBundle(ctx, userID)
	if err != nil {
		t.Fatalf("export bundle: %v", err)
	}
	if exported.Version != models.BundleVersionV1 {
		t.Fatalf("exported version = %q", exported.Version)
	}
	if len(exported.Profile) != 2 {
		t.Fatalf("exported profile count = %d, want 2", len(exported.Profile))
	}
	if len(exported.Memory) != 1 {
		t.Fatalf("exported memory count = %d, want 1", len(exported.Memory))
	}

	exportedSkill, ok := exported.Skills["ahub-sync"]
	if !ok {
		t.Fatal("export missing ahub-sync skill")
	}
	if exportedSkill.Files["SKILL.md"] != files["SKILL.md"] {
		t.Fatal("exported SKILL.md content mismatch")
	}
	if _, ok := exportedSkill.Files["extra.md"]; ok {
		t.Fatal("export unexpectedly includes mirrored-away extra.md")
	}

	exportedBlob, ok := exportedSkill.BinaryFiles["assets/tiny.png"]
	if !ok {
		t.Fatal("export missing binary file")
	}
	if exportedBlob.ContentType != "image/png" || exportedBlob.SizeBytes != int64(len(binary)) || exportedBlob.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("unexpected exported blob metadata: %+v", exportedBlob)
	}

	decodedBlob, err := base64.StdEncoding.DecodeString(exportedBlob.ContentBase64)
	if err != nil {
		t.Fatalf("decode exported blob: %v", err)
	}
	if !bytes.Equal(decodedBlob, binary) {
		t.Fatal("exported blob content mismatch")
	}

	if _, ok := exported.Skills["keep-me"]; !ok {
		t.Fatal("export missing untouched skill")
	}
	if _, ok := exported.Skills["portability"]; ok {
		t.Fatal("export unexpectedly included system skill")
	}
}
