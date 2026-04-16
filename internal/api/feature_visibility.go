package api

import (
	"strings"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
)

var hiddenPublicFeaturePrefixes = []string{
	"/roles",
	"/inbox",
}

func isHiddenPublicFeaturePath(rawPath string) bool {
	publicPath := hubpath.NormalizePublic(rawPath)
	for _, prefix := range hiddenPublicFeaturePrefixes {
		if publicPath == prefix || strings.HasPrefix(publicPath, prefix+"/") {
			return true
		}
	}
	return false
}

func filterVisibleEntries(entries []models.FileTreeEntry) []models.FileTreeEntry {
	filtered := make([]models.FileTreeEntry, 0, len(entries))
	for _, entry := range entries {
		if isHiddenPublicFeaturePath(entry.Path) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func publicSearchPrefixes(scope string) []string {
	scope = strings.TrimSpace(scope)
	switch strings.ToLower(scope) {
	case "", "all":
		return []string{"/memory", "/projects", "/skills", "/platforms"}
	case "memory", "profile", "/memory", "/memory/":
		return []string{"/memory"}
	case "/memory/profile", "/memory/profile/":
		return []string{"/memory/profile"}
	case "projects", "project", "/projects", "/projects/":
		return []string{"/projects"}
	case "skills", "skill", "/skills", "/skills/":
		return []string{"/skills"}
	case "platform", "platforms", "/platforms", "/platforms/":
		return []string{"/platforms"}
	}
	if strings.HasPrefix(scope, "/") {
		return []string{scope}
	}
	return []string{"/memory", "/projects", "/skills", "/platforms"}
}
