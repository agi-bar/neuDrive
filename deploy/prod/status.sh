#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
APP_HOME="${APP_HOME:-$(cd "$REPO_ROOT/.." && pwd)}"
AGENTHUB_ENV_FILE="${AGENTHUB_ENV_FILE:-}"
NAMESPACE="${NAMESPACE:-agenthub}"
APP_HOST="${APP_HOST:-agenthub.agi.bar}"
HEALTHCHECK_URL="${HEALTHCHECK_URL:-}"

detect_env_file() {
  local candidate
  local candidates=()

  if [[ -n "$AGENTHUB_ENV_FILE" ]]; then
    candidates+=("$AGENTHUB_ENV_FILE")
  fi

  candidates+=(
    "$APP_HOME/config/agenthub.env"
    "$REPO_ROOT/agenthub.env"
    "$REPO_ROOT/.env"
  )

  for candidate in "${candidates[@]}"; do
    if [[ -f "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done

  return 1
}

if env_file="$(detect_env_file)"; then
  set -a
  # shellcheck disable=SC1090
  source "$env_file"
  set +a
  if [[ -n "${PUBLIC_BASE_URL:-}" && -z "$HEALTHCHECK_URL" ]]; then
    HEALTHCHECK_URL="${PUBLIC_BASE_URL%/}/api/health"
  fi
fi

if [[ -z "$HEALTHCHECK_URL" ]]; then
  HEALTHCHECK_URL="https://$APP_HOST/api/health"
fi

echo "repo_root=$REPO_ROOT"
echo "git_head=$(git -C "$REPO_ROOT" rev-parse HEAD)"
echo "origin_main=$(git -C "$REPO_ROOT" rev-parse origin/main 2>/dev/null || echo unknown)"
if [[ -n "${env_file:-}" ]]; then
  echo "config_env=$env_file"
fi
echo
kubectl -n "$NAMESPACE" get deploy,po,svc,ingress -o wide
echo
kubectl -n "$NAMESPACE" describe deployment agenthub-server | sed -n '1,140p'
echo
curl -fsS "$HEALTHCHECK_URL"
echo
