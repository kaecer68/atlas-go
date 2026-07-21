#!/usr/bin/env bash
# =============================================================================
# check_versions.sh — Verify auto-synced docs match VERSION
#
# Only enforces consistency for the curated TARGETS list that
# sync-version.sh maintains. The rest of the repo has many legitimate
# hardcoded version refs (historical changelogs, IP:port fixtures in
# tests, future-version reservations) which are out of scope.
#
# Usage: bash scripts/ci/check_versions.sh
# Exit: 0 = synced, 1 = drift detected
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

if [[ ! -f VERSION ]]; then
    echo "❌ VERSION file not found"
    exit 1
fi

EXPECTED="v$(tr -d '[:space:]' < VERSION)"
PATTERN='v0\.[0-9]+\.[0-9]+\.[0-9]+(\+?)'

# Files that sync-version.sh keeps in sync.
TARGETS=(
    "AGENTS.md"
    "internal/AGENTS_INDEX.md"
    "internal/MATURITY.md"
    "internal/fubonproxy/AGENTS.md"
    "cmd/atlas-mcp/server/AGENTS.md"
)

DRIFT=0
for f in "${TARGETS[@]}"; do
    [[ -f "$f" ]] || continue
    while IFS= read -r match; do
        base="${match%\+}"
        if [[ "$base" != "$EXPECTED" ]]; then
            echo "❌ drift in $f: '$match' (expected '$EXPECTED')"
            DRIFT=$((DRIFT + 1))
        fi
    done < <(grep -vE " 新|完整 release notes" "$f" 2>/dev/null | grep -oE "${PATTERN}" || true)
done

if [[ $DRIFT -gt 0 ]]; then
    echo ""
    echo "Fix: run ./scripts/sync-version.sh"
    exit 1
fi

echo "✅ All $EXPECTED references in TARGETS files are in sync"
