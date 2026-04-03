package api

import "testing"

func TestSkillDescriptionUsesFirstBodyParagraph(t *testing.T) {
	t.Parallel()

	markdown := "# Demo Skill\n\nFirst paragraph line one.\nLine two.\n\nSecond paragraph."
	if got := skillDescription(markdown); got != "First paragraph line one. Line two." {
		t.Fatalf("skillDescription() = %q", got)
	}
}

func TestSkillDescriptionEmptyWhenNoBodyParagraph(t *testing.T) {
	t.Parallel()

	if got := skillDescription("# Demo Skill\n\n"); got != "" {
		t.Fatalf("skillDescription() = %q, want empty string", got)
	}
}
