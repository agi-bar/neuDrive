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

type archiveFile struct {
	Name string
	Data []byte
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
	files := make([]archiveFile, 0, len(zr.File))

	for _, file := range zr.File {
		if file.FileInfo().IsDir() {
			continue
		}
		cleanName, ok := normalizeArchivePath(file.Name)
		if !ok {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open archive entry %s: %w", cleanName, err)
		}
		entryData, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read archive entry %s: %w", cleanName, err)
		}
		files = append(files, archiveFile{Name: cleanName, Data: entryData})
	}

	manifestRoots := detectManifestRoots(files, inferredSkill)
	entries := entriesFromManifestRoots(files, manifestRoots)
	if len(entries) == 0 {
		entries = entriesFromLegacyLayout(files, inferredSkill)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no skill files found in archive")
	}
	hasSkillManifest := map[string]bool{}
	for _, entry := range entries {
		if entry.RelPath == "SKILL.md" {
			hasSkillManifest[entry.SkillName] = true
		}
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

func normalizeArchivePath(name string) (string, bool) {
	clean := path.Clean(strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "/"))
	if clean == "." || clean == "" {
		return "", false
	}
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return "", false
	}
	if strings.HasPrefix(clean, "__MACOSX/") || strings.HasSuffix(clean, "/.DS_Store") || path.Base(clean) == ".DS_Store" {
		return "", false
	}
	return clean, true
}

func detectManifestRoots(files []archiveFile, inferredSkill string) map[string]string {
	roots := make(map[string]string)
	for _, file := range files {
		if path.Base(file.Name) != "SKILL.md" {
			continue
		}
		root := path.Dir(file.Name)
		skillName := inferredSkill
		if root != "." {
			skillName = strings.TrimSpace(strings.TrimSuffix(path.Base(root), ".skill"))
		}
		if skillName == "" {
			continue
		}
		roots[root] = skillName
	}
	return roots
}

func entriesFromManifestRoots(files []archiveFile, manifestRoots map[string]string) []Entry {
	if len(manifestRoots) == 0 {
		return nil
	}
	roots := make([]string, 0, len(manifestRoots))
	for root := range manifestRoots {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		iDepth := strings.Count(roots[i], "/")
		jDepth := strings.Count(roots[j], "/")
		if iDepth != jDepth {
			return iDepth > jDepth
		}
		return len(roots[i]) > len(roots[j])
	})

	entries := make([]Entry, 0, len(files))
	for _, file := range files {
		for _, root := range roots {
			relPath, ok := relPathWithinRoot(file.Name, root)
			if !ok {
				continue
			}
			entries = append(entries, Entry{
				SkillName: manifestRoots[root],
				RelPath:   relPath,
				Data:      file.Data,
			})
			break
		}
	}
	return entries
}

func entriesFromLegacyLayout(files []archiveFile, inferredSkill string) []Entry {
	entries := make([]Entry, 0, len(files))
	for _, file := range files {
		skillName, relPath, ok := classifyLegacyEntry(file.Name, inferredSkill)
		if !ok {
			continue
		}
		entries = append(entries, Entry{
			SkillName: skillName,
			RelPath:   relPath,
			Data:      file.Data,
		})
	}
	return entries
}

func relPathWithinRoot(fileName, root string) (string, bool) {
	if root == "." {
		if fileName == "" {
			return "", false
		}
		return fileName, true
	}
	prefix := root + "/"
	if !strings.HasPrefix(fileName, prefix) {
		return "", false
	}
	relPath := strings.TrimPrefix(fileName, prefix)
	if relPath == "" || strings.HasPrefix(relPath, "../") {
		return "", false
	}
	return relPath, true
}

func classifyLegacyEntry(name, inferredSkill string) (string, string, bool) {
	clean, ok := normalizeArchivePath(name)
	if !ok {
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
