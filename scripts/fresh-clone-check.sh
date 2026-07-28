#!/usr/bin/env bash
# fresh-clone-check.sh — verify repo is self-contained by cloning to a temp
# directory and building from scratch (frontend → go build → go test).
#
# Purpose:
#   Catches hidden local dependencies (prebuilt dist/, stale .build-atlas/,
#   uncommitted generated files) that pass in a tainted worktree but break
#   for a fresh clone.
#
# Usage:
#   make fresh-clone-check
#   CLONE_TIMEOUT=360 ./scripts/fresh-clone-check.sh
#
# Exit: 0 = clean, 1 = build/test failure, 2 = prerequisites missing.

set -euo pipefail

CLONE_TIMEOUT="${CLONE_TIMEOUT:-300}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== fresh-clone-check (started $(date -u +%Y-%m-%dT%H:%M:%SZ)) ==="

# Prerequisites
for tool in git go npm; do
  if ! command -v $tool &>/dev/null; then
    echo "ERROR: $tool not found in PATH"
    exit 2
  fi
done

# Create temp directory
TMPDIR="${TMPDIR:-/tmp}"
CLONE_DIR="$(mktemp -d "$TMPDIR/fresh-clone-check.XXXXXX")"
trap 'rm -rf "$CLONE_DIR"' EXIT

REMOTE_URL="$(git -C "$REPO_ROOT" remote get-url origin 2>/dev/null || echo "")"
BRANCH="$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "HEAD")"

echo "[1/4] cloning into $CLONE_DIR"
if [ -n "$REMOTE_URL" ]; then
  git clone --depth 1 --branch "$BRANCH" "$REMOTE_URL" "$CLONE_DIR" 2>&1 || {
    # Fallback: clone from local
    echo "  remote clone failed, copying from local repo..."
    rm -rf "$CLONE_DIR"
    mkdir -p "$CLONE_DIR"
    (cd "$REPO_ROOT" && git archive --format=tar HEAD) | (cd "$CLONE_DIR" && tar xf -)
  }
else
  mkdir -p "$CLONE_DIR"
  (cd "$REPO_ROOT" && git archive --format=tar HEAD) | (cd "$CLONE_DIR" && tar xf -)
fi

echo "[2/4] building frontend"
cd "$CLONE_DIR"
for dir in admin_web client_web; do
  if [ -d "$dir" ]; then
    echo "  → $dir"
    (cd "$dir" && npm ci && npm run build) || {
      echo "ERROR: frontend build failed ($dir)"
      exit 1
    }
  fi
done

echo "[3/4] building Go backend"
go build -ldflags="-w -s" ./cmd/atlas ./cmd/atlas-mcp || {
  echo "ERROR: go build failed"
  exit 1
}

echo "[4/4] running tests (excluding cmd/atlas integration)"
go test -count=1 -timeout="${CLONE_TIMEOUT}s" $(go list ./... | grep -v '/cmd/atlas$') || {
  echo "WARNING: some tests failed — check output above"
  exit 1
}

echo ""
echo "=== fresh-clone-check PASSED ==="
