package hubpath

import (
	"path"
	"strings"
)

const (
	skillsRoot = "/skills"
)

// NormalizeStorage converts paths into the canonical file-tree storage
// namespace. Skills now live under "/skills/" internally and publicly.
// Legacy ".skills/" inputs are still accepted and normalized.
func NormalizeStorage(raw string) string {
	return normalize(raw)
}

// NormalizePublic converts any accepted path form into the canonical public
// representation. Legacy ".skills/" inputs are normalized to "/skills/...".
func NormalizePublic(raw string) string {
	return normalize(raw)
}

// StorageToPublic converts a stored file-tree path into the canonical public
// path form.
func StorageToPublic(raw string) string {
	if raw == "" || raw == "/" {
		return "/"
	}
	return normalize(raw)
}

// IsSkillsStoragePath reports whether the stored path points at the canonical
// skills namespace.
func IsSkillsStoragePath(raw string) bool {
	p := NormalizeStorage(raw)
	return p == skillsRoot || strings.HasPrefix(p, skillsRoot+"/")
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

func normalize(raw string) string {
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
	case trimmed == "conversations" || strings.HasPrefix(trimmed, "conversations/"):
		trimmed = "conversations" + strings.TrimPrefix(trimmed, "conversations")
	case trimmed == "memory/conversations" || strings.HasPrefix(trimmed, "memory/conversations/"):
		trimmed = "conversations" + strings.TrimPrefix(trimmed, "memory/conversations")
	case trimmed == "skills" || strings.HasPrefix(trimmed, "skills/"):
		trimmed = "skills" + strings.TrimPrefix(trimmed, "skills")
	case trimmed == ".skills" || strings.HasPrefix(trimmed, ".skills/"):
		trimmed = "skills" + strings.TrimPrefix(trimmed, ".skills")
	}

	if hasTrailingSlash && trimmed != "" && !strings.HasSuffix(trimmed, "/") {
		trimmed += "/"
	}

	return "/" + trimmed
}
