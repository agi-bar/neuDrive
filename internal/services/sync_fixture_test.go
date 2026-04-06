package services

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
)

type syncFixturePlan struct {
	SkillNames        []string              `json:"skill_names"`
	ExtraTextFiles    []syncFixtureTextFile `json:"extra_text_files"`
	BinaryAssignments map[string][]string   `json:"binary_assignments"`
}

type syncFixtureTextFile struct {
	Path   string `json:"path"`
	Source string `json:"source"`
	Repeat int    `json:"repeat"`
}

func loadSyncFixturePlan(t *testing.T) syncFixturePlan {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "sync-fixture-plan.json"))
	if err != nil {
		t.Fatalf("read sync fixture plan: %v", err)
	}
	var plan syncFixturePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatalf("decode sync fixture plan: %v", err)
	}
	return plan
}

func buildLargeFixtureBundle(t *testing.T, multiplier int) models.Bundle {
	t.Helper()
	if multiplier <= 0 {
		multiplier = 1
	}

	baseFiles := loadRealisticSkillFixture(t)
	binary := readRealisticBinaryFixture(t)
	plan := loadSyncFixturePlan(t)

	bundle := models.Bundle{
		Version:   models.BundleVersionV1,
		CreatedAt: time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Source:    "sync-fixture",
		Mode:      bundleModeMerge,
		Profile: map[string]string{
			"preferences": strings.Repeat("保持完整同步\n", 10*multiplier),
			"principles":  strings.Repeat("导入前先预览\n", 8*multiplier),
		},
		Skills: map[string]models.BundleSkill{},
		Memory: []models.BundleMemoryItem{
			{
				Title:     "fixture-memory",
				Content:   strings.Repeat("同步流程演练\n", 60*multiplier),
				Source:    "fixture",
				CreatedAt: time.Date(2026, 4, 6, 9, 30, 0, 0, time.UTC).Format(time.RFC3339),
				ExpiresAt: time.Date(2026, 4, 9, 9, 30, 0, 0, time.UTC).Format(time.RFC3339),
			},
		},
	}

	for _, skillName := range plan.SkillNames {
		skill := models.BundleSkill{
			Files:       map[string]string{},
			BinaryFiles: map[string]models.BundleBlobFile{},
		}
		for relPath, content := range baseFiles {
			skill.Files[relPath] = content
		}
		for _, extra := range plan.ExtraTextFiles {
			sourceContent, ok := baseFiles[extra.Source]
			if !ok {
				t.Fatalf("fixture plan references missing source %q", extra.Source)
			}
			skill.Files[extra.Path] = strings.Repeat(sourceContent+"\n", extra.Repeat*multiplier)
		}
		for _, relPath := range plan.BinaryAssignments[skillName] {
			expandedBinary := expandedBinaryFixture(binary, skillName+":"+relPath, multiplier)
			skill.BinaryFiles[relPath] = models.BundleBlobFile{
				ContentBase64: base64.StdEncoding.EncodeToString(expandedBinary),
				ContentType:   "image/png",
				SizeBytes:     int64(len(expandedBinary)),
				SHA256:        hex.EncodeToString(sha256Bytes(expandedBinary)),
			}
		}
		bundle.Skills[skillName] = skill
	}
	bundle.Stats = recalculateBundleStats(bundle)
	return bundle
}

func materializeFixtureSource(t *testing.T, multiplier int) string {
	t.Helper()
	if multiplier <= 0 {
		multiplier = 1
	}
	baseFiles := loadRealisticSkillFixture(t)
	binary := readRealisticBinaryFixture(t)
	plan := loadSyncFixturePlan(t)
	root := t.TempDir()
	for _, skillName := range plan.SkillNames {
		skillRoot := filepath.Join(root, skillName)
		if err := os.MkdirAll(skillRoot, 0o755); err != nil {
			t.Fatalf("create skill root %q: %v", skillRoot, err)
		}
		for relPath, content := range baseFiles {
			target := filepath.Join(skillRoot, filepath.FromSlash(relPath))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("create parent %q: %v", target, err)
			}
			if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
				t.Fatalf("write base file %q: %v", target, err)
			}
		}
		for _, extra := range plan.ExtraTextFiles {
			sourceContent := baseFiles[extra.Source]
			target := filepath.Join(skillRoot, filepath.FromSlash(extra.Path))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("create parent %q: %v", target, err)
			}
			if err := os.WriteFile(target, []byte(strings.Repeat(sourceContent+"\n", extra.Repeat*multiplier)), 0o644); err != nil {
				t.Fatalf("write extra file %q: %v", target, err)
			}
		}
		for _, relPath := range plan.BinaryAssignments[skillName] {
			target := filepath.Join(skillRoot, filepath.FromSlash(relPath))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("create parent %q: %v", target, err)
			}
			if err := os.WriteFile(target, expandedBinaryFixture(binary, skillName+":"+relPath, multiplier), 0o644); err != nil {
				t.Fatalf("write binary %q: %v", target, err)
			}
		}
	}
	return root
}

func sha256Bytes(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func expandedBinaryFixture(base []byte, seed string, multiplier int) []byte {
	if multiplier <= 0 {
		multiplier = 1
	}
	targetSize := len(base) + multiplier*(256<<10)
	payload := make([]byte, 0, targetSize)
	payload = append(payload, base...)
	counter := 0
	for len(payload) < targetSize {
		blockSeed := []byte(seed + ":" + strconv.Itoa(counter))
		hash := sha256.Sum256(blockSeed)
		payload = append(payload, hash[:]...)
		counter++
	}
	return payload[:targetSize]
}
