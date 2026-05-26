#!/usr/bin/env bash
# regenerate-subgraphs.sh
#
# One-command workflow to update the master graph and regenerate all sub-graph
# visualizations (core, analysis, research, infra).
#
# Usage:
#   bash scripts/regenerate-subgraphs.sh
#   bash scripts/regenerate-subgraphs.sh --dry-run   # preview without running
#
# This runs:
#   1. graphify update .  — re-extract code files, update graph.json (AST-only, no LLM cost)
#   2. python3 scripts/slice-graph.py  — split master graph into 4 sub-graphs + HTML

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=true
fi

echo "=== regenerate-subgraphs ==="
echo "Project: $PROJECT_DIR"
echo "Dry-run: $DRY_RUN"
echo ""

# --- Step 1: Update master graph ---
echo "[1/2] Updating master graph (graphify update .)..."
if $DRY_RUN; then
  echo "  SKIP: graphify update \"$PROJECT_DIR\""
else
  # graphify may warn "too large for HTML viz" but graph.json is still valid.
  # Ignore non-zero exit — the slicing step generates its own HTML per subgraph.
  set +e
  graphify update "$PROJECT_DIR"
  set -e
  echo "  Done."
fi
echo ""

# --- Step 2: Regenerate sub-graphs from updated master ---
echo "[2/2] Regenerating sub-graph visualizations..."
if $DRY_RUN; then
  echo "  SKIP: python3 \"$SCRIPT_DIR/slice-graph.py\""
else
  python3 "$SCRIPT_DIR/slice-graph.py"
  echo "  Done."
fi
echo ""

# --- Summary ---
if $DRY_RUN; then
  echo "=== DRY-RUN complete. Pass no arguments to execute. ==="
else
  echo "=== Done ==="
  ls -lh "$PROJECT_DIR/graphify-out/subgraphs/"*/graph.html "$PROJECT_DIR/graphify-out/graph.json" 2>/dev/null
fi
