#!/usr/bin/env bash
# =============================================================================
# check_frontend_dist.sh — frontend dist freshness gate (pre-push / CI)
#
# Verifies each frontend's dist/ exists and is not older than its sources.
# The Go backend embeds admin_web/dist + client_web/dist via go:embed
# (admin_web/embed.go, client_web/embed.go); a stale dist silently ships the
# old UI into the binary, and a missing dist makes embedded assets empty.
#
# Usage:
#   bash scripts/ci/check_frontend_dist.sh                     # check all frontends
#   bash scripts/ci/check_frontend_dist.sh admin_web client_web  # specific dirs
#   bash scripts/ci/check_frontend_dist.sh --diff origin/main...HEAD
#       # check only frontends whose files changed in the git range;
#       # no frontend files changed → exit 0 (pure Go/docs pushes skip)
#
# Exit 0 = all checked dists fresh (or nothing to check)
# Exit 1 = at least one dist missing / stale
# =============================================================================
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

FRONTENDS=(admin_web client_web)
# shared_web has no dist build of its own, but its sources are imported by
# admin_web/client_web (esbuild fallback) — a shared_web change must rebuild
# both dists, so it also triggers the check.
TRIGGER_PATHS=(admin_web client_web shared_web)

fail() { echo "  ❌ $*" >&2; exit 1; }

usage() {
  echo "usage: check_frontend_dist.sh [--diff <git-range>] [dir ...]" >&2
  exit 2
}

# mtime_of FILE → epoch seconds (portable: BSD `stat -f` vs GNU `stat -c`).
mtime_of() {
  if stat -f '%m' "$1" >/dev/null 2>&1; then
    stat -f '%m' "$1"
  else
    stat -c '%Y' "$1"
  fi
}

# newest_mtime DIR → epoch seconds of newest file under DIR (excluding
# node_modules/dist), or empty if no files.
#
# Excludes the two go:generate outputs (cmd/gentags): ci-gate runs
# `go generate ./...` unconditionally before this check, and gentags
# rewrites field_types.ts + valid_fields.json even when content is
# identical → their mtime is ALWAYS newer than any dist build, which
# would false-positive every frontend push as "dist 過期". Content drift
# of these generated files is already caught by ci-gate's
# `git diff --exit-code` (drift check), so excluding them is safe.
newest_mtime() {
  local dir=$1
  find "$dir" -type f \
    -not -path "$dir/node_modules/*" \
    -not -path "$dir/dist/*" \
    -not -path "$dir/static/js/shared/field_types.ts" \
    -not -path "$dir/static/js/shared/valid_fields.json" \
    -print0 2>/dev/null | while IFS= read -r -d '' f; do mtime_of "$f"; done | sort -nr | head -1
}

# dist_newest_mtime DIR → epoch seconds of newest file inside DIR/dist, or empty.
dist_newest_mtime() {
  local d=$1
  find "$d/dist" -type f -print0 2>/dev/null | while IFS= read -r -d '' f; do mtime_of "$f"; done | sort -nr | head -1
}

check_one() {
  local dir=$1
  if [ ! -f "$dir/package.json" ]; then
    echo "  - $dir: not a frontend (no package.json) — skip"
    return 0
  fi
  if [ ! -d "$dir/dist" ] || [ -z "$(ls -A "$dir/dist" 2>/dev/null)" ]; then
    echo "  ❌ $dir: dist/ 不存在或為空 — 先跑 (cd $dir && npm ci && npm run build)"
    return 1
  fi
  local src_max dist_max
  src_max=$(newest_mtime "$dir")
  dist_max=$(dist_newest_mtime "$dir")  # uses $dir from check_one
  if [ -z "$src_max" ] || [ -z "$dist_max" ]; then
    echo "  ❌ $dir: 無法計算 dist 新鮮度 (src=$src_max dist=$dist_max)"
    return 1
  fi
  if [ "$src_max" -gt "$dist_max" ]; then
    echo "  ❌ $dir: dist 過期 — 有源碼比 dist 新 (src=$src_max > dist=$dist_max)"
    echo "     修復: (cd $dir && npm ci && npm run build)"
    return 1
  fi
  echo "  ✓ $dir: dist 最新"
  return 0
}

DIRS=()
DIFF_MODE=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --diff)
      DIFF_MODE=1
      RANGE="$2"
      shift 2
      ;;
    -h|--help) usage ;;
    -*) echo "unknown option: $1" >&2; usage ;;
    *) DIRS+=("$1"); shift ;;
  esac
done

echo "=== Frontend dist freshness check ==="

if [ "$DIFF_MODE" -eq 1 ]; then
  changed=$(git diff --name-only "$RANGE" 2>/dev/null || true)
  trigger=0
  for p in "${TRIGGER_PATHS[@]}"; do
    if printf '%s\n' "$changed" | grep -q "^$p/"; then
      trigger=1
      break
    fi
  done
  if [ "$trigger" -eq 0 ]; then
    echo "  ℹ️  diff 未含 admin_web/client_web/shared_web 檔案 — 跳過 dist 檢查"
    echo "✅ check_frontend_dist: nothing to check"
    exit 0
  fi
  # changed frontend dirs only (admin_web/client_web build their own dist)
  for d in "${FRONTENDS[@]}"; do
    if printf '%s\n' "$changed" | grep -q "^$d/"; then
      DIRS+=("$d")
    fi
  done
  if [ "${#DIRS[@]}" -eq 0 ]; then
    # only shared_web changed → both frontends embed it → check both
    DIRS=("${FRONTENDS[@]}")
  fi
else
  [ "${#DIRS[@]}" -eq 0 ] && DIRS=("${FRONTENDS[@]}")
fi

rc=0
for d in "${DIRS[@]}"; do
  check_one "$d" || rc=1
done

if [ "$rc" -eq 0 ]; then
  echo "✅ check_frontend_dist: all dist fresh"
else
  echo "❌ check_frontend_dist FAILED — dist 過期/缺失, 修復後再 push"
fi
exit "$rc"
