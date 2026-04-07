package skillsarchive

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type Entry struct {
	SkillName string
	RelPath   string
	Data      []byte
}

func ParseZipBytes(data []byte, archiveName string) ([]Entry, error) {
	reader := bytes.NewReader(data)
	return ParseZipReader(reader, int64(len(data)), archiveName)
}

func ParseZipReader(readerAt io.ReaderAt, size int64, archiveName string) ([]Entry, error) {
	zr, err := zip.NewReader(readerAt, size)
	if err != nil {
		return nil, fmt.Errorf("open skills archive: %w", err)
	}

	inferredSkill := InferArchiveSkillName(archiveName)
	entries := make([]Entry, 0, len(zr.File))
	hasSkillManifest := map[string]bool{}

	for _, file := range zr.File {
		if file.FileInfo().IsDir() {
			continue
		}
		skillName, relPath, ok := classifyEntry(file.Name, inferredSkill)
		if !ok {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open archive entry %s: %w", file.Name, err)
		}
		entryData, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read archive entry %s: %w", file.Name, err)
		}
		entries = append(entries, Entry{
			SkillName: skillName,
			RelPath:   relPath,
			Data:      entryData,
		})
		if relPath == "SKILL.md" {
			hasSkillManifest[skillName] = true
		}
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no skill files found in archive")
	}
	for _, entry := range entries {
		if !hasSkillManifest[entry.SkillName] {
			return nil, fmt.Errorf("archive is missing %s/SKILL.md", entry.SkillName)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].SkillName == entries[j].SkillName {
			return entries[i].RelPath < entries[j].RelPath
		}
		return entries[i].SkillName < entries[j].SkillName
	})
	return entries, nil
}

func InferArchiveSkillName(archiveName string) string {
	base := filepath.Base(strings.TrimSpace(archiveName))
	switch ext := strings.ToLower(filepath.Ext(base)); ext {
	case ".zip", ".skill":
		base = strings.TrimSuffix(base, ext)
	}
	base = strings.TrimSuffix(base, ".skill")
	base = strings.TrimSpace(base)
	if base == "" {
		return "imported-skill"
	}
	return base
}

func DetectContentType(pathValue string, data []byte) string {
	if ext := strings.TrimSpace(strings.ToLower(filepath.Ext(pathValue))); ext != "" {
		if byExt := mime.TypeByExtension(ext); byExt != "" {
			return byExt
		}
	}
	return http.DetectContentType(data)
}

func LooksBinary(pathValue string, data []byte) bool {
	switch strings.ToLower(filepath.Ext(pathValue)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".pdf", ".zip", ".skill", ".bin", ".ico", ".woff", ".woff2", ".ttf":
		return true
	}
	if len(data) == 0 {
		return false
	}
	if !utf8.Valid(data) {
		return true
	}
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func classifyEntry(name, inferredSkill string) (string, string, bool) {
	clean := path.Clean(strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "/"))
	if clean == "." || clean == "" {
		return "", "", false
	}
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return "", "", false
	}
	if strings.HasPrefix(clean, "__MACOSX/") || strings.HasSuffix(clean, "/.DS_Store") || path.Base(clean) == ".DS_Store" {
		return "", "", false
	}

	parts := strings.Split(clean, "/")
	if len(parts) == 1 {
		if inferredSkill == "" {
			return "", "", false
		}
		return inferredSkill, parts[0], true
	}

	skillName := strings.TrimSpace(strings.TrimSuffix(parts[0], ".skill"))
	if skillName == "" || skillName == "." {
		return "", "", false
	}
	relPath := path.Clean(strings.Join(parts[1:], "/"))
	if relPath == "." || relPath == "" || strings.HasPrefix(relPath, "../") {
		return "", "", false
	}
	return skillName, relPath, true
}
