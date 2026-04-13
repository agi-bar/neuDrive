#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ "${1:-}" == "--unsafe-full" ]]; then
  cat >&2 <<'EOF'
tools/verify-agenthub-cli.sh no longer supports the old heavy verification mode.

Reason:
  The previous full-suite flow could overload local machines by spawning many
  temporary Agent Hub and Go build processes.

Use instead:
  tools/test-agenthub-cli.sh

This runs the safe smoke suite for the Agent Hub CLI without isolated Go caches
or the older heavy platform/import/git stress flow.
EOF
  exit 2
fi

cat >&2 <<'EOF'
tools/verify-agenthub-cli.sh is now a compatibility wrapper.
Forwarding to tools/test-agenthub-cli.sh (safe smoke suite only).
EOF

exec "${SCRIPT_DIR}/test-agenthub-cli.sh" "$@"
