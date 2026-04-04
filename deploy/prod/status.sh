#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
NAMESPACE="${NAMESPACE:-agenthub}"
APP_HOST="${APP_HOST:-agenthub.agi.bar}"

echo "repo_root=$REPO_ROOT"
echo "git_head=$(git -C "$REPO_ROOT" rev-parse HEAD)"
echo "origin_main=$(git -C "$REPO_ROOT" rev-parse origin/main 2>/dev/null || echo unknown)"
echo
kubectl -n "$NAMESPACE" get deploy,po,svc,ingress -o wide
echo
kubectl -n "$NAMESPACE" describe deployment agenthub-server | sed -n '1,140p'
echo
curl -fsS "https://$APP_HOST/api/health"
echo
