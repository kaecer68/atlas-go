#!/usr/bin/env bash
# check-binary-freshness.sh
#
# Verifies that every deployed binary's buildinfo.Commit matches HEAD.
# Run BEFORE marking work complete and BEFORE cleanup — see AGENTS.md
# workspace-close SOP.
#
# Exit codes:
#   0 — all binaries fresh (match HEAD)
#   1 — at least one stale binary detected (with per-binary report)
#   2 — environment error (git/docker not available, buildinfo missing, etc.)

set -euo pipefail

HEAD=$(git rev-parse HEAD 2>/dev/null) || { echo "ERROR: not in a git repo"; exit 2; }
echo "checking binaries against HEAD=$HEAD"
echo ""

declare -a STALE=()
declare -a MISSING_BUILDINFO=()

# Helper: extract buildinfo.Commit from a binary inside a docker container.
# BusyBox grep lacks -P/-o, so we use strings + grep + sed for portability.
extract_commit_container() {
    local container=$1
    local bin_path=$2
    docker exec "$container" sh -c "strings '$bin_path' 2>/dev/null | grep 'Commit='" 2>/dev/null \
        | head -1 | sed 's/.*Commit=\([a-f0-9]*\).*/\1/' | grep -E '^[a-f0-9]{7,}$' || echo ""
}

# Helper: extract buildinfo.Commit from a host binary file.
extract_commit_host() {
    local bin_path=$1
    strings "$bin_path" 2>/dev/null | grep 'Commit=' | head -1 \
        | sed 's/.*Commit=\([a-f0-9]*\).*/\1/' | grep -E '^[a-f0-9]{7,}$' || echo ""
}

check_one() {
    local label=$1
    local commit=$2
    local ref_image=${3:-}  # optional, for reporting only
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

echo "=== Docker images ==="

# atlas-atlas image (contains atlas-go, atlas-mcp, daily-replay-sync, backfill-replay, calibrate-seasonal)
# We extract atlas-go without running it: create a tmp container, copy, remove.
TMP_CID=$(docker create atlas-atlas:latest 2>/dev/null || echo "")
if [ -n "$TMP_CID" ]; then
    TMP_BIN=/tmp/.atlas-go-freshness-check
    docker cp "$TMP_CID:/app/atlas-go" "$TMP_BIN" 2>/dev/null
    docker rm "$TMP_CID" 2>/dev/null >/dev/null
    check_one "atlas-atlas image → /app/atlas-go" "$(extract_commit_host "$TMP_BIN")"
    rm -f "$TMP_BIN"

    # Also check the atlas-mcp inside atlas-atlas image (same ldflags as atlas-go)
    TMP_CID=$(docker create atlas-atlas:latest 2>/dev/null || echo "")
    if [ -n "$TMP_CID" ]; then
        TMP_BIN=/tmp/.atlas-mcp-freshness-check
        docker cp "$TMP_CID:/app/atlas-mcp" "$TMP_BIN" 2>/dev/null
        docker rm "$TMP_CID" 2>/dev/null >/dev/null
        check_one "atlas-atlas image → /app/atlas-mcp" "$(extract_commit_host "$TMP_BIN")"
        rm -f "$TMP_BIN"
    fi
fi

# Cron image (single image, used by all 10 cron containers)
TMP_CID=$(docker create atlas-cron-rebuilt:local 2>/dev/null || echo "")
if [ -n "$TMP_CID" ]; then
    TMP_BIN=/tmp/.macro-ingest-freshness-check
    docker cp "$TMP_CID:/app/macro-ingest" "$TMP_BIN" 2>/dev/null
    docker rm "$TMP_CID" 2>/dev/null >/dev/null
    check_one "atlas-cron-rebuilt:local → /app/macro-ingest" "$(extract_commit_host "$TMP_BIN")"
    rm -f "$TMP_BIN"
fi

echo ""
echo "=== Host binaries ==="

# Detect repo root (parent of scripts/ directory). Avoids cwd-dependent failures.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# bin/atlas-mcp (host MCP wrapper)
HOST_ATLAS_MCP="$REPO_ROOT/bin/atlas-mcp"
if [ -f "$HOST_ATLAS_MCP" ]; then
    check_one "bin/atlas-mcp" "$(extract_commit_host "$HOST_ATLAS_MCP")"
else
    echo "  ⚠ bin/atlas-mcp not found at $HOST_ATLAS_MCP (skipping)"
fi

# bin/execute-experiment — skip (removed as x86_64 legacy orphan)

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
