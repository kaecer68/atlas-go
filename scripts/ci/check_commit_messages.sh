#!/usr/bin/env bash
# scripts/ci/check_commit_messages.sh
# Validates PR commits follow semantic commit message format:
#   type(scope): description
#
# Allowed types: feat, fix, refactor, chore, docs, test, ci, style, perf, revert
# Allowed scope: any alphanumeric + hyphen module name
#
# Usage:
#   bash scripts/ci/check_commit_messages.sh            # check HEAD~1..HEAD
#   bash scripts/ci/check_commit_messages.sh <base>     # check base..HEAD
#   bash scripts/ci/check_commit_messages.sh <base> <head>

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

BASE="${1:-HEAD~1}"
HEAD="${2:-HEAD}"

echo "🔍 Checking commit messages from $BASE..$HEAD for semantic format..."

VALID_TYPES="feat|fix|refactor|chore|docs|test|ci|style|perf|revert"
PATTERN="^($VALID_TYPES)(\([a-z0-9][a-z0-9._-]*\))?: .+"

HAS_ERRORS=false
while IFS= read -r commit; do
  hash=$(echo "$commit" | awk '{print $1}')
  msg=$(echo "$commit" | cut -d' ' -f2-)
  if ! echo "$msg" | grep -qE "$PATTERN"; then
    echo -e "${RED}❌ INVALID: $hash${NC}"
    echo "   Message: $msg"
    echo "   Expected format: type(scope): description"
    echo "   Allowed types: ${VALID_TYPES//|/, }"
    HAS_ERRORS=true
  fi
done < <(git log --no-merges --format="%H %s" "$BASE..$HEAD" 2>/dev/null || true)

if $HAS_ERRORS; then
  echo ""
  echo -e "${RED}❌ Some commit messages don't follow semantic format.${NC}"
  echo "Fix with: git rebase -i $BASE   (then git commit --amend for each)"
  exit 1
fi

echo -e "${GREEN}✅ All commit messages follow semantic format${NC}"
