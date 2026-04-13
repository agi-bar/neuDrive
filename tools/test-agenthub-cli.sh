#!/usr/bin/env bash
set -u
set -o pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_ROOT=""
KEEP_TMP=0

PASS_COUNT=0
FAIL_COUNT=0
TOTAL_COUNT=0

CURRENT_TEST_NAME=""
CURRENT_TEST_FAILED=0
CURRENT_TEST_MESSAGES=()

LAST_CMD=()
LAST_STDOUT=""
LAST_STDERR=""
LAST_STATUS=0

AGENTHUB_BIN=""
OWNER_TOKEN=""
API_BASE=""
CONFIG_PATH=""
RUNTIME_PATH=""

usage() {
  cat <<'EOF'
Usage: tools/test-agenthub-cli.sh [--keep-tmp]

Run a safe smoke suite for the public Agent Hub CLI surface.

Safety:
  - Reuses the normal Go build caches and module cache
  - Forces serialized builds to reduce load
  - Uses an isolated HOME/XDG root only for Agent Hub config/data
  - Does not run the older heavy platform/import/git stress flow

Scope:
  - help
  - ls
  - read
  - write
  - search
  - create
  - log
  - stats
  - secret read/list via a seeded local fixture secret

Options:
  --keep-tmp   Preserve the temporary test directory even on success
  -h, --help   Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --keep-tmp)
      KEEP_TMP=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

cleanup() {
  if [[ -n "$AGENTHUB_BIN" && -x "$AGENTHUB_BIN" ]]; then
    "$AGENTHUB_BIN" daemon stop >/dev/null 2>&1 || true
  fi
  if [[ -n "$RUNTIME_PATH" && -f "$RUNTIME_PATH" ]]; then
    local daemon_pid=""
    daemon_pid="$(python3 - "$RUNTIME_PATH" <<'PY' 2>/dev/null || true
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    state = json.load(fh)
pid = state.get("pid")
if pid:
    print(pid)
PY
)"
    if [[ -n "$daemon_pid" ]]; then
      kill "$daemon_pid" >/dev/null 2>&1 || true
    fi
  fi
  if [[ -n "$TMP_ROOT" ]]; then
    if [[ "$KEEP_TMP" -eq 1 || "$FAIL_COUNT" -gt 0 ]]; then
      echo "Temporary directory preserved at: $TMP_ROOT"
    else
      chmod -R u+w "$TMP_ROOT" >/dev/null 2>&1 || true
      rm -rf "$TMP_ROOT"
    fi
  fi
}
trap cleanup EXIT

need_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "missing required command: $cmd" >&2
    exit 1
  fi
}

run_cmd() {
  local stdout_file stderr_file quoted=""
  LAST_CMD=("$@")
  for arg in "${LAST_CMD[@]}"; do
    quoted="${quoted} $(printf '%q' "$arg")"
  done
  echo
  echo "\$${quoted}"
  stdout_file="$TMP_ROOT/last.stdout"
  stderr_file="$TMP_ROOT/last.stderr"
  : >"$stdout_file"
  : >"$stderr_file"
  "${LAST_CMD[@]}" >"$stdout_file" 2>"$stderr_file"
  LAST_STATUS=$?
  LAST_STDOUT="$(cat "$stdout_file")"
  LAST_STDERR="$(cat "$stderr_file")"
  if [[ -n "$LAST_STDOUT" ]]; then
    printf '%s\n' "$LAST_STDOUT"
  fi
  if [[ -n "$LAST_STDERR" ]]; then
    printf '%s\n' "$LAST_STDERR" >&2
  fi
  return 0
}

start_test() {
  CURRENT_TEST_NAME="$1"
  CURRENT_TEST_FAILED=0
  CURRENT_TEST_MESSAGES=()
  echo
  echo "== $CURRENT_TEST_NAME =="
}

fail_current() {
  CURRENT_TEST_FAILED=1
  CURRENT_TEST_MESSAGES+=("$1")
}

finish_test() {
  TOTAL_COUNT=$((TOTAL_COUNT + 1))
  if [[ "$CURRENT_TEST_FAILED" -eq 0 ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    echo "[PASS] $CURRENT_TEST_NAME"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    echo "[FAIL] $CURRENT_TEST_NAME" >&2
    local msg
    for msg in "${CURRENT_TEST_MESSAGES[@]}"; do
      echo "  - $msg" >&2
    done
  fi
}

assert_command_ok() {
  if [[ "$LAST_STATUS" -ne 0 ]]; then
    fail_current "command exited with status $LAST_STATUS"
  fi
}

assert_nonempty() {
  local value="$1"
  local label="$2"
  if [[ -z "$value" ]]; then
    fail_current "$label is empty"
  fi
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    fail_current "$label missing: $needle"
  fi
}

run_setup() {
  local label="$1"
  shift
  echo
  echo "-- setup: $label --"
  run_cmd "$@"
  if [[ "$LAST_STATUS" -ne 0 ]]; then
    echo "setup failed: $label" >&2
    exit 1
  fi
}

read_json_field() {
  local json_path="$1"
  local field_path="$2"
  python3 - "$json_path" "$field_path" <<'PY'
import json
import sys

json_path, field_path = sys.argv[1], sys.argv[2]
with open(json_path, "r", encoding="utf-8") as fh:
    data = json.load(fh)

value = data
for part in field_path.split("."):
    if not part:
        continue
    value = value[part]

if isinstance(value, str):
    print(value)
else:
    print(json.dumps(value, ensure_ascii=False))
PY
}

bootstrap_local_hub() {
  run_setup "bootstrap local daemon and owner token" "$AGENTHUB_BIN" ls
  OWNER_TOKEN="$(read_json_field "$CONFIG_PATH" "local.owner_token")"
  API_BASE="$(read_json_field "$RUNTIME_PATH" "api_base")"
  if [[ -z "$OWNER_TOKEN" || -z "$API_BASE" ]]; then
    echo "failed to bootstrap local owner token or api base" >&2
    exit 1
  fi
}

seed_secret() {
  local payload
  payload='{"data":"ghp_agenthub_secret_fixture_123","description":"Fixture secret for safe CLI smoke verification"}'
  echo
  echo "-- setup: seed secret via local Agent API --"
  LAST_STDOUT="$(curl -fsS \
    -X PUT \
    -H "Authorization: Bearer $OWNER_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$payload" \
    "$API_BASE/agent/vault/auth.github.test")"
  LAST_STATUS=$?
  LAST_STDERR=""
  if [[ "$LAST_STATUS" -ne 0 || -z "$LAST_STDOUT" ]]; then
    echo "failed to seed secret via local Agent API" >&2
    exit 1
  fi
  printf '%s\n' "$LAST_STDOUT"
}

test_help_surface() {
  start_test "help surface is clear"
  run_cmd "$AGENTHUB_BIN" help
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "Root-directory command surface for local and remote Agent Hub data." "help root output"
  assert_contains "$LAST_STDOUT" "Public roots: profile, memory, project, skill, secret, platform" "help root output"

  run_cmd "$AGENTHUB_BIN" help write
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "Create or update Hub content from literal text, stdin, or a local file path." "help write output"

  run_cmd "$AGENTHUB_BIN" help project
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "Agent Hub Path Model" "help project output"
  assert_contains "$LAST_STDOUT" '`project/<name>` is a summary view.' "help project output"
  finish_test
}

test_ls_root() {
  start_test "ls root and public categories"
  run_cmd "$AGENTHUB_BIN" ls
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  for expected in $'dir\tprofile/' $'dir\tmemory/' $'dir\tproject/' $'dir\tskill/' $'dir\tsecret/' $'dir\tplatform/'; do
    assert_contains "$LAST_STDOUT" "$expected" "ls root output"
  done
  finish_test
}

test_profile_read_write() {
  start_test "write and read profile"
  run_cmd "$AGENTHUB_BIN" write profile/preferences "Keep responses concise and explicit."
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "Updated profile/preferences." "profile write output"

  run_cmd "$AGENTHUB_BIN" read profile/preferences
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "Keep responses concise and explicit." "profile read output"
  finish_test
}

test_memory_write_search() {
  start_test "write memory and search memory"
  run_cmd "$AGENTHUB_BIN" write memory "Remember the Alpha milestone for launch review."
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "Saved memory note." "memory write output"

  run_cmd "$AGENTHUB_BIN" search "Alpha milestone" memory
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "Remember the Alpha milestone for launch review." "memory search output"
  finish_test
}

test_project_create_write_log_read_search() {
  start_test "create project and verify nested project content"
  run_cmd "$AGENTHUB_BIN" create project demo
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "Created project/demo." "create project output"

  local project_file="$TMP_ROOT/project-alpha.txt"
  cat >"$project_file" <<'EOF'
Alpha launch checklist from file input.
Line two keeps the file non-empty.
EOF

  run_cmd "$AGENTHUB_BIN" write project/demo/docs/notes/alpha.md "$project_file"
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "Wrote project/demo/docs/notes/alpha.md." "project file write output"

  run_cmd "$AGENTHUB_BIN" read project/demo/docs/notes/alpha.md
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "Alpha launch checklist from file input." "project file read output"

  run_cmd "$AGENTHUB_BIN" log project/demo --action note --summary "Project demo log summary from smoke test."
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "Logged note on project/demo." "project log output"

  run_cmd "$AGENTHUB_BIN" read project/demo
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "name: demo" "project summary output"
  assert_contains "$LAST_STDOUT" "Project demo log summary from smoke test." "project summary output"

  run_cmd "$AGENTHUB_BIN" search "Alpha launch checklist" project/demo
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "Alpha launch checklist from file input." "project search output"
  finish_test
}

test_secret_ls_read() {
  start_test "list and read secret"
  run_cmd "$AGENTHUB_BIN" ls secret
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "secret/auth.github.test" "secret ls output"

  run_cmd "$AGENTHUB_BIN" read secret/auth.github.test
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  assert_contains "$LAST_STDOUT" "ghp_agenthub_secret_fixture_123" "secret read output"
  finish_test
}

test_stats() {
  start_test "stats summary is non-empty"
  run_cmd "$AGENTHUB_BIN" stats
  assert_command_ok
  assert_nonempty "$LAST_STDOUT" "stdout"
  for key in "files:" "memory:" "profile:" "projects:"; do
    assert_contains "$LAST_STDOUT" "$key" "stats output"
  done
  finish_test
}

print_summary() {
  echo
  echo "Summary"
  echo "  mode:    safe-smoke"
  echo "  total:   $TOTAL_COUNT"
  echo "  passed:  $PASS_COUNT"
  echo "  failed:  $FAIL_COUNT"
  if [[ "$KEEP_TMP" -eq 1 || "$FAIL_COUNT" -gt 0 ]]; then
    echo "  tmpdir:  $TMP_ROOT"
  else
    echo "  tmpdir:  $TMP_ROOT (will be cleaned)"
  fi
}

main() {
  need_cmd bash
  need_cmd go
  need_cmd curl
  need_cmd python3

  TMP_ROOT="$(mktemp -d)"
  export HOME="$TMP_ROOT/home"
  export XDG_CONFIG_HOME="$TMP_ROOT/config"
  export XDG_STATE_HOME="$TMP_ROOT/state"
  export XDG_DATA_HOME="$TMP_ROOT/data"
  export XDG_CACHE_HOME="$TMP_ROOT/cache"
  export AGENTHUB_CONFIG="$XDG_CONFIG_HOME/agenthub/config.json"
  export AGENTHUB_TOKEN=""
  export AGENTHUB_SYNC_TOKEN=""
  export AGENTHUB_API_BASE=""
  export AGENTHUB_SYNC_API_BASE=""
  export AGENTHUB_SYNC_PROFILE=""
  export GOMAXPROCS="${AGENTHUB_TEST_CLI_GOMAXPROCS:-2}"
  if [[ -n "${GOFLAGS:-}" ]]; then
    export GOFLAGS="$GOFLAGS -p=1"
  else
    export GOFLAGS="-p=1"
  fi

  mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$XDG_STATE_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME"
  CONFIG_PATH="$AGENTHUB_CONFIG"
  RUNTIME_PATH="$XDG_CONFIG_HOME/agenthub/runtime.json"

  AGENTHUB_BIN="$TMP_ROOT/agenthub"
  run_setup "build agenthub binary" go build -o "$AGENTHUB_BIN" ./cmd/agenthub
  bootstrap_local_hub
  seed_secret

  test_help_surface
  test_ls_root
  test_profile_read_write
  test_memory_write_search
  test_project_create_write_log_read_search
  test_secret_ls_read
  test_stats

  print_summary

  if [[ "$FAIL_COUNT" -gt 0 ]]; then
    exit 1
  fi
}

cd "$ROOT_DIR"
main "$@"
