#!/usr/bin/env bash
# deploy-staging.sh — Pull the latest main, restart the staging
# container, and run the first daily soak check to confirm
# everything came up cleanly.
#
# Prerequisites:
#   1. staging docker compose running locally or on a staging host
#   2. /var/lib/atlas/data/state/ mounted (or DATA_DB override)
#   3. scripts/staging-soak-check.sh installed at
#      /usr/local/bin/staging-soak-check.sh (or override path)
#
# Usage:
#   ./scripts/deploy-staging.sh
#   DATA_DB=/custom/path/atlas.db STAGING_URL=http://staging:18080 ./scripts/deploy-staging.sh

set -euo pipefail

STAGING_URL="${STAGING_URL:-http://localhost:18080}"
DATA_DB="${DATA_DB:-/var/lib/atlas/data/state/atlas.db}"
SOAK_SCRIPT="${SOAK_SCRIPT:-/usr/local/bin/staging-soak-check.sh}"
COMPOSE_DIR="${COMPOSE_DIR:-$(pwd)}"

echo "=== Atlas staging deploy (started $(date -u +%Y-%m-%dT%H:%M:%SZ)) ==="

# Step 1: pull latest code in the repo directory
echo "[1/4] git pull --ff-only"
git pull --ff-only

echo "[2/4] docker compose build && up -d"

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
echo "[3/4] wait for staging health"
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

# Step 4: run the daily soak check once to verify
echo "[4/4] run staging-soak-check"
if [[ -x "$SOAK_SCRIPT" ]]; then
    STAGING_URL="$STAGING_URL" DATA_DB="$DATA_DB" "$SOAK_SCRIPT"
else
    echo "  WARN: soak script not found at $SOAK_SCRIPT — skipping"
    echo "  install: sudo cp scripts/staging-soak-check.sh /usr/local/bin/"
fi

echo "=== deploy complete ==="
