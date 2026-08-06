#!/usr/bin/env bash
# install.sh — install agent hooks into this repo.
#
# Usage:
#   bash .agent-hooks/install.sh
#
# This makes the deny-dangerous hook available for agent sessions and
# optionally wires it into the shell environment. Git hooks are installed
# via scripts/install-hooks.sh (pre-existing); this script is for the
# agent-side guardrails.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

HOOK_DIR="${REPO_ROOT}/.agent-hooks"

echo "Installing agent hooks from ${HOOK_DIR}..."

# Ensure both hooks are executable.
chmod +x "${HOOK_DIR}/deny-dangerous.sh"
chmod +x "${HOOK_DIR}/aci-read-prompt.sh"

# Symlink a convenient alias in the repo root (gitignored by convention).
# Users can call ./agent-guard instead of .agent-hooks/deny-dangerous.sh.
if [[ ! -e "${REPO_ROOT}/agent-guard" ]]; then
  ln -s "${HOOK_DIR}/deny-dangerous.sh" "${REPO_ROOT}/agent-guard"
  echo "  Created alias: ./agent-guard"
fi

# Print usage.
echo ""
echo "Agent hooks installed."
echo ""
echo "Usage examples:"
echo "  ./agent-guard --check 'git push origin main'"
echo "  ./agent-guard --mode=enforce --check 'rm -rf /'"
echo "  ./agent-guard --dry-run"
echo ""
echo "To enforce in a production worktree, set ATLAS_ENV=production or ATLAS_HOOK_MODE=enforce."

# Check jq dependency for aci-read-prompt.sh.
if ! command -v jq >/dev/null 2>&1; then
  echo ""
  echo "⚠️  jq not found. aci-read-prompt.sh requires jq (1.6+)."
  echo "   Install: brew install jq    # macOS"
  echo "           sudo apt install jq   # Debian/Ubuntu"
  echo "   Without jq, the hook silently skips (graceful degradation)."
fi

# Inform the user about the soft-prompt hook activation.
SETTINGS_LOCAL="${REPO_ROOT}/.claude/settings.local.json"
if [[ -f "${SETTINGS_LOCAL}" ]]; then
  echo ""
  echo "✓ .claude/settings.local.json exists — ACI reminder hook ACTIVE for this worktree."
else
  echo ""
  echo "ℹ️  ACI reminder hook (soft prompt) NOT yet active for this worktree."
  echo "   To enable per-user, see: docs/operations/aci-hook-usage.md"
  echo "   Quick start: cp .claude/settings.json .claude/settings.local.json  # then edit to add PreToolUse entry"
fi

