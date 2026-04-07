package cli

import (
	"os"
	"strings"
	"testing"
)

func TestAgenthubDaemonCommands_LocalSQLite(t *testing.T) {
	binary := buildAgenthubBinary(t)
	env, _, statePath, _, _ := isolatedAgenthubEnv(t)

	stdout, _ := mustRunAgenthub(t, binary, env, "status")
	if !strings.Contains(stdout, "Local storage: sqlite") || !strings.Contains(stdout, "Local daemon: stopped") {
		t.Fatalf("unexpected status output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "doctor")
	if !strings.Contains(stdout, "- local daemon: not running") || !strings.Contains(stdout, "native Go runtime available") {
		t.Fatalf("unexpected doctor output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "daemon", "status")
	if !strings.Contains(stdout, "Local daemon is stopped.") {
		t.Fatalf("unexpected daemon status before start: %s", stdout)
	}

	mustRunAgenthub(t, binary, env, "sync", "history")

	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected runtime state after bootstrap: %v", err)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "status")
	if !strings.Contains(stdout, "Local daemon: running") {
		t.Fatalf("expected running daemon in status output: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "daemon", "status")
	if !strings.Contains(stdout, "Local daemon running at") {
		t.Fatalf("unexpected daemon status after bootstrap: %s", stdout)
	}

	stdout, _ = mustRunAgenthub(t, binary, env, "daemon", "logs", "--tail", "20")
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("expected daemon logs output")
	}

	mustRunAgenthub(t, binary, env, "daemon", "stop")

	stdout, _ = mustRunAgenthub(t, binary, env, "daemon", "status")
	if !strings.Contains(stdout, "Local daemon is stopped.") {
		t.Fatalf("unexpected daemon status after stop: %s", stdout)
	}
}
