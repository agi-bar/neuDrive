#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
K8S_DIR="${K8S_DIR:-$REPO_ROOT/deploy/k8s}"
NAMESPACE="${NAMESPACE:-agenthub}"
MINIKUBE_PROFILE="${MINIKUBE_PROFILE:-minikube}"
IMAGE_REPO="${IMAGE_REPO:-agenthub}"
FULL_SHA="$(git -C "$REPO_ROOT" rev-parse HEAD)"
SHORT_SHA="${FULL_SHA:0:12}"
IMAGE_TAG="${IMAGE_TAG:-$SHORT_SHA}"
APP_HOST="${APP_HOST:-agenthub.agi.bar}"
HEALTHCHECK_URL="${HEALTHCHECK_URL:-https://$APP_HOST/api/health}"

log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

apply_if_missing() {
  local kind="$1"
  local name="$2"
  local file="$3"

  if kubectl -n "$NAMESPACE" get "$kind" "$name" >/dev/null 2>&1; then
    log "Keeping existing $kind/$name in namespace $NAMESPACE"
    return
  fi

  log "Creating missing $kind/$name from $(basename "$file")"
  kubectl apply -f "$file"
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

log "Deploying commit $FULL_SHA"
log "Building $IMAGE_REPO:$IMAGE_TAG inside minikube docker daemon"
eval "$(minikube -p "$MINIKUBE_PROFILE" docker-env --shell bash)"
docker build -t "$IMAGE_REPO:$IMAGE_TAG" -t "$IMAGE_REPO:latest" "$REPO_ROOT"

log "Applying Kubernetes manifests"
kubectl apply -f "$K8S_DIR/namespace.yaml"
apply_if_missing secret agenthub-postgres "$K8S_DIR/postgres-secret.yaml"
apply_if_missing secret agenthub-app "$K8S_DIR/app-secret.yaml"
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
