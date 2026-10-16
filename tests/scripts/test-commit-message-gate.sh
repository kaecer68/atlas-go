#!/usr/bin/env bash
# Contract test for scripts/ci/check_commit_messages.sh — the semantic
# commit-message gate used by .github/workflows/ci-cd.yml (PR + push) and by
# `make ci` / `make ci-quick` / `.githooks/pre-push` (via `make ci-gate`).
#
# Regression for the "gate lies" class (FU-20261016-27): the gate used to run
#   done < <(git log --no-merges --format="%H %s" "$BASE..$HEAD" 2>/dev/null || true)
# so a range git could not resolve produced the SAME empty list as a range with
# no commits, and the script printed "✅ All commit messages follow semantic
# format" with rc=0. That was live in production: on push events ci-cd.yml
# passes `origin/${{ github.base_ref }}`, base_ref is empty there, so every push
# run logged "Checking commit messages from origin/..HEAD" + ✅.
#
# Scenarios:
#   1. every message valid                  → rc=0, prints ✅
#   2. one invalid message                  → rc=1, names the commit,
#                                             MUST NOT print ✅
#   3. range git cannot resolve             → rc=2 (fail loud), names the range,
#                                             MUST NOT print ✅  ← the defect
#   4. resolvable but EMPTY range (HEAD)    → rc=0 (a legitimate vacuous pass)
#   5. seam: git itself failing (any cause) → rc=2, MUST NOT print ✅
#   6. mutation self-proof: the OLD pipeline shape on the SAME fixture
#      reproduces the false green (rc=0 + ✅) ⇒ scenario 3 has discriminating
#      power (a probe that cannot tell the old code from the new code proves
#      nothing)
#   7. static wiring: no `2>/dev/null || true` on the git-log line; ci-cd.yml
#      selects a range that resolves on push events
set -euo pipefail

# git hooks export GIT_DIR & friends; a throwaway fixture repo would then
# resolve its git commands to the CALLER's repository (issue #1927). Drop the
# repo-selecting environment so the fixture stays hermetic.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SCRIPT="$ROOT/scripts/ci/check_commit_messages.sh"
WORKFLOW="$ROOT/.github/workflows/ci-cd.yml"

fail() { echo "FAIL: $*" >&2; exit 1; }

# fixture git with an identity that never depends on the developer config.
fixture_git() {
  local repo=$1
  shift
  git -C "$repo" -c commit.gpgsign=false -c user.email=test@atlas.local \
    -c user.name=test "$@"
}

# git_repo_with_messages MSG... → prints the fixture path (3 commits: seed,
# valid semantic commit, plus one commit per extra MSG).
new_fixture() {
  local tmp tmp_real fixture_root
  tmp=$(mktemp -d "${TMPDIR:-/tmp}/atlas-commit-msg-fixture.XXXXXX")
  tmp_real=$(cd "$tmp" && pwd -P)
  git init -q -b main "$tmp"
  fixture_root=$(git -C "$tmp" rev-parse --show-toplevel 2>/dev/null || echo "")
  [ "$fixture_root" = "$tmp_real" ] || \
    fail "fixture escaped its own directory (root='$fixture_root' tmp_real='$tmp_real')"
  mkdir -p "$tmp/scripts/ci"
  cp "$SCRIPT" "$tmp/scripts/ci/check_commit_messages.sh"
  printf 'seed\n' >"$tmp/seed.txt"
  fixture_git "$tmp" add seed.txt
  fixture_git "$tmp" commit -qm "chore(ci): seed"
  # Work happens on a branch: `main..HEAD` must be a real range (commit-gate
  # callers always check a range, never "the top commit of the checked-out
  # branch" — committing straight on main would make every range empty).
  fixture_git "$tmp" checkout -qb work
  echo "$tmp"
}

commit_msg() {
  local repo=$1 msg=$2 file
  file="$repo/$(echo "$msg" | tr -cs 'a-zA-Z0-9' '_')"
  printf '%s\n' "$msg" >"$file"
  fixture_git "$repo" add -A
  fixture_git "$repo" commit -qm "$msg"
}

run_gate() {  # run_gate <repo> [args...] → sets OUT / RC
  local repo=$1
  shift
  RC=0
  OUT=$(cd "$repo" && bash scripts/ci/check_commit_messages.sh "$@" 2>&1) || RC=$?
}

expect_not_pass() {  # expect_not_pass <scenario> <rc>
  printf '%s' "$OUT" | grep -qF "All commit messages follow semantic format" && \
    fail "$1: the gate claimed the messages are fine (rc=$2): $OUT"
  return 0
}

scenario1_valid_messages_pass() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_msg "$tmp" "fix(ci): a real message"
  commit_msg "$tmp" "feat(scope/sub): another one"
  run_gate "$tmp" "main" "HEAD"
  [ "$RC" -eq 0 ] || fail "scenario1: valid messages must exit 0, got $RC: $OUT"
  printf '%s' "$OUT" | grep -qF "All commit messages follow semantic format" || \
    fail "scenario1: expected the pass banner, got: $OUT"
  echo "  ✓ scenario1: valid messages pass (rc=0)"
}

scenario2_invalid_message_fails() {
  local tmp bad_hash; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_msg "$tmp" "fix(ci): a real message"
  commit_msg "$tmp" "this is not semantic"
  bad_hash=$(git -C "$tmp" rev-parse HEAD)
  run_gate "$tmp" "main" "HEAD"
  [ "$RC" -eq 1 ] || fail "scenario2: an invalid message must exit 1, got $RC: $OUT"
  printf '%s' "$OUT" | grep -qF "INVALID: $bad_hash" || \
    fail "scenario2: failure must name the offending commit, got: $OUT"
  expect_not_pass scenario2 "$RC"
  echo "  ✓ scenario2: invalid message fails (rc=1, names the commit)"
}

scenario3_unresolvable_range_fails_loud() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_msg "$tmp" "fix(ci): a real message"
  # Precondition: `origin/does-not-exist` must NOT resolve, otherwise this
  # scenario silently degrades into the "valid range" case.
  git -C "$tmp" rev-parse --verify -q origin/does-not-exist >/dev/null 2>&1 && \
    fail "scenario3: fixture unexpectedly resolves origin/does-not-exist"
  run_gate "$tmp" "origin/does-not-exist" "HEAD"
  [ "$RC" -eq 2 ] || fail "scenario3: unresolvable range must exit 2 (fail loud), got $RC: $OUT"
  printf '%s' "$OUT" | grep -qF "origin/does-not-exist..HEAD" || \
    fail "scenario3: failure must name the unusable range, got: $OUT"
  expect_not_pass scenario3 "$RC"
  echo "  ✓ scenario3: unresolvable range fails loud (rc=2, names the range)"
}

scenario4_empty_range_is_a_legitimate_pass() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  # HEAD..HEAD resolves and genuinely contains no commit: the fix must not turn
  # the legitimate vacuous pass into a red (a blanket red pushes operators to
  # --no-verify).
  run_gate "$tmp" "HEAD" "HEAD"
  [ "$RC" -eq 0 ] || fail "scenario4: resolvable empty range must exit 0, got $RC: $OUT"
  echo "  ✓ scenario4: resolvable empty range still passes (rc=0)"
}

scenario5_git_failure_is_not_a_pass() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_msg "$tmp" "fix(ci): a real message"
  # Seam: the RANGE is valid here — git itself fails (hook-exported GIT_DIR,
  # broken repo, no git binary: all the same code path).
  local fake="$tmp/fakebin"
  mkdir -p "$fake"
  printf '#!/usr/bin/env bash\necho "fatal: injected git failure (test seam)" >&2\nexit 128\n' >"$fake/git"
  chmod +x "$fake/git"
  RC=0
  OUT=$(cd "$tmp" && PATH="$fake:$PATH" bash scripts/ci/check_commit_messages.sh "main" "HEAD" 2>&1) || RC=$?
  [ "$RC" -eq 2 ] || fail "scenario5: git failure must exit 2, got $RC: $OUT"
  printf '%s' "$OUT" | grep -qF "128" || fail "scenario5: failure should report git's status, got: $OUT"
  expect_not_pass scenario5 "$RC"
  echo "  ✓ scenario5: injected git failure fails loud (rc=2, reports git rc)"
}

scenario6_old_shape_reproduces_the_false_green() {
  # Mutation self-proof: the exact pipeline shape from origin/main, verbatim, on
  # the SAME fixture as scenario3. If this does not print ✅/rc=0, then
  # scenario3's assertion would not distinguish old from new code at all.
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_msg "$tmp" "fix(ci): a real message"
  cat >"$tmp/old-shape.sh" <<'OLD'
#!/usr/bin/env bash
set -euo pipefail
BASE="${1:-HEAD~1}"
HEAD="${2:-HEAD}"
HAS_ERRORS=false
while IFS= read -r commit; do
  msg=$(echo "$commit" | cut -d' ' -f2-)
  if ! echo "$msg" | grep -qE '^(feat|fix)(\([a-z0-9]*\))?: .+'; then
    HAS_ERRORS=true
  fi
done < <(git log --no-merges --format="%H %s" "$BASE..$HEAD" 2>/dev/null || true)
if $HAS_ERRORS; then exit 1; fi
echo "✅ All commit messages follow semantic format"
OLD
  local rc=0 out
  out=$(cd "$tmp" && bash old-shape.sh "origin/does-not-exist" "HEAD" 2>&1) || rc=$?
  [ "$rc" -eq 0 ] || fail "scenario6: the old shape must reproduce the false green (rc=0), got $rc: $out"
  printf '%s' "$out" | grep -qF "All commit messages follow semantic format" || \
    fail "scenario6: the old shape must print the pass banner, got: $out"
  # …and the NEW script differs on the identical input.
  run_gate "$tmp" "origin/does-not-exist" "HEAD"
  [ "$RC" -eq 2 ] || fail "scenario6: new script must differ from the old shape, got $RC: $OUT"
  echo "  ✓ scenario6: old shape ⇒ rc=0 + ✅, new script ⇒ rc=2 (probe has teeth)"
}

scenario7_static_wiring() {
  # The fail-open shape must not come back, and the push-side caller must pass a
  # range that resolves.
  grep -Fq 'git log --no-merges --format="%H %s" "$BASE..$HEAD" 2>/dev/null || true' "$SCRIPT" && \
    fail "scenario7: check_commit_messages.sh still contains the fail-open git-log pipeline"
  grep -Fq 'exit 2' "$SCRIPT" || fail "scenario7: check_commit_messages.sh lost the rc=2 fail-loud path"
  grep -Fq 'BASE_REF: ${{ github.base_ref }}' "$WORKFLOW" || \
    fail "scenario7: ci-cd.yml no longer selects a push-safe commit range (FU-20261016-27)"
  # Makefile's ci-quick loop lists the bare path (no `bash` prefix); the glob
  # form `scripts/ci/check_*.sh` (make ci) is covered by the same string.
  grep -Fq "scripts/ci/check_commit_messages.sh" "$ROOT/Makefile" || \
    fail "scenario7: Makefile ci-quick no longer calls the commit-message gate"
  echo "  ✓ scenario7: static wiring (no fail-open pipeline, push-safe CI range)"
}

echo "=== test-commit-message-gate.sh ==="
scenario1_valid_messages_pass
scenario2_invalid_message_fails
scenario3_unresolvable_range_fails_loud
scenario4_empty_range_is_a_legitimate_pass
scenario5_git_failure_is_not_a_pass
scenario6_old_shape_reproduces_the_false_green
scenario7_static_wiring
echo "✅ test-commit-message-gate.sh PASSED (7/7)"
