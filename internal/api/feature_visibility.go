package api

import (
	"strings"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
)

var hiddenPublicFeaturePrefixes = []string{
	"/devices",
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
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "memory":
		return []string{"/memory", "/identity"}
	case "projects":
		return []string{"/projects"}
	case "skills":
		return []string{"/skills"}
	default:
		return []string{"/memory", "/identity", "/projects", "/skills"}
	}
}
