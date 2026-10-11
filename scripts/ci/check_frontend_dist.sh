#!/usr/bin/env bash
# =============================================================================
# check_frontend_dist.sh — frontend dist freshness gate (pre-push / CI)
#
# Verifies each frontend's dist/ exists and no build input inside it is older
# than the newest file in dist/.
# The Go backend embeds admin_web/dist + client_web/dist via go:embed
# (admin_web/embed.go, client_web/embed.go); a stale dist silently ships the
# old UI into the binary, and a missing dist makes embedded assets empty.
# 2026-10-07 (the incident this gate now also guards on the deploy path):
# Phase 0 removed the 錢潮預測卡 from the sources, but the deployed binary still
# served /client/js/home-*.js containing home-predictions — `make rebuild-all`
# never rebuilt the frontend.
#
# Judged inputs = the trees <DIR>/esbuild.config.mjs actually reads:
#   <DIR>/static + shared_web/static (see the comment above is_generated).
# A dist holding only embed-dirs' `.keep` placeholder counts as NOT built.
#
# Usage:
#   bash scripts/ci/check_frontend_dist.sh                     # check all frontends
#   bash scripts/ci/check_frontend_dist.sh admin_web client_web  # specific dirs
#   bash scripts/ci/check_frontend_dist.sh --diff origin/main...HEAD
#       # check only frontends whose files changed in the git range;
#       # no frontend files changed → exit 0 (pure Go/docs pushes skip)
#   make check-frontend-dist                                   # same, from Makefile
#
# Exit 0 = all checked dists fresh (or nothing to check)
# Exit 1 = at least one dist missing / stale
# ============================================================================
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

# source trees — what esbuild actually reads (taken from <DIR>/esbuild.config.mjs,
# not guessed):
#   · entryPoints live in <DIR>/static/js + shared_web/static/js (+ the css
#     entry in shared_web/static/css), and createSharedPlugin
#     (shared_web/esbuild-shared-plugin.mjs) resolves missing relative imports
#     across <DIR>/static <-> shared_web/static in BOTH directions
#     ⇒ both trees are build inputs, for both frontends.
#   · static/index.html is copied verbatim into dist by the copy-index-html
#     plugin ⇒ also an input.
# Everything else under <DIR>/ (tests/, smoke/, scripts/, playwright configs,
# package.json) is NOT read by the build ⇒ editing it must not report staleness.
SHARED_STATIC=shared_web/static

# is_generated FILE → 0 when FILE is a cmd/gentags go:generate output.
#
# ci-gate runs `go generate ./...` unconditionally before this check, and
# gentags rewrites field_types.ts + valid_fields.json even when the content is
# identical → their mtime is ALWAYS newer than any dist build, which would
# false-positive every frontend push as "dist 過期". Content drift of these
# generated files is already caught by ci-gate's `git diff --exit-code` (drift
# check), so excluding them is safe.
is_generated() {
  case "$1" in
    */static/js/shared/field_types.ts | */static/js/shared/valid_fields.json) return 0 ;;
    *) return 1 ;;
  esac
}

# stamp FILE → "epoch<TAB>path" (split back into two fields by check_one).
stamp() {
  printf '%s\t%s\n' "$(mtime_of "$1")" "$1"
}

# newest_source_file DIR → "epoch<TAB>path" of the newest non-generated build
# input of DIR, or empty when DIR has no source tree.
newest_source_file() {
  local dir=$1 roots=() t
  for t in "$dir/static" "$SHARED_STATIC"; do
    if [ -d "$t" ]; then roots+=("$t"); fi
  done
  if [ "${#roots[@]}" -eq 0 ]; then return 0; fi
  find "${roots[@]}" -type f -not -path '*/node_modules/*' -print0 2>/dev/null \
    | while IFS= read -r -d '' f; do
        if ! is_generated "$f"; then stamp "$f"; fi
      done \
    | sort -k1,1nr -k2,2 | head -1
}

# newest_dist_file DIR → "epoch<TAB>path" of the newest real file in DIR/dist.
# A bare `.keep` does NOT count: `make embed-dirs` creates that placeholder so
# `//go:embed all:dist` always finds a directory in a fresh worktree — a dist
# holding nothing but the placeholder has not been built.
newest_dist_file() {
  local dir=$1
  if [ ! -d "$dir/dist" ]; then return 0; fi
  find "$dir/dist" -type f -not -name '.keep' -print0 2>/dev/null \
    | while IFS= read -r -d '' f; do stamp "$f"; done \
    | sort -k1,1nr -k2,2 | head -1
}

check_one() {
  local dir=$1
  if [ ! -f "$dir/package.json" ]; then
    echo "  - $dir: not a frontend (no package.json) — skip"
    return 0
  fi
  if [ ! -d "$dir/dist" ]; then
    echo "  ❌ $dir: dist/ 不存在 — 先跑 make build-frontend"
    return 1
  fi
  local src_line dist_line src_max src_path dist_max dist_path
  src_line=$(newest_source_file "$dir" || true)
  dist_line=$(newest_dist_file "$dir" || true)
  if [ -z "$dist_line" ]; then
    echo "  ❌ $dir: dist/ 內沒有建置產物（空目錄，或只有 embed-dirs 的 .keep 佔位）"
    echo "     修復: make build-frontend"
    return 1
  fi
  if [ -z "$src_line" ]; then
    echo "  ❌ $dir: 找不到來源檔（$dir/static 或 \"$SHARED_STATIC\"）— 無法判斷 dist 新鮮度"
    return 1
  fi
  IFS=$'\t' read -r src_max src_path <<<"$src_line"
  IFS=$'\t' read -r dist_max dist_path <<<"$dist_line"
  if [ "$src_max" -gt "$dist_max" ]; then
    echo "  ❌ $dir: dist 過期 — 下列來源檔比 dist 內任何檔案新"
    echo "     來源: $src_path (mtime=$src_max)"
    echo "     dist: $dist_path (mtime=$dist_max)"
    echo "     修復: make build-frontend"
    return 1
  fi
  echo "  ✓ $dir: dist 最新 (來源最新=$src_path, dist 最新=$dist_path)"
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
  echo "   （部署路徑: make rebuild-all 已含 build-frontend，會先重建 dist）"
  echo "   修復: make build-frontend"
fi
exit "$rc"
