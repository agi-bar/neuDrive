package hubpath

import (
	"fmt"
	"path"
	"strings"
	"time"
	"unicode"
)

func IdentityProfilePath() string {
	return "/identity/profile.json"
}

func ProfilePath(category string) string {
	return fmt.Sprintf("/memory/profile/%s.md", sanitizeSegment(category, "profile"))
}

func ScratchPath(ts time.Time, slug string) string {
	date := ts.UTC().Format("2006-01-02")
	return fmt.Sprintf("/memory/scratch/%s/%s.md", date, sanitizeSegment(slug, "entry"))
}

func ProjectDir(name string) string {
	return fmt.Sprintf("/projects/%s/", sanitizeSegment(name, "project"))
}

func ProjectContextPath(name string) string {
	return path.Join(ProjectDir(name), "context.md")
}

func ProjectLogPath(name string) string {
	return path.Join(ProjectDir(name), "log.jsonl")
}

func SkillDocPath(name string) string {
	return fmt.Sprintf("/skills/%s/SKILL.md", sanitizeSegment(name, "skill"))
}

func ConversationsRoot() string {
	return "/conversations"
}

func ConversationPlatformDir(platform string) string {
	return fmt.Sprintf("%s/%s/", ConversationsRoot(), sanitizeSegment(platform, "conversation"))
}

func ConversationDir(platform, key string) string {
	return fmt.Sprintf("%s%s/", ConversationPlatformDir(platform), sanitizeSegment(key, "conversation"))
}

func ConversationTranscriptPath(platform, key string) string {
	return path.Join(ConversationDir(platform, key), "conversation.md")
}

func ConversationDocumentPath(platform, key string) string {
	return path.Join(ConversationDir(platform, key), "conversation.json")
}

func ConversationExportPath(platform, key, target string) string {
	return path.Join(ConversationDir(platform, key), fmt.Sprintf("resume-%s.md", sanitizeSegment(target, "platform")))
}

func ConversationIndexPath(platform string) string {
	return path.Join(ConversationPlatformDir(platform), "index.json")
}

func RoleSkillPath(name string) string {
	return fmt.Sprintf("/roles/%s/SKILL.md", sanitizeSegment(name, "role"))
}

func InboxMessagePath(role, status, messageID string) string {
	return fmt.Sprintf(
		"/inbox/%s/%s/%s.json",
		sanitizePathSegment(role, "default"),
		sanitizeSegment(status, "incoming"),
		sanitizeSegment(messageID, "message"),
	)
}

func sanitizeSegment(raw string, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-_.")
	if out == "" {
		return fallback
	}
	return out
}

func sanitizePathSegment(raw string, fallback string) string {
	return sanitizeSegment(strings.ReplaceAll(raw, "/", "-"), fallback)
}
