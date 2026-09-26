#!/usr/bin/env bash
# deploy-staging.sh — Pull the latest main, restart the staging
# container, and wait for /health to confirm it came up cleanly.
#
# NOTE (2026-09-27): the post-deploy soak step was removed together with
# scripts/staging-soak-check.sh, scripts/install-soak-automation.sh and
# scripts/com.atlas.soak-check.plist. The 7-day post-merge soak window
# (PR #1179) ended 2026-07-21 and the project declared "no staging" on
# 2026-08-15, so the step could never run again.
#
# Prerequisites:
#   1. staging docker compose running locally or on a staging host
#   2. /var/lib/atlas/data/state/ mounted
#
# Usage:
#   ./scripts/deploy-staging.sh
#   STAGING_URL=http://staging:18080 ./scripts/deploy-staging.sh

set -euo pipefail

STAGING_URL="${STAGING_URL:-http://localhost:18080}"
COMPOSE_DIR="${COMPOSE_DIR:-$(pwd)}"

echo "=== Atlas staging deploy (started $(date -u +%Y-%m-%dT%H:%M:%SZ)) ==="

# Step 1: pull latest code in the repo directory
echo "[1/3] git pull --ff-only"
git pull --ff-only

echo "[2/3] docker compose build && up -d"

# Binary freshness gate: refuse to deploy if binaries are stale vs HEAD.
echo "[Gate] checking binary freshness..."
if ! make -s check-binaries; then
	echo ""
	echo "ERROR: at least one deployed binary is stale (binary Commit != git HEAD)."
	echo "  Fix: make rebuild-all"
	echo ""
	exit 1
fi


( cd "$COMPOSE_DIR" && ATLAS_GIT_COMMIT="$(git rev-parse HEAD)" docker compose build atlas && ATLAS_GIT_COMMIT="$(git rev-parse HEAD)" docker compose up -d )

# Step 3: wait for /health endpoint (max 60s)
echo "[3/3] wait for staging health"
for i in $(seq 1 30); do
    if curl -sf --max-time 2 "$STAGING_URL/health" > /dev/null 2>&1; then
        echo "  staging up after ${i}*2s"
        break
    fi
    sleep 2
done

if ! curl -sf --max-time 2 "$STAGING_URL/health" > /dev/null 2>&1; then
    echo "ERROR: staging did not become healthy in 60s"
    exit 1
fi

echo "=== deploy complete ==="
