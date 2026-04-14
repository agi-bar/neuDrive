package cli

import (
	"os"
	"strings"
	"testing"
)

func TestUsageLineUsesInvokedCommandName(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() {
		os.Args = origArgs
	})

	os.Args = []string{"neu"}
	if got := usageLine("connect <platform>"); got != "usage: neu connect <platform>" {
		t.Fatalf("got %q", got)
	}

	os.Args = []string{"neudrive"}
	if got := usageLine("connect <platform>"); got != "usage: neudrive connect <platform>" {
		t.Fatalf("got %q", got)
	}
}

func TestRootUsageRendersShortCommandWhenInvokedAsNeu(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() {
		os.Args = origArgs
	})

	os.Args = []string{"neu"}
	stdout, stderr, code := captureRunForTest(t, func() int {
		return Run(nil)
	})
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "neu status") {
		t.Fatalf("expected short command in help output: %q", stdout)
	}
	if strings.Contains(stdout, "neudrive status") {
		t.Fatalf("did not expect canonical command in short help output: %q", stdout)
	}
}
