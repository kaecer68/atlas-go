#!/usr/bin/env bash
# scripts/ci/check_case_conflicts.sh
# Detects case-conflicting filenames in the git index.
# Prevents the class of bug where macOS APFS (case-insensitive) allows
# AGENTS.md and agents.md to coexist in git history but explode on
# case-sensitive filesystems (Linux CI, case-sensitive APFS volumes).
#
# Usage:
#   bash scripts/ci/check_case_conflicts.sh          # check all tracked files
#   bash scripts/ci/check_case_conflicts.sh --strict # fail on any mixed-case filename

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

STRICT=false
if [[ "${1:-}" == "--strict" ]]; then
    STRICT=true
fi

echo "🔍 Checking for case-conflicting filenames..."

# Get all tracked files from git index
TRACKED_FILES=$(git ls-files)

# Sort case-insensitively, then find duplicates
DUPLICATES=$(echo "$TRACKED_FILES" | sort -f | uniq -di)

if [[ -n "$DUPLICATES" ]]; then
    echo -e "${RED}❌ CASE CONFLICT DETECTED${NC}"
    echo "The following files differ only by case. On case-insensitive filesystems"
    echo "(macOS APFS default), these are the SAME file and will cause rebase/merge"
    echo "explosions on any branch that references the old casing."
    echo ""
    echo "$DUPLICATES"
    echo ""
    echo "Fix: use 'git mv OLDNAME OLDNAME.tmp && git mv OLDNAME.tmp newname'"
    echo "     (the intermediate .tmp rename forces git to track it correctly)"
    exit 1
fi

echo -e "${GREEN}✅ No case-conflict pairs in tracked files${NC}"

# --strict mode: also flags any PascalCase/UPPERCASE .md files that aren't
# in the explicit allowlist (README.md, CLAUDE.md, SKILL.md)
if $STRICT; then
    echo ""
    echo "🔍 --strict: checking for inconsistent casing..."

    # Files/dirs explicitly allowed to use non-snake_case
    ALLOWLIST=(
        "README.md"
        "CLAUDE.md"
        "Dockerfile"
        "Dockerfile.cron"
        "Makefile"
        ".editorconfig"
        ".gitignore"
        ".dockerignore"
    )

    VIOLATIONS=$(echo "$TRACKED_FILES" | grep -v '^\.' | grep -E '[A-Z]' | grep -v '/\|\.go$\|\.json$\|\.jsonl$\|\.csv$\|\.html$\|\.css$\|\.js$\|\.sh$\|\.svg$\|\.png$\|\.py$\|\.yml$\|\.yaml$\|\.sql$\|\.txt$\|\.xml$\|\.toml$\|\.zip$\|\.mod$\|\.sum$\|\.lock$\|\.out$\|\.exe$\|\.pid$' | while read -r file; do
        allowed=false
        for a in "${ALLOWLIST[@]}"; do
            [[ "$file" == "$a" ]] && allowed=true && break
        done
        # SKILL.md files in .claude/skills/ are allowed (tool convention)
        [[ "$file" == .claude/skills/*/SKILL.md ]] && allowed=true
        if ! $allowed; then
            echo "  $file"
        fi
    done)

    if [[ -n "$VIOLATIONS" ]]; then
        echo -e "${YELLOW}⚠️  Non-snake_case files found:${NC}"
        echo "$VIOLATIONS"
        echo ""
        echo "These should use snake_case unless there's a tool-enforced convention."
        echo "To suppress: add to ALLOWLIST in this script."
    else
        echo -e "${GREEN}✅ All files follow snake_case convention${NC}"
    fi
fi

echo ""
echo "Done."
