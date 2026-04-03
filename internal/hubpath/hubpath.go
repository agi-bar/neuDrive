package hubpath

import (
	"path"
	"strings"
)

const (
	publicSkillsRoot  = "/skills"
	storageSkillsRoot = ".skills"
)

// NormalizeStorage converts user-facing paths into the canonical file-tree
// storage namespace. Skills always live under ".skills/" internally.
func NormalizeStorage(raw string) string {
	return normalize(raw, true)
}

// NormalizePublic converts any accepted path form into the canonical public
// representation. Skills are always exposed as "/skills/...".
func NormalizePublic(raw string) string {
	return normalize(raw, false)
}

// StorageToPublic converts a stored file-tree path into the public path form.
func StorageToPublic(raw string) string {
	if raw == "" || raw == "/" {
		return "/"
	}
	return normalize(raw, false)
}

// IsSkillsStoragePath reports whether the stored path points at the internal
// skills namespace.
func IsSkillsStoragePath(raw string) bool {
	p := NormalizeStorage(raw)
	return p == storageSkillsRoot || strings.HasPrefix(p, storageSkillsRoot+"/")
}

// BaseName returns the final visible path segment.
func BaseName(raw string) string {
	p := NormalizePublic(raw)
	if p == "/" {
		return "/"
	}
	trimmed := strings.TrimSuffix(strings.TrimPrefix(p, "/"), "/")
	if trimmed == "" {
		return "/"
	}
	parts := strings.Split(trimmed, "/")
	return parts[len(parts)-1]
}

func normalize(raw string, storage bool) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" || raw == "/" {
		return "/"
	}

	hasTrailingSlash := strings.HasSuffix(raw, "/")
	cleaned := path.Clean("/" + strings.TrimLeft(raw, "/"))
	if cleaned == "." || cleaned == "/" {
		return "/"
	}

	trimmed := strings.TrimPrefix(cleaned, "/")
	switch {
	case trimmed == "skills" || strings.HasPrefix(trimmed, "skills/"):
		trimmed = storageSkillsRoot + strings.TrimPrefix(trimmed, "skills")
	case trimmed == storageSkillsRoot || strings.HasPrefix(trimmed, storageSkillsRoot+"/"):
		// Already canonical for storage.
	}

	if !storage && (trimmed == storageSkillsRoot || strings.HasPrefix(trimmed, storageSkillsRoot+"/")) {
		trimmed = "skills" + strings.TrimPrefix(trimmed, storageSkillsRoot)
	}

	if hasTrailingSlash && trimmed != "" && !strings.HasSuffix(trimmed, "/") {
		trimmed += "/"
	}

	if storage {
		if trimmed == storageSkillsRoot || strings.HasPrefix(trimmed, storageSkillsRoot+"/") {
			return trimmed
		}
		return "/" + trimmed
	}
	return "/" + trimmed
}
