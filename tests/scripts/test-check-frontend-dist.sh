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
#   7. go:generate outputs (cmd/gentags) newer than dist → still exit 0
#   8. dist holding only embed-dirs' `.keep` placeholder → exit 1 (never built)
#   9. files outside the esbuild source trees (tests/, smoke/) → not stale
#  10. stale failure names the offending source file + the newest dist file
#  11. --diff with a range git CANNOT resolve → exit 1 (never the
#      "nothing to check" skip), naming the range
#  12. seam: git itself failing (any cause, not just a bad range) → exit 1
#  13. a VALID range whose diff is genuinely empty → still skip (exit 0):
#      the fix must not turn the legitimate skip into a blanket red
set -euo pipefail

# git hooks (pre-push/pre-commit) export GIT_DIR, and any throwaway fixture repo
# created below would then resolve its git commands to the CALLER's repository —
# on 2026-09-23 that rewrote the caller's branch with fixture commits and set
# core.bare=true in the caller's .git/config (issue #1927). Unset every GIT_*
# override so the fixture stays hermetic even when this script runs from a hook.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SCRIPT="$ROOT/scripts/ci/check_frontend_dist.sh"

fail() { echo "FAIL: $*" >&2; exit 1; }

# Fresh git repo + the script copied in (so REPO_ROOT resolves inside fixture).
new_fixture() {
  local tmp
  # Explicit template + own-repo assertion (2026-09-23 incident): a bare
  # `mktemp -d` fixture once had its git commands land in the CALLER's worktree
  # and rewrote the checked-out branch with fixture commits. The fixture must
  # therefore (a) be created under an explicit path, and (b) prove it is its own
  # repository root before any git command runs against it.
  tmp=$(mktemp -d "${TMPDIR:-/tmp}/atlas-frontend-dist-fixture.XXXXXX")
  # pwd -P normalises macOS /var -> /private/var so the comparison below cannot
  # fail (or silently pass) on symlinked temp roots.
  tmp_real=$(cd "$tmp" && pwd -P)
  git init -q "$tmp"
  fixture_root=$(git -C "$tmp" rev-parse --show-toplevel 2>/dev/null || echo "")
  [ "$fixture_root" = "$tmp_real" ] || {
    echo "FAIL: fixture escaped its own directory (root='$fixture_root' tmp_real='$tmp_real')" >&2
    exit 1
  }
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
  # 精確斷言（issue #2011）：「擋下」= exit 1。exit 2（用法錯誤）/126/127（腳本不存在）
  # 都不是「擋下了」—— 舊寫法 `if ...; then fail; fi` 會把它們當成成功。
  local rc=0
  bash "$tmp/scripts/ci/check_frontend_dist.sh" admin_web >/dev/null 2>&1 || rc=$?
  if [ "$rc" -ne 1 ]; then fail "scenario2: stale dist should exit 1, got $rc"; fi
  echo "  ✓ scenario2: stale dist fails (exit=1)"
}

scenario3_missing_fails() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  make_frontend "$tmp" admin_web missing
  local rc=0
  bash "$tmp/scripts/ci/check_frontend_dist.sh" admin_web >/dev/null 2>&1 || rc=$?
  if [ "$rc" -ne 1 ]; then fail "scenario3: missing dist should exit 1, got $rc"; fi
  echo "  ✓ scenario3: missing dist fails (exit=1)"
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
  local rc=0
  bash "$tmp/scripts/ci/check_frontend_dist.sh" --diff main~1...HEAD >/dev/null 2>&1 || rc=$?
  if [ "$rc" -ne 1 ]; then fail "scenario5: frontend diff + stale dist should exit 1, got $rc"; fi
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
  local out rc=0
  out=$(bash "$tmp/scripts/ci/check_frontend_dist.sh" --diff main~1...HEAD 2>&1) || rc=$?
  [ "$rc" -eq 1 ] || fail "scenario6: shared_web change with stale dists should exit 1, got $rc"
  echo "$out" | grep -q "admin_web: dist 過期" || fail "scenario6: expected admin_web stale, got: $out"
  echo "$out" | grep -q "client_web: dist 過期" || fail "scenario6: expected client_web stale, got: $out"
  echo "  ✓ scenario6: shared_web change checks both frontends"
}

scenario7_generated_files_ignored() {
  # 模擬 pre-push 真實順序: ci-gate 先跑 go generate (無條件重寫
  # static/js/shared/field_types.ts + valid_fields.json, 內容相同也 bump
  # mtime), 之後才跑 dist check。生成檔 mtime 比 dist 新 → 必須不誤判為
  # dist 過期 (內容 drift 由 ci-gate 的 git diff --exit-code 把關)。
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  make_frontend "$tmp" admin_web fresh
  mkdir -p "$tmp/admin_web/static/js/shared"
  echo '// auto-generated by cmd/gentags' >"$tmp/admin_web/static/js/shared/field_types.ts"
  echo '[]' >"$tmp/admin_web/static/js/shared/valid_fields.json"
  touch -t 203501010000 "$tmp/admin_web/static/js/shared/field_types.ts" "$tmp/admin_web/static/js/shared/valid_fields.json"
  if ! bash "$tmp/scripts/ci/check_frontend_dist.sh" admin_web >/dev/null 2>&1; then
    fail "scenario7: go-generated files (mtime newer than dist) must not fail dist check"
  fi
  echo "  ✓ scenario7: go-generated shared files ignored by freshness check"
}

scenario8_placeholder_only_dist_fails() {
  # `make embed-dirs` (Makefile) creates <dir>/dist/.keep in a fresh worktree so
  # `//go:embed all:dist` finds a directory. That placeholder is NOT a build:
  # a dist holding only `.keep` must fail exactly like a missing dist, otherwise
  # a fresh worktree would report "dist 最新" with no UI embedded.
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  make_frontend "$tmp" admin_web missing
  touch -t 203001010000 "$tmp/admin_web/dist/.keep"
  local rc=0
  bash "$tmp/scripts/ci/check_frontend_dist.sh" admin_web >/dev/null 2>&1 || rc=$?
  if [ "$rc" -ne 1 ]; then fail "scenario8: placeholder-only dist should exit 1, got $rc"; fi
  echo "  ✓ scenario8: dist holding only embed-dirs .keep fails (exit=1)"
}

scenario9_non_build_inputs_ignored() {
  # Only the trees esbuild reads count (<dir>/static + shared_web/static). Editing
  # <dir>/tests or <dir>/smoke must NOT report dist as stale — the previous
  # whole-directory scan did, which made every frontend test-only push rebuild.
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  make_frontend "$tmp" admin_web fresh
  mkdir -p "$tmp/admin_web/tests" "$tmp/admin_web/smoke"
  echo '// test' >"$tmp/admin_web/tests/dashboard.spec.ts"
  echo '// smoke' >"$tmp/admin_web/smoke/run.mjs"
  touch -t 203601010000 "$tmp/admin_web/tests/dashboard.spec.ts" "$tmp/admin_web/smoke/run.mjs"
  local rc=0
  bash "$tmp/scripts/ci/check_frontend_dist.sh" admin_web >/dev/null 2>&1 || rc=$?
  if [ "$rc" -ne 0 ]; then fail "scenario9: non-build-input files must not fail dist check, got $rc"; fi
  echo "  ✓ scenario9: files outside the esbuild source trees do not count as stale"
}

scenario10_stale_report_names_the_file() {
  # The failure must name the offending source file (the operator should not have
  # to guess which file outran dist), including shared_web files.
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  make_frontend "$tmp" admin_web fresh
  mkdir -p "$tmp/shared_web/static/js"
  echo '// shared' >"$tmp/shared_web/static/js/shared-new.js"
  touch -t 203601010000 "$tmp/shared_web/static/js/shared-new.js"
  local out rc=0
  out=$(bash "$tmp/scripts/ci/check_frontend_dist.sh" admin_web 2>&1) || rc=$?
  [ "$rc" -eq 1 ] || fail "scenario10: shared_web source newer than dist should exit 1, got $rc"
  printf '%s\n' "$out" | grep -q "shared_web/static/js/shared-new.js" || \
    fail "scenario10: failure must name the stale source file, got: $out"
  printf '%s\n' "$out" | grep -q "admin_web/dist/js/main.js" || \
    fail "scenario10: failure must name the newest dist file, got: $out"
  echo "  ✓ scenario10: failure names the source file and the dist file"
}

# ── 2026-10-16: "git diff failed" must never be reported as "no frontend change"
#
# Both outcomes produce an empty file list, so `changed=$(git diff … || true)`
# (with 2>/dev/null on top) made them identical and the gate returned 0 — i.e. it
# said "nothing to check" for a push whose dist was provably stale. Scenarios 11
# and 12 pin the fail-loud side; scenario 13 pins the other side (a genuinely
# empty diff still skips), so the fix cannot degrade into "always red".

scenario11_unresolvable_range_fails_loud() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  make_frontend "$tmp" admin_web stale
  echo '// src2' >"$tmp/admin_web/static/js/app.js"
  touch -t 202501020000 "$tmp/admin_web/static/js/app.js"
  git -C "$tmp" add -A && git -C "$tmp" commit -qm "feat(web): admin change on a stale dist"
  # Precondition of this scenario: the range below MUST be unresolvable, otherwise
  # the test would silently degrade into "scenario5 passes again".
  if git -C "$tmp" rev-parse --verify -q origin/main >/dev/null; then
    fail "scenario11: fixture unexpectedly resolves origin/main — range is not unresolvable"
  fi
  # Control: the same tree with a resolvable range is red (the dist really is stale).
  local ctl=0
  bash "$tmp/scripts/ci/check_frontend_dist.sh" --diff main~1...HEAD >/dev/null 2>&1 || ctl=$?
  [ "$ctl" -eq 1 ] || fail "scenario11: control range should exit 1 (stale dist), got $ctl"
  local out rc=0
  out=$(bash "$tmp/scripts/ci/check_frontend_dist.sh" --diff origin/main...HEAD 2>&1) || rc=$?
  [ "$rc" -eq 1 ] || fail "scenario11: unresolvable range must exit 1, got $rc: $out"
  printf '%s\n' "$out" | grep -q "nothing to check" && \
    fail "scenario11: unresolvable range was reported as 'nothing to check': $out"
  printf '%s\n' "$out" | grep -qF "origin/main...HEAD" || \
    fail "scenario11: failure must name the unusable range, got: $out"
  echo "  ✓ scenario11: unresolvable --diff range fails loud (exit=1, names the range)"
}

scenario12_git_failure_is_not_no_change() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  make_frontend "$tmp" admin_web stale
  echo '// src2' >"$tmp/admin_web/static/js/app.js"
  touch -t 202501020000 "$tmp/admin_web/static/js/app.js"
  git -C "$tmp" add -A && git -C "$tmp" commit -qm "feat(web): admin change on a stale dist"
  # Seam: the RANGE is valid here — git itself fails (no git binary, not a work
  # tree, GIT_DIR pointing elsewhere: #1927's class). Only `git` is shadowed.
  local fake="$tmp/fakebin"
  mkdir -p "$fake"
  printf '#!/usr/bin/env bash\necho "fatal: injected git failure (test seam)" >&2\nexit 128\n' >"$fake/git"
  chmod +x "$fake/git"
  local out rc=0
  out=$(PATH="$fake:$PATH" bash "$tmp/scripts/ci/check_frontend_dist.sh" --diff main~1...HEAD 2>&1) || rc=$?
  [ "$rc" -eq 1 ] || fail "scenario12: git failure must exit 1 (not git's 128, not 0), got $rc: $out"
  printf '%s\n' "$out" | grep -q "nothing to check" && \
    fail "scenario12: git failure was reported as 'nothing to check': $out"
  printf '%s\n' "$out" | grep -q "128" || \
    fail "scenario12: failure should report git's status, got: $out"
  echo "  ✓ scenario12: injected git failure fails loud (exit=1, reports git rc)"
}

scenario13_valid_empty_range_still_skips() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  make_frontend "$tmp" admin_web stale
  git -C "$tmp" add -A && git -C "$tmp" commit -qm "chore: fixture"
  # `--diff HEAD` resolves and its diff is genuinely empty: the legitimate skip
  # must stay green (a blanket red would push operators to --no-verify).
  local out rc=0
  out=$(bash "$tmp/scripts/ci/check_frontend_dist.sh" --diff HEAD 2>&1) || rc=$?
  [ "$rc" -eq 0 ] || fail "scenario13: resolvable empty diff must skip (exit 0), got $rc: $out"
  printf '%s\n' "$out" | grep -q "nothing to check" || \
    fail "scenario13: expected the skip message, got: $out"
  echo "  ✓ scenario13: resolvable but empty diff still skips (exit=0)"
}

echo "=== test-check-frontend-dist.sh ==="
scenario1_fresh_passes
scenario2_stale_fails
scenario3_missing_fails
scenario4_pure_go_diff_skips
scenario5_frontend_diff_checks
scenario6_shared_web_change_checks_both
scenario7_generated_files_ignored
scenario8_placeholder_only_dist_fails
scenario9_non_build_inputs_ignored
scenario10_stale_report_names_the_file
scenario11_unresolvable_range_fails_loud
scenario12_git_failure_is_not_no_change
scenario13_valid_empty_range_still_skips
echo "✅ test-check-frontend-dist.sh PASSED (13/13)"
