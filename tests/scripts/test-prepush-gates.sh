#!/usr/bin/env bash
set -euo pipefail

# Contract test for .githooks/pre-push (hermetic: throwaway git repos, LOCAL bare
# remote, PATH-stubbed `make`, recording docker stub; no network, no docker
# daemon, no writes to the caller's checkout).
#
# Pins down two defects fixed 2026-09-26 in ONE file:
#   E4  — a failed `git fetch origin/main` used to `exit 0` (look-green,
#         checked-nothing: it skipped ci-full AND every diff-based gate).
#   FU-20260926-15 — no host binary freshness gate at all: source had moved past
#         bin/atlas-mcp and the push still went through.
# Plus the two properties the fix must NOT break: a docs-only push and a clone
# without bin/ (fresh/linked worktree) must never be blocked, and the push path
# must not require docker.
#
# Same GIT_DIR hazard as tests/scripts/test-binary-freshness-guard.sh (#1927):
# git exports GIT_DIR and friends to hooks, so a run under `make ci-gate` inside
# the pre-push hook would otherwise send every fixture commit to the caller's
# repo. Drop the repo-selecting environment and assert the fixtures resolve to
# themselves before committing anything.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX GIT_CONFIG_PARAMETERS

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
HOOK="$ROOT/.githooks/pre-push"
CHECK="$ROOT/scripts/check-binary-freshness.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() { grep -Fq -- "$2" "$1" || fail "$1 does not contain: $2"; }
assert_lacks()    { grep -Fq -- "$2" "$1" && fail "$1 unexpectedly contains: $2" || true; }

physical_path() { ( cd "$1" 2>/dev/null && pwd -P ); }

fake_git() {
  local repo=$1
  shift
  git -C "$repo" -c commit.gpgsign=false -c user.email=test@example.com \
    -c user.name=test "$@"
}

# ── fixture ─────────────────────────────────────────────────────────────────
# build_fixture <dir> : creates <dir>/origin.git (bare), <dir>/repo (clone),
# <dir>/stub/{make,docker}. The stub `make` logs and PASSES so the test never
# runs the real ci-gate/ci-full (and never recurses into this test); the stub
# `docker` exits 99 and logs, so "the push path needs no docker" is provable.
build_fixture() {
  local dir=$1
  FIX_REPO="$dir/repo"

  mkdir -p "$dir/stub"
  cat >"$dir/stub/make" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${FAKE_MAKE_LOG:?}"
exit 0
EOF
  cat >"$dir/stub/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${FAKE_DOCKER_LOG:?}"
exit 99
EOF
  chmod +x "$dir/stub/make" "$dir/stub/docker"
  : >"$dir/make.log"
  : >"$dir/docker.log"
  : >"$dir/frontend.log"

  git init -q --bare "$dir/origin.git"
  git init -q "$FIX_REPO"
  git -C "$FIX_REPO" symbolic-ref HEAD refs/heads/main

  # Own-repo assertion (#1927): refuse to write fixtures if a leaked GIT_DIR
  # would redirect these git calls into the caller's repository.
  test -d "$FIX_REPO/.git" || fail "throwaway repo $FIX_REPO has no .git (GIT_DIR leak?)"
  test "$(cd "$FIX_REPO" && git rev-parse --show-toplevel)" = "$(physical_path "$FIX_REPO")" || \
    fail "throwaway repo $FIX_REPO does not resolve to itself (GIT_DIR leak?); refusing to commit fixtures"
  test "$(physical_path "$FIX_REPO")" != "$(physical_path "$ROOT")" || \
    fail "throwaway repo $FIX_REPO resolves to the caller's checkout; refusing to commit fixtures"

  git -C "$FIX_REPO" remote add origin "$dir/origin.git"
}

# seed_main : README.md (not a build input) then main.go (build input) = origin/main.
# Leaves the checkout on branch fix/lane.
seed_main() {
  printf 'readme\n' >"$FIX_REPO/README.md"
  fake_git "$FIX_REPO" add README.md
  fake_git "$FIX_REPO" commit -q -m "docs: seed"
  SEED_ROOT=$(git -C "$FIX_REPO" rev-parse HEAD)
  printf 'package main\n' >"$FIX_REPO/main.go"
  fake_git "$FIX_REPO" add main.go
  fake_git "$FIX_REPO" commit -q -m "feat: build input"
  SEED_BUILD=$(git -C "$FIX_REPO" rev-parse HEAD)
  fake_git "$FIX_REPO" push -q origin main
  # Establish refs/remotes/origin/main the way the hook expects to find it.
  fake_git "$FIX_REPO" fetch -q origin main
  fake_git "$FIX_REPO" checkout -q -b fix/lane
}

add_go_commit() {
  printf 'package main\n// lane change\n' >"$FIX_REPO/handler.go"
  fake_git "$FIX_REPO" add handler.go
  fake_git "$FIX_REPO" commit -q -m "feat: go change"
  GO_COMMIT=$(git -C "$FIX_REPO" rev-parse HEAD)
}

add_docs_commit() {
  mkdir -p "$FIX_REPO/docs"
  printf 'note\n' >>"$FIX_REPO/docs/note.md"
  fake_git "$FIX_REPO" add docs/note.md
  fake_git "$FIX_REPO" commit -q -m "docs: note"
  DOCS_COMMIT=$(git -C "$FIX_REPO" rev-parse HEAD)
}

# fake_host_binary <rel-path> <commit-ish> — the checker reads buildinfo out of
# the file with `strings | grep Commit=`, so a text file is a valid stand-in.
fake_host_binary() {
  local rel=$1 commitish=$2 sha
  sha=$(git -C "$FIX_REPO" rev-parse "$commitish")
  mkdir -p "$FIX_REPO/$(dirname "$rel")"
  printf 'stand-in host binary\nbuildinfo.Commit=%s\n' "$sha" >"$FIX_REPO/$rel"
}

# install_fixture_hook <repo> : give the fixture its own copy of the REAL hook +
# checker (the hook and the checker both resolve paths from their own location,
# so the fixture must own them) and a stub frontend-dist check.
install_fixture_hook() {
  local repo=$1
  mkdir -p "$repo/.githooks" "$repo/scripts/ci"
  cp "$HOOK" "$repo/.githooks/pre-push"
  cp "$CHECK" "$repo/scripts/check-binary-freshness.sh"
  cat >"$repo/scripts/ci/check_frontend_dist.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${FAKE_FRONTEND_LOG:?}"
exit 0
EOF
  # Anti-drift: a copy that differs from the committed hook would make this test
  # pass while the real gate is broken.
  cmp -s "$HOOK" "$repo/.githooks/pre-push" || fail "fixture hook copy drifted from $HOOK"
  cmp -s "$CHECK" "$repo/scripts/check-binary-freshness.sh" || fail "fixture checker copy drifted from $CHECK"
}

# run_hook <dir> <repo> → HOOK_RC / HOOK_OUT (combined stdout+stderr)
run_hook() {
  local dir=$1 repo=$2
  install_fixture_hook "$repo"
  HOOK_RC=0
  HOOK_OUT=$(cd "$repo" && env \
      PATH="$dir/stub:$PATH" \
      FAKE_MAKE_LOG="$dir/make.log" \
      FAKE_DOCKER_LOG="$dir/docker.log" \
      FAKE_FRONTEND_LOG="$dir/frontend.log" \
      DOCKER_BIN="$dir/stub/docker" \
      PRE_PUSH_FULL="${PRE_PUSH_FULL_OVERRIDE:-auto}" \
      bash "$repo/.githooks/pre-push" origin "git@example.invalid:kaecer68/atlas-go.git" 2>&1) || HOOK_RC=$?
}

run_checker() {
  local repo=$1
  shift
  CHECK_RC=0
  CHECK_OUT=$(cd "$repo" && bash scripts/check-binary-freshness.sh "$@" 2>&1) || CHECK_RC=$?
}

# ── E4: fetch failure must refuse the push, not wave it through ─────────────
run_fetch_failure_test() {
  local dir repo out
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  build_fixture "$dir"
  seed_main
  add_go_commit
  repo=$FIX_REPO
  install_fixture_hook "$repo"

  # Unreachable origin: a local path that does not exist. No network, and the
  # real remote is never touched.
  git -C "$repo" remote set-url origin "$dir/does-not-exist.git"

  run_hook "$dir" "$repo"
  out=$HOOK_OUT
  test "$HOOK_RC" -ne 0 || fail "pre-push returned 0 when origin/main could not be fetched (E4 fail-open regression): $out"
  printf '%s\n' "$out" | grep -Fq -- "cannot fetch origin/main" || fail "fetch-failure message missing: $out"
  printf '%s\n' "$out" | grep -Fq -- "fail-closed" || fail "fetch-failure message does not say fail-closed: $out"
  printf '%s\n' "$out" | grep -Fq -- "git push --no-verify" || fail "fetch-failure message does not offer the explicit bypass: $out"
  # The old code ran make ci-gate first and then exited 0. Refusing early is the
  # point: do not spend 30s on gates whose verdict cannot be trusted.
  test ! -s "$dir/make.log" || fail "hook ran make before failing the fetch gate: $(cat "$dir/make.log")"
  test ! -s "$dir/docker.log" || fail "hook invoked docker: $(cat "$dir/docker.log")"
}

# ── FU-20260926-15: stale host binary blocks with an actionable message ─────
run_stale_host_binary_test() {
  local dir repo out
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  build_fixture "$dir"
  seed_main
  add_go_commit
  add_docs_commit
  repo=$FIX_REPO

  # bin/atlas-mcp built BEFORE the only build input in this push → STALE.
  # bin/atlas built AT that build input → fresh (and HEAD is one commit further,
  # which must NOT count as stale: the rule is "contains the last build input").
  fake_host_binary bin/atlas-mcp "$SEED_BUILD"
  fake_host_binary bin/atlas "$GO_COMMIT"

  run_hook "$dir" "$repo"
  out=$HOOK_OUT
  test "$HOOK_RC" -ne 0 || fail "pre-push allowed a push with a stale bin/atlas-mcp (FU-20260926-15 regression): $out"
  printf '%s\n' "$out" | grep -Fq -- "bin/atlas-mcp" || fail "stale-binary message does not name bin/atlas-mcp: $out"
  printf '%s\n' "$out" | grep -Fq -- "STALE" || fail "stale-binary message does not say STALE: $out"
  printf '%s\n' "$out" | grep -Fq -- "make rebuild-host-bin" || fail "stale-binary message does not give the fix command: $out"
  printf '%s\n' "$out" | grep -Fq -- "make build-backend" || fail "stale-binary message does not cover bin/atlas: $out"
  printf '%s\n' "$out" | grep -Fq -- "✓ bin/atlas:" || fail "fresh bin/atlas was not reported as fresh: $out"
  printf '%s\n' "$out" | grep -Fq -- "✗ STALE  bin/atlas:" && fail "fresh bin/atlas was reported as STALE (false red): $out" || true
  test ! -s "$dir/docker.log" || fail "host binary gate invoked docker: $(cat "$dir/docker.log")"

  # Same fixture, now with only bin/atlas stale: the CLI binary must be covered
  # too (bin/atlas* is the contract, not bin/atlas-mcp alone).
  fake_host_binary bin/atlas "$SEED_BUILD"
  fake_host_binary bin/atlas-mcp "$GO_COMMIT"
  run_hook "$dir" "$repo"
  test "$HOOK_RC" -ne 0 || fail "pre-push allowed a push with a stale bin/atlas: $HOOK_OUT"
  printf '%s\n' "$HOOK_OUT" | grep -Fq -- "✗ STALE  bin/atlas:" || fail "stale bin/atlas not named: $HOOK_OUT"
}

# ── fresh host binaries: must not block (negative control for false reds) ───
run_fresh_host_binary_test() {
  local dir repo out
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  build_fixture "$dir"
  seed_main
  add_go_commit
  add_docs_commit
  repo=$FIX_REPO

  fake_host_binary bin/atlas "$GO_COMMIT"
  fake_host_binary bin/atlas-mcp "$GO_COMMIT"

  run_hook "$dir" "$repo"
  out=$HOOK_OUT
  test "$HOOK_RC" -eq 0 || fail "pre-push blocked a push with fresh host binaries: $out"
  # The gate really evaluated (build input changed in origin/main...HEAD) and the
  # rest of the hook still ran.
  grep -Fq -- "ci-gate" "$dir/make.log" || fail "hook never ran make ci-gate: $(cat "$dir/make.log")"
  grep -Fq -- "ci-full" "$dir/make.log" || fail "hook never ran make ci-full for a code diff: $(cat "$dir/make.log")"
  test -s "$dir/frontend.log" || fail "hook never ran the frontend dist check"
  test ! -s "$dir/docker.log" || fail "hook invoked docker: $(cat "$dir/docker.log")"
}

# ── docs-only push: a pre-existing stale binary must NOT block it ───────────
run_docs_only_test() {
  local dir repo out
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  build_fixture "$dir"
  seed_main
  add_docs_commit
  repo=$FIX_REPO

  # STALE relative to the last build input (SEED_BUILD): this push carries no
  # build input, so the staleness cannot be caused by it.
  fake_host_binary bin/atlas-mcp "$SEED_ROOT"
  fake_host_binary bin/atlas "$SEED_ROOT"

  install_fixture_hook "$repo"
  # Control: without --diff-base the same binaries ARE red — so the pass below
  # proves the skip, not a broken rule.
  run_checker "$repo" --host-only
  test "$CHECK_RC" -eq 1 || fail "control: host-only check should have been STALE (exit 1), got $CHECK_RC: $CHECK_OUT"
  run_checker "$repo" --host-only --diff-base origin/main
  test "$CHECK_RC" -eq 0 || fail "host-only --diff-base should skip a docs-only push, got $CHECK_RC: $CHECK_OUT"
  printf '%s\n' "$CHECK_OUT" | grep -Fq -- "no build input changed" || \
    fail "docs-only skip was not explained: $CHECK_OUT"

  run_hook "$dir" "$repo"
  out=$HOOK_OUT
  test "$HOOK_RC" -eq 0 || fail "pre-push blocked a docs-only push over a pre-existing stale binary: $out"
  grep -Fq -- "ci-gate" "$dir/make.log" || fail "docs-only push did not run ci-gate: $(cat "$dir/make.log")"
  grep -Fq -- "ci-full" "$dir/make.log" && fail "docs-only push ran ci-full (PRE_PUSH_FULL=auto should skip): $(cat "$dir/make.log")" || true
}

# ── clone without bin/ (fresh / linked worktree) must never be blocked ──────
run_no_host_binaries_test() {
  local dir repo out
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  build_fixture "$dir"
  seed_main
  add_go_commit
  repo=$FIX_REPO

  run_hook "$dir" "$repo"
  out=$HOOK_OUT
  test "$HOOK_RC" -eq 0 || fail "pre-push blocked a clone that has no bin/ at all: $out"
  grep -Fq -- "ci-gate" "$dir/make.log" || fail "hook never ran make ci-gate: $(cat "$dir/make.log")"
}

# ── PRE_PUSH_FULL override must never be a silent skip ─────────────────────
run_override_test() {
  local dir1 dir2 out

  # (a) `never` on a code diff: ci-full must be skipped OUT LOUD.
  dir1=$(mktemp -d)
  trap 'rm -rf "$dir1"' RETURN
  build_fixture "$dir1"
  seed_main
  add_go_commit
  fake_host_binary bin/atlas "$GO_COMMIT"
  fake_host_binary bin/atlas-mcp "$GO_COMMIT"
  PRE_PUSH_FULL_OVERRIDE=never run_hook "$dir1" "$FIX_REPO"
  out=$HOOK_OUT
  test "$HOOK_RC" -eq 0 || fail "PRE_PUSH_FULL=never should not block: $out"
  printf '%s\n' "$out" | grep -Fq -- "PRE_PUSH_FULL=never — ci-full 明示跳過" || \
    fail "PRE_PUSH_FULL=never skipped ci-full silently: $out"
  grep -Fq -- "ci-full" "$dir1/make.log" && fail "PRE_PUSH_FULL=never still ran ci-full" || true

  # (b) `always` on a docs-only diff must still run ci-full (the `always` branch
  #     must not be swallowed by the auto/docs condition — `||` and `&&` have
  #     equal precedence in bash, so the braces are load-bearing).
  dir2=$(mktemp -d)
  trap 'rm -rf "$dir2"' RETURN
  build_fixture "$dir2"
  seed_main
  add_docs_commit
  PRE_PUSH_FULL_OVERRIDE=always run_hook "$dir2" "$FIX_REPO"
  test "$HOOK_RC" -eq 0 || fail "PRE_PUSH_FULL=always should not block: $HOOK_OUT"
  grep -Fq -- "ci-full" "$dir2/make.log" || \
    fail "PRE_PUSH_FULL=always did not run ci-full on a docs-only diff: $(cat "$dir2/make.log")"
  PRE_PUSH_FULL_OVERRIDE=auto
}

# ── static contract: the wiring itself ─────────────────────────────────────
run_static_contract_tests() {
  assert_contains "$HOOK" '--host-only --diff-base origin/main'
  assert_contains "$HOOK" 'PRE_PUSH_FULL=never — ci-full 明示跳過'
  assert_contains "$HOOK" 'check-binary-freshness.sh'
  assert_contains "$CHECK" "HOST_BINARIES=('bin/atlas' 'bin/atlas-mcp')"
  assert_contains "$CHECK" '--host-only'
  # Reverting to the E4 fail-open wording must break this test.
  assert_lacks "$HOOK" 'skipping ci-full/redundancy checks'
  # The sanctioned early exit stays (non-origin remote).
  assert_contains "$HOOK" 'if [ "$remote" != "origin" ]; then'
  # The checker must not reach for docker in host-only mode.
  assert_contains "$CHECK" 'if [ "$HOST_ONLY" -eq 0 ]; then'
}

run_fetch_failure_test
run_stale_host_binary_test
run_fresh_host_binary_test
run_docs_only_test
run_no_host_binaries_test
run_override_test
run_static_contract_tests

echo "PASS: pre-push gate contract tests"
