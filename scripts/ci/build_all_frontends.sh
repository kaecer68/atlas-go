#!/usr/bin/env bash
# scripts/ci/build_all_frontends.sh — Build all frontend assets required by Go embed.
#
# cmd/atlas embeds three frontend dist directories:
#   - web/dist
#   - admin_web/dist
#   - client_web/dist
#
# Workflows and smoke tests must build all three before any `go build ./...`.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

for dir in web admin_web client_web; do
  echo "==> Building $dir"
  cd "$REPO_ROOT/$dir"
  npm ci --no-audit --no-fund
  npm run build
  echo "==> $dir built"
done
