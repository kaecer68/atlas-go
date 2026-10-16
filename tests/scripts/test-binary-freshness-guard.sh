#!/usr/bin/env bash
set -euo pipefail

# This file creates throwaway git repositories. Git exports GIT_DIR (and
# friends) to hooks, so a hook-invoked run would send every `git -C <temp>`
# command to the CALLER's repository: on 2026-09-23 the pre-push run of
# `make ci-gate` committed this test's fixture commits ("docs seed",
# "go change", "compose + docs only") straight onto the branch being pushed,
# which then failed the commit-message check. Drop the repo-selecting
# environment so each git call honours its own -C/cwd.
# See issue #1927.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
CHECK="$ROOT/scripts/check-binary-freshness.sh"
SESSION_START="$ROOT/scripts/session-start.sh"
RETAG_TARGET=retag-cron-images
TMPDIR_ATLAS_SVCS=$(mktemp "${TMPDIR:-/tmp}/atlas-family-svcs.XXXXXX")

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# Physical (symlink-resolved) path. macOS reports /var/... for mktemp but git
# may report /private/var/...; comparing unresolved strings would hide a leak.
physical_path() {
  ( cd "$1" 2>/dev/null && pwd -P )
}

assert_contains() {
  local file=$1
  local pattern=$2
  grep -Fq -- "$pattern" "$file" || fail "$file does not contain: $pattern"
}

run_cron_retag_test() {
  local dir
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN

  cat >"$dir/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${FAKE_DOCKER_LOG:?}"
EOF
  chmod +x "$dir/docker"
  : >"$dir/docker.log"

  if ! make -s -C "$ROOT" "$RETAG_TARGET" DOCKER_BIN="$dir/docker" FAKE_DOCKER_LOG="$dir/docker.log"; then
    fail "Makefile does not provide a working $RETAG_TARGET target"
  fi
  # Drift guard (2026-09-23, #1898): derive the expected set from
  # docker-compose.yml instead of hardcoding a count. A cron service added to
  # compose without a matching CRON_IMAGE_TAGS entry used to surface only at
  # deploy time (atlas-cron-darwinian -> "No such image", Mac Mini 2026-09-23).
  local expected expected_count produced
  expected=$(sed -nE 's/^  ((cron|atlas-cron)-[a-z0-9-]+):.*/\1/p' "$ROOT/docker-compose.yml" \
    | sed 's/^/atlas-/' | sort -u)
  [ -n "$expected" ] || fail "could not derive cron services from docker-compose.yml"
  expected_count=$(printf '%s\n' "$expected" | wc -l | tr -d ' ')
  for tag in $expected; do
    grep -Fq -- "tag atlas-cron-rebuilt:local ${tag}:latest" "$dir/docker.log" || \
      fail "$RETAG_TARGET is missing an image tag for compose service $tag"
  done
  produced=$(grep -c '^tag ' "$dir/docker.log" || true)
  test "$produced" -eq "$expected_count" || \
    fail "$RETAG_TARGET retagged $produced images but docker-compose.yml declares $expected_count cron services"
}
run_cleanup_failure_test() {
  local dir
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN

  cat >"$dir/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  create)
    echo fake-freshness-container
    ;;
  cp)
    exit 42
    ;;
  rm)
    echo "$*" >>"${FAKE_DOCKER_RM_LOG:?}"
    ;;
  *)
    exit 99
    ;;
esac
EOF
  chmod +x "$dir/docker"
  : >"$dir/rm.log"

  # 精確化（issue #2011）：舊寫法 `if "$CHECK"; then fail; fi` 把**任何**非 0（含 2=用法錯、
  # 126/127=根本沒跑起來）都當成「正確地失敗」。這裡的失敗碼不固定（假 docker 的 `cp` 回 42，
  # 會被往上傳），所以斷言「非 0、且不是代表『沒跑起來』的 2/126/127」。
  local rc=0
  DOCKER_BIN="$dir/docker" FAKE_DOCKER_RM_LOG="$dir/rm.log" "$CHECK" >/dev/null 2>&1 || rc=$?
  case "$rc" in
    0)        fail "freshness check unexpectedly succeeded when docker cp failed" ;;
    2|126|127) fail "freshness check 沒跑起來（exit=${rc}）——這不是「正確地失敗」" ;;
  esac
  grep -Fxq -- 'rm -f fake-freshness-container' "$dir/rm.log" || \
    fail "freshness check did not remove container after docker cp failure"
}

run_missing_image_test() {
  local dir
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  cat >"$dir/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  create) exit 1 ;;
  *) exit 99 ;;
esac
EOF
  chmod +x "$dir/docker"
  local rc=0
  DOCKER_BIN="$dir/docker" "$CHECK" >/dev/null 2>&1 || rc=$?
  case "$rc" in
    0)        fail "freshness check unexpectedly succeeded when images were unavailable" ;;
    2|126|127) fail "freshness check 沒跑起來（exit=${rc}）——這不是「正確地失敗」" ;;
  esac
}

run_success_cleanup_test() {
  local dir
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN

  cat >"$dir/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
counter="${FAKE_DOCKER_COUNTER:?}"
case "${1:-}" in
  create)
    # A distinct name per create: "every created container was removed" is only
    # a real assertion when the names differ.
    n=$(cat "$counter")
    printf '%s\n' "$((n + 1))" >"$counter"
    printf 'fake-container-%s\n' "$n" | tee -a "${FAKE_DOCKER_CREATE_LOG:?}"
    ;;
  cp)
    destination=${@: -1}
    case "$destination" in
      "${FAKE_FRESHNESS_TMPDIR:?}"/*) ;;
      *) exit 43 ;;
    esac
    printf 'Commit=%s\n' "${FAKE_DOCKER_HEAD:?}" >"$destination"
    ;;
  rm)
    # The checker calls `docker rm -f <name>`; log the NAME, not "-f".
    printf '%s\n' "${@: -1}" >>"${FAKE_DOCKER_RM_LOG:?}"
    ;;
  *)
    : ;;
esac
EOF
  chmod +x "$dir/docker"
  printf '0\n' >"$dir/counter"
  : >"$dir/rm.log"
  : >"$dir/create.log"
  head=$(git -C "$ROOT" rev-parse HEAD)

  # Isolation (2026-09-26): run an isolated COPY of the checker. It derives
  # REPO_ROOT from its own path and then inspects <REPO_ROOT>/bin/atlas*, so
  # running the checkout's script made this test depend on the CALLER's host
  # binaries. With a stale bin/atlas-mcp the checker exits 1 here — and because
  # this invocation had no `|| rc=$?` under `set -e`, the whole suite died at
  # this line with rc=1 and EMPTY stdout+stderr: `make ci-gate` (gate 1 of
  # .githooks/pre-push) printed only "❌ make ci-gate FAILED" and every push from
  # that clone was blocked with no explanation and no way to act on it
  # (measured 2026-09-26 in the shared clone: bin/atlas-mcp=acdff93c while the
  # last build input was 92c3c823). What is under test here is TEMPORARY
  # CLEANUP, not the caller's binaries, so the checker must not see them.
  mkdir -p "$dir/tool" "$dir/freshness"
  cp "$CHECK" "$dir/tool/check-binary-freshness.sh"

  local rc=0
  DOCKER_BIN="$dir/docker" \
    FRESHNESS_TMPDIR="$dir/freshness" \
    FAKE_FRESHNESS_TMPDIR="$dir/freshness" \
    FAKE_DOCKER_COUNTER="$dir/counter" \
    FAKE_DOCKER_CREATE_LOG="$dir/create.log" \
    FAKE_DOCKER_RM_LOG="$dir/rm.log" \
    FAKE_DOCKER_HEAD="$head" \
    bash "$dir/tool/check-binary-freshness.sh" >/dev/null || rc=$?
  test "$rc" -eq 0 || fail "freshness check exited $rc in the cleanup-success fixture (this line must never kill the suite silently)"

  # Derive the expectation from the script itself (pre-existing drift fixed
  # 2026-09-23: this used to hardcode 6 while check-binary-freshness.sh inspects
  # 5 image binaries -> the guard test was red on main). The real invariant is
  # "every inspected image is cleaned up", not a magic number.
  local expected_rm
  # Indentation-tolerant (2026-09-26): the image checks are now inside
  # `if [ "$HOST_ONLY" -eq 0 ]; then ...`, so a `^check_image_binary` anchor
  # silently derived 0 — and because a zero-match `grep -c` exits 1, the plain
  # `$(...)` assignment under `set -e` killed the suite with empty output (the
  # same silent-death class this file was fixed for above). `|| true` + a
  # whitespace-tolerant pattern make the derivation immune to both.
  expected_rm=$(grep -cE '^[[:space:]]*check_image_binary ' "$CHECK" || true)
  [ "$expected_rm" -gt 0 ] || fail "could not derive image checks from $CHECK"
  test "$(wc -l <"$dir/rm.log" | tr -d ' ')" -eq "$expected_rm" || \
    fail "successful freshness check did not clean all temporary containers (rm=$(wc -l <"$dir/rm.log" | tr -d ' ') expected=$expected_rm)"
  # Count alone would also pass while removing the wrong container (the rm log
  # used to be filled with "-f"): check every created container by name.
  local created name
  created=$(wc -l <"$dir/create.log" | tr -d ' ')
  test "$created" -gt 0 || fail "freshness check created no temporary container (shell would report 0)"
  while read -r name; do
    grep -Fxq -- "$name" "$dir/rm.log" || \
      fail "successful freshness check did not remove temporary container $name"
  done <"$dir/create.log"
  leftover=$(cd "$dir/freshness" && shopt -s nullglob dotglob && echo *)
  if [ -n "$leftover" ]; then
    fail "freshness check left files in its isolated temporary directory: $leftover"
  fi
}

run_session_start_test() {
  local dir
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN

  cat >"$dir/make" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${FAKE_MAKE_LOG:?}"
case "$*" in
  check-binaries)
    n=$(wc -l <"${FAKE_MAKE_LOG}")
    if [ "$n" -eq 1 ]; then exit 1; fi
    ;;
  # Current session-start contract (2026-09-23): a stale binary triggers a
  # host/bin-only rebuild (no docker); the container rebuild stays a manual step
  # for the operator (see ~/.agents/AGENTS.md docker ban).
  rebuild-host-bin\ rebuild-atlas-bins\ rebuild-cron-bins) ;;
  *) exit 99 ;;
esac
EOF
  cat >"$dir/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *'rev-parse --git-dir') printf '%s\n' "${FAKE_GIT_DIR:?}" ;;
  *'rev-parse --git-common-dir') printf '%s\n' "${FAKE_GIT_COMMON:?}" ;;
  *) exit 99 ;;
esac
EOF
  chmod +x "$dir/make" "$dir/git"
  : >"$dir/make.log"
  chmod +x "$dir/make"
  : >"$dir/make.log"

  CLAUDE_PROJECT_DIR="$ROOT" \
    GIT_BIN="$dir/git" \
    MAKE_BIN="$dir/make" \
    FAKE_GIT_DIR=.git \
    FAKE_GIT_COMMON=.git \
    FAKE_MAKE_LOG="$dir/make.log" \
    "$SESSION_START" >/dev/null

  test "$(wc -l <"$dir/make.log" | tr -d ' ')" -eq 3 || \
    fail "session-start did not run check, rebuild, check"
  sed -n '1p' "$dir/make.log" | grep -Fq -- "check-binaries" || fail "first session-start command was not check-binaries"
  sed -n '2p' "$dir/make.log" | grep -Fq -- "rebuild-host-bin rebuild-atlas-bins rebuild-cron-bins" || \
    fail "second session-start command was not the docker-free host/bin rebuild (got: $(sed -n '2p' "$dir/make.log"))"
  sed -n '3p' "$dir/make.log" | grep -Fq -- "check-binaries" || fail "third session-start command was not check-binaries"

  : >"$dir/make.log"
  CLAUDE_PROJECT_DIR="$ROOT" \
    GIT_BIN="$dir/git" \
    MAKE_BIN="$dir/make" \
    FAKE_GIT_DIR=.git/worktrees/test \
    FAKE_GIT_COMMON=.git \
    FAKE_MAKE_LOG="$dir/make.log" \
    "$SESSION_START" >/dev/null
  test ! -s "$dir/make.log" || fail "linked worktree session-start modified shared Docker state"
}

# ── Freshness rule: judge the binary against the last BUILD INPUT commit ────
# 2026-09-23: `make check-binaries` went red whenever HEAD had moved past the
# deployed binaries, even when the commits in between could not possibly
# change a binary (docker-compose*.yml, docs/**, scripts/** other than
# cron-entrypoint.sh). That is the state the project is in most of the time,
# so the gate stopped carrying information. These cases pin the corrected rule
# down with a throwaway git repo + a docker stub, so they need no daemon.

# git with an identity/commit shape that never depends on the developer config.
fake_git() {
  local repo=$1
  shift
  git -C "$repo" -c commit.gpgsign=false -c user.email=test@example.com \
    -c user.name=test "$@"
}

# Build <dir>/repo with this history:
#   FAKE_EARLY_COMMIT  docs/seed.md                      (not a build input)
#   FAKE_BUILD_COMMIT  main.go                           (last build input)
#   FAKE_HEAD_COMMIT   docker-compose.yml + docs/readme.md  (HEAD, no build input)
make_fake_repo() {
  local dir=$1
  local fake_top root_top
  FAKE_REPO="$dir/repo"
  mkdir -p "$FAKE_REPO/docs"
  fake_git "$FAKE_REPO" init -q

  # Own-repo assertion (issue #1927). If a hook exported GIT_DIR, `git -C
  # <temp>` still acts on the caller's repo — `git init` then does not even
  # create <temp>/.git, and the fixture commits below would land on the branch
  # under test. Fail loudly and never commit.
  test -d "$FAKE_REPO/.git" || \
    fail "throwaway repo $FAKE_REPO has no .git directory (GIT_DIR leak?)"
  fake_top=$(physical_path "$FAKE_REPO")
  root_top=$(physical_path "$ROOT")
  test "$(cd "$FAKE_REPO" && git rev-parse --show-toplevel)" = "$fake_top" || \
    fail "throwaway repo $FAKE_REPO does not resolve to itself (GIT_DIR leak?); refusing to commit fixtures"
  test "$fake_top" != "$root_top" || \
    fail "throwaway repo $FAKE_REPO resolves to the caller's checkout; refusing to commit fixtures"

  printf 'seed\n' >"$FAKE_REPO/docs/seed.md"
  fake_git "$FAKE_REPO" add docs/seed.md
  fake_git "$FAKE_REPO" commit -q -m "docs seed"
  FAKE_EARLY_COMMIT=$(git -C "$FAKE_REPO" rev-parse HEAD)

  printf 'package main\n' >"$FAKE_REPO/main.go"
  fake_git "$FAKE_REPO" add main.go
  fake_git "$FAKE_REPO" commit -q -m "go change"
  FAKE_BUILD_COMMIT=$(git -C "$FAKE_REPO" rev-parse HEAD)

  printf 'services: {}\n' >"$FAKE_REPO/docker-compose.yml"
  printf 'readme\n' >"$FAKE_REPO/docs/readme.md"
  fake_git "$FAKE_REPO" add docker-compose.yml docs/readme.md
  fake_git "$FAKE_REPO" commit -q -m "compose + docs only"
  FAKE_HEAD_COMMIT=$(git -C "$FAKE_REPO" rev-parse HEAD)
}

# docker stub reporting FAKE_BINARY_COMMIT_LINE as every binary's buildinfo.
write_freshness_stub_docker() {
  cat >"$1/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  create) printf 'fake-container\n' ;;
  cp) printf '%s\n' "${FAKE_BINARY_COMMIT_LINE}" >"${@: -1}" ;;
  *) : ;;
esac
EOF
  chmod +x "$1/docker"
}

# Run the checker with cwd=$1 and the docker stub reporting $2, from an
# isolated copy of the script ($3/tool) so its REPO_ROOT — used only for
# bin/atlas-mcp — is not this checkout. Prints output; returns its exit status.
run_check_with_stub() {
  local cwd=$1
  local commit_line=$2
  local dir=$3
  mkdir -p "$dir/freshness"
  ( cd "$cwd" && \
    FAKE_BINARY_COMMIT_LINE="$commit_line" \
    DOCKER_BIN="$dir/docker" \
    FRESHNESS_TMPDIR="$dir/freshness" \
    bash "$dir/tool/check-binary-freshness.sh" 2>&1 )
}

run_freshness_rule_tests() {
  local dir out status
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  make_fake_repo "$dir"
  mkdir -p "$dir/tool"
  cp "$CHECK" "$dir/tool/check-binary-freshness.sh"
  write_freshness_stub_docker "$dir"

  # (a) HEAD leads the last build-input commit, but the commits in between only
  #     touched docker-compose.yml + docs → FRESH. This is the exact state that
  #     used to be reported as STALE (2026-09-23: binary=7126c3a7, HEAD=17e67439).
  if out=$(run_check_with_stub "$FAKE_REPO" "Commit=$FAKE_BUILD_COMMIT" "$dir"); then
    status=0
  else
    status=$?
  fi
  test "$status" -eq 0 || fail "(a) binary built at the last build-input commit was reported STALE (exit $status): $out"
  printf '%s\n' "$out" | grep -Fq -- "last build input commit: $FAKE_BUILD_COMMIT" || \
    fail "(a) checker did not print the last build input commit"

  # (b) The last build-input commit is NEWER than the binary → STALE.
  if out=$(run_check_with_stub "$FAKE_REPO" "Commit=$FAKE_EARLY_COMMIT" "$dir"); then
    status=0
  else
    status=$?
  fi
  test "$status" -eq 1 || fail "(b) binary older than the last build-input commit was not reported STALE (exit $status): $out"
  printf '%s\n' "$out" | grep -Fq -- "STALE" || fail "(b) STALE did not appear in the summary"
  printf '%s\n' "$out" | grep -Fq -- "last build input=$FAKE_BUILD_COMMIT" || \
    fail "(b) STALE reason did not name the last build-input commit"

  # (c) Binary without buildinfo.Commit → MISSING_BUILDINFO, still exit 1.
  if out=$(run_check_with_stub "$FAKE_REPO" "no buildinfo in this binary" "$dir"); then
    status=0
  else
    status=$?
  fi
  test "$status" -eq 1 || fail "(c) binary without buildinfo was not reported missing (exit $status): $out"
  printf '%s\n' "$out" | grep -Fq -- "MISSING_BUILDINFO" || fail "(c) MISSING_BUILDINFO did not appear in the summary"

  # (d) A build commit this clone does not know cannot be proven fresh.
  if out=$(run_check_with_stub "$FAKE_REPO" "Commit=deadbeefdeadbeefdeadbeefdeadbeefdeadbeef" "$dir"); then
    status=0
  else
    status=$?
  fi
  test "$status" -eq 1 || fail "(d) unknown build commit was accepted as FRESH (exit $status): $out"

  # (e) Real history: a binary built at this repo's last build-input commit is
  #     FRESH even though HEAD is ahead of it. Guards against the checker
  #     silently going back to "binary commit must equal HEAD".
  local real_build
  real_build=$(git -C "$ROOT" log -1 --format=%H -- '*.go' go.mod go.sum 'Dockerfile*' scripts/cron-entrypoint.sh)
  if [ -n "$real_build" ]; then
    if out=$(run_check_with_stub "$ROOT" "Commit=$real_build" "$dir"); then
      status=0
    else
      status=$?
    fi
    test "$status" -eq 0 || fail "(e) this repo's last build-input commit was reported STALE (exit $status): $out"
    printf '%s\n' "$out" | grep -Fq -- "last build input commit: $real_build" || \
      fail "(e) checker disagreed with 'git log -1 -- *.go go.mod go.sum Dockerfile* scripts/cron-entrypoint.sh'"
  fi
}

# ── --diff-base: "git diff failed" must not look like "no build input" ──────
# 2026-10-16 (FU-20261016-27): the flag's whole purpose is to SKIP the check when
# a push cannot invalidate a binary. The skip test was
#   changed=$(git diff --name-only "${DIFF_BASE}...HEAD" -- … 2>/dev/null || true)
# so `git diff` failing (no merge base — the guard above only proves the REF
# resolves, not that `<base>...HEAD` does) produced the same empty list as a
# genuinely build-input-free push and the gate exited 0 saying "host binary
# freshness not required". These cases pin both sides: git failing ⇒ rc=2 (fail
# loud), a real build-input-free push ⇒ still rc=0 (the fix must not turn the
# documented skip into a blanket red).

# Build <dir>/repo2: docs seed → main.go → compose-only, plus an orphan branch
# that shares no commit with main (the no-merge-base shape).
make_diff_base_repo() {
  local dir=$1
  local repo="$dir/repo2"
  mkdir -p "$repo/docs"
  # -b main: the cases below name `main` explicitly (an implicit `master` would
  # make every range in this section unresolvable for the wrong reason).
  fake_git "$repo" init -q -b main
  test -d "$repo/.git" || fail "diff-base fixture has no .git directory (GIT_DIR leak?)"
  printf 'seed\n' >"$repo/docs/seed.md"
  fake_git "$repo" add docs/seed.md
  fake_git "$repo" commit -q -m "docs seed"
  # (checked after the first commit: on an unborn branch rev-parse has no HEAD)
  test "$(git -C "$repo" rev-parse --abbrev-ref HEAD)" = "main" || \
    fail "diff-base fixture is not on main (git init -b main failed)"
  printf 'package main\n' >"$repo/main.go"
  fake_git "$repo" add main.go
  fake_git "$repo" commit -q -m "go change"
  printf 'services: {}\n' >"$repo/docker-compose.yml"
  fake_git "$repo" add docker-compose.yml
  fake_git "$repo" commit -q -m "compose only"
  fake_git "$repo" checkout -q --orphan orphan
  fake_git "$repo" rm -rq --cached . >/dev/null 2>&1 || true
  rm -rf "$repo/docs"
  printf 'package orphan\n' >"$repo/orphan.go"
  fake_git "$repo" add -A
  fake_git "$repo" commit -q -m "orphan root with a go file"
  # Back to main: cases (a)/(b)/(d) need a HEAD whose history is main's. Case (c)
  # switches to `orphan` itself (and asserts that it is on it).
  fake_git "$repo" checkout -q main
}

run_diff_base_tests() {
  local dir out status
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  make_diff_base_repo "$dir"
  local repo="$dir/repo2"
  mkdir -p "$dir/tool" "$dir/freshness"
  cp "$CHECK" "$dir/tool/check-binary-freshness.sh"

  run_diff_base() {  # run_diff_base <cwd> [args...] → sets DIFF_OUT / DIFF_RC
    local cwd=$1
    shift
    DIFF_RC=0
    DIFF_OUT=$( cd "$cwd" && FRESHNESS_TMPDIR="$dir/freshness" \
      bash "$dir/tool/check-binary-freshness.sh" --host-only "$@" 2>&1 ) || DIFF_RC=$?
  }

  # (a) resolvable range whose diff really has no build input ⇒ documented skip.
  run_diff_base "$repo" --diff-base "HEAD~1"
  test "$DIFF_RC" -eq 0 || fail "(a) a build-input-free range must still skip (exit 0), got $DIFF_RC: $DIFF_OUT"
  printf '%s\n' "$DIFF_OUT" | grep -Fq -- "no build input changed" || \
    fail "(a) expected the documented skip message, got: $DIFF_OUT"

  # (b) resolvable range that DOES move a build input ⇒ the gate judges.
  run_diff_base "$repo" --diff-base "HEAD~2"
  test "$DIFF_RC" -eq 0 || fail "(b) a build-input range must be evaluated (exit 0 here), got $DIFF_RC: $DIFF_OUT"
  printf '%s\n' "$DIFF_OUT" | grep -Fq -- "build input(s) changed" || \
    fail "(b) the gate did not report the changed build inputs: $DIFF_OUT"
  printf '%s\n' "$DIFF_OUT" | grep -Fq -- "no build input changed" && \
    fail "(b) a build-input range was reported as 'no build input changed': $DIFF_OUT"

  # (e) pre-existing guard kept: a --diff-base that does not resolve is still 2.
  run_diff_base "$repo" --diff-base "does-not-exist-xyz"
  test "$DIFF_RC" -eq 2 || fail "(e) an unresolvable --diff-base must exit 2, got $DIFF_RC: $DIFF_OUT"
  printf '%s\n' "$DIFF_OUT" | grep -Fq -- "does not resolve" || \
    fail "(e) expected the unresolved-ref message, got: $DIFF_OUT"

  # (d) seam: the range IS valid — git itself fails (broken GIT_DIR, no git).
  local fake="$dir/fakebin"
  mkdir -p "$fake"
  {
    printf '#!/usr/bin/env bash\n'
    printf 'case "$1" in\n'
    printf '  diff) echo "fatal: injected git failure (test seam)" >&2; exit 128 ;;\n'
    printf '  *) exec "%s" "$@" ;;\n' "$(command -v git)"
    printf 'esac\n'
  } >"$fake/git"
  chmod +x "$fake/git"
  DIFF_RC=0
  DIFF_OUT=$( cd "$repo" && PATH="$fake:$PATH" FRESHNESS_TMPDIR="$dir/freshness" \
    bash "$dir/tool/check-binary-freshness.sh" --host-only --diff-base "HEAD~1" 2>&1 ) || DIFF_RC=$?
  test "$DIFF_RC" -eq 2 || fail "(d) an injected git failure must exit 2, got $DIFF_RC: $DIFF_OUT"
  printf '%s\n' "$DIFF_OUT" | grep -Fq -- "128" || \
    fail "(d) the failure should report git's status, got: $DIFF_OUT"
  printf '%s\n' "$DIFF_OUT" | grep -Fq -- "no build input changed" && \
    fail "(d) an injected git failure was reported as 'no build input changed': $DIFF_OUT"

  # (c) the defect: ${DIFF_BASE}^{commit} resolves, but `${DIFF_BASE}...HEAD` has
  #     no merge base (shallow / cut history) ⇒ rc=2, never the skip. This half
  #     of the suite runs on the orphan branch (HEAD shares no commit with main).
  fake_git "$repo" checkout -q orphan
  test "$(git -C "$repo" rev-parse --abbrev-ref HEAD)" = "orphan" || \
    fail "(c) fixture must be on the orphan branch"
  git -C "$repo" rev-parse --verify -q "main^{commit}" >/dev/null || \
    fail "(c) fixture must resolve main^{commit}"
  ( cd "$repo" && git diff --name-only "main...HEAD" >/dev/null 2>&1 ) && \
    fail "(c) fixture unexpectedly resolves main...HEAD (no-merge-base case lost)"
  run_diff_base "$repo" --diff-base "main"
  test "$DIFF_RC" -eq 2 || fail "(c) a diff git cannot resolve must exit 2 (fail loud), got $DIFF_RC: $DIFF_OUT"
  printf '%s\n' "$DIFF_OUT" | grep -Fq -- "main...HEAD" || \
    fail "(c) failure must name the unusable range, got: $DIFF_OUT"
  printf '%s\n' "$DIFF_OUT" | grep -Fq -- "no build input changed" && \
    fail "(c) an unresolvable range was reported as 'no build input changed': $DIFF_OUT"
  printf '%s\n' "$DIFF_OUT" | grep -Fq -- "ALL BINARIES FRESH" && \
    fail "(c) an unresolvable range produced a green summary: $DIFF_OUT"

  # (f) mutation self-proof: the OLD pipeline shape reproduces the false green on
  #     the same fixture, so (c) really distinguishes old code from new code.
  cat >"$dir/old-shape.sh" <<'OLD'
#!/usr/bin/env bash
set -euo pipefail
DIFF_BASE=$1
BUILD_INPUT_PATHS=('*.go' 'go.mod' 'go.sum' 'Dockerfile*' 'scripts/cron-entrypoint.sh')
changed=$(git diff --name-only "${DIFF_BASE}...HEAD" -- "${BUILD_INPUT_PATHS[@]}" 2>/dev/null || true)
if [ -z "$changed" ]; then
  echo "  ℹ no build input changed in ${DIFF_BASE}...HEAD → host binary freshness not required for this push"
  exit 0
fi
echo "  build input(s) changed"
OLD
  local old_rc=0 old_out
  old_out=$( cd "$repo" && bash "$dir/old-shape.sh" "main" 2>&1 ) || old_rc=$?
  test "$old_rc" -eq 0 || fail "(f) the old shape must reproduce the false green (exit 0), got $old_rc: $old_out"
  printf '%s\n' "$old_out" | grep -Fq -- "no build input changed" || \
    fail "(f) the old shape must print the skip message, got: $old_out"

  # (g) static: the fail-open shape must not come back.
  grep -Fq 'git diff --name-only "${DIFF_BASE}...HEAD" -- "${BUILD_INPUT_PATHS[@]}" 2>/dev/null || true' "$CHECK" && \
    fail "(g) check-binary-freshness.sh still contains the fail-open git-diff pipeline"
  grep -Fq 'DIFF_RC' "$CHECK" || \
    fail "(g) check-binary-freshness.sh must capture git diff's status explicitly"
  echo "  ✓ --diff-base: real skip ⇒ rc=0 / unresolvable or failing diff ⇒ rc=2 / old shape ⇒ rc=0 (mutation)"
}

run_static_contract_tests() {
  assert_contains "$ROOT/docker-compose.yml" 'GIT_COMMIT: ${ATLAS_GIT_COMMIT:?'
  assert_contains "$ROOT/Dockerfile" 'GIT_COMMIT must be set'
  assert_contains "$ROOT/Dockerfile.cron" 'GIT_COMMIT must be set'
  assert_contains "$ROOT/.claude/settings.json" 'scripts/session-start.sh'
  for binary in atlas-go daily-replay-sync atlas-mcp calibrate-parameters calibrate-seasonal; do
    assert_contains "$ROOT/Dockerfile" "-o $binary"
  done
  assert_contains "$ROOT/docker-compose.yml" 'prism-worker:'
  assert_contains "$ROOT/.claude/settings.json" '"timeout": 600'
  for binary in daily-replay-sync calibrate-parameters calibrate-seasonal; do
    assert_contains "$ROOT/scripts/check-binary-freshness.sh" "/app/$binary"
  done
  assert_contains "$ROOT/scripts/deploy-staging.sh" 'ATLAS_GIT_COMMIT="$(git rev-parse HEAD)"'
  # The freshness gate judges against the last build input, not HEAD.
  assert_contains "$ROOT/scripts/check-binary-freshness.sh" "LAST_BUILD_COMMIT"
  assert_contains "$ROOT/scripts/check-binary-freshness.sh" "'scripts/cron-entrypoint.sh'"
  # This test builds throwaway repos, so it must drop a hook's GIT_DIR and
  # assert the fixtures resolve to themselves (issue #1927).
  assert_contains "$ROOT/tests/scripts/test-binary-freshness-guard.sh" 'unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE'
  assert_contains "$ROOT/tests/scripts/test-binary-freshness-guard.sh" 'rev-parse --show-toplevel'
  # #1898 bug class: a cron service declared in docker-compose.yml but missing
  # from CRON_IMAGE_TAGS only blows up at deploy time. The retag expectation
  # must therefore be derived from compose, never from the Makefile list that
  # is the thing possibly being wrong.
  assert_contains "$ROOT/tests/scripts/test-binary-freshness-guard.sh" 'could not derive cron services from docker-compose.yml'
  # The cleanup assertion must compare container NAMES (the rm log used to be
  # filled with "-f", so a count-only assertion proved nothing).
  assert_contains "$ROOT/tests/scripts/test-binary-freshness-guard.sh" 'did not remove temporary container'
  # session-start must be pinned to the docker-free host/bin rebuild contract.
  assert_contains "$ROOT/tests/scripts/test-binary-freshness-guard.sh" 'rebuild-host-bin rebuild-atlas-bins rebuild-cron-bins'
  # Only this hermetic test is wired into ci-gate; the generic test-scripts
  # target stays opt-in (issue #1927: ci-gate runs from the pre-push hook,
  # which is exactly where GIT_DIR gets exported).
  assert_contains "$ROOT/Makefile" 'tests/scripts/test-binary-freshness-guard.sh'
}

run_cron_retag_test
run_cleanup_failure_test
run_missing_image_test
run_success_cleanup_test
run_freshness_rule_tests
# Static guards for the "atlas family" image tags (2026-09-24, #1938).
#
# Why: `rebuild-atlas` builds ONE image (atlas-atlas:local) and retags it for
# every build-only service in the atlas family; `compose up -d` then reuses the
# existing tag. If a service is missing from ATLAS_IMAGE_TAGS, compose silently
# keeps a STALE image and `check-binaries` (which is image-list driven) can still
# report FRESH. 實證 2026-09-23: atlas-prism-worker 停在 7126c3a7，atlas-go 已
# 在 91fd474f。
#
# Two checks, both derived from the repo (no magic lists):
#   1) every service in `rebuild-atlas`'s `compose up -d <services>` list has an
#      ATLAS_IMAGE_TAGS entry ("atlas-<service>:latest");
#   2) every build-only compose service that is NOT a cron service (cron images
#      are covered by CRON_IMAGE_TAGS / the retag test) and NOT fubon-proxy
#      (its own Dockerfile/image) appears in that same up-list — so a newly
#      added build-only service cannot be silently left untagged.
run_atlas_family_tag_test() {
  local up_list declared svc img missing=0

  # Only the rebuild-atlas target block, skipping comment lines (the Makefile has
  # comments mentioning `compose up -d` earlier, e.g. the iMac guard rationale).
  # Require the literal `docker compose up -d ...` command form so Makefile
  # comments (which mention `compose up -d` inside `@#` lines) cannot pollute the
  # derived list.
  up_list=$(sed -n '/^rebuild-atlas:/,/^$/p' "$ROOT/Makefile" \
    | grep -o 'docker compose up -d [^"]*' \
    | sed 's/^.*docker compose up -d //' | tr ' ' '\n' | grep -E '^[a-z0-9-]+$' \
    | grep -vE '^(-d|--force-recreate|--no-build)$' | sort -u)
  [ -n "$up_list" ] || fail "could not derive the rebuild-atlas compose up service list"

  declared=$(grep -m1 '^ATLAS_IMAGE_TAGS :=' "$ROOT/Makefile" | cut -d= -f2- \
    | tr ' ' '\n' | grep -E ':latest$' | sort -u)
  [ -n "$declared" ] || fail "Makefile has no ATLAS_IMAGE_TAGS entries"

  # (1) every service the rebuild target brings up must be tagged
  for svc in $up_list; do
    case "$svc" in
      cron-*|atlas-cron-*) continue ;;   # cron family: covered by CRON_IMAGE_TAGS
    esac
    printf '%s\n' "$declared" | grep -qx -- "atlas-${svc}:latest" || {
      echo "    ✗ ATLAS_IMAGE_TAGS missing atlas-${svc}:latest (rebuild-atlas brings up '$svc')" >&2
      missing=1
    }
  done
  [ "$missing" -eq 0 ] || fail "ATLAS_IMAGE_TAGS does not cover every atlas-family service rebuilt by rebuild-atlas"

  # (2) completeness: no build-only service may be omitted from the up-list
  awk '
    /^  [a-z0-9-]+:[[:space:]]*$/ { svc=$1; sub(/:$/, "", svc); next }
    /dockerfile:/ && svc != "" { print svc; svc="" }
  ' "$ROOT/docker-compose.yml" | sort -u >"$TMPDIR_ATLAS_SVCS"
  while IFS= read -r svc; do
    case "$svc" in
      cron-*|atlas-cron-*|fubon-proxy) continue ;;
    esac
    printf '%s\n' "$up_list" | grep -qx -- "$svc" || \
      fail "build-only compose service '$svc' is not brought up by rebuild-atlas (add it + its ATLAS_IMAGE_TAGS entry)"
  done <"$TMPDIR_ATLAS_SVCS"
  rm -f "$TMPDIR_ATLAS_SVCS"
}

run_atlas_family_tag_test
run_diff_base_tests
run_session_start_test
run_static_contract_tests

echo "PASS: binary freshness guard contract tests"
