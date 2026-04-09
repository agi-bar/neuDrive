package hubpath

import "testing"

func TestNormalizeStorageAndPublic(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input   string
		storage string
		public  string
	}{
		{input: "/skills/demo/SKILL.md", storage: "/skills/demo/SKILL.md", public: "/skills/demo/SKILL.md"},
		{input: "skills/demo/", storage: "/skills/demo/", public: "/skills/demo/"},
		{input: "/.skills/demo/notes.md", storage: "/skills/demo/notes.md", public: "/skills/demo/notes.md"},
		{input: "/projects/demo/context.md", storage: "/projects/demo/context.md", public: "/projects/demo/context.md"},
		{input: "/", storage: "/", public: "/"},
	}

	for _, tc := range cases {
		if got := NormalizeStorage(tc.input); got != tc.storage {
			t.Fatalf("NormalizeStorage(%q) = %q, want %q", tc.input, got, tc.storage)
		}
		if got := NormalizePublic(tc.input); got != tc.public {
			t.Fatalf("NormalizePublic(%q) = %q, want %q", tc.input, got, tc.public)
		}
		if got := StorageToPublic(tc.storage); got != tc.public {
			t.Fatalf("StorageToPublic(%q) = %q, want %q", tc.storage, got, tc.public)
		}
	}
}

func TestBaseName(t *testing.T) {
	t.Parallel()

	if got := BaseName("/skills/demo/"); got != "demo" {
		t.Fatalf("BaseName(skill dir) = %q, want demo", got)
	}
	if got := BaseName("/projects/demo/context.md"); got != "context.md" {
		t.Fatalf("BaseName(project file) = %q, want context.md", got)
	}
}
