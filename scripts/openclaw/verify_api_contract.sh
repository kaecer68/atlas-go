#!/usr/bin/env bash
# verify_api_contract.sh — Cross-checks frontend API calls against registered backend routes.
#
# NOTE: This script is currently experimental and NOT enforced in CI. Its backend
# route extraction only greps literal route strings in a subset of Go source
# directories and misses routes registered via helpers, variables, or in other
# packages. As a result it reports many false-positive "missing backend handlers".
# See the web/ removal audit (2026-06-29) for details. Do not treat a failure here
# as a hard blocker until the extraction logic is rebuilt (e.g. using the same
# AST approach as cmd/mapgen).
#
# Usage: ./scripts/openclaw/verify_api_contract.sh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# ---- Backend: extract all registered route paths ----
BACKEND=$(find "$ROOT_DIR/internal/monitoring" -name '*.go' -not -name '*_test.go' \
  -exec grep -oh '"\(/api\|/admin\|/health\|/metrics\)/[^"]*"' {} + 2>/dev/null | \
  tr -d '"' | sort -u)

# ---- Frontend: extract all API endpoints called ----
# getJSON, postJSON, fetch() with /api/ prefix
# Strip query params for matching
JS_DIRS=(
  "$ROOT_DIR/admin_web/static/js"
  "$ROOT_DIR/client_web/static/js"
  "$ROOT_DIR/shared_web/static/js"
)
FRONTEND=$( ( \
  for js_dir in "${JS_DIRS[@]}"; do \
    find "$js_dir" -name '*.js' -exec grep -oh "getJSON('/api/[^?']*" {} + 2>/dev/null | sed "s/getJSON('//"; \
    find "$js_dir" -name '*.js' -exec grep -oh "postJSON('/api/[^?']*" {} + 2>/dev/null | sed "s/postJSON('//"; \
    find "$js_dir" -name '*.js' -exec grep -oh "fetch('/api/[^?']*" {} + 2>/dev/null | sed "s/fetch('//"; \
  done \
) | sort -u)

# ---- Analysis ----
MISSING=()
while IFS= read -r route; do
  [[ -z "$route" ]] && continue
  if ! echo "$BACKEND" | grep -qFx "$route"; then
    MISSING+=("$route")
  fi
done <<< "$FRONTEND"

# ---- Report ----
echo "=== Atlas API Contract Validation ==="
echo "Backend routes: $(echo "$BACKEND" | grep -c '[^[:space:]]')"
echo "Frontend calls: $(echo "$FRONTEND" | grep -c '[^[:space:]]')"
echo ""

if [[ ${#MISSING[@]} -gt 0 ]]; then
  echo "❌ FRONTEND CALLS WITH NO BACKEND HANDLER:"
  for route in "${MISSING[@]}"; do
    echo "   $route"
  done
  echo ""
  echo "❌ CONTRACT BROKEN — fix missing backend handlers before merging."
  exit 1
else
  echo "✅ All frontend API calls have matching backend handlers."
fi
