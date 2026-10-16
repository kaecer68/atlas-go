#!/usr/bin/env bash
# scripts/ci/check_commit_messages.sh
# Validates PR commits follow semantic commit message format:
#   type(scope): description
#
# Allowed types: feat, fix, refactor, chore, docs, test, ci, style, perf, revert
# Allowed scope: lowercase alphanumeric + . _ - / , separators
#   (supports nested module paths and cross-module scopes)
#
# Usage:
#   bash scripts/ci/check_commit_messages.sh            # check HEAD~1..HEAD
#   bash scripts/ci/check_commit_messages.sh <base>     # check base..HEAD
#   bash scripts/ci/check_commit_messages.sh <base> <head>
#
# Exit codes:
#   0 = every commit in the range follows the format. An empty range (base==head,
#       e.g. `HEAD HEAD`) is a legitimate 0: there is nothing to check.
#   1 = at least one commit message is invalid.
#   2 = the range could not be resolved (git failed) — the check did NOT run, so
#       it must never be reported as a pass (FU-20261016-27). This was live in
#       CI: on push events ci-cd.yml passed `origin/${{ github.base_ref }}`, and
#       base_ref is empty there ⇒ `origin/..HEAD` ⇒ git rc=128 ⇒ the gate used
#       to print ✅ and exit 0 for a range it never read.

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

BASE="${1:-HEAD~1}"
HEAD="${2:-HEAD}"

echo "🔍 Checking commit messages from $BASE..$HEAD for semantic format..."

VALID_TYPES="feat|fix|refactor|chore|docs|test|ci|style|perf|revert"
PATTERN="^($VALID_TYPES)(\([a-z0-9][a-z0-9._,\/-]*\))?: .+|^fixup! .+"

# `git log` FAILING must never be folded into "this range has no commits": both
# produce an empty list, so `2>/dev/null || true` (the previous shape) made them
# indistinguishable and the gate printed ✅ / exited 0 for a range git could not
# resolve (2026-10-16, reproduced: `origin/does-not-exist..HEAD` ⇒ ✅ rc=0, and in
# production CI logs the push run prints "Checking commit messages from
# origin/..HEAD" followed by ✅). Capture the status explicitly, leave git's own
# stderr visible (the operator needs the failing revision), and refuse to judge.
LOG_RC=0
COMMITS=$(git log --no-merges --format="%H %s" "$BASE..$HEAD") || LOG_RC=$?
if [ "$LOG_RC" -ne 0 ]; then
  echo ""
  echo -e "${RED}❌ commit-message check FAILED — git 無法解析 range '$BASE..$HEAD' (git rc=$LOG_RC)${NC}"
  echo "   無法判斷這個 range 裡有哪些 commit ⇒ 不以「沒有問題」放行（fail loud）"
  echo "   修復: 讓兩個端點都可解析（例: git fetch origin && git rev-parse '$BASE'），"
  echo "         或在真的沒有 commit 要檢查時明確傳入相同端點（例: HEAD HEAD）"
  exit 2
fi

HAS_ERRORS=false
if [ -n "$COMMITS" ]; then
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
  done <<< "$COMMITS"
fi

if $HAS_ERRORS; then
  echo ""
  echo -e "${RED}❌ Some commit messages don't follow semantic format.${NC}"
  echo "Fix with: git rebase -i $BASE   (then git commit --amend for each)"
  exit 1
fi

echo -e "${GREEN}✅ All commit messages follow semantic format${NC}"
