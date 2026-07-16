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

# Ensure hooks are executable.
chmod +x "${HOOK_DIR}/deny-dangerous.sh"

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
