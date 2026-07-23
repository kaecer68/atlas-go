#!/usr/bin/env bash
# check-binary-freshness.sh
#
# Verifies that every deployed binary's buildinfo.Commit matches HEAD.
# Temporary Docker containers and extracted files are always cleaned up, including
# when Docker copy fails or the shell exits early.

set -euo pipefail

DOCKER_BIN="${DOCKER_BIN:-docker}"
FRESHNESS_TMPDIR="${FRESHNESS_TMPDIR:-${TMPDIR:-/tmp}}"
declare -a TEMP_CONTAINERS=()
declare -a TEMP_FILES=()

declare -a STALE=()
declare -a MISSING_BUILDINFO=()

cleanup() {
    local status=$?
    set +e
    for file in "${TEMP_FILES[@]}"; do
        rm -f "$file"
    done
    for container in "${TEMP_CONTAINERS[@]}"; do
        "$DOCKER_BIN" rm -f "$container" >/dev/null 2>&1
    done
    exit "$status"
}
trap cleanup EXIT

HEAD=$(git rev-parse HEAD 2>/dev/null) || { echo "ERROR: not in a git repo"; exit 2; }
echo "checking binaries against HEAD=$HEAD"
echo ""

# Helper: extract buildinfo.Commit from a host binary file.
extract_commit_host() {
    local bin_path=$1
    strings "$bin_path" 2>/dev/null | grep 'Commit=' | head -1 \
        | sed 's/.*Commit=\([a-f0-9]*\).*/\1/' | grep -E '^[a-f0-9]{7,}$' || echo ""
}

check_one() {
    local label=$1
    local commit=$2
    if [ -z "$commit" ]; then
        MISSING_BUILDINFO+=("$label (no buildinfo.Commit found)")
        echo "  ⚠ $label: buildinfo.Commit NOT FOUND"
    elif [ "$commit" = "$HEAD" ]; then
        echo "  ✓ $label: $commit"
    else
        STALE+=("$label: $commit (HEAD=$HEAD)")
        echo "  ✗ STALE  $label: $commit"
    fi
}

# Copy one binary from a temporary container. Cleanup is deferred to EXIT so a
# failed docker cp cannot leave a random Created container behind.
check_image_binary() {
    local image=$1
    local binary=$2
    local label=$3
    local tmp_bin=$4
    local container

    container=$("$DOCKER_BIN" create --label atlas.binary-freshness=true "$image" 2>/dev/null || true)
    if [ -z "$container" ]; then
        MISSING_BUILDINFO+=("$label (image unavailable: $image)")
        echo "  ⚠ $label: image unavailable ($image)" >&2
        return 0
    fi
    TEMP_CONTAINERS+=("$container")
    TEMP_FILES+=("$tmp_bin")
    "$DOCKER_BIN" cp "$container:$binary" "$tmp_bin"
    check_one "$label" "$(extract_commit_host "$tmp_bin")"
}

echo "=== Docker images ==="
check_image_binary "atlas-atlas:latest" /app/atlas-go \
    "atlas-atlas image → /app/atlas-go" "$FRESHNESS_TMPDIR/.atlas-go-freshness-check-$$"
check_image_binary "atlas-atlas:latest" /app/atlas-mcp \
    "atlas-atlas image → /app/atlas-mcp" "$FRESHNESS_TMPDIR/.atlas-mcp-freshness-check-$$"
check_image_binary "atlas-atlas:latest" /app/daily-replay-sync \
    "atlas-atlas image → /app/daily-replay-sync" "$FRESHNESS_TMPDIR/.daily-replay-sync-freshness-check-$$"
check_image_binary "atlas-atlas:latest" /app/backfill-replay \
    "atlas-atlas image → /app/backfill-replay" "$FRESHNESS_TMPDIR/.backfill-replay-freshness-check-$$"
check_image_binary "atlas-atlas:latest" /app/calibrate-seasonal \
    "atlas-atlas image → /app/calibrate-seasonal" "$FRESHNESS_TMPDIR/.calibrate-seasonal-freshness-check-$$"
check_image_binary "atlas-cron-rebuilt:local" /app/macro-ingest \
    "atlas-cron-rebuilt:local → /app/macro-ingest" "$FRESHNESS_TMPDIR/.macro-ingest-freshness-check-$$"

echo ""
echo "=== Host binaries ==="
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HOST_ATLAS_MCP="$REPO_ROOT/bin/atlas-mcp"
if [ -f "$HOST_ATLAS_MCP" ]; then
    check_one "bin/atlas-mcp" "$(extract_commit_host "$HOST_ATLAS_MCP")"
else
    echo "  ⚠ bin/atlas-mcp not found at $HOST_ATLAS_MCP (skipping)"
fi

echo ""
echo "=== Summary ==="
echo "  HEAD: $HEAD"
if [ ${#STALE[@]} -eq 0 ] && [ ${#MISSING_BUILDINFO[@]} -eq 0 ]; then
    echo "  ✓ ALL BINARIES FRESH"
    exit 0
fi
if [ ${#STALE[@]} -gt 0 ]; then
    echo "  ✗ STALE (${#STALE[@]}):"
    printf '    %s\n' "${STALE[@]}"
fi
if [ ${#MISSING_BUILDINFO[@]} -gt 0 ]; then
    echo "  ⚠ MISSING_BUILDINFO (${#MISSING_BUILDINFO[@]}):"
    printf '    %s\n' "${MISSING_BUILDINFO[@]}"
fi
echo ""
echo "Fix: run 'make rebuild-all' to align binaries with HEAD."
exit 1
