package platforms

import "testing"

func TestResolveSupportsAliases(t *testing.T) {
	cases := map[string]string{
		"claude": "claude-code",
		"codex":  "codex",
		"gemini": "gemini-cli",
		"cursor": "cursor-agent",
	}
	for input, want := range cases {
		adapter, err := Resolve(input)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", input, err)
		}
		if adapter.ID() != want {
			t.Fatalf("Resolve(%q) => %q, want %q", input, adapter.ID(), want)
		}
	}
}
