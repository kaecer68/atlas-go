#!/bin/sh
# atlas-go — install git hooks from .githooks/
# Run once per clone, or after pulling new hooks.
#   bash scripts/install-hooks.sh

set -e

HOOK_SRC=".githooks"
HOOK_DST=".git/hooks"

echo "🔗 Installing git hooks from ${HOOK_SRC}/ → ${HOOK_DST}/"

if [ ! -d ".git" ]; then
  echo "ERROR: Not in a git repository root."
  exit 1
fi

mkdir -p "${HOOK_DST}"

installed=0
for hook in "${HOOK_SRC}"/*; do
  name=$(basename "$hook")
  if [ -f "$hook" ] && [ ! -L "${HOOK_DST}/${name}" ]; then
    cp "$hook" "${HOOK_DST}/${name}"
    chmod +x "${HOOK_DST}/${name}"
    echo "  ✅ installed: ${name}"
    installed=$((installed + 1))
  elif [ -L "${HOOK_DST}/${name}" ]; then
    echo "  ⏭️  skipped (symlink): ${name}"
  else
    echo "  ⏭️  skipped (exists): ${name}"
  fi
done

echo ""
echo "📋 ${installed} hook(s) installed."
echo "   Re-run this script after pulling new hooks from upstream."

# Also recommend core.hooksPath for future consistency
current=$(git config core.hooksPath 2>/dev/null || echo "")
if [ "$current" != "${HOOK_SRC}" ]; then
  echo ""
  echo "💡 Tip: set core.hooksPath to ${HOOK_SRC} for automatic syncing:"
  echo "   git config core.hooksPath ${HOOK_SRC}"
fi
