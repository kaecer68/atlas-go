#!/usr/bin/env bash
# verify_api_contract.sh — Cross-checks frontend API calls against registered backend routes.
# CI gate: fails if frontend calls an endpoint with no backend handler.
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
JS_DIR="$ROOT_DIR/web/static/js"
FRONTEND=$( ( \
  find "$JS_DIR" -name '*.js' -exec grep -oh "getJSON('/api/[^?']*" {} + 2>/dev/null | sed "s/getJSON('//"; \
  find "$JS_DIR" -name '*.js' -exec grep -oh "postJSON('/api/[^?']*" {} + 2>/dev/null | sed "s/postJSON('//"; \
  find "$JS_DIR" -name '*.js' -exec grep -oh "fetch('/api/[^?']*" {} + 2>/dev/null | sed "s/fetch('//" \
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
