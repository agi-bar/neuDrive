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
		if !strings.Contains(stdout, "Usage:") {
			t.Fatalf("expected root usage in stdout, got %q", stdout)
		}
	})

	cases := [][]string{
		{"status", "--help"},
		{"doctor", "--help"},
		{"platform", "--help"},
		{"platform", "ls", "--help"},
		{"platform", "show", "--help"},
		{"ls", "--help"},
		{"connect", "--help"},
		{"disconnect", "--help"},
		{"import", "--help"},
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
		{name: "platform show missing", args: []string{"platform", "show"}, want: 2, substr: "usage: agenthub platform show <platform>", stream: "stderr"},
		{name: "connect missing", args: []string{"connect"}, want: 2, substr: "usage: agenthub connect <platform>", stream: "stderr"},
		{name: "disconnect missing", args: []string{"disconnect"}, want: 2, substr: "usage: agenthub disconnect <platform>", stream: "stderr"},
		{name: "import missing", args: []string{"import"}, want: 0, substr: "usage: agenthub import <platform> [--mode agent|files|all]", stream: "stdout"},
		{name: "export missing", args: []string{"export"}, want: 2, substr: "usage: agenthub export <platform> [--output DIR]", stream: "stderr"},
		{name: "daemon unknown", args: []string{"daemon", "wat"}, want: 2, substr: "unknown daemon subcommand", stream: "stderr"},
		{name: "sync unknown", args: []string{"sync", "wat"}, want: 2, substr: "unknown sync subcommand", stream: "stderr"},
		{name: "remote unknown", args: []string{"remote", "wat"}, want: 2, substr: "unknown remote subcommand", stream: "stderr"},
		{name: "remote login missing", args: []string{"remote", "login"}, want: 2, substr: "usage: agenthub remote login <profile>", stream: "stderr"},
		{name: "remote use missing", args: []string{"remote", "use"}, want: 2, substr: "usage: agenthub remote use <profile>", stream: "stderr"},
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
