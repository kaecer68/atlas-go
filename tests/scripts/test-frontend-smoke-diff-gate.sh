#!/usr/bin/env bash
# Contract test for scripts/ci/frontend_smoke.sh — the Playwright smoke gate.
#
# Regression for the "gate lies" class (FU-20261016-27):
#   CHANGED=$(git diff --name-only "$BASE_REF"...HEAD 2>/dev/null || true)
# A diff git could NOT resolve and a diff that genuinely touched no frontend
# file both produce an empty list, so the gate printed
#   "No diff (empty commit or base==HEAD)" + "→ SKIP" + "Smoke gate skipped"
# and exited 0 — for a tree whose client_web files had provably changed. The
# second half of the same item: the header has always declared
# "3 = diff 解析失敗" and nothing in the file ever exited 3.
#
# Scenarios:
#   1. resolvable diff, no frontend file → rc=0 + SMOKE_RESULT=SKIP (legit skip)
#   2. resolvable EMPTY diff (HEAD)      → rc=0 + SMOKE_RESULT=SKIP
#   3. unresolvable range (no merge base)→ rc=3, names the range, no SKIP  ← defect
#   4. seam: git itself failing          → rc=3, no SKIP
#   5. base ref absent → documented full-smoke fallback (NOT the rc=3 path)
#   6. mutation self-proof: the OLD shape on the SAME fixture reproduces
#      rc=0 + SKIP ⇒ scenario 3 has discriminating power
#   7. static wiring: exit 3 implemented + documented, no fail-open pipeline,
#      SKIP still wired, quality.yml still calls the gate
#
# Everything here stays on the pre-build path (diff parsing / decision), so no
# frontend build, no atlas binary, no Playwright, no network.
set -euo pipefail

unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SCRIPT="$ROOT/scripts/ci/frontend_smoke.sh"
WORKFLOW="$ROOT/.github/workflows/quality.yml"
REAL_GIT=$(command -v git)

fail() { echo "FAIL: $*" >&2; exit 1; }

fixture_git() {
  local repo=$1
  shift
  git -C "$repo" -c commit.gpgsign=false -c user.email=test@atlas.local \
    -c user.name=test "$@"
}

# new_fixture → prints the fixture path. `main` holds the seed commit and is left
# behind; the checkout stays on `work`, so `main...HEAD` is a real range.
new_fixture() {
  local tmp tmp_real fixture_root
  tmp=$(mktemp -d "${TMPDIR:-/tmp}/atlas-smoke-fixture.XXXXXX")
  tmp_real=$(cd "$tmp" && pwd -P)
  git init -q -b main "$tmp"
  fixture_root=$(git -C "$tmp" rev-parse --show-toplevel 2>/dev/null || echo "")
  [ "$fixture_root" = "$tmp_real" ] || \
    fail "fixture escaped its own directory (root='$fixture_root' tmp_real='$tmp_real')"
  mkdir -p "$tmp/scripts/ci" "$tmp/docs"
  cp "$SCRIPT" "$tmp/scripts/ci/frontend_smoke.sh"
  printf 'seed\n' >"$tmp/docs/seed.md"
  fixture_git "$tmp" add -A
  fixture_git "$tmp" commit -qm "chore(ci): seed"
  fixture_git "$tmp" checkout -qb work
  echo "$tmp"
}

# orphan_fixture → a repo whose `work` branch shares NO commit with `main`
# (`git checkout --orphan`): `main` resolves, but `main...HEAD` has no merge
# base. That is the same code path a shallow/parentless history reaches, and it
# is what the old `2>/dev/null || true` silently turned into "no diff".
new_orphan_fixture() {
  local tmp tmp_real fixture_root
  tmp=$(mktemp -d "${TMPDIR:-/tmp}/atlas-smoke-orphan.XXXXXX")
  tmp_real=$(cd "$tmp" && pwd -P)
  git init -q -b main "$tmp"
  fixture_root=$(git -C "$tmp" rev-parse --show-toplevel 2>/dev/null || echo "")
  [ "$fixture_root" = "$tmp_real" ] || \
    fail "orphan fixture escaped its own directory (root='$fixture_root' tmp_real='$tmp_real')"
  mkdir -p "$tmp/scripts/ci" "$tmp/docs"
  cp "$SCRIPT" "$tmp/scripts/ci/frontend_smoke.sh"
  printf 'seed\n' >"$tmp/docs/seed.md"
  fixture_git "$tmp" add -A
  # -q does NOT silence the "nothing to commit" path, so every commit below has
  # real content — an empty commit would print to stdout and corrupt `$( )`.
  fixture_git "$tmp" commit -qm "chore(ci): seed"
  fixture_git "$tmp" checkout -q --orphan work
  fixture_git "$tmp" rm -rq --cached . >/dev/null 2>&1 || true
  rm -rf "$tmp/docs"
  mkdir -p "$tmp/client_web"
  printf '// client page\n' >"$tmp/client_web/page.js"
  fixture_git "$tmp" add -A
  fixture_git "$tmp" commit -qm "feat(web): client page"
  echo "$tmp"
}

run_smoke() {  # run_smoke <repo> [args...] → sets OUT / RC
  local repo=$1
  shift
  RC=0
  OUT=$(cd "$repo" && bash scripts/ci/frontend_smoke.sh "$@" 2>&1) || RC=$?
}

expect_no_skip() {
  printf '%s' "$OUT" | grep -qF "SMOKE_RESULT=SKIP" && \
    fail "$1: the gate reported the diff as 'nothing relevant' (rc=$2): $OUT"
  return 0
}

scenario1_no_frontend_change_skips() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  printf 'more\n' >"$tmp/docs/readme.md"
  fixture_git "$tmp" add -A
  fixture_git "$tmp" commit -qm "docs(ci): readme only"
  run_smoke "$tmp" "main"
  [ "$RC" -eq 0 ] || fail "scenario1: docs-only diff must exit 0 (skip), got $RC: $OUT"
  printf '%s' "$OUT" | grep -qF "SMOKE_RESULT=SKIP" || \
    fail "scenario1: the skip path must stay machine-readable (SMOKE_RESULT=SKIP), got: $OUT"
  echo "  ✓ scenario1: docs-only diff skips (rc=0, SMOKE_RESULT=SKIP)"
}

scenario2_empty_diff_skips() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  run_smoke "$tmp" "HEAD"
  [ "$RC" -eq 0 ] || fail "scenario2: resolvable empty diff must exit 0, got $RC: $OUT"
  printf '%s' "$OUT" | grep -qF "SMOKE_RESULT=SKIP" || \
    fail "scenario2: expected the skip marker, got: $OUT"
  echo "  ✓ scenario2: resolvable empty diff still skips (rc=0)"
}

scenario3_unresolvable_range_fails_loud() {
  local tmp; tmp=$(new_orphan_fixture); trap 'rm -rf "$tmp"' RETURN
  # Precondition of this scenario: `main` MUST resolve while `main...HEAD` must
  # NOT — otherwise it silently degrades into scenario 1/2.
  git -C "$tmp" rev-parse --verify -q main >/dev/null || \
    fail "scenario3: fixture must resolve main"
  git -C "$tmp" diff --name-only "main...HEAD" >/dev/null 2>&1 && \
    fail "scenario3: fixture unexpectedly resolves main...HEAD (no-merge-base case lost)"
  run_smoke "$tmp" "main"
  [ "$RC" -eq 3 ] || fail "scenario3: unresolvable range must exit 3, got $RC: $OUT"
  printf '%s' "$OUT" | grep -qF "main...HEAD" || \
    fail "scenario3: failure must name the unusable range, got: $OUT"
  expect_no_skip scenario3 "$RC"
  echo "  ✓ scenario3: unresolvable range fails loud (rc=3, names the range)"
}

scenario4_git_failure_is_not_a_skip() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  printf '// client page\n' >"$tmp/client_web_page.js"
  mkdir -p "$tmp/client_web"
  mv "$tmp/client_web_page.js" "$tmp/client_web/page.js"
  fixture_git "$tmp" add -A
  fixture_git "$tmp" commit -qm "feat(web): client page"
  # Seam: the range is valid — only `git diff` fails (mirrors a broken GIT_DIR).
  local fake="$tmp/fakebin"
  mkdir -p "$fake"
  {
    printf '#!/usr/bin/env bash\n'
    printf 'case "$1 $2" in\n'
    printf '  "rev-parse --verify") exec "%s" "$@" ;;\n' "$REAL_GIT"
    printf 'esac\n'
    printf 'echo "fatal: injected git failure (test seam)" >&2\n'
    printf 'exit 128\n'
  } >"$fake/git"
  chmod +x "$fake/git"
  RC=0
  OUT=$(cd "$tmp" && PATH="$fake:$PATH" bash scripts/ci/frontend_smoke.sh "main" 2>&1) || RC=$?
  [ "$RC" -eq 3 ] || fail "scenario4: git failure must exit 3, got $RC: $OUT"
  printf '%s' "$OUT" | grep -qF "128" || \
    fail "scenario4: failure should report git's status, got: $OUT"
  expect_no_skip scenario4 "$RC"
  echo "  ✓ scenario4: injected git failure fails loud (rc=3, reports git rc)"
}

scenario5_missing_base_ref_is_the_documented_fallback() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  # Not a failure path: an unresolvable BASE REF escalates to the FULL smoke
  # (a superset of the checks), which is deliberately soft. It must stay
  # distinguishable from the rc=3 "could not parse the diff" state, otherwise
  # the two meanings collapse again.
  run_smoke "$tmp" "origin/does-not-exist"
  printf '%s' "$OUT" | grep -qF "falling back to full smoke" || \
    fail "scenario5: expected the full-smoke fallback message, got: $OUT"
  [ "$RC" -ne 3 ] || fail "scenario5: the conservative fallback must not use the rc=3 diff-parse code"
  expect_no_skip scenario5 "$RC"
  echo "  ✓ scenario5: missing base ref escalates to full smoke (rc≠3)"
}

scenario6_old_shape_reproduces_the_false_green() {
  # Mutation self-proof: the exact old pipeline shape, verbatim, on the SAME
  # orphan fixture as scenario3. If it does not print SKIP/rc=0 then scenario3
  # would not distinguish the old code from the new one.
  local tmp; tmp=$(new_orphan_fixture); trap 'rm -rf "$tmp"' RETURN
  cat >"$tmp/old-shape.sh" <<'OLD'
#!/usr/bin/env bash
set -euo pipefail
BASE_REF="${1:-origin/main}"
CHANGED=$(git diff --name-only "$BASE_REF"...HEAD 2>/dev/null || true)
if [[ -z "$CHANGED" ]]; then
  echo "No diff (empty commit or base==HEAD)"
fi
echo "No relevant frontend mapped from diff → SKIP"
echo "Smoke gate skipped (no relevant changes)"
OLD
  local rc=0 out
  out=$(cd "$tmp" && bash old-shape.sh "main" 2>&1) || rc=$?
  [ "$rc" -eq 0 ] || fail "scenario6: the old shape must reproduce the false green (rc=0), got $rc: $out"
  printf '%s' "$out" | grep -qF "Smoke gate skipped" || \
    fail "scenario6: the old shape must print the skip banner, got: $out"
  run_smoke "$tmp" "main"
  [ "$RC" -eq 3 ] || fail "scenario6: new script must differ from the old shape, got $RC: $OUT"
  echo "  ✓ scenario6: old shape ⇒ rc=0 + SKIP, new script ⇒ rc=3 (probe has teeth)"
}

scenario7_static_wiring() {
  grep -Fq 'git diff --name-only "$BASE_REF"...HEAD 2>/dev/null || true' "$SCRIPT" && \
    fail "scenario7: frontend_smoke.sh still contains the fail-open git-diff pipeline"
  grep -Fq "exit 3" "$SCRIPT" || \
    fail "scenario7: frontend_smoke.sh must implement the exit 3 its header declares (FU-20261016-27)"
  grep -Fq "3 = diff 解析失敗" "$SCRIPT" || \
    fail "scenario7: the exit-code block must still document 3 as a diff-parse failure"
  grep -Fq "SMOKE_RESULT=SKIP" "$SCRIPT" || grep -Fq "SMOKE_RESULT=PASS" "$SCRIPT" || \
    fail "scenario7: skip/pass must stay machine-readable (SMOKE_RESULT marker)"
  grep -Fq "bash scripts/ci/frontend_smoke.sh origin/" "$WORKFLOW" || \
    fail "scenario7: quality.yml no longer calls the frontend smoke gate"
  echo "  ✓ scenario7: static wiring (exit 3 implemented + documented, skip marker, CI call)"
}

echo "=== test-frontend-smoke-diff-gate.sh ==="
# The defect-pinning scenarios run FIRST: the negative proof runs this suite
# against the pre-fix script, and a report must then name the defect itself
# (rc must be 3, not 0) rather than an unrelated cosmetic difference.
scenario3_unresolvable_range_fails_loud
scenario4_git_failure_is_not_a_skip
scenario1_no_frontend_change_skips
scenario2_empty_diff_skips
scenario5_missing_base_ref_is_the_documented_fallback
scenario6_old_shape_reproduces_the_false_green
scenario7_static_wiring
echo "✅ test-frontend-smoke-diff-gate.sh PASSED (7/7)"
