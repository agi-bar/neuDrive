#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ "${1:-}" == "--unsafe-full" ]]; then
  cat >&2 <<'EOF'
tools/verify-neudrive-cli.sh no longer supports the old heavy verification mode.

Reason:
  The previous full-suite flow could overload local machines by spawning many
  temporary neuDrive and Go build processes.

Use instead:
  tools/test-neudrive-cli.sh

This runs the safe smoke suite for the neuDrive CLI without isolated Go caches
or the older heavy platform/import/git stress flow.
EOF
  exit 2
fi

cat >&2 <<'EOF'
tools/verify-neudrive-cli.sh is now a compatibility wrapper.
Forwarding to tools/test-neudrive-cli.sh (safe smoke suite only).
EOF

exec "${SCRIPT_DIR}/test-neudrive-cli.sh" "$@"
