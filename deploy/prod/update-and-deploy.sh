#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REMOTE="${REMOTE:-origin}"
BRANCH="${BRANCH:-main}"

log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

if [[ -n "$(git -C "$REPO_ROOT" status --porcelain)" ]]; then
  echo "refusing to update dirty working tree: $REPO_ROOT" >&2
  exit 1
fi

log "Fetching $REMOTE/$BRANCH"
git -C "$REPO_ROOT" fetch --prune "$REMOTE" "$BRANCH"
git -C "$REPO_ROOT" checkout "$BRANCH"
git -C "$REPO_ROOT" pull --ff-only "$REMOTE" "$BRANCH"

exec "$REPO_ROOT/deploy/prod/deploy.sh"
