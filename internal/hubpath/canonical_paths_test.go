package hubpath

import (
	"strings"
	"testing"
	"time"
)

func TestCanonicalPaths(t *testing.T) {
	ts := time.Date(2026, 4, 4, 10, 30, 0, 0, time.UTC)

	if got := ProfilePath("preferences"); got != "/memory/profile/preferences.md" {
		t.Fatalf("ProfilePath() = %q", got)
	}
	if got := ProjectContextPath("demo"); got != "/projects/demo/context.md" {
		t.Fatalf("ProjectContextPath() = %q", got)
	}
	if got := InboxMessagePath("worker:policy@de.hub", "incoming", "abc123"); !strings.Contains(got, "/incoming/abc123.json") {
		t.Fatalf("InboxMessagePath() = %q", got)
	}
	if got := ScratchPath(ts, "Daily Summary"); got != "/memory/scratch/2026-04-04/daily-summary.md" {
		t.Fatalf("ScratchPath() = %q", got)
	}
}
