#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
SOURCE_K8S_DIR="${SOURCE_K8S_DIR:-$REPO_ROOT/deploy/k8s}"
K8S_DIR="${K8S_DIR:-$SOURCE_K8S_DIR}"
NAMESPACE="${NAMESPACE:-agenthub}"
MINIKUBE_PROFILE="${MINIKUBE_PROFILE:-minikube}"
IMAGE_REPO="${IMAGE_REPO:-agenthub}"
FULL_SHA="$(git -C "$REPO_ROOT" rev-parse HEAD)"
SHORT_SHA="${FULL_SHA:0:12}"
IMAGE_TAG="${IMAGE_TAG:-$SHORT_SHA}"
APP_HOME="${APP_HOME:-$(cd "$REPO_ROOT/.." && pwd)}"
AGENTHUB_ENV_FILE="${AGENTHUB_ENV_FILE:-}"
APP_HOST="${APP_HOST:-}"
HEALTHCHECK_URL="${HEALTHCHECK_URL:-}"

log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

die() {
  echo "$*" >&2
  exit 1
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    die "missing required command: $1"
  fi
}

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

require_env() {
  local key="$1"
  if [[ -z "${!key:-}" ]]; then
    die "required setting is missing: $key"
  fi
}

load_config() {
  local env_file

  env_file="$(detect_env_file)" || die "missing config file; expected one of: $APP_HOME/config/agenthub.env, $REPO_ROOT/agenthub.env, $REPO_ROOT/.env"
  log "Loading config from $env_file"

  set -a
  # shellcheck disable=SC1090
  source "$env_file"
  set +a

  POSTGRES_DB="${POSTGRES_DB:-agenthub}"
  POSTGRES_USER="${POSTGRES_USER:-agenthub}"
  POSTGRES_PORT="${POSTGRES_PORT:-5432}"
  PORT="${PORT:-8080}"

  require_env POSTGRES_PASSWORD
  require_env JWT_SECRET
  require_env VAULT_MASTER_KEY

  if [[ -z "${PUBLIC_BASE_URL:-}" ]]; then
    PUBLIC_BASE_URL="https://agenthub.agi.bar"
  fi
  if [[ "$PUBLIC_BASE_URL" != http://* && "$PUBLIC_BASE_URL" != https://* ]]; then
    die "PUBLIC_BASE_URL must start with http:// or https://"
  fi

  CORS_ORIGINS="${CORS_ORIGINS:-$PUBLIC_BASE_URL,http://localhost:3000,http://localhost:5173}"
  DATABASE_URL="${DATABASE_URL:-postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@agenthub-postgres.${NAMESPACE}.svc.cluster.local:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable}"

  if [[ -z "$APP_HOST" ]]; then
    APP_HOST="$(printf '%s' "$PUBLIC_BASE_URL" | sed -E 's#^https?://([^/]+)/?.*$#\1#')"
  fi
  if [[ -z "$HEALTHCHECK_URL" ]]; then
    HEALTHCHECK_URL="${PUBLIC_BASE_URL%/}/api/health"
  fi
}

sync_manifest_files() {
  local name

  mkdir -p "$K8S_DIR"
  for name in namespace.yaml postgres.yaml app.yaml ingress.yaml cloudflared.yaml; do
    if [[ "$K8S_DIR/$name" == "$SOURCE_K8S_DIR/$name" ]]; then
      continue
    fi
    cp "$SOURCE_K8S_DIR/$name" "$K8S_DIR/$name"
  done
}

sync_config_map() {
  kubectl -n "$NAMESPACE" create configmap agenthub-config \
    --from-literal=PORT="$PORT" \
    --from-literal=CORS_ORIGINS="$CORS_ORIGINS" \
    --from-literal=PUBLIC_BASE_URL="$PUBLIC_BASE_URL" \
    --dry-run=client -o yaml | kubectl apply -f -
}

sync_secret() {
  local name="$1"
  shift
  kubectl -n "$NAMESPACE" create secret generic "$name" \
    "$@" \
    --dry-run=client -o yaml | kubectl apply -f -
}

sync_runtime_secrets() {
  sync_secret agenthub-postgres \
    --from-literal=POSTGRES_DB="$POSTGRES_DB" \
    --from-literal=POSTGRES_USER="$POSTGRES_USER" \
    --from-literal=POSTGRES_PASSWORD="$POSTGRES_PASSWORD"

  sync_secret agenthub-app \
    --from-literal=DATABASE_URL="$DATABASE_URL" \
    --from-literal=JWT_SECRET="$JWT_SECRET" \
    --from-literal=VAULT_MASTER_KEY="$VAULT_MASTER_KEY" \
    --from-literal=GITHUB_CLIENT_ID="${GITHUB_CLIENT_ID:-}" \
    --from-literal=GITHUB_CLIENT_SECRET="${GITHUB_CLIENT_SECRET:-}"

  if [[ -n "${CLOUDFLARED_TUNNEL_TOKEN:-}" ]]; then
    sync_secret agenthub-cloudflared-token \
      --from-literal=TUNNEL_TOKEN="$CLOUDFLARED_TUNNEL_TOKEN"
  fi
}

wait_for_http() {
  local url="$1"
  local attempts="${2:-20}"
  local sleep_seconds="${3:-3}"
  local i

  for ((i = 1; i <= attempts; i += 1)); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$sleep_seconds"
  done

  return 1
}

require_cmd git
require_cmd kubectl
require_cmd minikube
require_cmd docker
require_cmd curl

load_config
sync_manifest_files

log "Deploying commit $FULL_SHA"
log "Building $IMAGE_REPO:$IMAGE_TAG inside minikube docker daemon"
eval "$(minikube -p "$MINIKUBE_PROFILE" docker-env --shell bash)"
docker build -t "$IMAGE_REPO:$IMAGE_TAG" -t "$IMAGE_REPO:latest" "$REPO_ROOT"

log "Applying Kubernetes manifests"
kubectl apply -f "$K8S_DIR/namespace.yaml"
sync_runtime_secrets
sync_config_map
kubectl apply -f "$K8S_DIR/postgres.yaml"
kubectl apply -f "$K8S_DIR/app.yaml"
kubectl apply -f "$K8S_DIR/ingress.yaml"

log "Updating deployment image to $IMAGE_REPO:$IMAGE_TAG"
kubectl -n "$NAMESPACE" set image deployment/agenthub-server \
  agenthub-server="$IMAGE_REPO:$IMAGE_TAG"
kubectl -n "$NAMESPACE" annotate deployment/agenthub-server \
  agenthub.agi.bar/deployed-git-sha="$FULL_SHA" \
  agenthub.agi.bar/deployed-at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')" \
  --overwrite

log "Waiting for rollout"
kubectl -n "$NAMESPACE" rollout status deployment/agenthub-server --timeout=10m

log "Waiting for public healthcheck: $HEALTHCHECK_URL"
if ! wait_for_http "$HEALTHCHECK_URL"; then
  echo "public healthcheck failed: $HEALTHCHECK_URL" >&2
  exit 1
fi

log "Deployment complete"
kubectl -n "$NAMESPACE" get pods -o wide
kubectl -n "$NAMESPACE" get ingress
curl -fsS "$HEALTHCHECK_URL"
echo
