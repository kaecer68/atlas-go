#!/usr/bin/env bash
# =============================================================================
# sync-version.sh — Sync hardcoded version strings in docs from VERSION file
#
# Replaces patterns like "0.0.0.X" or "v0.0.0.X" with the current VERSION
# in a curated allowlist of doc files. VERSION file is the single source of
# truth; this script keeps docs from drifting.
#
# Usage: bash scripts/sync-version.sh
# Exits 0 on success, non-zero on any update failure.
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

if [[ ! -f VERSION ]]; then
    echo "❌ VERSION file not found in $REPO_ROOT"
    exit 1
fi

VERSION=$(tr -d '[:space:]' < VERSION)

# Files where version is hardcoded and should be synced from VERSION
TARGETS=(
    "AGENTS.md"
    "internal/AGENTS_INDEX.md"
    "internal/MATURITY.md"
    "internal/fubonproxy/AGENTS.md"
    "cmd/atlas-mcp/server/AGENTS.md"
)

# Match version-shaped strings only — require either `v` prefix OR non-zero
# 4th segment (so `0.0.0.0` IP wildcard is NOT matched). Preserve optional "+" suffix.
PATTERN='v0\.[0-9]+\.[0-9]+\.[0-9]+(\+?)'

echo "Syncing version to v${VERSION}"
UPDATED=0
for f in "${TARGETS[@]}"; do
    [[ -f "$f" ]] || continue
    if grep -qE "${PATTERN}" "$f"; then
        perl -i -pe "next if / 新|完整 release notes/; s/${PATTERN}/v${VERSION}\$1/g" "$f"
        echo "  ✓ $f"
        UPDATED=$((UPDATED + 1))
    fi
done

echo ""
echo "Synced $UPDATED file(s)"
