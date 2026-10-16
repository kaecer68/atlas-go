#!/usr/bin/env bash
# scripts/ci/check_case_conflicts.sh
# Detects case-conflicting filenames in the git index.
# Prevents the class of bug where macOS APFS (case-insensitive) allows
# AGENTS.md and agents.md to coexist in git history but explode on
# case-sensitive filesystems (Linux CI, case-sensitive APFS volumes).
#
# Usage:
#   bash scripts/ci/check_case_conflicts.sh          # check all tracked files
#   bash scripts/ci/check_case_conflicts.sh --strict # exit 1 on a non-allowlisted name
#                                                    # (only flags the add-on check below
#                                                    # treats as violations; the
#                                                    # case-conflict check always exits 1)
# Unknown flags exit 2: a misspelled flag must not silently fall back to the
# warning-only mode (FU-20261016-23).

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Arg parsing is explicit: an unknown flag is a hard error. Silently ignoring it
# used to turn a typo (`--stric`) into the WARNING-only default, i.e. the same
# "gate that cannot fail" shape this flag exists to avoid (FU-20261016-23).
# Exit code 2 = usage error, matching check_frontend_dist.sh / check-binary-freshness.sh.
STRICT=false
case "${1:-}" in
    "")
        ;;
    --strict)
        STRICT=true
        ;;
    *)
        echo "unknown option: $1" >&2
        echo "usage: check_case_conflicts.sh [--strict]" >&2
        exit 2
        ;;
esac

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

    # Files/dirs explicitly allowed to use non-snake_case.
    # Every entry must name a **tool-enforced** convention (this check's own
    # stated criterion), not a one-off exemption:
    #   * GitHub community-health files — the uppercase name IS the recognised
    #     spelling: README.md, LICENSE, NOTICE, CHANGELOG.md, CONTRIBUTING.md,
    #     SECURITY.md
    #   * AI-tool instruction files — AGENTS.md is a published cross-tool spec
    #     (https://agents.md); CLAUDE.md is Claude Code's convention
    #   * Build / container entrypoints — Makefile, VERSION and the
    #     `Dockerfile<.suffix>` variants (docker keeps the `Dockerfile` stem)
    # NOTE: the three dotfile entries are unreachable today — the pipeline above
    # runs `grep -v '^\.'` first, so dotfiles never reach this loop. Kept as
    # documentation of intent (FU-20261016-23 已知限制).
    ALLOWLIST=(
        "README.md"
        "LICENSE"
        "NOTICE"
        "CHANGELOG.md"
        "CONTRIBUTING.md"
        "SECURITY.md"
        "AGENTS.md"
        "CLAUDE.md"
        "Makefile"
        "VERSION"
        "Dockerfile"
        "Dockerfile.cron"
        "Dockerfile.atlas.local"
        "Dockerfile.cron.local"
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
        VIOLATION_COUNT=$(printf '%s\n' "$VIOLATIONS" | wc -l | tr -d ' ')
        echo -e "${RED}❌ Non-snake_case files found:${NC}"
        echo "$VIOLATIONS"
        echo ""
        echo "These should use snake_case unless there's a tool-enforced convention."
        echo "To suppress: add to ALLOWLIST in this script."
        echo ""
        # `--strict` means "treat this as a gate": a non-allowlisted name must end
        # in a non-zero exit. Until FU-20261016-23 this branch only printed a
        # warning, so the mode CI (quality.yml `naming`) and local-ci.sh call was
        # indistinguishable from the default run — the flag had no teeth.
        echo -e "${RED}❌ --strict: ${VIOLATION_COUNT} file(s) above are not in the ALLOWLIST${NC}"
        exit 1
    else
        echo -e "${GREEN}✅ All files follow snake_case convention${NC}"
    fi
fi

echo ""
echo "Done."
