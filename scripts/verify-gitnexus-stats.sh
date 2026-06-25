#!/bin/bash
# verify-gitnexus-stats.sh — 檢查 GitNexus 索引統計與文件一致性
# Usage: bash scripts/verify-gitnexus-stats.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
META_FILE="$REPO_ROOT/.gitnexus/meta.json"

if [ ! -f "$META_FILE" ]; then
    echo "Error: GitNexus meta file not found at $META_FILE"
    echo "Run: npx gitnexus analyze --skip-agents-md"
    exit 1
fi

# Extract stats from meta.json
NODES=$(python3 -c "import json; print(json.load(open('$META_FILE'))['stats']['nodes'])" 2>/dev/null || echo "")
EDGES=$(python3 -c "import json; print(json.load(open('$META_FILE'))['stats']['edges'])" 2>/dev/null || echo "")
PROCESSES=$(python3 -c "import json; print(json.load(open('$META_FILE'))['stats']['processes'])" 2>/dev/null || echo "")

if [ -z "$NODES" ] || [ -z "$EDGES" ] || [ -z "$PROCESSES" ]; then
    echo "Error: Failed to parse meta.json"
    exit 1
fi

EXPECTED="$NODES symbols, $EDGES relationships, $PROCESSES execution flows"

# Find markdown files with GitNexus stats section
FILES_WITH_STATS=$(grep -rl "This project is indexed by GitNexus" "$REPO_ROOT" --include="*.md" 2>/dev/null || true)

if [ -z "$FILES_WITH_STATS" ]; then
    echo "No markdown files found with GitNexus stats"
    exit 0
fi

MISMATCHES=0
for file in $FILES_WITH_STATS; do
    # Extract the stats line
    STATS_LINE=$(grep "This project is indexed by GitNexus" "$file" || true)
    if [ -z "$STATS_LINE" ]; then
        continue
    fi
    
    # Extract numbers from the line
    FILE_NODES=$(echo "$STATS_LINE" | grep -oE '[0-9]+ symbols' | grep -oE '[0-9]+' || true)
    FILE_EDGES=$(echo "$STATS_LINE" | grep -oE '[0-9]+ relationships' | grep -oE '[0-9]+' || true)
    FILE_PROCESSES=$(echo "$STATS_LINE" | grep -oE '[0-9]+ execution flows' | grep -oE '[0-9]+' || true)
    
    if [ "$FILE_NODES" != "$NODES" ] || [ "$FILE_EDGES" != "$EDGES" ] || [ "$FILE_PROCESSES" != "$PROCESSES" ]; then
        echo "❌ MISMATCH: ${file#$REPO_ROOT/}"
        echo "   Expected: $EXPECTED"
        echo "   Found:    ${FILE_NODES:-?} symbols, ${FILE_EDGES:-?} relationships, ${FILE_PROCESSES:-?} execution flows"
        MISMATCHES=$((MISMATCHES + 1))
    else
        echo "✅ ${file#$REPO_ROOT/} — up to date ($EXPECTED)"
    fi
done

if [ "$MISMATCHES" -gt 0 ]; then
    echo ""
    echo "Found $MISMATCHES file(s) with outdated stats."
    echo "Fix: remove the stats line from markdown files, or run 'npx gitnexus analyze --skip-agents-md' if the index is stale."
    exit 1
else
    echo ""
    echo "All GitNexus stats are up to date ($EXPECTED)"
    exit 0
fi
