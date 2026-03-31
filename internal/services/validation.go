package services

import (
	"fmt"
	"regexp"
)

var slugRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// validateSlug checks that a string contains only allowed characters and is within maxLen.
func validateSlug(s string, maxLen int) error {
	if s == "" {
		return fmt.Errorf("value is required")
	}
	if len(s) > maxLen {
		return fmt.Errorf("value exceeds maximum length of %d characters", maxLen)
	}
	if !slugRegex.MatchString(s) {
		return fmt.Errorf("value contains invalid characters (allowed: a-z, A-Z, 0-9, '.', '_', '-')")
	}
	return nil
}

const maxContentBytes = 64 * 1024 // 64 KB

// validateContentLength checks that a content string does not exceed the given limit in bytes.
func validateContentLength(content string, maxBytes int) error {
	if len(content) > maxBytes {
		return fmt.Errorf("content exceeds maximum size of %d bytes", maxBytes)
	}
	return nil
}
