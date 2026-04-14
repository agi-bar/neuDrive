package cli

import (
	"strings"
	"testing"
)

func TestRootCommandsHelpSurface(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		stdout, stderr, code := runRootForTest(t, "--help")
		if code != 0 {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "Root-directory command surface for local and remote neuDrive data.") {
			t.Fatalf("expected root usage in stdout, got %q", stdout)
		}
		if !strings.Contains(stdout, "neudrive help [topic]") {
			t.Fatalf("expected explicit help command in stdout, got %q", stdout)
		}
	})

	t.Run("help command", func(t *testing.T) {
		stdout, stderr, code := runRootForTest(t, "help", "write")
		if code != 0 {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		for _, expected := range []string{
			"neudrive write",
			"Create or update Hub content from literal text, stdin, or a local file path.",
			"Use `--literal` when an argument that looks like a path should stay plain text.",
		} {
			if !strings.Contains(stdout, expected) {
				t.Fatalf("expected %q in stdout, got %q", expected, stdout)
			}
		}
	})

	t.Run("help root alias", func(t *testing.T) {
		stdout, stderr, code := runRootForTest(t, "help", "project")
		if code != 0 {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		for _, expected := range []string{
			"neuDrive Path Model",
			"Public roots are `profile`, `memory`, `project`, `skill`, `secret`, and `platform`.",
			"`project/<name>` is a summary view.",
		} {
			if !strings.Contains(stdout, expected) {
				t.Fatalf("expected %q in stdout, got %q", expected, stdout)
			}
		}
	})

	cases := [][]string{
		{"ls", "--help"},
		{"read", "--help"},
		{"write", "--help"},
		{"search", "--help"},
		{"create", "--help"},
		{"log", "--help"},
		{"browse", "--help"},
		{"status", "--help"},
		{"doctor", "--help"},
		{"platform", "--help"},
		{"platform", "ls", "--help"},
		{"platform", "show", "--help"},
		{"connect", "--help"},
		{"disconnect", "--help"},
		{"import", "--help"},
		{"token", "--help"},
		{"token", "create", "--help"},
		{"stats", "--help"},
		{"export", "--help"},
		{"daemon", "--help"},
		{"server", "--help"},
		{"mcp", "--help"},
		{"mcp", "stdio", "--help"},
		{"sync", "--help"},
		{"sync", "login", "--help"},
		{"sync", "profiles", "--help"},
		{"sync", "use", "--help"},
		{"sync", "whoami", "--help"},
		{"sync", "logout", "--help"},
		{"sync", "export", "--help"},
		{"sync", "preview", "--help"},
		{"sync", "push", "--help"},
		{"sync", "pull", "--help"},
		{"sync", "resume", "--help"},
		{"sync", "history", "--help"},
		{"sync", "diff", "--help"},
		{"remote", "--help"},
		{"remote", "login", "--help"},
		{"remote", "use", "--help"},
		{"remote", "logout", "--help"},
		{"remote", "whoami", "--help"},
	}

	for _, args := range cases {
		name := strings.Join(args, " ")
		t.Run(name, func(t *testing.T) {
			stdout, stderr, code := runRootForTest(t, args...)
			if code != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			if strings.TrimSpace(stdout) == "" && strings.TrimSpace(stderr) == "" {
				t.Fatalf("expected help output for %v", args)
			}
		})
	}

	t.Run("write --help is descriptive", func(t *testing.T) {
		stdout, stderr, code := runRootForTest(t, "write", "--help")
		if code != 0 {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "Create or update Hub content from literal text, stdin, or a local file path.") {
			t.Fatalf("expected descriptive write help, got %q", stdout)
		}
	})

	t.Run("git --help is descriptive", func(t *testing.T) {
		stdout, stderr, code := runRootForTest(t, "git", "--help")
		if code != 0 {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "Mirror local neuDrive data into a local Git repository and keep it refreshed.") {
			t.Fatalf("expected descriptive git help, got %q", stdout)
		}
	})
}

func TestRootCommandsUsageAndExitCodes(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		want   int
		substr string
		stream string
	}{
		{name: "unknown root", args: []string{"wat"}, want: 2, substr: "unknown command", stream: "stderr"},
		{name: "platform unknown", args: []string{"platform", "wat"}, want: 2, substr: "unknown platform subcommand", stream: "stderr"},
		{name: "platform show missing", args: []string{"platform", "show"}, want: 2, substr: "usage: neudrive platform show <platform>", stream: "stderr"},
		{name: "read missing", args: []string{"read"}, want: 2, substr: "usage: neudrive read <path>", stream: "stderr"},
		{name: "write missing", args: []string{"write"}, want: 2, substr: "usage: neudrive write <path> <content-or-file>", stream: "stderr"},
		{name: "search missing", args: []string{"search"}, want: 2, substr: "usage: neudrive search <query> [path]", stream: "stderr"},
		{name: "create missing", args: []string{"create"}, want: 2, substr: "usage: neudrive create <category> <name>", stream: "stderr"},
		{name: "log missing", args: []string{"log"}, want: 2, substr: "usage: neudrive log <path>", stream: "stderr"},
		{name: "connect missing", args: []string{"connect"}, want: 2, substr: "usage: neudrive connect <platform>", stream: "stderr"},
		{name: "disconnect missing", args: []string{"disconnect"}, want: 2, substr: "usage: neudrive disconnect <platform>", stream: "stderr"},
		{name: "import missing", args: []string{"import"}, want: 0, substr: "Bring local files or platform exports into neuDrive.", stream: "stdout"},
		{name: "import zip invalid mode", args: []string{"import", "platform", "claude", "--zip", "skills.zip", "--mode", "agent"}, want: 2, substr: "--zip can only be combined with --mode files", stream: "stderr"},
		{name: "token missing", args: []string{"token"}, want: 0, substr: "Create short-lived tokens for sync or prepared skills upload workflows.", stream: "stdout"},
		{name: "export missing", args: []string{"export"}, want: 2, substr: "usage: neudrive export <platform> [--output DIR]", stream: "stderr"},
		{name: "browse extra", args: []string{"browse", "/one", "/two"}, want: 2, substr: "usage: neudrive browse [--print-url] [/route]", stream: "stderr"},
		{name: "daemon unknown", args: []string{"daemon", "wat"}, want: 2, substr: "unknown daemon subcommand", stream: "stderr"},
		{name: "sync unknown", args: []string{"sync", "wat"}, want: 2, substr: "unknown sync subcommand", stream: "stderr"},
		{name: "remote unknown", args: []string{"remote", "wat"}, want: 2, substr: "unknown remote subcommand", stream: "stderr"},
		{name: "remote login missing", args: []string{"remote", "login"}, want: 2, substr: "usage: neudrive remote login <profile>", stream: "stderr"},
		{name: "remote use missing", args: []string{"remote", "use"}, want: 2, substr: "usage: neudrive remote use <profile>", stream: "stderr"},
		{name: "help unknown topic", args: []string{"help", "wat"}, want: 2, substr: "available topics:", stream: "stderr"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runRootForTest(t, tc.args...)
			if code != tc.want {
				t.Fatalf("code=%d want=%d stdout=%q stderr=%q", code, tc.want, stdout, stderr)
			}
			got := stdout
			if tc.stream == "stderr" {
				got = stderr
			}
			if !strings.Contains(got, tc.substr) {
				t.Fatalf("expected %q in %s, got stdout=%q stderr=%q", tc.substr, tc.stream, stdout, stderr)
			}
		})
	}
}
