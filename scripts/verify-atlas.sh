#!/usr/bin/env bash
# verify-atlas.sh — one-command verification for atlas-go.
#
# Usage:
#   ./scripts/verify-atlas.sh          # full verification
#   ./scripts/verify-atlas.sh --quick  # format + build + vet only

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
QUICK=0

SKIP_FRONTEND=0
QUICK=0
SKIP_FRONTEND=0
for arg in "$@"; do
  case "$arg" in
    --quick) QUICK=1 ;;
    --skip-frontend) SKIP_FRONTEND=1 ;;
  esac
done

cd "$REPO_ROOT"

failures=0

run_check() {
  local name="$1"
  shift
  echo ""
  echo "▶ $name"
  if "$@"; then
    echo "✅ $name passed"
  else
    echo "❌ $name failed"
    failures=$((failures + 1))
  fi
}

# Frontend build is required before `go build` because admin_web/client_web embed `all:dist`.
if [[ "$SKIP_FRONTEND" -eq 0 ]]; then
  run_check "frontend build" bash -c '
    for app in admin_web client_web; do
      if [[ ! -d "$app/dist" ]]; then
        if [[ ! -d "$app/node_modules" ]]; then
          echo "$app/node_modules missing; installing..."
          (cd "$app" && npm install)
        fi
        (cd "$app" && npm run build)
      fi
    done
  '
else
  # Create minimal dist directories so go build can compile backend without frontend assets.
  for app in admin_web client_web; do
    mkdir -p "$app/dist"
    touch "$app/dist/.verify-placeholder"
  done
fi

# Always-run fast checks
run_check "gofmt" bash -c 'test -z "$(gofmt -l .)"'
run_check "go build ./..." go build ./...
run_check "go vet ./..." go vet ./...

if [[ "$QUICK" -eq 0 ]]; then
  run_check "go test ./..." go test ./...
  if command -v golangci-lint >/dev/null 2>&1; then
    run_check "golangci-lint run" golangci-lint run ./...
  else
    run_check "staticcheck ./..." staticcheck ./...
    echo "⚠️  golangci-lint not found; CI uses golangci-lint, consider installing it."
  fi

  # Verify manifests if any exist.
  if compgen -G "docs/manifests/*.md" > /dev/null; then
    for manifest in docs/manifests/*.md; do
      if [[ "$(basename "$manifest")" == "README.md" || "$(basename "$manifest")" == "TEMPLATE.md" ]]; then
        continue
      fi
      run_check "verify manifest $(basename "$manifest")" ./scripts/verify-manifest.sh "$manifest"
    done
  fi

  # Drift check: AGENTS.md links should point to existing files.
  run_check "AGENTS.md link drift" bash -c '
    grep -oE "\[([^]]+)\]\(([^)]+)\)" AGENTS.md | \
    grep -oE "\]\([^)]+\)" | tr -d "]()" | \
    while read -r link; do
      case "$link" in
        http*|https*) continue ;;
        *.md)
          if [[ ! -f "$link" && ! -d "$(dirname "$link")" ]]; then
            echo "Broken link in AGENTS.md: $link"
            exit 1
          fi
          ;;
      esac
    done
  '

  # .omo/ whitelist check (skip if .omo/ does not exist).
  if [[ -d ".omo" ]]; then
    run_check ".omo/ whitelist" bash -c '
      allowed="briefs plans evidence traces notepads handoffs workspaces run-continuation phaseN wave-N maps boulder.json"
      for entry in .omo/*/ .omo/*; do
        [[ -e "$entry" ]] || continue
        name=$(basename "$entry")
        found=0
        for a in $allowed; do
          if [[ "$name" == "$a" ]]; then
            found=1
            break
          fi
        done
        if [[ "$found" -eq 0 ]]; then
          echo "Disallowed .omo/ entry: $name"
          exit 1
        fi
      done
    '
  fi
fi

echo ""
if [[ "$failures" -eq 0 ]]; then
  echo "🎉 All atlas-go verification checks passed."
  exit 0
else
  echo "💥 $failures check(s) failed."
  exit 1
fi
