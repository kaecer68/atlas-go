#!/usr/bin/env bash
set -euo pipefail

# Contract test for .githooks/pre-push (hermetic: throwaway git repos, LOCAL bare
# remote, PATH-stubbed `make`, recording docker stub; no network, no docker
# daemon, no writes to the caller's checkout).
#
# Pins down three defects fixed 2026-09-26/27 in ONE file:
#   E30 — a delete-only push (`git push --delete origin <branch>`, `git push
#         origin :<branch>`) was judged by content gates and refused, so routine
#         post-merge branch cleanup had to use `--no-verify`. Git passes the
#         pushed refspecs on the hook's STDIN; a delete line reads
#         `(delete) 0000…0 <remote ref> <old sha>`.
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

add_config_commit() {
  mkdir -p "$FIX_REPO/configs"
  printf '{"k":1}\n' >>"$FIX_REPO/configs/parameters.json"
  fake_git "$FIX_REPO" add configs/parameters.json
  fake_git "$FIX_REPO" commit -q -m "config: parameters value change"
  CONFIG_COMMIT=$(git -C "$FIX_REPO" rev-parse HEAD)
}

add_docs_commit() {
  mkdir -p "$FIX_REPO/docs"
  printf 'note\n' >>"$FIX_REPO/docs/note.md"
  fake_git "$FIX_REPO" add docs/note.md
  fake_git "$FIX_REPO" commit -q -m "docs: note"
  DOCS_COMMIT=$(git -C "$FIX_REPO" rev-parse HEAD)
}

# add_empty_commit : make branch fix/lane ALREADY MERGED into origin/main, with
# main then moving one EMPTY commit further, then come back to fix/lane.
# Result: lane is an ancestor of origin/main, so
#   * `rev-parse HEAD` != origin/main  → Gate 2 passes, and
#   * `git diff --name-only origin/main...HEAD` is empty → Gate 3 fires.
# That is the real post-merge shape: the branch is merged, and all that is left
# is deleting the now-redundant remote branch. That is exactly the push E30 is
# about. (Without the merge step the merge base would sit before the lane commit
# and the diff would be non-empty, i.e. an ordinary content push.)
add_empty_commit() {
  fake_git "$FIX_REPO" push -q origin fix/lane:refs/heads/fix/lane
  fake_git "$FIX_REPO" checkout -q main
  fake_git "$FIX_REPO" merge -q --ff-only fix/lane
  fake_git "$FIX_REPO" push -q origin main
  fake_git "$FIX_REPO" commit -q --allow-empty -m "chore: empty"
  fake_git "$FIX_REPO" push -q origin main
  fake_git "$FIX_REPO" fetch -q origin main
  fake_git "$FIX_REPO" checkout -q fix/lane
}

# build_merged_fixture <dir> : fixture in that merged, zero-diff state (FIX_REPO).
build_merged_fixture() {
  local dir=$1
  build_fixture "$dir"
  seed_main
  add_go_commit
  add_empty_commit
  # Fixture self-check: without this shape the delete cases below would pass for
  # the wrong reason (Gate 2 would fire first, or the branch would not be
  # zero-diff and Gate 3 would have nothing to say).
  test "$(git -C "$FIX_REPO" rev-parse HEAD)" != "$(git -C "$FIX_REPO" rev-parse origin/main)" || \
    fail "fixture: HEAD == origin/main — Gate 2 would fire before Gate 3"
  test -z "$(git -C "$FIX_REPO" diff --name-only origin/main...HEAD)" || \
    fail "fixture: branch is not zero-diff vs origin/main"
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

# run_hook <dir> <repo> [refspecs] → HOOK_RC / HOOK_OUT (combined stdout+stderr)
# The optional third argument is the refspec list git writes to the hook's STDIN,
# one line per ref: `<local ref> <local sha> <remote ref> <remote sha>`. It always
# feeds a PIPE (never the caller's terminal), so omitting it means "Git gave the
# hook no refspec information" — the case the classification must fail closed on.
run_hook() {
  local dir=$1 repo=$2 refspecs=${3-} stdin_text=""
  install_fixture_hook "$repo"
  [ -z "$refspecs" ] || stdin_text=$(printf '%s\n' "$refspecs")
  HOOK_RC=0
  HOOK_OUT=$(cd "$repo" && printf '%s' "$stdin_text" | env \
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

# ── E30 (a): delete-only push (the refspec git writes for `--delete`) ───────
run_delete_only_refspec_test() {
  local dir repo out
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  build_merged_fixture "$dir"
  repo=$FIX_REPO

  run_hook "$dir" "$repo" "(delete) 0000000000000000000000000000000000000000 refs/heads/lane $(git -C "$repo" rev-parse HEAD)"
  out=$HOOK_OUT
  test "$HOOK_RC" -eq 0 || fail "(a) delete-only push was blocked (E30): $out"
  # Not silent: the skip and its reason must both be on the record.
  printf '%s\n' "$out" | grep -Fq -- "delete-only push" || fail "(a) delete-only push produced no delete-only explanation (silent skip): $out"
  printf '%s\n' "$out" | grep -Fq -- "沒有內容可守" || fail "(a) delete-only skip does not state the reason (沒有內容可守): $out"
  printf '%s\n' "$out" | grep -Fq -- "1 個 ref 全是刪除" || fail "(a) skipped refspec count not reported: $out"
  # Every skipped gate must be named.
  printf '%s\n' "$out" | grep -Fq -- "Gate 0b" || fail "(a) skip does not name Gate 0b (host binary freshness): $out"
  printf '%s\n' "$out" | grep -Fq -- "make ci-gate" || fail "(a) skip does not name Gate 1 (ci-gate): $out"
  printf '%s\n' "$out" | grep -Fq -- "make ci-full" || fail "(a) skip does not name Gate 1b (ci-full): $out"
  printf '%s\n' "$out" | grep -Fq -- "Gate 1c  frontend dist freshness" || fail "(a) skip does not name Gate 1c (frontend dist): $out"
  printf '%s\n' "$out" | grep -Fq -- "Gate 2" || fail "(a) skip does not name Gate 2 (HEAD == origin/main): $out"
  printf '%s\n' "$out" | grep -Fq -- "Gate 3" || fail "(a) skip does not name Gate 3 (zero diff): $out"
  # ...and the refusal it replaces must not appear.
  printf '%s\n' "$out" | grep -Fq -- "❌ pre-push: branch has zero file diff" && fail "(a) Gate 3 still fired on a delete-only push: $out" || true
  # Nothing was executed: no make (ci-gate/ci-full), no docker, no frontend check.
  test ! -s "$dir/make.log" || fail "(a) delete-only push ran make: $(cat "$dir/make.log")"
  test ! -s "$dir/docker.log" || fail "(a) delete-only push invoked docker: $(cat "$dir/docker.log")"
  test ! -s "$dir/frontend.log" || fail "(a) delete-only push ran the frontend dist check"
}

# ── E30 (b): same outcome when REAL git supplies the refspec on stdin ───────
# `git push --delete origin <b>` and `git push origin :<b>` both make git write
# the stdin line itself, so this is the un-faked end of the path (local bare
# remote only; the real origin is never touched).
run_delete_only_real_git_test() {
  local dir rc out
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  build_merged_fixture "$dir"
  install_fixture_hook "$FIX_REPO"
  # A second, identical lane for the `:<branch>` spelling. Publish it BEFORE the
  # hook is armed below — every push from here on really goes through it.
  fake_git "$FIX_REPO" push -q origin fix/lane:refs/heads/fix/colon
  git -C "$FIX_REPO" config core.hooksPath "$FIX_REPO/.githooks"

  rc=0
  out=$(cd "$FIX_REPO" && env PATH="$dir/stub:$PATH" \
        FAKE_MAKE_LOG="$dir/make.log" FAKE_DOCKER_LOG="$dir/docker.log" \
        FAKE_FRONTEND_LOG="$dir/frontend.log" DOCKER_BIN="$dir/stub/docker" \
        git push --delete origin fix/lane 2>&1) || rc=$?
  test "$rc" -eq 0 || fail "(b) git push --delete origin fix/lane was blocked (E30): $out"
  printf '%s\n' "$out" | grep -Fq -- "delete-only push" || fail "(b) git push --delete skipped the gates silently: $out"
  test ! -s "$dir/make.log" || fail "(b) git push --delete ran make: $(cat "$dir/make.log")"
  test ! -s "$dir/docker.log" || fail "(b) git push --delete invoked docker: $(cat "$dir/docker.log")"
  # The push really happened — the hook let it through, it did not fake it.
  git -C "$dir/origin.git" rev-parse --verify -q refs/heads/fix/lane >/dev/null && \
    fail "(b) remote branch fix/lane still exists after the delete push" || true

  rc=0
  out=$(cd "$FIX_REPO" && env PATH="$dir/stub:$PATH" \
        FAKE_MAKE_LOG="$dir/make.log" FAKE_DOCKER_LOG="$dir/docker.log" \
        FAKE_FRONTEND_LOG="$dir/frontend.log" DOCKER_BIN="$dir/stub/docker" \
        git push origin :fix/colon 2>&1) || rc=$?
  test "$rc" -eq 0 || fail "(b) git push origin :fix/colon was blocked (E30): $out"
  printf '%s\n' "$out" | grep -Fq -- "delete-only push" || fail "(b) git push origin :branch skipped the gates silently: $out"
  test ! -s "$dir/make.log" || fail "(b) git push origin :branch ran make: $(cat "$dir/make.log")"
  git -C "$dir/origin.git" rev-parse --verify -q refs/heads/fix/colon >/dev/null && \
    fail "(b) remote branch fix/colon still exists after the delete push" || true
}

# ── E30 (c): a content push is still judged (and still blocked) ────────────
run_content_push_still_gated_test() {
  local dir repo sha out
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  build_merged_fixture "$dir"
  repo=$FIX_REPO
  sha=$(git -C "$repo" rev-parse HEAD)

  run_hook "$dir" "$repo" "$sha $sha refs/heads/lane $sha"
  out=$HOOK_OUT
  test "$HOOK_RC" -ne 0 || fail "(c) content push on a zero-diff branch was allowed: $out"
  printf '%s\n' "$out" | grep -Fq -- "zero file diff vs origin/main" || fail "(c) Gate 3 did not fire for a content push: $out"
  printf '%s\n' "$out" | grep -Fq -- "delete-only push" && fail "(c) content push took the delete-only skip path: $out" || true
  grep -Fq -- "ci-gate" "$dir/make.log" || fail "(c) content push did not run ci-gate: $(cat "$dir/make.log")"
}

# ── E30 (c2): a CREATE line carries an all-zero REMOTE sha, not a delete ────
run_create_refspec_is_not_delete_test() {
  local dir repo sha out
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  build_merged_fixture "$dir"
  repo=$FIX_REPO
  sha=$(git -C "$repo" rev-parse HEAD)

  run_hook "$dir" "$repo" "$sha $sha refs/heads/lane 0000000000000000000000000000000000000000"
  out=$HOOK_OUT
  test "$HOOK_RC" -ne 0 || fail "(c2) create-shaped refspec (all-zero remote sha) was treated as a delete: $out"
  printf '%s\n' "$out" | grep -Fq -- "zero file diff vs origin/main" || fail "(c2) Gate 3 did not fire for a create-shaped refspec: $out"
  printf '%s\n' "$out" | grep -Fq -- "delete-only push" && fail "(c2) create-shaped refspec took the delete-only skip path: $out" || true
}

# ── E30 (d): a mixed push must not let the delete whitewash the content ────
run_mixed_delete_and_content_test() {
  local dir repo sha out
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  build_merged_fixture "$dir"
  repo=$FIX_REPO
  sha=$(git -C "$repo" rev-parse HEAD)

  run_hook "$dir" "$repo" "(delete) 0000000000000000000000000000000000000000 refs/heads/other $sha
$sha $sha refs/heads/lane $sha"
  out=$HOOK_OUT
  test "$HOOK_RC" -ne 0 || fail "(d) mixed delete+content push was allowed (a delete whitewashed content): $out"
  printf '%s\n' "$out" | grep -Fq -- "zero file diff vs origin/main" || fail "(d) Gate 3 did not fire on the content half of a mixed push: $out"
  printf '%s\n' "$out" | grep -Fq -- "delete-only push" && fail "(d) mixed push took the delete-only skip path: $out" || true
  grep -Fq -- "ci-gate" "$dir/make.log" || fail "(d) mixed push did not run ci-gate: $(cat "$dir/make.log")"
}

# ── E30 (e): no refspec information at all must fail CLOSED ────────────────
run_empty_stdin_is_content_push_test() {
  local dir repo out
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  build_merged_fixture "$dir"
  repo=$FIX_REPO

  run_hook "$dir" "$repo"          # no refspecs: git gave us nothing to read
  out=$HOOK_OUT
  test "$HOOK_RC" -ne 0 || fail "(e) empty stdin was treated as a delete-only push (fail-open): $out"
  printf '%s\n' "$out" | grep -Fq -- "zero file diff vs origin/main" || fail "(e) Gate 3 did not fire when stdin was empty: $out"
  printf '%s\n' "$out" | grep -Fq -- "delete-only push" && fail "(e) empty stdin took the delete-only skip path: $out" || true
  grep -Fq -- "ci-gate" "$dir/make.log" || fail "(e) empty-stdin push did not run ci-gate: $(cat "$dir/make.log")"
}

# ── static contract: the wiring itself ─────────────────────────────────────
line_of() { grep -Fn -- "$2" "$1" | head -n 1 | cut -d: -f1; }


# ── configs/** 變更屬「值變更」⇒ 必須跑 ci-full（不得歸為 docs-only）────────
# 缺陷（2026-09-30 實證）：分類 grep 未含 configs/ ⇒ 只改 configs/parameters.json
# 的 push 被當成 docs-only 而跳過 ci-full。
run_config_only_requires_ci_full_test() {
  local dir repo out
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  build_fixture "$dir"
  seed_main
  add_config_commit
  repo=$FIX_REPO
  fake_host_binary bin/atlas "$SEED_ROOT"
  fake_host_binary bin/atlas-mcp "$SEED_ROOT"

  run_hook "$dir" "$repo"
  out=$HOOK_OUT
  test "$HOOK_RC" -eq 0 || fail "pre-push blocked a config-only push: $out"
  assert_contains "$dir/make.log" "ci-gate"
  assert_contains "$dir/make.log" "ci-full"
}

# ── 混合變更（configs ＋ docs）：OR 語意 ⇒ 任一命中即 code ⇒ 跑 ci-full ──────
run_mixed_config_docs_requires_ci_full_test() {
  local dir repo out
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  build_fixture "$dir"
  seed_main
  add_config_commit
  add_docs_commit
  repo=$FIX_REPO
  fake_host_binary bin/atlas "$SEED_ROOT"
  fake_host_binary bin/atlas-mcp "$SEED_ROOT"

  run_hook "$dir" "$repo"
  out=$HOOK_OUT
  test "$HOOK_RC" -eq 0 || fail "pre-push blocked a mixed config+docs push: $out"
  assert_contains "$dir/make.log" "ci-gate"
  assert_contains "$dir/make.log" "ci-full"
}

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

  # E30: delete-only classification must exist, be keyed on git's `(delete)`
  # marker, name its reason, and sit BEFORE the content gates it bypasses.
  # Reverting any of this must turn this test red.
  assert_contains "$HOOK" 'delete_only=1'
  assert_contains "$HOOK" "'(delete) '*"
  assert_contains "$HOOK" 'refspec_deletes'
  assert_contains "$HOOK" 'delete-only push：沒有內容可守'
  assert_contains "$HOOK" 'delete-only push（'
  local del_line gate_line
  del_line=$(line_of "$HOOK" 'if [ "$refspec_total" -gt 0 ]')
  gate_line=$(line_of "$HOOK" 'host_out=$(bash scripts/check-binary-freshness.sh')
  test -n "$del_line" && test -n "$gate_line" || \
    fail "static: E30 classification / first content gate marker missing from $HOOK"
  test "$del_line" -lt "$gate_line" || \
    fail "static: delete-only classification (line $del_line) must precede the content gates (line $gate_line)"
  # ...and this test file must actually be able to feed stdin (its whole point).
  assert_contains "$0" 'refspecs=${3-}'
}

run_fetch_failure_test
run_stale_host_binary_test
run_fresh_host_binary_test
run_docs_only_test
run_config_only_requires_ci_full_test
run_mixed_config_docs_requires_ci_full_test
run_no_host_binaries_test
run_override_test
run_delete_only_refspec_test
run_delete_only_real_git_test
run_content_push_still_gated_test
run_create_refspec_is_not_delete_test
run_mixed_delete_and_content_test
run_empty_stdin_is_content_push_test
run_static_contract_tests

echo "PASS: pre-push gate contract tests"
