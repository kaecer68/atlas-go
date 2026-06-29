#!/usr/bin/env bash
# scripts/ci/build_all_frontends.sh — Build all frontend assets required by Go embed.
#
# cmd/atlas embeds two frontend dist directories: admin_web/dist and
# client_web/dist. Workflows must build both before any `go build`.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"

for dir in admin_web client_web; do
  echo "==> Building $dir"
  cd "$ROOT_DIR/$dir"
  npm ci --no-audit --no-fund
  npm run build
done

echo "==> All frontend builds complete"
