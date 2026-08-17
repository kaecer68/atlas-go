#!/usr/bin/env bash
# Test scripts/ci/check_frontend_dist.sh — frontend dist freshness gate.
#
# Regression for the "pure-Go push gets dragged into frontend build checks"
# friction: the pre-push hook must only require a fresh dist when the diff
# touches admin_web/client_web/shared_web; pure Go/docs changes skip entirely.
#
# Scenarios:
#   1. fresh dist  → exit 0
#   2. stale dist (source newer than dist) → exit 1
#   3. missing dist → exit 1
#   4. --diff with only Go files → skip (exit 0)
#   5. --diff with frontend file + stale dist → exit 1; rebuilt → exit 0
#   6. --diff with only shared_web change → checks both frontends (exit 1 if stale)
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SCRIPT="$ROOT/scripts/ci/check_frontend_dist.sh"

fail() { echo "FAIL: $*" >&2; exit 1; }

# Fresh git repo + the script copied in (so REPO_ROOT resolves inside fixture).
new_fixture() {
  local tmp
  tmp=$(mktemp -d)
  git init -q "$tmp"
  git -C "$tmp" config user.email test@atlas.local
  git -C "$tmp" config user.name test
  git -C "$tmp" commit --allow-empty -qm base
  git -C "$tmp" branch -m main
  mkdir -p "$tmp/scripts/ci"
  cp "$SCRIPT" "$tmp/scripts/ci/check_frontend_dist.sh"
  echo "$tmp"
}

# make_frontend DIR NAME FRESHNESS(fresh|stale|missing)
make_frontend() {
  local dir=$1 name=$2 freshness=$3
  local fe="$dir/$name"
  mkdir -p "$fe/static/js"
  printf '{"name":"%s"}\n' "$name" >"$fe/package.json"
  case "$freshness" in
    missing)
      mkdir -p "$fe/dist"   # dir may exist but must be empty to count as missing
      ;;
    fresh|stale)
      mkdir -p "$fe/dist/js"
      echo "// built" >"$fe/dist/js/main.js"
      echo "// source" >"$fe/static/js/main.js"
      # backdate everything non-dist (incl. package.json) → older than dist
      find "$fe" -type f -not -path "$fe/dist/*" -exec touch -t 202001010000 {} +
      touch -t 202001020000 "$fe/dist/js/main.js"
      if [ "$freshness" = "stale" ]; then
        touch -t 202501010000 "$fe/static/js/main.js"  # newer than dist
      fi
      ;;
  esac
}

scenario1_fresh_passes() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  make_frontend "$tmp" admin_web fresh
  make_frontend "$tmp" client_web fresh
  bash "$tmp/scripts/ci/check_frontend_dist.sh" >/dev/null 2>&1 || fail "scenario1: fresh dist should exit 0"
  echo "  ✓ scenario1: fresh dist passes"
}

scenario2_stale_fails() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  make_frontend "$tmp" admin_web stale
  if bash "$tmp/scripts/ci/check_frontend_dist.sh" admin_web >/dev/null 2>&1; then
    fail "scenario2: stale dist should exit 1"
  fi
  echo "  ✓ scenario2: stale dist fails"
}

scenario3_missing_fails() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  make_frontend "$tmp" admin_web missing
  if bash "$tmp/scripts/ci/check_frontend_dist.sh" admin_web >/dev/null 2>&1; then
    fail "scenario3: missing dist should exit 1"
  fi
  echo "  ✓ scenario3: missing dist fails"
}

scenario4_pure_go_diff_skips() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  mkdir -p "$tmp/internal/foo"
  echo 'package foo' >"$tmp/internal/foo/foo.go"
  git -C "$tmp" add -A && git -C "$tmp" commit -qm "feat(go): pure go change"
  local out
  out=$(bash "$tmp/scripts/ci/check_frontend_dist.sh" --diff main~1...HEAD 2>&1) || \
    fail "scenario4: pure-Go diff must skip (exit 0), got: $out"
  echo "$out" | grep -qi "未含\|nothing to check" || fail "scenario4: expected skip message, got: $out"
  echo "  ✓ scenario4: pure Go diff skips dist check"
}

scenario5_frontend_diff_checks() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  make_frontend "$tmp" admin_web stale
  git -C "$tmp" add -A && git -C "$tmp" commit -qm "feat(web): admin change (stale dist)"
  if bash "$tmp/scripts/ci/check_frontend_dist.sh" --diff main~1...HEAD >/dev/null 2>&1; then
    fail "scenario5: frontend diff + stale dist should exit 1"
  fi
  # rebuild → fresh
  touch -t 203001010000 "$tmp/admin_web/dist/js/main.js"
  bash "$tmp/scripts/ci/check_frontend_dist.sh" --diff main~1...HEAD >/dev/null 2>&1 || \
    fail "scenario5: frontend diff + fresh dist should exit 0"
  echo "  ✓ scenario5: frontend diff triggers dist check (stale fails, fresh passes)"
}

scenario6_shared_web_change_checks_both() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  make_frontend "$tmp" admin_web stale
  make_frontend "$tmp" client_web stale
  mkdir -p "$tmp/shared_web/static/js/pages"
  echo '// page' >"$tmp/shared_web/static/js/pages/demo.js"
  git -C "$tmp" add -A && git -C "$tmp" commit -qm "feat(web): shared page"
  local out
  out=$(bash "$tmp/scripts/ci/check_frontend_dist.sh" --diff main~1...HEAD 2>&1) && \
    fail "scenario6: shared_web change with stale dists should exit 1"
  echo "$out" | grep -q "admin_web: dist 過期" || fail "scenario6: expected admin_web stale, got: $out"
  echo "$out" | grep -q "client_web: dist 過期" || fail "scenario6: expected client_web stale, got: $out"
  echo "  ✓ scenario6: shared_web change checks both frontends"
}

echo "=== test-check-frontend-dist.sh ==="
scenario1_fresh_passes
scenario2_stale_fails
scenario3_missing_fails
scenario4_pure_go_diff_skips
scenario5_frontend_diff_checks
scenario6_shared_web_change_checks_both
echo "✅ test-check-frontend-dist.sh PASSED (6/6)"
