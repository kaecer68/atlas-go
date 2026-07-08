#!/bin/sh
# atlas-go — install git hooks from .githooks/
# Run once per clone, or after pulling new hooks.
#   bash scripts/install-hooks.sh
#
# This script:
# 1. Sets core.hooksPath to .githooks/ (so git reads hooks directly,
#    no re-run needed when new hooks are added)
# 2. Defense-in-depth: also syncs hooks to .git/hooks/ in case
#    core.hooksPath gets reset by some other tool
# 3. Self-test: verifies commit-msg hook actually catches a known
#    violation (catches the "hook shipped but not installed" failure
#    mode that bit PR #1001 capitalflow scope case and PR #1019
#    L2.4 scope case)

set -e

HOOK_SRC=".githooks"
HOOK_FALLBACK_DST=".git/hooks"

echo "🔗 atlas-go git hooks installer"
echo ""

if [ ! -d ".git" ]; then
  echo "ERROR: Not in a git repository root."
  exit 1
fi

# Step 1: set core.hooksPath so git reads hooks directly from .githooks/
# This means future hook additions are auto-picked up — no re-run needed.
current=$(git config core.hooksPath 2>/dev/null || echo "")
if [ "$current" = "${HOOK_SRC}" ]; then
  echo "✅ core.hooksPath already set to ${HOOK_SRC}"
else
  git config core.hooksPath "${HOOK_SRC}"
  echo "✅ Set core.hooksPath = ${HOOK_SRC} (git reads hooks directly)"
fi

# Step 2: defense in depth — also copy into .git/hooks/ so hooks work
# even if core.hooksPath gets reset by some other tool.
mkdir -p "${HOOK_FALLBACK_DST}"
synced=0
for hook in "${HOOK_SRC}"/*; do
  name=$(basename "$hook")
  if [ -f "$hook" ]; then
    if [ ! -f "${HOOK_FALLBACK_DST}/${name}" ] || [ "${HOOK_FALLBACK_DST}/${name}" -ot "$hook" ]; then
      cp "$hook" "${HOOK_FALLBACK_DST}/${name}"
      chmod +x "${HOOK_FALLBACK_DST}/${name}"
      synced=$((synced + 1))
    fi
  fi
done
if [ "$synced" -gt 0 ]; then
  echo "✅ Synced ${synced} hook(s) to ${HOOK_FALLBACK_DST}/ (fallback)"
fi

# Step 3: self-test — verify commit-msg hook catches a known violation.
# This catches the "hook shipped but not installed" failure mode that bit
# PR #1001 (capitalflow scope case, uppercase P0-1) and PR #1019
# (L2.4 scope case, uppercase L in L2.4).
echo ""
echo "🧪 Self-test: verifying commit-msg hook catches uppercase scope..."

# Resolve hook path the same way git would
HOOK_PATH=$(git rev-parse --git-path hooks/commit-msg 2>/dev/null)
if [ -z "$HOOK_PATH" ] || [ ! -f "$HOOK_PATH" ]; then
  echo "❌ FAIL: commit-msg hook not found at ${HOOK_PATH}"
  echo "   Inspect .githooks/commit-msg — it should exist and be executable."
  exit 1
fi

TEST_MSG_FILE=$(mktemp)
# shellcheck disable=SC2064
trap 'rm -f "$TEST_MSG_FILE"' EXIT
echo "feat(P0-1): this should be rejected" > "$TEST_MSG_FILE"

if sh "$HOOK_PATH" "$TEST_MSG_FILE" >/dev/null 2>&1; then
  echo "❌ FAIL: hook did NOT reject 'feat(P0-1):' — uppercase scope passed"
  echo "   Hook content may have been corrupted. Inspect: ${HOOK_PATH}"
  exit 1
else
  echo "✅ Hook correctly rejected 'feat(P0-1):' — uppercase scope caught"
fi

echo ""
echo "✅ All hooks installed and verified."
echo "   If you ever reset core.hooksPath, re-run: bash scripts/install-hooks.sh"